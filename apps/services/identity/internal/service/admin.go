package service

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	defaultPage     = 1
	defaultPageSize = 20
	maxPageSize     = 100

	maxSearchLength        = 100
	maxVehicleTypeLength   = 30
	maxVehicleNumberLength = 20
)

var (
	// ErrCannotDeactivateSelf means an administrator tried to deactivate their
	// own account (409 CANNOT_DEACTIVATE_SELF).
	ErrCannotDeactivateSelf = errors.New("cannot deactivate your own account")

	// ErrCannotChangeOwnRole means an administrator tried to change their own
	// role, which could leave the system without an administrator (409 CONFLICT).
	ErrCannotChangeOwnRole = errors.New("cannot change your own role")
)

// AdminUser is the administrative view of a user (API v1.1 sections 2.10 to
// 2.13).
type AdminUser = repository.AdminUser

// CourierListing is one courier offered for assignment (API v1.1 section 2.15).
type CourierListing = repository.CourierListing

// staffRoles can be created through POST /admin/users (API v1.1 section 2.11).
var staffRoles = map[auth.Role]bool{
	auth.RoleCourier:    true,
	auth.RoleDispatcher: true,
	auth.RoleAdmin:      true,
}

// assignableRoles can be set through PATCH /admin/users/{userId}. MERCHANT is
// excluded: a merchant account belongs to a merchant record owned by the
// Merchant service.
var assignableRoles = map[auth.Role]bool{
	auth.RoleCustomer:   true,
	auth.RoleCourier:    true,
	auth.RoleDispatcher: true,
	auth.RoleAdmin:      true,
}

// sortFields are the fields the user list can be ordered by.
var sortFields = map[string]bool{"createdAt": true, "fullName": true, "email": true}

// AdminStore is the persistence the admin use cases need.
// *repository.Repository satisfies it.
type AdminStore interface {
	ListAdminUsers(ctx context.Context, filter repository.UserFilter) ([]repository.AdminUser, int, error)
	AdminUserByID(ctx context.Context, userID string) (repository.AdminUser, error)
	CreateStaffUser(ctx context.Context, params repository.CreateStaffParams, actorID string) (repository.AdminUser, error)
	UpdateAdminUser(ctx context.Context, userID string, change repository.UserChange, actorID string) (repository.AdminUser, error)
	SetUserActive(ctx context.Context, userID string, active bool, actorID string, now time.Time) error
	ListCouriers(ctx context.Context, filter repository.CourierFilter) ([]repository.CourierListing, int, error)
}

// AdminUsers implements user management for administrators and the courier
// list for dispatchers (API v1.1 sections 2.10 to 2.15). It reuses Service for
// the password hasher and clock. Who may call it is decided by the HTTP layer.
type AdminUsers struct {
	base  *Service
	store AdminStore
}

// NewAdminUsers returns an AdminUsers.
func NewAdminUsers(base *Service, store AdminStore) (*AdminUsers, error) {
	if base == nil || store == nil {
		return nil, errors.New("admin users requires a service and a store")
	}
	return &AdminUsers{base: base, store: store}, nil
}

// CurrentAccess is a caller's role and active flag as stored now, not as they
// were when the caller's access token was issued.
type CurrentAccess struct {
	Role   auth.Role
	Active bool
}

// CurrentAccess returns the stored role and active flag of the user. An access
// token is a signed copy of the role at the time it was issued and stays valid
// until it expires, so the admin and courier endpoints call this to stop a
// deactivated or demoted user from keeping their old privileges for the rest of
// the token's life. It returns ErrUserNotFound for an unknown or malformed ID.
func (a *AdminUsers) CurrentAccess(ctx context.Context, userID string) (CurrentAccess, error) {
	user, err := a.GetUser(ctx, userID)
	if err != nil {
		return CurrentAccess{}, err
	}
	return CurrentAccess{Role: user.Role, Active: user.IsActive}, nil
}

// ListUsersInput filters the admin user list. Zero Page and PageSize mean the
// defaults (page 1, 20 per page).
type ListUsersInput struct {
	Role     string
	IsActive *bool
	Search   string
	Sort     string // "createdAt:desc", "fullName", "email:asc", ...
	Page     int
	PageSize int
}

// UserPage is one page of users.
type UserPage struct {
	Users      []AdminUser
	Page       int
	PageSize   int
	TotalItems int
}

// ListUsers returns a page of users. A bad filter returns a *ValidationError.
func (a *AdminUsers) ListUsers(ctx context.Context, input ListUsersInput) (UserPage, error) {
	page, pageSize, issues := normalizePaging(input.Page, input.PageSize)

	filter := repository.UserFilter{
		IsActive:  input.IsActive,
		Search:    strings.TrimSpace(input.Search),
		SortField: "createdAt",
		SortDesc:  true,
		Limit:     pageSize,
		Offset:    (page - 1) * pageSize,
	}
	if input.Role != "" {
		role, ok := auth.ParseRole(input.Role)
		if !ok {
			issues = append(issues, Issue{Field: "role", Issue: "must be one of CUSTOMER, MERCHANT, COURIER, DISPATCHER, ADMIN"})
		}
		filter.Role = role
	}
	if utf8.RuneCountInString(filter.Search) > maxSearchLength {
		issues = append(issues, Issue{Field: "q", Issue: fmt.Sprintf("must be at most %d characters", maxSearchLength)})
	}
	if input.Sort != "" {
		field, desc, ok := parseSort(input.Sort)
		if !ok {
			issues = append(issues, Issue{Field: "sort", Issue: "must be createdAt, fullName or email, optionally followed by :asc or :desc"})
		}
		filter.SortField, filter.SortDesc = field, desc
	}
	if len(issues) > 0 {
		return UserPage{}, &ValidationError{Issues: issues}
	}

	users, total, err := a.store.ListAdminUsers(ctx, filter)
	if err != nil {
		return UserPage{}, fmt.Errorf("list users: %w", err)
	}
	return UserPage{Users: users, Page: page, PageSize: pageSize, TotalItems: total}, nil
}

// GetUser returns one user, or ErrUserNotFound for an unknown or malformed ID.
func (a *AdminUsers) GetUser(ctx context.Context, userID string) (AdminUser, error) {
	if !ValidUUID(userID) {
		return AdminUser{}, ErrUserNotFound
	}
	user, err := a.store.AdminUserByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return AdminUser{}, ErrUserNotFound
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("get user: %w", err)
	}
	return user, nil
}

// CourierProfileInput is the courier profile of a new staff account.
type CourierProfileInput struct {
	VehicleType   string
	VehicleNumber string
	HomeZoneID    string
}

// CreateStaffInput is a new courier, dispatcher or administrator account.
type CreateStaffInput struct {
	Email    string
	Password string
	FullName string
	Phone    string
	Role     string
	Courier  *CourierProfileInput
}

// CreateStaffUser creates a staff account and audits it. It returns a
// *ValidationError for bad input and ErrEmailAlreadyExists for a duplicate
// email.
func (a *AdminUsers) CreateStaffUser(ctx context.Context, actorID string, input CreateStaffInput) (AdminUser, error) {
	email := NormalizeEmail(input.Email)

	var issues []Issue
	// The email, password, name and phone rules are the registration rules.
	if err := validateRegistration(RegisterInput{
		Email: input.Email, Password: input.Password, FullName: input.FullName, Phone: input.Phone,
	}, email); err != nil {
		var validation *ValidationError
		if errors.As(err, &validation) {
			issues = append(issues, validation.Issues...)
		}
	}

	role, ok := auth.ParseRole(input.Role)
	if !ok || !staffRoles[role] {
		issues = append(issues, Issue{Field: "role", Issue: "must be one of COURIER, DISPATCHER, ADMIN"})
	}
	var courier *repository.CourierProfileInput
	switch {
	case ok && role == auth.RoleCourier && input.Courier == nil:
		issues = append(issues, Issue{Field: "courierProfile", Issue: "is required when role is COURIER"})
	case input.Courier != nil && ok && role != auth.RoleCourier:
		issues = append(issues, Issue{Field: "courierProfile", Issue: "is allowed only when role is COURIER"})
	case input.Courier != nil:
		issues = append(issues, validateCourierFields(
			&input.Courier.VehicleType, &input.Courier.VehicleNumber, &input.Courier.HomeZoneID)...)
		courier = &repository.CourierProfileInput{
			VehicleType:   strings.TrimSpace(input.Courier.VehicleType),
			VehicleNumber: strings.TrimSpace(input.Courier.VehicleNumber),
			HomeZoneID:    strings.TrimSpace(input.Courier.HomeZoneID),
		}
	}
	if len(issues) > 0 {
		return AdminUser{}, &ValidationError{Issues: issues}
	}

	hash, err := a.base.hasher.Hash(input.Password)
	if err != nil {
		return AdminUser{}, fmt.Errorf("create staff user: %w", err)
	}
	created, err := a.store.CreateStaffUser(ctx, repository.CreateStaffParams{
		Email:        email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(input.FullName),
		Phone:        input.Phone,
		Role:         role,
		Courier:      courier,
	}, actorID)
	if errors.Is(err, repository.ErrEmailTaken) {
		return AdminUser{}, ErrEmailAlreadyExists
	}
	if err != nil {
		return AdminUser{}, fmt.Errorf("create staff user: %w", err)
	}
	a.base.logger.InfoContext(ctx, "staff user created", "actor_id", actorID, "user_id", created.ID, "role", string(created.Role))
	return created, nil
}

// CourierProfilePatch is a partial courier profile update; nil means unchanged.
type CourierProfilePatch struct {
	VehicleType   *string
	VehicleNumber *string
	HomeZoneID    *string
	IsAvailable   *bool
}

// UpdateUserInput is a partial user update; nil means unchanged.
type UpdateUserInput struct {
	FullName *string
	Phone    *string
	Role     *string
	Courier  *CourierProfilePatch
}

// UpdateUser edits a user and audits it. It returns ErrUserNotFound, a
// *ValidationError, or ErrCannotChangeOwnRole.
func (a *AdminUsers) UpdateUser(ctx context.Context, actorID, userID string, input UpdateUserInput) (AdminUser, error) {
	if !ValidUUID(userID) {
		return AdminUser{}, ErrUserNotFound
	}

	var issues []Issue
	change := repository.UserChange{}
	if input.FullName == nil && input.Phone == nil && input.Role == nil && input.Courier == nil {
		issues = append(issues, Issue{Field: "body", Issue: "must contain at least one of fullName, phone, role, courierProfile"})
	}
	if input.FullName != nil {
		name := strings.TrimSpace(*input.FullName)
		if length := utf8.RuneCountInString(name); length < minFullNameLength || length > maxFullNameLength {
			issues = append(issues, Issue{Field: "fullName", Issue: "must be 2 to 150 characters"})
		}
		change.FullName = &name
	}
	if input.Phone != nil {
		if !e164.MatchString(*input.Phone) {
			issues = append(issues, Issue{Field: "phone", Issue: "must be in E.164 format, for example +94771234567"})
		}
		change.Phone = input.Phone
	}
	if input.Role != nil {
		role, ok := auth.ParseRole(*input.Role)
		if !ok || !assignableRoles[role] {
			issues = append(issues, Issue{Field: "role", Issue: "must be one of CUSTOMER, COURIER, DISPATCHER, ADMIN"})
		}
		change.Role = &role
	}
	if input.Courier != nil {
		issues = append(issues, validateCourierFields(input.Courier.VehicleType, input.Courier.VehicleNumber, input.Courier.HomeZoneID)...)
		change.Courier = &repository.CourierProfileChange{
			VehicleType:   trimmedPointer(input.Courier.VehicleType),
			VehicleNumber: trimmedPointer(input.Courier.VehicleNumber),
			HomeZoneID:    trimmedPointer(input.Courier.HomeZoneID),
			IsAvailable:   input.Courier.IsAvailable,
		}
	}
	if len(issues) > 0 {
		return AdminUser{}, &ValidationError{Issues: issues}
	}

	if change.Role != nil {
		current, err := a.GetUser(ctx, userID)
		if err != nil {
			return AdminUser{}, err
		}
		if current.Role == auth.RoleMerchant {
			return AdminUser{}, &ValidationError{Issues: []Issue{{Field: "role", Issue: "cannot be changed for a merchant account"}}}
		}
		if userID == actorID && *change.Role != current.Role {
			return AdminUser{}, ErrCannotChangeOwnRole
		}
	}

	updated, err := a.store.UpdateAdminUser(ctx, userID, change, actorID)
	switch {
	case errors.Is(err, repository.ErrNotFound):
		return AdminUser{}, ErrUserNotFound
	case errors.Is(err, repository.ErrCourierProfileRequired):
		return AdminUser{}, &ValidationError{Issues: []Issue{{Field: "courierProfile", Issue: "is required when the role is COURIER"}}}
	case errors.Is(err, repository.ErrCourierProfileNotAllowed):
		return AdminUser{}, &ValidationError{Issues: []Issue{{Field: "courierProfile", Issue: "is allowed only for couriers"}}}
	case err != nil:
		return AdminUser{}, fmt.Errorf("update user: %w", err)
	}
	a.base.logger.InfoContext(ctx, "user updated", "actor_id", actorID, "user_id", userID)
	return updated, nil
}

// SetUserActive activates or deactivates a user and audits it. Deactivation
// revokes the user's refresh tokens. It returns ErrUserNotFound or
// ErrCannotDeactivateSelf.
func (a *AdminUsers) SetUserActive(ctx context.Context, actorID, userID string, active bool) error {
	if !ValidUUID(userID) {
		return ErrUserNotFound
	}
	if !active && userID == actorID {
		return ErrCannotDeactivateSelf
	}
	err := a.store.SetUserActive(ctx, userID, active, actorID, a.base.nowUTC())
	if errors.Is(err, repository.ErrNotFound) {
		return ErrUserNotFound
	}
	if err != nil {
		return fmt.Errorf("set user status: %w", err)
	}
	a.base.logger.InfoContext(ctx, "user status changed", "actor_id", actorID, "user_id", userID, "is_active", active)
	return nil
}

// ListCouriersInput filters the courier list. Zero Page and PageSize mean the
// defaults.
type ListCouriersInput struct {
	IsAvailable *bool
	ZoneID      string
	Page        int
	PageSize    int
}

// CourierPage is one page of couriers.
type CourierPage struct {
	Couriers   []CourierListing
	Page       int
	PageSize   int
	TotalItems int
}

// ListCouriers returns a page of active couriers. A malformed zone ID returns a
// *ValidationError.
func (a *AdminUsers) ListCouriers(ctx context.Context, input ListCouriersInput) (CourierPage, error) {
	page, pageSize, issues := normalizePaging(input.Page, input.PageSize)
	zone := strings.TrimSpace(input.ZoneID)
	if zone != "" && !ValidUUID(zone) {
		issues = append(issues, Issue{Field: "zoneId", Issue: "must be a UUID"})
	}
	if len(issues) > 0 {
		return CourierPage{}, &ValidationError{Issues: issues}
	}

	couriers, total, err := a.store.ListCouriers(ctx, repository.CourierFilter{
		IsAvailable: input.IsAvailable,
		ZoneID:      zone,
		Limit:       pageSize,
		Offset:      (page - 1) * pageSize,
	})
	if err != nil {
		return CourierPage{}, fmt.Errorf("list couriers: %w", err)
	}
	return CourierPage{Couriers: couriers, Page: page, PageSize: pageSize, TotalItems: total}, nil
}

// normalizePaging applies the defaults and limits of API v1.1 section 1.5:
// page from 1, pageSize from 1 to 100, default 20.
func normalizePaging(page, pageSize int) (int, int, []Issue) {
	var issues []Issue
	if page == 0 {
		page = defaultPage
	}
	if page < 1 {
		issues = append(issues, Issue{Field: "page", Issue: "must be 1 or greater"})
		page = defaultPage
	}
	if pageSize == 0 {
		pageSize = defaultPageSize
	}
	if pageSize < 1 || pageSize > maxPageSize {
		issues = append(issues, Issue{Field: "pageSize", Issue: fmt.Sprintf("must be between 1 and %d", maxPageSize)})
		pageSize = defaultPageSize
	}
	return page, pageSize, issues
}

// parseSort reads "field", "field:asc" or "field:desc". A bare field sorts
// ascending.
func parseSort(value string) (field string, desc, ok bool) {
	field, direction, hasDirection := strings.Cut(value, ":")
	if !sortFields[field] {
		return "", false, false
	}
	if !hasDirection {
		return field, false, true
	}
	switch direction {
	case "asc":
		return field, false, true
	case "desc":
		return field, true, true
	}
	return "", false, false
}

// validateCourierFields checks the optional courier profile fields. Nil or
// blank fields are skipped; issues are named courierProfile.<field>.
func validateCourierFields(vehicleType, vehicleNumber, homeZoneID *string) []Issue {
	var issues []Issue
	if vehicleType != nil && utf8.RuneCountInString(strings.TrimSpace(*vehicleType)) > maxVehicleTypeLength {
		issues = append(issues, Issue{Field: "courierProfile.vehicleType", Issue: fmt.Sprintf("must be at most %d characters", maxVehicleTypeLength)})
	}
	if vehicleNumber != nil && utf8.RuneCountInString(strings.TrimSpace(*vehicleNumber)) > maxVehicleNumberLength {
		issues = append(issues, Issue{Field: "courierProfile.vehicleNumber", Issue: fmt.Sprintf("must be at most %d characters", maxVehicleNumberLength)})
	}
	if zone := trimmedPointer(homeZoneID); zone != nil && !ValidUUID(*zone) {
		issues = append(issues, Issue{Field: "courierProfile.homeZoneId", Issue: "must be a UUID"})
	}
	return issues
}

// trimmedPointer returns a trimmed copy of *value, or nil when value is nil or
// blank, so blank input means "not given".
func trimmedPointer(value *string) *string {
	if value == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}
