package service

import (
	"context"
	"errors"
	"net/netip"
	"strings"
	"testing"
	"time"

	"golang.org/x/crypto/bcrypt"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

const testSecret = "unit-test-secret-that-is-long-enough-0123456789"

var errStore = errors.New("store unavailable")

// memoryStore is an in-memory Store. It mirrors the repository's observable
// rules; the repository itself is covered by PostgreSQL integration tests.
type memoryStore struct {
	users     map[string]*repository.User
	tokens    map[string]*storedToken
	nextID    int
	failOnOps map[string]bool

	// changePasswordErr, when set, is returned by ChangePassword in place of
	// the real outcome, to stand in for a concurrent change.
	changePasswordErr error

	// lastIssued records the metadata stored with the newest refresh token.
	lastAgent string
	lastIP    *netip.Addr
}

type storedToken struct {
	userID    string
	expiresAt time.Time
	revoked   bool

	// revokedAt and replacedBy are set when the token is rotated; a token that
	// is revoked without a successor (logout) has no replacedBy.
	revokedAt  time.Time
	replacedBy string
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		users:     map[string]*repository.User{},
		tokens:    map[string]*storedToken{},
		failOnOps: map[string]bool{},
	}
}

func (m *memoryStore) fail(op string) error {
	if m.failOnOps[op] {
		return errStore
	}
	return nil
}

func (m *memoryStore) CreateUser(_ context.Context, params repository.CreateUserParams) (repository.User, error) {
	if err := m.fail("CreateUser"); err != nil {
		return repository.User{}, err
	}
	for _, existing := range m.users {
		if strings.EqualFold(existing.Email, params.Email) {
			return repository.User{}, repository.ErrEmailTaken
		}
	}
	m.nextID++
	hash, phone := params.PasswordHash, params.Phone
	user := &repository.User{
		ID:           "user-" + string(rune('0'+m.nextID)),
		Email:        params.Email,
		PasswordHash: &hash,
		FullName:     params.FullName,
		Phone:        &phone,
		Role:         params.Role,
		AuthProvider: "LOCAL",
		IsActive:     true,
		CreatedAt:    time.Date(2026, time.November, 2, 9, 0, 0, 0, time.UTC),
	}
	m.users[user.ID] = user
	return *user, nil
}

func (m *memoryStore) UserByEmail(_ context.Context, email string) (repository.User, error) {
	if err := m.fail("UserByEmail"); err != nil {
		return repository.User{}, err
	}
	for _, user := range m.users {
		if strings.EqualFold(user.Email, email) {
			return *user, nil
		}
	}
	return repository.User{}, repository.ErrNotFound
}

func (m *memoryStore) UserByID(_ context.Context, userID string) (repository.User, error) {
	if err := m.fail("UserByID"); err != nil {
		return repository.User{}, err
	}
	user, ok := m.users[userID]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	return *user, nil
}

func (m *memoryStore) RecordSuccessfulLogin(_ context.Context, userID string, _ time.Time) error {
	if err := m.fail("RecordSuccessfulLogin"); err != nil {
		return err
	}
	user, ok := m.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	user.FailedLoginCount, user.LockedUntil = 0, nil
	return nil
}

func (m *memoryStore) RecordFailedLogin(
	_ context.Context,
	userID string,
	policy lockout.Policy,
	now time.Time,
) (lockout.State, error) {
	if err := m.fail("RecordFailedLogin"); err != nil {
		return lockout.State{}, err
	}
	user, ok := m.users[userID]
	if !ok {
		return lockout.State{}, repository.ErrNotFound
	}
	state := policy.AfterFailure(user.LockoutState(), now)
	user.FailedLoginCount, user.LockedUntil = state.FailedCount, state.LockedUntil
	return state, nil
}

func (m *memoryStore) CreateRefreshToken(_ context.Context, token repository.NewRefreshToken) (string, error) {
	if err := m.fail("CreateRefreshToken"); err != nil {
		return "", err
	}
	m.tokens[token.TokenHash] = &storedToken{userID: token.UserID, expiresAt: token.ExpiresAt}
	m.lastAgent, m.lastIP = token.UserAgent, token.IPAddress
	return token.TokenHash, nil
}

func (m *memoryStore) RotateRefreshToken(
	_ context.Context,
	oldHash string,
	next repository.NextRefreshToken,
	now time.Time,
) (string, error) {
	if err := m.fail("RotateRefreshToken"); err != nil {
		return "", err
	}
	old, ok := m.tokens[oldHash]
	if !ok {
		return "", repository.ErrRefreshTokenInvalid
	}
	if old.revoked && old.replacedBy != "" && !now.Before(old.revokedAt.Add(repository.RefreshReuseGrace)) {
		for hash := old.replacedBy; hash != ""; hash = m.tokens[hash].replacedBy {
			if token := m.tokens[hash]; !token.revoked {
				token.revoked = true
				token.revokedAt = now
			}
		}
		return old.userID, repository.ErrRefreshTokenReused
	}
	if old.revoked || !now.Before(old.expiresAt) || !m.users[old.userID].IsActive {
		return "", repository.ErrRefreshTokenInvalid
	}
	old.revoked = true
	old.revokedAt = now
	old.replacedBy = next.TokenHash
	m.tokens[next.TokenHash] = &storedToken{userID: old.userID, expiresAt: next.ExpiresAt}
	return old.userID, nil
}

func (m *memoryStore) RevokeRefreshToken(_ context.Context, userID, tokenHash string, _ time.Time) error {
	if err := m.fail("RevokeRefreshToken"); err != nil {
		return err
	}
	if token, ok := m.tokens[tokenHash]; ok && token.userID == userID {
		token.revoked = true
	}
	return nil
}

func (m *memoryStore) UpdateProfile(_ context.Context, userID string, change repository.ProfileChange) (repository.User, error) {
	if err := m.fail("UpdateProfile"); err != nil {
		return repository.User{}, err
	}
	user, ok := m.users[userID]
	if !ok {
		return repository.User{}, repository.ErrNotFound
	}
	if change.FullName != nil {
		user.FullName = *change.FullName
	}
	if change.Phone != nil {
		phone := *change.Phone
		user.Phone = &phone
	}
	return *user, nil
}

func (m *memoryStore) ChangePassword(_ context.Context, userID, expectedHash, newHash string, _ time.Time) error {
	if err := m.fail("ChangePassword"); err != nil {
		return err
	}
	if m.changePasswordErr != nil {
		return m.changePasswordErr
	}
	user, ok := m.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	if user.PasswordHash == nil || *user.PasswordHash != expectedHash {
		return repository.ErrPasswordMismatch
	}
	user.PasswordHash = &newHash
	user.FailedLoginCount, user.LockedUntil = 0, nil
	for _, token := range m.tokens {
		if token.userID == userID {
			token.revoked = true
		}
	}
	return nil
}

// spyHasher counts decoy comparisons.
type spyHasher struct {
	*password.Hasher
	burns int
}

func (s *spyHasher) Burn(plain string) {
	s.burns++
	s.Hasher.Burn(plain)
}

type fixture struct {
	service *Service
	store   *memoryStore
	hasher  *spyHasher
	tokens  *auth.TokenManager
	now     time.Time
	// notes records the account emails the service asked for.
	notes *recordingNotifier
}

func (f *fixture) advance(d time.Duration) { f.now = f.now.Add(d) }

func newFixture(t *testing.T) *fixture {
	t.Helper()

	f := &fixture{
		store: newMemoryStore(),
		now:   time.Date(2026, time.November, 2, 9, 30, 0, 0, time.UTC),
		notes: &recordingNotifier{},
	}
	clock := func() time.Time { return f.now }

	hasher, err := password.NewHasher(bcrypt.MinCost)
	if err != nil {
		t.Fatalf("NewHasher returned error: %v", err)
	}
	f.hasher = &spyHasher{Hasher: hasher}

	f.tokens, err = auth.NewTokenManager(testSecret, 15*time.Minute, auth.WithClock(clock))
	if err != nil {
		t.Fatalf("NewTokenManager returned error: %v", err)
	}

	f.service, err = New(f.store, f.hasher, f.tokens, Config{
		RefreshTTL: 7 * 24 * time.Hour,
		Lockout:    lockout.Policy{MaxFailures: 5, Duration: 15 * time.Minute},
	}, WithClock(clock), WithNotifier(f.notes))
	if err != nil {
		t.Fatalf("New returned error: %v", err)
	}
	return f
}

func validRegistration() RegisterInput {
	return RegisterInput{
		Email:    "nimal@example.com",
		Password: "Str0ngPass!",
		FullName: "Nimal Perera",
		Phone:    "+94771234567",
	}
}

func (f *fixture) register(t *testing.T) Account {
	t.Helper()
	account, err := f.service.Register(context.Background(), validRegistration())
	if err != nil {
		t.Fatalf("Register returned error: %v", err)
	}
	return account
}

func (f *fixture) login(password string) (Session, error) {
	return f.service.Login(context.Background(), LoginInput{Email: "nimal@example.com", Password: password})
}

func TestNew(t *testing.T) {
	hasher, _ := password.NewHasher(bcrypt.MinCost)
	tokens, _ := auth.NewTokenManager(testSecret, time.Minute)
	valid := Config{RefreshTTL: time.Hour, Lockout: lockout.Policy{MaxFailures: 5, Duration: time.Minute}}

	tests := []struct {
		name   string
		store  Store
		hasher Hasher
		tokens TokenIssuer
		config Config
	}{
		{name: "missing store", hasher: hasher, tokens: tokens, config: valid},
		{name: "missing hasher", store: newMemoryStore(), tokens: tokens, config: valid},
		{name: "missing token issuer", store: newMemoryStore(), hasher: hasher, config: valid},
		{name: "no refresh lifetime", store: newMemoryStore(), hasher: hasher, tokens: tokens, config: Config{Lockout: valid.Lockout}},
		{name: "no failure limit", store: newMemoryStore(), hasher: hasher, tokens: tokens, config: Config{RefreshTTL: time.Hour, Lockout: lockout.Policy{Duration: time.Minute}}},
		{name: "no lock duration", store: newMemoryStore(), hasher: hasher, tokens: tokens, config: Config{RefreshTTL: time.Hour, Lockout: lockout.Policy{MaxFailures: 5}}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if _, err := New(test.store, test.hasher, test.tokens, test.config); err == nil {
				t.Fatal("New returned no error")
			}
		})
	}
}

func TestService_Register(t *testing.T) {
	t.Run("creates a customer and stores only a hash", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		if account.Role != auth.RoleCustomer || account.Email != "nimal@example.com" || account.Phone != "+94771234567" {
			t.Fatalf("account = %+v", account)
		}
		stored := f.store.users[account.ID]
		if stored.PasswordHash == nil || *stored.PasswordHash == "Str0ngPass!" || !strings.HasPrefix(*stored.PasswordHash, "$2") {
			t.Fatal("the password was not stored as a bcrypt hash")
		}
	})

	t.Run("normalizes the email and trims the name", func(t *testing.T) {
		f := newFixture(t)
		input := validRegistration()
		input.Email = "  Nimal@Example.COM "
		input.FullName = "  Nimal Perera  "

		account, err := f.service.Register(context.Background(), input)
		if err != nil {
			t.Fatalf("Register returned error: %v", err)
		}
		if account.Email != "nimal@example.com" || account.FullName != "Nimal Perera" {
			t.Fatalf("account = %+v", account)
		}
	})

	t.Run("a duplicate email is refused in any letter case", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		input := validRegistration()
		input.Email = "NIMAL@example.com"
		if _, err := f.service.Register(context.Background(), input); !errors.Is(err, ErrEmailAlreadyExists) {
			t.Fatalf("Register error = %v, want ErrEmailAlreadyExists", err)
		}
	})

	t.Run("a store failure is not reported as a duplicate", func(t *testing.T) {
		f := newFixture(t)
		f.store.failOnOps["CreateUser"] = true

		_, err := f.service.Register(context.Background(), validRegistration())
		if !errors.Is(err, errStore) || errors.Is(err, ErrEmailAlreadyExists) {
			t.Fatalf("Register error = %v, want the store error", err)
		}
	})
}

func TestService_Register_validation(t *testing.T) {
	tests := []struct {
		name       string
		change     func(*RegisterInput)
		wantFields []string
	}{
		{name: "missing email", change: func(i *RegisterInput) { i.Email = "" }, wantFields: []string{"email"}},
		{name: "email without domain dot", change: func(i *RegisterInput) { i.Email = "nimal@localhost" }, wantFields: []string{"email"}},
		{name: "email with display name", change: func(i *RegisterInput) { i.Email = "Nimal <nimal@example.com>" }, wantFields: []string{"email"}},
		{name: "email too long", change: func(i *RegisterInput) { i.Email = strings.Repeat("a", 250) + "@example.com" }, wantFields: []string{"email"}},
		{name: "weak password", change: func(i *RegisterInput) { i.Password = "password" }, wantFields: []string{"password"}},
		{name: "short name", change: func(i *RegisterInput) { i.FullName = "N" }, wantFields: []string{"fullName"}},
		{name: "blank name", change: func(i *RegisterInput) { i.FullName = "   " }, wantFields: []string{"fullName"}},
		{name: "name too long", change: func(i *RegisterInput) { i.FullName = strings.Repeat("a", 151) }, wantFields: []string{"fullName"}},
		{name: "phone without plus", change: func(i *RegisterInput) { i.Phone = "0771234567" }, wantFields: []string{"phone"}},
		{name: "phone too long", change: func(i *RegisterInput) { i.Phone = "+9477123456789012" }, wantFields: []string{"phone"}},
		{name: "missing phone", change: func(i *RegisterInput) { i.Phone = "" }, wantFields: []string{"phone"}},
		{
			name: "every field wrong reports every field",
			change: func(i *RegisterInput) {
				*i = RegisterInput{Email: "nope", Password: "x", FullName: "", Phone: "1"}
			},
			wantFields: []string{"email", "password", "fullName", "phone"},
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newFixture(t)
			input := validRegistration()
			test.change(&input)

			_, err := f.service.Register(context.Background(), input)

			var validation *ValidationError
			if !errors.As(err, &validation) {
				t.Fatalf("Register error = %v, want *ValidationError", err)
			}
			var fields []string
			for _, issue := range validation.Issues {
				fields = append(fields, issue.Field)
				if issue.Issue == "" {
					t.Fatalf("issue for %q has no text", issue.Field)
				}
				// Short submitted values such as "x" occur inside ordinary words, so
				// only a realistic password is checked for leakage.
				if len(input.Password) >= 8 && strings.Contains(issue.Issue, input.Password) {
					t.Fatalf("issue %+v leaks the submitted password", issue)
				}
			}
			if strings.Join(fields, ",") != strings.Join(test.wantFields, ",") {
				t.Fatalf("invalid fields = %v, want %v", fields, test.wantFields)
			}
			if len(f.store.users) != 0 {
				t.Fatal("an invalid registration created a user")
			}
		})
	}
}

func TestService_Login(t *testing.T) {
	t.Run("returns a verifiable access token and a stored, hashed refresh token", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		session, err := f.login("Str0ngPass!")
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}

		claims, err := f.tokens.Verify(session.AccessToken)
		if err != nil {
			t.Fatalf("access token does not verify: %v", err)
		}
		if claims.UserID != account.ID || claims.Role != auth.RoleCustomer {
			t.Fatalf("claims = %+v", claims)
		}
		if session.ExpiresIn != 900 || !session.AccessExpiresAt.Equal(f.now.Add(15*time.Minute)) {
			t.Fatalf("access lifetime = %ds until %v", session.ExpiresIn, session.AccessExpiresAt)
		}
		if session.Account.ID != account.ID {
			t.Fatalf("session account = %+v", session.Account)
		}

		if !strings.HasPrefix(session.RefreshToken, "rt_") || !session.RefreshExpiresAt.Equal(f.now.Add(7*24*time.Hour)) {
			t.Fatalf("refresh token %q expires %v", session.RefreshToken, session.RefreshExpiresAt)
		}
		if _, raw := f.store.tokens[session.RefreshToken]; raw {
			t.Fatal("the raw refresh token was stored")
		}
		if _, hashed := f.store.tokens[hashRefreshToken(session.RefreshToken)]; !hashed {
			t.Fatal("the refresh token hash was not stored")
		}
	})

	t.Run("matches the email in any letter case", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		_, err := f.service.Login(context.Background(), LoginInput{Email: " NIMAL@Example.com ", Password: "Str0ngPass!"})
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
	})

	t.Run("records the client address and agent with the session", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		_, err := f.service.Login(context.Background(), LoginInput{
			Email:     "nimal@example.com",
			Password:  "Str0ngPass!",
			UserAgent: "test-agent",
			IP:        netip.MustParseAddr("203.0.113.9"),
		})
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
		if f.store.lastAgent != "test-agent" || f.store.lastIP == nil || f.store.lastIP.String() != "203.0.113.9" {
			t.Fatalf("stored agent %q and address %v", f.store.lastAgent, f.store.lastIP)
		}
	})

	t.Run("a wrong password and an unknown email give the same error", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		_, wrong := f.login("Wr0ngPass!")
		_, unknown := f.service.Login(context.Background(), LoginInput{Email: "nobody@example.com", Password: "Str0ngPass!"})

		if !errors.Is(wrong, ErrInvalidCredentials) || !errors.Is(unknown, ErrInvalidCredentials) {
			t.Fatalf("errors = %v / %v, want ErrInvalidCredentials for both", wrong, unknown)
		}
		if wrong.Error() != unknown.Error() {
			t.Fatalf("error texts differ: %q vs %q", wrong, unknown)
		}
	})

	t.Run("an unknown email still spends the time of a password check", func(t *testing.T) {
		f := newFixture(t)

		_, _ = f.service.Login(context.Background(), LoginInput{Email: "nobody@example.com", Password: "Str0ngPass!"})
		if f.hasher.burns != 1 {
			t.Fatalf("decoy comparisons = %d, want 1", f.hasher.burns)
		}
	})

	t.Run("a wrong password counts as a failure", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		_, _ = f.login("Wr0ngPass!")
		if got := f.store.users[account.ID].FailedLoginCount; got != 1 {
			t.Fatalf("failed login count = %d, want 1", got)
		}
	})

	t.Run("a Google-only account cannot log in with a password and is not locked by guessing", func(t *testing.T) {
		f := newFixture(t)
		f.store.users["google-1"] = &repository.User{
			ID: "google-1", Email: "gina@example.com", FullName: "Gina Google", Role: auth.RoleCustomer,
			AuthProvider: "GOOGLE", IsActive: true,
		}

		for range 8 {
			_, err := f.service.Login(context.Background(), LoginInput{Email: "gina@example.com", Password: "Str0ngPass!"})
			if !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
			}
		}
		if user := f.store.users["google-1"]; user.FailedLoginCount != 0 || user.LockedUntil != nil {
			t.Fatalf("guessing changed the lockout state: %+v", user.LockoutState())
		}
	})

	t.Run("a store failure is not reported as bad credentials", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)
		f.store.failOnOps["UserByEmail"] = true

		if _, err := f.login("Str0ngPass!"); !errors.Is(err, errStore) || errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want the store error", err)
		}
	})

	t.Run("a failure to store the refresh token fails the login", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)
		f.store.failOnOps["CreateRefreshToken"] = true

		if _, err := f.login("Str0ngPass!"); !errors.Is(err, errStore) {
			t.Fatalf("Login error = %v, want the store error", err)
		}
	})
}

func TestService_Login_disabledAccount(t *testing.T) {
	f := newFixture(t)
	account := f.register(t)
	f.store.users[account.ID].IsActive = false

	t.Run("the right password reports the account as disabled", func(t *testing.T) {
		if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrAccountDisabled) {
			t.Fatalf("Login error = %v, want ErrAccountDisabled", err)
		}
	})

	t.Run("a wrong password does not reveal that the account is disabled", func(t *testing.T) {
		if _, err := f.login("Wr0ngPass!"); !errors.Is(err, ErrInvalidCredentials) {
			t.Fatalf("Login error = %v, want ErrInvalidCredentials", err)
		}
	})
}

func TestService_Login_lockout(t *testing.T) {
	t.Run("five failures lock the account, even for the right password", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		for attempt := 1; attempt <= 5; attempt++ {
			if _, err := f.login("Wr0ngPass!"); !errors.Is(err, ErrInvalidCredentials) {
				t.Fatalf("attempt %d error = %v, want ErrInvalidCredentials", attempt, err)
			}
		}
		if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrAccountLocked) {
			t.Fatalf("Login error = %v, want ErrAccountLocked", err)
		}
		if _, err := f.login("Wr0ngPass!"); !errors.Is(err, ErrAccountLocked) {
			t.Fatalf("Login error = %v, want ErrAccountLocked", err)
		}
	})

	t.Run("four failures do not lock", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		for range 4 {
			_, _ = f.login("Wr0ngPass!")
		}
		if _, err := f.login("Str0ngPass!"); err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
	})

	t.Run("a success resets the counter", func(t *testing.T) {
		f := newFixture(t)
		account := f.register(t)

		for range 4 {
			_, _ = f.login("Wr0ngPass!")
		}
		if _, err := f.login("Str0ngPass!"); err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
		if got := f.store.users[account.ID].FailedLoginCount; got != 0 {
			t.Fatalf("failed login count = %d, want 0", got)
		}
	})

	t.Run("the lock ends after 15 minutes", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)

		for range 5 {
			_, _ = f.login("Wr0ngPass!")
		}
		f.advance(14 * time.Minute)
		if _, err := f.login("Str0ngPass!"); !errors.Is(err, ErrAccountLocked) {
			t.Fatalf("Login after 14 minutes error = %v, want ErrAccountLocked", err)
		}

		f.advance(2 * time.Minute)
		if _, err := f.login("Str0ngPass!"); err != nil {
			t.Fatalf("Login after 16 minutes returned error: %v", err)
		}
	})

	t.Run("a store failure while counting fails the login", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)
		f.store.failOnOps["RecordFailedLogin"] = true

		if _, err := f.login("Wr0ngPass!"); !errors.Is(err, errStore) {
			t.Fatalf("Login error = %v, want the store error", err)
		}
	})
}

func TestService_Refresh(t *testing.T) {
	login := func(t *testing.T, f *fixture) Session {
		t.Helper()
		f.register(t)
		session, err := f.login("Str0ngPass!")
		if err != nil {
			t.Fatalf("Login returned error: %v", err)
		}
		return session
	}

	t.Run("rotates the refresh token and issues a new access token", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)
		f.advance(5 * time.Minute)

		second, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if err != nil {
			t.Fatalf("Refresh returned error: %v", err)
		}
		if second.RefreshToken == first.RefreshToken || second.AccessToken == first.AccessToken {
			t.Fatal("refresh did not issue new tokens")
		}
		claims, err := f.tokens.Verify(second.AccessToken)
		if err != nil || claims.UserID != first.Account.ID {
			t.Fatalf("new access token claims = %+v, error = %v", claims, err)
		}
		if !second.RefreshExpiresAt.Equal(f.now.Add(7 * 24 * time.Hour)) {
			t.Fatalf("new refresh token expires %v", second.RefreshExpiresAt)
		}
	})

	t.Run("a used refresh token cannot be used again", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)

		if _, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken}); err != nil {
			t.Fatalf("first Refresh returned error: %v", err)
		}
		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("second Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("the rotated token keeps working", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)
		second, _ := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})

		if _, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: second.RefreshToken}); err != nil {
			t.Fatalf("Refresh with the rotated token returned error: %v", err)
		}
	})

	t.Run("replaying a rotated token later ends the session it belonged to", func(t *testing.T) {
		f := newFixture(t)
		var logs strings.Builder
		f.service.logger = newCapturingLogger(&logs)
		first := login(t, f)

		second, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if err != nil {
			t.Fatalf("first Refresh returned error: %v", err)
		}
		third, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: second.RefreshToken})
		if err != nil {
			t.Fatalf("second Refresh returned error: %v", err)
		}

		f.advance(repository.RefreshReuseGrace + time.Second)
		_, err = f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("replayed Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
		if errors.Is(err, repository.ErrRefreshTokenReused) {
			t.Fatal("the caller can tell a replayed token from an unknown one")
		}

		_, err = f.service.Refresh(context.Background(), RefreshInput{RefreshToken: third.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("the newest token still works after a replay: error = %v", err)
		}
		text := logs.String()
		if !strings.Contains(text, "refresh token reuse detected") || !strings.Contains(text, first.Account.ID) {
			t.Fatalf("reuse was not logged with the user ID: %q", text)
		}
		if strings.Contains(text, first.RefreshToken) || strings.Contains(text, third.RefreshToken) {
			t.Fatalf("log leaks a refresh token: %q", text)
		}
	})

	t.Run("a quick retry of the old token is refused but does not end the session", func(t *testing.T) {
		f := newFixture(t)
		var logs strings.Builder
		f.service.logger = newCapturingLogger(&logs)
		first := login(t, f)

		second, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if err != nil {
			t.Fatalf("first Refresh returned error: %v", err)
		}
		f.advance(repository.RefreshReuseGrace - time.Second)
		_, err = f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("retried Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}

		if _, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: second.RefreshToken}); err != nil {
			t.Fatalf("the newest token stopped working after a quick retry: %v", err)
		}
		if strings.Contains(logs.String(), "reuse detected") {
			t.Fatalf("a quick retry was logged as reuse: %q", logs.String())
		}
	})

	t.Run("a logged out token is refused without reuse handling", func(t *testing.T) {
		f := newFixture(t)
		var logs strings.Builder
		f.service.logger = newCapturingLogger(&logs)
		first := login(t, f)

		if err := f.service.Logout(context.Background(), first.Account.ID, first.RefreshToken); err != nil {
			t.Fatalf("Logout returned error: %v", err)
		}
		f.advance(time.Hour)
		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
		if strings.Contains(logs.String(), "reuse detected") {
			t.Fatalf("logout was logged as reuse: %q", logs.String())
		}
	})

	t.Run("an expired token is refused", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)
		f.advance(7*24*time.Hour + time.Second)

		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("a deactivated user's token is refused", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)
		f.store.users[first.Account.ID].IsActive = false

		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("malformed and unknown tokens are refused without touching the store", func(t *testing.T) {
		f := newFixture(t)
		f.store.failOnOps["RotateRefreshToken"] = true

		for _, token := range []string{"", "not-a-token", "rt_" + strings.Repeat("a", maxRefreshTokenLength)} {
			_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: token})
			if !errors.Is(err, ErrRefreshTokenInvalid) {
				t.Fatalf("Refresh(%.12q) error = %v, want ErrRefreshTokenInvalid", token, err)
			}
		}
	})

	t.Run("an unknown but well formed token is refused", func(t *testing.T) {
		f := newFixture(t)
		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: "rt_unknownunknownunknown"})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("a store failure is not reported as an invalid token", func(t *testing.T) {
		f := newFixture(t)
		first := login(t, f)
		f.store.failOnOps["RotateRefreshToken"] = true

		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: first.RefreshToken})
		if !errors.Is(err, errStore) || errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh error = %v, want the store error", err)
		}
	})
}

func TestService_Logout(t *testing.T) {
	t.Run("revokes the refresh token", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)
		session, _ := f.login("Str0ngPass!")

		if err := f.service.Logout(context.Background(), session.Account.ID, session.RefreshToken); err != nil {
			t.Fatalf("Logout returned error: %v", err)
		}
		_, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: session.RefreshToken})
		if !errors.Is(err, ErrRefreshTokenInvalid) {
			t.Fatalf("Refresh after logout error = %v, want ErrRefreshTokenInvalid", err)
		}
	})

	t.Run("does not revoke another user's token", func(t *testing.T) {
		f := newFixture(t)
		f.register(t)
		session, _ := f.login("Str0ngPass!")

		if err := f.service.Logout(context.Background(), "someone-else", session.RefreshToken); err != nil {
			t.Fatalf("Logout returned error: %v", err)
		}
		if _, err := f.service.Refresh(context.Background(), RefreshInput{RefreshToken: session.RefreshToken}); err != nil {
			t.Fatalf("the token was revoked by a different user: %v", err)
		}
	})

	t.Run("succeeds without a refresh token", func(t *testing.T) {
		f := newFixture(t)
		f.store.failOnOps["RevokeRefreshToken"] = true

		for _, token := range []string{"", "garbage"} {
			if err := f.service.Logout(context.Background(), "user-1", token); err != nil {
				t.Fatalf("Logout(%q) returned error: %v", token, err)
			}
		}
	})

	t.Run("reports a store failure", func(t *testing.T) {
		f := newFixture(t)
		f.store.failOnOps["RevokeRefreshToken"] = true

		err := f.service.Logout(context.Background(), "user-1", "rt_somethingsomethingsomething")
		if !errors.Is(err, errStore) {
			t.Fatalf("Logout error = %v, want the store error", err)
		}
	})
}
