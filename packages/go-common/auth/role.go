// Package auth holds the access-token contract shared by every SwiftDrop
// service: the role vocabulary, JWT issuing and verification, and the Gin
// middleware that enforces authentication and role checks (API v1.1 sections
// 1.2 and 1.3).
package auth

// Role is a SwiftDrop account role. The values match identity.roles.name and
// the Role type in API v1.1 section 1.8.
type Role string

const (
	RoleCustomer   Role = "CUSTOMER"
	RoleMerchant   Role = "MERCHANT"
	RoleCourier    Role = "COURIER"
	RoleDispatcher Role = "DISPATCHER"
	RoleAdmin      Role = "ADMIN"
)

var knownRoles = map[Role]struct{}{
	RoleCustomer:   {},
	RoleMerchant:   {},
	RoleCourier:    {},
	RoleDispatcher: {},
	RoleAdmin:      {},
}

// ParseRole converts a stored or transmitted role name to a Role. Unknown
// values are rejected so a tampered or future role never grants access.
func ParseRole(value string) (Role, bool) {
	role := Role(value)
	_, ok := knownRoles[role]
	return role, ok
}

// Valid reports whether r is one of the five SwiftDrop roles.
func (r Role) Valid() bool {
	_, ok := knownRoles[r]
	return ok
}

// Satisfies reports whether a caller holding r may use an endpoint that
// requires the given role. ADMIN also satisfies every DISPATCHER permission
// (API v1.1 section 1.3); every other role satisfies only itself.
func (r Role) Satisfies(required Role) bool {
	if !r.Valid() || !required.Valid() {
		return false
	}
	return r == required || (r == RoleAdmin && required == RoleDispatcher)
}
