package service

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// googleMemoryStore is an in-memory GoogleStore.
type googleMemoryStore struct {
	states         map[string]repository.NewOAuthState
	users          map[string]*repository.User
	subs           map[string]string // google_sub -> user ID
	tokens         []repository.NewRefreshToken
	logins         []string
	revokedUserIDs []string
	nextID         int

	failOn       map[string]error
	beforeCreate func()
}

func newGoogleMemoryStore() *googleMemoryStore {
	return &googleMemoryStore{
		states: map[string]repository.NewOAuthState{},
		users:  map[string]*repository.User{},
		subs:   map[string]string{},
		failOn: map[string]error{},
	}
}

func (s *googleMemoryStore) addUser(email string, active bool, provider string) *repository.User {
	s.nextID++
	user := &repository.User{
		ID: "user-" + string(rune('0'+s.nextID)), Email: email, FullName: "Existing User",
		Role: auth.RoleCustomer, AuthProvider: provider,
		EmailVerified: provider == "GOOGLE", IsActive: active,
	}
	s.users[user.ID] = user
	return user
}

func (s *googleMemoryStore) CreateOAuthState(_ context.Context, state repository.NewOAuthState) error {
	if err := s.failOn["CreateOAuthState"]; err != nil {
		return err
	}
	s.states[state.State] = state
	return nil
}

func (s *googleMemoryStore) ConsumeOAuthState(_ context.Context, state, browserBindingHash string, now time.Time) (repository.OAuthState, error) {
	if err := s.failOn["ConsumeOAuthState"]; err != nil {
		return repository.OAuthState{}, err
	}
	saved, ok := s.states[state]
	if !ok || !now.Before(saved.ExpiresAt) || saved.BrowserBindingHash == "" || browserBindingHash != saved.BrowserBindingHash {
		return repository.OAuthState{}, repository.ErrOAuthStateInvalid
	}
	delete(s.states, state)
	return repository.OAuthState{CodeVerifier: saved.CodeVerifier, RedirectTo: saved.RedirectTo}, nil
}

func (s *googleMemoryStore) PurgeExpiredOAuthStates(_ context.Context, now time.Time) error {
	if err := s.failOn["PurgeExpiredOAuthStates"]; err != nil {
		return err
	}
	for key, saved := range s.states {
		if !now.Before(saved.ExpiresAt) {
			delete(s.states, key)
		}
	}
	return nil
}

func (s *googleMemoryStore) UserByGoogleSub(_ context.Context, sub string) (repository.User, error) {
	if id, ok := s.subs[sub]; ok {
		return *s.users[id], nil
	}
	return repository.User{}, repository.ErrNotFound
}

func (s *googleMemoryStore) UserByEmail(_ context.Context, email string) (repository.User, error) {
	for _, user := range s.users {
		if strings.EqualFold(user.Email, email) {
			return *user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (s *googleMemoryStore) CreateGoogleUser(_ context.Context, params repository.CreateGoogleUserParams) (repository.User, error) {
	if s.beforeCreate != nil {
		s.beforeCreate()
	}
	if err := s.failOn["CreateGoogleUser"]; err != nil {
		return repository.User{}, err
	}
	user := s.addUser(params.Email, true, "GOOGLE")
	user.FullName, user.Role = params.FullName, params.Role
	s.subs[params.GoogleSub] = user.ID
	return *user, nil
}

func (s *googleMemoryStore) LinkGoogleAccount(_ context.Context, userID, sub string, _ time.Time) (bool, error) {
	if err := s.failOn["LinkGoogleAccount"]; err != nil {
		return false, err
	}
	user, ok := s.users[userID]
	if !ok {
		return false, repository.ErrNotFound
	}
	for existingSub, id := range s.subs {
		if id == userID && existingSub != sub {
			return false, repository.ErrGoogleAccountConflict
		}
	}
	cleared := !user.EmailVerified
	if cleared {
		user.PasswordHash = nil
		s.revokedUserIDs = append(s.revokedUserIDs, userID)
	}
	user.EmailVerified = true
	s.subs[sub] = userID
	return cleared, nil
}

func (s *googleMemoryStore) RecordSuccessfulLogin(_ context.Context, userID string, _ time.Time) error {
	s.logins = append(s.logins, userID)
	return nil
}

func (s *googleMemoryStore) CreateRefreshToken(_ context.Context, token repository.NewRefreshToken) (string, error) {
	if err := s.failOn["CreateRefreshToken"]; err != nil {
		return "", err
	}
	s.tokens = append(s.tokens, token)
	return "token-id", nil
}

type fakeProvider struct {
	profile GoogleProfile
	err     error

	gotCode, gotVerifier string
	gotState             string
}

func (p *fakeProvider) AuthURL(state, verifier string) string {
	p.gotState = state
	return "https://accounts.example/auth?state=" + state + "&v=" + verifier
}

func (p *fakeProvider) Exchange(_ context.Context, code, verifier string) (GoogleProfile, error) {
	p.gotCode, p.gotVerifier = code, verifier
	return p.profile, p.err
}

type googleFixture struct {
	*fixture
	google   *GoogleAuth
	store    *googleMemoryStore
	provider *fakeProvider
}

func newGoogleFixture(t *testing.T) *googleFixture {
	t.Helper()
	base := newFixture(t)
	store := newGoogleMemoryStore()
	provider := &fakeProvider{profile: GoogleProfile{
		Subject: "google-sub-1", Email: "Nimal@Example.com", EmailVerified: true, FullName: "Nimal Perera",
	}}
	google, err := NewGoogleAuth(base.service, store, provider)
	if err != nil {
		t.Fatalf("NewGoogleAuth returned error: %v", err)
	}
	return &googleFixture{fixture: base, google: google, store: store, provider: provider}
}

// startedLogin is a login attempt plus its browser binding.
type startedLogin struct {
	state   string
	binding string
}

// begin starts a login and returns the saved state and binding value.
func (f *googleFixture) begin(t *testing.T, redirectTo string) startedLogin {
	t.Helper()
	start, err := f.google.StartGoogleLogin(context.Background(), redirectTo)
	if err != nil {
		t.Fatalf("StartGoogleLogin returned error: %v", err)
	}
	return startedLogin{state: f.provider.gotState, binding: start.BindingValue}
}

func (f *googleFixture) callback(start startedLogin) (GoogleLoginResult, error) {
	return f.callbackRaw(start.state, start.binding)
}

func (f *googleFixture) callbackRaw(state, binding string) (GoogleLoginResult, error) {
	return f.google.CompleteGoogleLogin(context.Background(), GoogleCallbackInput{
		Code: "auth-code", State: state, BrowserBinding: binding, UserAgent: "test-agent", IP: netip.MustParseAddr("203.0.113.5"),
	})
}

func TestNewGoogleAuth(t *testing.T) {
	base := newFixture(t)
	if _, err := NewGoogleAuth(nil, newGoogleMemoryStore(), &fakeProvider{}); err == nil {
		t.Fatal("NewGoogleAuth accepted a nil service")
	}
	if _, err := NewGoogleAuth(base.service, nil, &fakeProvider{}); err == nil {
		t.Fatal("NewGoogleAuth accepted a nil store")
	}
	if _, err := NewGoogleAuth(base.service, newGoogleMemoryStore(), nil); err == nil {
		t.Fatal("NewGoogleAuth accepted a nil provider")
	}
}

func TestGoogleAuth_StartGoogleLogin(t *testing.T) {
	f := newGoogleFixture(t)

	start, err := f.google.StartGoogleLogin(context.Background(), "/parcels")
	if err != nil {
		t.Fatalf("StartGoogleLogin returned error: %v", err)
	}
	state := f.provider.gotState
	if len(state) < 43 || len(state) > maxOAuthStateLength {
		t.Fatalf("state length = %d, want 43 to %d", len(state), maxOAuthStateLength)
	}
	if !strings.Contains(start.AuthURL, state) {
		t.Fatal("authorization URL does not carry the state")
	}
	if len(start.BindingValue) < 43 {
		t.Fatalf("binding length = %d, want at least 43", len(start.BindingValue))
	}
	wantCookie, ok := OAuthBindingCookieName(state)
	if !ok || start.BindingCookieName != wantCookie {
		t.Fatalf("binding cookie = %q, want %q", start.BindingCookieName, wantCookie)
	}

	saved, ok := f.store.states[state]
	if !ok {
		t.Fatal("state was not saved")
	}
	if saved.RedirectTo != "/parcels" || len(saved.CodeVerifier) < 43 {
		t.Fatalf("saved state = %+v", saved)
	}
	if saved.BrowserBindingHash != hashOAuthBinding(start.BindingValue) {
		t.Fatal("saved binding hash does not match the cookie value")
	}
	if want := f.now.Add(oauthStateTTL); !saved.ExpiresAt.Equal(want) {
		t.Fatalf("state expires at %v, want %v", saved.ExpiresAt, want)
	}
	if !strings.Contains(start.AuthURL, saved.CodeVerifier) {
		t.Fatal("provider was not given the saved PKCE verifier")
	}

	t.Run("two logins never share a state", func(t *testing.T) {
		if second := f.begin(t, ""); second.state == state {
			t.Fatal("state was reused")
		}
	})
}

func TestGoogleAuth_StartGoogleLogin_rejectsUnsafeRedirect(t *testing.T) {
	f := newGoogleFixture(t)

	for _, redirect := range []string{"https://evil.example", "//evil.example", `/\evil.example`, "dashboard"} {
		_, err := f.google.StartGoogleLogin(context.Background(), redirect)
		var validation *ValidationError
		if !errors.As(err, &validation) || validation.Issues[0].Field != "redirectTo" {
			t.Fatalf("StartGoogleLogin(%q) error = %v, want a redirectTo validation error", redirect, err)
		}
	}
	if len(f.store.states) != 0 {
		t.Fatal("a state was saved for an unsafe redirect")
	}
}

func TestGoogleAuth_StartGoogleLogin_purgesExpiredStatesAndSurvivesPurgeFailure(t *testing.T) {
	f := newGoogleFixture(t)
	f.store.states["stale"] = repository.NewOAuthState{State: "stale", ExpiresAt: f.now.Add(-time.Minute)}

	f.begin(t, "")
	if _, ok := f.store.states["stale"]; ok {
		t.Fatal("expired state was not purged")
	}

	f.store.failOn["PurgeExpiredOAuthStates"] = errStore
	if _, err := f.google.StartGoogleLogin(context.Background(), ""); err != nil {
		t.Fatalf("a purge failure must not block login, got %v", err)
	}
}

func TestGoogleAuth_StartGoogleLogin_storeFailure(t *testing.T) {
	f := newGoogleFixture(t)
	f.store.failOn["CreateOAuthState"] = errStore

	if _, err := f.google.StartGoogleLogin(context.Background(), ""); !errors.Is(err, errStore) {
		t.Fatalf("StartGoogleLogin() error = %v, want the store error", err)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_createsCustomer(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "/parcels")
	verifier := f.store.states[start.state].CodeVerifier

	result, err := f.callback(start)
	if err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}

	if f.provider.gotCode != "auth-code" || f.provider.gotVerifier != verifier {
		t.Fatalf("provider got code %q verifier %q", f.provider.gotCode, f.provider.gotVerifier)
	}
	account := result.Session.Account
	if account.Role != auth.RoleCustomer || account.Email != "nimal@example.com" || account.FullName != "Nimal Perera" {
		t.Fatalf("new account = %+v", account)
	}
	if result.RedirectTo != "/parcels" {
		t.Fatalf("RedirectTo = %q, want /parcels", result.RedirectTo)
	}

	stored := f.store.users[account.ID]
	if stored.AuthProvider != "GOOGLE" || stored.PasswordHash != nil {
		t.Fatalf("stored user = %+v, want a GOOGLE user without a password", stored)
	}
	if f.store.subs["google-sub-1"] != account.ID {
		t.Fatal("google_sub was not saved")
	}

	if result.Session.AccessToken == "" || !strings.HasPrefix(result.Session.RefreshToken, "rt_") {
		t.Fatal("session tokens were not issued")
	}
	claims, err := f.tokens.Verify(result.Session.AccessToken)
	if err != nil || claims.UserID != account.ID || claims.Role != auth.RoleCustomer {
		t.Fatalf("access token claims = %+v, err = %v", claims, err)
	}
	if len(f.store.tokens) != 1 || f.store.tokens[0].TokenHash != hashRefreshToken(result.Session.RefreshToken) {
		t.Fatal("only the refresh token hash may be stored")
	}
	if f.store.tokens[0].UserAgent != "test-agent" || f.store.tokens[0].IPAddress == nil {
		t.Fatal("request details were not stored with the refresh token")
	}
	if len(f.store.logins) != 1 {
		t.Fatal("successful login was not recorded")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_stateIsSingleUse(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")

	if _, err := f.callback(start); err != nil {
		t.Fatalf("first callback returned error: %v", err)
	}
	if _, err := f.callback(start); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("replayed state error = %v, want ErrOAuthStateInvalid", err)
	}
	if len(f.store.tokens) != 1 {
		t.Fatal("a replayed state created a second session")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_rejectsBadState(t *testing.T) {
	f := newGoogleFixture(t)
	valid := f.begin(t, "")

	tests := []struct {
		name  string
		state string
		wait  time.Duration
	}{
		{name: "empty", state: ""},
		{name: "never issued", state: "not-a-real-state"},
		{name: "too long", state: strings.Repeat("a", maxOAuthStateLength+1)},
		{name: "expired", state: valid.state, wait: oauthStateTTL},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f.advance(test.wait)
			if _, err := f.callbackRaw(test.state, valid.binding); !errors.Is(err, ErrOAuthStateInvalid) {
				t.Fatalf("error = %v, want ErrOAuthStateInvalid", err)
			}
		})
	}
	if f.provider.gotCode != "" {
		t.Fatal("Google was contacted for an invalid state")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_missingBindingDoesNotBurnState(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")

	if _, err := f.callbackRaw(start.state, ""); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("error = %v, want ErrOAuthStateInvalid", err)
	}
	if _, err := f.callback(start); err != nil {
		t.Fatalf("the legitimate callback after a missing binding failed: %v", err)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_mismatchedBindingDoesNotBurnState(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")

	if _, err := f.callbackRaw(start.state, "not-the-binding-value"); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("error = %v, want ErrOAuthStateInvalid", err)
	}
	if _, err := f.callback(start); err != nil {
		t.Fatalf("the legitimate callback after a mismatched binding failed: %v", err)
	}
	if f.provider.gotCode != "auth-code" {
		t.Fatal("Google was not contacted for the legitimate retry")
	}
}

func TestOAuthBindingCookieName(t *testing.T) {
	valid := strings.Repeat("a", 43)
	name, ok := OAuthBindingCookieName(valid)
	if !ok || name != "sd_oauth_binding_aaaaaaaa" {
		t.Fatalf("OAuthBindingCookieName(%q) = %q, %t", valid, name, ok)
	}

	for _, state := range []string{"", "short", strings.Repeat("a", maxOAuthStateLength+1), "bad/state/with/slashes", "bad state"} {
		if _, ok := OAuthBindingCookieName(state); ok {
			t.Fatalf("OAuthBindingCookieName(%q) accepted an unsafe state", state)
		}
	}
}

func TestGoogleAuth_CompleteGoogleLogin_stateIsBurnedEvenWhenGoogleFails(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")
	f.provider.err = errors.New("google is down")

	if _, err := f.callback(start); !errors.Is(err, ErrOAuthProvider) {
		t.Fatalf("error = %v, want ErrOAuthProvider", err)
	}
	f.provider.err = nil
	if _, err := f.callback(start); !errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("retry with the same state error = %v, want ErrOAuthStateInvalid", err)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_storeFailureOnState(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")
	f.store.failOn["ConsumeOAuthState"] = errStore

	_, err := f.callback(start)
	if !errors.Is(err, errStore) || errors.Is(err, ErrOAuthStateInvalid) {
		t.Fatalf("error = %v, want the store error, not an invalid state", err)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_missingCode(t *testing.T) {
	f := newGoogleFixture(t)
	start := f.begin(t, "")

	_, err := f.google.CompleteGoogleLogin(context.Background(), GoogleCallbackInput{
		State: start.state, BrowserBinding: start.binding,
	})
	if !errors.Is(err, ErrOAuthProvider) {
		t.Fatalf("error = %v, want ErrOAuthProvider", err)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_unusableProfiles(t *testing.T) {
	tests := []struct {
		name    string
		profile GoogleProfile
		want    error
	}{
		{name: "email not verified", profile: GoogleProfile{Subject: "s", Email: "a@example.com"}, want: ErrGoogleEmailNotVerified},
		{name: "subject too long for the column", profile: GoogleProfile{Subject: strings.Repeat("1", maxGoogleSubjectLength+1), Email: "a@example.com", EmailVerified: true}, want: ErrOAuthProvider},
		{name: "no subject", profile: GoogleProfile{Email: "a@example.com", EmailVerified: true}, want: ErrOAuthProvider},
		{name: "no email", profile: GoogleProfile{Subject: "s", EmailVerified: true}, want: ErrOAuthProvider},
		{name: "invalid email", profile: GoogleProfile{Subject: "s", Email: "nope", EmailVerified: true}, want: ErrOAuthProvider},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newGoogleFixture(t)
			f.provider.profile = test.profile
			start := f.begin(t, "")

			if _, err := f.callback(start); !errors.Is(err, test.want) {
				t.Fatalf("error = %v, want %v", err, test.want)
			}
			if len(f.store.users) != 0 || len(f.store.tokens) != 0 {
				t.Fatal("an account or session was created from an unusable profile")
			}
		})
	}
}

func TestGoogleAuth_CompleteGoogleLogin_existingGoogleSub(t *testing.T) {
	f := newGoogleFixture(t)
	existing := f.store.addUser("old-address@example.com", true, "GOOGLE")
	f.store.subs["google-sub-1"] = existing.ID

	result, err := f.callback(f.begin(t, ""))
	if err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}
	if result.Session.Account.ID != existing.ID {
		t.Fatalf("signed in as %q, want the account linked to the Google subject", result.Session.Account.ID)
	}
	if len(f.store.users) != 1 {
		t.Fatal("a duplicate account was created")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_linksExistingEmailAccount(t *testing.T) {
	f := newGoogleFixture(t)
	existing := f.store.addUser("nimal@example.com", true, "LOCAL")
	existing.Role = auth.RoleMerchant
	existing.PasswordHash = stringPtr("attacker-chosen-hash")

	result, err := f.callback(f.begin(t, ""))
	if err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}
	if result.Session.Account.ID != existing.ID || result.Session.Account.Role != auth.RoleMerchant {
		t.Fatalf("account = %+v, want the existing account with its role unchanged", result.Session.Account)
	}
	if f.store.subs["google-sub-1"] != existing.ID {
		t.Fatal("Google account was not linked")
	}
	if existing.PasswordHash != nil {
		t.Fatal("an unverified account kept its password after Google linking")
	}
	if !existing.EmailVerified {
		t.Fatal("the linked account was not marked email verified")
	}
	if len(f.store.revokedUserIDs) != 1 || f.store.revokedUserIDs[0] != existing.ID {
		t.Fatalf("revoked users = %v, want the linked user", f.store.revokedUserIDs)
	}
	if len(f.store.users) != 1 {
		t.Fatal("a duplicate account was created")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_keepsVerifiedPassword(t *testing.T) {
	f := newGoogleFixture(t)
	existing := f.store.addUser("nimal@example.com", true, "LOCAL")
	existing.EmailVerified = true
	existing.PasswordHash = stringPtr("legitimate-hash")

	if _, err := f.callback(f.begin(t, "")); err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}
	if existing.PasswordHash == nil {
		t.Fatal("a verified account lost its password on Google linking")
	}
	if len(f.store.revokedUserIDs) != 0 {
		t.Fatalf("sessions were revoked for a verified account: %v", f.store.revokedUserIDs)
	}
}

func TestGoogleAuth_CompleteGoogleLogin_creationRaceRelinks(t *testing.T) {
	f := newGoogleFixture(t)
	f.store.failOn["CreateGoogleUser"] = repository.ErrEmailTaken
	var winner *repository.User
	f.store.beforeCreate = func() {
		winner = f.store.addUser("nimal@example.com", true, "GOOGLE")
		f.store.subs["google-sub-1"] = winner.ID
	}

	result, err := f.callback(f.begin(t, ""))
	if err != nil {
		t.Fatalf("CompleteGoogleLogin returned error: %v", err)
	}
	if winner == nil || result.Session.Account.ID != winner.ID {
		t.Fatalf("signed in as %v, want the account created by the winning request", result.Session.Account.ID)
	}
	if len(f.store.users) != 1 {
		t.Fatal("the race created a duplicate account")
	}
	if len(f.store.tokens) != 1 {
		t.Fatal("the losing request did not receive a session")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_linkConflict(t *testing.T) {
	f := newGoogleFixture(t)
	f.store.addUser("nimal@example.com", true, "LOCAL")
	f.store.failOn["LinkGoogleAccount"] = repository.ErrGoogleAccountConflict

	if _, err := f.callback(f.begin(t, "")); !errors.Is(err, ErrOAuthProvider) {
		t.Fatalf("error = %v, want ErrOAuthProvider", err)
	}
	if len(f.store.tokens) != 0 {
		t.Fatal("a session was issued despite the conflict")
	}
}

func TestGoogleAuth_CompleteGoogleLogin_disabledAccount(t *testing.T) {
	t.Run("matched by email is not linked", func(t *testing.T) {
		f := newGoogleFixture(t)
		f.store.addUser("nimal@example.com", false, "LOCAL")

		if _, err := f.callback(f.begin(t, "")); !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("error = %v, want ErrAccountDisabled", err)
		}
		if len(f.store.subs) != 0 || len(f.store.tokens) != 0 {
			t.Fatal("a disabled account was linked or given a session")
		}
	})

	t.Run("matched by google_sub", func(t *testing.T) {
		f := newGoogleFixture(t)
		disabled := f.store.addUser("nimal@example.com", false, "GOOGLE")
		f.store.subs["google-sub-1"] = disabled.ID

		if _, err := f.callback(f.begin(t, "")); !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("error = %v, want ErrAccountDisabled", err)
		}
		if len(f.store.tokens) != 0 {
			t.Fatal("a disabled account was given a session")
		}
	})
}

func TestGoogleAuth_CompleteGoogleLogin_storeFailures(t *testing.T) {
	for _, op := range []string{"CreateGoogleUser", "CreateRefreshToken"} {
		t.Run(op, func(t *testing.T) {
			f := newGoogleFixture(t)
			f.store.failOn[op] = errStore

			if _, err := f.callback(f.begin(t, "")); !errors.Is(err, errStore) {
				t.Fatalf("error = %v, want the store error", err)
			}
		})
	}
}

func stringPtr(value string) *string { return &value }

func TestGoogleDisplayName(t *testing.T) {
	tests := []struct {
		name, input, email, want string
	}{
		{name: "normal name is trimmed", input: "  Nimal Perera ", email: "n@example.com", want: "Nimal Perera"},
		{name: "empty name uses the email name", input: "", email: "nimal.perera@example.com", want: "nimal.perera"},
		{name: "one letter name uses the email name", input: "N", email: "nimal@example.com", want: "nimal"},
		{name: "nothing usable", input: "", email: "a@example.com", want: "Google User"},
		{name: "long name is cut to 150", input: strings.Repeat("é", 200), email: "n@example.com", want: strings.Repeat("é", 150)},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := googleDisplayName(test.input, test.email); got != test.want {
				t.Fatalf("googleDisplayName() = %q, want %q", got, test.want)
			}
		})
	}
}
