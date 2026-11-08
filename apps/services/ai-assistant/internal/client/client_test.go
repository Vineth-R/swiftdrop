package client

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func testConfig(t *testing.T, serverURL string) Config {
	t.Helper()
	return Config{BaseURL: serverURL, Timeout: time.Second}
}

func TestNormalizeBaseURL(t *testing.T) {
	valid := map[string]string{
		"http://pricing:8083":    "http://pricing:8083",
		"https://example.com/":   "https://example.com",
		" http://127.0.0.1:8082": "http://127.0.0.1:8082",
	}
	for input, want := range valid {
		got, err := normalizeBaseURL(input)
		if err != nil {
			t.Fatalf("normalizeBaseURL(%q) returned error: %v", input, err)
		}
		if got != want {
			t.Fatalf("normalizeBaseURL(%q) = %q, want %q", input, got, want)
		}
	}

	invalid := []string{
		"",
		"pricing:8083",
		"ftp://example.com",
		"http://user:pass@example.com",
		"http://example.com/path",
		"http://example.com/?q=1",
		"http://example.com/#fragment",
		"http://",
	}
	for _, input := range invalid {
		if _, err := normalizeBaseURL(input); err == nil {
			t.Fatalf("normalizeBaseURL(%q) should have failed", input)
		}
	}
}

func TestZonesSuccessAndNoAuthorization(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/zones" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public request leaked Authorization: %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"zone-1","code":"CMB","name":"Colombo","districts":["Colombo","Gampaha"],"isActive":true,"secret":"nope"}]}`)
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	zones, err := client.Zones(context.Background())
	if err != nil {
		t.Fatalf("Zones: %v", err)
	}
	if len(zones) != 1 || zones[0].ID != "zone-1" || zones[0].Code != "CMB" || zones[0].Name != "Colombo" {
		t.Fatalf("unexpected zones: %+v", zones)
	}
	if len(zones[0].Districts) != 2 || zones[0].Districts[1] != "Gampaha" {
		t.Fatalf("unexpected districts: %+v", zones[0].Districts)
	}
}

func TestEstimateExactRequestAndAmounts(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/pricing/estimate" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Content-Type"); got != "application/json" {
			t.Fatalf("Content-Type = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public request leaked Authorization: %q", got)
		}

		decoder := json.NewDecoder(r.Body)
		decoder.UseNumber()
		var body map[string]any
		if err := decoder.Decode(&body); err != nil {
			t.Fatalf("decode body: %v", err)
		}
		if len(body) != 4 {
			t.Fatalf("unexpected body keys: %+v", body)
		}
		if body["originZoneId"] != "origin-1" || body["destinationZoneId"] != "dest-1" {
			t.Fatalf("unexpected zone ids: %+v", body)
		}
		if body["weightKg"] != json.Number("2.5") {
			t.Fatalf("weightKg = %#v", body["weightKg"])
		}
		if body["sizeCategory"] != "SMALL" {
			t.Fatalf("sizeCategory = %#v", body["sizeCategory"])
		}

		_, _ = io.WriteString(w, `{"data":{"originZoneId":"origin-1","destinationZoneId":"dest-1","weightKg":2.50,"sizeCategory":"SMALL","currency":"LKR","baseFee":450.00,"weightCharge":100.00,"sizeSurcharge":0.00,"total":550.00}}`)
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	estimate, err := client.Estimate(context.Background(), EstimateInput{
		OriginZoneID:      "origin-1",
		DestinationZoneID: "dest-1",
		WeightKg:          json.Number("2.5"),
		SizeCategory:      "SMALL",
	})
	if err != nil {
		t.Fatalf("Estimate: %v", err)
	}
	if estimate.BaseFee != "450.00" || estimate.WeightCharge != "100.00" ||
		estimate.SizeSurcharge != "0.00" || estimate.Total != "550.00" {
		t.Fatalf("amounts were not preserved exactly: %+v", estimate)
	}
	if estimate.Currency != "LKR" {
		t.Fatalf("currency = %q", estimate.Currency)
	}
}

func TestEstimateRejectsMalformedSuccessWithoutFabricating(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":{"originZoneId":"origin-1","destinationZoneId":"dest-1","weightKg":2,"sizeCategory":"SMALL","currency":"LKR","baseFee":450,"weightCharge":100,"sizeSurcharge":0}}`)
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}
	_, err = client.Estimate(context.Background(), EstimateInput{
		OriginZoneID: "origin-1", DestinationZoneID: "dest-1",
		WeightKg: json.Number("2"), SizeCategory: "SMALL",
	})
	if !IsKind(err, KindUnavailable) {
		t.Fatalf("Estimate error = %v, want KindUnavailable", err)
	}
}

func TestInternalClientsDisableEnvironmentProxy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	pricing, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}
	parcel, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	for name, transport := range map[string]*http.Transport{
		"pricing": pricing.http.http.Transport.(*http.Transport),
		"parcel":  parcel.http.http.Transport.(*http.Transport),
	} {
		if transport.Proxy != nil {
			t.Fatalf("%s client uses an environment proxy", name)
		}
	}
}

func TestEstimateRejectsInvalidInputWithoutCall(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	_, err = client.Estimate(context.Background(), EstimateInput{OriginZoneID: "a", DestinationZoneID: "b", WeightKg: "not-a-number", SizeCategory: "SMALL"})
	if !IsKind(err, KindInvalid) {
		t.Fatalf("expected KindInvalid, got %v", err)
	}
	if calls.Load() != 0 {
		t.Fatalf("invalid input should not call upstream")
	}
}

func TestTrackingSanitizesUpstreamFields(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/parcels/tracking/SDK7M2Q9XP4" {
			t.Fatalf("unexpected path %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer token-a" {
			t.Fatalf("tracking request did not forward the caller token: %q", got)
		}
		_, _ = io.WriteString(w, `{"data":{
			"trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT",
			"pickupCity":"Colombo","dropoffCity":"Kandy",
			"lastUpdatedAt":"2026-11-01T01:00:00Z",
			"timeline":[{"status":"CREATED","at":"2026-11-01T00:00:00Z"}],
			"recipientName":"Private Person","recipientPhone":"+94770000000",
			"dropoffAddress":"1 Secret Road","internalId":"parcel-123","note":"gate code"
		}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}

	tracking, err := client.Tracking(context.Background(), "SDK7M2Q9XP4", "token-a")
	if err != nil {
		t.Fatalf("Tracking: %v", err)
	}

	encoded, err := json.Marshal(tracking)
	if err != nil {
		t.Fatalf("marshal tracking: %v", err)
	}
	for _, private := range []string{"recipientName", "recipientPhone", "dropoffAddress", "internalId", "note"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("tracking output leaked %q: %s", private, encoded)
		}
	}
	if !strings.Contains(string(encoded), `"trackingNumber":"SDK7M2Q9XP4"`) {
		t.Fatalf("tracking output missing allowed field: %s", encoded)
	}
}

func TestHistoryForwardsTokenAndSanitizes(t *testing.T) {
	const parcelID = "b1f0f9d0-2a63-4a1f-9d7e-3c2c2f4a5b6c"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/parcels/"+parcelID+"/history" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"previousStatus":null,"status":"CREATED","changedAt":"2026-11-01T00:00:00Z","note":"created","failureReason":"private failure","actorRole":"ADMIN"}]}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}

	items, err := client.History(context.Background(), parcelID, "secret-token")
	if err != nil {
		t.Fatalf("History: %v", err)
	}
	if len(items) != 1 || items[0].Status != "CREATED" || items[0].PreviousStatus != nil || items[0].Note == nil || *items[0].Note != "created" {
		t.Fatalf("unexpected history: %+v", items)
	}

	encoded, err := json.Marshal(items)
	if err != nil {
		t.Fatalf("marshal history: %v", err)
	}
	for _, private := range []string{"failureReason", "actorRole"} {
		if strings.Contains(string(encoded), private) {
			t.Fatalf("history output leaked %q: %s", private, encoded)
		}
	}
}

func TestHistoryOmitsAuthorizationWhenTokenEmpty(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("Authorization should be absent, got %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	if _, err := client.History(context.Background(), "b1f0f9d0-2a63-4a1f-9d7e-3c2c2f4a5b6c", ""); err != nil {
		t.Fatalf("History: %v", err)
	}
}

func TestListExactQueryAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/parcels" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "page=1&pageSize=20&status=IN_TRANSIT" {
			t.Fatalf("query = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secret-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT","recipientName":"Recipient","dropoffCity":"Kandy","total":550.00,"currency":"LKR","createdAt":"2026-11-01T00:00:00Z","dropoffAddress":"secret"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}

	list, err := client.List(context.Background(), "IN_TRANSIT", "secret-token")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].Total != "550.00" {
		t.Fatalf("unexpected list: %+v", list)
	}
	if list.Pagination.TotalItems != 1 || list.Pagination.Page != 1 {
		t.Fatalf("unexpected pagination: %+v", list.Pagination)
	}

	encoded, err := json.Marshal(list.Items)
	if err != nil {
		t.Fatalf("marshal list: %v", err)
	}
	if strings.Contains(string(encoded), "dropoffAddress") {
		t.Fatalf("list output leaked private field: %s", encoded)
	}
}

func TestListRejectsMalformedTotal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"data":[{"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT","recipientName":"Recipient","dropoffCity":"Kandy","currency":"LKR","createdAt":"2026-11-01T00:00:00Z"}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	if _, err := client.List(context.Background(), "", "token"); !IsKind(err, KindUnavailable) {
		t.Fatalf("List error = %v, want KindUnavailable", err)
	}
}

func TestListOmitsEmptyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RawQuery; got != "page=1&pageSize=20" {
			t.Fatalf("query = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":0,"totalPages":0}}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	list, err := client.List(context.Background(), "", "token")
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected empty list, got %+v", list.Items)
	}
}

func TestErrorMapping(t *testing.T) {
	cases := []struct {
		status int
		kind   Kind
		code   string
	}{
		{http.StatusUnauthorized, KindUnauthorized, "UNAUTHENTICATED"},
		{http.StatusForbidden, KindUnauthorized, "FORBIDDEN"},
		{http.StatusNotFound, KindNotFound, "TRACKING_NOT_FOUND"},
		{http.StatusBadRequest, KindInvalid, "BAD_REQUEST"},
		{http.StatusUnprocessableEntity, KindInvalid, "ROUTE_NOT_SERVICEABLE"},
		{http.StatusTooManyRequests, KindUnavailable, "RATE_LIMITED"},
		{http.StatusInternalServerError, KindUnavailable, "INTERNAL_ERROR"},
		{http.StatusServiceUnavailable, KindUnavailable, "SERVICE_UNAVAILABLE"},
	}

	for _, tc := range cases {
		t.Run(http.StatusText(tc.status), func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, `{"error":{"code":"`+tc.code+`","message":"sensitive upstream detail"},"meta":{"requestId":"req"}}`)
			}))
			defer server.Close()

			client, err := NewPricingClient(testConfig(t, server.URL))
			if err != nil {
				t.Fatalf("NewPricingClient: %v", err)
			}

			_, err = client.Zones(context.Background())
			if err == nil {
				t.Fatalf("expected error for status %d", tc.status)
			}
			var typed *Error
			if !errors.As(err, &typed) {
				t.Fatalf("expected *Error, got %T", err)
			}
			if typed.Kind != tc.kind || typed.Status != tc.status || typed.Code != tc.code {
				t.Fatalf("unexpected error: %+v", typed)
			}
			if strings.Contains(err.Error(), "sensitive upstream detail") {
				t.Fatalf("error leaked upstream body: %v", err)
			}
		})
	}
}

func TestTimeoutIsUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(100 * time.Millisecond)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer server.Close()

	client, err := NewPricingClient(Config{BaseURL: server.URL, Timeout: 10 * time.Millisecond})
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	_, err = client.Zones(context.Background())
	if !IsKind(err, KindUnavailable) {
		t.Fatalf("expected KindUnavailable, got %v", err)
	}
}

func TestOversizedBodyIsRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(strings.Repeat("x", maxResponseBytes+1)))
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	_, err = client.Zones(context.Background())
	if !IsKind(err, KindUnavailable) {
		t.Fatalf("expected KindUnavailable for oversized body, got %v", err)
	}
}

func TestRedirectsAreNotFollowed(t *testing.T) {
	var redirected atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		redirected.Add(1)
		_, _ = io.WriteString(w, `{"data":[]}`)
	}))
	defer target.Close()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer server.Close()

	client, err := NewPricingClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewPricingClient: %v", err)
	}

	_, err = client.Zones(context.Background())
	if err == nil {
		t.Fatalf("expected redirect to fail")
	}
	if redirected.Load() != 0 {
		t.Fatalf("client followed a redirect")
	}
	if !IsKind(err, KindUnavailable) {
		t.Fatalf("expected KindUnavailable, got %v", err)
	}
}

func TestAdminParcelsExactQueryAndAuth(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/admin/parcels" {
			t.Fatalf("unexpected request %s %s", r.Method, r.URL.Path)
		}
		if got := r.URL.RawQuery; got != "page=1&pageSize=20&status=IN_TRANSIT" {
			t.Fatalf("query = %q", got)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer dispatcher-token" {
			t.Fatalf("Authorization = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[{
			"id":"p1","trackingNumber":"SDK7M2Q9XP4","status":"IN_TRANSIT",
			"pickupCity":"Colombo","dropoffCity":"Kandy","courierId":"c1",
			"customerId":"private-customer","merchantId":"private-merchant",
			"total":550.00,"createdAt":"2026-11-01T00:00:00Z"
		}],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":1,"totalPages":1}}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}

	list, err := client.AdminParcels(context.Background(), "IN_TRANSIT", "dispatcher-token")
	if err != nil {
		t.Fatalf("AdminParcels: %v", err)
	}
	if len(list.Items) != 1 || list.Items[0].TrackingNumber != "SDK7M2Q9XP4" {
		t.Fatalf("unexpected admin list: %+v", list)
	}
	if list.Items[0].CourierID == nil || *list.Items[0].CourierID != "c1" {
		t.Fatalf("courier id = %+v", list.Items[0].CourierID)
	}
	if list.Pagination.TotalItems != 1 {
		t.Fatalf("pagination = %+v", list.Pagination)
	}

	encoded, err := json.Marshal(list.Items)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	for _, forbidden := range []string{"customerId", "merchantId", "private-customer", "private-merchant"} {
		if strings.Contains(string(encoded), forbidden) {
			t.Fatalf("admin list leaked %q: %s", forbidden, encoded)
		}
	}
}

func TestAdminParcelsOmitsEmptyStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.URL.RawQuery; got != "page=1&pageSize=20" {
			t.Fatalf("query = %q", got)
		}
		_, _ = io.WriteString(w, `{"data":[],"meta":{"pagination":{"page":1,"pageSize":20,"totalItems":0,"totalPages":0}}}`)
	}))
	defer server.Close()

	client, err := NewParcelClient(testConfig(t, server.URL))
	if err != nil {
		t.Fatalf("NewParcelClient: %v", err)
	}
	list, err := client.AdminParcels(context.Background(), "", "token")
	if err != nil {
		t.Fatalf("AdminParcels: %v", err)
	}
	if len(list.Items) != 0 {
		t.Fatalf("expected empty list, got %+v", list.Items)
	}
}
