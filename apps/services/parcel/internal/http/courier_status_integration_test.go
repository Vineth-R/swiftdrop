//go:build integration && postgres

package servicehttp

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/objectstore"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestCourierStatusHTTPDatabase(t *testing.T) {
	database := parceltest.New(t)
	repository := courier.NewRepository(database.Pool)
	storage, err := objectstore.NewLocal(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := storage.Close(); err != nil {
			t.Error(err)
		}
	})
	workflow := courier.NewWorkflowService(database.Pool, storage, 0)
	router := reportRouter(t, Options{AccessSecret: reportTestSecret, SessionCheck: allowSessions(),
		Courier: repository, CourierStatus: repository, CourierWorkflow: workflow})
	id, other := parceltest.UUID(t), parceltest.UUID(t)
	_, err = database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcels (
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
		pickupBody, pickupType := proofForm(t, map[string]string{"note": "Collected"}, workflowPNG(t), "image/png")
		pickup := workflowHTTPRequest(t, router, "/courier/deliveries/"+id+"/pickup", pickupBody, pickupType)
		if pickup.Code != 200 || !strings.Contains(pickup.Body.String(), `"status":"IN_TRANSIT"`) {
			t.Fatalf("pickup: %d %s", pickup.Code, pickup.Body)
		}
		if location := pickup.Header().Get("Location"); location != "" {
			t.Fatalf("pickup redirected to %q", location)
		}

		out := courierStatusRequestPath(t, router, "/courier/deliveries/"+id+"/status/", `{"status":"OUT_FOR_DELIVERY"}`, courierTestID, auth.RoleCourier, true)
		if out.Code != 200 || !strings.Contains(out.Body.String(), `"status":"OUT_FOR_DELIVERY"`) {
			t.Fatalf("out for delivery: %d %s", out.Code, out.Body)
		}

		// The generic route must not bypass the delivery proof transaction.
		noProof := courierStatusRequest(t, router, id, `{"status":"DELIVERED","recipientName":"Accepted By"}`, courierTestID, auth.RoleCourier, true)
		if noProof.Code != 422 || !strings.Contains(noProof.Body.String(), `"code":"PROOF_REQUIRED"`) {
			t.Fatalf("delivery without proof: %d %s", noProof.Code, noProof.Body)
		}

		deliveryBody, deliveryType := proofForm(t, map[string]string{
			"recipientName": "Accepted By", "note": "Handed over",
		}, workflowPNG(t), "image/png")
		delivery := workflowHTTPRequest(t, router, "/courier/deliveries/"+id+"/delivery/", deliveryBody, deliveryType)
		if delivery.Code != 200 || !strings.Contains(delivery.Body.String(), `"status":"DELIVERED"`) {
			t.Fatalf("delivery: %d %s", delivery.Code, delivery.Body)
		}

		// The pre-existing read routes return the committed status and history.
		detail := courierRequest(t, router, "/courier/deliveries/"+id, courierTestID, auth.RoleCourier, false)
		var body struct {
			Data courier.Detail `json:"data"`
		}
		if detail.Code != 200 {
			t.Fatalf("detail: %d %s", detail.Code, detail.Body)
		}
		if err := json.Unmarshal(detail.Body.Bytes(), &body); err != nil ||
			body.Data.Status != "DELIVERED" || len(body.Data.History) != 4 ||
			len(body.Data.Proofs) != 2 || body.Data.Proofs[0].Kind != courier.ProofKindPickup ||
			body.Data.Proofs[1].Kind != courier.ProofKindDelivery {
			t.Fatalf("read after write: %s %v", detail.Body, err)
		}
		date := body.Data.History[len(body.Data.History)-1].ChangedAt.Add(5*time.Hour + 30*time.Minute).Format(time.DateOnly)
		list := courierRequest(t, router, "/courier/deliveries?date="+date+"&status=DELIVERED", courierTestID, auth.RoleCourier, false)
		if list.Code != 200 || !strings.Contains(list.Body.String(), id) {
			t.Fatalf("list after write: %d %s", list.Code, list.Body)
		}
	})
}

func workflowHTTPRequest(t *testing.T, router *gin.Engine, path string, body []byte, contentType string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", contentType)
	manager, err := auth.NewTokenManager(reportTestSecret, time.Hour)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue(courierTestID, auth.RoleCourier)
	if err != nil {
		t.Fatal(err)
	}
	req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}
