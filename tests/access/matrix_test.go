//go:build access

package access

import (
	"net/http"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/tests/access/accesskit"
)

const (
	anyID = "00000000-0000-4000-8000-000000000000"

	merchantWiring = "the merchant service does not verify access tokens yet: it only learns who the caller is from the API-key middleware"
)

// matrix lists every endpoint with an access rule and the roles that may call
// it (API v1.1 appendix, endpoint table). To add your service's endpoints, add
// a Case. If the endpoint is not built yet, add it with Missing set and remove
// the field when it exists. ADMIN also satisfies every DISPATCHER rule, so list
// DISPATCHER and ADMIN gets in without being named.
//
// Write requests carry a body the handler rejects, so running the suite never
// changes data.
var matrix = []accesskit.Case{
	// Identity
	{Name: "GET /users/me", Method: http.MethodGet, Path: "/api/v1/users/me", Allowed: accesskit.Roles},
	{Name: "GET /admin/users/", Method: http.MethodGet, Path: "/api/v1/admin/users/", Allowed: []auth.Role{auth.RoleAdmin}},
	{Name: "POST /admin/users/", Method: http.MethodPost, Path: "/api/v1/admin/users/", Body: `{}`, Allowed: []auth.Role{auth.RoleAdmin}},
	{Name: "PATCH /admin/users/{id}/status", Method: http.MethodPatch, Path: "/api/v1/admin/users/" + anyID + "/status", Body: `{}`, Allowed: []auth.Role{auth.RoleAdmin}},
	{Name: "GET /couriers/", Method: http.MethodGet, Path: "/api/v1/couriers/", Allowed: []auth.Role{auth.RoleDispatcher}},

	// Pricing
	{Name: "GET /admin/pricing/rules", Method: http.MethodGet, Path: "/api/v1/admin/pricing/rules", Allowed: []auth.Role{auth.RoleAdmin}},
	{Name: "GET /admin/zones", Method: http.MethodGet, Path: "/api/v1/admin/zones", Allowed: []auth.Role{auth.RoleAdmin}},
	{Name: "POST /admin/zones", Method: http.MethodPost, Path: "/api/v1/admin/zones", Body: `{}`, Allowed: []auth.Role{auth.RoleAdmin}},

	// Notification
	{Name: "GET /admin/notification-logs", Method: http.MethodGet, Path: "/api/v1/admin/notification-logs", Allowed: []auth.Role{auth.RoleAdmin}},

	// Merchant: routes exist, but none verifies a token yet, so only the refusals are checked.
	{Name: "GET /merchants/me", Method: http.MethodGet, Path: "/api/v1/merchants/me", Allowed: []auth.Role{auth.RoleMerchant}, Pending: merchantWiring},
	{Name: "GET /merchants/me/keys", Method: http.MethodGet, Path: "/api/v1/merchants/me/keys", Allowed: []auth.Role{auth.RoleMerchant}, Pending: merchantWiring},
	{Name: "POST /merchants/me/keys", Method: http.MethodPost, Path: "/api/v1/merchants/me/keys", Body: `{}`, Allowed: []auth.Role{auth.RoleMerchant}, Pending: merchantWiring},
	{Name: "GET /admin/merchants/{id}", Method: http.MethodGet, Path: "/api/v1/admin/merchants/" + anyID, Allowed: []auth.Role{auth.RoleAdmin}, Pending: merchantWiring},
	{Name: "POST /admin/merchants/{id}/approve", Method: http.MethodPost, Path: "/api/v1/admin/merchants/" + anyID + "/approve", Body: `{}`, Allowed: []auth.Role{auth.RoleAdmin}, Pending: merchantWiring},

	// Customer parcel and dispatcher operations. Courier lifecycle remains separate.
	{Name: "POST /parcels", Method: http.MethodPost, Path: "/api/v1/parcels", Body: `{}`, Allowed: []auth.Role{auth.RoleCustomer}},
	{Name: "GET /parcels", Method: http.MethodGet, Path: "/api/v1/parcels", Allowed: []auth.Role{auth.RoleCustomer}},
	{Name: "GET /parcels/{id}", Method: http.MethodGet, Path: "/api/v1/parcels/" + anyID, Allowed: []auth.Role{auth.RoleCustomer, auth.RoleDispatcher}},
	{Name: "GET /parcels/{id}/history", Method: http.MethodGet, Path: "/api/v1/parcels/" + anyID + "/history", Allowed: []auth.Role{auth.RoleCustomer, auth.RoleDispatcher}},
	{Name: "GET /parcels/{id}/receipt", Method: http.MethodGet, Path: "/api/v1/parcels/" + anyID + "/receipt", Allowed: []auth.Role{auth.RoleCustomer, auth.RoleAdmin}},
	{Name: "GET /dispatch/unassigned", Method: http.MethodGet, Path: "/api/v1/dispatch/unassigned", Allowed: []auth.Role{auth.RoleDispatcher}},
	{Name: "GET /dispatch/workload", Method: http.MethodGet, Path: "/api/v1/dispatch/workload", Allowed: []auth.Role{auth.RoleDispatcher}},
	{Name: "PUT /dispatch/parcels/{id}/assignment", Method: http.MethodPut, Path: "/api/v1/dispatch/parcels/" + anyID + "/assignment", Body: `{}`, Allowed: []auth.Role{auth.RoleDispatcher}},
	{Name: "GET /admin/parcels", Method: http.MethodGet, Path: "/api/v1/admin/parcels/", Allowed: []auth.Role{auth.RoleDispatcher}},
	{Name: "GET /courier/deliveries", Method: http.MethodGet, Path: "/api/v1/courier/deliveries", Allowed: []auth.Role{auth.RoleCourier}, Missing: "the parcel service has no courier routes yet (API 6.1)"},
}

func TestRoleMatrix(t *testing.T) {
	s := useStack(t)
	active, missing := accesskit.Split(matrix)

	violations, err := accesskit.Check(accesskit.Gateway{BaseURL: s.base, Client: s.client, Tokens: s.tokens}, active)
	if err != nil {
		t.Fatal(err)
	}
	for _, violation := range violations {
		t.Error(violation)
	}
	t.Logf("%d endpoints checked as anonymous and as each of the five roles", len(active))

	for _, c := range missing {
		t.Run("not built yet/"+c.Name, func(t *testing.T) { t.Skip(c.Missing) })
	}
	for _, c := range active {
		if c.Pending != "" {
			t.Logf("allowed roles not checked for %s: %s", c.Name, c.Pending)
		}
	}
}
