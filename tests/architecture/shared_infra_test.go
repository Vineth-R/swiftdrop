package architecture

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"
)

const (
	httpxImport   = "github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	loggingImport = "github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	slogImport    = "log/slog"
)

// TestRepositorySharedInfrastructure is the repository guard run in CI. It
// checks conventional wiring: every service/worker assigns logging.New to a
// logger, no service main.go bypasses it with slog.New, and every HTTP service
// initializes httpx.NewRouter in main/run or in an internal/http.New function
// that main/run calls.
func TestRepositorySharedInfrastructure(t *testing.T) {
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("cannot locate the test file")
	}
	root := filepath.Join(filepath.Dir(file), "..", "..")

	violations := checkRepository(root)
	if len(violations) > 0 {
		t.Fatalf("shared infrastructure violations:\n  %s", strings.Join(violations, "\n  "))
	}
}

func TestCheckerDetectsMissingSharedCalls(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "good", `
package main

import (
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/good/internal/http"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
	_ = servicehttp.New()
}
`, `
package servicehttp

import "github.com/Vineth-R/swiftdrop/packages/go-common/httpx"

func New() { _ = httpx.NewRouter(nil) }
`)
	writeService(t, root, "nohttp", `
package main

import "github.com/Vineth-R/swiftdrop/packages/go-common/logging"

func main() {
	logger, _ := logging.New()
	_ = logger
}
`, "")
	writeService(t, root, "bad", `
package main

import "github.com/Vineth-R/swiftdrop/packages/go-common/logging"

func main() { _ = logging.NewBootstrap("service") }
`, `
package servicehttp

import "github.com/gin-gonic/gin"

func New() { _ = gin.New() }
`)

	violations := checkRepository(root)
	want := []string{
		"bad/internal/http must initialize httpx.NewRouter in a New function and main/run must call internal/http.New",
		"bad/main.go must assign logging.New to a logger",
	}
	assertViolations(t, violations, want)
}

func TestCheckerResolvesAliasedImports(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "aliased", `
package main

import (
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/aliased/internal/http"
	log "github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := log.New()
	_ = logger
	_ = servicehttp.New()
}
`, `
package servicehttp

import api "github.com/Vineth-R/swiftdrop/packages/go-common/httpx"

func New() { _ = api.NewRouter(nil) }
`)

	if violations := checkRepository(root); len(violations) > 0 {
		t.Fatalf("aliased imports produced violations: %v", violations)
	}
}

func TestCheckerHandlesDefaultHTTPPackageName(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "defaultname", `
package main

import (
	"github.com/Vineth-R/swiftdrop/apps/services/defaultname/internal/http"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
	_ = servicehttp.New()
}
`, `
package servicehttp

import "github.com/Vineth-R/swiftdrop/packages/go-common/httpx"

func New() { _ = httpx.NewRouter(nil) }
`)

	if violations := checkRepository(root); len(violations) > 0 {
		t.Fatalf("default HTTP package name produced violations: %v", violations)
	}
}

func TestCheckerFindsRouterOutsideRouterFile(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "norouter", `
package main

import (
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/norouter/internal/http"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
	_ = servicehttp.New()
}
`, `
package servicehttp

import "github.com/Vineth-R/swiftdrop/packages/go-common/httpx"

func New() { _ = httpx.NewRouter(nil) }
`)
	// Replace router.go with a differently named file while keeping the same
	// package-level New function.
	if err := os.Rename(
		filepath.Join(root, "apps", "services", "norouter", "internal", "http", "router.go"),
		filepath.Join(root, "apps", "services", "norouter", "internal", "http", "http.go"),
	); err != nil {
		t.Fatalf("rename router fixture: %v", err)
	}

	if violations := checkRepository(root); len(violations) > 0 {
		t.Fatalf("router outside router.go produced violations: %v", violations)
	}
}

func TestCheckerAllowsRouterInMain(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "mainrouter", `
package main

import (
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
	_ = httpx.NewRouter(nil)
}
`, "")

	if violations := checkRepository(root); len(violations) > 0 {
		t.Fatalf("router in main produced violations: %v", violations)
	}
}

func TestCheckerRejectsUnusedRouterFunction(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "unusedrouter", `
package main

import (
	servicehttp "github.com/Vineth-R/swiftdrop/apps/services/unusedrouter/internal/http"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
	_ = servicehttp.New()
}
`, `
package servicehttp

import (
	"github.com/gin-gonic/gin"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
)

func New() { _ = gin.New() }

func unused() { _ = httpx.NewRouter(nil) }
`)

	assertViolations(t, checkRepository(root), []string{
		"unusedrouter/internal/http must initialize httpx.NewRouter in a New function and main/run must call internal/http.New",
	})
}

func TestCheckerRejectsUnusedRouterInMain(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "unusedmainrouter", `
package main

import (
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
)

func main() {
	logger, _ := logging.New()
	_ = logger
}

func unused() { _ = httpx.NewRouter(nil) }
`, "")

	assertViolations(t, checkRepository(root), []string{
		"unusedmainrouter/main.go must initialize httpx.NewRouter in main or run",
	})
}

func TestCheckerRejectsDiscardedLogger(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "discardedlogger", `
package main

import "github.com/Vineth-R/swiftdrop/packages/go-common/logging"

func main() { _ = logging.New() }
`, "")

	assertViolations(t, checkRepository(root), []string{
		"discardedlogger/main.go must assign logging.New to a logger",
	})
}

func TestCheckerRejectsDirectSlogBypass(t *testing.T) {
	root := t.TempDir()
	writeService(t, root, "slogonly", `
package main

import "log/slog"

func main() {
	logger := slog.New(nil)
	_ = logger
}
`, "")

	assertViolations(t, checkRepository(root), []string{
		"slogonly/main.go must assign logging.New to a logger",
		"slogonly/main.go must not call slog.New directly",
	})
}

func checkRepository(root string) []string {
	servicesDir := filepath.Join(root, "apps", "services")
	entries, err := os.ReadDir(servicesDir)
	if err != nil {
		return []string{fmt.Sprintf("read apps/services: %v", err)}
	}

	var violations []string
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		service := entry.Name()
		serviceDir := filepath.Join(servicesDir, service)
		mainPath := filepath.Join(serviceDir, "main.go")
		if _, err := os.Stat(mainPath); err != nil {
			continue
		}

		if !fileAssignsCall(mainPath, loggingImport, "New") {
			violations = append(violations, fmt.Sprintf("%s/main.go must assign logging.New to a logger", service))
		}
		if fileCalls(mainPath, slogImport, "New") {
			violations = append(violations, fmt.Sprintf("%s/main.go must not call slog.New directly", service))
		}

		httpDir := filepath.Join(serviceDir, "internal", "http")
		hasHTTPDir := isDir(httpDir)
		mainRouterAnywhere := fileCalls(mainPath, httpxImport, "NewRouter")
		mainRouterInEntry := functionCalls(mainPath, "main", httpxImport, "NewRouter") ||
			functionCalls(mainPath, "run", httpxImport, "NewRouter")
		if !hasHTTPDir && !mainRouterAnywhere {
			continue
		}
		if mainRouterInEntry {
			continue
		}
		if hasHTTPDir &&
			dirFunctionCalls(httpDir, "New", httpxImport, "NewRouter") &&
			mainCallsHTTPNew(mainPath, serviceDir, service) {
			continue
		}
		if hasHTTPDir {
			violations = append(violations, fmt.Sprintf("%s/internal/http must initialize httpx.NewRouter in a New function and main/run must call internal/http.New", service))
		} else {
			violations = append(violations, fmt.Sprintf("%s/main.go must initialize httpx.NewRouter in main or run", service))
		}
	}
	sort.Strings(violations)
	return violations
}

func mainCallsHTTPNew(mainPath, serviceDir, service string) bool {
	importPath := "github.com/Vineth-R/swiftdrop/apps/services/" + service + "/internal/http"
	localName := importedPackageName(mainPath, filepath.Join(serviceDir, "internal", "http"), importPath)
	if localName == "" {
		return false
	}
	return functionCallsSelector(mainPath, "main", localName, "New") ||
		functionCallsSelector(mainPath, "run", localName, "New")
}

func importedPackageName(filePath, packageDir, importPath string) string {
	file, _, err := parseFile(filePath)
	if err != nil {
		return ""
	}
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil || value != importPath {
			continue
		}
		if spec.Name != nil {
			return spec.Name.Name
		}
		if name := directoryPackageName(packageDir); name != "" {
			return name
		}
		return path.Base(importPath)
	}
	return ""
}

func directoryPackageName(dir string) string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return ""
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, _, err := parseFile(filepath.Join(dir, entry.Name()))
		if err == nil {
			return file.Name.Name
		}
	}
	return ""
}

func functionCallsSelector(filePath, functionName, selectorName, callName string) bool {
	file, _, err := parseFile(filePath)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		found := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if !ok {
				return true
			}
			selector, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || selector.Sel.Name != callName {
				return true
			}
			identifier, ok := selector.X.(*ast.Ident)
			if ok && identifier.Name == selectorName {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func dirFunctionCalls(dir, functionName, importPath, callName string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		if functionCalls(filepath.Join(dir, entry.Name()), functionName, importPath, callName) {
			return true
		}
	}
	return false
}

func fileAssignsCall(filePath, importPath, function string) bool {
	file, imports, err := parseFile(filePath)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		switch statement := node.(type) {
		case *ast.AssignStmt:
			for _, right := range statement.Rhs {
				if callMatchesExpr(right, imports, importPath, function) && hasNonBlankTarget(statement.Lhs) {
					found = true
					return false
				}
			}
		case *ast.ValueSpec:
			for index, value := range statement.Values {
				if callMatchesExpr(value, imports, importPath, function) &&
					index < len(statement.Names) && statement.Names[index].Name != "_" {
					found = true
					return false
				}
			}
		}
		return true
	})
	return found
}

func hasNonBlankTarget(targets []ast.Expr) bool {
	for _, target := range targets {
		if identifier, ok := target.(*ast.Ident); ok && identifier.Name != "_" {
			return true
		}
	}
	return false
}

func fileCalls(filePath, importPath, function string) bool {
	file, imports, err := parseFile(filePath)
	if err != nil {
		return false
	}
	found := false
	ast.Inspect(file, func(node ast.Node) bool {
		call, ok := node.(*ast.CallExpr)
		if ok && callMatches(call, imports, importPath, function) {
			found = true
			return false
		}
		return true
	})
	return found
}

func functionCalls(filePath, functionName, importPath, callName string) bool {
	file, imports, err := parseFile(filePath)
	if err != nil {
		return false
	}
	for _, declaration := range file.Decls {
		function, ok := declaration.(*ast.FuncDecl)
		if !ok || function.Name.Name != functionName || function.Body == nil {
			continue
		}
		found := false
		ast.Inspect(function.Body, func(node ast.Node) bool {
			call, ok := node.(*ast.CallExpr)
			if ok && callMatches(call, imports, importPath, callName) {
				found = true
				return false
			}
			return true
		})
		if found {
			return true
		}
	}
	return false
}

func callMatchesExpr(expression ast.Expr, imports map[string]string, importPath, function string) bool {
	call, ok := expression.(*ast.CallExpr)
	return ok && callMatches(call, imports, importPath, function)
}

func callMatches(call *ast.CallExpr, imports map[string]string, importPath, function string) bool {
	selector, ok := call.Fun.(*ast.SelectorExpr)
	if !ok || selector.Sel.Name != function {
		return false
	}
	identifier, ok := selector.X.(*ast.Ident)
	if !ok {
		return false
	}
	return imports[identifier.Name] == importPath
}

func parseFile(filePath string) (*ast.File, map[string]string, error) {
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, filePath, nil, parser.SkipObjectResolution)
	if err != nil {
		return nil, nil, err
	}
	imports := make(map[string]string)
	for _, spec := range file.Imports {
		value, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			continue
		}
		name := path.Base(value)
		if spec.Name != nil {
			name = spec.Name.Name
		}
		imports[name] = value
	}
	return file, imports, nil
}

func isDir(dir string) bool {
	info, err := os.Stat(dir)
	return err == nil && info.IsDir()
}

func assertViolations(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("violations = %v, want %v", got, want)
	}
	for index := range want {
		if got[index] != want[index] {
			t.Fatalf("violations = %v, want %v", got, want)
		}
	}
}

func writeService(t *testing.T, root, service, mainSource, routerSource string) {
	t.Helper()
	writeFile(t, root, "apps/services/"+service+"/main.go", mainSource)
	if routerSource != "" {
		writeFile(t, root, "apps/services/"+service+"/internal/http/router.go", routerSource)
	}
}

func writeFile(t *testing.T, root, name, content string) {
	t.Helper()
	target := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", name, err)
	}
	if err := os.WriteFile(target, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}
