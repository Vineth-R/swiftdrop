package proof

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
)

var ErrInvalidStorageKey = errors.New("proof storage key is invalid")

// NewKey generates the server-owned object key for one proof photo. Client
// file names never reach storage.
func NewKey(parcelID, proofID, mimeType string) (string, error) {
	if _, err := uuid.Parse(parcelID); err != nil {
		return "", fmt.Errorf("%w: parcel ID is not a UUID", ErrInvalidStorageKey)
	}
	if _, err := uuid.Parse(proofID); err != nil {
		return "", fmt.Errorf("%w: proof ID is not a UUID", ErrInvalidStorageKey)
	}
	extension := ".jpg"
	switch mimeType {
	case "image/jpeg":
	case "image/png":
		extension = ".png"
	default:
		return "", fmt.Errorf("%w: unsupported verified MIME type", ErrUnsupportedPhotoType)
	}
	return parcelID + "/" + proofID + extension, nil
}
