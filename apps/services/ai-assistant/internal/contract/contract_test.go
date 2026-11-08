package contract

import "testing"

func TestCanonicalToolKeyIsOrderIndependent(t *testing.T) {
	first := CanonicalToolKey("calculate_delivery_fee", map[string]any{
		"origin": "Colombo", "destination": "Kandy", "weightKg": float64(2),
	})
	second := CanonicalToolKey("calculate_delivery_fee", map[string]any{
		"weightKg": float64(2), "destination": "Kandy", "origin": "Colombo",
	})
	if first != second {
		t.Fatalf("canonical keys differ:\n%s\n%s", first, second)
	}
}

func TestCanonicalToolKeyDistinguishesDelimiters(t *testing.T) {
	first := CanonicalToolKey("get_my_parcel_history", map[string]any{"parcelId": "a;b=c"})
	second := CanonicalToolKey("get_my_parcel_history", map[string]any{"parcelId": "a", "b": "c"})
	if first == second {
		t.Fatalf("distinct arguments collided: %s", first)
	}
}

func TestCanonicalToolKeyHandlesNestedArguments(t *testing.T) {
	key := CanonicalToolKey("tool", map[string]any{"filter": map[string]any{"status": "OK"}})
	if key == "tool" {
		t.Fatal("nested arguments were not encoded")
	}
}
