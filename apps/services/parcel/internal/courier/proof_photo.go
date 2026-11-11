package courier

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
)

var ErrProofPhotoNotFound = errors.New("delivery proof photo not found")

const DefaultProofPresignTTL = 5 * time.Minute

// PhotoViewer identifies the caller without trusting any request-supplied ID.
type PhotoViewer struct {
	UserID  string
	Staff   bool
	Courier bool
}

// ProofPhotoDownload is a short-lived URL, or a stream marker for providers
// that cannot issue presigned URLs. Stream is true only for local storage.
type ProofPhotoDownload struct {
	URL       string
	ExpiresAt time.Time
	MIMEType  string
	Stream    bool
}

type ProofPhotoService struct {
	db      ProofDB
	storage objectstore.Store
	ttl     time.Duration
}

func NewProofPhotoService(db ProofDB, storage objectstore.Store, ttl time.Duration) *ProofPhotoService {
	if ttl <= 0 {
		ttl = DefaultProofPresignTTL
	}
	return &ProofPhotoService{db: db, storage: storage, ttl: ttl}
}

// Photo authorizes the viewer against the parcel and returns a short-lived
// download target for the latest proof photo. Customers see only their own
// parcels; couriers only parcels currently assigned to them; dispatchers and
// admins may read operationally.
func (s *ProofPhotoService) Photo(ctx context.Context, parcelID string, viewer PhotoViewer) (ProofPhotoDownload, error) {
	if s == nil || s.storage == nil {
		return ProofPhotoDownload{}, ErrUnavailable
	}
	key, contentType, err := s.resolve(ctx, parcelID, viewer)
	if err != nil {
		return ProofPhotoDownload{}, err
	}

	expiresAt := time.Now().Add(s.ttl).UTC()
	presigned, _, err := s.storage.PresignGet(ctx, key, s.ttl)
	if err == nil {
		return ProofPhotoDownload{URL: presigned, ExpiresAt: expiresAt, MIMEType: contentType}, nil
	}
	if !errors.Is(err, objectstore.ErrPresignUnsupported) {
		return ProofPhotoDownload{}, fmt.Errorf("create proof photo URL: %w", err)
	}

	// Local development cannot presign. The browser calls the authorized
	// content route instead; no filesystem path is ever exposed.
	return ProofPhotoDownload{
		URL:       "/api/v1/parcels/" + url.PathEscape(parcelID) + "/proof/photo/content",
		ExpiresAt: expiresAt,
		MIMEType:  contentType,
		Stream:    true,
	}, nil
}

// Open returns the private photo stream used by the authorized content route.
func (s *ProofPhotoService) Open(ctx context.Context, parcelID string, viewer PhotoViewer) (io.ReadCloser, string, int64, error) {
	if s == nil || s.storage == nil {
		return nil, "", 0, ErrUnavailable
	}
	key, contentType, err := s.resolve(ctx, parcelID, viewer)
	if err != nil {
		return nil, "", 0, err
	}
	object, err := s.storage.Open(ctx, key)
	if err != nil {
		if errors.Is(err, objectstore.ErrNotFound) {
			return nil, "", 0, fmt.Errorf("%w: object is missing from storage", ErrProofPhotoNotFound)
		}
		return nil, "", 0, fmt.Errorf("open proof photo: %w", err)
	}
	if object.ContentType != "" {
		contentType = object.ContentType
	}
	return object.Body, contentType, object.Size, nil
}

func (s *ProofPhotoService) resolve(ctx context.Context, parcelID string, viewer PhotoViewer) (string, string, error) {
	if s == nil || s.db == nil {
		return "", "", ErrUnavailable
	}

	var customer *string
	var currentCourier *string
	var assignmentCurrent bool
	err := s.db.QueryRow(ctx, `SELECT p.customer_user_id, p.current_courier_id, EXISTS (
		SELECT 1 FROM parcel.parcel_assignments a
		WHERE a.parcel_id=p.id AND a.courier_id=$2 AND a.is_current)
		FROM parcel.parcels p WHERE p.id=$1`, parcelID, viewer.UserID).
		Scan(&customer, &currentCourier, &assignmentCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("read proof photo parcel: %w", err)
	}

	switch {
	case viewer.Staff:
	case viewer.Courier:
		if currentCourier == nil || *currentCourier != viewer.UserID || !assignmentCurrent {
			return "", "", ErrForbidden
		}
	case customer != nil && *customer == viewer.UserID:
	default:
		// Do not reveal whether another customer's parcel exists.
		return "", "", ErrNotFound
	}

	var key string
	var mimeType *string
	err = s.db.QueryRow(ctx, `SELECT photo_path, photo_mime_type FROM parcel.delivery_proofs
		WHERE parcel_id=$1 AND photo_path IS NOT NULL
		ORDER BY created_at DESC, id DESC LIMIT 1`, parcelID).Scan(&key, &mimeType)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", ErrProofPhotoNotFound
	}
	if err != nil {
		return "", "", fmt.Errorf("read proof photo metadata: %w", err)
	}
	if !objectstore.ValidKey(key) {
		return "", "", fmt.Errorf("%w: stored object key is invalid", ErrProofPhotoNotFound)
	}

	contentType := "application/octet-stream"
	if mimeType != nil && *mimeType != "" {
		contentType = *mimeType
	}
	return key, contentType, nil
}
