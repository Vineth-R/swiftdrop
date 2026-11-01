package service

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const (
	// oauthStateTTL is how long a Google login attempt stays valid (DB v1.1
	// section 3.5: about 10 minutes).
	oauthStateTTL = 10 * time.Minute

	// oauthRandomBytes gives 43 URL-safe characters, which fits the 64 character
	// state column and satisfies the 43 character minimum for a PKCE verifier.
	oauthRandomBytes = 32

	// maxOAuthStateLength is the state column size. Longer input cannot be a
	// state we issued, so it is refused before touching the database.
	maxOAuthStateLength = 64

	maxCodeLength = 2048

	// maxGoogleSubjectLength is the users.google_sub column size.
	maxGoogleSubjectLength = 100

	// oauthBindingCookiePrefix names the per-attempt browser-binding cookie.
	// The state prefix keeps concurrent login attempts in separate tabs usable.
	oauthBindingCookiePrefix = "sd_oauth_binding_"

	// oauthBindingStatePrefixLength is how much of the state appears in the
	// cookie name. The state itself is public (it is in Google's redirect URL);
	// the binding value inside the cookie is the secret.
	oauthBindingStatePrefixLength = 8

	// maxOAuthBindingLength bounds the work done on untrusted cookie input. A
	// real binding is 43 base64url characters.
	maxOAuthBindingLength = 128
)

var (
	// ErrOAuthStateInvalid means the state is missing, unknown, expired or
	// already used (400 OAUTH_STATE_INVALID).
	ErrOAuthStateInvalid = errors.New("oauth state invalid")

	// ErrOAuthProvider means Google failed or returned something unusable
	// (502 OAUTH_PROVIDER_ERROR).
	ErrOAuthProvider = errors.New("oauth provider error")

	// ErrGoogleEmailNotVerified means Google did not vouch for the email, so the
	// account cannot be matched or created from it.
	ErrGoogleEmailNotVerified = errors.New("google email not verified")
)

// GoogleProfile is the identity Google vouches for after a verified login.
type GoogleProfile struct {
	Subject       string
	Email         string
	EmailVerified bool
	FullName      string
}

// GoogleProvider talks to Google. internal/google implements it; tests use a
// fake so no request ever leaves the machine.
type GoogleProvider interface {
	// AuthURL returns the Google consent URL for state, protected by the PKCE
	// codeVerifier.
	AuthURL(state, codeVerifier string) string
	// Exchange swaps the authorization code for tokens and returns the profile
	// from the verified ID token. Any failure is returned as an error.
	Exchange(ctx context.Context, code, codeVerifier string) (GoogleProfile, error)
}

// GoogleStore is the persistence Google sign-in needs. *repository.Repository
// satisfies it.
type GoogleStore interface {
	CreateOAuthState(ctx context.Context, state repository.NewOAuthState) error
	ConsumeOAuthState(ctx context.Context, state, browserBindingHash string, now time.Time) (repository.OAuthState, error)
	PurgeExpiredOAuthStates(ctx context.Context, now time.Time) error
	UserByGoogleSub(ctx context.Context, googleSub string) (repository.User, error)
	UserByEmail(ctx context.Context, email string) (repository.User, error)
	CreateGoogleUser(ctx context.Context, params repository.CreateGoogleUserParams) (repository.User, error)
	LinkGoogleAccount(ctx context.Context, userID, googleSub string) error
	RecordSuccessfulLogin(ctx context.Context, userID string, now time.Time) error
	CreateRefreshToken(ctx context.Context, token repository.NewRefreshToken) (string, error)
}

// GoogleCallbackInput is what Google's redirect delivers, plus the browser
// binding cookie value and request details stored with the new refresh token.
type GoogleCallbackInput struct {
	Code           string
	State          string
	BrowserBinding string
	UserAgent      string
	IP             netip.Addr
}

// GoogleLoginStart is a started login: where to send the browser and the
// short-lived cookie that binds the attempt to that browser.
type GoogleLoginStart struct {
	AuthURL           string
	BindingCookieName string
	BindingValue      string
}

// OAuthBindingCookieName returns the browser-binding cookie name for a state.
// It reports false when the state cannot safely appear in a cookie name.
func OAuthBindingCookieName(state string) (string, bool) {
	if len(state) < oauthBindingStatePrefixLength || len(state) > maxOAuthStateLength {
		return "", false
	}
	for _, r := range state {
		if !(r >= 'a' && r <= 'z') && !(r >= 'A' && r <= 'Z') && !(r >= '0' && r <= '9') && r != '-' && r != '_' {
			return "", false
		}
	}
	return oauthBindingCookiePrefix + state[:oauthBindingStatePrefixLength], true
}

// GoogleLoginResult is a finished Google sign-in. RedirectTo is the relative
// path chosen when the login started, or empty.
type GoogleLoginResult struct {
	Session    Session
	RedirectTo string
}

// GoogleAuth implements Google sign-in (API v1.1 sections 2.5 and 2.6). It
// reuses Service for refresh tokens and sessions so every login method issues
// identical sessions.
type GoogleAuth struct {
	base     *Service
	store    GoogleStore
	provider GoogleProvider
}

// NewGoogleAuth returns a GoogleAuth that builds sessions with base.
func NewGoogleAuth(base *Service, store GoogleStore, provider GoogleProvider) (*GoogleAuth, error) {
	if base == nil || store == nil || provider == nil {
		return nil, errors.New("google auth requires a service, a store and a provider")
	}
	return &GoogleAuth{base: base, store: store, provider: provider}, nil
}

// StartGoogleLogin remembers a new login attempt and returns the Google URL to
// send the browser to, plus the browser-binding cookie the handler must set.
// redirectTo is optional; when given it must be a safe relative path, otherwise
// a *ValidationError is returned.
func (g *GoogleAuth) StartGoogleLogin(ctx context.Context, redirectTo string) (GoogleLoginStart, error) {
	if redirectTo != "" && !ValidRedirectPath(redirectTo) {
		return GoogleLoginStart{}, &ValidationError{Issues: []Issue{{Field: "redirectTo", Issue: "must be a relative path such as /dashboard"}}}
	}

	state, err := randomURLSafe()
	if err != nil {
		return GoogleLoginStart{}, fmt.Errorf("start google login: %w", err)
	}
	verifier, err := randomURLSafe()
	if err != nil {
		return GoogleLoginStart{}, fmt.Errorf("start google login: %w", err)
	}
	binding, err := randomURLSafe()
	if err != nil {
		return GoogleLoginStart{}, fmt.Errorf("start google login: %w", err)
	}
	cookieName, ok := OAuthBindingCookieName(state)
	if !ok {
		return GoogleLoginStart{}, errors.New("start google login: generated state cannot be bound to a cookie")
	}

	now := g.base.now()
	// Housekeeping only: a failure here must not stop someone from signing in.
	if err := g.store.PurgeExpiredOAuthStates(ctx, now); err != nil {
		g.base.logger.WarnContext(ctx, "could not purge expired oauth states", "error", err.Error())
	}

	if err := g.store.CreateOAuthState(ctx, repository.NewOAuthState{
		State:              state,
		CodeVerifier:       verifier,
		RedirectTo:         redirectTo,
		BrowserBindingHash: hashOAuthBinding(binding),
		ExpiresAt:          now.Add(oauthStateTTL),
	}); err != nil {
		return GoogleLoginStart{}, fmt.Errorf("start google login: %w", err)
	}
	return GoogleLoginStart{
		AuthURL:           g.provider.AuthURL(state, verifier),
		BindingCookieName: cookieName,
		BindingValue:      binding,
	}, nil
}

// CompleteGoogleLogin finishes a login after Google redirects back. It checks
// and burns the state, verifies the Google identity, finds the account by
// google_sub and then by email (or creates a CUSTOMER) and starts a session.
//
// Errors: ErrOAuthStateInvalid, ErrOAuthProvider, ErrGoogleEmailNotVerified,
// ErrAccountDisabled.
func (g *GoogleAuth) CompleteGoogleLogin(ctx context.Context, input GoogleCallbackInput) (GoogleLoginResult, error) {
	if input.State == "" || len(input.State) > maxOAuthStateLength {
		return GoogleLoginResult{}, ErrOAuthStateInvalid
	}
	// A missing binding is rejected before the database is touched, so a
	// callback without the initiating browser's cookie cannot burn the state.
	if input.BrowserBinding == "" || len(input.BrowserBinding) > maxOAuthBindingLength {
		return GoogleLoginResult{}, ErrOAuthStateInvalid
	}
	now := g.base.now()

	// The state is consumed before anything else, so it works exactly once even
	// if the rest of the login fails. The binding hash is part of the atomic
	// DELETE, so a wrong binding does not consume the legitimate state.
	saved, err := g.store.ConsumeOAuthState(ctx, input.State, hashOAuthBinding(input.BrowserBinding), now)
	if errors.Is(err, repository.ErrOAuthStateInvalid) {
		return GoogleLoginResult{}, ErrOAuthStateInvalid
	}
	if err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w", err)
	}

	if input.Code == "" || len(input.Code) > maxCodeLength {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w: missing authorization code", ErrOAuthProvider)
	}
	profile, err := g.provider.Exchange(ctx, input.Code, saved.CodeVerifier)
	if err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w: %w", ErrOAuthProvider, err)
	}

	email := normalizeEmail(profile.Email)
	if profile.Subject == "" || len(profile.Subject) > maxGoogleSubjectLength || !validEmail(email) {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w: incomplete profile", ErrOAuthProvider)
	}
	if !profile.EmailVerified {
		return GoogleLoginResult{}, ErrGoogleEmailNotVerified
	}

	user, err := g.resolveUser(ctx, profile, email)
	if err != nil {
		return GoogleLoginResult{}, err
	}
	if !user.IsActive {
		return GoogleLoginResult{}, ErrAccountDisabled
	}

	if err := g.store.RecordSuccessfulLogin(ctx, user.ID, now); err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w", err)
	}
	refresh, err := g.base.newRefreshRecord(input.UserAgent, input.IP, now)
	if err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w", err)
	}
	if _, err := g.store.CreateRefreshToken(ctx, repository.NewRefreshToken{
		UserID:    user.ID,
		TokenHash: refresh.record.TokenHash,
		ExpiresAt: refresh.expiresAt,
		UserAgent: input.UserAgent,
		IPAddress: refresh.record.IPAddress,
	}); err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w", err)
	}
	session, err := g.base.sessionFor(user, refresh)
	if err != nil {
		return GoogleLoginResult{}, fmt.Errorf("google login: %w", err)
	}
	return GoogleLoginResult{Session: session, RedirectTo: saved.RedirectTo}, nil
}

// resolveUser finds the account for a verified Google profile: by google_sub,
// then by email (linking the Google account), otherwise it creates a CUSTOMER.
func (g *GoogleAuth) resolveUser(ctx context.Context, profile GoogleProfile, email string) (repository.User, error) {
	user, err := g.store.UserByGoogleSub(ctx, profile.Subject)
	if err == nil {
		return user, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, fmt.Errorf("google login: %w", err)
	}

	user, err = g.store.UserByEmail(ctx, email)
	if err == nil {
		if !user.IsActive {
			// The caller refuses a disabled account; do not link it first.
			return user, nil
		}
		// Google verified this email, so the owner of the Google account owns the
		// SwiftDrop account. Link them so the next login matches on google_sub.
		if err := g.store.LinkGoogleAccount(ctx, user.ID, profile.Subject); err != nil {
			if errors.Is(err, repository.ErrGoogleAccountConflict) {
				return repository.User{}, fmt.Errorf("google login: %w: account linked to another google account", ErrOAuthProvider)
			}
			return repository.User{}, fmt.Errorf("google login: %w", err)
		}
		g.base.logger.InfoContext(ctx, "google account linked to existing user", "user_id", user.ID)
		return user, nil
	}
	if !errors.Is(err, repository.ErrNotFound) {
		return repository.User{}, fmt.Errorf("google login: %w", err)
	}

	user, err = g.store.CreateGoogleUser(ctx, repository.CreateGoogleUserParams{
		Email:     email,
		FullName:  googleDisplayName(profile.FullName, email),
		GoogleSub: profile.Subject,
		Role:      auth.RoleCustomer,
	})
	if err != nil {
		return repository.User{}, fmt.Errorf("google login: %w", err)
	}
	return user, nil
}

// googleDisplayName fits Google's name into the 2 to 150 character full_name
// rule. A missing or one-letter name falls back to the part of the email before
// the @.
func googleDisplayName(name, email string) string {
	name = truncateRunes(strings.TrimSpace(name), maxFullNameLength)
	if utf8.RuneCountInString(name) >= minFullNameLength {
		return name
	}
	local, _, _ := strings.Cut(email, "@")
	local = truncateRunes(local, maxFullNameLength)
	if utf8.RuneCountInString(local) >= minFullNameLength {
		return local
	}
	return "Google User"
}

func truncateRunes(value string, limit int) string {
	if utf8.RuneCountInString(value) <= limit {
		return value
	}
	return string([]rune(value)[:limit])
}

// hashOAuthBinding is the SHA-256 hex digest stored in oauth_states. The raw
// cookie value never reaches PostgreSQL or the logs.
func hashOAuthBinding(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomURLSafe() (string, error) {
	var buffer [oauthRandomBytes]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", fmt.Errorf("generate random value: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buffer[:]), nil
}
