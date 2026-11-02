package service

import (
	"context"
	"errors"
	"testing"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	knownID   = "3f1c7a52-0000-4000-8000-000000000001"
	unknownID = "3f1c7a52-0000-4000-8000-0000000000ff"
)

type fakeInternalStore struct {
	eligibility map[string]repository.CourierEligibility
	contacts    map[string]repository.UserContact
	err         error
	calls       int
}

func (f *fakeInternalStore) CourierEligibility(_ context.Context, id string) (repository.CourierEligibility, error) {
	f.calls++
	if f.err != nil {
		return repository.CourierEligibility{}, f.err
	}
	if found, ok := f.eligibility[id]; ok {
		return found, nil
	}
	return repository.CourierEligibility{}, repository.ErrNotFound
}

func (f *fakeInternalStore) UserContact(_ context.Context, id string) (repository.UserContact, error) {
	f.calls++
	if f.err != nil {
		return repository.UserContact{}, f.err
	}
	if found, ok := f.contacts[id]; ok {
		return found, nil
	}
	return repository.UserContact{}, repository.ErrNotFound
}

func newInternalFixture(t *testing.T) (*InternalAPI, *fakeInternalStore) {
	t.Helper()
	phone := "+94771234567"
	store := &fakeInternalStore{
		eligibility: map[string]repository.CourierEligibility{
			knownID: {IsActive: true, IsAvailable: true, Role: auth.RoleCourier},
		},
		contacts: map[string]repository.UserContact{
			knownID: {FullName: "Nimal Perera", Email: "nimal@example.com", Phone: &phone},
		},
	}
	api, err := NewInternalAPI(store)
	if err != nil {
		t.Fatalf("NewInternalAPI returned error: %v", err)
	}
	return api, store
}

func TestNewInternalAPI_requiresAStore(t *testing.T) {
	if _, err := NewInternalAPI(nil); err == nil {
		t.Fatal("NewInternalAPI accepted a nil store")
	}
}

func TestValidUUID(t *testing.T) {
	valid := []string{knownID, "3F1C7A52-0000-4000-8000-00000000ABCD"}
	invalid := []string{"", "abc", "3f1c7a52-0000-4000-8000-00000000000", knownID + "0",
		"3f1c7a52000040008000000000000001", "3f1c7a52-0000-4000-8000-00000000000g", "' OR 1=1 --", knownID + "\n"}
	for _, value := range valid {
		if !ValidUUID(value) {
			t.Errorf("ValidUUID(%q) = false, want true", value)
		}
	}
	for _, value := range invalid {
		if ValidUUID(value) {
			t.Errorf("ValidUUID(%q) = true, want false", value)
		}
	}
}

func TestInternalAPI_CourierEligibility(t *testing.T) {
	api, store := newInternalFixture(t)
	store.eligibility["inactive"] = repository.CourierEligibility{}
	store.eligibility["3f1c7a52-0000-4000-8000-000000000002"] = repository.CourierEligibility{IsActive: false, IsAvailable: true, Role: auth.RoleCourier}
	store.eligibility["3f1c7a52-0000-4000-8000-000000000003"] = repository.CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleCourier}
	store.eligibility["3f1c7a52-0000-4000-8000-000000000004"] = repository.CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleDispatcher}

	tests := []struct {
		name string
		id   string
		want CourierEligibility
	}{
		{name: "eligible courier", id: knownID, want: CourierEligibility{IsActive: true, IsAvailable: true, Role: auth.RoleCourier}},
		{name: "inactive courier", id: "3f1c7a52-0000-4000-8000-000000000002", want: CourierEligibility{IsActive: false, IsAvailable: true, Role: auth.RoleCourier}},
		{name: "unavailable courier", id: "3f1c7a52-0000-4000-8000-000000000003", want: CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleCourier}},
		{name: "user who is not a courier", id: "3f1c7a52-0000-4000-8000-000000000004", want: CourierEligibility{IsActive: true, IsAvailable: false, Role: auth.RoleDispatcher}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := api.CourierEligibility(context.Background(), test.id)
			if err != nil || got != test.want {
				t.Fatalf("CourierEligibility() = %+v, %v, want %+v", got, err, test.want)
			}
		})
	}
}

func TestInternalAPI_unknownAndMalformedIDsAreNotFound(t *testing.T) {
	api, store := newInternalFixture(t)

	for _, id := range []string{unknownID, "not-a-uuid", "", "' OR 1=1 --"} {
		if _, err := api.CourierEligibility(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("CourierEligibility(%q) error = %v, want ErrUserNotFound", id, err)
		}
		if _, err := api.UserContact(context.Background(), id); !errors.Is(err, ErrUserNotFound) {
			t.Fatalf("UserContact(%q) error = %v, want ErrUserNotFound", id, err)
		}
	}
	// Malformed IDs must not even reach the database; only unknownID does.
	if store.calls != 2 {
		t.Fatalf("store was called %d times, want 2 (one per method for the well-formed ID)", store.calls)
	}
}

func TestInternalAPI_UserContact(t *testing.T) {
	api, store := newInternalFixture(t)
	store.contacts["3f1c7a52-0000-4000-8000-000000000002"] = repository.UserContact{FullName: "Google Person", Email: "g@example.com"}

	got, err := api.UserContact(context.Background(), knownID)
	if err != nil || got != (UserContact{FullName: "Nimal Perera", Email: "nimal@example.com", Phone: "+94771234567"}) {
		t.Fatalf("UserContact() = %+v, %v", got, err)
	}

	got, err = api.UserContact(context.Background(), "3f1c7a52-0000-4000-8000-000000000002")
	if err != nil || got.Phone != "" || got.Email != "g@example.com" {
		t.Fatalf("UserContact() for a user without a phone = %+v, %v", got, err)
	}
}

func TestInternalAPI_storeFailureIsNotReportedAsNotFound(t *testing.T) {
	api, store := newInternalFixture(t)
	store.err = errStore

	if _, err := api.CourierEligibility(context.Background(), knownID); !errors.Is(err, errStore) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("CourierEligibility() error = %v, want the store error", err)
	}
	if _, err := api.UserContact(context.Background(), knownID); !errors.Is(err, errStore) || errors.Is(err, ErrUserNotFound) {
		t.Fatalf("UserContact() error = %v, want the store error", err)
	}
}
