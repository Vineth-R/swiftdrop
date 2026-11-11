// Package service holds the identity business rules: registration, password
// login with lockout, rotating refresh tokens and logout (API v1.1 sections 2.1
// to 2.4). It knows nothing about HTTP; handlers translate its errors.
package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/netip"
	"strings"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// Store is the persistence the service needs. *repository.Repository satisfies it.
type Store interface {
	CreateUser(ctx context.Context, params repository.CreateUserParams) (repository.User, error)
	CreateUserWithVerificationToken(ctx context.Context, params repository.CreateUserParams, token repository.NewEmailVerificationToken) (repository.User, error)
	IssueEmailVerificationToken(ctx context.Context, token repository.NewEmailVerificationToken, now time.Time) error
	ConsumeEmailVerificationToken(ctx context.Context, tokenHash string, now time.Time) (string, error)
	UserByEmail(ctx context.Context, email string) (repository.User, error)
	UserByID(ctx context.Context, userID string) (repository.User, error)
	RecordSuccessfulLogin(ctx context.Context, userID string, now time.Time) error
	RecordFailedLogin(ctx context.Context, userID string, policy lockout.Policy, now time.Time) (lockout.State, error)
	CreateRefreshToken(ctx context.Context, token repository.NewRefreshToken) (string, error)
	RotateRefreshToken(ctx context.Context, oldHash string, next repository.NextRefreshToken, now time.Time) (string, error)
	RevokeRefreshToken(ctx context.Context, userID, tokenHash string, now time.Time) error
	UpdateProfile(ctx context.Context, userID string, change repository.ProfileChange) (repository.User, error)
	ChangePassword(ctx context.Context, userID, expectedHash, newHash string, now time.Time) error
}

// Hasher hashes and checks passwords. *password.Hasher satisfies it.
type Hasher interface {
	Hash(plain string) (string, error)
	Matches(hash, plain string) bool
	Burn(plain string)
}

// TokenIssuer signs access tokens. *auth.TokenManager satisfies it.
type TokenIssuer interface {
	IssueWithSession(userID string, role auth.Role, sessionVersion int) (string, auth.Claims, error)
}

// MerchantApprovalChecker reads the current merchant application status from
// the Merchant service. Merchant logins fail closed when it is unavailable.
//
// Approval returns the status (empty when the owner has no merchant record) and,
// for a REJECTED or SUSPENDED application, the administrator's reason.
type MerchantApprovalChecker interface {
	Approval(ctx context.Context, userID string) (status, reason string, err error)
}

// Config is the policy the service applies.
type Config struct {
	RefreshTTL           time.Duration
	EmailVerificationTTL time.Duration
	FrontendBaseURL      string
	// SupportEmail is shown to a merchant whose login is refused. Empty when
	// none is configured.
	SupportEmail string
	Lockout      lockout.Policy
}

// Option customises a Service.
type Option func(*Service)

// WithClock replaces the time source. It exists so tests can control time.
// Callers should use nowUTC, which normalizes the instant to UTC.
func WithClock(now func() time.Time) Option {
	return func(s *Service) {
		if now != nil {
			s.now = now
		}
	}
}

// WithLogger sets the logger used for security events. Events carry user IDs
// only, never emails, passwords or tokens.
func WithLogger(logger *slog.Logger) Option {
	return func(s *Service) {
		if logger != nil {
			s.logger = logger
		}
	}
}

// WithMerchantApproval sets the merchant status checker used by merchant
// login and refresh.
func WithMerchantApproval(checker MerchantApprovalChecker) Option {
	return func(s *Service) {
		if checker != nil {
			s.merchantApproval = checker
		}
	}
}

// Service implements the identity use cases.
type Service struct {
	store  Store
	hasher Hasher
	tokens TokenIssuer
	config Config
	now    func() time.Time
	logger *slog.Logger

	// notifier sends account emails. It is never nil.
	notifier Notifier

	// merchantApproval checks merchant status during login and refresh.
	// Without it merchant sessions fail closed.
	merchantApproval MerchantApprovalChecker
}

// New returns a Service.
func New(store Store, hasher Hasher, tokens TokenIssuer, config Config, opts ...Option) (*Service, error) {
	if store == nil || hasher == nil || tokens == nil {
		return nil, errors.New("identity service requires a store, a hasher and a token issuer")
	}
	if config.RefreshTTL <= 0 {
		return nil, errors.New("refresh token lifetime must be greater than zero")
	}
	if config.Lockout.MaxFailures < 1 || config.Lockout.Duration <= 0 {
		return nil, errors.New("lockout policy needs a positive failure limit and duration")
	}
	if config.EmailVerificationTTL <= 0 {
		config.EmailVerificationTTL = 30 * time.Minute
	}

	service := &Service{
		store:  store,
		hasher: hasher,
		tokens: tokens,
		config: config,
		now:    time.Now,
		logger: slog.New(slog.DiscardHandler),

		notifier: noNotifier{},
	}
	for _, opt := range opts {
		opt(service)
	}
	return service, nil
}

// nowUTC returns the configured clock's instant in UTC. Every persisted
// expiration value goes through this method.
func (s *Service) nowUTC() time.Time { return s.now().UTC() }

// Account is the public view of a user.
type Account struct {
	ID            string
	Email         string
	FullName      string
	Phone         string
	Role          auth.Role
	EmailVerified bool
	CreatedAt     time.Time
}

// Session is the result of a successful login or refresh.
type Session struct {
	Account          Account
	AccessToken      string
	AccessExpiresAt  time.Time
	ExpiresIn        int
	RefreshToken     string
	RefreshExpiresAt time.Time
}

// RegisterInput is a self-service customer registration.
type RegisterInput struct {
	Email    string
	Password string
	FullName string
	Phone    string
}

// LoginInput is an email and password login. UserAgent and IP are stored with
// the refresh token for session review.
type LoginInput struct {
	Email     string
	Password  string
	UserAgent string
	IP        netip.Addr
}

// RefreshInput exchanges a refresh token for a new session.
type RefreshInput struct {
	RefreshToken string
	UserAgent    string
	IP           netip.Addr
}

// Register creates a CUSTOMER account. It returns a *ValidationError for bad
// input and ErrEmailAlreadyExists for a duplicate email.
func (s *Service) Register(ctx context.Context, input RegisterInput) (Account, error) {
	email := NormalizeEmail(input.Email)
	if err := validateRegistration(input, email); err != nil {
		return Account{}, err
	}

	hash, err := s.hasher.Hash(input.Password)
	if err != nil {
		return Account{}, fmt.Errorf("register: %w", err)
	}

	token, err := newVerificationToken()
	if err != nil {
		return Account{}, fmt.Errorf("register: %w", err)
	}
	now := s.nowUTC()
	user, err := s.store.CreateUserWithVerificationToken(ctx, repository.CreateUserParams{
		Email:        email,
		PasswordHash: hash,
		FullName:     strings.TrimSpace(input.FullName),
		Phone:        input.Phone,
		Role:         auth.RoleCustomer,
	}, repository.NewEmailVerificationToken{
		UserID:    "",
		TokenHash: hashVerificationToken(token),
		ExpiresAt: now.Add(s.config.EmailVerificationTTL),
	})
	if errors.Is(err, repository.ErrEmailTaken) {
		return Account{}, ErrEmailAlreadyExists
	}
	if err != nil {
		return Account{}, fmt.Errorf("register: %w", err)
	}
	s.notifier.VerifyEmail(ctx, recipientOf(user), s.verificationLink(token), s.config.EmailVerificationTTL)
	return accountOf(user), nil
}

// Login checks an email and password and starts a session.
//
// Errors: ErrInvalidCredentials (unknown email, wrong password or no password),
// ErrAccountLocked, ErrAccountDisabled. A disabled account is reported only
// after the correct password, so the status is not exposed to guessers.
func (s *Service) Login(ctx context.Context, input LoginInput) (Session, error) {
	now := s.nowUTC()

	user, err := s.store.UserByEmail(ctx, NormalizeEmail(input.Email))
	if errors.Is(err, repository.ErrNotFound) {
		s.hasher.Burn(input.Password)
		return Session{}, ErrInvalidCredentials
	}
	if err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}

	if user.LockoutState().Locked(now) {
		return Session{}, ErrAccountLocked
	}

	// A Google-only account has no password to guess. Failed attempts on it are
	// not counted, so password guessing cannot lock the owner out of Google login.
	if user.PasswordHash == nil {
		s.hasher.Burn(input.Password)
		return Session{}, ErrInvalidCredentials
	}

	if !s.hasher.Matches(*user.PasswordHash, input.Password) {
		state, err := s.store.RecordFailedLogin(ctx, user.ID, s.config.Lockout, now)
		if err != nil {
			return Session{}, fmt.Errorf("login: %w", err)
		}
		if state.Locked(now) {
			s.logger.WarnContext(ctx, "account locked after repeated failed logins", "user_id", user.ID)
			s.notifier.AccountLocked(ctx, recipientOf(user), s.config.Lockout.Duration)
		}
		return Session{}, ErrInvalidCredentials
	}

	if !user.IsActive {
		if user.Role == auth.RoleMerchant {
			return Session{}, s.inactiveMerchantLogin(ctx, user.ID)
		}
		return Session{}, ErrAccountDisabled
	}

	if user.Role == auth.RoleMerchant {
		status, reason, err := s.merchantStanding(ctx, user.ID)
		if err != nil {
			return Session{}, err
		}
		if status != "APPROVED" {
			return Session{}, s.inactiveMerchantError(status, reason)
		}
	}
	if user.Role == auth.RoleCustomer && !user.EmailVerified {
		return Session{}, ErrEmailVerificationRequired
	}

	if err := s.store.RecordSuccessfulLogin(ctx, user.ID, now); err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}

	refresh, err := s.newRefreshRecord(input.UserAgent, input.IP, now)
	if err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	if _, err := s.store.CreateRefreshToken(ctx, repository.NewRefreshToken{
		UserID:    user.ID,
		TokenHash: refresh.record.TokenHash,
		ExpiresAt: refresh.expiresAt,
		UserAgent: input.UserAgent,
		IPAddress: refresh.record.IPAddress,
	}); err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}

	session, err := s.sessionFor(user, refresh)
	if err != nil {
		return Session{}, fmt.Errorf("login: %w", err)
	}
	return session, nil
}

// Refresh exchanges a refresh token for a new access token and a rotated
// refresh token. The presented token stops working. It returns
// ErrRefreshTokenInvalid for any token that is not currently usable. Presenting
// a token that was already rotated, after the short grace period, also revokes
// the session it belonged to and logs a warning (repository.RefreshReuseGrace).
func (s *Service) Refresh(ctx context.Context, input RefreshInput) (Session, error) {
	if !plausibleRefreshToken(input.RefreshToken) {
		return Session{}, ErrRefreshTokenInvalid
	}
	now := s.nowUTC()

	refresh, err := s.newRefreshRecord(input.UserAgent, input.IP, now)
	if err != nil {
		return Session{}, fmt.Errorf("refresh: %w", err)
	}

	userID, err := s.store.RotateRefreshToken(ctx, hashRefreshToken(input.RefreshToken), refresh.record, now)
	if errors.Is(err, repository.ErrRefreshTokenReused) {
		// The caller still sees an ordinary invalid token, so it learns nothing
		// about the session it tried to take over.
		s.logger.WarnContext(ctx, "refresh token reuse detected, session revoked", "user_id", userID)
	}
	if errors.Is(err, repository.ErrRefreshTokenInvalid) {
		return Session{}, ErrRefreshTokenInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("refresh: %w", err)
	}

	user, err := s.store.UserByID(ctx, userID)
	if errors.Is(err, repository.ErrNotFound) {
		return Session{}, ErrRefreshTokenInvalid
	}
	if err != nil {
		return Session{}, fmt.Errorf("refresh: %w", err)
	}
	if !user.IsActive {
		return Session{}, ErrRefreshTokenInvalid
	}
	if user.Role == auth.RoleMerchant {
		approved, err := s.merchantApproved(ctx, user.ID)
		if err != nil {
			return Session{}, err
		}
		if !approved {
			return Session{}, ErrRefreshTokenInvalid
		}
	}
	if user.Role == auth.RoleCustomer && !user.EmailVerified {
		return Session{}, ErrRefreshTokenInvalid
	}

	session, err := s.sessionFor(user, refresh)
	if err != nil {
		return Session{}, fmt.Errorf("refresh: %w", err)
	}
	return session, nil
}

// Logout revokes the caller's current refresh token. It always succeeds for a
// missing or unknown token, so signing out is never blocked.
func (s *Service) Logout(ctx context.Context, userID, refreshToken string) error {
	if !plausibleRefreshToken(refreshToken) {
		return nil
	}
	if err := s.store.RevokeRefreshToken(ctx, userID, hashRefreshToken(refreshToken), s.nowUTC()); err != nil {
		return fmt.Errorf("logout: %w", err)
	}
	return nil
}

// refreshRecord pairs a freshly generated refresh token with the record that
// stores its hash.
type refreshRecord struct {
	token     string
	expiresAt time.Time
	record    repository.NextRefreshToken
}

func (s *Service) newRefreshRecord(userAgent string, ip netip.Addr, now time.Time) (refreshRecord, error) {
	token, hash, err := newRefreshToken()
	if err != nil {
		return refreshRecord{}, err
	}

	expiresAt := now.Add(s.config.RefreshTTL)
	return refreshRecord{
		token:     token,
		expiresAt: expiresAt,
		record: repository.NextRefreshToken{
			TokenHash: hash,
			ExpiresAt: expiresAt,
			UserAgent: userAgent,
			IPAddress: addressPointer(ip),
		},
	}, nil
}

// sessionFor issues an access token for user and pairs it with the refresh
// token that has already been stored.
func (s *Service) sessionFor(user repository.User, refresh refreshRecord) (Session, error) {
	accessToken, claims, err := s.tokens.IssueWithSession(user.ID, user.Role, user.SessionVersion)
	if err != nil {
		return Session{}, err
	}

	return Session{
		Account:          accountOf(user),
		AccessToken:      accessToken,
		AccessExpiresAt:  claims.ExpiresAt,
		ExpiresIn:        int(claims.ExpiresAt.Sub(claims.IssuedAt) / time.Second),
		RefreshToken:     refresh.token,
		RefreshExpiresAt: refresh.expiresAt,
	}, nil
}

func accountOf(user repository.User) Account {
	account := Account{
		ID:            user.ID,
		Email:         user.Email,
		FullName:      user.FullName,
		Role:          user.Role,
		EmailVerified: user.EmailVerified,
		CreatedAt:     user.CreatedAt,
	}
	if user.Phone != nil {
		account.Phone = *user.Phone
	}
	return account
}

func addressPointer(ip netip.Addr) *netip.Addr {
	if !ip.IsValid() {
		return nil
	}
	return &ip
}
