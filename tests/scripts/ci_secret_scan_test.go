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
//   - Protected branch/tag pushes retain full-history scanning.
//   - Feature branch pushes scan only the pushed range.
//   - Missing or unresolved SHAs fail closed.
func TestCISecretScanScopes(t *testing.T) {
	workflow := readCIWorkflow(t)
	required := []string{
		"if: github.event_name == 'push' && (github.ref == 'refs/heads/development' || github.ref == 'refs/heads/main' || github.ref_type == 'tag')",
		"if: github.event_name == 'pull_request'",
		"PR_BASE_SHA: ${{ github.event.pull_request.base.sha }}",
		"PR_HEAD_SHA: ${{ github.event.pull_request.head.sha }}",
		`git cat-file -e "${PR_BASE_SHA}^{commit}"`,
		`git cat-file -e "${PR_HEAD_SHA}^{commit}"`,
		`--log-opts="${PR_BASE_SHA}..${PR_HEAD_SHA} -m"`,
		"if: github.event_name == 'push' && github.ref_type == 'branch' && github.ref != 'refs/heads/development' && github.ref != 'refs/heads/main'",
		"PUSH_BEFORE_SHA: ${{ github.event.before }}",
		"PUSH_AFTER_SHA: ${{ github.event.after }}",
		`git cat-file -e "${PUSH_AFTER_SHA}^{commit}"`,
		"git merge-base origin/development",
		`--log-opts="${BASE_SHA}..${PUSH_AFTER_SHA} -m"`,
	}
	for _, want := range required {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow is missing required secret-scan policy fragment:\n%s", want)
		}
	}

	if strings.Contains(workflow, "github.event_name != 'pull_request'") {
		t.Fatal("feature-branch pushes must not fall through to a full-history scan")
	}
	if count := strings.Count(workflow, "gitleaks git"); count != 3 {
		t.Fatalf("workflow has %d gitleaks invocations, want 3 scoped invocations", count)
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

	if code := runGitleaks(t, bin, repo, base+".."+head+" -m"); code != 0 {
		t.Fatalf("gitleaks found leaks in the PR range %s..%s (exit %d)", base[:12], head[:12], code)
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
	config := filepath.Join(repositoryRoot(t), ".gitleaks.toml")
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
