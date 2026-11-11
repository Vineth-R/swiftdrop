package servicehttp

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

type ProofPhotoStore interface {
	Photo(context.Context, string, courier.PhotoViewer) (courier.ProofPhotoDownload, error)
	Open(context.Context, string, courier.PhotoViewer) (io.ReadCloser, string, int64, error)
}

// unavailableProofPhotoStore keeps the route registered while failing closed
// when object storage is not configured.
type unavailableProofPhotoStore struct{}

func (unavailableProofPhotoStore) Photo(context.Context, string, courier.PhotoViewer) (courier.ProofPhotoDownload, error) {
	return courier.ProofPhotoDownload{}, courier.ErrUnavailable
}

func (unavailableProofPhotoStore) Open(context.Context, string, courier.PhotoViewer) (io.ReadCloser, string, int64, error) {
	return nil, "", 0, courier.ErrUnavailable
}

func (h *parcelHandlers) proofPhoto(c *gin.Context) {
	store := h.photoStore
	if store == nil {
		store = unavailableProofPhotoStore{}
	}
	parcelID, viewer, ok := proofPhotoRequest(c)
	if !ok {
		return
	}
	result, err := store.Photo(c.Request.Context(), parcelID, viewer)
	if err != nil {
		h.writeProofPhotoError(c, err)
		return
	}
	httpx.Success(c, http.StatusOK, gin.H{
		"url":       result.URL,
		"expiresAt": result.ExpiresAt,
		"mimeType":  result.MIMEType,
		"stream":    result.Stream,
	})
}

func (h *parcelHandlers) proofPhotoContent(c *gin.Context) {
	store := h.photoStore
	if store == nil {
		store = unavailableProofPhotoStore{}
	}
	parcelID, viewer, ok := proofPhotoRequest(c)
	if !ok {
		return
	}
	body, contentType, size, err := store.Open(c.Request.Context(), parcelID, viewer)
	if err != nil {
		h.writeProofPhotoError(c, err)
		return
	}
	defer func() { _ = body.Close() }()

	c.Header("Content-Type", contentType)
	if size > 0 {
		c.Header("Content-Length", strconv.FormatInt(size, 10))
	}
	c.Header("Content-Disposition", `inline; filename="delivery-proof"`)
	c.Header("X-Content-Type-Options", "nosniff")
	c.Header("Cache-Control", "no-store")
	c.Status(http.StatusOK)
	if _, err := io.Copy(c.Writer, body); err != nil {
		h.logger.ErrorContext(c.Request.Context(), "delivery proof stream failed",
			"request_id", requestid.FromGin(c), "error", err)
	}
}

func proofPhotoRequest(c *gin.Context) (string, courier.PhotoViewer, bool) {
	parcelID := c.Param("parcelId")
	if !validation.UUID(parcelID) {
		httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "parcelId must be a valid UUID")
		return "", courier.PhotoViewer{}, false
	}
	claims, ok := auth.ClaimsFrom(c)
	if !ok || claims.UserID == "" {
		httpx.Error(c, http.StatusUnauthorized, "UNAUTHORIZED", "authentication is required")
		return "", courier.PhotoViewer{}, false
	}
	viewer := courier.PhotoViewer{UserID: claims.UserID}
	switch claims.Role {
	case auth.RoleAdmin, auth.RoleDispatcher:
		viewer.Staff = true
	case auth.RoleCourier:
		viewer.Courier = true
	}
	return parcelID, viewer, true
}

func (h *parcelHandlers) writeProofPhotoError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, courier.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "NOT_FOUND", "parcel not found")
	case errors.Is(err, courier.ErrForbidden):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "proof photo is not available to this user")
	case errors.Is(err, courier.ErrProofPhotoNotFound):
		httpx.Error(c, http.StatusNotFound, "PROOF_PHOTO_NOT_FOUND", "no delivery proof photo is available")
	case errors.Is(err, courier.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "delivery proof storage is unavailable")
	default:
		h.logger.ErrorContext(c.Request.Context(), "delivery proof photo failed",
			"request_id", requestid.FromGin(c), "error", err)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "delivery proof photo could not be read")
	}
}
