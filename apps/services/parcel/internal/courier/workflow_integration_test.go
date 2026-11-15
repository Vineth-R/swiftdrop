//go:build integration && postgres

package courier

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/statemachine"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func workflowPhoto(t *testing.T, mimeType string) *ProofPhoto {
	t.Helper()
	return &ProofPhoto{Data: proofIntegrationImage(t, mimeType), MIMEType: mimeType}
}

func TestCourierWorkflowDatabase(t *testing.T) {
	database := parceltest.New(t)
	pool := database.Pool
	storage, directory := proofIntegrationStorage(t)
	service := NewWorkflowService(pool, storage, 0)

	t.Run("pickup commits proof both history rows and both events", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		result, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: statusCourierID, Note: "Collected from sender",
			Photo: workflowPhoto(t, "image/png"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.PreviousStatus != statemachine.StatusCreated ||
			result.Status != statemachine.StatusInTransit || result.AlreadyApplied ||
			len(result.History) != 2 || result.Proof == nil || result.Proof.Kind != ProofKindPickup {
			t.Fatalf("result = %+v", result)
		}

		var status string
		if err := pool.QueryRow(t.Context(), `SELECT status
			FROM parcel.parcels WHERE id=$1`, id).Scan(&status); err != nil || status != "IN_TRANSIT" {
			t.Fatalf("parcel = %s %v", status, err)
		}
		rows, err := pool.Query(t.Context(), `SELECT previous_status, new_status, note, actor_role
			FROM parcel.parcel_status_history WHERE parcel_id=$1 ORDER BY created_at, id`, id)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()
		type history struct{ previous, status, note, role *string }
		var historyRows []history
		for rows.Next() {
			var entry history
			if err := rows.Scan(&entry.previous, &entry.status, &entry.note, &entry.role); err != nil {
				t.Fatal(err)
			}
			historyRows = append(historyRows, entry)
		}
		if len(historyRows) != 2 || *historyRows[0].previous != "CREATED" ||
			*historyRows[0].status != "PICKED_UP" || *historyRows[1].previous != "PICKED_UP" ||
			*historyRows[1].status != "IN_TRANSIT" || *historyRows[0].note != "Collected from sender" ||
			historyRows[1].note != nil || *historyRows[0].role != "COURIER" {
			t.Fatalf("history = %+v", historyRows)
		}
		var eventCount int
		if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.outbox_events
			WHERE aggregate_id=$1 AND event_type='parcel.status_changed'`, id).Scan(&eventCount); err != nil || eventCount != 2 {
			t.Fatalf("events = %d %v", eventCount, err)
		}
		var kind, capturedBy, path string
		var events []string
		if err := pool.QueryRow(t.Context(), `SELECT proof_kind, captured_by::text, photo_path FROM parcel.delivery_proofs WHERE parcel_id=$1`, id).
			Scan(&kind, &capturedBy, &path); err != nil || kind != ProofKindPickup || capturedBy != statusCourierID {
			t.Fatalf("proof = %s/%s/%s %v", kind, capturedBy, path, err)
		}
		if err := pool.QueryRow(t.Context(), `SELECT array_agg(payload->>'status' ORDER BY created_at, id)
			FROM parcel.outbox_events WHERE aggregate_id=$1`, id).Scan(&events); err != nil ||
			len(events) != 2 || events[0] != "PICKED_UP" || events[1] != "IN_TRANSIT" {
			t.Fatalf("event statuses = %v %v", events, err)
		}
		if files := proofStorageFiles(t, directory); len(files) != 1 {
			t.Fatalf("stored files = %d", len(files))
		}
	})

	t.Run("pickup retry creates no duplicate proof history or events", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		if _, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: statusCourierID, Photo: workflowPhoto(t, "image/png"),
		}); err != nil {
			t.Fatal(err)
		}
		first, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: statusCourierID, Photo: workflowPhoto(t, "image/png"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if !first.AlreadyApplied || first.Status != statemachine.StatusInTransit {
			t.Fatalf("retry = %+v", first)
		}
		var history, proofs, events int
		if err := pool.QueryRow(t.Context(), `SELECT
			(SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id=$1),
			(SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1),
			(SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id=$1)`, id).
			Scan(&history, &proofs, &events); err != nil || history != 2 || proofs != 1 || events != 2 {
			t.Fatalf("rows = %d/%d/%d %v", history, proofs, events, err)
		}
		// One stored object for the committed pickup proof; the retry's upload
		// was deleted after the idempotent read.
		if files := proofStorageFiles(t, directory); len(files) != 2 {
			// The previous subtest left one object, so this parcel adds one.
			t.Fatalf("stored files = %d", len(files))
		}
	})

	t.Run("pickup rejects a foreign courier and a non-created parcel", func(t *testing.T) {
		foreign := "00000000-0000-4000-8000-000000000098"
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		if _, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: foreign, Photo: workflowPhoto(t, "image/png"),
		}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("foreign courier: %v", err)
		}
		assigned := insertStatusParcel(t, pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		if _, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: assigned, CourierID: statusCourierID, Photo: workflowPhoto(t, "image/png"),
		}); !errors.Is(err, statemachine.ErrInvalidTransition) {
			t.Fatalf("wrong status: %v", err)
		}
	})

	t.Run("pickup rolls back the proof row and object when commit fails", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		constraint := fmt.Sprintf(
			"ALTER TABLE parcel.outbox_events ADD CONSTRAINT m8_reject_pickup CHECK (aggregate_id <> '%s'::uuid)", id)
		if _, err := pool.Exec(t.Context(), constraint); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() {
			if _, err := pool.Exec(context.Background(),
				"ALTER TABLE parcel.outbox_events DROP CONSTRAINT m8_reject_pickup"); err != nil {
				t.Error(err)
			}
		})
		before := len(proofStorageFiles(t, directory))
		if _, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: statusCourierID, Photo: workflowPhoto(t, "image/png"),
		}); err == nil {
			t.Fatal("expected injected outbox failure")
		}
		var status string
		var history, proofs int
		if err := pool.QueryRow(t.Context(), `SELECT status,
			(SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id=$1),
			(SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1)
			FROM parcel.parcels WHERE id=$1`, id).Scan(&status, &history, &proofs); err != nil ||
			status != "CREATED" || history != 0 || proofs != 0 {
			t.Fatalf("partial pickup: %s/%d/%d %v", status, history, proofs, err)
		}
		if after := len(proofStorageFiles(t, directory)); after != before {
			t.Fatalf("stored files = %d, want %d", after, before)
		}
	})

	t.Run("delivery commits proof recipient and transition", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		result, err := service.ConfirmDelivery(t.Context(), DeliveryInput{
			ParcelID: id, CourierID: statusCourierID, RecipientName: "Amali Fernando",
			Note: "Handed over", Photo: workflowPhoto(t, "image/jpeg"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if result.Status != statemachine.StatusDelivered || result.AlreadyApplied ||
			result.Proof == nil || result.Proof.Kind != ProofKindDelivery {
			t.Fatalf("result = %+v", result)
		}
		var status, deliveredTo, name, note, kind string
		var deliveredAt time.Time
		if err := pool.QueryRow(t.Context(), `SELECT p.status, p.delivered_to_name, p.delivered_at,
			d.recipient_name, d.note, d.proof_kind
			FROM parcel.parcels p JOIN parcel.delivery_proofs d ON d.parcel_id=p.id WHERE p.id=$1`, id).
			Scan(&status, &deliveredTo, &deliveredAt, &name, &note, &kind); err != nil ||
			status != "DELIVERED" || deliveredTo != "Amali Fernando" || deliveredAt.IsZero() ||
			name != "Amali Fernando" || note != "Handed over" || kind != ProofKindDelivery {
			t.Fatalf("delivery = %s/%s/%v/%s/%s/%s %v", status, deliveredTo, deliveredAt, name, note, kind, err)
		}
		var history, events int
		if err := pool.QueryRow(t.Context(), `SELECT
			(SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id=$1),
			(SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id=$1)`, id).
			Scan(&history, &events); err != nil || history != 1 || events != 1 {
			t.Fatalf("rows = %d/%d %v", history, events, err)
		}
	})

	t.Run("delivery retry is idempotent", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "OUT_FOR_DELIVERY", statusCourierID, false)
		input := DeliveryInput{ParcelID: id, CourierID: statusCourierID,
			RecipientName: "Amali", Photo: workflowPhoto(t, "image/png")}
		if _, err := service.ConfirmDelivery(t.Context(), input); err != nil {
			t.Fatal(err)
		}
		second, err := service.ConfirmDelivery(t.Context(), input)
		if err != nil {
			t.Fatal(err)
		}
		if !second.AlreadyApplied {
			t.Fatalf("retry = %+v", second)
		}
		var proofs, events int
		if err := pool.QueryRow(t.Context(), `SELECT
			(SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1),
			(SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id=$1)`, id).
			Scan(&proofs, &events); err != nil || proofs != 1 || events != 1 {
			t.Fatalf("rows = %d/%d %v", proofs, events, err)
		}
	})

	t.Run("detail exposes both proofs and the photo route is assignment scoped", func(t *testing.T) {
		id := insertStatusParcel(t, pool, "CREATED", statusCourierID, false)
		if _, err := service.ConfirmPickup(t.Context(), PickupInput{
			ParcelID: id, CourierID: statusCourierID, Photo: workflowPhoto(t, "image/png"),
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := NewRepository(pool).UpdateStatus(t.Context(), StatusInput{
			ParcelID: id, CourierID: statusCourierID, Status: statemachine.StatusOutForDelivery,
		}); err != nil {
			t.Fatal(err)
		}
		if _, err := service.ConfirmDelivery(t.Context(), DeliveryInput{
			ParcelID: id, CourierID: statusCourierID, RecipientName: "Amali",
			Photo: workflowPhoto(t, "image/png"),
		}); err != nil {
			t.Fatal(err)
		}
		detail, err := NewRepository(pool).Detail(t.Context(), statusCourierID, id)
		if err != nil {
			t.Fatal(err)
		}
		if len(detail.Proofs) != 2 || detail.Proofs[0].Kind != ProofKindPickup ||
			detail.Proofs[1].Kind != ProofKindDelivery || detail.Proof == nil ||
			detail.Proof.Kind != ProofKindDelivery {
			t.Fatalf("proofs = %+v latest = %+v", detail.Proofs, detail.Proof)
		}
		photos := NewProofPhotoService(pool, storage, time.Minute)
		for _, proof := range detail.Proofs {
			download, err := photos.PhotoByProof(t.Context(), id, proof.ID,
				PhotoViewer{UserID: statusCourierID, Courier: true})
			if err != nil || !download.Stream || download.MIMEType == "" {
				t.Fatalf("photo %s = %+v %v", proof.Kind, download, err)
			}
		}
		foreign := "00000000-0000-4000-8000-000000000098"
		if _, err := photos.PhotoByProof(t.Context(), id, detail.Proofs[0].ID,
			PhotoViewer{UserID: foreign, Courier: true}); !errors.Is(err, ErrForbidden) {
			t.Fatalf("foreign photo = %v", err)
		}
		if _, err := photos.PhotoByProof(t.Context(), id,
			"00000000-0000-4000-8000-000000000097",
			PhotoViewer{UserID: statusCourierID, Courier: true}); !errors.Is(err, ErrProofNotFound) {
			t.Fatalf("unknown proof = %v", err)
		}
	})
}
