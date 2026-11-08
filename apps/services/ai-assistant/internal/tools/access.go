package tools

import (
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// requireParcelAccess is the single authorization gate for the two private
// parcel tools. It uses only the trusted caller context populated by the HTTP
// authentication middleware. Guests are asked to sign in; signed-in roles that
// are not allowed get a safe denial. The model can never supply identity,
// role or ownership here.
func requireParcelAccess(caller contract.CallerContext, signInMessage string) (contract.ToolResult, bool) {
	if !caller.Authenticated ||
		strings.TrimSpace(caller.UserID) == "" ||
		strings.TrimSpace(caller.AccessToken) == "" {
		return deniedResult(signInMessage), false
	}

	switch auth.Role(caller.Role) {
	case auth.RoleCustomer, auth.RoleDispatcher, auth.RoleAdmin:
		return contract.ToolResult{}, true
	default:
		return deniedResult("This information is available only to customer accounts and authorized delivery staff."), false
	}
}

// isPrivilegedParcelRole reports whether the verified role may read parcels
// across customers for operational purposes.
func isPrivilegedParcelRole(role string) bool {
	return role == string(auth.RoleDispatcher) || role == string(auth.RoleAdmin)
}

// isActiveStatus reports whether a parcel status still needs operational or
// customer attention.
func isActiveStatus(status string) bool {
	switch status {
	case "DELIVERED", "FAILED":
		return false
	default:
		return true
	}
}
