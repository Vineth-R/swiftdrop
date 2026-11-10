package servicehttp

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"strings"
	"unicode/utf8"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/packages/go-common/requestid"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const proofRequestBytes = proofmedia.MaxPhotoBytes + 64*1024

type CourierProofStore interface {
	CreateProof(context.Context, courier.ProofInput) (courier.ProofResult, error)
}

func registerCourierProof(group *gin.RouterGroup, store CourierProofStore, logger *slog.Logger) {
	if store == nil {
		store = courier.NewProofService(nil, nil)
	}
	upload := func(c *gin.Context) {
		claims, ok := courierClaims(c)
		if !ok {
			return
		}
		parcelID := c.Param("parcelId")
		if !validation.UUID(parcelID) {
			httpx.Error(c, http.StatusBadRequest, "BAD_REQUEST", "parcelId must be a valid UUID")
			return
		}
		input, err := readProofForm(c)
		if err != nil {
			writeProofFormError(c, err)
			return
		}
		input.ParcelID = parcelID
		input.CourierID = claims.UserID
		result, err := store.CreateProof(c.Request.Context(), input)
		switch {
		case err == nil:
			httpx.Success(c, http.StatusCreated, result)
		case errors.Is(err, courier.ErrNotFound):
			httpx.Error(c, http.StatusNotFound, "PARCEL_NOT_FOUND", "parcel not found")
		case errors.Is(err, courier.ErrForbidden):
			httpx.Error(c, http.StatusForbidden, "FORBIDDEN", "parcel is not assigned to this courier")
		case errors.Is(err, proofmedia.ErrPhotoTooLarge):
			httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "photo exceeds the 5 MiB limit")
		case errors.Is(err, courier.ErrProofUnsupported):
			httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "photo must be a valid JPEG or PNG")
		case errors.Is(err, courier.ErrProofEmpty), errors.Is(err, courier.ErrProofValidation):
			httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "delivery proof fields are invalid")
		case errors.Is(err, courier.ErrUnavailable):
			httpx.Error(c, http.StatusServiceUnavailable, "SERVICE_UNAVAILABLE", "delivery proof uploads are unavailable")
		default:
			logger.ErrorContext(c.Request.Context(), "courier proof upload failed", "request_id", requestid.FromGin(c))
			httpx.Error(c, http.StatusInternalServerError, "INTERNAL_ERROR", "delivery proof could not be saved")
		}
	}
	group.POST("/:parcelId/proof", upload)
	group.POST("/:parcelId/proof/", upload)
}

var (
	errProofBadMultipart = errors.New("invalid delivery proof multipart form")
	errProofTooLarge     = errors.New("delivery proof request is too large")
	errProofMedia        = errors.New("delivery proof media type is unsupported")
)

func readProofForm(c *gin.Context) (courier.ProofInput, error) {
	mediaType, _, err := mime.ParseMediaType(c.GetHeader("Content-Type"))
	if err != nil || mediaType != "multipart/form-data" {
		return courier.ProofInput{}, errProofMedia
	}
	c.Request.Body = http.MaxBytesReader(c.Writer, c.Request.Body, proofRequestBytes)
	reader, err := c.Request.MultipartReader()
	if err != nil {
		return courier.ProofInput{}, errProofBadMultipart
	}
	var input courier.ProofInput
	seen := map[string]bool{}
	for {
		part, nextErr := reader.NextPart()
		if errors.Is(nextErr, io.EOF) {
			break
		}
		if nextErr != nil {
			return courier.ProofInput{}, classifyProofReadError(nextErr)
		}
		name := part.FormName()
		if seen[name] || (name != "photo" && name != "recipientName" && name != "note") {
			_ = part.Close()
			return courier.ProofInput{}, errProofBadMultipart
		}
		seen[name] = true
		switch name {
		case "photo":
			if strings.TrimSpace(part.FileName()) == "" {
				_ = part.Close()
				return courier.ProofInput{}, errProofBadMultipart
			}
			data, readErr := io.ReadAll(io.LimitReader(part, proofmedia.MaxPhotoBytes+1))
			_ = part.Close()
			if readErr != nil {
				return courier.ProofInput{}, classifyProofReadError(readErr)
			}
			if len(data) > proofmedia.MaxPhotoBytes {
				return courier.ProofInput{}, errProofTooLarge
			}
			input.Photo = &courier.ProofPhoto{Data: data, MIMEType: part.Header.Get("Content-Type")}
		case "recipientName", "note":
			if part.FileName() != "" {
				_ = part.Close()
				return courier.ProofInput{}, errProofBadMultipart
			}
			data, readErr := io.ReadAll(io.LimitReader(part, 4097))
			_ = part.Close()
			if readErr != nil {
				return courier.ProofInput{}, classifyProofReadError(readErr)
			}
			if len(data) > 4096 || !utf8Valid(data) {
				return courier.ProofInput{}, errProofBadMultipart
			}
			if name == "recipientName" {
				input.RecipientName = string(data)
			} else {
				input.Note = string(data)
			}
		}
	}
	return input, nil
}

func utf8Valid(data []byte) bool {
	return utf8.Valid(data)
}

func classifyProofReadError(err error) error {
	var maxBytes *http.MaxBytesError
	if errors.As(err, &maxBytes) {
		return errProofTooLarge
	}
	return fmt.Errorf("%w: %v", errProofBadMultipart, err)
}

func writeProofFormError(c *gin.Context, err error) {
	switch {
	case errors.Is(err, errProofTooLarge):
		httpx.Error(c, http.StatusRequestEntityTooLarge, "PAYLOAD_TOO_LARGE", "photo exceeds the 5 MiB limit")
	case errors.Is(err, errProofMedia):
		httpx.Error(c, http.StatusUnsupportedMediaType, "UNSUPPORTED_MEDIA_TYPE", "content type must be multipart/form-data")
	default:
		httpx.Error(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR", "delivery proof form is invalid")
	}
}
