package courier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
)

const DefaultProofMaxPerParcel = 20

var (
	ErrProofEmpty        = errors.New("delivery proof must contain at least one field")
	ErrProofValidation   = errors.New("delivery proof validation failed")
	ErrProofUnsupported  = errors.New("delivery proof photo is unsupported")
	ErrProofLimitReached = errors.New("delivery proof limit reached for this parcel")
)

type ProofPhoto struct {
	Data     []byte
	MIMEType string
}

type ProofInput struct {
	ParcelID, CourierID string
	RecipientName       string
	Note                string
	Photo               *ProofPhoto
}

type ProofResult struct {
	ID            string    `json:"id"`
	ParcelID      string    `json:"parcelId"`
	RecipientName *string   `json:"recipientName"`
	Note          *string   `json:"note"`
	HasPhoto      bool      `json:"hasPhoto"`
	CreatedAt     time.Time `json:"createdAt"`
}

type ProofDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

type ProofService struct {
	db            ProofDB
	storage       objectstore.Store
	maxPerParcel  int
	maxPhotoBytes int
}

func NewProofService(db ProofDB, storage objectstore.Store, maxPerParcel int) *ProofService {
	if maxPerParcel < 1 {
		maxPerParcel = DefaultProofMaxPerParcel
	}
	return &ProofService{db: db, storage: storage, maxPerParcel: maxPerParcel, maxPhotoBytes: proofmedia.MaxPhotoBytes}
}

// WithMaxPhotoBytes lowers the configured upload limit. Values above the
// database's 5 MiB constraint are ignored.
func (s *ProofService) WithMaxPhotoBytes(limit int) *ProofService {
	if s != nil && limit > 0 && limit <= proofmedia.MaxPhotoBytes {
		s.maxPhotoBytes = limit
	}
	return s
}

// CreateProof verifies the courier's current assignment before uploading the
// photo, then re-verifies under the parcel row lock before inserting metadata.
// PostgreSQL and object storage do not share a transaction, so a failed insert
// deletes the uploaded object. A committed row therefore always references an
// object that was uploaded before the commit.
func (s *ProofService) CreateProof(ctx context.Context, input ProofInput) (result ProofResult, err error) {
	if s == nil || s.db == nil || s.storage == nil {
		return ProofResult{}, ErrUnavailable
	}
	recipient := strings.TrimSpace(input.RecipientName)
	note := strings.TrimSpace(input.Note)
	if utf8.RuneCountInString(recipient) > 150 || utf8.RuneCountInString(note) > 500 {
		return ProofResult{}, ErrProofValidation
	}
	if recipient == "" && note == "" && input.Photo == nil {
		return ProofResult{}, ErrProofEmpty
	}

	var metadata proofmedia.PhotoMetadata
	if input.Photo != nil {
		metadata, err = proofmedia.ValidatePhotoLimit(input.Photo.Data, input.Photo.MIMEType, s.maxPhotoBytes)
		if err != nil {
			switch {
			case errors.Is(err, proofmedia.ErrPhotoTooLarge):
				return ProofResult{}, proofmedia.ErrPhotoTooLarge
			default:
				return ProofResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
			}
		}
	}

	result = ProofResult{ParcelID: input.ParcelID, HasPhoto: input.Photo != nil}
	if recipient != "" {
		result.RecipientName = &recipient
	}
	if note != "" {
		result.Note = &note
	}
	result.ID, err = proofID()
	if err != nil {
		return ProofResult{}, err
	}

	var storageKey *string
	uploadedKey := ""
	if input.Photo != nil {
		// Fail before writing an object when the courier is not assigned.
		if err := s.checkAssignment(ctx, input.ParcelID, input.CourierID, false); err != nil {
			return ProofResult{}, err
		}
		// Avoid uploading when the quota is already exhausted; the transaction
		// re-checks under the row lock for concurrent uploads.
		count, countErr := s.proofCount(ctx, input.ParcelID)
		if countErr != nil {
			return ProofResult{}, countErr
		}
		if count >= s.maxPerParcel {
			return ProofResult{}, ErrProofLimitReached
		}
		key, keyErr := proofmedia.NewKey(input.ParcelID, result.ID, metadata.MIMEType)
		if keyErr != nil {
			return ProofResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, keyErr)
		}
		if putErr := s.storage.Put(ctx, key, input.Photo.Data, metadata.MIMEType); putErr != nil {
			if errors.Is(putErr, proofmedia.ErrUnsupportedPhotoType) {
				return ProofResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, putErr)
			}
			return ProofResult{}, fmt.Errorf("store courier proof photo: %w", putErr)
		}
		storageKey = &key
		uploadedKey = key
		defer func() {
			if uploadedKey == "" {
				return
			}
			cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if deleteErr := s.storage.Delete(cleanupCtx, uploadedKey); deleteErr != nil {
				err = errors.Join(err, fmt.Errorf("clean up courier proof photo: %w", deleteErr))
			}
		}()
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return ProofResult{}, fmt.Errorf("begin courier proof: %w", err)
	}
	committed := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed) || committed {
			return
		}
		// A committed proof is durable. A later rollback error must never turn a
		// successful operation into a reported failure.
		err = errors.Join(err, fmt.Errorf("rollback courier proof: %w", rollbackErr))
	}()

	var currentCourierID *string
	var assignmentCurrent bool
	err = tx.QueryRow(ctx, `SELECT p.current_courier_id, EXISTS (
		SELECT 1 FROM parcel.parcel_assignments a
		WHERE a.parcel_id=p.id AND a.courier_id=$2 AND a.is_current)
		FROM parcel.parcels p WHERE p.id=$1 FOR UPDATE`, input.ParcelID, input.CourierID).
		Scan(&currentCourierID, &assignmentCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProofResult{}, ErrNotFound
	}
	if err != nil {
		return ProofResult{}, fmt.Errorf("lock courier proof parcel: %w", err)
	}
	if currentCourierID == nil || *currentCourierID != input.CourierID {
		return ProofResult{}, ErrForbidden
	}
	if !assignmentCurrent {
		return ProofResult{}, ErrInconsistentAssignment
	}

	// The parcel row lock serializes proof inserts for one parcel, so this count
	// cannot be bypassed by concurrent uploads. Counting before storing the file
	// also avoids writing a file that would be rejected by the quota.
	var proofCount int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, input.ParcelID).Scan(&proofCount); err != nil {
		return ProofResult{}, fmt.Errorf("count courier proofs: %w", err)
	}
	if proofCount >= s.maxPerParcel {
		return ProofResult{}, ErrProofLimitReached
	}

	var mimeType *string
	var sizeBytes *int
	if input.Photo != nil {
		mimeType = &metadata.MIMEType
		sizeBytes = &metadata.SizeBytes
	}
	err = tx.QueryRow(ctx, `INSERT INTO parcel.delivery_proofs
		(id,parcel_id,recipient_name,note,photo_path,photo_mime_type,photo_size_bytes,captured_by)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8) RETURNING created_at`, result.ID, input.ParcelID,
		result.RecipientName, result.Note, storageKey, mimeType, sizeBytes, input.CourierID).
		Scan(&result.CreatedAt)
	if err != nil {
		return ProofResult{}, fmt.Errorf("insert courier proof: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		// A transport error during COMMIT has an unknown outcome. Resolve it by
		// proof ID before deciding whether the file is an orphan.
		var exists bool
		probeCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		probeErr := s.db.QueryRow(probeCtx,
			`SELECT EXISTS (SELECT 1 FROM parcel.delivery_proofs WHERE id=$1)`, result.ID).Scan(&exists)
		if probeErr != nil {
			// Keep the object: the row may have committed and deleting it would
			// break that proof. Reconciliation reports the ambiguity.
			uploadedKey = ""
			return ProofResult{}, errors.Join(fmt.Errorf("commit courier proof: %w", err),
				fmt.Errorf("resolve courier proof commit: %w", probeErr))
		}
		if !exists {
			return ProofResult{}, fmt.Errorf("commit courier proof: %w", err)
		}
	}
	committed = true
	uploadedKey = ""
	result.CreatedAt = result.CreatedAt.UTC()
	return result, nil
}

func (s *ProofService) proofCount(ctx context.Context, parcelID string) (int, error) {
	var count int
	if err := s.db.QueryRow(ctx,
		`SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, parcelID).Scan(&count); err != nil {
		return 0, fmt.Errorf("count courier proofs: %w", err)
	}
	return count, nil
}

func (s *ProofService) checkAssignment(ctx context.Context, parcelID, courierID string, lock bool) error {
	query := `SELECT p.current_courier_id, EXISTS (
		SELECT 1 FROM parcel.parcel_assignments a
		WHERE a.parcel_id=p.id AND a.courier_id=$2 AND a.is_current)
		FROM parcel.parcels p WHERE p.id=$1`
	if lock {
		query += " FOR UPDATE"
	}
	var currentCourierID *string
	var assignmentCurrent bool
	err := s.db.QueryRow(ctx, query, parcelID, courierID).Scan(&currentCourierID, &assignmentCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return fmt.Errorf("check courier proof assignment: %w", err)
	}
	if currentCourierID == nil || *currentCourierID != courierID {
		return ErrForbidden
	}
	if !assignmentCurrent {
		return ErrInconsistentAssignment
	}
	return nil
}

func proofID() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", fmt.Errorf("generate courier proof ID: %w", err)
	}
	return id.String(), nil
}
