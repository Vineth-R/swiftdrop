package scripts_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

const ciWorkflow = ".github/workflows/ci.yml"

var goDockerServices = []string{
	"identity",
	"merchant",
	"notification-worker",
	"parcel",
	"pricing",
	"realtime",
	"webhook-worker",
}

type pathFilterSet map[string][]string

func TestCIPathFiltersRouteChangesToTheRightJobs(t *testing.T) {
	filters := loadCIPathFilters(t)

	tests := []struct {
		name           string
		changed        []string
		wantServices   []string
		wantAccessFlag bool
		wantAccessJob  bool
	}{
		{
			name:           "compose change runs access tests but no Docker build",
			changed:        []string{"docker-compose.yml"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "nginx change runs access tests",
			changed:        []string{"infra/nginx/default.conf"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "postgres init change runs access tests",
			changed:        []string{"infra/docker/postgres/init/01-schemas.sql"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "migration change runs access tests",
			changed:        []string{"migrations/pricing/000003_demo.up.sql"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "access test change runs access tests",
			changed:        []string{"tests/access/matrix_test.go"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "makefile change runs access tests",
			changed:        []string{"Makefile"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "migration script change runs access tests",
			changed:        []string{"scripts/migrate.sh"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "env example change runs access tests",
			changed:        []string{".env.example"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
		{
			name:           "shared go package change builds every go image and runs access tests",
			changed:        []string{"packages/go-common/httpx/router.go"},
			wantServices:   goDockerServices,
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "new shared package builds every go image",
			changed:        []string{"packages/new-package/file.go"},
			wantServices:   goDockerServices,
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "go module change builds every go image",
			changed:        []string{"go.mod"},
			wantServices:   goDockerServices,
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "go dockerfile change builds every go image but not web",
			changed:        []string{"infra/docker/go.Dockerfile"},
			wantServices:   goDockerServices,
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "dockerignore change builds every image",
			changed:        []string{".dockerignore"},
			wantServices:   append(append([]string{}, goDockerServices...), "web"),
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "web dockerfile change builds web",
			changed:        []string{"infra/docker/web.Dockerfile"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "web source change builds web",
			changed:        []string{"apps/web/src/app/page.tsx"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "pnpm workspace change builds web",
			changed:        []string{"pnpm-workspace.yaml"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "nx configuration change builds web",
			changed:        []string{"nx.json"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "base tsconfig change builds web",
			changed:        []string{"tsconfig.base.json"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "root tsconfig change builds web",
			changed:        []string{"tsconfig.json"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "e2e package manifest change builds web",
			changed:        []string{"apps/web-e2e/package.json"},
			wantServices:   []string{"web"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "service source change builds only that service",
			changed:        []string{"apps/services/pricing/internal/http/router.go"},
			wantServices:   []string{"pricing"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "worker source change builds only that worker",
			changed:        []string{"apps/services/notification-worker/internal/worker/worker.go"},
			wantServices:   []string{"notification-worker"},
			wantAccessFlag: false,
			wantAccessJob:  true,
		},
		{
			name:           "readme only change skips docker and access",
			changed:        []string{"README.md"},
			wantAccessFlag: false,
			wantAccessJob:  false,
		},
		{
			name:           "docs only change skips docker and access",
			changed:        []string{"docs/architecture.md"},
			wantAccessFlag: false,
			wantAccessJob:  false,
		},
		{
			name:           "mixed service and access change keeps both paths",
			changed:        []string{"docker-compose.yml", "apps/services/pricing/main.go"},
			wantServices:   []string{"pricing"},
			wantAccessFlag: true,
			wantAccessJob:  true,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched := filters.matched(test.changed)
			services := dockerServices(matched)

			assertStringSet(t, "Docker services", services, test.wantServices)
			if got := matched["access"]; got != test.wantAccessFlag {
				t.Fatalf("access filter matched = %t, want %t", got, test.wantAccessFlag)
			}

			// The access job also runs whenever any Docker image changes,
			// because those changes can alter the Compose stack behaviour.
			accessJob := matched["access"] || len(services) > 0
			if accessJob != test.wantAccessJob {
				t.Fatalf("access job runs = %t, want %t", accessJob, test.wantAccessJob)
			}
		})
	}
}

func TestCIPathFiltersKeepAccessOutOfTheDockerMatrix(t *testing.T) {
	filters := loadCIPathFilters(t)
	if _, ok := filters["access"]; !ok {
		t.Fatal("workflow has no access path filter")
	}

	for name, patterns := range filters {
		// access and notificationflow are separate non-Docker filters. Only
		// the service filters feed the Docker build matrix.
		if name == "access" || name == "notificationflow" {
			continue
		}
		for _, pattern := range patterns {
			if pattern == "docker-compose.yml" || strings.HasPrefix(pattern, "migrations/") {
				t.Fatalf("Docker service filter %q must not include access-only path %q", name, pattern)
			}
		}
	}
}

func TestCIPathFiltersRouteNotificationFlowChanges(t *testing.T) {
	filters := loadCIPathFilters(t)
	if _, ok := filters["notificationflow"]; !ok {
		t.Fatal("workflow has no notificationflow path filter")
	}

	tests := []struct {
		name    string
		changed []string
		want    bool
	}{
		{"notificationflow suite", []string{"tests/integration/notificationflow/harness_test.go"}, true},
		{"notification worker source", []string{"apps/services/notification-worker/internal/worker/worker.go"}, true},
		{"shared go package", []string{"packages/go-common/messaging/consumer.go"}, true},
		{"notification migration", []string{"migrations/notification/000002_tables.up.sql"}, true},
		{"postgres init", []string{"infra/docker/postgres/init/01-schemas.sql"}, true},
		{"compose", []string{"docker-compose.yml"}, true},
		{"makefile", []string{"Makefile"}, true},
		{"migration script", []string{"scripts/migrate.sh"}, true},
		{"env example", []string{".env.example"}, true},
		{"go module", []string{"go.mod"}, true},
		{"go dockerfile", []string{"infra/docker/go.Dockerfile"}, true},
		{"ci workflow", []string{".github/workflows/ci.yml"}, true},
		{"readme only", []string{"README.md"}, false},
		{"pricing migration", []string{"migrations/pricing/000003_demo.up.sql"}, false},
		{"access test", []string{"tests/access/matrix_test.go"}, false},
		{"nginx config", []string{"infra/nginx/default.conf"}, false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			matched := filters.matched(test.changed)
			if got := matched["notificationflow"]; got != test.want {
				t.Fatalf("notificationflow filter matched = %t, want %t", got, test.want)
			}
		})
	}
}

func TestCIWorkflowSeparatesAccessDetectionFromDockerBuilds(t *testing.T) {
	workflow := readCIWorkflow(t)

	changes := workflowJob(t, workflow, "changes")
	for _, want := range []string{
		"services: ${{ steps.matrix.outputs.services }}",
		"access: ${{ steps.filter.outputs.access }}",
		`select(. != "access")`,
	} {
		if !strings.Contains(changes, want) {
			t.Fatalf("changes job does not contain %q:\n%s", want, changes)
		}
	}

	docker := workflowJob(t, workflow, "docker")
	if strings.Contains(docker, "outputs.access") {
		t.Fatalf("docker job must depend on the services output, not access:\n%s", docker)
	}
	if !strings.Contains(docker, "needs.changes.outputs.services") {
		t.Fatalf("docker job does not use the services output:\n%s", docker)
	}

	access := workflowJob(t, workflow, "access")
	if !strings.Contains(access, "needs.changes.outputs.access == 'true'") {
		t.Fatalf("access job does not use the access output:\n%s", access)
	}
	if !strings.Contains(access, "needs.changes.outputs.services") {
		t.Fatalf("access job does not keep running for Docker service changes:\n%s", access)
	}
}

func TestRequiredGateRejectsFailedOrCancelledDockerAndAccessJobs(t *testing.T) {
	workflow := readCIWorkflow(t)
	script := extractRunScript(t, workflowJob(t, workflow, "gate"))

	successes := map[string]string{
		"GO_RESULT":               "success",
		"HYGIENE_RESULT":          "success",
		"INTEGRATION_RESULT":      "success",
		"DATABASE_RESULT":         "success",
		"FRONTEND_RESULT":         "success",
		"E2E_RESULT":              "success",
		"CHANGES_RESULT":          "success",
		"DOCKER_RESULT":           "success",
		"ACCESS_RESULT":           "success",
		"NOTIFICATIONFLOW_RESULT": "success",
	}

	tests := []struct {
		name    string
		result  string
		value   string
		wantErr bool
	}{
		{name: "docker success", result: "DOCKER_RESULT", value: "success"},
		{name: "docker skipped", result: "DOCKER_RESULT", value: "skipped"},
		{name: "docker failure", result: "DOCKER_RESULT", value: "failure", wantErr: true},
		{name: "docker cancelled", result: "DOCKER_RESULT", value: "cancelled", wantErr: true},
		{name: "access success", result: "ACCESS_RESULT", value: "success"},
		{name: "access skipped", result: "ACCESS_RESULT", value: "skipped"},
		{name: "access failure", result: "ACCESS_RESULT", value: "failure", wantErr: true},
		{name: "access cancelled", result: "ACCESS_RESULT", value: "cancelled", wantErr: true},
		{name: "notificationflow success", result: "NOTIFICATIONFLOW_RESULT", value: "success"},
		{name: "notificationflow skipped", result: "NOTIFICATIONFLOW_RESULT", value: "skipped"},
		{name: "notificationflow failure", result: "NOTIFICATIONFLOW_RESULT", value: "failure", wantErr: true},
		{name: "notificationflow cancelled", result: "NOTIFICATIONFLOW_RESULT", value: "cancelled", wantErr: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			results := make(map[string]string, len(successes))
			for key, value := range successes {
				results[key] = value
			}
			results[test.result] = test.value

			output, err := runGateScript(t, script, results)
			if test.wantErr && err == nil {
				t.Fatalf("gate accepted %s=%s:\n%s", test.result, test.value, output)
			}
			if !test.wantErr && err != nil {
				t.Fatalf("gate rejected %s=%s: %v\n%s", test.result, test.value, err, output)
			}
			if test.wantErr && !strings.Contains(output, strings.ToLower(strings.TrimSuffix(test.result, "_RESULT"))) {
				t.Fatalf("gate failure did not identify %s:\n%s", test.result, output)
			}
		})
	}
}

func loadCIPathFilters(t *testing.T) pathFilterSet {
	t.Helper()
	return parsePathFilters(readCIWorkflow(t))
}

func readCIWorkflow(t *testing.T) string {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(repositoryRoot(t), ciWorkflow))
	if err != nil {
		t.Fatalf("read %s: %v", ciWorkflow, err)
	}
	return string(content)
}

func repositoryRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatalf("resolve repository root: %v", err)
	}
	return root
}

func parsePathFilters(workflow string) pathFilterSet {
	filters := pathFilterSet{}
	lines := strings.Split(workflow, "\n")
	inBlock := false
	blockIndent := 0
	current := ""

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if !inBlock {
			if trimmed == "filters: |" {
				inBlock = true
				blockIndent = leadingSpaces(line)
			}
			continue
		}

		if trimmed == "" {
			continue
		}
		indent := leadingSpaces(line)
		if indent <= blockIndent {
			break
		}

		if indent == blockIndent+2 && !strings.HasPrefix(trimmed, "-") && strings.HasSuffix(trimmed, ":") {
			current = strings.TrimSuffix(trimmed, ":")
			filters[current] = nil
			continue
		}

		if current != "" && strings.HasPrefix(trimmed, "- ") {
			pattern := strings.TrimSpace(strings.TrimPrefix(trimmed, "- "))
			pattern = strings.Trim(pattern, `'"`)
			filters[current] = append(filters[current], pattern)
		}
	}

	return filters
}

func (filters pathFilterSet) matched(changed []string) map[string]bool {
	matched := make(map[string]bool, len(filters))
	for name, patterns := range filters {
		for _, pattern := range patterns {
			expression := globExpression(pattern)
			for _, path := range changed {
				if expression.MatchString(path) {
					matched[name] = true
					break
				}
			}
			if matched[name] {
				break
			}
		}
	}
	return matched
}

func globExpression(pattern string) *regexp.Regexp {
	var expression strings.Builder
	expression.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		switch char := pattern[index]; char {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				expression.WriteString(".*")
				index++
			} else {
				expression.WriteString("[^/]*")
			}
		case '?':
			expression.WriteString("[^/]")
		case '.', '+', '(', ')', '|', '^', '$', '{', '}', '[', ']', '\\':
			expression.WriteByte('\\')
			expression.WriteByte(char)
		default:
			expression.WriteByte(char)
		}
	}
	expression.WriteString("$")
	return regexp.MustCompile(expression.String())
}

func dockerServices(matched map[string]bool) []string {
	services := []string{}
	for name, changed := range matched {
		if changed && name != "access" && name != "notificationflow" {
			services = append(services, name)
		}
	}
	sort.Strings(services)
	return services
}

func assertStringSet(t *testing.T, label string, got, want []string) {
	t.Helper()
	got = append([]string{}, got...)
	want = append([]string{}, want...)
	sort.Strings(got)
	sort.Strings(want)
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Fatalf("%s = %v, want %v", label, got, want)
	}
}

func leadingSpaces(line string) int {
	return len(line) - len(strings.TrimLeft(line, " "))
}

func workflowJob(t *testing.T, workflow, name string) string {
	t.Helper()
	lines := strings.Split(workflow, "\n")
	marker := "  " + name + ":"
	start := -1
	for index, line := range lines {
		if line == marker {
			start = index
			break
		}
	}
	if start < 0 {
		t.Fatalf("workflow has no %q job", name)
	}

	end := len(lines)
	for index := start + 1; index < len(lines); index++ {
		line := lines[index]
		if strings.TrimSpace(line) == "" {
			continue
		}
		if leadingSpaces(line) == 2 && strings.HasSuffix(strings.TrimSpace(line), ":") {
			end = index
			break
		}
	}
	return strings.Join(lines[start:end], "\n")
}

func extractRunScript(t *testing.T, job string) string {
	t.Helper()
	lines := strings.Split(job, "\n")
	for index, line := range lines {
		if strings.TrimSpace(line) != "run: |" {
			continue
		}
		runIndent := leadingSpaces(line)
		script := []string{}
		for _, body := range lines[index+1:] {
			if strings.TrimSpace(body) == "" {
				script = append(script, "")
				continue
			}
			if leadingSpaces(body) <= runIndent {
				break
			}
			cut := runIndent + 2
			if len(body) <= cut {
				script = append(script, "")
				continue
			}
			script = append(script, body[cut:])
		}
		return strings.Join(script, "\n")
	}
	t.Fatal("job has no block-scalar run step")
	return ""
}

func runGateScript(t *testing.T, script string, results map[string]string) (string, error) {
	t.Helper()
	cmd := exec.Command("bash", "-c", script)
	env := []string{"PATH=" + os.Getenv("PATH")}
	for key, value := range results {
		env = append(env, key+"="+value)
	}
	cmd.Env = env
	output, err := cmd.CombinedOutput()
	return string(output), err
}
