package seed

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

const (
	testOwnerUserID = "3f1c7a52-0000-4000-8000-0000000000b1"
	testZoneID      = "3f1c7a52-0000-4000-8000-0000000000aa"
)

func testConfig(t *testing.T) Config {
	t.Helper()
	config, err := LoadConfig(newLoader(validValues()))
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	return config
}

type fakeZones struct {
	zone pricingclient.Zone
	err  error
}

func (f fakeZones) ZoneByCode(_ context.Context, code string) (pricingclient.Zone, error) {
	if f.err != nil {
		return pricingclient.Zone{}, f.err
	}
	if !strings.EqualFold(f.zone.Code, code) {
		return pricingclient.Zone{}, errors.New("zone not found")
	}
	return f.zone, nil
}

func activeZone() pricingclient.Zone {
	return pricingclient.Zone{ID: testZoneID, Code: "CMB", Name: "Colombo", IsActive: true}
}

type identityState struct {
	mu            sync.Mutex
	createStatus  int
	createCreated bool
	userID        string
	creates       int
	lifecycles    int
	contacts      int
	lastToken     string
}

func newTestIdentity(t *testing.T, state *identityState) *identityclient.Client {
	t.Helper()
	if state.userID == "" {
		state.userID = testOwnerUserID
	}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		state.mu.Lock()
		state.lastToken = r.Header.Get("X-Internal-Token")
		state.mu.Unlock()

		if r.Header.Get("X-Internal-Token") != testInternalToken {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/internal/merchant-owners":
			state.mu.Lock()
			state.creates++
			state.mu.Unlock()
			if state.createStatus == http.StatusConflict {
				writeEnvelope(t, w, http.StatusConflict, map[string]any{
					"error": map[string]string{"code": "EMAIL_ALREADY_EXISTS"},
				})
				return
			}
			status := http.StatusOK
			if state.createCreated {
				status = http.StatusCreated
			}
			writeEnvelope(t, w, status, map[string]any{
				"data": map[string]any{"userId": state.userID, "created": state.createCreated},
			})
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/lifecycle"):
			state.mu.Lock()
			state.lifecycles++
			state.mu.Unlock()
			var input identityclient.LifecycleInput
			_ = json.NewDecoder(r.Body).Decode(&input)
			writeEnvelope(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"userId": state.userID, "transitionId": input.TransitionID,
					"sequence": input.Sequence, "operation": input.Operation,
					"alreadyApplied": false, "sessionVersion": 1, "isActive": true,
				},
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/contact"):
			state.mu.Lock()
			state.contacts++
			state.mu.Unlock()
			writeEnvelope(t, w, http.StatusOK, map[string]any{
				"data": map[string]any{
					"fullName": "Seed Merchant Owner",
					"email":    "seed.merchant@example.com",
					"phone":    "+94771234567",
				},
			})
		case r.Method == http.MethodDelete && strings.HasPrefix(r.URL.Path, "/internal/merchant-owners/"):
			w.WriteHeader(http.StatusNoContent)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)

	client, err := identityclient.New(server.URL, testInternalToken, time.Second)
	if err != nil {
		t.Fatalf("identityclient.New returned error: %v", err)
	}
	return client
}

func writeEnvelope(t *testing.T, w http.ResponseWriter, status int, payload any) {
	t.Helper()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	if err := json.NewEncoder(w).Encode(payload); err != nil {
		t.Fatalf("encode identity response: %v", err)
	}
}

type memRepo struct {
	attempt     repository.RegistrationAttempt
	hasAttempt  bool
	merchant    repository.Merchant
	hasMerchant bool
	transitions []repository.StatusTransition
}

func (m *memRepo) EnsureRegistrationAttempt(_ context.Context, id, key, email string) (repository.RegistrationAttempt, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if m.hasAttempt {
		if m.attempt.IdempotencyKey == key {
			return m.attempt, nil
		}
		if strings.EqualFold(m.attempt.Email, email) && (m.attempt.Status == "PENDING" || m.attempt.Status == "COMPLETED") {
			return repository.RegistrationAttempt{}, repository.ErrDuplicateApplication
		}
	}
	m.attempt = repository.RegistrationAttempt{ID: id, IdempotencyKey: key, Email: email, Status: "PENDING"}
	m.hasAttempt = true
	return m.attempt, nil
}

func (m *memRepo) SetRegistrationAttemptUser(_ context.Context, attemptID, userID string) error {
	m.attempt.UserID = &userID
	m.attempt.Status = "PENDING"
	return nil
}

func (m *memRepo) CreatePendingMerchant(_ context.Context, attemptID string, params repository.CreateMerchantParams) (repository.Merchant, error) {
	regNo := params.BusinessRegNo
	m.merchant = repository.Merchant{
		ID: params.ID, OwnerUserID: params.OwnerUserID, BusinessName: params.BusinessName,
		BusinessRegNo: &regNo, ContactEmail: params.ContactEmail, ContactPhone: params.ContactPhone,
		PickupAddress: params.PickupAddress, PickupZoneID: params.PickupZoneID,
		Status: repository.MerchantStatusPending, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(),
	}
	m.hasMerchant = true
	m.attempt.Status = "COMPLETED"
	m.attempt.MerchantID = &params.ID
	return m.merchant, nil
}

func (m *memRepo) CompleteRegistrationAttempt(_ context.Context, _ string, merchantID string) error {
	m.attempt.Status = "COMPLETED"
	m.attempt.MerchantID = &merchantID
	return nil
}

func (m *memRepo) FailRegistrationAttempt(_ context.Context, _ string, message string) error {
	m.attempt.Status = "FAILED"
	m.attempt.LastError = &message
	return nil
}

func (m *memRepo) MerchantByID(_ context.Context, merchantID string) (repository.Merchant, error) {
	if !m.hasMerchant || m.merchant.ID != merchantID {
		return repository.Merchant{}, repository.ErrNotFound
	}
	return m.merchant, nil
}

func (m *memRepo) BeginApprovalTransition(_ context.Context, merchantID, actorID, transitionID string) (repository.Merchant, repository.StatusTransition, error) {
	if !m.hasMerchant || m.merchant.ID != merchantID {
		return repository.Merchant{}, repository.StatusTransition{}, repository.ErrNotFound
	}
	m.merchant.LifecycleSequence++
	transition := repository.StatusTransition{
		ID: transitionID, MerchantID: merchantID, Sequence: m.merchant.LifecycleSequence,
		FromStatus: m.merchant.Status, ToStatus: repository.MerchantStatusApproved,
		ActorUserID: actorID, SyncStatus: "PENDING",
	}
	m.transitions = append(m.transitions, transition)
	return m.merchant, transition, nil
}

func (m *memRepo) ApplyApprovalTransition(_ context.Context, merchantID, actorID, transitionID string) (repository.Merchant, error) {
	if !m.hasMerchant || m.merchant.ID != merchantID {
		return repository.Merchant{}, repository.ErrNotFound
	}
	now := time.Now().UTC()
	m.merchant.Status = repository.MerchantStatusApproved
	m.merchant.ApprovedBy = &actorID
	m.merchant.ApprovedAt = &now
	m.merchant.UpdatedAt = now
	for index := range m.transitions {
		if m.transitions[index].ID == transitionID {
			m.transitions[index].SyncStatus = "APPLIED"
		}
	}
	return m.merchant, nil
}

func (m *memRepo) BeginRejectionTransition(context.Context, string, string, string, *string) (repository.Merchant, repository.StatusTransition, error) {
	return repository.Merchant{}, repository.StatusTransition{}, errors.New("not implemented")
}

func (m *memRepo) BeginSuspensionTransition(context.Context, string, string, string, *string) (repository.Merchant, repository.StatusTransition, error) {
	return repository.Merchant{}, repository.StatusTransition{}, errors.New("not implemented")
}

func (m *memRepo) MarkTransitionResult(_ context.Context, transitionID string, applied bool, message string) error {
	for index := range m.transitions {
		if m.transitions[index].ID != transitionID {
			continue
		}
		if applied {
			m.transitions[index].SyncStatus = "APPLIED"
		} else {
			m.transitions[index].SyncStatus = "FAILED"
			m.transitions[index].LastError = &message
		}
	}
	return nil
}

func (m *memRepo) LatestTransition(_ context.Context, merchantID string, target repository.MerchantStatus) (repository.StatusTransition, error) {
	for index := len(m.transitions) - 1; index >= 0; index-- {
		transition := m.transitions[index]
		if transition.MerchantID != merchantID {
			continue
		}
		if target != "" && transition.ToStatus != target {
			continue
		}
		return transition, nil
	}
	return repository.StatusTransition{}, repository.ErrNotFound
}

func (m *memRepo) MerchantByContactEmail(_ context.Context, contactEmail string) (repository.Merchant, error) {
	if !m.hasMerchant || !strings.EqualFold(m.merchant.ContactEmail, strings.TrimSpace(contactEmail)) {
		return repository.Merchant{}, repository.ErrNotFound
	}
	return m.merchant, nil
}

func TestRunCreatesAndApprovesMerchant(t *testing.T) {
	ctx := context.Background()
	config := testConfig(t)
	repo := &memRepo{}
	state := &identityState{createCreated: true}
	identity := newTestIdentity(t, state)

	result, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.AlreadySeeded {
		t.Fatal("first Run reported AlreadySeeded")
	}
	merchant := result.Merchant
	if merchant.Status != repository.MerchantStatusApproved {
		t.Fatalf("status = %s, want APPROVED", merchant.Status)
	}
	if merchant.BusinessName != config.BusinessName ||
		merchant.BusinessRegNo == nil || *merchant.BusinessRegNo != config.BusinessRegNo ||
		merchant.ContactEmail != config.ContactEmail ||
		merchant.ContactPhone != config.ContactPhone ||
		merchant.PickupAddress != config.PickupAddress ||
		merchant.PickupZoneID != testZoneID ||
		merchant.OwnerUserID != testOwnerUserID {
		t.Fatalf("merchant fields do not match the seed config: %+v", merchant)
	}
	if merchant.ApprovedBy == nil || *merchant.ApprovedBy != seedAdminActorID {
		t.Fatalf("ApprovedBy = %v, want %s", merchant.ApprovedBy, seedAdminActorID)
	}
	if merchant.ApprovedAt == nil || merchant.ApprovedAt.IsZero() {
		t.Fatal("ApprovedAt was not set")
	}
	if state.creates != 1 || state.lifecycles != 1 {
		t.Fatalf("identity calls = creates %d, lifecycles %d; want 1 each", state.creates, state.lifecycles)
	}

	before := merchant
	changed := config
	changed.MerchantPassword = "DifferentPass123"
	changed.MerchantFullName = "Different Name"
	changed.BusinessName = "Different Business"
	changed.ContactPhone = "+94779999999"
	changed.PickupAddress = "9 Different Street, Colombo"

	second, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, changed)
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if !second.AlreadySeeded {
		t.Fatal("second Run did not report AlreadySeeded")
	}
	if second.Merchant != before {
		t.Fatalf("rerun changed the approved merchant: before=%+v after=%+v", before, second.Merchant)
	}
	if state.creates != 1 || state.lifecycles != 1 {
		t.Fatalf("rerun called identity again: creates %d, lifecycles %d", state.creates, state.lifecycles)
	}
}

func TestRunExistingApprovedMerchantUntouched(t *testing.T) {
	ctx := context.Background()
	config := testConfig(t)
	approvedAt := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	existing := repository.Merchant{
		ID: "3f1c7a52-0000-4000-8000-0000000000c1", OwnerUserID: "3f1c7a52-0000-4000-8000-0000000000c2",
		BusinessName: "Existing Business", ContactEmail: config.ContactEmail, ContactPhone: "+94770000000",
		PickupAddress: "9 Existing Road, Colombo", PickupZoneID: testZoneID,
		Status: repository.MerchantStatusApproved, ApprovedBy: stringPtr("existing-admin"),
		ApprovedAt: &approvedAt, LifecycleSequence: 4,
	}
	repo := &memRepo{merchant: existing, hasMerchant: true}
	state := &identityState{createCreated: true}
	identity := newTestIdentity(t, state)

	result, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if !result.AlreadySeeded {
		t.Fatal("Run did not report AlreadySeeded for an approved merchant")
	}
	if result.Merchant != existing {
		t.Fatalf("approved merchant changed: before=%+v after=%+v", existing, result.Merchant)
	}
	if state.creates != 0 || state.lifecycles != 0 {
		t.Fatalf("Run touched identity: creates %d, lifecycles %d", state.creates, state.lifecycles)
	}
}

func TestRunApprovesPendingMerchantIdempotently(t *testing.T) {
	ctx := context.Background()
	config := testConfig(t)
	regNo := config.BusinessRegNo
	pending := repository.Merchant{
		ID: "3f1c7a52-0000-4000-8000-0000000000d1", OwnerUserID: testOwnerUserID,
		BusinessName: config.BusinessName, BusinessRegNo: &regNo, ContactEmail: config.ContactEmail,
		ContactPhone: config.ContactPhone, PickupAddress: config.PickupAddress, PickupZoneID: testZoneID,
		Status: repository.MerchantStatusPending,
	}
	repo := &memRepo{merchant: pending, hasMerchant: true}
	state := &identityState{createCreated: true}
	identity := newTestIdentity(t, state)

	result, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err != nil {
		t.Fatalf("Run returned error: %v", err)
	}
	if result.AlreadySeeded || result.Merchant.Status != repository.MerchantStatusApproved {
		t.Fatalf("result = %+v, want a newly approved merchant", result)
	}
	if state.creates != 0 || state.lifecycles != 1 {
		t.Fatalf("identity calls = creates %d, lifecycles %d; want 0 and 1", state.creates, state.lifecycles)
	}

	second, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err != nil {
		t.Fatalf("second Run returned error: %v", err)
	}
	if !second.AlreadySeeded {
		t.Fatal("second Run did not report AlreadySeeded")
	}
	if state.lifecycles != 1 {
		t.Fatalf("second Run reapplied the approval: lifecycles %d", state.lifecycles)
	}
}

func TestRunIdentityEmailConflictIsSafeFailure(t *testing.T) {
	ctx := context.Background()
	config := testConfig(t)
	repo := &memRepo{}
	state := &identityState{createStatus: http.StatusConflict}
	identity := newTestIdentity(t, state)

	_, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err == nil {
		t.Fatal("Run succeeded despite an identity email conflict")
	}
	message := err.Error()
	if !strings.Contains(message, "already registered") || !strings.Contains(message, "refusing") {
		t.Fatalf("error = %v, want an actionable identity conflict", err)
	}
	for _, secret := range []string{config.MerchantPassword, config.InternalToken, config.DatabaseURL} {
		if strings.Contains(message, secret) {
			t.Fatalf("error leaked a secret: %v", err)
		}
	}
	if repo.hasMerchant {
		t.Fatal("Run created a merchant despite the identity conflict")
	}
	if state.lifecycles != 0 {
		t.Fatalf("Run applied %d lifecycle transitions after a conflict", state.lifecycles)
	}
}

func TestRunResumedIdentityWithoutMerchantIsSafeFailure(t *testing.T) {
	ctx := context.Background()
	config := testConfig(t)
	repo := &memRepo{}
	state := &identityState{createCreated: false}
	identity := newTestIdentity(t, state)

	_, err := Run(ctx, repo, identity, fakeZones{zone: activeZone()}, config)
	if err == nil {
		t.Fatal("Run attached a merchant to an existing identity account")
	}
	if !strings.Contains(err.Error(), "already exists without a merchant") {
		t.Fatalf("error = %v, want an existing-account refusal", err)
	}
	if repo.hasMerchant {
		t.Fatal("Run created a merchant for an existing identity account")
	}
	if state.lifecycles != 0 {
		t.Fatalf("Run applied %d lifecycle transitions", state.lifecycles)
	}
}

func TestRunRejectsUnknownOrInactiveZone(t *testing.T) {
	ctx := context.Background()
	repo := &memRepo{}
	state := &identityState{createCreated: true}
	identity := newTestIdentity(t, state)

	if _, err := Run(ctx, repo, identity, fakeZones{zone: pricingclient.Zone{ID: testZoneID, Code: "KDY", IsActive: true}}, testConfig(t)); err == nil {
		t.Fatal("Run accepted an unknown pickup zone")
	}
	inactive := activeZone()
	inactive.IsActive = false
	if _, err := Run(ctx, repo, identity, fakeZones{zone: inactive}, testConfig(t)); err == nil {
		t.Fatal("Run accepted an inactive pickup zone")
	}
	if state.creates != 0 {
		t.Fatalf("Run called identity before zone validation: creates %d", state.creates)
	}
}

func stringPtr(value string) *string { return &value }
