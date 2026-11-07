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

const statusCourierID = "00000000-0000-4000-8000-000000000071"
const statusParcelID = "00000000-0000-4000-8000-000000000072"

type statusDB struct {
	tx  pgx.Tx
	err error
}

func (db statusDB) Begin(ctx context.Context) (pgx.Tx, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return db.tx, db.err
}

type statusRow struct {
	values []any
	err    error
}

func (row statusRow) Scan(dest ...any) error {
	if row.err != nil {
		return row.err
	}
	for i, value := range row.values {
		reflect.ValueOf(dest[i]).Elem().Set(reflect.ValueOf(value))
	}
	return nil
}

type statusTx struct {
	pgx.Tx
	from                       statemachine.ParcelStatus
	owner, customer, merchant  *string
	assignmentCurrent          bool
	fail                       string
	cause                      error
	commits, rollbacks, writes int
	ctx                        context.Context
	cleanupCancelled           bool
	payload                    []byte
	historyArgs, updateArgs    []any
	stamp                      time.Time
}

func (tx *statusTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	tx.ctx = ctx
	switch {
	case strings.Contains(sql, "FOR UPDATE"):
		if tx.fail == "lock" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{"SDK7M2Q9XP4", tx.from, tx.owner, tx.customer, tx.merchant}}
	case strings.Contains(sql, "SELECT EXISTS ("):
		if tx.fail == "assignment" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{tx.assignmentCurrent}}
	case strings.Contains(sql, "UPDATE parcel.parcels"):
		tx.writes++
		tx.updateArgs = args
		if tx.fail == "update" {
			return statusRow{err: tx.cause}
		}
		return statusRow{values: []any{tx.stamp}}
	case strings.Contains(sql, "INSERT INTO parcel.parcel_status_history"):
		tx.writes++
		tx.historyArgs = args
		if tx.fail == "history" {
			return statusRow{err: tx.cause}
		}
		createdAt := tx.stamp
		if params, ok := args[8].(*time.Time); ok && params != nil {
			createdAt = *params
		}
		return statusRow{values: []any{args[0].(string), args[1].(string),
			args[2].(*repository.ParcelStatus), args[3].(repository.ParcelStatus),
			args[4].(*string), args[5].(*string), args[6].(*string), args[7].(*string), createdAt}}
	default:
		return statusRow{err: errors.New("unexpected query")}
	}
}

func (tx *statusTx) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	tx.ctx = ctx
	tx.writes++
	tx.payload = args[0].([]byte)
	if tx.fail == "outbox" {
		return pgconn.CommandTag{}, tx.cause
	}
	return pgconn.NewCommandTag("INSERT 0 1"), nil
}

func (tx *statusTx) Commit(ctx context.Context) error {
	tx.ctx = ctx
	tx.commits++
	if tx.fail == "commit" {
		return tx.cause
	}
	return nil
}

func (tx *statusTx) Rollback(ctx context.Context) error {
	tx.rollbacks++
	tx.cleanupCancelled = ctx.Err() != nil
	if tx.fail == "rollback" {
		return tx.cause
	}
	return pgx.ErrTxClosed
}

func newStatusTx(from statemachine.ParcelStatus) *statusTx {
	owner, customer := statusCourierID, statusCourierID
	return &statusTx{from: from, owner: &owner, customer: &customer, assignmentCurrent: true,
		stamp: time.Date(2026, 11, 7, 12, 0, 0, 0, time.FixedZone("test", 19800))}
}

func TestUpdateStatusTransitions(t *testing.T) {
	for _, from := range statemachine.AllStatuses() {
		for _, to := range statemachine.AllStatuses() {
			t.Run(string(from)+" to "+string(to), func(t *testing.T) {
				tx := newStatusTx(from)
				ctx := t.Context()
				result, err := NewRepository(statusDB{tx: tx}).UpdateStatus(ctx, StatusInput{
					ParcelID: statusParcelID, CourierID: statusCourierID, Status: to,
					RecipientName: " Test Recipient ", FailureReason: " Recipient unavailable ", Note: "Status note",
				})
				if !statemachine.CanTransition(from, to) {
					if !errors.Is(err, statemachine.ErrInvalidTransition) || tx.writes != 0 || tx.commits != 0 {
						t.Fatalf("invalid transition: err=%v writes=%d commits=%d", err, tx.writes, tx.commits)
					}
					return
				}
				if err != nil || tx.commits != 1 || tx.writes != 3 || tx.ctx != ctx || tx.rollbacks != 1 {
					t.Fatalf("successful transition: err=%v tx=%+v", err, tx)
				}
				if result.PreviousStatus != from || result.Status != to || result.ParcelID != statusParcelID || result.TrackingNumber != "SDK7M2Q9XP4" {
					t.Fatalf("result: %+v", result)
				}
				if result.UpdatedAt.Location() != time.UTC || !result.UpdatedAt.Equal(tx.stamp) {
					t.Fatalf("timestamp: %v", result.UpdatedAt)
				}
				var data map[string]any
				if err := json.Unmarshal(tx.payload, &data); err != nil {
					t.Fatal(err)
				}
				if data["actor_user_id"] != statusCourierID || data["customer_user_id"] != statusCourierID || data["previous_status"] != string(from) || data["status"] != string(to) || data["note"] != "Status note" {
					t.Fatalf("event: %s", tx.payload)
				}
				if to == statemachine.StatusFailed && data["failure_reason"] != "Recipient unavailable" {
					t.Fatal("missing normalized failure reason")
				}
				if to != statemachine.StatusFailed && data["failure_reason"] != nil {
					t.Fatal("irrelevant failure reason persisted")
				}
				for _, forbidden := range []string{"recipient", "phone", "address", "event_type", "event_id"} {
					if strings.Contains(string(tx.payload), forbidden) {
						t.Fatalf("unexpected event field %s", forbidden)
					}
				}
				if *tx.historyArgs[4].(*string) != statusCourierID || *tx.historyArgs[5].(*string) != "COURIER" {
					t.Fatal("history did not record authenticated actor")
				}
			})
		}
	}
}

func TestUpdateStatusValidationOrderAndBounds(t *testing.T) {
	for _, tc := range []struct {
		name, owner, actor      string
		from, to                statemachine.ParcelStatus
		recipient, reason, note string
		want                    error
	}{
		{name: "foreign before transition and fields", owner: "another", actor: statusCourierID, from: "CREATED", to: "DELIVERED", want: statemachine.ErrForbidden},
		{name: "unassigned", actor: statusCourierID, from: "CREATED", to: "PICKED_UP", want: statemachine.ErrForbidden},
		{name: "missing actor", owner: statusCourierID, from: "CREATED", to: "PICKED_UP", want: statemachine.ErrForbidden},
		{name: "transition before recipient", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "DELIVERED", want: statemachine.ErrInvalidTransition},
		{name: "unknown status", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "OTHER", want: statemachine.ErrUnknownStatus},
		{name: "recipient blank", owner: statusCourierID, actor: statusCourierID, from: "OUT_FOR_DELIVERY", to: "DELIVERED", recipient: "  ", want: statemachine.ErrRecipientRequired},
		{name: "recipient long", owner: statusCourierID, actor: statusCourierID, from: "OUT_FOR_DELIVERY", to: "DELIVERED", recipient: strings.Repeat("名", 151), want: statemachine.ErrValidationError},
		{name: "recipient Unicode limit", owner: statusCourierID, actor: statusCourierID, from: "OUT_FOR_DELIVERY", to: "DELIVERED", recipient: strings.Repeat("名", 150)},
		{name: "reason missing", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "FAILED", want: statemachine.ErrFailureReasonRequired},
		{name: "reason short after trimming", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "FAILED", reason: " abcd ", want: statemachine.ErrFailureReasonRequired},
		{name: "reason too long", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "FAILED", reason: strings.Repeat("名", 256), want: statemachine.ErrFailureReasonRequired},
		{name: "reason minimum", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "FAILED", reason: " abcde "},
		{name: "reason Unicode maximum", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "FAILED", reason: strings.Repeat("名", 255)},
		{name: "note too long", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "PICKED_UP", note: strings.Repeat("名", 501), want: statemachine.ErrValidationError},
		{name: "note Unicode maximum", owner: statusCourierID, actor: statusCourierID, from: "CREATED", to: "PICKED_UP", note: strings.Repeat("名", 500)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tx := newStatusTx(tc.from)
			tx.owner = &tc.owner
			_, err := NewRepository(statusDB{tx: tx}).UpdateStatus(t.Context(), StatusInput{
				ParcelID: statusParcelID, CourierID: tc.actor, Status: tc.to,
				RecipientName: tc.recipient, FailureReason: tc.reason, Note: tc.note,
			})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error = %v, want %v", err, tc.want)
			}
			if tc.want != nil && (tx.writes != 0 || tx.commits != 0) {
				t.Fatal("validation failure wrote or committed")
			}
		})
	}
}

func TestUpdateStatusTransactionFailures(t *testing.T) {
	cause := errors.New("database failed")
	for _, stage := range []string{"begin", "lock", "assignment", "update", "history", "outbox", "commit"} {
		t.Run(stage, func(t *testing.T) {
			tx := newStatusTx(statemachine.StatusCreated)
			tx.fail, tx.cause = stage, cause
			db := statusDB{tx: tx}
			if stage == "begin" {
				db.err = cause
			}
			result, err := NewRepository(db).UpdateStatus(t.Context(), StatusInput{
				ParcelID: statusParcelID, CourierID: statusCourierID, Status: statemachine.StatusPickedUp,
			})
			if !errors.Is(err, cause) {
				t.Fatalf("error = %v", err)
			}
			if result != (StatusResult{}) {
				t.Fatal("failed write returned a success result")
			}
			if stage != "begin" && (tx.rollbacks != 1 || tx.cleanupCancelled) {
				t.Fatal("transaction was not cleaned up")
			}
			if stage == "commit" {
				if tx.commits != 1 {
					t.Fatal("commit was not attempted")
				}
			} else if tx.commits != 0 {
				t.Fatal("committed a failed transaction")
			}
		})
	}
	t.Run("unexpected rollback error after commit is not a failure", func(t *testing.T) {
		tx := newStatusTx(statemachine.StatusCreated)
		tx.fail, tx.cause = "rollback", cause
		result, err := NewRepository(statusDB{tx: tx}).UpdateStatus(t.Context(), StatusInput{
			ParcelID: statusParcelID, CourierID: statusCourierID, Status: statemachine.StatusPickedUp,
		})
		if err != nil {
			t.Fatalf("committed update reported an error: %v", err)
		}
		if result.Status != statemachine.StatusPickedUp || result.ParcelID != statusParcelID || tx.commits != 1 || tx.rollbacks != 1 {
			t.Fatalf("result=%+v tx=%+v", result, tx)
		}
	})
	if _, err := NewRepository(nil).UpdateStatus(t.Context(), StatusInput{}); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("unconfigured: %v", err)
	}
	tx := newStatusTx(statemachine.StatusCreated)
	tx.fail, tx.cause = "lock", pgx.ErrNoRows
	if _, err := NewRepository(statusDB{tx: tx}).UpdateStatus(t.Context(), StatusInput{}); !errors.Is(err, ErrNotFound) {
		t.Fatalf("missing parcel: %v", err)
	}
	tx = newStatusTx(statemachine.StatusCreated)
	tx.assignmentCurrent = false
	if _, err := NewRepository(statusDB{tx: tx}).UpdateStatus(t.Context(), StatusInput{
		ParcelID: statusParcelID, CourierID: statusCourierID, Status: statemachine.StatusPickedUp,
	}); !errors.Is(err, ErrInconsistentAssignment) {
		t.Fatalf("inconsistent assignment: %v", err)
	}
	if tx.writes != 0 || tx.commits != 0 {
		t.Fatal("inconsistent assignment wrote or committed")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := NewRepository(statusDB{tx: tx}).UpdateStatus(ctx, StatusInput{}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled: %v", err)
	}
}
