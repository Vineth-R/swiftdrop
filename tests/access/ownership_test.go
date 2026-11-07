//go:build access

package access

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// ownershipCases are the "you can only touch your own" rules from the team task
// sheet. Each needs two real accounts of the same role and a record owned by
// one of them, so each needs the owning service's create endpoint. They are
// listed here with their exact request and answer, so the owner turns each
// into a real test the day the endpoint exists: create the record as account A,
// repeat Request as account B, expect Want, and delete the Skip.
//
// Keep Want exactly as written. A 404 for another customer's parcel (not 403)
// is deliberate, so a caller cannot learn that the parcel exists.
var ownershipCases = []struct {
	rule    string
	request string
	want    string
	blocked string
}{
	{
		rule:    "a courier cannot update another courier's parcel",
		request: "POST /api/v1/courier/deliveries/{parcelId}/status as courier B, for a parcel assigned to courier A (API 6.3)",
		want:    "403 FORBIDDEN, and the parcel status is unchanged",
		blocked: "the parcel service has no courier delivery routes or dispatch assignment yet",
	},
	{
		rule:    "a merchant cannot see another merchant's order",
		request: "GET /api/v1/merchant/orders/{orderId} as merchant B, for an order merchant A created (API 8.18)",
		want:    "404 NOT_FOUND, with no part of the order in the body",
		blocked: "the merchant service has no order routes yet and does not verify access tokens",
	},
}

// TestCustomerParcelOwnership runs through the public gateway with real login,
// pricing and parcel persistence. Run the access suite only on a disposable
// local stack: its tagged parcel/history/outbox fixture is deliberately retained
// because status history is append-only. Account cleanup remains unchanged.
func TestCustomerParcelOwnership(t *testing.T) {
	s := useStack(t)
	owner, other := s.newCustomer(t), s.newCustomer(t)
	ownerLogin, ownerSession := s.login(t, owner.email, owner.password)
	otherLogin, otherSession := s.login(t, other.email, other.password)
	if ownerLogin.status != http.StatusOK || otherLogin.status != http.StatusOK || ownerSession.AccessToken == "" || otherSession.AccessToken == "" {
		t.Fatal("both customer fixtures must log in successfully")
	}
	zonesReply := s.mustDo(t, http.MethodGet, "/api/v1/zones", "", nil)
	var zones []struct{ ID, Code, Name string }
	if err := zonesReply.data(&zones); err != nil || zonesReply.status != http.StatusOK {
		t.Fatalf("read real pricing zones: HTTP %d; decode %v", zonesReply.status, err)
	}
	var origin, destination string
	for _, zone := range zones {
		switch zone.Code {
		case "CMB":
			origin = zone.ID
		case "KDY":
			destination = zone.ID
		}
	}
	if origin == "" || destination == "" {
		t.Fatal("the disposable access stack needs the migration-seeded CMB and KDY pricing zones")
	}
	estimateBody, err := json.Marshal(map[string]any{"originZoneId": origin, "destinationZoneId": destination, "weightKg": 2, "sizeCategory": "SMALL"})
	if err != nil {
		t.Fatal(err)
	}
	estimate := s.mustDo(t, http.MethodPost, "/api/v1/pricing/estimate", string(estimateBody), nil)
	var fee struct{ Total json.Number }
	if err := estimate.data(&fee); err != nil || estimate.status != http.StatusOK || fee.Total == "" {
		t.Fatalf("obtain real pricing estimate: HTTP %d; decode %v", estimate.status, err)
	}
	fixtureMarker := "M5 access ownership fixture " + randomHex(8)
	createBody, err := json.Marshal(map[string]any{
		"sender":    map[string]string{"name": "Ownership Sender", "phone": "+94771234567", "address": "12 Test Road", "city": "Colombo", "zoneId": origin},
		"recipient": map[string]string{"name": "Ownership Recipient", "phone": "+94712345678", "address": "45 Test Street", "city": "Kandy", "zoneId": destination},
		"weightKg":  2, "sizeCategory": "SMALL", "description": fixtureMarker, "expectedFee": fee.Total,
	})
	if err != nil {
		t.Fatal(err)
	}
	created := s.mustDo(t, http.MethodPost, "/api/v1/parcels", string(createBody), bearer(ownerSession.AccessToken))
	var parcel struct{ ID, TrackingNumber string }
	if err := created.data(&parcel); err != nil || created.status != http.StatusCreated || parcel.ID == "" || parcel.TrackingNumber == "" {
		t.Fatalf("create ownership fixture: HTTP %d; decode %v", created.status, err)
	}
	t.Logf("retained append-only tagged parcel fixture %s; use a disposable local stack", parcel.ID)
	for _, suffix := range []string{"", "/history", "/receipt"} {
		path := "/api/v1/parcels/" + parcel.ID + suffix
		owned := s.mustDo(t, http.MethodGet, path, "", bearer(ownerSession.AccessToken))
		if owned.status != http.StatusOK {
			t.Fatalf("owner read %s: HTTP %d", suffix, owned.status)
		}
		if suffix == "/receipt" && (owned.header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(owned.body), "%PDF-") || owned.header.Get("Content-Disposition") != `attachment; filename="receipt_`+parcel.TrackingNumber+`.pdf"`) {
			t.Fatal("owner receipt is not the parcel PDF")
		}
		denied := s.mustDo(t, http.MethodGet, path, "", bearer(otherSession.AccessToken))
		if denied.status != http.StatusNotFound || denied.code() != "NOT_FOUND" {
			t.Fatalf("cross-customer %s: HTTP %d code %s; want 404 NOT_FOUND", suffix, denied.status, denied.code())
		}
		for _, private := range []string{parcel.ID, parcel.TrackingNumber, "Ownership Recipient", "+94712345678", "45 Test Street", fixtureMarker} {
			if strings.Contains(string(denied.body), private) {
				t.Fatalf("cross-customer %s response disclosed parcel data", suffix)
			}
		}
	}
	for _, caller := range []struct {
		name, token string
		wantParcel  bool
	}{{"owner", ownerSession.AccessToken, true}, {"other customer", otherSession.AccessToken, false}} {
		listed := s.mustDo(t, http.MethodGet, "/api/v1/parcels?status=CREATED&page=1&pageSize=100", "", bearer(caller.token))
		var rows []struct{ ID string }
		if err := listed.data(&rows); err != nil || listed.status != http.StatusOK {
			t.Fatalf("%s parcel list: HTTP %d; decode %v", caller.name, listed.status, err)
		}
		found := false
		for _, row := range rows {
			found = found || row.ID == parcel.ID
		}
		if found != caller.wantParcel {
			t.Fatalf("%s list contains fixture=%t, want %t", caller.name, found, caller.wantParcel)
		}
	}
}

func TestOwnership(t *testing.T) {
	for _, c := range ownershipCases {
		t.Run(c.rule, func(t *testing.T) {
			t.Skipf("not checkable yet: %s.\nRequest: %s\nExpect: %s", c.blocked, c.request, c.want)
		})
	}
}
