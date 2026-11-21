package scripts_test

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"
)

const syftInstallScript = "scripts/install-syft.sh"

// TestCISyftInstallPolicy pins the supply-chain properties of the Syft
// installer and confirms the SBOM/Trivy pipeline is still present.
func TestCISyftInstallPolicy(t *testing.T) {
	workflow := readCIWorkflow(t)
	for _, want := range []string{
		"name: Install pinned Syft",
		"bash scripts/install-syft.sh",
		"name: Generate SBOM",
		"syft scan",
		"-o cyclonedx-json",
		`test -s "${output}"`,
		"aquasecurity/trivy-action@",
		"severity: HIGH,CRITICAL",
		"exit-code: 1",
		"if-no-files-found: error",
	} {
		if !strings.Contains(workflow, want) {
			t.Fatalf("workflow is missing Syft/SBOM policy fragment: %s", want)
		}
	}
	if strings.Contains(workflow, "anchore/sbom-action") {
		t.Fatal("workflow still uses the non-retrying Syft action installer")
	}
	if strings.Contains(workflow, "continue-on-error") {
		t.Fatal("SBOM/Trivy checks must not be allowed to continue on error")
	}

	scriptPath := filepath.Join(repositoryRoot(t), syftInstallScript)
	content, err := os.ReadFile(scriptPath)
	if err != nil {
		t.Fatalf("read %s: %v", syftInstallScript, err)
	}
	script := string(content)
	for _, want := range []string{
		`readonly SYFT_VERSION="v1.56.0"`,
		"readonly MAX_ATTEMPTS=5",
		"readonly INITIAL_DELAY_SECONDS=2",
		"delay=$((delay * 2))",
		"sha256sum --check --strict",
		"shasum -a 256 -c",
		"https://github.com/anchore/syft/releases/download/",
		"actual_version",
		`if [ "${actual_version}" != "${SYFT_VERSION_NO_V}" ]`,
		`install -m 0755`,
	} {
		if !strings.Contains(script, want) {
			t.Fatalf("Syft installer is missing required policy fragment: %s", want)
		}
	}
	if strings.Contains(script, "http://") {
		t.Fatal("Syft installer must use the official HTTPS release source only")
	}
	checksum := regexp.MustCompile(`(?m)^readonly SYFT_SHA256="([0-9a-f]{64})"$`).FindStringSubmatch(script)
	if len(checksum) != 2 || checksum[1] == strings.Repeat("0", 64) {
		t.Fatal("Syft installer must pin a non-empty 64-hex SHA-256")
	}

	command := exec.Command("bash", "-n", scriptPath)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("bash -n %s: %v\n%s", syftInstallScript, err, output)
	}
}

func TestInstallSyftRetriesTransientDownload(t *testing.T) {
	archive, checksum := fakeSyftArchive(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/syft_1.56.0_linux_amd64.tar.gz" {
			http.NotFound(w, r)
			return
		}
		if requests.Add(1) <= 2 {
			http.Error(w, "temporary", http.StatusGatewayTimeout)
			return
		}
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	installDir := t.TempDir()
	script := syftTestScript(t, server.URL, checksum)
	if code, output := runSyftInstaller(t, script, installDir); code != 0 {
		t.Fatalf("installer exited %d, want 0:\n%s", code, output)
	}
	if got := requests.Load(); got != 3 {
		t.Fatalf("download requests = %d, want 3 (two 504s then success)", got)
	}
	if _, err := os.Stat(filepath.Join(installDir, "syft")); err != nil {
		t.Fatalf("installed Syft not found: %v", err)
	}
}

func TestInstallSyftFailsAfterRetryLimit(t *testing.T) {
	_, checksum := fakeSyftArchive(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "temporary", http.StatusGatewayTimeout)
	}))
	defer server.Close()

	script := syftTestScript(t, server.URL, checksum)
	if code, _ := runSyftInstaller(t, script, t.TempDir()); code == 0 {
		t.Fatal("installer succeeded despite persistent HTTP 504 responses")
	}
	if got := requests.Load(); got != 5 {
		t.Fatalf("download attempts = %d, want the bounded limit of 5", got)
	}
}

func TestInstallSyftRejectsChecksumMismatch(t *testing.T) {
	archive, _ := fakeSyftArchive(t)
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	script := syftTestScript(t, server.URL, strings.Repeat("0", 64))
	if code, _ := runSyftInstaller(t, script, t.TempDir()); code == 0 {
		t.Fatal("installer accepted an invalid checksum")
	}
	if got := requests.Load(); got != 1 {
		t.Fatalf("download attempts = %d, want 1 (checksum mismatch is permanent)", got)
	}
}

func TestInstallSyftRejectsUnexpectedVersion(t *testing.T) {
	archive, checksum := fakeSyftArchiveWithVersion(t, "9.9.9")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer server.Close()

	script := syftTestScript(t, server.URL, checksum)
	if code, _ := runSyftInstaller(t, script, t.TempDir()); code == 0 {
		t.Fatal("installer accepted an unexpected Syft version")
	}
}

func fakeSyftArchive(t *testing.T) ([]byte, string) {
	t.Helper()
	return fakeSyftArchiveWithVersion(t, "1.56.0")
}

func fakeSyftArchiveWithVersion(t *testing.T, version string) ([]byte, string) {
	t.Helper()
	pkgDir := t.TempDir()
	syftPath := filepath.Join(pkgDir, "syft")
	content := fmt.Sprintf("#!/usr/bin/env bash\nprintf 'Version: %s\\n'\n", version)
	if err := os.WriteFile(syftPath, []byte(content), 0o755); err != nil {
		t.Fatalf("write fake syft: %v", err)
	}
	archivePath := filepath.Join(t.TempDir(), "syft.tar.gz")
	command := exec.Command("tar", "-czf", archivePath, "-C", pkgDir, "syft")
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("create fake archive: %v\n%s", err, output)
	}
	archive, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatalf("read fake archive: %v", err)
	}
	sum := sha256.Sum256(archive)
	return archive, hex.EncodeToString(sum[:])
}

func syftTestScript(t *testing.T, releaseBase, checksum string) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), syftInstallScript))
	if err != nil {
		t.Fatalf("read installer: %v", err)
	}
	script := string(content)
	script = regexp.MustCompile(`(?m)^readonly RELEASE_BASE=.*$`).ReplaceAllString(script, `readonly RELEASE_BASE="`+releaseBase+`"`)
	script = regexp.MustCompile(`(?m)^readonly SYFT_SHA256=.*$`).ReplaceAllString(script, `readonly SYFT_SHA256="`+checksum+`"`)
	script = strings.Replace(script, "readonly INITIAL_DELAY_SECONDS=2", "readonly INITIAL_DELAY_SECONDS=0", 1)
	return script
}

func runSyftInstaller(t *testing.T, script, installDir string) (int, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "install-syft.sh")
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write installer copy: %v", err)
	}
	command := exec.Command("bash", path)
	command.Dir = t.TempDir()
	command.Env = append(environmentWithout("GITHUB_PATH", "SYFT_INSTALL_DIR"),
		"SYFT_INSTALL_DIR="+installDir)
	output, err := command.CombinedOutput()
	if err == nil {
		return 0, string(output)
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		return exit.ExitCode(), string(output)
	}
	t.Fatalf("run installer: %v", err)
	return -1, ""
}

func environmentWithout(keys ...string) []string {
	blocked := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		blocked[key] = struct{}{}
	}
	environment := make([]string, 0, len(os.Environ()))
	for _, entry := range os.Environ() {
		key, _, _ := strings.Cut(entry, "=")
		if _, skip := blocked[key]; skip {
			continue
		}
		environment = append(environment, entry)
	}
	return environment
}
