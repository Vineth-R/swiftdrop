package courier

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"

	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
)

const (
	// ProofKindPickup marks the photo taken when the courier collects a parcel.
	ProofKindPickup = "PICKUP"
	// ProofKindDelivery marks the photo taken when the parcel is handed over.
	ProofKindDelivery = "DELIVERY"
)

var (
	// ErrPhotoRequired is returned when a proof-backed workflow has no photo.
	ErrPhotoRequired = errors.New("a proof photo is required")
	// ErrDeliveryProofRequired keeps the generic status endpoint from marking a
	// parcel DELIVERED without the delivery proof transaction.
	ErrDeliveryProofRequired = errors.New("delivery confirmation requires the delivery proof endpoint")
	// ErrPickupProofRequired keeps the generic status endpoint from marking a
	// parcel PICKED_UP without the pickup proof transaction.
	ErrPickupProofRequired = errors.New("pickup confirmation requires the pickup proof endpoint")
)

// PickupInput carries the authenticated courier separately from the request.
type PickupInput struct {
	ParcelID  string
	CourierID string
	Note      string
	Photo     *ProofPhoto
}

// DeliveryInput carries the authenticated courier separately from the request.
type DeliveryInput struct {
	ParcelID      string
	CourierID     string
	RecipientName string
	Note          string
	Photo         *ProofPhoto
}

// WorkflowResult is the state after a pickup or delivery confirmation. A
// retried request reports alreadyApplied and creates no duplicate proof,
// history or outbox rows.
type WorkflowResult struct {
	ParcelID       string                    `json:"parcelId"`
	TrackingNumber string                    `json:"trackingNumber"`
	PreviousStatus statemachine.ParcelStatus `json:"previousStatus"`
	Status         statemachine.ParcelStatus `json:"status"`
	UpdatedAt      time.Time                 `json:"updatedAt"`
	AlreadyApplied bool                      `json:"alreadyApplied"`
	History        []History                 `json:"history"`
	Proof          *Proof                    `json:"proof"`
}

// WorkflowDB is the write dependency for proof-backed courier transitions.
type WorkflowDB interface {
	Begin(context.Context) (pgx.Tx, error)
	QueryRow(context.Context, string, ...any) pgx.Row
}

// WorkflowService commits pickup and delivery proof, history and outbox data
// in one transaction. Object storage is written before the transaction and
// removed again if the transaction does not commit.
type WorkflowService struct {
	db            WorkflowDB
	storage       objectstore.Store
	maxPerParcel  int
	maxPhotoBytes int
}

func NewWorkflowService(db WorkflowDB, storage objectstore.Store, maxPerParcel int) *WorkflowService {
	if maxPerParcel < 1 {
		maxPerParcel = DefaultProofMaxPerParcel
	}
	return &WorkflowService{
		db: db, storage: storage, maxPerParcel: maxPerParcel,
		maxPhotoBytes: proofmedia.MaxPhotoBytes,
	}
}

// WithMaxPhotoBytes lowers the configured upload limit. Values above the
// database's 5 MiB constraint are ignored.
func (s *WorkflowService) WithMaxPhotoBytes(limit int) *WorkflowService {
	if s != nil && limit > 0 && limit <= proofmedia.MaxPhotoBytes {
		s.maxPhotoBytes = limit
	}
	return s
}

// ConfirmPickup records the pickup proof and both status transitions
// (CREATED -> PICKED_UP -> IN_TRANSIT) atomically. The courier performs one
// action, but the history and outbox keep both transitions so downstream
// consumers see the same sequence as before.
func (s *WorkflowService) ConfirmPickup(ctx context.Context, input PickupInput) (result WorkflowResult, err error) {
	if s == nil || s.db == nil || s.storage == nil {
		return WorkflowResult{}, ErrUnavailable
	}
	note := strings.TrimSpace(input.Note)
	if utf8.RuneCountInString(note) > 500 {
		return WorkflowResult{}, statemachine.NewValidationError("note must not exceed 500 characters")
	}
	if input.Photo == nil {
		return WorkflowResult{}, ErrPhotoRequired
	}
	metadata, err := s.photoMetadata(input.Photo)
	if err != nil {
		return WorkflowResult{}, err
	}

	summary, err := s.summary(ctx, input.ParcelID, input.CourierID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if summary.Status == statemachine.StatusInTransit {
		return appliedWorkflowResult(input.ParcelID, summary.TrackingNumber,
			statemachine.StatusCreated, statemachine.StatusInTransit), nil
	}
	if summary.Status != statemachine.StatusCreated {
		return WorkflowResult{}, pickupNotAllowed(summary.Status)
	}

	proofID, err := proofID()
	if err != nil {
		return WorkflowResult{}, err
	}
	key, err := proofmedia.NewKey(input.ParcelID, proofID, metadata.MIMEType)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
	}
	if err := s.storage.Put(ctx, key, input.Photo.Data, metadata.MIMEType); err != nil {
		if errors.Is(err, proofmedia.ErrUnsupportedPhotoType) {
			return WorkflowResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
		}
		return WorkflowResult{}, fmt.Errorf("store pickup proof photo: %w", err)
	}
	uploadedKey := key
	defer func() {
		if uploadedKey == "" {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if deleteErr := s.storage.Delete(cleanupCtx, uploadedKey); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up pickup proof photo: %w", deleteErr))
		}
	}()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("begin courier pickup: %w", err)
	}
	committed := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed) || committed {
			return
		}
		err = errors.Join(err, fmt.Errorf("rollback courier pickup: %w", rollbackErr))
	}()

	locked, err := lockCourierParcel(ctx, tx, input.ParcelID, input.CourierID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if locked.Status == statemachine.StatusInTransit {
		return appliedWorkflowResult(input.ParcelID, locked.TrackingNumber,
			statemachine.StatusCreated, statemachine.StatusInTransit), nil
	}
	if locked.Status != statemachine.StatusCreated {
		return WorkflowResult{}, pickupNotAllowed(locked.Status)
	}
	if err := s.checkQuota(ctx, tx, input.ParcelID); err != nil {
		return WorkflowResult{}, err
	}

	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	var pickedAt time.Time
	err = tx.QueryRow(ctx, `WITH ts AS MATERIALIZED (SELECT clock_timestamp() AS t)
		UPDATE parcel.parcels SET status = $2::text, updated_at = ts.t
		FROM ts WHERE id = $1 RETURNING ts.t`,
		input.ParcelID, statemachine.StatusPickedUp).Scan(&pickedAt)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("mark courier parcel picked up: %w", err)
	}
	transitAt := pickedAt.Add(time.Microsecond)
	if _, err := tx.Exec(ctx, `UPDATE parcel.parcels
		SET status = $2::text, updated_at = $3 WHERE id = $1`,
		input.ParcelID, statemachine.StatusInTransit, transitAt); err != nil {
		return WorkflowResult{}, fmt.Errorf("mark courier parcel in transit: %w", err)
	}
	if err := insertWorkflowProof(ctx, tx, proofID, input.ParcelID, ProofKindPickup,
		nil, notePtr, key, metadata.MIMEType, metadata.SizeBytes, input.CourierID, pickedAt); err != nil {
		return WorkflowResult{}, err
	}

	parcelRepo, err := repository.New(tx)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("create courier pickup history writer: %w", err)
	}
	role := "COURIER"
	created := statemachine.StatusCreated
	picked := statemachine.StatusPickedUp
	createdRepo := repository.ParcelStatus(created)
	pickedRepo := repository.ParcelStatus(picked)
	pickedHistoryID, err := statusHistoryID()
	if err != nil {
		return WorkflowResult{}, err
	}
	if _, err := parcelRepo.AppendStatusHistory(ctx, repository.AppendStatusHistoryParams{
		ID: pickedHistoryID, ParcelID: input.ParcelID, PreviousStatus: &createdRepo,
		NewStatus: pickedRepo, ActorUserID: &input.CourierID,
		ActorRole: &role, Note: notePtr, CreatedAt: &pickedAt,
	}); err != nil {
		return WorkflowResult{}, fmt.Errorf("append courier pickup history: %w", err)
	}
	transitHistoryID, err := statusHistoryID()
	if err != nil {
		return WorkflowResult{}, err
	}
	if _, err := parcelRepo.AppendStatusHistory(ctx, repository.AppendStatusHistoryParams{
		ID: transitHistoryID, ParcelID: input.ParcelID, PreviousStatus: &pickedRepo,
		NewStatus: repository.ParcelStatus(statemachine.StatusInTransit), ActorUserID: &input.CourierID,
		ActorRole: &role, CreatedAt: &transitAt,
	}); err != nil {
		return WorkflowResult{}, fmt.Errorf("append courier in-transit history: %w", err)
	}

	if err := writeStatusEvent(ctx, tx, statusEvent{
		ParcelID: input.ParcelID, TrackingNumber: locked.TrackingNumber,
		PreviousStatus: created, Status: picked, ActorUserID: input.CourierID,
		CustomerID: locked.CustomerID, MerchantID: locked.MerchantID,
		Note: notePtr, OccurredAt: pickedAt,
	}); err != nil {
		return WorkflowResult{}, err
	}
	if err := writeStatusEvent(ctx, tx, statusEvent{
		ParcelID: input.ParcelID, TrackingNumber: locked.TrackingNumber,
		PreviousStatus: picked, Status: statemachine.StatusInTransit,
		ActorUserID: input.CourierID, CustomerID: locked.CustomerID,
		MerchantID: locked.MerchantID, OccurredAt: transitAt,
	}); err != nil {
		return WorkflowResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowResult{}, fmt.Errorf("commit courier pickup: %w", err)
	}
	committed = true
	uploadedKey = ""
	proof := Proof{
		ID: proofID, Kind: ProofKindPickup, Note: notePtr, HasPhoto: true,
		CreatedAt: pickedAt.UTC(),
	}
	return WorkflowResult{
		ParcelID: input.ParcelID, TrackingNumber: locked.TrackingNumber,
		PreviousStatus: created, Status: statemachine.StatusInTransit,
		UpdatedAt: transitAt.UTC(), Proof: &proof,
		History: []History{
			{PreviousStatus: &createdRepo, Status: pickedRepo, Note: notePtr,
				ActorRole: &role, ChangedAt: pickedAt.UTC()},
			{PreviousStatus: &pickedRepo, Status: repository.ParcelStatus(statemachine.StatusInTransit),
				ActorRole: &role, ChangedAt: transitAt.UTC()},
		},
	}, nil
}

// ConfirmDelivery records the delivery proof, recipient and status transition
// (OUT_FOR_DELIVERY -> DELIVERED) atomically.
func (s *WorkflowService) ConfirmDelivery(ctx context.Context, input DeliveryInput) (result WorkflowResult, err error) {
	if s == nil || s.db == nil || s.storage == nil {
		return WorkflowResult{}, ErrUnavailable
	}
	recipient := strings.TrimSpace(input.RecipientName)
	note := strings.TrimSpace(input.Note)
	if err := statemachine.ValidateTransition(statemachine.StatusOutForDelivery,
		statemachine.StatusDelivered, statemachine.TransitionInput{
			RecipientName: recipient, Note: note,
		}); err != nil {
		return WorkflowResult{}, err
	}
	if input.Photo == nil {
		return WorkflowResult{}, ErrPhotoRequired
	}
	metadata, err := s.photoMetadata(input.Photo)
	if err != nil {
		return WorkflowResult{}, err
	}

	summary, err := s.summary(ctx, input.ParcelID, input.CourierID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if summary.Status == statemachine.StatusDelivered {
		return appliedWorkflowResult(input.ParcelID, summary.TrackingNumber,
			statemachine.StatusOutForDelivery, statemachine.StatusDelivered), nil
	}
	if summary.Status != statemachine.StatusOutForDelivery {
		return WorkflowResult{}, statemachine.NewInvalidTransitionError(summary.Status, statemachine.StatusDelivered)
	}

	proofID, err := proofID()
	if err != nil {
		return WorkflowResult{}, err
	}
	key, err := proofmedia.NewKey(input.ParcelID, proofID, metadata.MIMEType)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
	}
	if err := s.storage.Put(ctx, key, input.Photo.Data, metadata.MIMEType); err != nil {
		if errors.Is(err, proofmedia.ErrUnsupportedPhotoType) {
			return WorkflowResult{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
		}
		return WorkflowResult{}, fmt.Errorf("store delivery proof photo: %w", err)
	}
	uploadedKey := key
	defer func() {
		if uploadedKey == "" {
			return
		}
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if deleteErr := s.storage.Delete(cleanupCtx, uploadedKey); deleteErr != nil {
			err = errors.Join(err, fmt.Errorf("clean up delivery proof photo: %w", deleteErr))
		}
	}()

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("begin courier delivery: %w", err)
	}
	committed := false
	defer func() {
		cleanupCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		rollbackErr := tx.Rollback(cleanupCtx)
		if rollbackErr == nil || errors.Is(rollbackErr, pgx.ErrTxClosed) || committed {
			return
		}
		err = errors.Join(err, fmt.Errorf("rollback courier delivery: %w", rollbackErr))
	}()

	locked, err := lockCourierParcel(ctx, tx, input.ParcelID, input.CourierID)
	if err != nil {
		return WorkflowResult{}, err
	}
	if locked.Status == statemachine.StatusDelivered {
		return appliedWorkflowResult(input.ParcelID, locked.TrackingNumber,
			statemachine.StatusOutForDelivery, statemachine.StatusDelivered), nil
	}
	if locked.Status != statemachine.StatusOutForDelivery {
		return WorkflowResult{}, statemachine.NewInvalidTransitionError(locked.Status, statemachine.StatusDelivered)
	}
	if err := s.checkQuota(ctx, tx, input.ParcelID); err != nil {
		return WorkflowResult{}, err
	}

	var notePtr *string
	if note != "" {
		notePtr = &note
	}
	var deliveredAt time.Time
	err = tx.QueryRow(ctx, `WITH ts AS MATERIALIZED (SELECT clock_timestamp() AS t)
		UPDATE parcel.parcels SET status = $2::text, delivered_to_name = $3,
		delivered_at = ts.t, updated_at = ts.t
		FROM ts WHERE id = $1 RETURNING ts.t`,
		input.ParcelID, statemachine.StatusDelivered, recipient).Scan(&deliveredAt)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("mark courier parcel delivered: %w", err)
	}
	if err := insertWorkflowProof(ctx, tx, proofID, input.ParcelID, ProofKindDelivery,
		&recipient, notePtr, key, metadata.MIMEType, metadata.SizeBytes,
		input.CourierID, deliveredAt); err != nil {
		return WorkflowResult{}, err
	}

	parcelRepo, err := repository.New(tx)
	if err != nil {
		return WorkflowResult{}, fmt.Errorf("create courier delivery history writer: %w", err)
	}
	role := "COURIER"
	previous := statemachine.StatusOutForDelivery
	previousRepo := repository.ParcelStatus(previous)
	historyID, err := statusHistoryID()
	if err != nil {
		return WorkflowResult{}, err
	}
	if _, err := parcelRepo.AppendStatusHistory(ctx, repository.AppendStatusHistoryParams{
		ID: historyID, ParcelID: input.ParcelID, PreviousStatus: &previousRepo,
		NewStatus:   repository.ParcelStatus(statemachine.StatusDelivered),
		ActorUserID: &input.CourierID, ActorRole: &role, Note: notePtr,
		CreatedAt: &deliveredAt,
	}); err != nil {
		return WorkflowResult{}, fmt.Errorf("append courier delivery history: %w", err)
	}
	if err := writeStatusEvent(ctx, tx, statusEvent{
		ParcelID: input.ParcelID, TrackingNumber: locked.TrackingNumber,
		PreviousStatus: previous, Status: statemachine.StatusDelivered,
		ActorUserID: input.CourierID, CustomerID: locked.CustomerID,
		MerchantID: locked.MerchantID, Note: notePtr, OccurredAt: deliveredAt,
	}); err != nil {
		return WorkflowResult{}, err
	}

	if err := tx.Commit(ctx); err != nil {
		return WorkflowResult{}, fmt.Errorf("commit courier delivery: %w", err)
	}
	committed = true
	uploadedKey = ""
	proof := Proof{
		ID: proofID, Kind: ProofKindDelivery, RecipientName: &recipient,
		Note: notePtr, HasPhoto: true, CreatedAt: deliveredAt.UTC(),
	}
	return WorkflowResult{
		ParcelID: input.ParcelID, TrackingNumber: locked.TrackingNumber,
		PreviousStatus: previous, Status: statemachine.StatusDelivered,
		UpdatedAt: deliveredAt.UTC(), Proof: &proof,
		History: []History{{
			PreviousStatus: &previousRepo, Status: repository.ParcelStatus(statemachine.StatusDelivered),
			Note: notePtr, ActorRole: &role, ChangedAt: deliveredAt.UTC(),
		}},
	}, nil
}

type parcelSummary struct {
	TrackingNumber string
	Status         statemachine.ParcelStatus
}

// summary checks ownership and reads the current status without locking, so a
// retried or unauthorized request does not upload a photo that will be thrown
// away.
func (s *WorkflowService) summary(ctx context.Context, parcelID, courierID string) (parcelSummary, error) {
	var summary parcelSummary
	var currentCourierID *string
	var assignmentCurrent bool
	err := s.db.QueryRow(ctx, `SELECT p.tracking_number, p.status, p.current_courier_id,
		EXISTS (SELECT 1 FROM parcel.parcel_assignments a
			WHERE a.parcel_id = p.id AND a.courier_id = $2 AND a.is_current)
		FROM parcel.parcels p WHERE p.id = $1`, parcelID, courierID).
		Scan(&summary.TrackingNumber, &summary.Status, &currentCourierID, &assignmentCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return parcelSummary{}, ErrNotFound
	}
	if err != nil {
		return parcelSummary{}, fmt.Errorf("read courier workflow parcel: %w", err)
	}
	if currentCourierID == nil || *currentCourierID != courierID {
		return parcelSummary{}, ErrForbidden
	}
	if !assignmentCurrent {
		return parcelSummary{}, ErrInconsistentAssignment
	}
	return summary, nil
}

// appliedWorkflowResult reports a retried transition that is already
// committed. Nothing is written, so no proof, history or outbox row is
// duplicated. The client refreshes the delivery for the full history.
func appliedWorkflowResult(parcelID, trackingNumber string, previous, current statemachine.ParcelStatus) WorkflowResult {
	return WorkflowResult{
		ParcelID: parcelID, TrackingNumber: trackingNumber,
		PreviousStatus: previous, Status: current, UpdatedAt: time.Now().UTC(),
		AlreadyApplied: true, History: []History{},
	}
}

type lockedParcel struct {
	TrackingNumber string
	Status         statemachine.ParcelStatus
	CustomerID     *string
	MerchantID     *string
}

// lockCourierParcel holds the parcel row for the rest of the transaction and
// re-verifies ownership and the current assignment, so a reassignment cannot
// race a workflow write.
func lockCourierParcel(ctx context.Context, tx pgx.Tx, parcelID, courierID string) (lockedParcel, error) {
	var parcel lockedParcel
	var currentCourierID *string
	var assignmentCurrent bool
	err := tx.QueryRow(ctx, `SELECT p.tracking_number, p.status, p.current_courier_id,
		p.customer_user_id, p.merchant_id,
		EXISTS (SELECT 1 FROM parcel.parcel_assignments a
			WHERE a.parcel_id = p.id AND a.courier_id = $2 AND a.is_current)
		FROM parcel.parcels p WHERE p.id = $1 FOR UPDATE`, parcelID, courierID).
		Scan(&parcel.TrackingNumber, &parcel.Status, &currentCourierID,
			&parcel.CustomerID, &parcel.MerchantID, &assignmentCurrent)
	if errors.Is(err, pgx.ErrNoRows) {
		return lockedParcel{}, ErrNotFound
	}
	if err != nil {
		return lockedParcel{}, fmt.Errorf("lock courier workflow parcel: %w", err)
	}
	if currentCourierID == nil || *currentCourierID != courierID {
		return lockedParcel{}, ErrForbidden
	}
	if !assignmentCurrent {
		return lockedParcel{}, ErrInconsistentAssignment
	}
	return parcel, nil
}

func (s *WorkflowService) photoMetadata(photo *ProofPhoto) (proofmedia.PhotoMetadata, error) {
	metadata, err := proofmedia.ValidatePhotoLimit(photo.Data, photo.MIMEType, s.maxPhotoBytes)
	if err != nil {
		if errors.Is(err, proofmedia.ErrPhotoTooLarge) {
			return proofmedia.PhotoMetadata{}, proofmedia.ErrPhotoTooLarge
		}
		return proofmedia.PhotoMetadata{}, fmt.Errorf("%w: %v", ErrProofUnsupported, err)
	}
	return metadata, nil
}

func (s *WorkflowService) checkQuota(ctx context.Context, tx pgx.Tx, parcelID string) error {
	var count int
	if err := tx.QueryRow(ctx,
		`SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id = $1`, parcelID).
		Scan(&count); err != nil {
		return fmt.Errorf("count courier proofs: %w", err)
	}
	if count >= s.maxPerParcel {
		return ErrProofLimitReached
	}
	return nil
}

func insertWorkflowProof(
	ctx context.Context,
	tx pgx.Tx,
	id, parcelID, kind string,
	recipient, note *string,
	key, mimeType string,
	sizeBytes int,
	capturedBy string,
	createdAt time.Time,
) error {
	_, err := tx.Exec(ctx, `INSERT INTO parcel.delivery_proofs
		(id, parcel_id, proof_kind, recipient_name, note, photo_path,
		photo_mime_type, photo_size_bytes, captured_by, created_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		id, parcelID, kind, recipient, note, key, mimeType, sizeBytes, capturedBy, createdAt)
	if err != nil {
		return fmt.Errorf("insert courier workflow proof: %w", err)
	}
	return nil
}

func pickupNotAllowed(status statemachine.ParcelStatus) error {
	if status == statemachine.StatusPickedUp {
		return &statemachine.DomainError{
			HTTPStatus: 409,
			Code:       "INVALID_TRANSITION",
			Message:    "pickup has already been recorded; continue the delivery with the in-transit action",
		}
	}
	return statemachine.NewInvalidTransitionError(status, statemachine.StatusPickedUp)
}
