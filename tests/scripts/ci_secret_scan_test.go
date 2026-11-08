package scripts_test

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// TestCISecretScanScopes pins the Gitleaks policy in the workflow:
//
//   - PRs scan only base..head (with merge diffs) and never unrelated refs.
//   - Protected branch/tag pushes scan the full history reachable from the
//     pushed protected ref, not every ref fetched by the checkout.
//   - Feature branch pushes scan only the pushed range, in the dedicated
//     secret-scan-feature.yml workflow.
//   - Missing or unresolved SHAs fail closed.
func TestCISecretScanScopes(t *testing.T) {
	workflow := readCIWorkflow(t)
	required := []string{
		"if: github.event_name == 'push' && (github.ref == 'refs/heads/development' || github.ref == 'refs/heads/main' || github.ref_type == 'tag')",
		"PROTECTED_SHA: ${{ github.sha }}",
		`git cat-file -e "${PROTECTED_SHA}^{commit}"`,
		`--log-opts="${PROTECTED_SHA} -m"`,
		"if: github.event_name == 'pull_request'",
		"PR_BASE_SHA: ${{ github.event.pull_request.base.sha }}",
		"PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}",
		`git cat-file -e "${PR_BASE_SHA}^{commit}"`,
		`git cat-file -e "${PR_HEAD_SHA}^{commit}"`,
		`PR_CONFIG="$(mktemp)"`,
		`trap 'rm -f "${PR_CONFIG}"' EXIT`,
		`git cat-file -e "${PR_BASE_SHA}:.gitleaks.toml"`,
		`git show "${PR_BASE_SHA}:.gitleaks.toml" > "${PR_CONFIG}"`,
		`--config "${PR_CONFIG}"`,
		"::error::The PR base has no .gitleaks.toml; refusing to scan",
		`--log-opts="${PR_BASE_SHA}..${PR_HEAD_SHA} -m"`,
	}
	for _, want := range required {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow is missing required secret-scan policy fragment:\n%s", want)
		}
	}

	if strings.Contains(workflow, "github.event_name != 'pull_request'") {
		t.Fatal("feature-branch pushes must not fall through to a full-history scan")
	}
	if strings.Contains(workflow, "github.ref_type == 'branch'") {
		t.Fatal("feature-branch pushes belong to secret-scan-feature.yml, not ci.yml")
	}
	if count := strings.Count(workflow, "gitleaks git"); count != 2 {
		t.Fatalf("ci.yml has %d gitleaks invocations, want 2 scoped invocations", count)
	}
	if count := strings.Count(workflow, "--log-opts="); count != 2 {
		t.Fatalf("ci.yml has %d --log-opts scopes, want 2 (no unscoped scan)", count)
	}
	if count := strings.Count(workflow, "--config .gitleaks.toml"); count != 1 {
		t.Fatalf("ci.yml has %d untrusted .gitleaks.toml configs, want 1 (protected ref only)", count)
	}
	if count := strings.Count(workflow, `--config "${PR_CONFIG}"`); count != 1 {
		t.Fatalf("ci.yml has %d trusted PR configs, want 1", count)
	}

	featurePath := filepath.Join(repositoryRoot(t), ".github/workflows/secret-scan-feature.yml")
	featureWorkflowBytes, err := os.ReadFile(featurePath)
	if err != nil {
		t.Fatalf("read feature secret-scan workflow: %v", err)
	}
	featureWorkflow := string(featureWorkflowBytes)
	featureRequired := []string{
		"name: Secret scanning (feature branches)",
		"branches-ignore:",
		"- development",
		"- main",
		"PUSH_BEFORE_SHA: ${{ github.event.before }}",
		"PUSH_AFTER_SHA: ${{ github.event.after }}",
		`git cat-file -e "${PUSH_AFTER_SHA}^{commit}"`,
		"git merge-base origin/development",
		`git cat-file -e "${BASE_SHA}^{commit}"`,
		`BASE_CONFIG="$(mktemp)"`,
		`trap 'rm -f "${BASE_CONFIG}"' EXIT`,
		`git cat-file -e "origin/development:.gitleaks.toml"`,
		`git show "origin/development:.gitleaks.toml" > "${BASE_CONFIG}"`,
		`--config "${BASE_CONFIG}"`,
		"::error::The trusted development Gitleaks config is missing; refusing to scan",
		`--log-opts="${BASE_SHA}..${PUSH_AFTER_SHA} -m"`,
	}
	for _, want := range featureRequired {
		if !strings.Contains(featureWorkflow, want) {
			t.Fatalf("feature secret-scan workflow is missing required fragment:\n%s", want)
		}
	}
	if count := strings.Count(featureWorkflow, "gitleaks git"); count != 1 {
		t.Fatalf("feature workflow has %d gitleaks invocations, want 1", count)
	}
	if count := strings.Count(featureWorkflow, "--log-opts="); count != 1 {
		t.Fatalf("feature workflow has %d --log-opts scopes, want 1", count)
	}
	if count := strings.Count(featureWorkflow, "--config .gitleaks.toml"); count != 0 {
		t.Fatalf("feature workflow has %d untrusted .gitleaks.toml configs, want 0", count)
	}
	if count := strings.Count(featureWorkflow, `--config "${BASE_CONFIG}"`); count != 1 {
		t.Fatalf("feature workflow has %d trusted development configs, want 1", count)
	}
	for _, pinned := range []string{
		"GITLEAKS_VERSION: '8.30.1'",
		"GITLEAKS_LINUX_X64_SHA256: '551f6fc83ea457d62a0d98237cbad105af8d557003051f41f3e7ca7b3f2470eb'",
	} {
		if !strings.Contains(workflow, pinned) || !strings.Contains(featureWorkflow, pinned) {
			t.Fatalf("both secret-scan workflows must pin %q", pinned)
		}
	}

	config, err := os.ReadFile(filepath.Join(repositoryRoot(t), ".gitleaks.toml"))
	if err != nil {
		t.Fatalf("read .gitleaks.toml: %v", err)
	}
	if strings.Contains(string(config), "apps/services/realtime/internal/ws/handler_test.go") {
		t.Fatal("Gitleaks config must not allowlist the unrelated realtime fixture")
	}
}

// TestGitleaksCurrentPRRange is an opt-in local/CI-with-binary check. It is
// skipped unless GITLEAKS_BIN points at the pinned Gitleaks binary.
func TestGitleaksCurrentPRRange(t *testing.T) {
	bin := gitleaksBinary(t)
	repo := repositoryRoot(t)

	base := gitOutput(t, repo, "rev-parse", "--verify", "origin/development")
	head := gitOutput(t, repo, "rev-parse", "--verify", "HEAD")

	trustedConfig := filepath.Join(t.TempDir(), ".gitleaks.toml")
	if err := os.WriteFile(trustedConfig, []byte(gitOutput(t, repo, "show", base+":.gitleaks.toml")), 0o600); err != nil {
		t.Fatalf("write trusted base config: %v", err)
	}
	if code := runGitleaksWithConfig(t, bin, repo, trustedConfig, base+".."+head+" -m"); code != 0 {
		t.Fatalf("gitleaks found leaks in the PR range %s..%s with the trusted base config (exit %d)", base[:12], head[:12], code)
	}
}

// TestGitleaksMergeOnlySecretDetected retains the merge-commit regression
// test: a secret introduced only while resolving a merge conflict is missed
// without -m and detected with -m.
func TestGitleaksMergeOnlySecretDetected(t *testing.T) {
	bin := gitleaksBinary(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "checkout", "-q", "-b", "main")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test")

	writeTestFile(t, repo, "config.txt", "value = \"safe\"\n")
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "base")
	base := gitOutput(t, repo, "rev-parse", "HEAD")

	gitRun(t, repo, "checkout", "-q", "-b", "feature")
	writeTestFile(t, repo, "config.txt", "value = \"feature\"\n")
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "feature")

	gitRun(t, repo, "checkout", "-q", "main")
	writeTestFile(t, repo, "config.txt", "value = \"main\"\n")
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "main")

	merge := exec.Command("git", "-C", repo, "merge", "feature")
	if err := merge.Run(); err == nil {
		t.Fatal("expected a merge conflict in the test repository")
	}

	writeTestFile(t, repo, "config.txt", fmt.Sprintf("api_key = %q\n", testSecret(t)))
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "merge")
	head := gitOutput(t, repo, "rev-parse", "HEAD")

	if code := runGitleaks(t, bin, repo, base+".."+head); code != 0 {
		t.Fatalf("gitleaks without -m exited %d, want 0 (merge-only secret should be missed)", code)
	}
	if code := runGitleaks(t, bin, repo, base+".."+head+" -m"); code != 1 {
		t.Fatalf("gitleaks with -m exited %d, want 1 (merge-only secret must be detected)", code)
	}
}

// TestGitleaksProtectedRefScope pins the protected-ref boundary: an unmerged
// feature branch must not fail the protected-branch job, while a secret on the
// protected ref itself must still be detected.
func TestGitleaksProtectedRefScope(t *testing.T) {
	bin := gitleaksBinary(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "checkout", "-q", "-b", "development")
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test")

	writeTestFile(t, repo, "safe.txt", "value = \"safe\"\n")
	gitRun(t, repo, "add", "safe.txt")
	gitRun(t, repo, "commit", "-q", "-m", "protected base")

	// An unmerged feature branch carries a synthetic secret.
	gitRun(t, repo, "checkout", "-q", "-b", "feature")
	writeTestFile(t, repo, "config.txt", fmt.Sprintf("api_key = %q\n", testSecret(t)))
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "feature secret")

	// The protected branch advances without that commit.
	gitRun(t, repo, "checkout", "-q", "development")
	writeTestFile(t, repo, "safe.txt", "value = \"still safe\"\n")
	gitRun(t, repo, "add", "safe.txt")
	gitRun(t, repo, "commit", "-q", "-m", "protected head")
	protectedHead := gitOutput(t, repo, "rev-parse", "HEAD")

	if code := runGitleaks(t, bin, repo, protectedHead+" -m"); code != 0 {
		t.Fatalf("protected-ref scan exited %d, want 0; unmerged refs must not be scanned", code)
	}
	if code := runGitleaks(t, bin, repo, "--all"); code != 1 {
		t.Fatalf("all-ref scan exited %d, want 1; the fixture must remain detectable", code)
	}

	// A secret introduced on the protected ref itself is still caught.
	writeTestFile(t, repo, "config.txt", fmt.Sprintf("api_key = %q\n", testSecret(t)))
	gitRun(t, repo, "add", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "protected secret")
	protectedSecretHead := gitOutput(t, repo, "rev-parse", "HEAD")

	if code := runGitleaks(t, bin, repo, protectedSecretHead+" -m"); code != 1 {
		t.Fatalf("protected-ref scan exited %d, want 1; protected history must be scanned", code)
	}
}

// TestGitleaksPRConfigBypass proves a PR cannot suppress its own introduced
// secret by editing .gitleaks.toml: the head config allows it, while the
// trusted base config still detects it. The workflow pins the trusted config.
func TestGitleaksPRConfigBypass(t *testing.T) {
	testGitleaksConfigBypass(t, "main", "pr")
}

// TestGitleaksFeatureConfigBypass repeats the proof for a feature branch whose
// trusted config comes from development.
func TestGitleaksFeatureConfigBypass(t *testing.T) {
	testGitleaksConfigBypass(t, "development", "feature")
}

func testGitleaksConfigBypass(t *testing.T, baseBranch, featureBranch string) {
	t.Helper()
	bin := gitleaksBinary(t)
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not available")
	}

	repo := t.TempDir()
	gitRun(t, repo, "init", "-q")
	gitRun(t, repo, "checkout", "-q", "-b", baseBranch)
	gitRun(t, repo, "config", "user.email", "test@example.com")
	gitRun(t, repo, "config", "user.name", "Test")

	trusted := "[extend]\nuseDefault = true\n"
	writeTestFile(t, repo, ".gitleaks.toml", trusted)
	gitRun(t, repo, "add", ".gitleaks.toml")
	gitRun(t, repo, "commit", "-q", "-m", "trusted config")
	base := gitOutput(t, repo, "rev-parse", "HEAD")

	gitRun(t, repo, "checkout", "-q", "-b", featureBranch)
	writeTestFile(t, repo, "config.txt", fmt.Sprintf("api_key = %q\n", testSecret(t)))
	writeTestFile(t, repo, ".gitleaks.toml", trusted+`
[[rules]]
id = "generic-api-key"
[[rules.allowlists]]
condition = "AND"
regexTarget = "match"
paths = ['''^config\.txt$''']
regexes = ['''api_key\s*=\s*"''']
`)
	gitRun(t, repo, "add", ".gitleaks.toml", "config.txt")
	gitRun(t, repo, "commit", "-q", "-m", "feature secret with allowlist")
	head := gitOutput(t, repo, "rev-parse", "HEAD")

	if code := runGitleaksWithConfig(t, bin, repo, filepath.Join(repo, ".gitleaks.toml"), base+".."+head+" -m"); code != 0 {
		t.Fatalf("feature-controlled config exited %d, want 0 (the bypass must be demonstrable)", code)
	}

	trustedPath := filepath.Join(t.TempDir(), ".gitleaks.toml")
	if err := os.WriteFile(trustedPath, []byte(gitOutput(t, repo, "show", base+":.gitleaks.toml")), 0o600); err != nil {
		t.Fatalf("write trusted base config: %v", err)
	}
	if code := runGitleaksWithConfig(t, bin, repo, trustedPath, base+".."+head+" -m"); code != 1 {
		t.Fatalf("trusted base config exited %d, want 1 (the secret must be detected)", code)
	}
}

func gitleaksBinary(t *testing.T) string {
	t.Helper()
	bin := strings.TrimSpace(os.Getenv("GITLEAKS_BIN"))
	if bin == "" {
		t.Skip("GITLEAKS_BIN is not set; skipping dynamic Gitleaks test")
	}
	if _, err := exec.LookPath(bin); err != nil {
		t.Skipf("GITLEAKS_BIN is not executable: %v", err)
	}
	return bin
}

func runGitleaks(t *testing.T, bin, repo, logOpts string) int {
	t.Helper()
	return runGitleaksWithConfig(t, bin, repo, filepath.Join(repositoryRoot(t), ".gitleaks.toml"), logOpts)
}

func runGitleaksWithConfig(t *testing.T, bin, repo, config, logOpts string) int {
	t.Helper()
	command := exec.Command(bin,
		"git",
		"--config", config,
		"--redact",
		"--no-banner",
		"--no-color",
		"--exit-code", "1",
		"--log-opts", logOpts,
		".",
	)
	command.Dir = repo
	var output bytes.Buffer
	command.Stdout = &output
	command.Stderr = &output
	err := command.Run()
	if err == nil {
		return 0
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode()
	}
	t.Fatalf("run gitleaks: %v", err)
	return -1
}

func gitRun(t *testing.T, repo string, args ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %s: %v\n%s", strings.Join(args, " "), err, output)
	}
}

func gitOutput(t *testing.T, repo string, args ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", repo}, args...)...)
	output, err := command.Output()
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return strings.TrimSpace(string(output))
}

func writeTestFile(t *testing.T, repo, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(repo, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

func testSecret(t *testing.T) string {
	t.Helper()
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		t.Fatalf("generate test secret: %v", err)
	}
	return hex.EncodeToString(buffer[:]) + "-test-secret"
}
