package onboarding

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
)

type fakeStore struct {
	attempt       repository.RegistrationAttempt
	merchant      repository.Merchant
	transition    repository.StatusTransition
	createErr     error
	applyErr      error
	compensated   bool
	markedApplied bool
	lastReason    *string
	lastOperation string
	// latest is what LatestTransition returns, to stand for a decision made earlier.
	latest *repository.StatusTransition
}

func (f *fakeStore) EnsureRegistrationAttempt(_ context.Context, id, key, email string) (repository.RegistrationAttempt, error) {
	f.attempt = repository.RegistrationAttempt{ID: id, IdempotencyKey: key, Email: email, Status: "PENDING"}
	return f.attempt, nil
}
func (f *fakeStore) SetRegistrationAttemptUser(_ context.Context, _, userID string) error {
	f.attempt.UserID = &userID
	return nil
}
func (f *fakeStore) CreatePendingMerchant(_ context.Context, _ string, params repository.CreateMerchantParams) (repository.Merchant, error) {
	if f.createErr != nil {
		return repository.Merchant{}, f.createErr
	}
	f.merchant = repository.Merchant{ID: params.ID, OwnerUserID: params.OwnerUserID, BusinessName: params.BusinessName, BusinessRegNo: &params.BusinessRegNo, ContactEmail: params.ContactEmail, Status: repository.MerchantStatusPending, CreatedAt: time.Now()}
	merchantID := f.merchant.ID
	f.attempt.MerchantID = &merchantID
	f.attempt.Status = "COMPLETED"
	return f.merchant, nil
}
func (f *fakeStore) CompleteRegistrationAttempt(_ context.Context, _, merchantID string) error {
	f.attempt.MerchantID = &merchantID
	return nil
}
func (f *fakeStore) FailRegistrationAttempt(_ context.Context, _, _ string) error { return nil }
func (f *fakeStore) MerchantByID(context.Context, string) (repository.Merchant, error) {
	return f.merchant, nil
}
func (f *fakeStore) BeginApprovalTransition(_ context.Context, merchantID, actorID, transitionID string) (repository.Merchant, repository.StatusTransition, error) {
	f.transition = repository.StatusTransition{ID: transitionID, MerchantID: merchantID, Sequence: 1, FromStatus: f.merchant.Status, ToStatus: repository.MerchantStatusApproved, ActorUserID: actorID, SyncStatus: "PENDING"}
	return f.merchant, f.transition, nil
}
func (f *fakeStore) ApplyApprovalTransition(_ context.Context, _, _, _ string) (repository.Merchant, error) {
	if f.applyErr != nil {
		return repository.Merchant{}, f.applyErr
	}
	f.merchant.Status = repository.MerchantStatusApproved
	return f.merchant, nil
}
func (f *fakeStore) BeginRejectionTransition(_ context.Context, merchantID, actorID, transitionID string, reason *string) (repository.Merchant, repository.StatusTransition, error) {
	f.merchant.Status = repository.MerchantStatusRejected
	f.lastReason = reason
	f.transition = repository.StatusTransition{ID: transitionID, MerchantID: merchantID, Sequence: 1, ToStatus: repository.MerchantStatusRejected, ActorUserID: actorID, Reason: reason, SyncStatus: "PENDING"}
	return f.merchant, f.transition, nil
}
func (f *fakeStore) BeginSuspensionTransition(_ context.Context, merchantID, actorID, transitionID string, reason *string) (repository.Merchant, repository.StatusTransition, error) {
	f.merchant.Status = repository.MerchantStatusSuspended
	f.lastReason = reason
	f.transition = repository.StatusTransition{ID: transitionID, MerchantID: merchantID, Sequence: 2, ToStatus: repository.MerchantStatusSuspended, ActorUserID: actorID, Reason: reason, SyncStatus: "PENDING"}
	return f.merchant, f.transition, nil
}
func (f *fakeStore) MarkTransitionResult(_ context.Context, _ string, applied bool, _ string) error {
	f.markedApplied = applied
	return nil
}
func (f *fakeStore) LatestTransition(context.Context, string, repository.MerchantStatus) (repository.StatusTransition, error) {
	if f.latest != nil {
		return *f.latest, nil
	}
	return repository.StatusTransition{}, repository.ErrNotFound
}

type fakeIdentity struct {
	createErr      error
	lifecycleErr   error
	created        bool
	deleted        bool
	lifecycleCalls []identityclient.LifecycleInput
}

func (f *fakeIdentity) CreateMerchantOwner(context.Context, identityclient.OwnerInput) (identityclient.OwnerResult, error) {
	if f.createErr != nil {
		return identityclient.OwnerResult{}, f.createErr
	}
	return identityclient.OwnerResult{UserID: "user-1", Created: true}, nil
}
func (f *fakeIdentity) ApplyLifecycle(_ context.Context, _ string, input identityclient.LifecycleInput) (identityclient.LifecycleResult, error) {
	f.lifecycleCalls = append(f.lifecycleCalls, input)
	if f.lifecycleErr != nil {
		return identityclient.LifecycleResult{}, f.lifecycleErr
	}
	return identityclient.LifecycleResult{AlreadyApplied: false}, nil
}
func (f *fakeIdentity) DeleteMerchantOwner(context.Context, string) error {
	f.deleted = true
	return nil
}
func (f *fakeIdentity) Contact(context.Context, string) (identityclient.Contact, error) {
	return identityclient.Contact{FullName: "Owner", Email: "owner@example.com"}, nil
}

type fakePricing struct{ active bool }

func (f fakePricing) Zone(context.Context, string) (pricingclient.Zone, error) {
	return pricingclient.Zone{ID: "zone-1", IsActive: f.active}, nil
}

type fakeNotifier struct {
	received  int
	approved  int
	rejected  *string
	suspended *string

	rejectedCalls  int
	suspendedCalls int
}

func (f *fakeNotifier) Received(context.Context, string, string, string, string, time.Time) {
	f.received++
}
func (f *fakeNotifier) Approved(context.Context, string, string, string, time.Time) { f.approved++ }
func (f *fakeNotifier) Rejected(_ context.Context, _, _, _ string, reason *string) {
	f.rejected = reason
	f.rejectedCalls++
}
func (f *fakeNotifier) Suspended(_ context.Context, _, _, _ string, reason *string) {
	f.suspended = reason
	f.suspendedCalls++
}

func validInput() RegisterInput {
	return RegisterInput{
		FullName: "Ruwan Jayasuriya", Email: "owner@shop.lk", Phone: "+94773334455",
		Password: "Str0ngPass!", Business: BusinessInput{
			Name: "ShopZone", RegistrationNumber: "pv 123456", ContactEmail: "support@shop.lk",
			ContactPhone: "+94112223344", BusinessAddress: "88 Galle Road", PickupAddress: "88 Galle Road",
			PickupZoneID: "7a1e0000-0000-4000-8000-000000000001",
		},
	}
}

func fixture() (*Service, *fakeStore, *fakeIdentity, *fakeNotifier) {
	store := &fakeStore{}
	identity := &fakeIdentity{}
	notifier := &fakeNotifier{}
	service, _ := New(store, identity, fakePricing{active: true}, notifier, nil)
	return service, store, identity, notifier
}

func TestRegisterCreatesPendingApplicationAndSendsEmail(t *testing.T) {
	service, store, identity, notifier := fixture()
	result, err := service.Register(context.Background(), validInput())
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != repository.MerchantStatusPending || result.UserID != "user-1" {
		t.Fatalf("result = %+v", result)
	}
	if store.attempt.Status != "COMPLETED" || store.attempt.MerchantID == nil {
		t.Fatalf("attempt = %+v", store.attempt)
	}
	if identity.lifecycleCalls != nil || notifier.received != 1 {
		t.Fatalf("unexpected calls: lifecycle=%d received=%d", len(identity.lifecycleCalls), notifier.received)
	}
	if store.merchant.BusinessRegNo == nil || *store.merchant.BusinessRegNo != "PV 123456" {
		t.Fatalf("registration number = %v, want normalized", store.merchant.BusinessRegNo)
	}
}

func TestRegisterRejectsDuplicateEmailAndCompensates(t *testing.T) {
	service, store, identity, _ := fixture()
	identity.createErr = identityclient.ErrEmailTaken
	if _, err := service.Register(context.Background(), validInput()); !errors.Is(err, ErrEmailInUse) {
		t.Fatalf("error = %v, want ErrEmailInUse", err)
	}
	service, store, identity, _ = fixture()
	store.createErr = repository.ErrDuplicateApplication
	if _, err := service.Register(context.Background(), validInput()); !errors.Is(err, ErrDuplicateApplication) {
		t.Fatalf("error = %v, want ErrDuplicateApplication", err)
	}
	if !identity.deleted {
		t.Fatal("a created owner was not compensated")
	}
}

func TestRegisterValidatesRequiredBusinessFields(t *testing.T) {
	service, _, _, _ := fixture()
	input := validInput()
	input.Business.RegistrationNumber = ""
	if _, err := service.Register(context.Background(), input); err == nil {
		t.Fatal("blank registration number was accepted")
	}
	input = validInput()
	input.Business.BusinessAddress = ""
	if _, err := service.Register(context.Background(), input); err == nil {
		t.Fatal("blank business address was accepted")
	}
}

func TestApproveActivatesThenCommits(t *testing.T) {
	service, store, identity, notifier := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: repository.MerchantStatusPending}
	result, err := service.Approve(context.Background(), "m-1", "admin-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Merchant.Status != repository.MerchantStatusApproved || result.IdentitySync != "APPLIED" {
		t.Fatalf("result = %+v", result)
	}
	if len(identity.lifecycleCalls) != 1 || identity.lifecycleCalls[0].Operation != "APPROVE" {
		t.Fatalf("lifecycle calls = %+v", identity.lifecycleCalls)
	}
	if notifier.approved != 1 {
		t.Fatalf("approval emails = %d, want 1", notifier.approved)
	}
}

func TestApproveActivationFailureLeavesMerchantPending(t *testing.T) {
	service, store, identity, _ := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", Status: repository.MerchantStatusPending}
	identity.lifecycleErr = errors.New("identity down")
	if _, err := service.Approve(context.Background(), "m-1", "admin-1"); !errors.Is(err, ErrActivationFailed) {
		t.Fatalf("error = %v, want ErrActivationFailed", err)
	}
	if store.merchant.Status != repository.MerchantStatusPending {
		t.Fatalf("status = %s, want PENDING", store.merchant.Status)
	}
}

func TestRejectReasonIsOptionalAndNormalized(t *testing.T) {
	for _, test := range []struct {
		name   string
		reason *string
		want   *string
	}{
		{"missing", nil, nil},
		{"empty", ptr("   "), nil},
		{"reason", ptr(" Documents could not be verified "), ptr("Documents could not be verified")},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, store, _, notifier := fixture()
			store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: repository.MerchantStatusPending}
			if _, err := service.Reject(context.Background(), "m-1", "admin-1", test.reason); err != nil {
				t.Fatal(err)
			}
			if !sameReason(store.lastReason, test.want) {
				t.Fatalf("stored reason = %v, want %v", store.lastReason, test.want)
			}
			if !sameReason(notifier.rejected, test.want) {
				t.Fatalf("email reason = %v, want %v", notifier.rejected, test.want)
			}
		})
	}
}

func TestRejectReasonOverLimitIsRejected(t *testing.T) {
	service, store, _, _ := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", Status: repository.MerchantStatusPending}
	long := strings.Repeat("a", 1001)
	if _, err := service.Reject(context.Background(), "m-1", "admin-1", &long); err == nil {
		t.Fatal("overlong reason was accepted")
	}
}

func TestSuspendReturnsPendingSyncWhenIdentityFails(t *testing.T) {
	service, store, identity, notifier := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: repository.MerchantStatusApproved}
	identity.lifecycleErr = errors.New("identity down")
	result, err := service.Suspend(context.Background(), "m-1", "admin-1", ptr("Policy breach"))
	if err != nil {
		t.Fatal(err)
	}
	if result.Merchant.Status != repository.MerchantStatusSuspended || result.IdentitySync != "PENDING" {
		t.Fatalf("result = %+v", result)
	}
	if notifier.suspended == nil && result.IdentitySync != "PENDING" {
		t.Fatal("suspension email was not requested")
	}
}

// An administrator can retry a sync that failed. It reaches Identity again with
// the same transition, and the merchant is not emailed a second time.
func TestRetryingAPendingSyncDoesNotEmailTheMerchantAgain(t *testing.T) {
	for _, test := range []struct {
		name   string
		status repository.MerchantStatus
		retry  func(*Service) (DecisionResult, error)
		calls  func(*fakeNotifier) int
		op     string
	}{
		{"suspension", repository.MerchantStatusSuspended,
			func(s *Service) (DecisionResult, error) {
				return s.Suspend(context.Background(), "m-1", "admin-1", ptr("Policy breach"))
			},
			func(n *fakeNotifier) int { return n.suspendedCalls }, "SUSPEND"},
		{"rejection", repository.MerchantStatusRejected,
			func(s *Service) (DecisionResult, error) {
				return s.Reject(context.Background(), "m-1", "admin-1", nil)
			},
			func(n *fakeNotifier) int { return n.rejectedCalls }, "REJECT"},
	} {
		t.Run(test.name, func(t *testing.T) {
			service, store, identity, notifier := fixture()
			store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: test.status}
			store.latest = &repository.StatusTransition{ID: "t-1", MerchantID: "m-1", Sequence: 3, ToStatus: test.status, SyncStatus: "FAILED"}

			result, err := test.retry(service)

			if err != nil {
				t.Fatal(err)
			}
			if result.IdentitySync != "APPLIED" || result.AlreadyDone {
				t.Fatalf("result = %+v, want a fresh APPLIED sync", result)
			}
			if len(identity.lifecycleCalls) != 1 || identity.lifecycleCalls[0].TransitionID != "t-1" ||
				identity.lifecycleCalls[0].Sequence != 3 || identity.lifecycleCalls[0].Operation != test.op {
				t.Fatalf("identity calls = %+v, want the stored transition replayed once", identity.lifecycleCalls)
			}
			if got := test.calls(notifier); got != 0 {
				t.Fatalf("the merchant was emailed %d times on a retry, want 0", got)
			}
			if !store.markedApplied {
				t.Fatal("the transition was not marked applied")
			}
		})
	}
}

func TestRetryStaysPendingWhileIdentityIsDown(t *testing.T) {
	service, store, identity, notifier := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: repository.MerchantStatusSuspended}
	store.latest = &repository.StatusTransition{ID: "t-1", MerchantID: "m-1", Sequence: 2, ToStatus: repository.MerchantStatusSuspended, SyncStatus: "FAILED"}
	identity.lifecycleErr = errors.New("identity down")

	result, err := service.Suspend(context.Background(), "m-1", "admin-1", ptr("Policy breach"))

	if err != nil || result.IdentitySync != "PENDING" {
		t.Fatalf("result = %+v, err = %v, want PENDING", result, err)
	}
	if notifier.suspendedCalls != 0 {
		t.Fatal("a failed retry emailed the merchant")
	}
}

func TestADecisionIsEmailedOnceEvenWhenIdentityFailsFirst(t *testing.T) {
	service, store, identity, notifier := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", BusinessName: "ShopZone", Status: repository.MerchantStatusApproved}
	identity.lifecycleErr = errors.New("identity down")

	if _, err := service.Suspend(context.Background(), "m-1", "admin-1", ptr("Policy breach")); err != nil {
		t.Fatal(err)
	}

	if notifier.suspendedCalls != 1 {
		t.Fatalf("suspension emails = %d, want 1 with the decision itself", notifier.suspendedCalls)
	}
}

func TestRetryOfASyncThatAlreadyReachedIdentityDoesNothing(t *testing.T) {
	service, store, identity, notifier := fixture()
	store.merchant = repository.Merchant{ID: "m-1", OwnerUserID: "user-1", Status: repository.MerchantStatusSuspended}
	store.latest = &repository.StatusTransition{ID: "t-1", MerchantID: "m-1", Sequence: 2, ToStatus: repository.MerchantStatusSuspended, SyncStatus: "APPLIED"}

	result, err := service.Suspend(context.Background(), "m-1", "admin-1", ptr("Policy breach"))

	if err != nil || !result.AlreadyDone || result.IdentitySync != "APPLIED" {
		t.Fatalf("result = %+v, err = %v, want AlreadyDone", result, err)
	}
	if len(identity.lifecycleCalls) != 0 || notifier.suspendedCalls != 0 {
		t.Fatal("a finished sync called Identity or sent an email again")
	}
}

func ptr(value string) *string { return &value }

func sameReason(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
