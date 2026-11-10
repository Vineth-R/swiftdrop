//go:build integration && postgres

package servicehttp

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/courier"
	proofmedia "github.com/Vineth-R/swiftdrop/apps/services/parcel/internal/proof"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/tests/integration/parceltest"
)

func TestCourierProofHTTPDatabase(t *testing.T) {
	database := parceltest.New(t)
	storage, err := proofmedia.NewLocalStorage(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = storage.Close() })
	service := courier.NewProofService(database.Pool, storage)
	router := proofRouter(t, service)

	id := parceltest.UUID(t)
	_, err = database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcels (
		id,tracking_number,customer_user_id,sender_name,sender_phone,pickup_address,
		pickup_city,pickup_zone_id,recipient_name,recipient_phone,dropoff_address,
		dropoff_city,dropoff_zone_id,weight_kg,size_category,price_amount,price_breakdown,
		created_by_user_id,current_courier_id,status
	) VALUES ($1,'SDK87654321',$2,'Sender','+94770000001','Pickup','Colombo',$2,
		'Recipient','+94770000002','Dropoff','Kandy',$2,2,'SMALL',550,
		'{"base_fee":450,"weight_charge":100,"size_surcharge":0}',$2,$2,'OUT_FOR_DELIVERY')`, id, courierTestID)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.Pool.Exec(t.Context(), `INSERT INTO parcel.parcel_assignments
		(parcel_id,courier_id,assigned_by) VALUES ($1,$2,$2)`, id, courierTestID); err != nil {
		t.Fatal(err)
	}
	body, contentType := proofForm(t, map[string]string{"recipientName": "Recipient"}, nil, "")
	request := func(parcelID, userID string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodPost, "/courier/deliveries/"+parcelID+"/proof", bytes.NewReader(body))
		req.Header.Set("Content-Type", contentType)
		manager, tokenErr := auth.NewTokenManager(reportTestSecret, time.Hour)
		if tokenErr != nil {
			t.Fatal(tokenErr)
		}
		token, _, tokenErr := manager.Issue(userID, auth.RoleCourier)
		if tokenErr != nil {
			t.Fatal(tokenErr)
		}
		req.AddCookie(&http.Cookie{Name: "sd_access", Value: token})
		recorder := httptest.NewRecorder()
		router.ServeHTTP(recorder, req)
		return recorder
	}

	if recorder := request(id, courierTestID); recorder.Code != http.StatusCreated || !strings.Contains(recorder.Body.String(), `"hasPhoto":false`) {
		t.Fatalf("created response %d %s", recorder.Code, recorder.Body)
	}
	var count int
	if err := database.Pool.QueryRow(t.Context(), `SELECT count(*) FROM parcel.delivery_proofs WHERE parcel_id=$1`, id).Scan(&count); err != nil || count != 1 {
		t.Fatalf("proof count=%d error=%v", count, err)
	}
	if recorder := request(id, parceltest.UUID(t)); recorder.Code != http.StatusForbidden {
		t.Fatalf("foreign response %d %s", recorder.Code, recorder.Body)
	}
	if recorder := request(parceltest.UUID(t), courierTestID); recorder.Code != http.StatusNotFound {
		t.Fatalf("missing response %d %s", recorder.Code, recorder.Body)
	}
}
