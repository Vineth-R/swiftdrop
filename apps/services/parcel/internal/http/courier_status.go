package servicehttp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// CourierStatusStore is separate from CourierStore to preserve the read interface.
type CourierStatusStore interface {
	UpdateStatus(context.Context, courier.StatusInput) (courier.StatusResult, error)
}

func registerCourierStatus(group *gin.RouterGroup, reads CourierStore, logger *slog.Logger) {
	store, ok := reads.(CourierStatusStore)
	if !ok {
		store = courier.NewRepository(nil)
	}
	group.POST("/:parcelId/status", func(c *gin.Context) {
		claims, ok := courierClaims(c)
		if !ok {
			return
		}
		parcelID := c.Param("parcelId")
		if !validation.UUID(parcelID) {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "parcelId must be a valid UUID")
			return
		}
		var body *struct {
			Status        statemachine.ParcelStatus `json:"status"`
			RecipientName string                    `json:"recipientName"`
			FailureReason string                    `json:"failureReason"`
			Note          string                    `json:"note"`
		}
		// Even 500 four-byte characters fit comfortably. Reject extra JSON values
		// and caller-selected courier IDs instead of silently ignoring them.
		c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, 16*1024)
		decoder := json.NewDecoder(c.Request.Body)
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&body); err != nil || body == nil {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "body must be a valid status update object")
			return
		}
		if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "body must contain one JSON object")
			return
		}
		result, err := store.UpdateStatus(c.Request.Context(), courier.StatusInput{
			ParcelID: parcelID, CourierID: claims.UserID, Status: body.Status,
			RecipientName: body.RecipientName, FailureReason: body.FailureReason, Note: body.Note,
		})
		var domain *statemachine.DomainError
		switch {
		case err == nil:
			httpx.Success(c, http.StatusOK, result)
		case errors.Is(err, courier.ErrNotFound):
			httpx.Error(c, http.StatusNotFound, "PARCEL_NOT_FOUND", "parcel not found")
		case errors.As(err, &domain):
			httpx.Error(c, domain.HTTPStatus, domain.Code, domain.Message)
		case errors.Is(err, courier.ErrUnavailable):
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "courier status updates are unavailable")
		default:
			logger.ErrorContext(c.Request.Context(), "courier status update failed", "request_id", requestid.FromGin(c))
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "delivery status could not be saved")
		}
	})
}
