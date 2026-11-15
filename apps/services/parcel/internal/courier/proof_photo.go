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

var (
	ErrProofPhotoNotFound = errors.New("delivery proof photo not found")
	// ErrProofNotFound reports a proof ID that does not belong to the parcel.
	ErrProofNotFound = errors.New("delivery proof not found")
)

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
	_, key, contentType, err := s.resolve(ctx, parcelID, viewer, "")
	if err != nil {
		return ProofPhotoDownload{}, err
	}
	// Keep the original latest-photo content route for existing clients.
	return s.target(ctx, parcelID, "", key, contentType)
}

// PhotoByProof is the same authorized lookup for one specific proof, so the
// activity view can show a pickup photo and a delivery photo independently.
func (s *ProofPhotoService) PhotoByProof(ctx context.Context, parcelID, proofID string, viewer PhotoViewer) (ProofPhotoDownload, error) {
	if s == nil || s.storage == nil {
		return ProofPhotoDownload{}, ErrUnavailable
	}
	resolvedID, key, contentType, err := s.resolve(ctx, parcelID, viewer, proofID)
	if err != nil {
		return ProofPhotoDownload{}, err
	}
	return s.target(ctx, parcelID, resolvedID, key, contentType)
}

// Open returns the private photo stream used by the authorized content route.
func (s *ProofPhotoService) Open(ctx context.Context, parcelID string, viewer PhotoViewer) (io.ReadCloser, string, int64, error) {
	if s == nil || s.storage == nil {
		return nil, "", 0, ErrUnavailable
	}
	_, key, contentType, err := s.resolve(ctx, parcelID, viewer, "")
	if err != nil {
		return nil, "", 0, err
	}
	return s.open(ctx, key, contentType)
}

// OpenByProof returns the private photo stream for one specific proof.
func (s *ProofPhotoService) OpenByProof(ctx context.Context, parcelID, proofID string, viewer PhotoViewer) (io.ReadCloser, string, int64, error) {
	if s == nil || s.storage == nil {
		return nil, "", 0, ErrUnavailable
	}
	_, key, contentType, err := s.resolve(ctx, parcelID, viewer, proofID)
	if err != nil {
		return nil, "", 0, err
	}
	return s.open(ctx, key, contentType)
}

func (s *ProofPhotoService) target(ctx context.Context, parcelID, proofID, key, contentType string) (ProofPhotoDownload, error) {
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
	path := "/api/v1/parcels/" + url.PathEscape(parcelID) + "/proof/photo/content"
	if proofID != "" {
		path = "/api/v1/parcels/" + url.PathEscape(parcelID) + "/proofs/" +
			url.PathEscape(proofID) + "/photo/content"
	}
	return ProofPhotoDownload{
		URL: path, ExpiresAt: expiresAt, MIMEType: contentType, Stream: true,
	}, nil
}

func (s *ProofPhotoService) open(ctx context.Context, key, contentType string) (io.ReadCloser, string, int64, error) {
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

// resolve authorizes the viewer and returns the stored object key. An empty
// proofID selects the latest photo; otherwise the proof must belong to the
// parcel. Authorization is identical for both lookups.
func (s *ProofPhotoService) resolve(ctx context.Context, parcelID string, viewer PhotoViewer, proofID string) (string, string, string, error) {
	if s == nil || s.db == nil {
		return "", "", "", ErrUnavailable
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
		return "", "", "", ErrNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("read proof photo parcel: %w", err)
	}

	switch {
	case viewer.Staff:
	case viewer.Courier:
		if currentCourier == nil || *currentCourier != viewer.UserID || !assignmentCurrent {
			return "", "", "", ErrForbidden
		}
	case customer != nil && *customer == viewer.UserID:
	default:
		// Do not reveal whether another customer's parcel exists.
		return "", "", "", ErrNotFound
	}

	var id string
	var key string
	var mimeType *string
	if proofID == "" {
		err = s.db.QueryRow(ctx, `SELECT id, photo_path, photo_mime_type FROM parcel.delivery_proofs
			WHERE parcel_id=$1 AND photo_path IS NOT NULL
			ORDER BY created_at DESC, id DESC LIMIT 1`, parcelID).Scan(&id, &key, &mimeType)
	} else {
		err = s.db.QueryRow(ctx, `SELECT id, photo_path, photo_mime_type FROM parcel.delivery_proofs
			WHERE parcel_id=$1 AND id=$2 AND photo_path IS NOT NULL`, parcelID, proofID).Scan(&id, &key, &mimeType)
	}
	if errors.Is(err, pgx.ErrNoRows) {
		if proofID != "" {
			return "", "", "", ErrProofNotFound
		}
		return "", "", "", ErrProofPhotoNotFound
	}
	if err != nil {
		return "", "", "", fmt.Errorf("read proof photo metadata: %w", err)
	}
	if !objectstore.ValidKey(key) {
		return "", "", "", fmt.Errorf("%w: stored object key is invalid", ErrProofPhotoNotFound)
	}

	contentType := "application/octet-stream"
	if mimeType != nil && *mimeType != "" {
		contentType = *mimeType
	}
	return id, key, contentType, nil
}
