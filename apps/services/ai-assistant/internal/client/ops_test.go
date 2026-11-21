package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newParcelTestClient(t *testing.T, handler http.HandlerFunc) (*ParcelClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	parcel, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	return parcel, server.Close
}

func newMerchantTestClient(t *testing.T, handler http.HandlerFunc) (*MerchantClient, func()) {
	t.Helper()
	server := httptest.NewServer(handler)
	merchant, err := NewMerchantClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewMerchantClient: %v", err)
	}
	return merchant, server.Close
}

func TestPublicTrackingUsesAnonymousProjection(t *testing.T) {
	parcel, closeServer := newParcelTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/tracking/SDK7M2Q9XP4" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public tracking sent Authorization %q", got)
		}
		_, _ = io.WriteString(w, `{"data":{"trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT","pickupCity":"Colombo","dropoffCity":"Kandy","lastUpdatedAt":"2026-11-01T00:00:00Z","timeline":[{"status":"CREATED","at":"2026-11-01T00:00:00Z"}],"recipientPhone":"0770000000"},"meta":{}}`)
	})
	defer closeServer()

	snapshot, err := parcel.PublicTracking(context.Background(), "SDK7M2Q9XP4")
	if err != nil {
		t.Fatalf("PublicTracking: %v", err)
	}
	if snapshot.Status != "IN_TRANSIT" || len(snapshot.Timeline) != 1 {
		t.Fatalf("snapshot = %+v", snapshot)
	}
	encoded, _ := json.Marshal(snapshot)
	if strings.Contains(string(encoded), "0770000000") || strings.Contains(string(encoded), "recipientPhone") {
		t.Fatalf("public projection leaked a phone: %s", encoded)
	}
}

func TestParcelDetailOnlyModelsAllowlistedFields(t *testing.T) {
	parcel, closeServer := newParcelTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parcels/p1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing caller credentials")
		}
		_, _ = io.WriteString(w, `{"data":{
			"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"FAILED",
			"sender":{"name":"Private Sender","phone":"0770000000","address":"1 Secret Lane","city":"Colombo","zoneId":"z1"},
			"recipient":{"name":"Private Recipient","phone":"0770000001","address":"2 Secret Lane","city":"Kandy","zoneId":"z2"},
			"weightKg":2.00,"sizeCategory":"SMALL",
			"fee":{"currency":"LKR","baseFee":450.00,"weightCharge":100.00,"sizeSurcharge":0.00,"total":550.00},
			"courier":{"id":"courier-1","fullName":"Courier Name","phone":"0770000002"},
			"failureReason":"Recipient unavailable",
			"createdAt":"2026-11-01T00:00:00Z","updatedAt":"2026-11-02T00:00:00Z","deliveredAt":null,
			"internalNotes":"do not leak","proofPath":"/proofs/secret"
		},"meta":{}}`)
	})
	defer closeServer()

	detail, err := parcel.Detail(context.Background(), "p1", "token")
	if err != nil {
		t.Fatalf("Detail: %v", err)
	}
	if detail.PickupCity != "Colombo" || detail.DropoffCity != "Kandy" || detail.FailureReason == nil || *detail.FailureReason != "Recipient unavailable" {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.CourierName == nil || *detail.CourierName != "Courier Name" {
		t.Fatalf("courier name = %+v", detail.CourierName)
	}
	encoded, _ := json.Marshal(detail)
	for _, forbidden := range []string{"0770000000", "0770000001", "0770000002", "Secret Lane", "Private Sender", "Private Recipient", "internalNotes", "proofPath", "zoneId", "courier-1"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("detail leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestAdminParcelDateFiltersBecomeRFC3339(t *testing.T) {
	parcel, closeServer := newParcelTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		query := r.URL.Query()
		if query.Get("from") != "2026-11-01T00:00:00Z" || query.Get("to") != "2026-11-20T23:59:59Z" {
			t.Fatalf("date bounds = %q %q", query.Get("from"), query.Get("to"))
		}
		if query.Get("trackingNumber") != "SDK7" {
			t.Fatalf("tracking filter = %q", query.Get("trackingNumber"))
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":0,"totalPages":0}}}`)
	})
	defer closeServer()

	if _, err := parcel.AdminParcels(context.Background(), AdminParcelFilter{
		FromDate: "2026-11-01", ToDate: "2026-11-20", TrackingNumber: "SDK7",
	}, "token"); err != nil {
		t.Fatalf("AdminParcels: %v", err)
	}
}

func TestUnassignedAndWorkloadEndpoints(t *testing.T) {
	var paths []string
	parcel, closeServer := newParcelTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.Header.Get("Authorization") != "Bearer token" {
			t.Fatalf("missing caller credentials")
		}
		if strings.HasPrefix(r.URL.Path, "/dispatch/unassigned") {
			_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"CREATED","pickupAddress":"1 Secret Lane","pickupCity":"Colombo","dropoffAddress":"2 Secret Lane","dropoffCity":"Kandy","weightKg":2.00,"sizeCategory":"SMALL","createdAt":"2026-11-01T00:00:00Z"}] ,"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
			return
		}
		_, _ = io.WriteString(w, `{"data":[{"courierId":"c1","fullName":"Courier","phone":"0770000000","isAvailable":true,"activeDeliveries":2,"byStatus":{"IN_TRANSIT":2}}],"meta":{}}`)
	})
	defer closeServer()

	list, err := parcel.Unassigned(context.Background(), "token")
	if err != nil {
		t.Fatalf("Unassigned: %v", err)
	}
	encoded, _ := json.Marshal(list.Items)
	if strings.Contains(string(encoded), "Secret Lane") || strings.Contains(string(encoded), "pickupAddress") {
		t.Fatalf("unassigned leaked an address: %s", encoded)
	}

	workload, err := parcel.Workload(context.Background(), "token")
	if err != nil {
		t.Fatalf("Workload: %v", err)
	}
	if len(workload) != 1 || workload[0].ActiveDeliveries != 2 {
		t.Fatalf("workload = %+v", workload)
	}
	if len(paths) != 2 {
		t.Fatalf("paths = %v", paths)
	}
}

func TestCourierEndpointsUseStatusDateAndDetailHistory(t *testing.T) {
	var listQuery string
	parcel, closeServer := newParcelTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/deliveries") {
			listQuery = r.URL.RawQuery
			_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"CREATED","pickupAddress":"1 Secret Lane","dropoffAddress":"2 Secret Lane","dropoffCity":"Kandy","recipientName":"Recipient","recipientPhone":"0770000000","weightKg":2.00,"assignedAt":"2026-11-01T00:00:00Z"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
			return
		}
		if r.URL.Path != "/courier/deliveries/p1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_, _ = io.WriteString(w, `{"data":{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"FAILED","pickupAddress":"1 Secret Lane","dropoffAddress":"2 Secret Lane","dropoffCity":"Kandy","recipientName":"Recipient","recipientPhone":"0770000000","weightKg":2.00,"assignedAt":"2026-11-01T00:00:00Z","description":"Documents","sizeCategory":"SMALL","senderName":"Sender","senderPhone":"0770000001","history":[{"previousStatus":"OUT_FOR_DELIVERY","status":"FAILED","note":"Gate locked","failureReason":"No access","actorRole":"COURIER","changedAt":"2026-11-02T00:00:00Z"}],"proof":null,"proofs":[]},"meta":{}}`)
	})
	defer closeServer()

	list, err := parcel.CourierDeliveries(context.Background(), "FAILED", "2026-11-20", "token")
	if err != nil {
		t.Fatalf("CourierDeliveries: %v", err)
	}
	if !strings.Contains(listQuery, "status=FAILED") || !strings.Contains(listQuery, "date=2026-11-20") {
		t.Fatalf("courier query = %q", listQuery)
	}
	encoded, _ := json.Marshal(list.Items)
	for _, forbidden := range []string{"0770000000", "Secret Lane", "recipientPhone", "pickupAddress"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("courier list leaked %q: %s", forbidden, encoded)
		}
	}

	detail, err := parcel.CourierDelivery(context.Background(), "p1", "token")
	if err != nil {
		t.Fatalf("CourierDelivery: %v", err)
	}
	if len(detail.History) != 1 || detail.History[0].FailureReason == nil || *detail.History[0].FailureReason != "No access" {
		t.Fatalf("courier history = %+v", detail.History)
	}
	encoded, _ = json.Marshal(detail)
	for _, forbidden := range []string{"0770000000", "0770000001", "Secret Lane", "senderName", "proofs"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("courier detail leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestMerchantOrdersAndWebhookDeliveries(t *testing.T) {
	var queries []string
	merchant, closeServer := newMerchantTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		queries = append(queries, r.URL.Path+"?"+r.URL.RawQuery)
		if r.Header.Get("Authorization") != "Bearer merchant-token" {
			t.Fatalf("missing merchant credentials")
		}
		switch r.URL.Path {
		case "/merchant/orders":
			_, _ = io.WriteString(w, `{"data":[{"orderId":"order-1","externalOrderId":"SZ-1","trackingNumber":"SDN4T8W2LC6","status":"IN_TRANSIT","recipientName":"Recipient","dropoffCity":"Kandy","total":550.00,"currency":"LKR","createdAt":"2026-11-01T00:00:00Z","updatedAt":"2026-11-02T00:00:00Z","recipientPhone":"0770000000"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
		case "/merchants/me/webhook/deliveries":
			if !strings.Contains(r.URL.RawQuery, "success=false") {
				t.Fatalf("webhook query = %q", r.URL.RawQuery)
			}
			_, _ = io.WriteString(w, `{"data":[{"eventId":"event-1","trackingNumber":"SDN4T8W2LC6","attemptNumber":2,"httpStatus":502,"success":false,"nextRetryAt":"2026-11-02T01:00:00Z","deadLettered":false,"createdAt":"2026-11-02T00:00:00Z","payload":"secret"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})
	defer closeServer()

	orders, err := merchant.Orders(context.Background(), MerchantOrderFilter{
		Status: "IN_TRANSIT", ExternalOrderID: "SZ-1", PageSize: 100,
	}, "merchant-token")
	if err != nil {
		t.Fatalf("Orders: %v", err)
	}
	if !strings.Contains(queries[0], "pageSize=100") || !strings.Contains(queries[0], "status=IN_TRANSIT") || !strings.Contains(queries[0], "externalOrderId=SZ-1") {
		t.Fatalf("order query = %q", queries[0])
	}
	encoded, _ := json.Marshal(orders.Items)
	for _, forbidden := range []string{"0770000000", "recipientPhone", "merchantId"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("order leaked %q: %s", forbidden, encoded)
		}
	}

	failed := false
	deliveries, err := merchant.WebhookDeliveries(context.Background(), &failed, "merchant-token")
	if err != nil {
		t.Fatalf("WebhookDeliveries: %v", err)
	}
	if len(deliveries.Items) != 1 || deliveries.Items[0].HTTPStatus == nil || *deliveries.Items[0].HTTPStatus != 502 {
		t.Fatalf("webhook = %+v", deliveries.Items)
	}
	encoded, _ = json.Marshal(deliveries.Items)
	if strings.Contains(string(encoded), "secret") {
		t.Fatalf("webhook payload leaked: %s", encoded)
	}
}

func TestMerchantWebhookNotConfiguredCode(t *testing.T) {
	merchant, closeServer := newMerchantTestClient(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `{"error":{"code":"WEBHOOK_NOT_CONFIGURED","message":"no webhook is configured"},"meta":{}}`)
	})
	defer closeServer()

	_, err := merchant.WebhookDeliveries(context.Background(), nil, "merchant-token")
	var upstream *Error
	if !IsKind(err, KindNotFound) || !errors.As(err, &upstream) || upstream.Code != "WEBHOOK_NOT_CONFIGURED" {
		t.Fatalf("error = %v", err)
	}
}
