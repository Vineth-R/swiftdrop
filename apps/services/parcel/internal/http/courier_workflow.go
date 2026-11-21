package servicehttp

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

// CourierWorkflowStore is the proof-backed transition dependency. It is
// separate from the generic status store so delivery cannot be confirmed
// without the transactional proof path.
type CourierWorkflowStore interface {
	ConfirmPickup(context.Context, courier.PickupInput) (courier.WorkflowResult, error)
	ConfirmDelivery(context.Context, courier.DeliveryInput) (courier.WorkflowResult, error)
}

type unavailableWorkflowStore struct{}

func (unavailableWorkflowStore) ConfirmPickup(context.Context, courier.PickupInput) (courier.WorkflowResult, error) {
	return courier.WorkflowResult{}, courier.ErrUnavailable
}

func (unavailableWorkflowStore) ConfirmDelivery(context.Context, courier.DeliveryInput) (courier.WorkflowResult, error) {
	return courier.WorkflowResult{}, courier.ErrUnavailable
}

func registerCourierWorkflow(group *gin.RouterGroup, store CourierWorkflowStore, logger *slog.Logger, maxPhotoBytes int) {
	if store == nil {
		store = unavailableWorkflowStore{}
	}
	if maxPhotoBytes <= 0 || maxPhotoBytes > proofmedia.MaxPhotoBytes {
		maxPhotoBytes = proofmedia.MaxPhotoBytes
	}
	limiter := httpx.NewRateLimiter(proofUploadsPerMinute, time.Minute)
	rateLimit := limiter.MiddlewareKeyed(func(c *gin.Context) string {
		claims, ok := auth.ClaimsFrom(c)
		if !ok || claims.UserID == "" {
			return ""
		}
		return "courier:" + strings.ToLower(claims.UserID)
	})
	confirm := func(handle func(c *gin.Context, claims auth.Claims, input courier.ProofInput)) gin.HandlerFunc {
		return func(c *gin.Context) {
			_ = http.NewResponseController(c.Writer).SetReadDeadline(time.Now().Add(proofUploadReadTimeout))
			claims, ok := courierClaims(c)
			if !ok {
				return
			}
			parcelID := c.Param("parcelId")
			if !validation.UUID(parcelID) {
				httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "parcelId must be a valid UUID")
				return
			}
			input, err := readProofForm(c, maxPhotoBytes)
			if err != nil {
				writeProofFormError(c, err)
				return
			}
			input.ParcelID = parcelID
			input.CourierID = claims.UserID
			handle(c, claims, input)
		}
	}

	pickup := confirm(func(c *gin.Context, claims auth.Claims, input courier.ProofInput) {
		result, err := store.ConfirmPickup(c.Request.Context(), courier.PickupInput{
			ParcelID: input.ParcelID, CourierID: claims.UserID,
			Note: input.Note, Photo: input.Photo,
		})
		if err != nil {
			writeWorkflowError(c, logger, "pickup", err)
			return
		}
		httpx.Success(c, http.StatusOK, result)
	})
	delivery := confirm(func(c *gin.Context, claims auth.Claims, input courier.ProofInput) {
		result, err := store.ConfirmDelivery(c.Request.Context(), courier.DeliveryInput{
			ParcelID: input.ParcelID, CourierID: claims.UserID,
			RecipientName: input.RecipientName, Note: input.Note, Photo: input.Photo,
		})
		if err != nil {
			writeWorkflowError(c, logger, "delivery", err)
			return
		}
		httpx.Success(c, http.StatusOK, result)
	})

	// Register both forms so a trailing slash never falls back to Gin's
	// redirect, whose Location would omit the gateway's external /api/v1 prefix.
	group.POST("/:parcelId/pickup", rateLimit, pickup)
	group.POST("/:parcelId/pickup/", rateLimit, pickup)
	group.POST("/:parcelId/delivery", rateLimit, delivery)
	group.POST("/:parcelId/delivery/", rateLimit, delivery)
}

func writeWorkflowError(c *gin.Context, logger *slog.Logger, operation string, err error) {
	var domain *statemachine.DomainError
	switch {
	case errors.Is(err, courier.ErrNotFound):
		httpx.Error(c, http.StatusNotFound, "PARCEL_NOT_FOUND", "parcel not found")
	case errors.Is(err, courier.ErrForbidden):
		httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "parcel is not assigned to this courier")
	case errors.As(err, &domain):
		httpx.Error(c, domain.HTTPStatus, domain.Code, domain.Message)
	case errors.Is(err, courier.ErrPhotoRequired):
		httpx.Error(c, http.StatusUnprocessableEntity, "PROOF_REQUIRED", "a proof photo is required")
	case errors.Is(err, proofmedia.ErrPhotoTooLarge):
		httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "photo exceeds the 5 MiB limit")
	case errors.Is(err, courier.ErrProofUnsupported):
		httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "photo must be a valid JPEG or PNG")
	case errors.Is(err, courier.ErrProofEmpty), errors.Is(err, courier.ErrProofValidation):
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "delivery proof fields are invalid")
	case errors.Is(err, courier.ErrProofLimitReached):
		httpx.Error(c, http.StatusConflict, "PROOF_LIMIT_REACHED", "delivery proof limit reached for this parcel")
	case errors.Is(err, courier.ErrUnavailable):
		httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "courier workflow is unavailable")
	default:
		logger.ErrorContext(c.Request.Context(), "courier workflow failed",
			"request_id", requestid.FromGin(c), "operation", operation)
		httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "delivery could not be saved")
	}
}
