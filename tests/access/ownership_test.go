//go:build access

package access

import (
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

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
	fixtureMarker := "M5 access ownership fixture " + randomHex(8)
	parcelID, trackingNumber := s.createParcel(t, ownerSession.AccessToken, fixtureMarker)
	t.Logf("retained append-only tagged parcel fixture %s; use a disposable local stack", parcelID)
	for _, suffix := range []string{"", "/history", "/receipt"} {
		path := "/api/v1/parcels/" + parcelID + suffix
		owned := s.mustDo(t, http.MethodGet, path, "", bearer(ownerSession.AccessToken))
		if owned.status != http.StatusOK {
			t.Fatalf("owner read %s: HTTP %d", suffix, owned.status)
		}
		if suffix == "/receipt" && (owned.header.Get("Content-Type") != "application/pdf" || !strings.HasPrefix(string(owned.body), "%PDF-") || owned.header.Get("Content-Disposition") != `attachment; filename="receipt_`+trackingNumber+`.pdf"`) {
			t.Fatal("owner receipt is not the parcel PDF")
		}
		denied := s.mustDo(t, http.MethodGet, path, "", bearer(otherSession.AccessToken))
		if denied.status != http.StatusNotFound || denied.code() != "NOT_FOUND" {
			t.Fatalf("cross-customer %s: HTTP %d code %s; want 404 NOT_FOUND", suffix, denied.status, denied.code())
		}
		for _, private := range []string{parcelID, trackingNumber, "Ownership Recipient", "+94712345678", "45 Test Street", fixtureMarker} {
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
			found = found || row.ID == parcelID
		}
		if found != caller.wantParcel {
			t.Fatalf("%s list contains fixture=%t, want %t", caller.name, found, caller.wantParcel)
		}
	}
}

func TestCourierParcelOwnership(t *testing.T) {
	s := useStack(t)
	courierA := s.userIDs[auth.RoleCourier]
	courierAToken := s.tokens[auth.RoleCourier]
	if err := s.sql(fmt.Sprintf(
		`INSERT INTO identity.courier_profiles (user_id, is_available) VALUES ('%s', true)
		 ON CONFLICT (user_id) DO UPDATE SET is_available = true;`, courierA)); err != nil {
		t.Fatal(err)
	}
	courierB := s.newCourier(t)
	loginB, sessionB := s.login(t, courierB.email, courierB.password)
	if loginB.status != http.StatusOK || sessionB.AccessToken == "" {
		t.Fatalf("courier B login: HTTP %d", loginB.status)
	}

	customer := s.newCustomer(t)
	login, ownerSession := s.login(t, customer.email, customer.password)
	if login.status != http.StatusOK || ownerSession.AccessToken == "" {
		t.Fatalf("customer login: HTTP %d", login.status)
	}
	parcelID, _ := s.createParcel(t, ownerSession.AccessToken, "M7 courier ownership fixture "+randomHex(8))
	t.Logf("retained append-only tagged courier fixture %s; use a disposable local stack", parcelID)

	assigned := s.mustDo(t, http.MethodPut, "/api/v1/dispatch/parcels/"+parcelID+"/assignment",
		`{"courierId":"`+courierA+`","reason":"access ownership fixture"}`, bearer(s.tokens[auth.RoleDispatcher]))
	if assigned.status != http.StatusOK {
		t.Fatalf("assign courier A: HTTP %d %s", assigned.status, assigned.body)
	}

	// Courier A uses the trailing-slash route through the gateway so the
	// gateway rewrite and the second registration are both exercised end to end.
	updated := s.mustDo(t, http.MethodPost, "/api/v1/courier/deliveries/"+parcelID+"/status/",
		`{"status":"PICKED_UP","note":"courier A owns this parcel"}`, bearer(courierAToken))
	if updated.status != http.StatusOK {
		t.Fatalf("courier A update: HTTP %d %s", updated.status, updated.body)
	}

	// Exercise both route forms to prove the trailing-slash registration is
	// guarded by the same ownership and role checks.
	for _, suffix := range []string{"/status", "/status/"} {
		denied := s.mustDo(t, http.MethodPost, "/api/v1/courier/deliveries/"+parcelID+suffix,
			`{"status":"IN_TRANSIT"}`, bearer(sessionB.AccessToken))
		if denied.status != http.StatusForbidden || denied.code() != "FORBIDDEN" {
			t.Fatalf("courier B update %s: HTTP %d code %s; want 403 FORBIDDEN", suffix, denied.status, denied.code())
		}
		for _, private := range []string{"owner", "recipient", "tracking"} {
			if strings.Contains(string(denied.body), private) {
				t.Fatalf("rejected %s response disclosed %q", suffix, private)
			}
		}
	}

	detail := s.mustDo(t, http.MethodGet, "/api/v1/courier/deliveries/"+parcelID, "", bearer(courierAToken))
	var parcel struct {
		Status  string `json:"status"`
		History []struct {
			Status string `json:"status"`
		} `json:"history"`
	}
	if err := detail.data(&parcel); err != nil || detail.status != http.StatusOK {
		t.Fatalf("courier A detail: HTTP %d; decode %v", detail.status, err)
	}
	if parcel.Status != "PICKED_UP" || len(parcel.History) != 2 {
		t.Fatalf("rejected update changed parcel: status=%q history=%d", parcel.Status, len(parcel.History))
	}
	historyCount := s.sqlQuery(t, fmt.Sprintf(
		`SELECT count(*) FROM parcel.parcel_status_history WHERE parcel_id = '%s';`, parcelID))
	eventCount := s.sqlQuery(t, fmt.Sprintf(
		`SELECT count(*) FROM parcel.outbox_events WHERE aggregate_id = '%s' AND event_type = 'parcel.status_changed';`, parcelID))
	if historyCount != "2" {
		t.Fatalf("status history rows = %s, want 2 (CREATED and PICKED_UP only)", historyCount)
	}
	if eventCount != "1" {
		t.Fatalf("status_changed outbox rows = %s, want 1 (courier A only)", eventCount)
	}
}

func TestMerchantOrderOwnership(t *testing.T) {
	s := useStack(t)
	cmb := s.sqlQuery(t, `SELECT id::text FROM pricing.zones WHERE code = 'CMB'`)
	kdy := s.sqlQuery(t, `SELECT id::text FROM pricing.zones WHERE code = 'KDY'`)
	merchantA := s.userIDs[auth.RoleMerchant]
	if err := s.sql(fmt.Sprintf(
		`UPDATE merchant.merchants SET pickup_zone_id = '%s' WHERE owner_user_id = '%s';`,
		cmb, merchantA)); err != nil {
		t.Fatal(err)
	}

	keyResponse := s.mustDo(t, http.MethodPost, "/api/v1/merchants/me/keys",
		`{"name":"access ownership fixture"}`, bearer(s.tokens[auth.RoleMerchant]))
	var issued struct {
		APIKey string `json:"apiKey"`
	}
	if err := keyResponse.data(&issued); err != nil || keyResponse.status != http.StatusCreated || issued.APIKey == "" {
		t.Fatalf("create merchant key: HTTP %d; decode %v", keyResponse.status, err)
	}
	externalOrderID := "ACCESS-" + randomHex(8)
	body := fmt.Sprintf(`{
		"externalOrderId": %q,
		"recipient": {"name":"Ownership Recipient","phone":"+94712345678","address":"45 Test Street","city":"Kandy","zoneId":%q},
		"weightKg": 1.5,
		"sizeCategory": "SMALL"
	}`, externalOrderID, kdy)
	created := s.mustDo(t, http.MethodPost, "/api/v1/merchant/orders", body,
		map[string]string{"X-API-Key": issued.APIKey})
	var order struct {
		OrderID        string `json:"orderId"`
		TrackingNumber string `json:"trackingNumber"`
	}
	if err := created.data(&order); err != nil || created.status != http.StatusCreated || order.OrderID == "" {
		t.Fatalf("create merchant order: HTTP %d; decode %v", created.status, err)
	}

	otherEmail := "access.merchant.b." + randomHex(4) + "@example.test"
	otherID, err := s.register(otherEmail, "Str0ngPass!")
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sql(fmt.Sprintf(
		`UPDATE identity.users SET role_id = (SELECT id FROM identity.roles WHERE name = 'MERCHANT'), email_verified = true WHERE id = '%s';`, otherID)); err != nil {
		t.Fatal(err)
	}
	if err := s.sql(fmt.Sprintf(
		`INSERT INTO merchant.merchants (owner_user_id, business_name, business_reg_no, business_address, contact_email, contact_phone, pickup_address, pickup_zone_id, status, approved_by, approved_at)
		 VALUES ('%s', 'Access Merchant B', 'ACCESS-B-%s', '1 Test Road', 'merchantb@example.test', '+94771234567', '1 Test Road', '%s', 'APPROVED', '%s', now());`,
		otherID, randomHex(4), cmb, s.userIDs[auth.RoleAdmin])); err != nil {
		t.Fatal(err)
	}
	login, sessionB := s.login(t, otherEmail, "Str0ngPass!")
	if login.status != http.StatusOK || sessionB.AccessToken == "" {
		t.Fatalf("merchant B login: HTTP %d", login.status)
	}

	denied := s.mustDo(t, http.MethodGet, "/api/v1/merchant/orders/"+order.OrderID, "", bearer(sessionB.AccessToken))
	if denied.status != http.StatusNotFound || denied.code() != "NOT_FOUND" {
		t.Fatalf("cross-merchant order: HTTP %d code %s; want 404 NOT_FOUND", denied.status, denied.code())
	}
	for _, private := range []string{order.OrderID, order.TrackingNumber, externalOrderID, "Ownership Recipient", "45 Test Street"} {
		if strings.Contains(string(denied.body), private) {
			t.Fatalf("cross-merchant response disclosed %q", private)
		}
	}
}
