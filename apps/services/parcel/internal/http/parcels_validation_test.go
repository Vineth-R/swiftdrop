package servicehttp

import (
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

func TestParcelNULRequestsReturn422(t *testing.T) {
	testParcelInvalidTextRequestsReturn422(t, false)
}

func TestParcelOversizedRawTextRequestsReturn422(t *testing.T) {
	testParcelInvalidTextRequestsReturn422(t, true)
}

func testParcelInvalidTextRequestsReturn422(t *testing.T, oversized bool) {
	t.Helper()
	secret := strings.Repeat("s", 32)
	manager, err := auth.NewTokenManager(secret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.Issue("11111111-1111-4111-8111-111111111111", auth.RoleCustomer)
	if err != nil {
		t.Fatal(err)
	}
	router, err := New(slog.New(slog.NewTextHandler(io.Discard, nil)), Options{AccessSecret: secret, InternalToken: "test-internal-token"})
	if err != nil {
		t.Fatal(err)
	}
	for _, merchant := range []bool{false, true} {
		path, sender := "/parcels", "sender"
		if merchant {
			path, sender = "/internal/parcels", "pickup"
		}
		fields := []string{sender + ".name", sender + ".phone", sender + ".address", sender + ".city", "recipient.name", "recipient.phone", "recipient.address", "recipient.city", "description"}
		if oversized {
			fields = []string{sender + ".name", sender + ".address", sender + ".city", "recipient.name", "recipient.address", "recipient.city"}
		} else if merchant {
			fields = append(fields, "externalOrderId", "idempotencyKey")
		}
		for _, field := range fields {
			t.Run(path+"/"+field, func(t *testing.T) {
				body := map[string]any{
					sender:      map[string]any{"name": "Sender", "phone": "+94771234567", "address": "12 Road", "city": "Colombo", "zoneId": "11111111-1111-4111-8111-111111111111"},
					"recipient": map[string]any{"name": "Recipient", "phone": "+94712345678", "address": "45 Road", "city": "Kandy", "zoneId": "22222222-2222-4222-8222-222222222222"},
					"weightKg":  2, "sizeCategory": "SMALL", "description": "Books",
				}
				if merchant {
					body["merchantId"] = "22222222-2222-4222-8222-222222222222"
					body["createdByUserId"] = "11111111-1111-4111-8111-111111111111"
					body["externalOrderId"] = "SHOP-100"
					body["idempotencyKey"] = "merchant-key"
				} else {
					body["expectedFee"] = 550
				}
				parts := strings.Split(field, ".")
				value := "PRIVATE-M5-INPUT\x00"
				if oversized {
					maximum := map[string]int{"name": 150, "address": 255, "city": 100}[parts[1]]
					value = " " + strings.Repeat("é", maximum) + " "
				}
				if len(parts) == 2 {
					body[parts[0]].(map[string]any)[parts[1]] = value
				} else {
					body[field] = value
				}
				raw, err := json.Marshal(body)
				if err != nil {
					t.Fatal(err)
				}
				request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(string(raw)))
				request.Header.Set("Content-Type", "application/json")
				if merchant {
					request.Header.Set("X-Internal-Token", "test-internal-token")
				} else {
					request.Header.Set("Authorization", "Bearer "+token)
				}
				recorder := httptest.NewRecorder()
				router.ServeHTTP(recorder, request)
				var result struct {
					Error struct{ Code string } `json:"error"`
				}
				if err := json.Unmarshal(recorder.Body.Bytes(), &result); err != nil || recorder.Code != http.StatusUnprocessableEntity || result.Error.Code != "VALIDATION_ERROR" {
					t.Fatalf("invalid text HTTP %d code %s decode %v; want 422 VALIDATION_ERROR", recorder.Code, result.Error.Code, err)
				}
				if strings.Contains(recorder.Body.String(), "PRIVATE-M5-INPUT") {
					t.Fatal("validation response exposed input values")
				}
			})
		}
	}
}
