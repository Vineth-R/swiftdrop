//go:build integration && postgres

package servicehttp

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestCourierStatusHTTPDatabase(t *testing.T) {
	database := parceltest.New(t)
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, Courier: courier.NewRepository(database.Pool)})
	id, other := parceltest.UUID(t), parceltest.UUID(t)
	_, err := database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcels (
		id,tracking_number,customer_user_id,sender_name,sender_phone,pickup_address,
		pickup_city,pickup_zone_id,recipient_name,recipient_phone,dropoff_address,
		dropoff_city,dropoff_zone_id,weight_kg,size_category,price_amount,price_breakdown,
		created_by_user_id,current_courier_id
	) VALUES ($1,'SDK12345678',$2,'Sender','+94770000001','Pickup','Colombo',$2,
		'Original Recipient','+94770000002','Dropoff','Kandy',$2,2,'SMALL',550,
		'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',$2,$2)`, id, courierTestID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcel_assignments
		(parcel_id,courier_id,assigned_by) VALUES ($1,$2,$2)`, id, courierTestID); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, body, actor, code string
		want                    int
	}{
		{name: "foreign before invalid transition", actor: other, body: `{"status":"DELIVERED"}`, want: 403, code: "FORBIDDEN"},
		{name: "skip before recipient check", actor: courierTestID, body: `{"status":"DELIVERED"}`, want: 409, code: "INVALID_TRANSITION"},
		{name: "failed requires reason", actor: courierTestID, body: `{"status":"FAILED"}`, want: 422, code: "FAILURE_REASON_REQUIRED"},
		{name: "unknown status", actor: courierTestID, body: `{"status":"unknown"}`, want: 422, code: "VALIDATION_ERROR"},
		{name: "missing status", actor: courierTestID, body: `{}`, want: 422, code: "VALIDATION_ERROR"},
		{name: "long note", actor: courierTestID, body: `{"status":"PICKED_UP","note":"` + strings.Repeat("x", 501) + `"}`, want: 422, code: "VALIDATION_ERROR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := courierStatusRequest(t, router, id, tc.body, tc.actor, auth.RoleCourier, false)
			if rec.Code != tc.want || !strings.Contains(rec.Body.String(), `"code":"`+tc.code+`"`) {
				t.Fatalf("response %d %s", rec.Code, rec.Body)
			}
			var status string
			var history, events int
			if err := database.Pool.QueryRow(t.Context(), `SELECT status,
				(SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id=$1),
				(SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id=$1)
				FROM parcel.parcels WHERE id=$1`, id).Scan(&status, &history, &events); err != nil || status != "CREATED" || history != 0 || events != 0 {
				t.Fatalf("rejected request changed rows: %s/%d/%d %v", status, history, events, err)
			}
		})
	}
	missing := courierStatusRequest(t, router, parceltest.UUID(t), `{"status":"PICKED_UP"}`, courierTestID, auth.RoleCourier, false)
	if missing.Code != 404 {
		t.Fatalf("missing parcel: %d %s", missing.Code, missing.Body)
	}
	t.Run("HTTP lifecycle and existing reads", func(t *testing.T) {
		for _, next := range []string{"PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED"} {
			if next == "DELIVERED" {
				invalid := courierStatusRequest(t, router, id, `{"status":"DELIVERED","recipientName":"  "}`, courierTestID, auth.RoleCourier, true)
				if invalid.Code != 422 || !strings.Contains(invalid.Body.String(), `"code":"RECIPIENT_REQUIRED"`) {
					t.Fatalf("missing recipient: %d %s", invalid.Code, invalid.Body)
				}
			}
			rec := courierStatusRequest(t, router, id, `{"status":"`+next+`","recipientName":"Accepted By"}`, courierTestID, auth.RoleCourier, true)
			if rec.Code != 200 {
				t.Fatalf("update: %d %s", rec.Code, rec.Body)
			}
			var body struct {
				Data courier.StatusResult `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil || string(body.Data.Status) != next || body.Data.ParcelID != id || body.Data.UpdatedAt.IsZero() {
				t.Fatalf("envelope: %s %v", rec.Body, err)
			}
		}
		// The pre-existing read routes return the committed status and history.
		detail := courierRequest(t, router, "/courier/deliveries/"+id, courierTestID, auth.RoleCourier, false)
		var body struct {
			Data courier.Detail `json:"data"`
		}
		if detail.Code != 200 {
			t.Fatalf("detail: %d %s", detail.Code, detail.Body)
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &body); err != nil || body.Data.Status != "DELIVERED" || len(body.Data.History) != 4 || body.Data.RecipientName != "Original Recipient" {
			t.Fatalf("read after write: %s %v", detail.Body, err)
		}
		date := body.Data.History[len(body.Data.History)-1].ChangedAt.Add(5*time.Hour + 30*time.Minute).Format(time.DateOnly)
		list := courierRequest(t, router, "/courier/deliveries?date="+date+"&status=DELIVERED", courierTestID, auth.RoleCourier, false)
		if list.Code != 200 || !strings.Contains(list.Body.String(), id) {
			t.Fatalf("list after write: %d %s", list.Code, list.Body)
		}
	})
}
