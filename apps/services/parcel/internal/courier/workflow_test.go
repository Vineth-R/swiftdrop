package courier

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
)

const workflowParcelID = "00000000-0000-4000-8000-000000000091"

type workflowProofInsert struct {
	id, parcelID, kind string
	recipient, note    *string
	key, mimeType      string
	size               int
	capturedBy         string
	createdAt          time.Time
}

type workflowTx struct {
	pgx.Tx
	status       statemachine.ParcelStatus
	lockStatus   *statemachine.ParcelStatus
	owner        *string
	assignment   bool
	proofCount   int
	stamp        time.Time
	fail         string
	cause        error
	writes       int
	commits      int
	rollbacks    int
	secondStatus statemachine.ParcelStatus
	proofs       []workflowProofInsert
	history      []repository.AppendStatusHistoryParams
	events       [][]byte
	deliveredTo  *string
}

func (tx *workflowTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if err := ctx.Err(); err != nil {
		return statusRow{err: err}
	}
	switch {
	case strings.Contains(sql, "FOR UPDATE"):
		if tx.fail == "lock" {
			return statusRow{err: tx.cause}
		}
		status := tx.status
		if tx.lockStatus != nil {
			status = *tx.lockStatus
		}
		return statusRow{values: []any{"SDK7M2Q9XP4", status, tx.owner,
			(*string)(nil), (*string)(nil), tx.assignment}}
	case strings.Contains(sql, "FROM parcel.parcels p WHERE p.id = $1"):
		if tx.fail == "summary" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{"SDK7M2Q9XP4", tx.status, tx.owner, tx.assignment}}
	case strings.Contains(sql, "count(*) FROM parcel.delivery_proofs"):
		if tx.fail == "quota" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{tx.proofCount}}
	case strings.Contains(sql, "WITH ts AS MATERIALIZED"):
		tx.writes++
		if strings.Contains(sql, "delivered_to_name") {
			deliveredTo := args[2].(string)
			tx.deliveredTo = &deliveredTo
		}
		if tx.fail == "update" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{tx.stamp}}
	case strings.Contains(sql, "INSERT INTO parcel.parcel_status_history"):
		tx.writes++
		if tx.fail == "history" {
			return statusRow{err: tx.cause}
		}
		params, ok := argsToHistory(args)
		if ok {
			tx.history = append(tx.history, params)
		}
		createdAt := tx.stamp
		if params.CreatedAt != nil {
			createdAt = *params.CreatedAt
		}
		return statusRow{values: []any{args[0].(string), args[1].(string),
			args[2].(*repository.ParcelStatus), args[3].(repository.ParcelStatus),
			args[4].(*string), args[5].(*string), args[6].(*string),
			args[7].(*string), createdAt}}
	default:
		return statusRow{err: errors.New("unexpected workflow query")}
	}
}

func argsToHistory(args []any) (repository.AppendStatusHistoryParams, bool) {
	if len(args) != 9 {
		return repository.AppendStatusHistoryParams{}, false
	}
	return repository.AppendStatusHistoryParams{
		ID: args[0].(string), ParcelID: args[1].(string),
		PreviousStatus: args[2].(*repository.ParcelStatus),
		NewStatus:      args[3].(repository.ParcelStatus),
		ActorUserID:    args[4].(*string), ActorRole: args[5].(*string),
		Note: args[6].(*string), FailureReason: args[7].(*string),
		CreatedAt: args[8].(*time.Time),
	}, true
}

func (tx *workflowTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	if err := ctx.Err(); err != nil {
		return pgconn.CommandTag{}, err
	}
	switch {
	case strings.Contains(sql, "INSERT INTO parcel.delivery_proofs"):
		tx.writes++
		if tx.fail == "proof" {
			return pgconn.CommandTag{}, tx.cause
		}
		tx.proofs = append(tx.proofs, workflowProofInsert{
			id: args[0].(string), parcelID: args[1].(string), kind: args[2].(string),
			recipient: args[3].(*string), note: args[4].(*string),
			key: args[5].(string), mimeType: args[6].(string),
			size: args[7].(int), capturedBy: args[8].(string),
			createdAt: args[9].(time.Time),
		})
	case strings.Contains(sql, "UPDATE parcel.parcels"):
		tx.writes++
		tx.secondStatus = args[1].(statemachine.ParcelStatus)
		if tx.fail == "second-update" {
			return pgconn.CommandTag{}, tx.cause
		}
	case strings.Contains(sql, "INSERT INTO parcel.outbox_events"):
		tx.writes++
		if tx.fail == "outbox" {
			return pgconn.CommandTag{}, tx.cause
		}
		tx.events = append(tx.events, append([]byte(nil), args[0].([]byte)...))
	default:
		return pgconn.CommandTag{}, errors.New("unexpected workflow exec")
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *workflowTx) Commit(context.Context) error {
	tx.commits++
	if tx.fail == "commit" {
		return tx.cause
	}
	return nil
}

func (tx *workflowTx) Rollback(context.Context) error {
	tx.rollbacks++
	if tx.fail == "rollback" {
		return tx.cause
	}
	return pgx.ErrTxClosed
}

type workflowDB struct {
	tx       *workflowTx
	beginErr error
}

func (db *workflowDB) Begin(context.Context) (pgx.Tx, error) {
	return db.tx, db.beginErr
}

func (db *workflowDB) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	return db.tx.QueryRow(ctx, sql, args...)
}

func newWorkflowFixture(status statemachine.ParcelStatus) (*workflowDB, *workflowTx) {
	owner := statusCourierID
	tx := &workflowTx{
		status: status, owner: &owner, assignment: true,
		stamp: time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("test", 19800)),
	}
	return &workflowDB{tx: tx}, tx
}

func TestConfirmPickupCommitsProofAndBothTransitions(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	storage := &proofTestStorage{}
	service := NewWorkflowService(db, storage, 0)

	result, err := service.ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID, Note: " At the shop ",
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != statemachine.StatusInTransit || result.PreviousStatus != statemachine.StatusCreated ||
		result.AlreadyApplied || result.TrackingNumber != "SDK7M2Q9XP4" {
		t.Fatalf("result = %+v", result)
	}
	if len(result.History) != 2 || result.History[0].Status != repository.ParcelStatus(statemachine.StatusPickedUp) ||
		result.History[1].Status != repository.ParcelStatus(statemachine.StatusInTransit) {
		t.Fatalf("history = %+v", result.History)
	}
	if result.Proof == nil || result.Proof.Kind != ProofKindPickup || !result.Proof.HasPhoto ||
		result.Proof.Note == nil || *result.Proof.Note != "At the shop" {
		t.Fatalf("proof = %+v", result.Proof)
	}
	if tx.secondStatus != statemachine.StatusInTransit || tx.commits != 1 || tx.rollbacks != 1 {
		t.Fatalf("tx = %+v", tx)
	}
	if storage.saves != 1 || storage.deletes != 0 || !strings.HasPrefix(storage.savedKey, workflowParcelID+"/") {
		t.Fatalf("storage = %+v", storage)
	}
	if len(tx.proofs) != 1 || tx.proofs[0].kind != ProofKindPickup || tx.proofs[0].recipient != nil ||
		tx.proofs[0].note == nil || *tx.proofs[0].note != "At the shop" ||
		tx.proofs[0].capturedBy != statusCourierID || tx.proofs[0].mimeType != "image/png" {
		t.Fatalf("proof row = %+v", tx.proofs)
	}
	if len(tx.history) != 2 || tx.history[0].NewStatus != repository.ParcelStatus(statemachine.StatusPickedUp) ||
		tx.history[1].NewStatus != repository.ParcelStatus(statemachine.StatusInTransit) {
		t.Fatalf("history rows = %+v", tx.history)
	}
	if len(tx.events) != 2 {
		t.Fatalf("events = %d", len(tx.events))
	}
	for index, want := range []string{"PICKED_UP", "IN_TRANSIT"} {
		var payload map[string]any
		if err := json.Unmarshal(tx.events[index], &payload); err != nil {
			t.Fatal(err)
		}
		if payload["status"] != want || payload["parcel_id"] != workflowParcelID ||
			payload["actor_user_id"] != statusCourierID {
			t.Fatalf("event %d = %s", index, tx.events[index])
		}
	}
}

func TestConfirmPickupRetryIsIdempotent(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusInTransit)
	storage := &proofTestStorage{}
	result, err := NewWorkflowService(db, storage, 0).ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyApplied || result.Status != statemachine.StatusInTransit ||
		result.PreviousStatus != statemachine.StatusCreated {
		t.Fatalf("result = %+v", result)
	}
	if storage.saves != 0 || tx.writes != 0 || tx.commits != 0 {
		t.Fatalf("retry wrote: storage=%+v tx=%+v", storage, tx)
	}
}

func TestConfirmPickupConcurrentRetryDiscardsSecondUpload(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	// The pre-check sees CREATED, then another request commits before our lock.
	inTransit := statemachine.StatusInTransit
	tx.lockStatus = &inTransit
	storage := &proofTestStorage{}
	result, err := NewWorkflowService(db, storage, 0).ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyApplied || result.Status != statemachine.StatusInTransit {
		t.Fatalf("result = %+v", result)
	}
	if storage.saves != 1 || storage.deletes != 1 {
		t.Fatalf("retry upload was not discarded: %+v", storage)
	}
	if len(tx.proofs) != 0 || len(tx.history) != 0 || len(tx.events) != 0 || tx.commits != 0 {
		t.Fatalf("retry wrote rows: %+v", tx)
	}
}

func TestConfirmPickupRejectsUnassignedCourier(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	other := "00000000-0000-4000-8000-000000000099"
	tx.owner = &other
	storage := &proofTestStorage{}
	_, err := NewWorkflowService(db, storage, 0).ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if !errors.Is(err, ErrForbidden) || storage.saves != 0 || tx.writes != 0 {
		t.Fatalf("err = %v storage = %+v tx = %+v", err, storage, tx)
	}
}

func TestConfirmPickupValidatesPhoto(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	service := NewWorkflowService(db, &proofTestStorage{}, 0)
	if _, err := service.ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
	}); !errors.Is(err, ErrPhotoRequired) {
		t.Fatalf("missing photo: %v", err)
	}
	_, err := service.ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: []byte("not an image"), MIMEType: "image/png"},
	})
	if !errors.Is(err, ErrProofUnsupported) {
		t.Fatalf("invalid photo: %v", err)
	}
	if tx.writes != 0 || tx.commits != 0 {
		t.Fatal("validation wrote to the database")
	}
}

func TestConfirmPickupRejectsWrongStatus(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusOutForDelivery)
	_, err := NewWorkflowService(db, &proofTestStorage{}, 0).ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if !errors.Is(err, statemachine.ErrInvalidTransition) || tx.writes != 0 {
		t.Fatalf("err = %v tx = %+v", err, tx)
	}
}

func TestConfirmPickupCleansUpWhenTransactionFails(t *testing.T) {
	cause := errors.New("database failed")
	for _, stage := range []string{"proof", "second-update", "history", "outbox", "commit"} {
		t.Run(stage, func(t *testing.T) {
			db, tx := newWorkflowFixture(statemachine.StatusCreated)
			tx.fail, tx.cause = stage, cause
			storage := &proofTestStorage{}
			_, err := NewWorkflowService(db, storage, 0).ConfirmPickup(t.Context(), PickupInput{
				ParcelID: workflowParcelID, CourierID: statusCourierID,
				Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
			})
			if !errors.Is(err, cause) {
				t.Fatalf("err = %v", err)
			}
			wantCommits := 0
			if stage == "commit" {
				wantCommits = 1
			}
			if storage.saves != 1 || storage.deletes != 1 || tx.commits != wantCommits {
				t.Fatalf("storage = %+v tx = %+v", storage, tx)
			}
		})
	}
}

func TestConfirmPickupStopsAtProofQuota(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	tx.proofCount = 2
	storage := &proofTestStorage{}
	_, err := NewWorkflowService(db, storage, 2).ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if !errors.Is(err, ErrProofLimitReached) || len(tx.proofs) != 0 || storage.deletes != 1 {
		t.Fatalf("err = %v proofs = %+v storage = %+v", err, tx.proofs, storage)
	}
}

func TestConfirmDeliveryCommitsProofRecipientAndTransition(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusOutForDelivery)
	storage := &proofTestStorage{}
	result, err := NewWorkflowService(db, storage, 0).ConfirmDelivery(t.Context(), DeliveryInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		RecipientName: " Amali Fernando ", Note: " Handed over ",
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != statemachine.StatusDelivered ||
		result.PreviousStatus != statemachine.StatusOutForDelivery || result.AlreadyApplied {
		t.Fatalf("result = %+v", result)
	}
	if len(result.History) != 1 || result.History[0].Status != repository.ParcelStatus(statemachine.StatusDelivered) {
		t.Fatalf("history = %+v", result.History)
	}
	if result.Proof == nil || result.Proof.Kind != ProofKindDelivery ||
		result.Proof.RecipientName == nil || *result.Proof.RecipientName != "Amali Fernando" {
		t.Fatalf("proof = %+v", result.Proof)
	}
	if len(tx.proofs) != 1 || tx.proofs[0].kind != ProofKindDelivery ||
		tx.proofs[0].recipient == nil || *tx.proofs[0].recipient != "Amali Fernando" ||
		tx.deliveredTo == nil || *tx.deliveredTo != "Amali Fernando" {
		t.Fatalf("proof row = %+v deliveredTo = %v", tx.proofs, tx.deliveredTo)
	}
	if len(tx.events) != 1 || tx.commits != 1 || storage.saves != 1 || storage.deletes != 0 {
		t.Fatalf("tx = %+v storage = %+v", tx, storage)
	}
	var payload map[string]any
	if err := json.Unmarshal(tx.events[0], &payload); err != nil {
		t.Fatal(err)
	}
	if payload["status"] != "DELIVERED" || payload["previous_status"] != "OUT_FOR_DELIVERY" {
		t.Fatalf("event = %s", tx.events[0])
	}
}

func TestConfirmDeliveryValidation(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusOutForDelivery)
	service := NewWorkflowService(db, &proofTestStorage{}, 0)
	photo := &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"}
	if _, err := service.ConfirmDelivery(t.Context(), DeliveryInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID, Photo: photo,
	}); !errors.Is(err, statemachine.ErrRecipientRequired) {
		t.Fatalf("missing recipient: %v", err)
	}
	if _, err := service.ConfirmDelivery(t.Context(), DeliveryInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID, RecipientName: "Amali",
	}); !errors.Is(err, ErrPhotoRequired) {
		t.Fatalf("missing photo: %v", err)
	}
	if tx.writes != 0 || tx.commits != 0 {
		t.Fatal("validation wrote to the database")
	}
}

func TestConfirmDeliveryRetryIsIdempotent(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusDelivered)
	storage := &proofTestStorage{}
	result, err := NewWorkflowService(db, storage, 0).ConfirmDelivery(t.Context(), DeliveryInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID, RecipientName: "Amali",
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if !result.AlreadyApplied || result.Status != statemachine.StatusDelivered {
		t.Fatalf("result = %+v", result)
	}
	if storage.saves != 0 || tx.writes != 0 {
		t.Fatalf("retry wrote: storage=%+v tx=%+v", storage, tx)
	}
}

func TestConfirmDeliveryRejectsWrongStatus(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusInTransit)
	_, err := NewWorkflowService(db, &proofTestStorage{}, 0).ConfirmDelivery(t.Context(), DeliveryInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID, RecipientName: "Amali",
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if !errors.Is(err, statemachine.ErrInvalidTransition) || tx.writes != 0 {
		t.Fatalf("err = %v tx = %+v", err, tx)
	}
}

func TestWorkflowUnavailableWithoutStorage(t *testing.T) {
	db, _ := newWorkflowFixture(statemachine.StatusCreated)
	if _, err := NewWorkflowService(db, nil, 0).ConfirmPickup(t.Context(), PickupInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("pickup: %v", err)
	}
	if _, err := NewWorkflowService(db, nil, 0).ConfirmDelivery(t.Context(), DeliveryInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("delivery: %v", err)
	}
	if _, err := (*WorkflowService)(nil).ConfirmPickup(t.Context(), PickupInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("nil service: %v", err)
	}
}

func TestWorkflowRunCountersAreRecorded(t *testing.T) {
	db, tx := newWorkflowFixture(statemachine.StatusCreated)
	service := NewWorkflowService(db, &proofTestStorage{}, 0).WithMaxPhotoBytes(1024)
	if service.maxPhotoBytes != 1024 {
		t.Fatalf("maxPhotoBytes = %d", service.maxPhotoBytes)
	}
	_, err := service.ConfirmPickup(t.Context(), PickupInput{
		ParcelID: workflowParcelID, CourierID: statusCourierID,
		Photo: &ProofPhoto{Data: validProofPNG(t), MIMEType: "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if tx.rollbacks != 1 || !reflect.DeepEqual(tx.secondStatus, statemachine.StatusInTransit) {
		t.Fatalf("tx = %+v", tx)
	}
}
