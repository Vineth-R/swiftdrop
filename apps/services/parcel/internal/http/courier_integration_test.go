//go:build integration && postgres

package servicehttp

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/dispatch"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/httpx"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestCourierDatabaseReads(t *testing.T) {
	database := parceltest.New(t)
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()
	// PostgreSQL session timezone deliberately differs from the business timezone.
	conn, err := pgx.Connect(ctx, database.URL())
	if err != nil {
		t.Fatal("open isolated courier read connection")
	}
	defer func() {
		if err := conn.Close(context.Background()); err != nil {
			t.Error("close courier read connection")
		}
	}()
	if _, err := conn.Exec(ctx, "SET TIME ZONE 'America/New_York'"); err != nil {
		t.Fatal(err)
	}
	store := courier.NewRepository(conn)
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(), Courier: store})
	otherCourier := parceltest.UUID(t)
	ids := make([]string, 0)
	start, end, err := courier.DayBounds("2026-11-07", time.Now())
	if err != nil {
		t.Fatal(err)
	}
	insert := func(status string, finish time.Time, owner *string) string {
		t.Helper()
		id := parceltest.UUID(t)
		var deliveredAt, failedAt, recipient, reason any
		if status == "DELIVERED" {
			deliveredAt = finish
			recipient = "Test Recipient"
		}
		if status == "FAILED" {
			failedAt = finish
			reason = "Recipient unavailable"
		}
		_, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcels (
			id, tracking_number, customer_user_id, sender_name, sender_phone,
			pickup_address, pickup_city, pickup_zone_id, recipient_name, recipient_phone,
			dropoff_address, dropoff_city, dropoff_zone_id, weight_kg, size_category,
			description, status, failure_reason, current_courier_id, price_amount,
			price_breakdown, created_by_user_id, created_at, delivered_at, failed_at, delivered_to_name
		) VALUES ($1,$2,$3,'Test Sender','+94770000001','Pickup','Colombo',$3,
			'Test Recipient','+94770000002','Dropoff','Kandy',$3,2.25,'SMALL', 'Books',
			$4,$5,$6,550,'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',
			$3,'2026-10-01T00:00:00Z',$7,$8,$9)`,
			id, fmt.Sprintf("SDK%08d", len(ids)+1), courierTestID, status, reason, owner,
			deliveredAt, failedAt, recipient)
		if err != nil {
			t.Fatalf("insert courier fixture: %v", err)
		}
		if owner != nil {
			_, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcel_assignments
				(parcel_id,courier_id,assigned_by,assigned_at) VALUES ($1,$2,$3,$4)`,
				id, *owner, courierTestID, start.Add(time.Duration(len(ids))*time.Minute))
			if err != nil {
				t.Fatal(err)
			}
		}
		ids = append(ids, id)
		return id
	}
	activeIDs := []string{}
	for _, status := range []string{"CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY"} {
		activeIDs = append(activeIDs, insert(status, time.Time{}, ptrCourier(courierTestID)))
	}
	delivered := insert("DELIVERED", start, ptrCourier(courierTestID))
	failed := insert("FAILED", end.Add(-time.Microsecond), ptrCourier(courierTestID))
	insert("DELIVERED", start.Add(-time.Microsecond), ptrCourier(courierTestID))
	insert("DELIVERED", end, ptrCourier(courierTestID))
	insert("FAILED", start.Add(-time.Microsecond), ptrCourier(courierTestID))
	insert("FAILED", end, ptrCourier(courierTestID))
	foreign := insert("PICKED_UP", time.Time{}, &otherCourier)
	unassigned := insert("CREATED", time.Time{}, nil)
	// A closed historical assignment must never grant access.
	if _, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcel_assignments
		(parcel_id,courier_id,assigned_by,is_current,unassigned_at)
		VALUES ($1,$2,$2,false,now())`, foreign, courierTestID); err != nil {
		t.Fatal(err)
	}

	type listResponse struct {
		Data []courier.Delivery `json:"data"`
		Meta httpx.SuccessMeta  `json:"meta"`
	}
	list := func(t *testing.T, query, userID string) listResponse {
		t.Helper()
		rec := courierRequest(t, router, "/courier/deliveries?"+query, userID, auth.RoleCourier, false)
		if rec.Code != 200 {
			t.Fatalf("list: %d %s", rec.Code, rec.Body)
		}
		var body listResponse
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data == nil || body.Meta.Pagination == nil {
			t.Fatalf("invalid list envelope %s", rec.Body)
		}
		return body
	}
	t.Run("own active plus Colombo day completion boundaries", func(t *testing.T) {
		body := list(t, "date=2026-11-07", courierTestID)
		if len(body.Data) != 6 || body.Meta.Pagination.TotalItems != 6 {
			t.Fatalf("expected four active and two completed: %+v", body)
		}
		got := map[string]bool{}
		for _, item := range body.Data {
			got[item.ID] = true
			if item.WeightKG != 2.25 || item.AssignedAt.IsZero() || item.RecipientName != "Test Recipient" || item.RecipientPhone != "+94770000002" {
				t.Fatalf("missing contract fields %+v", item)
			}
		}
		for _, id := range append(activeIDs, delivered, failed) {
			if !got[id] {
				t.Fatalf("missing %s", id)
			}
		}
		if got[foreign] || got[unassigned] {
			t.Fatal("unauthorized parcel leaked")
		}
	})
	for _, tc := range []struct {
		name, query string
		total       int
	}{
		{name: "delivered day", query: "date=2026-11-07&status=DELIVERED", total: 1},
		{name: "failed day", query: "date=2026-11-07&status=FAILED", total: 1},
		{name: "active status ignores day", query: "date=2020-01-01&status=PICKED_UP", total: 1},
		{name: "all active ignore day", query: "date=2020-01-01", total: 4},
		{name: "empty completed day", query: "date=2020-01-01&status=DELIVERED", total: 0},
		{name: "previous local day", query: "date=2026-11-06&status=FAILED", total: 1},
		{name: "next local day", query: "date=2026-11-08&status=DELIVERED", total: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			body := list(t, tc.query, courierTestID)
			if len(body.Data) != tc.total || body.Meta.Pagination.TotalItems != tc.total {
				t.Fatalf("%+v", body)
			}
		})
	}
	t.Run("pagination stable no duplicate rows", func(t *testing.T) {
		seen := map[string]bool{}
		for page := 1; page <= 3; page++ {
			body := list(t, fmt.Sprintf("date=2026-11-07&pageSize=2&page=%d", page), courierTestID)
			if len(body.Data) != 2 || body.Meta.Pagination.TotalPages != 3 || body.Meta.Pagination.Page != page {
				t.Fatalf("pagination %+v", body)
			}
			for _, item := range body.Data {
				if seen[item.ID] {
					t.Fatal("unstable pagination")
				}
				seen[item.ID] = true
			}
		}
		body := list(t, "date=2026-11-07&page=9&pageSize=2", courierTestID)
		if len(body.Data) != 0 || body.Meta.Pagination.TotalItems != 6 {
			t.Fatalf("out-of-range %+v", body)
		}
	})
	t.Run("another courier list is isolated", func(t *testing.T) {
		body := list(t, "date=2026-11-07", otherCourier)
		if len(body.Data) != 1 || body.Data[0].ID != foreign {
			t.Fatalf("foreign scope %+v", body)
		}
		body = list(t, "date=2026-11-07", parceltest.UUID(t))
		if len(body.Data) != 0 || body.Meta.Pagination.TotalItems != 0 {
			t.Fatal("empty courier received parcels")
		}
	})
	for _, tc := range []struct {
		name, id, userID string
		want             int
	}{
		{name: "own", id: delivered, userID: courierTestID, want: 200},
		{name: "other courier", id: delivered, userID: otherCourier, want: 403},
		{name: "old assignment", id: foreign, userID: courierTestID, want: 403},
		{name: "unassigned", id: unassigned, userID: courierTestID, want: 403},
		{name: "missing", id: parceltest.UUID(t), userID: courierTestID, want: 404},
	} {
		t.Run("detail "+tc.name, func(t *testing.T) {
			rec := courierRequest(t, router, "/courier/deliveries/"+tc.id, tc.userID, auth.RoleCourier, true)
			if rec.Code != tc.want {
				t.Fatalf("%d %s", rec.Code, rec.Body)
			}
			if tc.want != 200 && strings.Contains(rec.Body.String(), "Test Recipient") {
				t.Fatal("contact leaked")
			}
			if tc.want == 200 && (!strings.Contains(rec.Body.String(), `"history":[]`) || !strings.Contains(rec.Body.String(), `"proof":null`)) {
				t.Fatalf("empty detail metadata %s", rec.Body)
			}
		})
	}
	t.Run("authorized ordered history and most recent private proof metadata", func(t *testing.T) {
		for _, status := range []string{"CREATED", "PICKED_UP"} {
			_, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcel_status_history
				(parcel_id,previous_status,new_status,actor_role,note,created_at)
				VALUES ($1,$2,$3,'COURIER','History note',$4)`, delivered,
				map[string]any{"CREATED": nil, "PICKED_UP": "CREATED"}[status], status,
				map[string]time.Time{"CREATED": start, "PICKED_UP": start.Add(time.Hour)}[status])
			if err != nil {
				t.Fatal(err)
			}
		}
		for _, stamp := range []time.Time{start, start.Add(time.Hour)} {
			_, err := database.Pool.Exec(ctx, `INSERT INTO parcel.delivery_proofs
				(parcel_id,recipient_name,note,photo_path,photo_mime_type,photo_size_bytes,captured_by,created_at)
				VALUES ($1,'Accepted by','Proof note','private/generated.png','image/png',100,$2,$3)`, delivered, courierTestID, stamp)
			if err != nil {
				t.Fatal(err)
			}
		}
		rec := courierRequest(t, router, "/courier/deliveries/"+delivered, courierTestID, auth.RoleCourier, false)
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		var response struct {
			Data courier.Detail `json:"data"`
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &response); err != nil {
			t.Fatal(err)
		}
		d := response.Data
		if d.SenderName != "Test Sender" || d.SenderPhone != "+94770000001" || d.Description == nil || *d.Description != "Books" || d.SizeCategory != "SMALL" {
			t.Fatalf("detail fields %+v", d)
		}
		if len(d.History) != 2 || d.History[0].PreviousStatus != nil || d.History[1].Status != "PICKED_UP" || d.History[1].ActorRole == nil {
			t.Fatalf("history %+v", d.History)
		}
		if d.Proof == nil || !d.Proof.HasPhoto || !d.Proof.CreatedAt.Equal(start.Add(time.Hour)) {
			t.Fatalf("proof %+v", d.Proof)
		}
		for _, private := range []string{"private/generated.png", "photoPath", "capturedBy", "actorUserId", "customerUserId", "priceBreakdown"} {
			if strings.Contains(rec.Body.String(), private) {
				t.Fatalf("private metadata leaked: %s", private)
			}
		}
	})
	t.Run("note-only proof and failed history preserve null fields", func(t *testing.T) {
		if _, err := database.Pool.Exec(ctx, `INSERT INTO parcel.delivery_proofs
			(parcel_id,note,captured_by) VALUES ($1,'No photo available',$2)`, failed, courierTestID); err != nil {
			t.Fatal(err)
		}
		if _, err := database.Pool.Exec(ctx, `INSERT INTO parcel.parcel_status_history
			(parcel_id,previous_status,new_status,failure_reason,actor_role)
			VALUES ($1,'CREATED','FAILED','Recipient unavailable','COURIER')`, failed); err != nil {
			t.Fatal(err)
		}
		rec := courierRequest(t, router, "/courier/deliveries/"+failed, courierTestID, auth.RoleCourier, false)
		var body struct {
			Data courier.Detail `json:"data"`
		}
		if rec.Code != 200 {
			t.Fatalf("%d %s", rec.Code, rec.Body)
		}
		if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		if body.Data.Proof == nil || body.Data.Proof.HasPhoto || body.Data.Proof.RecipientName != nil {
			t.Fatalf("proof %+v", body.Data.Proof)
		}
		if len(body.Data.History) != 1 || body.Data.History[0].FailureReason == nil || *body.Data.History[0].FailureReason != "Recipient unavailable" {
			t.Fatal("missing failure history")
		}
		foreign := courierRequest(t, router, "/courier/deliveries/"+failed, otherCourier, auth.RoleCourier, false)
		if foreign.Code != 403 || strings.Contains(foreign.Body.String(), "No photo available") || strings.Contains(foreign.Body.String(), "Recipient unavailable") {
			t.Fatal("populated history/proof leaked")
		}
	})
	t.Run("M6 reassignment removes previous courier access immediately", func(t *testing.T) {
		repo := dispatch.NewRepository(database.Pool)
		_, err := repo.AssignCourier(ctx, dispatch.AssignInput{ParcelID: activeIDs[0], CourierID: otherCourier, ActorUserID: courierTestID})
		if err != nil {
			t.Fatal(err)
		}
		old := courierRequest(t, router, "/courier/deliveries/"+activeIDs[0], courierTestID, auth.RoleCourier, false)
		fresh := courierRequest(t, router, "/courier/deliveries/"+activeIDs[0], otherCourier, auth.RoleCourier, false)
		if old.Code != 403 || fresh.Code != 200 {
			t.Fatalf("old/new access %d/%d", old.Code, fresh.Code)
		}
		body := list(t, "date=2026-11-07", courierTestID)
		for _, item := range body.Data {
			if item.ID == activeIDs[0] {
				t.Fatal("old courier retained reassigned list item")
			}
		}
	})
	t.Run("current courier without a current assignment is a controlled internal error", func(t *testing.T) {
		// The helper inserts with no owner and no assignment; setting the copy
		// afterwards creates the inconsistent state without fabricating data.
		orphan := insert("CREATED", time.Time{}, nil)
		if _, err := database.Pool.Exec(ctx,
			`UPDATE parcel.parcels SET current_courier_id = $1 WHERE id = $2`,
			courierTestID, orphan); err != nil {
			t.Fatal(err)
		}
		for _, path := range []string{
			"/courier/deliveries?date=2026-11-07",
			"/courier/deliveries/" + orphan,
		} {
			rec := courierRequest(t, router, path, courierTestID, auth.RoleCourier, false)
			if rec.Code != 500 || !strings.Contains(rec.Body.String(), `"code":"INTERNAL_ERROR"`) {
				t.Fatalf("%s: %d %s", path, rec.Code, rec.Body)
			}
			for _, internal := range []string{"assigned_at", "parcel_assignments", "current_courier_id", "SQL"} {
				if strings.Contains(rec.Body.String(), internal) {
					t.Fatalf("%s leaked internal detail %q: %s", path, internal, rec.Body)
				}
			}
		}
		// A valid assignment continues to work after the inconsistent row exists.
		rec := courierRequest(t, router, "/courier/deliveries/"+delivered, courierTestID, auth.RoleCourier, false)
		if rec.Code != 200 {
			t.Fatalf("valid assignment regressed: %d %s", rec.Code, rec.Body)
		}
	})
}

func ptrCourier(id string) *string { return &id }
