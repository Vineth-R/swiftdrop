package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// userRequestsPerMinute is the limit for endpoints without a specific limit,
// per signed-in user (API v1.1 section 1.7).
const userRequestsPerMinute = 300

// AdminService is the user management use cases the handlers call.
// *service.AdminUsers satisfies it.
type AdminService interface {
	ListUsers(ctx context.Context, input service.ListUsersInput) (service.UserPage, error)
	GetUser(ctx context.Context, userID string) (service.AdminUser, error)
	CreateStaffUser(ctx context.Context, actorID string, input service.CreateStaffInput) (service.AdminUser, error)
	UpdateUser(ctx context.Context, actorID, userID string, input service.UpdateUserInput) (service.AdminUser, error)
	SetUserActive(ctx context.Context, actorID, userID string, active bool) error
	CurrentAccess(ctx context.Context, userID string) (service.CurrentAccess, error)
	ListCouriers(ctx context.Context, input service.ListCouriersInput) (service.CourierPage, error)
}

// AdminRoutes carries what the admin user endpoints and GET /couriers need.
type AdminRoutes struct {
	Service  AdminService
	Verifier auth.Verifier
	Cookies  CookieConfig
}

// WithAdminUsers mounts the admin user endpoints (API v1.1 sections 2.10 to
// 2.14, ADMIN only) and GET /couriers (section 2.15, DISPATCHER and ADMIN).
func WithAdminUsers(routes AdminRoutes) Option {
	return func(options *routerOptions) {
		options.admin = &routes
	}
}

type adminHandlers struct {
	service AdminService
	auth    *authHandlers
	logger  *slog.Logger
}

type createUserRequest struct {
	Email          string                 `json:"email"`
	Password       string                 `json:"password"`
	FullName       string                 `json:"fullName"`
	Phone          string                 `json:"phone"`
	Role           string                 `json:"role"`
	CourierProfile *courierProfileRequest `json:"courierProfile"`
}

type courierProfileRequest struct {
	VehicleType   string `json:"vehicleType"`
	VehicleNumber string `json:"vehicleNumber"`
	HomeZoneID    string `json:"homeZoneId"`
}

type updateUserRequest struct {
	FullName       *string              `json:"fullName"`
	Phone          *string              `json:"phone"`
	Role           *string              `json:"role"`
	CourierProfile *courierPatchRequest `json:"courierProfile"`
}

type courierPatchRequest struct {
	VehicleType   *string `json:"vehicleType"`
	VehicleNumber *string `json:"vehicleNumber"`
	HomeZoneID    *string `json:"homeZoneId"`
	IsAvailable   *bool   `json:"isAvailable"`
}

type statusRequest struct {
	IsActive *bool `json:"isActive"`
}

// adminUserItem is a user as listed (API v1.1 section 2.10).
type adminUserItem struct {
	ID        string    `json:"id"`
	Email     string    `json:"email"`
	FullName  string    `json:"fullName"`
	Phone     string    `json:"phone"`
	Role      auth.Role `json:"role"`
	IsActive  bool      `json:"isActive"`
	CreatedAt string    `json:"createdAt"`
}

// adminUserDetail is the full administrative view (sections 2.11 to 2.13).
type adminUserDetail struct {
	adminUserItem
	CourierProfile *courierProfileBody `json:"courierProfile,omitempty"`
	LastLoginAt    *string             `json:"lastLoginAt"`
	UpdatedAt      string              `json:"updatedAt"`
}

type courierProfileBody struct {
	VehicleType   *string `json:"vehicleType"`
	VehicleNumber *string `json:"vehicleNumber"`
	HomeZoneID    *string `json:"homeZoneId"`
	IsAvailable   bool    `json:"isAvailable"`
}

type statusResponse struct {
	ID       string `json:"id"`
	IsActive bool   `json:"isActive"`
}

type courierItem struct {
	ID          string  `json:"id"`
	FullName    string  `json:"fullName"`
	Phone       string  `json:"phone"`
	VehicleType *string `json:"vehicleType"`
	HomeZoneID  *string `json:"homeZoneId"`
	IsAvailable bool    `json:"isAvailable"`
}

func registerAdminRoutes(router *gin.Engine, routes AdminRoutes, logger *slog.Logger) {
	handlers := &adminHandlers{service: routes.Service, auth: &authHandlers{cookies: routes.Cookies, logger: logger}, logger: logger}
	limiter := newRateLimiter(userRequestsPerMinute, time.Minute, nil)
	authenticate := auth.Authenticate(routes.Verifier, routes.Cookies.AccessName)

	// Nginx forwards /api/v1/admin/users/ with a trailing slash, and Gin would
	// answer a missing-slash route with a redirect that drops the /api/v1
	// prefix. The collection routes are therefore registered both ways.
	//
	// Each group checks the role in the token first (cheap), then the caller's
	// current account in the database, so a user who was deactivated or demoted
	// after their token was issued is refused at once instead of when it expires.
	admin := router.Group("/admin/users", noStore(), authenticate, auth.RequireRoles(auth.RoleAdmin),
		handlers.requireCurrentAccess(auth.RoleAdmin), limitByUser(limiter))
	for _, path := range []string{"", "/"} {
		admin.GET(path, handlers.listUsers)
		admin.POST(path, handlers.createUser)
	}
	admin.GET("/:userId", handlers.getUser)
	admin.PATCH("/:userId", handlers.updateUser)
	admin.PATCH("/:userId/status", handlers.setStatus)

	// ADMIN also satisfies DISPATCHER (API v1.1 section 1.3).
	couriers := router.Group("/couriers", noStore(), authenticate, auth.RequireRoles(auth.RoleDispatcher),
		handlers.requireCurrentAccess(auth.RoleDispatcher), limitByUser(limiter))
	for _, path := range []string{"", "/"} {
		couriers.GET(path, handlers.listCouriers)
	}
}

// requireCurrentAccess refuses a caller whose account is now disabled or no
// longer holds the required role, whatever their access token still says. It
// must run after auth.Authenticate. A caller whose account no longer exists gets
// 401; a disabled one gets 403 ACCOUNT_DISABLED (API v1.1 section 1.9 and the
// users table, Database Design 3.2).
func (h *adminHandlers) requireCurrentAccess(required auth.Role) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFrom(c)
		if !ok {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}

		access, err := h.service.CurrentAccess(c.Request.Context(), claims.UserID)
		switch {
		case errors.Is(err, service.ErrUserNotFound):
			httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		case err != nil:
			h.logger.ErrorContext(c.Request.Context(), "could not check the caller's account",
				"request_id", requestid.FromGin(c), "path", c.FullPath(), "error", err.Error())
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		case !access.Active:
			httpx.Error(c, http.StatusForbidden, "ACCOUNT_DISABLED", "this account has been disabled")
		case !access.Role.Satisfies(required):
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "not allowed")
		case access.SessionVersion != claims.SessionVersion:
			httpx.Error(c, http.StatusUnauthorized, "SESSION_REVOKED", "this session is no longer valid")
		default:
			c.Next()
		}
	}
}

// limitByUser applies a per-user request limit. It must run after
// auth.Authenticate.
func limitByUser(limiter *rateLimiter) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims, ok := auth.ClaimsFrom(c)
		if !ok {
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
			return
		}
		allowed, remaining, resetIn := limiter.allow(claims.UserID)
		c.Header("X-RateLimit-Remaining", strconv.Itoa(remaining))
		if !allowed {
			rejectRateLimited(c, resetIn)
			return
		}
		c.Next()
	}
}

// listUsers handles GET /admin/users (API v1.1 section 2.10).
func (h *adminHandlers) listUsers(c *gin.Context) {
	query := queryReader{c: c, issues: validation.New()}
	input := service.ListUsersInput{
		Role:     c.Query("role"),
		IsActive: query.boolean("isActive"),
		Search:   c.Query("q"),
		Sort:     c.Query("sort"),
		Page:     query.integer("page"),
		PageSize: query.integer("pageSize"),
	}
	if query.invalid() {
		return
	}

	page, err := h.service.ListUsers(c.Request.Context(), input)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]adminUserItem, 0, len(page.Users))
	for _, user := range page.Users {
		items = append(items, userItem(user))
	}
	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page.Page, page.PageSize, page.TotalItems))
}

// createUser handles POST /admin/users (API v1.1 section 2.11).
func (h *adminHandlers) createUser(c *gin.Context) {
	var request createUserRequest
	if !bindJSON(c, &request) {
		return
	}
	input := service.CreateStaffInput{
		Email: request.Email, Password: request.Password, FullName: request.FullName,
		Phone: request.Phone, Role: request.Role,
	}
	if profile := request.CourierProfile; profile != nil {
		input.Courier = &service.CourierProfileInput{
			VehicleType: profile.VehicleType, VehicleNumber: profile.VehicleNumber, HomeZoneID: profile.HomeZoneID,
		}
	}

	actor, ok := actorID(c)
	if !ok {
		return
	}
	created, err := h.service.CreateStaffUser(c.Request.Context(), actor, input)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, userDetail(created))
}

// getUser handles GET /admin/users/{userId} (API v1.1 section 2.12).
func (h *adminHandlers) getUser(c *gin.Context) {
	user, err := h.service.GetUser(c.Request.Context(), c.Param("userId"))
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, userDetail(user))
}

// updateUser handles PATCH /admin/users/{userId} (API v1.1 section 2.13).
func (h *adminHandlers) updateUser(c *gin.Context) {
	var request updateUserRequest
	if !bindJSON(c, &request) {
		return
	}
	input := service.UpdateUserInput{FullName: request.FullName, Phone: request.Phone, Role: request.Role}
	if patch := request.CourierProfile; patch != nil {
		input.Courier = &service.CourierProfilePatch{
			VehicleType: patch.VehicleType, VehicleNumber: patch.VehicleNumber,
			HomeZoneID: patch.HomeZoneID, IsAvailable: patch.IsAvailable,
		}
	}

	actor, ok := actorID(c)
	if !ok {
		return
	}
	updated, err := h.service.UpdateUser(c.Request.Context(), actor, c.Param("userId"), input)
	if err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, userDetail(updated))
}

// setStatus handles PATCH /admin/users/{userId}/status (API v1.1 section 2.14).
func (h *adminHandlers) setStatus(c *gin.Context) {
	var request statusRequest
	if !bindJSON(c, &request) {
		return
	}
	if request.IsActive == nil {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.",
			[]httpx.ErrorItem{{Field: "isActive", Issue: "is required"}})
		return
	}

	actor, ok := actorID(c)
	if !ok {
		return
	}
	userID := c.Param("userId")
	if err := h.service.SetUserActive(c.Request.Context(), actor, userID, *request.IsActive); err != nil {
		h.fail(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, statusResponse{ID: userID, IsActive: *request.IsActive})
}

// listCouriers handles GET /couriers (API v1.1 section 2.15).
func (h *adminHandlers) listCouriers(c *gin.Context) {
	query := queryReader{c: c, issues: validation.New()}
	input := service.ListCouriersInput{
		IsAvailable: query.boolean("isAvailable"),
		ZoneID:      c.Query("zoneId"),
		Page:        query.integer("page"),
		PageSize:    query.integer("pageSize"),
	}
	if query.invalid() {
		return
	}

	page, err := h.service.ListCouriers(c.Request.Context(), input)
	if err != nil {
		h.fail(c, err)
		return
	}
	items := make([]courierItem, 0, len(page.Couriers))
	for _, courier := range page.Couriers {
		items = append(items, courierItem{
			ID: courier.ID, FullName: courier.FullName, Phone: deref(courier.Phone),
			VehicleType: courier.VehicleType, HomeZoneID: courier.HomeZoneID, IsAvailable: courier.IsAvailable,
		})
	}
	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page.Page, page.PageSize, page.TotalItems))
}

func (h *adminHandlers) fail(c *gin.Context, err error) {
	switch {
	case errors.Is(err, service.ErrUserNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "user not found")
	case errors.Is(err, service.ErrCannotDeactivateSelf):
		httpx.Error(c, http.StatusConflict, "CANNOT_DEACTIVATE_SELF", "you cannot deactivate your own account")
	case errors.Is(err, service.ErrCannotChangeOwnRole):
		httpx.Error(c, http.StatusConflict, "CONFLICT", "you cannot change your own role")
	default:
		// Validation errors, duplicate emails and unexpected failures share the
		// mapping used by the auth endpoints.
		h.auth.fail(c, err)
	}
}

// actorID returns the signed-in administrator. It answers 500 itself when the
// claims are missing, which means a routing mistake rather than a client error.
func actorID(c *gin.Context) (string, bool) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "internal server error")
		return "", false
	}
	return claims.UserID, true
}

// queryReader reads typed query parameters and collects every malformed one, so
// the caller gets all problems in a single 422.
type queryReader struct {
	c      *gin.Context
	issues *validation.Collector
}

// boolean reads "true" or "false" only; an absent parameter is nil.
func (q *queryReader) boolean(name string) *bool {
	switch value := q.c.Query(name); value {
	case "":
		return nil
	case "true", "false":
		result := value == "true"
		return &result
	default:
		q.issues.Add(name, "must be true or false")
		return nil
	}
}

// integer reads a whole number; an absent parameter is 0, which the service
// treats as "use the default".
func (q *queryReader) integer(name string) int {
	value := q.c.Query(name)
	if value == "" {
		return 0
	}
	number, err := strconv.Atoi(value)
	if err != nil {
		q.issues.Add(name, "must be a whole number")
		return 0
	}
	return number
}

// invalid answers 422 with every malformed parameter and reports whether it
// did.
func (q *queryReader) invalid() bool {
	if q.issues.Empty() {
		return false
	}
	httpx.ErrorWithDetails(q.c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(q.issues.Issues()))
	return true
}

func userItem(user service.AdminUser) adminUserItem {
	return adminUserItem{
		ID: user.ID, Email: user.Email, FullName: user.FullName, Phone: deref(user.Phone),
		Role: user.Role, IsActive: user.IsActive, CreatedAt: user.CreatedAt.UTC().Format(time.RFC3339),
	}
}

func userDetail(user service.AdminUser) adminUserDetail {
	detail := adminUserDetail{
		adminUserItem: userItem(user),
		UpdatedAt:     user.UpdatedAt.UTC().Format(time.RFC3339),
	}
	if user.LastLoginAt != nil {
		formatted := user.LastLoginAt.UTC().Format(time.RFC3339)
		detail.LastLoginAt = &formatted
	}
	if profile := user.Courier; profile != nil {
		detail.CourierProfile = &courierProfileBody{
			VehicleType: profile.VehicleType, VehicleNumber: profile.VehicleNumber,
			HomeZoneID: profile.HomeZoneID, IsAvailable: profile.IsAvailable,
		}
	}
	return detail
}

func deref(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}
