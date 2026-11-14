// Package onboarding implements merchant registration and the administrator
// approval, rejection and suspension lifecycle.
package onboarding

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/businessreg"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/notify"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/pricingclient"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

var (
	ErrValidation           = errors.New("validation failed")
	ErrEmailInUse           = errors.New("email is already registered")
	ErrDuplicateApplication = errors.New("a merchant application already exists")
	ErrDuplicateBusiness    = errors.New("business registration number already registered")
	ErrUnavailable          = errors.New("required service is unavailable")
	ErrActivationFailed     = errors.New("merchant activation failed")
)

type ValidationIssue = validation.Issue

type ValidationError struct{ Issues []ValidationIssue }

func (e *ValidationError) Error() string { return ErrValidation.Error() }

type Store interface {
	EnsureRegistrationAttempt(ctx context.Context, id, idempotencyKey, email string) (repository.RegistrationAttempt, error)
	SetRegistrationAttemptUser(ctx context.Context, attemptID, userID string) error
	CreatePendingMerchant(ctx context.Context, attemptID string, params repository.CreateMerchantParams) (repository.Merchant, error)
	CompleteRegistrationAttempt(ctx context.Context, attemptID, merchantID string) error
	FailRegistrationAttempt(ctx context.Context, attemptID, message string) error
	MerchantByID(ctx context.Context, merchantID string) (repository.Merchant, error)
	BeginApprovalTransition(ctx context.Context, merchantID, actorID, transitionID string) (repository.Merchant, repository.StatusTransition, error)
	ApplyApprovalTransition(ctx context.Context, merchantID, actorID, transitionID string) (repository.Merchant, error)
	BeginRejectionTransition(ctx context.Context, merchantID, actorID, transitionID string, reason *string) (repository.Merchant, repository.StatusTransition, error)
	BeginSuspensionTransition(ctx context.Context, merchantID, actorID, transitionID string, reason *string) (repository.Merchant, repository.StatusTransition, error)
	MarkTransitionResult(ctx context.Context, transitionID string, applied bool, message string) error
	LatestTransition(ctx context.Context, merchantID string, target repository.MerchantStatus) (repository.StatusTransition, error)
}

type Identity interface {
	CreateMerchantOwner(ctx context.Context, input identityclient.OwnerInput) (identityclient.OwnerResult, error)
	ApplyLifecycle(ctx context.Context, userID string, input identityclient.LifecycleInput) (identityclient.LifecycleResult, error)
	DeleteMerchantOwner(ctx context.Context, userID string) error
	Contact(ctx context.Context, userID string) (identityclient.Contact, error)
}

type Zoner interface {
	Zone(ctx context.Context, zoneID string) (pricingclient.Zone, error)
}

type Notifier interface {
	Received(ctx context.Context, to, ownerName, businessName, contactEmail string, submittedAt time.Time)
	Approved(ctx context.Context, to, ownerName, businessName string, approvedAt time.Time)
	Rejected(ctx context.Context, to, ownerName, businessName string, reason *string)
	Suspended(ctx context.Context, to, ownerName, businessName string, reason *string)
}

type Logger interface {
	Error(string, ...any)
	Info(string, ...any)
}

type Service struct {
	store    Store
	identity Identity
	pricing  Zoner
	notifier Notifier
	logger   Logger
	now      func() time.Time
}

func New(store Store, identity Identity, pricing Zoner, notifier Notifier, logger Logger) (*Service, error) {
	if store == nil || identity == nil || pricing == nil {
		return nil, errors.New("onboarding requires a store, identity and pricing client")
	}
	if notifier == nil {
		notifier = noNotifier{}
	}
	return &Service{store: store, identity: identity, pricing: pricing, notifier: notifier, logger: logger, now: time.Now}, nil
}

type BusinessInput struct {
	Name               string `json:"name"`
	RegistrationNumber string `json:"registrationNumber"`
	ContactEmail       string `json:"contactEmail"`
	ContactPhone       string `json:"contactPhone"`
	BusinessAddress    string `json:"businessAddress"`
	PickupAddress      string `json:"pickupAddress"`
	PickupZoneID       string `json:"pickupZoneId"`
}

type RegisterInput struct {
	FullName       string        `json:"fullName"`
	Email          string        `json:"email"`
	Phone          string        `json:"phone"`
	Password       string        `json:"password"`
	Business       BusinessInput `json:"business"`
	IdempotencyKey string
}

type RegisterResult struct {
	MerchantID string
	UserID     string
	Status     repository.MerchantStatus
}

func (s *Service) Register(ctx context.Context, input RegisterInput) (RegisterResult, error) {
	email := strings.ToLower(strings.TrimSpace(input.Email))
	fullName := strings.TrimSpace(input.FullName)
	phone := strings.TrimSpace(input.Phone)
	businessName := strings.TrimSpace(input.Business.Name)
	regNo := businessreg.Normalize(input.Business.RegistrationNumber)
	contactEmail := strings.ToLower(strings.TrimSpace(input.Business.ContactEmail))
	if contactEmail == "" {
		contactEmail = email
	}
	contactPhone := strings.TrimSpace(input.Business.ContactPhone)
	if contactPhone == "" {
		contactPhone = phone
	}
	businessAddress := strings.TrimSpace(input.Business.BusinessAddress)
	pickupAddress := strings.TrimSpace(input.Business.PickupAddress)
	zoneID := strings.TrimSpace(input.Business.PickupZoneID)

	issues := validation.New()
	issues.Email("email", email)
	issues.Length("fullName", fullName, 2, 150, "must be 2 to 150 characters")
	issues.Custom("phone", e164(phone), "must be in E.164 format, for example +94771234567")
	issues.Custom("password", validPassword(input.Password), "must be at least 8 characters with upper case, lower case and a digit")
	issues.Length("business.name", businessName, 2, 150, "must be 2 to 150 characters")
	issues.Custom("business.registrationNumber", businessreg.Valid(regNo), "must be 3 to 50 characters with at least one digit")
	issues.Email("business.contactEmail", contactEmail)
	issues.Custom("business.contactPhone", e164(contactPhone), "must be in E.164 format, for example +94771234567")
	issues.Length("business.businessAddress", businessAddress, 5, 255, "must be 5 to 255 characters")
	issues.Length("business.pickupAddress", pickupAddress, 5, 255, "must be 5 to 255 characters")
	issues.Custom("business.pickupZoneId", validation.UUID(zoneID), "must be a UUID")
	if !issues.Empty() {
		return RegisterResult{}, &ValidationError{Issues: issues.Issues()}
	}

	zone, err := s.pricing.Zone(ctx, zoneID)
	if err != nil {
		return RegisterResult{}, ErrUnavailable
	}
	if !zone.IsActive {
		return RegisterResult{}, &ValidationError{Issues: []ValidationIssue{{Field: "business.pickupZoneId", Issue: "must be an active zone"}}}
	}

	key := strings.TrimSpace(input.IdempotencyKey)
	if key == "" {
		sum := sha256.Sum256([]byte(email + "\x00" + regNo))
		key = hex.EncodeToString(sum[:])
	}
	if len(key) > 128 {
		return RegisterResult{}, &ValidationError{Issues: []ValidationIssue{{Field: "Idempotency-Key", Issue: "must be at most 128 characters"}}}
	}
	attemptID, err := newUUID()
	if err != nil {
		return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
	}
	attempt, err := s.store.EnsureRegistrationAttempt(ctx, attemptID, key, email)
	switch {
	case errors.Is(err, repository.ErrDuplicateApplication), errors.Is(err, repository.ErrRegistrationConflict):
		return RegisterResult{}, ErrDuplicateApplication
	case err != nil:
		return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
	}
	if attempt.Status == "COMPLETED" && attempt.MerchantID != nil {
		merchant, err := s.store.MerchantByID(ctx, *attempt.MerchantID)
		if err != nil {
			return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
		}
		return RegisterResult{MerchantID: merchant.ID, UserID: merchant.OwnerUserID, Status: merchant.Status}, nil
	}

	owner, err := s.identity.CreateMerchantOwner(ctx, identityclient.OwnerInput{
		Email: email, Password: input.Password, FullName: fullName, Phone: phone,
	})
	if errors.Is(err, identityclient.ErrEmailTaken) {
		_ = s.store.FailRegistrationAttempt(ctx, attempt.ID, "email already registered")
		return RegisterResult{}, ErrEmailInUse
	}
	if err != nil {
		_ = s.store.FailRegistrationAttempt(ctx, attempt.ID, "identity unavailable")
		return RegisterResult{}, ErrUnavailable
	}
	if err := s.store.SetRegistrationAttemptUser(ctx, attempt.ID, owner.UserID); err != nil {
		s.compensate(ctx, owner.UserID, owner.Created)
		return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
	}
	merchantID, err := newUUID()
	if err != nil {
		s.compensate(ctx, owner.UserID, owner.Created)
		return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
	}
	merchant, err := s.store.CreatePendingMerchant(ctx, attempt.ID, repository.CreateMerchantParams{
		ID: merchantID, OwnerUserID: owner.UserID, BusinessName: businessName,
		BusinessRegNo: regNo, BusinessAddress: businessAddress, ContactEmail: contactEmail,
		ContactPhone: contactPhone, PickupAddress: pickupAddress, PickupZoneID: zoneID,
	})
	switch {
	case errors.Is(err, repository.ErrDuplicateApplication), errors.Is(err, repository.ErrRegistrationConflict):
		_ = s.store.FailRegistrationAttempt(ctx, attempt.ID, "duplicate registration")
		s.compensate(ctx, owner.UserID, owner.Created)
		return RegisterResult{}, ErrDuplicateApplication
	case errors.Is(err, repository.ErrDuplicateBusiness):
		_ = s.store.FailRegistrationAttempt(ctx, attempt.ID, "duplicate business registration")
		s.compensate(ctx, owner.UserID, owner.Created)
		return RegisterResult{}, ErrDuplicateBusiness
	case err != nil:
		_ = s.store.FailRegistrationAttempt(ctx, attempt.ID, "merchant creation failed")
		s.compensate(ctx, owner.UserID, owner.Created)
		return RegisterResult{}, fmt.Errorf("register merchant: %w", err)
	}
	_ = s.store.CompleteRegistrationAttempt(ctx, attempt.ID, merchant.ID)
	s.notifier.Received(ctx, email, fullName, businessName, contactEmail, merchant.CreatedAt)
	if s.logger != nil {
		s.logger.Info("merchant application created", "merchant_id", merchant.ID, "user_id", merchant.OwnerUserID)
	}
	return RegisterResult{MerchantID: merchant.ID, UserID: merchant.OwnerUserID, Status: merchant.Status}, nil
}

func (s *Service) compensate(ctx context.Context, userID string, created bool) {
	if !created {
		return
	}
	if err := s.identity.DeleteMerchantOwner(ctx, userID); err != nil && s.logger != nil {
		s.logger.Error("merchant owner compensation failed", "user_id", userID, "error", err, "error_type", fmt.Sprintf("%T", err))
	}
}

type DecisionResult struct {
	Merchant     repository.Merchant
	IdentitySync string
	AlreadyDone  bool
}

func (s *Service) Approve(ctx context.Context, merchantID, actorID string) (DecisionResult, error) {
	merchant, err := s.store.MerchantByID(ctx, merchantID)
	if err != nil {
		return DecisionResult{}, err
	}
	transition, terr := s.store.LatestTransition(ctx, merchantID, repository.MerchantStatusApproved)
	if terr == nil && transition.SyncStatus == "APPLIED" && merchant.Status == repository.MerchantStatusApproved {
		return DecisionResult{Merchant: merchant, IdentitySync: "APPLIED", AlreadyDone: true}, nil
	}
	if terr == nil && transition.SyncStatus != "APPLIED" {
		return s.applyApproval(ctx, merchant, transition, actorID)
	}
	transitionID, err := newUUID()
	if err != nil {
		return DecisionResult{}, fmt.Errorf("approve merchant: %w", err)
	}
	merchant, transition, err = s.store.BeginApprovalTransition(ctx, merchantID, actorID, transitionID)
	if err != nil {
		return DecisionResult{}, err
	}
	return s.applyApproval(ctx, merchant, transition, actorID)
}

func (s *Service) applyApproval(ctx context.Context, merchant repository.Merchant, transition repository.StatusTransition, actorID string) (DecisionResult, error) {
	applied, err := s.identity.ApplyLifecycle(ctx, merchant.OwnerUserID, identityclient.LifecycleInput{
		TransitionID: transition.ID, Sequence: transition.Sequence, Operation: "APPROVE",
	})
	if err != nil {
		_ = s.store.MarkTransitionResult(ctx, transition.ID, false, "identity lifecycle failed")
		return DecisionResult{}, ErrActivationFailed
	}
	updated, err := s.store.ApplyApprovalTransition(ctx, merchant.ID, actorID, transition.ID)
	if err != nil {
		return DecisionResult{}, fmt.Errorf("approve merchant: %w", err)
	}
	_ = s.store.MarkTransitionResult(ctx, transition.ID, true, "")
	wasApproved := merchant.Status == repository.MerchantStatusApproved
	if !wasApproved {
		contact := s.contact(ctx, merchant)
		s.notifier.Approved(ctx, contact.Email, contact.FullName, merchant.BusinessName, s.now().UTC())
	}
	return DecisionResult{Merchant: updated, IdentitySync: "APPLIED", AlreadyDone: applied.AlreadyApplied}, nil
}

func (s *Service) Reject(ctx context.Context, merchantID, actorID string, reason *string) (DecisionResult, error) {
	reason = normalizeReason(reason)
	if issue := validateReason(reason); issue != "" {
		return DecisionResult{}, &ValidationError{Issues: []ValidationIssue{{Field: "reason", Issue: issue}}}
	}
	merchant, err := s.store.MerchantByID(ctx, merchantID)
	if err != nil {
		return DecisionResult{}, err
	}
	transition, terr := s.store.LatestTransition(ctx, merchantID, repository.MerchantStatusRejected)
	if merchant.Status == repository.MerchantStatusRejected && terr == nil {
		if transition.SyncStatus == "APPLIED" {
			return DecisionResult{Merchant: merchant, IdentitySync: "APPLIED", AlreadyDone: true}, nil
		}
		return s.syncRejection(ctx, merchant, transition, false)
	}
	transitionID, err := newUUID()
	if err != nil {
		return DecisionResult{}, fmt.Errorf("reject merchant: %w", err)
	}
	merchant, transition, err = s.store.BeginRejectionTransition(ctx, merchantID, actorID, transitionID, reason)
	if err != nil {
		return DecisionResult{}, err
	}
	return s.syncRejection(ctx, merchant, transition, true)
}

// syncRejection applies the rejection to Identity. The decision email goes out
// only with the decision itself (notify), never again when an administrator
// retries a sync that failed, so the merchant is not told twice.
func (s *Service) syncRejection(ctx context.Context, merchant repository.Merchant, transition repository.StatusTransition, notify bool) (DecisionResult, error) {
	_, err := s.identity.ApplyLifecycle(ctx, merchant.OwnerUserID, identityclient.LifecycleInput{
		TransitionID: transition.ID, Sequence: transition.Sequence, Operation: "REJECT",
	})
	sync := "APPLIED"
	if err != nil {
		sync = "PENDING"
		_ = s.store.MarkTransitionResult(ctx, transition.ID, false, "identity lifecycle failed")
	} else {
		_ = s.store.MarkTransitionResult(ctx, transition.ID, true, "")
	}
	if notify {
		contact := s.contact(ctx, merchant)
		s.notifier.Rejected(ctx, contact.Email, contact.FullName, merchant.BusinessName, transition.Reason)
	}
	return DecisionResult{Merchant: merchant, IdentitySync: sync}, nil
}

func (s *Service) Suspend(ctx context.Context, merchantID, actorID string, reason *string) (DecisionResult, error) {
	reason = normalizeReason(reason)
	if reason == nil {
		return DecisionResult{}, &ValidationError{Issues: []ValidationIssue{{Field: "reason", Issue: "is required for a suspension"}}}
	}
	if issue := validateReason(reason); issue != "" {
		return DecisionResult{}, &ValidationError{Issues: []ValidationIssue{{Field: "reason", Issue: issue}}}
	}
	merchant, err := s.store.MerchantByID(ctx, merchantID)
	if err != nil {
		return DecisionResult{}, err
	}
	transition, terr := s.store.LatestTransition(ctx, merchantID, repository.MerchantStatusSuspended)
	if merchant.Status == repository.MerchantStatusSuspended && terr == nil {
		if transition.SyncStatus == "APPLIED" {
			return DecisionResult{Merchant: merchant, IdentitySync: "APPLIED", AlreadyDone: true}, nil
		}
		return s.syncSuspension(ctx, merchant, transition, false)
	}
	transitionID, err := newUUID()
	if err != nil {
		return DecisionResult{}, fmt.Errorf("suspend merchant: %w", err)
	}
	merchant, transition, err = s.store.BeginSuspensionTransition(ctx, merchantID, actorID, transitionID, reason)
	if err != nil {
		return DecisionResult{}, err
	}
	return s.syncSuspension(ctx, merchant, transition, true)
}

// syncSuspension applies the suspension to Identity. Like syncRejection, it
// sends the decision email only with the decision, not on a retry.
func (s *Service) syncSuspension(ctx context.Context, merchant repository.Merchant, transition repository.StatusTransition, notify bool) (DecisionResult, error) {
	_, err := s.identity.ApplyLifecycle(ctx, merchant.OwnerUserID, identityclient.LifecycleInput{
		TransitionID: transition.ID, Sequence: transition.Sequence, Operation: "SUSPEND",
	})
	sync := "APPLIED"
	if err != nil {
		sync = "PENDING"
		_ = s.store.MarkTransitionResult(ctx, transition.ID, false, "identity lifecycle failed")
	} else {
		_ = s.store.MarkTransitionResult(ctx, transition.ID, true, "")
	}
	if notify {
		contact := s.contact(ctx, merchant)
		s.notifier.Suspended(ctx, contact.Email, contact.FullName, merchant.BusinessName, transition.Reason)
	}
	return DecisionResult{Merchant: merchant, IdentitySync: sync}, nil
}

type contact struct{ FullName, Email string }

func (s *Service) contact(ctx context.Context, merchant repository.Merchant) contact {
	if found, err := s.identity.Contact(ctx, merchant.OwnerUserID); err == nil && found.Email != "" {
		return contact{FullName: found.FullName, Email: found.Email}
	}
	return contact{FullName: merchant.BusinessName, Email: merchant.ContactEmail}
}

func normalizeReason(reason *string) *string {
	if reason == nil {
		return nil
	}
	trimmed := strings.TrimSpace(*reason)
	if trimmed == "" {
		return nil
	}
	return &trimmed
}

func validateReason(reason *string) string {
	if reason == nil {
		return ""
	}
	if utf8.RuneCountInString(*reason) > 1000 {
		return "must be at most 1000 characters"
	}
	return ""
}

func validPassword(value string) bool {
	if utf8.RuneCountInString(value) < 8 || len(value) > 72 {
		return false
	}
	var upper, lower, digit bool
	for _, char := range value {
		switch {
		case unicode.IsUpper(char):
			upper = true
		case unicode.IsLower(char):
			lower = true
		case unicode.IsDigit(char):
			digit = true
		}
	}
	return upper && lower && digit
}

func e164(value string) bool {
	if len(value) < 3 || len(value) > 16 || value[0] != '+' || value[1] == '0' {
		return false
	}
	for _, char := range value[1:] {
		if !unicode.IsDigit(char) {
			return false
		}
	}
	return true
}

func newUUID() (string, error) {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	value[6] = (value[6] & 0x0f) | 0x40
	value[8] = (value[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", value[0:4], value[4:6], value[6:8], value[8:10], value[10:16]), nil
}

type noNotifier struct{}

func (noNotifier) Received(context.Context, string, string, string, string, time.Time) {}
func (noNotifier) Approved(context.Context, string, string, string, time.Time)         {}
func (noNotifier) Rejected(context.Context, string, string, string, *string)           {}
func (noNotifier) Suspended(context.Context, string, string, string, *string)          {}

var _ Notifier = (*notify.Notifier)(nil)
