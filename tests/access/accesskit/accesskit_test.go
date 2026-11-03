package accesskit

import (
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const testSecret = "access-kit-self-test-secret-0123456789abcdef"

// newServer serves the routes the self-test checks. The first three are guarded
// the way the services guard theirs, with the shared auth middleware. The last
// ones are broken on purpose.
func newServer(t *testing.T) (*httptest.Server, map[auth.Role]string) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	manager, err := auth.NewTokenManager(testSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}
	authenticate := auth.Authenticate(manager, "sd_access")
	ok := func(c *gin.Context) { c.Status(http.StatusNoContent) }

	router := gin.New()
	router.GET("/signed-in", authenticate, ok)
	router.GET("/admin-only", authenticate, auth.RequireRoles(auth.RoleAdmin), ok)
	router.GET("/dispatch", authenticate, auth.RequireRoles(auth.RoleDispatcher), ok)
	router.GET("/pending-closed", authenticate, func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })

	// Broken on purpose: the role check was forgotten.
	router.GET("/leaky-admin", authenticate, ok)
	// Broken on purpose: the route is open to anyone.
	router.GET("/open-admin", ok)
	// Broken on purpose: a pending case that serves data to everyone.
	router.GET("/pending-leaky", authenticate, ok)
	// Broken on purpose: a pending case that serves data without any token.
	router.GET("/pending-open", ok)
	// A pending service that cannot tell "no token" from "wrong role" refuses
	// everyone with 403, which is acceptable while it is pending.
	router.GET("/pending-403", func(c *gin.Context) { c.AbortWithStatus(http.StatusForbidden) })

	server := httptest.NewServer(router)
	t.Cleanup(server.Close)

	tokens := map[auth.Role]string{}
	for _, role := range Roles {
		token, _, err := manager.Issue("11111111-1111-4111-8111-111111111111", role)
		if err != nil {
			t.Fatalf("Issue(%s) returned error: %v", role, err)
		}
		tokens[role] = token
	}
	return server, tokens
}

func summary(violations []Violation) []string {
	lines := make([]string, len(violations))
	for i, v := range violations {
		lines[i] = v.String()
	}
	sort.Strings(lines)
	return lines
}

func TestCheck_acceptsRulesThatAreKept(t *testing.T) {
	server, tokens := newServer(t)

	violations, err := Check(Gateway{BaseURL: server.URL, Tokens: tokens}, []Case{
		{Name: "signed in", Method: http.MethodGet, Path: "/signed-in", Allowed: Roles},
		{Name: "admin only", Method: http.MethodGet, Path: "/admin-only", Allowed: []auth.Role{auth.RoleAdmin}},
		// ADMIN satisfies DISPATCHER, so ADMIN must be let in as well.
		{Name: "dispatch", Method: http.MethodGet, Path: "/dispatch", Allowed: []auth.Role{auth.RoleDispatcher}},
		{
			Name: "pending but closed", Method: http.MethodGet, Path: "/pending-closed",
			Allowed: []auth.Role{auth.RoleMerchant}, Pending: "the service does not check tokens yet",
		},
		{
			Name: "pending but 403 for all", Method: http.MethodGet, Path: "/pending-403",
			Allowed: []auth.Role{auth.RoleMerchant}, Pending: "the service does not check tokens yet",
		},
		// Skipped, so the route that does not exist is never called.
		{
			Name: "not built yet", Method: http.MethodGet, Path: "/does-not-exist",
			Allowed: []auth.Role{auth.RoleCourier}, Missing: "the service has no such route yet",
		},
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if len(violations) != 0 {
		t.Fatalf("a correct server was reported as broken:\n%s", strings.Join(summary(violations), "\n"))
	}
}

// This is the "break one rule on purpose" check: each broken route must be
// reported, and nothing else.
func TestCheck_reportsABrokenRule(t *testing.T) {
	server, tokens := newServer(t)

	violations, err := Check(Gateway{BaseURL: server.URL, Tokens: tokens}, []Case{
		{Name: "admin only", Method: http.MethodGet, Path: "/admin-only", Allowed: []auth.Role{auth.RoleAdmin}},
		{Name: "leaky admin", Method: http.MethodGet, Path: "/leaky-admin", Allowed: []auth.Role{auth.RoleAdmin}},
		{Name: "open admin", Method: http.MethodGet, Path: "/open-admin", Allowed: []auth.Role{auth.RoleAdmin}},
		{
			Name: "pending leaky", Method: http.MethodGet, Path: "/pending-leaky",
			Allowed: []auth.Role{auth.RoleMerchant}, Pending: "the service does not check tokens yet",
		},
		{
			Name: "pending open", Method: http.MethodGet, Path: "/pending-open",
			Allowed: []auth.Role{auth.RoleMerchant}, Pending: "the service does not check tokens yet",
		},
	})
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	want := []string{
		// The role check was forgotten: four roles that must get 403 get through.
		"leaky admin as COURIER: got 204, want 403",
		"leaky admin as CUSTOMER: got 204, want 403",
		"leaky admin as DISPATCHER: got 204, want 403",
		"leaky admin as MERCHANT: got 204, want 403",
		// The route is open: anonymous gets in, and every role too.
		"open admin as COURIER: got 204, want 403",
		"open admin as CUSTOMER: got 204, want 403",
		"open admin as DISPATCHER: got 204, want 403",
		"open admin as MERCHANT: got 204, want 403",
		"open admin as anonymous: got 204, want 401",
		// A pending case still has to refuse everyone it is not meant for.
		"pending leaky as COURIER: got 204, want 401 or 403",
		"pending leaky as CUSTOMER: got 204, want 401 or 403",
		"pending leaky as DISPATCHER: got 204, want 401 or 403",
		"pending leaky as ADMIN: got 204, want 401 or 403",
		"pending open as COURIER: got 204, want 401 or 403",
		"pending open as CUSTOMER: got 204, want 401 or 403",
		"pending open as DISPATCHER: got 204, want 401 or 403",
		"pending open as ADMIN: got 204, want 401 or 403",
		"pending open as anonymous: got 204, want 401 or 403",
	}
	sort.Strings(want)
	got := summary(violations)
	if strings.Join(got, "\n") != strings.Join(want, "\n") {
		t.Fatalf("violations =\n%s\nwant\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestCheck_reportsAnUnreachableGateway(t *testing.T) {
	_, tokens := newServer(t)

	_, err := Check(Gateway{BaseURL: "http://127.0.0.1:1", Tokens: tokens}, []Case{
		{Name: "anything", Method: http.MethodGet, Path: "/", Allowed: Roles},
	})
	if err == nil {
		t.Fatal("Check hid a gateway that could not be reached")
	}
}

func TestCheck_needsATokenForEveryRole(t *testing.T) {
	server, tokens := newServer(t)
	delete(tokens, auth.RoleCourier)

	_, err := Check(Gateway{BaseURL: server.URL, Tokens: tokens}, []Case{
		{Name: "signed in", Method: http.MethodGet, Path: "/signed-in", Allowed: Roles},
	})
	if err == nil || !strings.Contains(err.Error(), "COURIER") {
		t.Fatalf("Check error = %v, want one naming the missing COURIER token", err)
	}
}

func TestCase_Allows(t *testing.T) {
	dispatch := Case{Allowed: []auth.Role{auth.RoleDispatcher}}
	for role, want := range map[auth.Role]bool{
		auth.RoleCustomer: false, auth.RoleMerchant: false, auth.RoleCourier: false,
		auth.RoleDispatcher: true, auth.RoleAdmin: true,
	} {
		if got := dispatch.Allows(role); got != want {
			t.Errorf("DISPATCHER endpoint allows %s = %t, want %t", role, got, want)
		}
	}
	admin := Case{Allowed: []auth.Role{auth.RoleAdmin}}
	if admin.Allows(auth.RoleDispatcher) {
		t.Error("an ADMIN endpoint allows DISPATCHER")
	}
}

func TestSplit(t *testing.T) {
	active, missing := Split([]Case{
		{Name: "built"},
		{Name: "not built", Missing: "no route yet"},
	})
	if len(active) != 1 || active[0].Name != "built" || len(missing) != 1 || missing[0].Name != "not built" {
		t.Fatalf("Split = %+v, %+v", active, missing)
	}
}
