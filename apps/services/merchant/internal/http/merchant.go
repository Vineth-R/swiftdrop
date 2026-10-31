package servicehttp

import (
	"crypto/rand"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/apikey"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
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
	repository *repository.Repository
}

func registerMerchantRoutes(router *gin.Engine, repo *repository.Repository) {
	h := &merchantHandler{repository: repo}
	merchants := router.Group("/merchants")
	merchants.GET("/me", h.profile)
	merchants.PATCH("/me", h.updateProfile)
	merchants.GET("/me/keys", h.listKeys)
	merchants.POST("/me/keys", h.createKey)
	merchants.DELETE("/me/keys/:keyId", h.revokeKey)
	admin := router.Group("/admin/merchants")
	admin.GET("/:merchantId", h.adminDetail)
	admin.POST("/:merchantId/approve", h.approve)
	admin.POST("/:merchantId/reject", h.reject)
	admin.POST("/:merchantId/suspend", h.suspend)
}

func (h *merchantHandler) listKeys(c *gin.Context) {
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	keys, err := h.repository.APIClientsByMerchant(c.Request.Context(), identity.MerchantID)
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	data := make([]gin.H, 0, len(keys))
	for _, key := range keys {
		data = append(data, gin.H{"id": key.ID, "name": key.Name, "keyPrefix": key.KeyPrefix, "status": key.Status, "lastUsedAt": key.LastUsedAt, "createdAt": key.CreatedAt, "revokedAt": key.RevokedAt})
	}
	respond(c, http.StatusOK, data)
}

type keyRequest struct {
	Name string `json:"name" binding:"required"`
}

func (h *merchantHandler) createKey(c *gin.Context) {
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	merchant, err := h.repository.MerchantByID(c.Request.Context(), identity.MerchantID)
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	if merchant.Status != repository.MerchantStatusApproved {
		httpx.Error(c, http.StatusForbidden, "merchant_not_approved", "merchant approval is required")
		return
	}
	var request keyRequest
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Name) == "" {
		httpx.Error(c, http.StatusUnprocessableEntity, "validation_error", "a key name is required")
		return
	}
	key, err := apikey.Generate()
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal_error", "could not generate API key")
		return
	}
	clientID, err := randomUUID()
	if err != nil {
		httpx.Error(c, http.StatusInternalServerError, "internal_error", "could not generate API key identifier")
		return
	}
	if err := h.repository.CreateAPIClient(c.Request.Context(), repository.CreateAPIClientParams{ID: clientID, MerchantID: identity.MerchantID, Name: strings.TrimSpace(request.Name), KeyPrefix: key.KeyPrefix, LookupID: key.LookupID, KeyHash: key.Hash}); err != nil {
		if errors.Is(err, repository.ErrKeyLimit) {
			httpx.Error(c, http.StatusConflict, "key_limit_reached", "maximum active API keys reached")
			return
		}
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusCreated, gin.H{"id": clientID, "name": strings.TrimSpace(request.Name), "apiKey": key.Full, "keyPrefix": key.KeyPrefix})
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
	identity, ok := merchantIdentity(c)
	if !ok {
		return
	}
	if err := h.repository.RevokeAPIClient(c.Request.Context(), identity.MerchantID, c.Param("keyId")); err != nil {
		handleRepositoryError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func merchantIdentity(c *gin.Context) (Identity, bool) {
	identity, ok := identityFrom(c)
	if !ok || identity.MerchantID == "" || identity.Role != "MERCHANT" {
		httpx.Error(c, http.StatusUnauthorized, "unauthorized", "merchant authentication required")
		return Identity{}, false
	}
	return identity, true
}

func (h *merchantHandler) profile(c *gin.Context) {
	identity, ok := identityFrom(c)
	if !ok || identity.MerchantID == "" || identity.Role != "MERCHANT" {
		httpx.Error(c, http.StatusUnauthorized, "unauthorized", "merchant authentication required")
		return
	}
	merchant, err := h.repository.MerchantByID(c.Request.Context(), identity.MerchantID)
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

type profileRequest struct {
	ContactEmail  string `json:"contactEmail" binding:"required,email"`
	ContactPhone  string `json:"contactPhone" binding:"required"`
	PickupAddress string `json:"pickupAddress" binding:"required"`
	PickupZoneID  string `json:"pickupZoneId" binding:"required"`
}

func (h *merchantHandler) updateProfile(c *gin.Context) {
	identity, ok := identityFrom(c)
	if !ok || identity.MerchantID == "" || identity.Role != "MERCHANT" {
		httpx.Error(c, http.StatusUnauthorized, "unauthorized", "merchant authentication required")
		return
	}
	var request profileRequest
	if err := c.ShouldBindJSON(&request); err != nil {
		httpx.Error(c, http.StatusUnprocessableEntity, "validation_error", "invalid merchant profile")
		return
	}
	merchant, err := h.repository.UpdateProfile(c.Request.Context(), identity.MerchantID, repository.ProfileUpdate{
		ContactEmail: strings.TrimSpace(request.ContactEmail), ContactPhone: strings.TrimSpace(request.ContactPhone),
		PickupAddress: strings.TrimSpace(request.PickupAddress), PickupZoneID: strings.TrimSpace(request.PickupZoneID),
	})
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

func (h *merchantHandler) adminDetail(c *gin.Context) {
	if !requireAdmin(c) {
		return
	}
	merchant, err := h.repository.MerchantByID(c.Request.Context(), c.Param("merchantId"))
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

func (h *merchantHandler) approve(c *gin.Context) {
	identity, ok := requireAdminIdentity(c)
	if !ok {
		return
	}
	merchant, err := h.repository.ApproveMerchant(c.Request.Context(), c.Param("merchantId"), identity.UserID)
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

type reasonRequest struct {
	Reason string `json:"reason" binding:"required"`
}

func (h *merchantHandler) reject(c *gin.Context) {
	identity, ok := requireAdminIdentity(c)
	if !ok {
		return
	}
	var request reasonRequest
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Reason) == "" {
		httpx.Error(c, http.StatusUnprocessableEntity, "validation_error", "a rejection reason is required")
		return
	}
	merchant, err := h.repository.RejectMerchant(c.Request.Context(), c.Param("merchantId"), identity.UserID, strings.TrimSpace(request.Reason))
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

func (h *merchantHandler) suspend(c *gin.Context) {
	identity, ok := requireAdminIdentity(c)
	if !ok {
		return
	}
	var request reasonRequest
	if c.ShouldBindJSON(&request) != nil || strings.TrimSpace(request.Reason) == "" {
		httpx.Error(c, http.StatusUnprocessableEntity, "validation_error", "a suspension reason is required")
		return
	}
	merchant, err := h.repository.SuspendMerchant(c.Request.Context(), c.Param("merchantId"), identity.UserID, strings.TrimSpace(request.Reason))
	if err != nil {
		handleRepositoryError(c, err)
		return
	}
	respond(c, http.StatusOK, merchantResponse(merchant))
}

func requireAdmin(c *gin.Context) bool { _, ok := requireAdminIdentity(c); return ok }
func requireAdminIdentity(c *gin.Context) (Identity, bool) {
	identity, ok := identityFrom(c)
	if !ok || identity.Role != "ADMIN" {
		httpx.Error(c, http.StatusForbidden, "forbidden", "administrator access required")
		return Identity{}, false
	}
	return identity, true
}

func handleRepositoryError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, repository.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "not_found", "merchant not found")
	case errors.Is(err, repository.ErrInvalidState):
		httpx.Error(c, http.StatusConflict, "invalid_merchant_state", "merchant state transition is not allowed")
	default:
		httpx.Error(c, http.StatusInternalServerError, "internal_error", "merchant operation failed")
	}
}

type merchantDTO struct {
	ID               string                    `json:"id"`
	OwnerUserID      string                    `json:"ownerUserId"`
	BusinessName     string                    `json:"businessName"`
	BusinessRegNo    *string                   `json:"registrationNumber,omitempty"`
	ContactEmail     string                    `json:"contactEmail"`
	ContactPhone     string                    `json:"contactPhone"`
	PickupAddress    string                    `json:"pickupAddress"`
	PickupZoneID     string                    `json:"pickupZoneId"`
	Status           repository.MerchantStatus `json:"status"`
	ApprovedBy       *string                   `json:"approvedBy,omitempty"`
	ApprovedAt       any                       `json:"approvedAt,omitempty"`
	RejectionReason  *string                   `json:"rejectionReason,omitempty"`
	SuspensionReason *string                   `json:"suspensionReason,omitempty"`
	CreatedAt        any                       `json:"createdAt"`
	UpdatedAt        any                       `json:"updatedAt"`
}

func merchantResponse(merchant repository.Merchant) merchantDTO {
	return merchantDTO{ID: merchant.ID, OwnerUserID: merchant.OwnerUserID, BusinessName: merchant.BusinessName, BusinessRegNo: merchant.BusinessRegNo, ContactEmail: merchant.ContactEmail, ContactPhone: merchant.ContactPhone, PickupAddress: merchant.PickupAddress, PickupZoneID: merchant.PickupZoneID, Status: merchant.Status, ApprovedBy: merchant.ApprovedBy, ApprovedAt: merchant.ApprovedAt, RejectionReason: merchant.RejectionReason, SuspensionReason: merchant.SuspensionReason, CreatedAt: merchant.CreatedAt, UpdatedAt: merchant.UpdatedAt}
}
func respond(c *gin.Context, status int, data any) {
	c.JSON(status, gin.H{"data": data, "meta": gin.H{"requestId": requestid.FromGin(c)}})
}
