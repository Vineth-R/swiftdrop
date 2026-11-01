// Package proof validates delivery-proof inputs owned by the parcel service.
package proof

import (
	"bytes"
	"errors"
	"fmt"
	"image/jpeg"
	"image/png"
	"mime"
	"net/http"
)

// MaxPhotoBytes matches parcel.delivery_proofs.photo_size_bytes (5 MiB, inclusive).
const MaxPhotoBytes = 5 << 20

// These inclusive limits are implementation safeguards, not API v1.1 requirements.
// The encoded byte limit alone cannot bound decoded memory. Inspect dimensions
// before decoding; cap each side at 8192 and total pixels at 16 megapixels to
// bound per-image allocations (up to 128 MB of 16-bit RGBA pixels, plus overhead).
const (
	MaxPhotoDimension = 8192
	MaxPhotoPixels    = 16_000_000
)

var (
	ErrEmptyPhoto            = errors.New("photo is empty")
	ErrPhotoTooLarge         = errors.New("photo exceeds the byte limit")
	ErrUnsupportedPhotoType  = errors.New("photo must be JPEG or PNG")
	ErrPhotoMIMETypeMismatch = errors.New("declared photo MIME type does not match content")
	ErrInvalidPhoto          = errors.New("photo is malformed or truncated")
	ErrPhotoDimensions       = errors.New("photo exceeds the dimension or pixel limit")
)

// PhotoMetadata contains verified values suitable for delivery-proof metadata.
type PhotoMetadata struct {
	MIMEType  string
	SizeBytes int
}

// ValidatePhoto checks a supplied photo without modifying data. The declared MIME
// type must be image/jpeg or image/png; valid MIME parameters are accepted, but
// only the canonical media type is returned. Missing and generic MIME types fail.
// The Go decoders validate image structure after signature and dimension checks.
// Callers must bound upload reads before allocating data, and persist the same
// bytes they validated. An omitted optional photo should not call this function.
func ValidatePhoto(data []byte, declaredMIME string) (PhotoMetadata, error) {
	if len(data) == 0 {
		return PhotoMetadata{}, ErrEmptyPhoto
	}
	if len(data) > MaxPhotoBytes {
		return PhotoMetadata{}, ErrPhotoTooLarge
	}

	declared, _, err := mime.ParseMediaType(declaredMIME)
	if err != nil {
		return PhotoMetadata{}, fmt.Errorf("%w: invalid declared MIME type", ErrUnsupportedPhotoType)
	}
	if declared != "image/jpeg" && declared != "image/png" {
		return PhotoMetadata{}, ErrUnsupportedPhotoType
	}

	actual := http.DetectContentType(data)
	if actual != "image/jpeg" && actual != "image/png" {
		return PhotoMetadata{}, ErrUnsupportedPhotoType
	}
	if actual != declared {
		return PhotoMetadata{}, ErrPhotoMIMETypeMismatch
	}

	// Use explicit decoders so other packages cannot expand the accepted formats
	// by registering an additional image decoder through init().
	decodeConfig := jpeg.DecodeConfig
	decode := jpeg.Decode
	if actual == "image/png" {
		decodeConfig = png.DecodeConfig
		decode = png.Decode
	}

	config, err := decodeConfig(bytes.NewReader(data))
	if err != nil {
		return PhotoMetadata{}, fmt.Errorf("%w: decode header: %w", ErrInvalidPhoto, err)
	}
	if config.Width <= 0 || config.Height <= 0 {
		return PhotoMetadata{}, ErrInvalidPhoto
	}
	exceedsDimension := config.Width > MaxPhotoDimension || config.Height > MaxPhotoDimension
	// Division avoids overflowing an int when inspecting untrusted dimensions.
	exceedsPixels := config.Width > MaxPhotoPixels/config.Height
	if exceedsDimension || exceedsPixels {
		return PhotoMetadata{}, ErrPhotoDimensions
	}

	if _, err := decode(bytes.NewReader(data)); err != nil {
		return PhotoMetadata{}, fmt.Errorf("%w: decode content: %w", ErrInvalidPhoto, err)
	}
	return PhotoMetadata{MIMEType: actual, SizeBytes: len(data)}, nil
}
