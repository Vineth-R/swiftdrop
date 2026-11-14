package eventschema

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

const statusEvent = `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-14T10:00:00Z","producer":"parcel","data":{"parcel_id":"22222222-2222-4222-8222-222222222222","tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP"}}`

func TestValidateAcceptsAValidEvent(t *testing.T) {
	if err := Validate([]byte(statusEvent)); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestValidateRejectsInvalidEvents(t *testing.T) {
	tests := map[string]string{
		"missing required field":   `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-14T10:00:00Z","producer":"parcel","data":{"parcel_id":"22222222-2222-4222-8222-222222222222","tracking_number":"SDK7M2Q9XP4","status":"PICKED_UP"}}`,
		"wrong field type":         `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-14T10:00:00Z","producer":"parcel","data":{"parcel_id":7,"tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP"}}`,
		"unexpected payload field": `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"parcel.status_changed.v1","event_version":1,"occurred_at":"2026-11-14T10:00:00Z","producer":"parcel","data":{"parcel_id":"22222222-2222-4222-8222-222222222222","tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP","unexpected":true}}`,
		"invalid version":          `{"event_id":"11111111-1111-4111-8111-111111111111","event_type":"parcel.status_changed.v1","event_version":2,"occurred_at":"2026-11-14T10:00:00Z","producer":"parcel","data":{"parcel_id":"22222222-2222-4222-8222-222222222222","tracking_number":"SDK7M2Q9XP4","previous_status":"CREATED","status":"PICKED_UP"}}`,
		"malformed json":           `{`,
	}
	for name, body := range tests {
		t.Run(name, func(t *testing.T) {
			if err := Validate([]byte(body)); !errors.Is(err, ErrInvalidEvent) {
				t.Fatalf("Validate() error = %v, want ErrInvalidEvent", err)
			}
		})
	}
}

func TestValidateAllowsAnUnknownEventType(t *testing.T) {
	if err := Validate([]byte(`{"event_type":"other.event.v1"}`)); err != nil {
		t.Fatalf("Validate() error = %v", err)
	}
}

func TestEmbeddedSchemasMatchContracts(t *testing.T) {
	contractDir := filepath.Join("..", "..", "..", "contracts", "events")
	entries, err := os.ReadDir("schemas")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		embedded, err := os.ReadFile(filepath.Join("schemas", entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		contract, err := os.ReadFile(filepath.Join(contractDir, entry.Name()))
		if err != nil {
			t.Fatalf("read contract %s: %v", entry.Name(), err)
		}
		if string(embedded) != string(contract) {
			t.Fatalf("%s differs from contracts/events", entry.Name())
		}
	}
}
