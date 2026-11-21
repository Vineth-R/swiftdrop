package tools

import (
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/ai-assistant/internal/contract"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	signInCustomerMessage = "Please sign in to view your deliveries."
	signInStaffMessage    = "Please sign in with a dispatcher or administrator account to view operational deliveries."
	signInCourierMessage  = "Please sign in with a courier account to view your deliveries."
	signInMerchantMessage = "Please sign in with a merchant account to view your orders."
)

// hasVerifiedIdentity reports whether the trusted middleware supplied a complete
// authenticated caller. The model can never populate these fields.
func hasVerifiedIdentity(caller contract.CallerContext) bool {
	return caller.Authenticated &&
		strings.TrimSpace(caller.UserID) != "" &&
		strings.TrimSpace(caller.AccessToken) != ""
}

// isStaffRole reports whether the verified role may read operational data.
// ADMIN inherits DISPATCHER permissions.
func isStaffRole(role string) bool {
	return role == string(auth.RoleDispatcher) || role == string(auth.RoleAdmin)
}

func isCustomerRole(role string) bool { return role == string(auth.RoleCustomer) }

func isCourierRole(role string) bool { return role == string(auth.RoleCourier) }

func isMerchantRole(role string) bool { return role == string(auth.RoleMerchant) }

// requireParcelAccess is the single authorization gate for customer and staff
// parcel tools. It uses only the trusted caller context populated by the HTTP
// authentication middleware. Guests are asked to sign in; signed-in roles that
// are not allowed get a safe denial.
func requireParcelAccess(caller contract.CallerContext, signInMessage string) (contract.ToolResult, bool) {
	if !hasVerifiedIdentity(caller) {
		return deniedResult(signInMessage), false
	}

	switch auth.Role(caller.Role) {
	case auth.RoleCustomer, auth.RoleDispatcher, auth.RoleAdmin:
		return contract.ToolResult{}, true
	default:
		return deniedResult("This information is available only to customer accounts and authorized delivery staff."), false
	}
}

// requireStaffAccess gates dispatcher/admin operational reads.
func requireStaffAccess(caller contract.CallerContext) (contract.ToolResult, bool) {
	if !hasVerifiedIdentity(caller) {
		return deniedResult(signInStaffMessage), false
	}
	if !isStaffRole(caller.Role) {
		return deniedResult("This information is available only to dispatchers and administrators."), false
	}
	return contract.ToolResult{}, true
}

// requireCourierAccess gates the courier's own delivery reads.
func requireCourierAccess(caller contract.CallerContext) (contract.ToolResult, bool) {
	if !hasVerifiedIdentity(caller) {
		return deniedResult(signInCourierMessage), false
	}
	if caller.Role != string(auth.RoleCourier) {
		return deniedResult("This information is available only to courier accounts."), false
	}
	return contract.ToolResult{}, true
}

// requireMerchantAccess gates the merchant's own order and webhook reads.
func requireMerchantAccess(caller contract.CallerContext) (contract.ToolResult, bool) {
	if !hasVerifiedIdentity(caller) {
		return deniedResult(signInMerchantMessage), false
	}
	if caller.Role != string(auth.RoleMerchant) {
		return deniedResult("This information is available only to merchant accounts."), false
	}
	return contract.ToolResult{}, true
}

// isPrivilegedParcelRole reports whether the verified role may read parcels
// across customers for operational purposes.
func isPrivilegedParcelRole(role string) bool {
	return isStaffRole(role)
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
