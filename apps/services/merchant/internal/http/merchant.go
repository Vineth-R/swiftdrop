package servicehttp

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/notify"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/onboarding"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/orders"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type identityKey struct{}

type Identity struct {
	UserID     string
	MerchantID string
	Role       string
}

func WithIdentity(c *gin.Context, identity Identity) {
	c.Set(identityKey{}, identity)
}

func identityFrom(c *gin.Context) (Identity, bool) {
	value, ok := c.Get(identityKey{})
	if !ok {
		return Identity{}, false
	}
	identity, ok := value.(Identity)
	return identity, ok && identity.UserID != ""
}

type merchantHandler struct {
	repository         *repository.Repository
	onboarding         *onboarding.Service
	identity           *identityclient.Client
	orders             *orders.Service
	notifier           *notify.Notifier
	internalTokenValue string
	cookieName         string
	logger             *slog.Logger
}

func registerMerchantRoutes(router *gin.Engine, options Options) {
	repo := options.Repository
	cookie := options.CookieName
	if cookie == "" {
		cookie = "sd_access"
	}
	h := &merchantHandler{
		repository: repo, onboarding: options.Onboarding, identity: options.Identity,
		orders: options.Orders, notifier: options.Notifier,
		internalTokenValue: options.InternalToken, cookieName: cookie, logger: options.Logger,
	}

	limiter := httpx.NewRateLimiter(10, time.Minute)
	router.POST("/merchants/register", limiter.Middleware(), h.register)
	router.GET("/internal/merchants/owners/:userId/approval", h.internalApproval)

	merchants := router.Group("/merchants", noStore())
	admin := router.Group("/admin/merchants", noStore())
	var manager *auth.TokenManager
	if options.AccessSecret == "" {
		merchants.Use(unavailable("authentication is not configured"))
		admin.Use(unavailable("authentication is not configured"))
	} else {
		var err error
		manager, err = auth.NewTokenManager(options.AccessSecret, time.Minute)
		if err == nil {
			merchants.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleMerchant), h.requireSession)
			admin.Use(auth.Authenticate(manager, cookie), auth.RequireRoles(auth.RoleAdmin))
		} else {
			manager = nil
			merchants.Use(unavailable("authentication is not configured"))
			admin.Use(unavailable("authentication is not configured"))
		}
	}

	merchants.GET("/me", h.profile)
	merchants.GET("/me/keys", h.listKeys)
	merchants.POST("/me/keys", h.createKey)
	merchants.DELETE("/me/keys/:keyId", h.revokeKey)
	merchants.PATCH("/me", h.updateProfile)

	admin.GET("", h.list)
	admin.GET("/:merchantId", h.adminDetail)
	admin.POST("/:merchantId/approve", h.approve)
	admin.POST("/:merchantId/reject", h.reject)
	admin.POST("/:merchantId/suspend", h.suspend)

	registerMerchantOrderRoutes(router, h, manager)
}

func (h *merchantHandler) register(c *gin.Context) {
	if h.onboarding == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant registration is not configured")
		return
	}
	var input onboarding.RegisterInput
	if c.ContentType() != "application/json" {
		httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be application/json")
		return
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 64<<10)
	if err := c.ShouldBindJSON(&input); err != nil {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request body must be valid JSON")
		return
	}
	input.IdempotencyKey = c.GetHeader("Idempotency-Key")
	result, err := h.onboarding.Register(c.Request.Context(), input)
	if err != nil {
		h.onboardingError(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, gin.H{"merchantId": result.MerchantID, "userId": result.UserID, "status": result.Status})
}

func (h *merchantHandler) requireSession(c *gin.Context) {
	if h.requireSessionCore(c) {
		c.Next()
	}
}

func (h *merchantHandler) requireSessionCore(c *gin.Context) bool {
	claims, ok := auth.ClaimsFrom(c)
	if !ok || h.identity == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication could not be verified")
		return false
	}
	state, err := h.identity.SessionState(c.Request.Context(), claims.UserID)
	if err != nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "authentication could not be verified")
		return false
	}
	if !sessionIsCurrent(claims, state) {
		if h.logger != nil {
			h.logger.WarnContext(c.Request.Context(), "merchant session rejected",
				"user_id", claims.UserID, "claim_role", claims.Role, "claim_version", claims.SessionVersion,
				"state_role", state.Role, "state_active", state.IsActive, "state_version", state.SessionVersion)
		}
		httpx.Error(c, http.StatusUnauthorized, "SESSION_REVOKED", "this session is no longer valid")
		return false
	}
	merchant, err := h.repository.MerchantByOwnerUserID(c.Request.Context(), claims.UserID)
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_PROVISIONED", "merchant account is not provisioned")
		return false
	}
	if err != nil {
		h.internalError(c, "merchant_lookup", "merchant account could not be read", err)
		return false
	}
	WithIdentity(c, Identity{UserID: claims.UserID, MerchantID: merchant.ID, Role: string(auth.RoleMerchant)})
	c.Set("merchant.status", merchant.Status)
	return true
}

func sessionIsCurrent(claims auth.Claims, state identityclient.SessionState) bool {
	return state.IsActive && state.Role == string(claims.Role) && state.SessionVersion == claims.SessionVersion
}

func (h *merchantHandler) merchantIdentity(c *gin.Context) (Identity, repository.Merchant, bool) {
	identity, ok := identityFrom(c)
	if !ok || identity.MerchantID == "" || identity.Role != string(auth.RoleMerchant) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "merchant authentication required")
		return Identity{}, repository.Merchant{}, false
	}
	merchant, err := h.repository.MerchantByID(c.Request.Context(), identity.MerchantID)
	if err != nil {
		h.handleRepositoryError(c, err)
		return Identity{}, repository.Merchant{}, false
	}
	return identity, merchant, true
}

func (h *merchantHandler) requireApproved(c *gin.Context, merchant repository.Merchant) bool {
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "merchant approval is required")
		return false
	}
	return true
}

func (h *merchantHandler) profile(c *gin.Context) {
	_, merchant, ok := h.merchantIdentity(c)
	if !ok {
		return
	}
	httpx.Success(c, http.StatusOK, merchantResponse(merchant))
}

type profileRequest struct {
	ContactEmail  string `json:"contactEmail"`
	ContactPhone  string `json:"contactPhone"`
	PickupAddress string `json:"pickupAddress"`
	PickupZoneID  string `json:"pickupZoneId"`
}

func (h *merchantHandler) updateProfile(c *gin.Context) {
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok {
		return
	}
	if merchant.Status != repository.MerchantStatusPending && merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "MERCHANT_NOT_APPROVED", "this merchant account cannot be edited")
		return
	}
	var request profileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "invalid merchant profile")
		return
	}
	issues := validation.New()
	issues.Email("contactEmail", strings.TrimSpace(request.ContactEmail))
	issues.RequiredExact("contactPhone", request.ContactPhone)
	issues.RequiredExact("pickupAddress", request.PickupAddress)
	issues.Custom("pickupZoneId", validation.UUID(strings.TrimSpace(request.PickupZoneID)), "must be a UUID")
	if !issues.Empty() {
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
		return
	}
	updated, err := h.repository.UpdateProfile(c.Request.Context(), identity.MerchantID, repository.ProfileUpdate{
		ContactEmail: strings.TrimSpace(request.ContactEmail), ContactPhone: strings.TrimSpace(request.ContactPhone),
		PickupAddress: strings.TrimSpace(request.PickupAddress), PickupZoneID: strings.TrimSpace(request.PickupZoneID),
	})
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, merchantResponse(updated))
}

func (h *merchantHandler) listKeys(c *gin.Context) {
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	keys, err := h.repository.APIClientsByMerchant(c.Request.Context(), identity.MerchantID)
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for _, key := range keys {
		data = append(data, gin.H{"id": key.ID, "name": key.Name, "keyPrefix": key.KeyPrefix, "status": key.Status, "lastUsedAt": key.LastUsedAt, "createdAt": key.CreatedAt, "revokedAt": key.RevokedAt})
	}
	httpx.Success(c, http.StatusOK, data)
}

type keyRequest struct {
	Name string `json:"name"`
}

func (h *merchantHandler) createKey(c *gin.Context) {
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	var request keyRequest
	if c.ShouldBindJSON(&request) != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "a key name is required")
		return
	}
	name := strings.TrimSpace(request.Name)
	if !validation.Required(name) || validation.RuneLength(name) > 100 || strings.ContainsRune(name, '\x00') {
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "a key name of at most 100 characters is required")
		return
	}
	key, err := apikey.Generate()
	if err != nil {
		h.internalError(c, "generate_api_key", "could not generate API key", err)
		return
	}
	clientID, err := randomUUID()
	if err != nil {
		h.internalError(c, "generate_api_key_id", "could not generate API key identifier", err)
		return
	}
	client, err := h.repository.CreateAPIClient(c.Request.Context(), repository.CreateAPIClientParams{ID: clientID, MerchantID: identity.MerchantID, Name: name, KeyPrefix: key.KeyPrefix, LookupID: key.LookupID, KeyHash: key.Hash})
	if err != nil {
		if errors.Is(err, repository.ErrKeyLimit) {
			httpx.Error(c, http.StatusConflict, "KEY_LIMIT_REACHED", "maximum active API keys reached")
			return
		}
		h.handleRepositoryError(c, err)
		return
	}
	httpx.Success(c, http.StatusCreated, gin.H{"id": client.ID, "name": client.Name, "apiKey": key.Full, "keyPrefix": client.KeyPrefix, "createdAt": client.CreatedAt})
	h.notifyAPIKeyCreated(c.Request.Context(), merchant, client.Name, client.KeyPrefix, client.CreatedAt)
}

// notifyAPIKeyCreated sends the security notice for a new key. The key itself
// never leaves the handler: only its display prefix is passed to the email.
// A delivery or Identity failure must not affect the already-created key.
func (h *merchantHandler) notifyAPIKeyCreated(ctx context.Context, merchant repository.Merchant, name, prefix string, createdAt time.Time) {
	if h.notifier == nil {
		return
	}
	to, owner := merchant.ContactEmail, merchant.BusinessName
	if h.identity != nil {
		if contact, err := h.identity.Contact(ctx, merchant.OwnerUserID); err == nil && contact.Email != "" {
			to = contact.Email
			if contact.FullName != "" {
				owner = contact.FullName
			}
		}
	}
	h.notifier.APIKeyCreated(ctx, to, owner, name, prefix, createdAt)
}

func randomUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

func (h *merchantHandler) revokeKey(c *gin.Context) {
	identity, merchant, ok := h.merchantIdentity(c)
	if !ok || !h.requireApproved(c, merchant) {
		return
	}
	keyID := strings.ToLower(strings.TrimSpace(c.Param("keyId")))
	if !validation.UUID(keyID) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "API key not found")
		return
	}
	if err := h.repository.RevokeAPIClient(c.Request.Context(), identity.MerchantID, keyID); err != nil {
		if errors.Is(err, repository.ErrNotFound) {
			httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "API key not found")
			return
		}
		h.handleRepositoryError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *merchantHandler) list(c *gin.Context) {
	query := c.Request.URL.Query()
	page, _ := strconv.Atoi(query.Get("page"))
	pageSize, _ := strconv.Atoi(query.Get("pageSize"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	desc := true
	if value := query.Get("sort"); strings.HasSuffix(value, ":asc") {
		desc = false
	}
	merchants, total, err := h.repository.ListMerchants(c.Request.Context(), repository.MerchantFilter{
		Status: strings.ToUpper(query.Get("status")), Search: query.Get("q"),
		Sort: strings.TrimSuffix(strings.TrimSuffix(query.Get("sort"), ":asc"), ":desc"),
		Desc: desc, Limit: pageSize, Offset: (page - 1) * pageSize,
	})
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	ids := make([]string, 0, len(merchants))
	for _, merchant := range merchants {
		ids = append(ids, merchant.ID)
	}
	pendingSync, err := h.repository.PendingIdentitySyncs(c.Request.Context(), ids)
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	items := make([]merchantDTO, 0, len(merchants))
	for _, merchant := range merchants {
		items = append(items, adminMerchantResponse(merchant, pendingSync))
	}
	httpx.SuccessList(c, http.StatusOK, items, httpx.NewPagination(page, pageSize, total))
}

func (h *merchantHandler) adminDetail(c *gin.Context) {
	merchantID := strings.ToLower(strings.TrimSpace(c.Param("merchantId")))
	if !validation.UUID(merchantID) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
		return
	}
	merchant, err := h.repository.MerchantByID(c.Request.Context(), merchantID)
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	pendingSync, err := h.repository.PendingIdentitySyncs(c.Request.Context(), []string{merchant.ID})
	if err != nil {
		h.handleRepositoryError(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, adminMerchantResponse(merchant, pendingSync))
}

func (h *merchantHandler) approve(c *gin.Context) {
	h.decide(c, "approve")
}

type reasonRequest struct {
	Reason *string `json:"reason"`
}

func (h *merchantHandler) reject(c *gin.Context) {
	h.decide(c, "reject")
}

func (h *merchantHandler) suspend(c *gin.Context) {
	h.decide(c, "suspend")
}

func (h *merchantHandler) decide(c *gin.Context, action string) {
	claims, ok := auth.ClaimsFrom(c)
	if !ok {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHENTICATED", "authentication required")
		return
	}
	merchantID := strings.ToLower(strings.TrimSpace(c.Param("merchantId")))
	if !validation.UUID(merchantID) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
		return
	}
	if h.onboarding == nil {
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "merchant lifecycle is not configured")
		return
	}
	var reason *string
	if action != "approve" {
		var request reasonRequest
		if c.Request.ContentLength != 0 {
			if c.ContentType() != "application/json" {
				httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be application/json")
				return
			}
			c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 8<<10)
			if err := c.ShouldBindJSON(&request); err != nil {
				httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "request body must be valid JSON")
				return
			}
		}
		reason = request.Reason
	}
	var (
		result onboarding.DecisionResult
		err    error
	)
	switch action {
	case "approve":
		result, err = h.onboarding.Approve(c.Request.Context(), merchantID, claims.UserID)
	case "reject":
		result, err = h.onboarding.Reject(c.Request.Context(), merchantID, claims.UserID, reason)
	default:
		result, err = h.onboarding.Suspend(c.Request.Context(), merchantID, claims.UserID, reason)
	}
	if err != nil {
		h.onboardingError(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"id": result.Merchant.ID, "status": result.Merchant.Status, "identitySync": result.IdentitySync,
		"alreadyDone": result.AlreadyDone, "approvedAt": result.Merchant.ApprovedAt,
		"rejectionReason": result.Merchant.RejectionReason, "suspensionReason": result.Merchant.SuspensionReason,
	})
}

func (h *merchantHandler) internalApproval(c *gin.Context) {
	if !requireInternalToken(c, h.internalTokenValue) {
		return
	}
	if !requestid.Valid(c.GetHeader(requestid.Header)) {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "a valid X-Request-Id header is required")
		return
	}
	ownerUserID := strings.ToLower(strings.TrimSpace(c.Param("userId")))
	if !validation.UUID(ownerUserID) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
		return
	}
	merchant, err := h.repository.MerchantByOwnerUserID(c.Request.Context(), ownerUserID)
	if errors.Is(err, repository.ErrNotFound) {
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
		return
	}
	if err != nil {
		h.internalError(c, "merchant_approval_lookup", "merchant status could not be read", err)
		return
	}
	body := gin.H{"merchantId": merchant.ID, "status": merchant.Status}
	// Identity shows the owner why a login was refused, but only after the
	// correct password. The reason is the administrator's own text for that
	// owner, the same text the decision email carries.
	switch {
	case merchant.Status == repository.MerchantStatusRejected && merchant.RejectionReason != nil:
		body["reason"] = *merchant.RejectionReason
	case merchant.Status == repository.MerchantStatusSuspended && merchant.SuspensionReason != nil:
		body["reason"] = *merchant.SuspensionReason
	}
	httpx.Success(c, http.StatusOK, body)
}

func (h *merchantHandler) onboardingError(c *gin.Context, err error) {
	var validationErr *onboarding.ValidationError
	switch {
	case errors.As(err, &validationErr):
		httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "One or more fields are invalid.", httpx.ErrorItems(validationErr.Issues))
	case errors.Is(err, onboarding.ErrEmailInUse):
		httpx.Error(c, http.StatusConflict, "EMAIL_ALREADY_EXISTS", "an account with this email already exists")
	case errors.Is(err, onboarding.ErrDuplicateApplication):
		httpx.Error(c, http.StatusConflict, "DUPLICATE_MERCHANT_APPLICATION", "a merchant application already exists for this email")
	case errors.Is(err, onboarding.ErrDuplicateBusiness):
		httpx.Error(c, http.StatusConflict, "DUPLICATE_BUSINESS_REGISTRATION", "this business registration number is already registered")
	case errors.Is(err, onboarding.ErrActivationFailed):
		httpx.Error(c, http.StatusServiceUnavailable, "MERCHANT_ACTIVATION_FAILED", "merchant activation failed; retry safely")
	case errors.Is(err, onboarding.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "a required service is unavailable, try again later")
	case errors.Is(err, repository.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
	case errors.Is(err, repository.ErrInvalidState):
		httpx.Error(c, http.StatusConflict, "INVALID_MERCHANT_STATE", "merchant state transition is not allowed")
	default:
		h.internalError(c, "merchant_onboarding", "merchant operation failed", err)
	}
}

func (h *merchantHandler) handleRepositoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "merchant not found")
	case errors.Is(err, repository.ErrInvalidState):
		httpx.Error(c, http.StatusConflict, "INVALID_MERCHANT_STATE", "merchant state transition is not allowed")
	default:
		h.internalError(c, "merchant_repository", "merchant operation failed", err)
	}
}

func (h *merchantHandler) internalError(c *gin.Context, operation, message string, err error) {
	if h.logger != nil {
		h.logger.ErrorContext(c.Request.Context(), "merchant request failed",
			"request_id", requestid.FromGin(c), "path", c.FullPath(), "operation", operation, "error_class", errorClass(err))
	}
	httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", message)
}

// errorClass returns a stable classification for the log. The raw error text is
// never logged: repository and driver errors can contain contact details, and
// webhook and signing errors can carry URLs or secrets.
func errorClass(err error) string {
	switch {
	case err == nil:
		return "none"
	case errors.Is(err, context.Canceled):
		return "canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "deadline_exceeded"
	case errors.Is(err, repository.ErrNotFound):
		return "not_found"
	case errors.Is(err, repository.ErrInvalidState):
		return "invalid_state"
	case errors.Is(err, repository.ErrWebhookNotConfigured):
		return "webhook_not_configured"
	default:
		return "internal"
	}
}

type merchantDTO struct {
	ID                 string                    `json:"id"`
	OwnerUserID        string                    `json:"ownerUserId"`
	BusinessName       string                    `json:"businessName"`
	RegistrationNumber *string                   `json:"registrationNumber,omitempty"`
	BusinessAddress    *string                   `json:"businessAddress,omitempty"`
	ContactEmail       string                    `json:"contactEmail"`
	ContactPhone       string                    `json:"contactPhone"`
	PickupAddress      string                    `json:"pickupAddress"`
	PickupZoneID       string                    `json:"pickupZoneId"`
	Status             repository.MerchantStatus `json:"status"`
	ApprovedBy         *string                   `json:"approvedBy,omitempty"`
	ApprovedAt         any                       `json:"approvedAt,omitempty"`
	RejectionReason    *string                   `json:"rejectionReason,omitempty"`
	SuspensionReason   *string                   `json:"suspensionReason,omitempty"`
	// IdentitySync is "PENDING" in the administrator's views when a REJECTED or
	// SUSPENDED status has not yet reached Identity and a retry is needed. It is
	// left out otherwise.
	IdentitySync string `json:"identitySync,omitempty"`
	CreatedAt    any    `json:"createdAt"`
	UpdatedAt    any    `json:"updatedAt"`
}

// adminMerchantResponse is merchantResponse plus whether the merchant's status
// still needs to reach Identity.
func adminMerchantResponse(merchant repository.Merchant, pendingSync map[string]bool) merchantDTO {
	dto := merchantResponse(merchant)
	if pendingSync[merchant.ID] {
		dto.IdentitySync = "PENDING"
	}
	return dto
}

func merchantResponse(merchant repository.Merchant) merchantDTO {
	return merchantDTO{ID: merchant.ID, OwnerUserID: merchant.OwnerUserID, BusinessName: merchant.BusinessName, RegistrationNumber: merchant.BusinessRegNo, BusinessAddress: merchant.BusinessAddress, ContactEmail: merchant.ContactEmail, ContactPhone: merchant.ContactPhone, PickupAddress: merchant.PickupAddress, PickupZoneID: merchant.PickupZoneID, Status: merchant.Status, ApprovedBy: merchant.ApprovedBy, ApprovedAt: merchant.ApprovedAt, RejectionReason: merchant.RejectionReason, SuspensionReason: merchant.SuspensionReason, CreatedAt: merchant.CreatedAt, UpdatedAt: merchant.UpdatedAt}
}

// merchantIdentity is retained for handler-level tests and callers that only
// need the authenticated identity. New routes resolve the merchant row through
// merchantHandler.merchantIdentity.
func merchantIdentity(c *gin.Context) (Identity, bool) {
	identity, ok := identityFrom(c)
	if !ok || identity.MerchantID == "" || identity.Role != string(auth.RoleMerchant) {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "merchant authentication required")
		return Identity{}, false
	}
	return identity, true
}

func requireAdminIdentity(c *gin.Context) (Identity, bool) {
	identity, ok := identityFrom(c)
	if !ok || identity.Role != string(auth.RoleAdmin) {
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "administrator access required")
		return Identity{}, false
	}
	return identity, true
}

func validateProfileRequest(request profileRequest) []validation.Issue {
	issues := validation.New()
	issues.RequiredExact("contactPhone", request.ContactPhone)
	issues.RequiredExact("pickupAddress", request.PickupAddress)
	issues.RequiredExact("pickupZoneId", request.PickupZoneID)
	return issues.Issues()
}
