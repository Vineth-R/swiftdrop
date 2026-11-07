//go:build integration && postgres

package courier

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/outbox"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func insertStatusParcel(t *testing.T, pool *postgres.Pool, status, courierID string, merchant bool) string {
	t.Helper()
	id := parceltest.UUID(t)
	var owner any
	if courierID != "" {
		owner = courierID
	}
	var customer, merchantID any = statusCourierID, nil
	tracking := "SDK" + strings.ReplaceAll(id, "-", "")[:8]
	if merchant {
		customer, merchantID = nil, statusCourierID
		tracking = "SDN" + strings.ReplaceAll(id, "-", "")[:8]
	}
	_, err := pool.Exec(t.Context(), `INSERT INTO parcel.parcels (
		id, tracking_number, customer_user_id, merchant_id, sender_name, sender_phone,
		pickup_address, pickup_city, pickup_zone_id, recipient_name, recipient_phone,
		dropoff_address, dropoff_city, dropoff_zone_id, weight_kg, size_category,
		status, current_courier_id, price_amount, price_breakdown, created_by_user_id,
		delivered_at, delivered_to_name, failed_at, failure_reason
	) VALUES ($1,$2,$3,$4,'Sender','+94770000001','Pickup','Colombo',$5,
		'Original Recipient','+94770000002','Dropoff','Kandy',$5,2,'SMALL',$6::text,$7,550,
		'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',$5,
		CASE WHEN $6='DELIVERED' THEN now() END,
		CASE WHEN $6='DELIVERED' THEN 'Prior recipient' END,
		CASE WHEN $6='FAILED' THEN now() END,
		CASE WHEN $6='FAILED' THEN 'Prior failure' END)`,
		id, tracking, customer, merchantID, statusCourierID, status, owner)
	if err != nil {
		t.Fatal(err)
	}
	if owner != nil {
		if _, err := pool.Exec(t.Context(), `INSERT INTO parcel.parcel_assignments
			(parcel_id,courier_id,assigned_by) VALUES ($1,$2,$2)`, id, owner); err != nil {
			t.Fatal(err)
		}
	}
	return id
}

func assertStatusRows(t *testing.T, pool *postgres.Pool, id, status string, count int) {
	t.Helper()
	var got string
	var history, events int
	err := pool.QueryRow(t.Context(), `SELECT status,
		(SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id=p.id),
		(SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id=p.id AND event_type='parcel.status_changed')
		FROM parcel.parcels p WHERE id=$1`, id).Scan(&got, &history, &events)
	if err != nil || got != status || history != count || events != count {
		t.Fatalf("status/history/outbox = %s/%d/%d, want %s/%d/%d; error=%v", got, history, events, status, count, count, err)
	}
}

func TestCourierStatusDatabase(t *testing.T) {
	database := parceltest.New(t)
	pool := database.Pool
	repo := NewRepository(pool)

	t.Run("full lifecycle stores delivery fields and matching event history", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		previous := statemachine.StatusCreated
		for i, next := range []statemachine.ParcelStatus{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
			result, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id,
				CourierID: statusCourierID, Status: next, RecipientName: " Accepted By ", Note: "At door"})
			if err != nil || result.PreviousStatus != previous || result.UpdatedAt.IsZero() {
				t.Fatalf("%+v %v", result, err)
			}
			assertStatusRows(t, pool, id, string(next), i+1)
			var historyPrevious, actor, role, note string
			var historyTime time.Time
			err = pool.QueryRow(t.Context(), `SELECT previous_status,actor_user_id,actor_role,note,created_at
				FROM parcel.parcel_status_history WHERE parcel_id=$1 AND new_status=$2`, id, next).
				Scan(&historyPrevious, &actor, &role, &note, &historyTime)
			if err != nil || historyPrevious != string(previous) || actor != statusCourierID || role != "COURIER" || note != "At door" || !historyTime.Equal(result.UpdatedAt) {
				t.Fatalf("history %s/%s/%s/%s/%s; %v", historyPrevious, actor, role, note, historyTime, err)
			}
			var data map[string]any
			var raw []byte
			var key, eventStatus string
			var attempts int
			var eventTime time.Time
			err = pool.QueryRow(t.Context(), `SELECT payload,routing_key,status,attempts,created_at
				FROM parcel.outbox_events WHERE aggregate_id=$1 AND payload->>'status'=$2`, id, next).
				Scan(&raw, &key, &eventStatus, &attempts, &eventTime)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			if key != "parcel.status.changed" || eventStatus != "PENDING" || attempts != 0 || !eventTime.Equal(result.UpdatedAt) {
				t.Fatalf("outbox %s/%s/%d/%s", key, eventStatus, attempts, eventTime)
			}
			if data["parcel_id"] != id || data["status"] != string(next) || data["previous_status"] != string(previous) || data["customer_user_id"] != statusCourierID || data["actor_user_id"] != statusCourierID {
				t.Fatalf("event data %s", raw)
			}
			for _, forbidden := range []string{"recipient_name", "recipientName", "phone", "address", "event_type", "event_id"} {
				if strings.Contains(string(raw), forbidden) {
					t.Fatalf("unexpected event field %s", forbidden)
				}
			}
			previous = next
		}
		var name, original string
		var delivered time.Time
		var failed *time.Time
		var reason *string
		if err := pool.QueryRow(t.Context(), `SELECT delivered_to_name,recipient_name,delivered_at,failed_at,failure_reason
			FROM parcel.parcels WHERE id=$1`, id).Scan(&name, &original, &delivered, &failed, &reason); err != nil {
			t.Fatal(err)
		}
		if name != "Accepted By" || original != "Original Recipient" || delivered.IsZero() || failed != nil || reason != nil {
			t.Fatal("incorrect terminal delivery fields")
		}
	})

	for _, from := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY"} {
		t.Run("failure from "+from, func(t *testing.T) {
			id := insertStatusParcel(t, pool, from, statusCourierID, true)
			result, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id, CourierID: statusCourierID,
				Status: "FAILED", FailureReason: " Recipient unavailable ", Note: strings.Repeat("名", 500)})
			if err != nil {
				t.Fatal(err)
			}
			assertStatusRows(t, pool, id, "FAILED", 1)
			var failed time.Time
			var reason, historyReason string
			var recipient *string
			var raw []byte
			if err := pool.QueryRow(t.Context(), `SELECT p.failed_at,p.failure_reason,p.delivered_to_name,h.failure_reason,e.payload
				FROM parcel.parcels p JOIN parcel.parcel_status_history h ON h.parcel_id=p.id
				JOIN parcel.outbox_events e ON e.aggregate_id=p.id WHERE p.id=$1`, id).
				Scan(&failed, &reason, &recipient, &historyReason, &raw); err != nil {
				t.Fatal(err)
			}
			var data map[string]any
			if err := json.Unmarshal(raw, &data); err != nil {
				t.Fatal(err)
			}
			if !failed.Equal(result.UpdatedAt) || reason != "Recipient unavailable" || historyReason != reason || recipient != nil || data["failure_reason"] != reason || data["merchant_id"] != statusCourierID || data["customer_user_id"] != nil {
				t.Fatalf("failure fields or merchant routing: failed=%v updated=%v equal=%t reason=%q historyReason=%q recipient=%v dataReason=%v merchant=%v customer=%v payload=%s",
					failed, result.UpdatedAt, failed.Equal(result.UpdatedAt), reason, historyReason, recipient,
					data["failure_reason"], data["merchant_id"], data["customer_user_id"], raw)
			}
		})
	}

	for _, tc := range []struct {
		name, from, owner, to, recipient, reason, note string
		want                                           error
	}{
		{name: "foreign before transition", from: "CREATED", owner: statusParcelID, to: "DELIVERED", want: statemachine.ErrForbidden},
		{name: "unassigned", from: "CREATED", to: "PICKED_UP", want: statemachine.ErrForbidden},
		{name: "skip before recipient", from: "CREATED", owner: statusCourierID, to: "DELIVERED", want: statemachine.ErrInvalidTransition},
		{name: "delivered is terminal", from: "DELIVERED", owner: statusCourierID, to: "FAILED", reason: "Unavailable", want: statemachine.ErrInvalidTransition},
		{name: "failed is terminal", from: "FAILED", owner: statusCourierID, to: "PICKED_UP", want: statemachine.ErrInvalidTransition},
		{name: "repeat", from: "PICKED_UP", owner: statusCourierID, to: "PICKED_UP", want: statemachine.ErrInvalidTransition},
		{name: "backward", from: "IN_TRANSIT", owner: statusCourierID, to: "PICKED_UP", want: statemachine.ErrInvalidTransition},
		{name: "recipient missing", from: "OUT_FOR_DELIVERY", owner: statusCourierID, to: "DELIVERED", want: statemachine.ErrRecipientRequired},
		{name: "reason short", from: "CREATED", owner: statusCourierID, to: "FAILED", reason: " abcd ", want: statemachine.ErrFailureReasonRequired},
		{name: "reason long", from: "CREATED", owner: statusCourierID, to: "FAILED", reason: strings.Repeat("名", 256), want: statemachine.ErrFailureReasonRequired},
		{name: "note long", from: "CREATED", owner: statusCourierID, to: "PICKED_UP", note: strings.Repeat("名", 501), want: statemachine.ErrValidationError},
	} {
		t.Run(tc.name, func(t *testing.T) {
			id := insertStatusParcel(t, pool, tc.from, tc.owner, false)
			_, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id, CourierID: statusCourierID,
				Status: statemachine.ParcelStatus(tc.to), RecipientName: tc.recipient, FailureReason: tc.reason, Note: tc.note})
			if !errors.Is(err, tc.want) {
				t.Fatalf("error=%v want=%v", err, tc.want)
			}
			assertStatusRows(t, pool, id, tc.from, 0)
		})
	}
	t.Run("inconsistent assignment does not authorize a status write", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		if _, err := pool.Exec(t.Context(), `DELETE FROM parcel.parcel_assignments WHERE parcel_id=$1`, id); err != nil {
			t.Fatal(err)
		}
		_, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id, CourierID: statusCourierID, Status: "PICKED_UP"})
		if !errors.Is(err, ErrInconsistentAssignment) {
			t.Fatalf("error=%v want=%v", err, ErrInconsistentAssignment)
		}
		assertStatusRows(t, pool, id, "CREATED", 0)
	})
	for _, table := range []string{"parcel_status_history", "outbox_events"} {
		t.Run("rollback on "+table+" failure", func(t *testing.T) {
			id := insertStatusParcel(t, pool, "OUT_FOR_DELIVERY", statusCourierID, false)
			column := "parcel_id"
			if table == "outbox_events" {
				column = "aggregate_id"
			}
			// Fixture-only constraint, scoped to a generated UUID and disposable DB.
			constraint := fmt.Sprintf("ALTER TABLE parcel.%s ADD CONSTRAINT m7_reject_status CHECK (%s <> '%s'::uuid)", table, column, id)
			if _, err := pool.Exec(t.Context(), constraint); err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() {
				if _, err := pool.Exec(context.Background(), "ALTER TABLE parcel."+table+" DROP CONSTRAINT m7_reject_status"); err != nil {
					t.Error(err)
				}
			})
			if _, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id, CourierID: statusCourierID,
				Status: "DELIVERED", RecipientName: "Accepted By"}); err == nil {
				t.Fatal("expected injected persistence failure")
			}
			assertStatusRows(t, pool, id, "OUT_FOR_DELIVERY", 0)
			var name *string
			var delivered *time.Time
			if err := pool.QueryRow(t.Context(), `SELECT delivered_to_name,delivered_at FROM parcel.parcels WHERE id=$1`, id).Scan(&name, &delivered); err != nil || name != nil || delivered != nil {
				t.Fatalf("delivery fields escaped rollback: %v", err)
			}
		})
	}
	t.Run("concurrent identical requests commit once", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		start := make(chan struct{})
		errorsCh := make(chan error, 2)
		for range 2 {
			go func() {
				<-start
				_, err := repo.UpdateStatus(ctx, StatusInput{ParcelID: id, CourierID: statusCourierID, Status: "PICKED_UP"})
				errorsCh <- err
			}()
		}
		close(start)
		success, conflict := 0, 0
		for range 2 {
			switch err := <-errorsCh; {
			case err == nil:
				success++
			case errors.Is(err, statemachine.ErrInvalidTransition):
				conflict++
			default:
				t.Fatalf("unexpected concurrent error: %v", err)
			}
		}
		if success != 1 || conflict != 1 {
			t.Fatalf("success/conflict=%d/%d", success, conflict)
		}
		assertStatusRows(t, pool, id, "PICKED_UP", 1)
	})
	t.Run("uncommitted status history and event are invisible to relay", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		tx, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		ready, release := make(chan struct{}), make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			gated := statusCommitGate{Tx: tx, ready: ready, release: release}
			_, err := NewRepository(statusDB{tx: gated}).UpdateStatus(ctx,
				StatusInput{ParcelID: id, CourierID: statusCourierID, Status: "PICKED_UP"})
			finished <- err
		}()
		select {
		case <-ready:
		case <-ctx.Done():
			t.Fatal("status transaction did not reach commit")
		}
		assertStatusRows(t, pool, id, "CREATED", 0)
		store := outbox.NewSQLStore(pool)
		_, err = store.ProcessPending(ctx, 100, func(event outbox.Event) error {
			if event.AggregateID != nil && *event.AggregateID == id {
				t.Error("relay observed an uncommitted status event")
			}
			return outbox.ErrSkip
		})
		if err != nil {
			t.Fatal(err)
		}
		close(release)
		if err := <-finished; err != nil {
			t.Fatal(err)
		}
		assertStatusRows(t, pool, id, "PICKED_UP", 1)
	})
	t.Run("M6 reassignment wins before blocked status update", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
		defer cancel()
		locked, err := pool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer func() {
			if err := locked.Rollback(context.Background()); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
				t.Error(err)
			}
		}()
		if _, err := locked.Exec(ctx, "SELECT id FROM parcel.parcels WHERE id=$1 FOR UPDATE", id); err != nil {
			t.Fatal(err)
		}
		attempted := make(chan struct{})
		finished := make(chan error, 1)
		go func() {
			_, err := NewRepository(statusObservedDB{DB: pool, attempted: attempted}).UpdateStatus(ctx,
				StatusInput{ParcelID: id, CourierID: statusCourierID, Status: "PICKED_UP"})
			finished <- err
		}()
		select {
		case <-attempted:
		case <-ctx.Done():
			t.Fatal("status request did not attempt its lock")
		}
		// Exercise M6's unchanged transaction through its normal repository API.
		dispatchRepo := dispatch.NewRepository(statusDispatchDB{Pool: pool, tx: locked})
		_, err = dispatchRepo.AssignCourier(ctx, dispatch.AssignInput{ParcelID: id,
			CourierID: statusParcelID, ActorUserID: statusCourierID})
		if err != nil {
			t.Fatal(err)
		}
		if err := <-finished; !errors.Is(err, statemachine.ErrForbidden) {
			t.Fatalf("former courier retained write access: %v", err)
		}
		assertStatusRows(t, pool, id, "CREATED", 0)
	})
	t.Run("relay uses canonical envelope and publish failure cannot undo status", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		if _, err := repo.UpdateStatus(t.Context(), StatusInput{ParcelID: id, CourierID: statusCourierID, Status: "PICKED_UP"}); err != nil {
			t.Fatal(err)
		}
		publisher := &statusFailingPublisher{parcelID: id}
		relay := outbox.NewRelay(outbox.NewSQLStore(pool), publisher, slog.New(slog.NewTextHandler(io.Discard, nil)))
		if _, err := relay.PollOnce(t.Context()); err != nil {
			t.Fatal(err)
		}
		if publisher.problem != "" || !publisher.seen {
			t.Fatalf("relay compatibility: seen=%t problem=%s", publisher.seen, publisher.problem)
		}
		assertStatusRows(t, pool, id, "PICKED_UP", 1)
		var state string
		var attempts int
		if err := pool.QueryRow(t.Context(), `SELECT status,attempts FROM parcel.outbox_events WHERE aggregate_id=$1`, id).Scan(&state, &attempts); err != nil || state != "PENDING" || attempts != 1 {
			t.Fatalf("failed publish did not remain pending: %s/%d %v", state, attempts, err)
		}
	})
}

type statusObservedDB struct {
	DB
	attempted chan struct{}
}

func (db statusObservedDB) Begin(ctx context.Context) (pgx.Tx, error) {
	tx, err := db.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	return statusObservedTx{Tx: tx, attempted: db.attempted}, nil
}

type statusObservedTx struct {
	pgx.Tx
	attempted chan struct{}
}

func (tx statusObservedTx) QueryRow(ctx context.Context, sql string, args ...any) pgx.Row {
	if strings.Contains(sql, "FOR UPDATE") {
		close(tx.attempted)
	}
	return tx.Tx.QueryRow(ctx, sql, args...)
}

type statusDispatchDB struct {
	*postgres.Pool
	tx pgx.Tx
}

func (db statusDispatchDB) Begin(context.Context) (pgx.Tx, error) { return db.tx, nil }

type statusCommitGate struct {
	pgx.Tx
	ready   chan struct{}
	release chan struct{}
}

func (tx statusCommitGate) Commit(ctx context.Context) error {
	close(tx.ready)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-tx.release:
		return tx.Tx.Commit(ctx)
	}
}

type statusFailingPublisher struct {
	parcelID, problem string
	seen              bool
}

func (p *statusFailingPublisher) Publish(_ context.Context, exchange, key string, body []byte) error {
	var envelope messaging.Envelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		p.problem = err.Error()
		return err
	}
	var data map[string]any
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		p.problem = err.Error()
		return err
	}
	if data["parcel_id"] == p.parcelID {
		p.seen = true
		if exchange != "swiftdrop.events" || key != "parcel.status_changed.v1" || envelope.Type != key || envelope.Version != 1 || envelope.Producer != "parcel" || envelope.ID == "" {
			p.problem = "incorrect canonical envelope or routing"
		}
	}
	return errors.New("broker unavailable")
}
