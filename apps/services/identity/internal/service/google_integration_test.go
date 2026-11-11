//go:build integration

package service_test

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/netip"
	"net/url"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/lockout"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/password"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/repository"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/postgres"
)

const (
	integrationSigningSecret    = "integration-signing-secret-0123456789abcdef"
	integrationOAuthStateTTL    = 7 * time.Minute
	integrationPasswordResetTTL = 45 * time.Minute
)

type fakeGoogleProvider struct {
	profile service.GoogleProfile
	err     error
}

func (p *fakeGoogleProvider) AuthURL(state, _ string) string {
	return "https://accounts.example/auth?state=" + state
}

func (p *fakeGoogleProvider) Exchange(context.Context, string, string) (service.GoogleProfile, error) {
	return p.profile, p.err
}

type captureMailer struct {
	messages []mailer.Message
}

func (m *captureMailer) Send(_ context.Context, message mailer.Message) error {
	m.messages = append(m.messages, message)
	return nil
}

// approvedMerchants answers that every merchant owner is approved. A merchant
// signs in only with a current approval, and these tests are about linking.
type approvedMerchants struct{}

func (approvedMerchants) Approval(context.Context, string) (string, string, error) {
	return "APPROVED", "", nil
}

type googleFlow struct {
	google   *service.GoogleAuth
	base     *service.Service
	repo     *repository.Repository
	pool     *postgres.Pool
	provider *fakeGoogleProvider
}

func newGoogleFlow(t *testing.T) *googleFlow {
	t.Helper()
	pool := openIsolatedDB(t)
	repo, err := repository.New(pool)
	if err != nil {
		t.Fatalf("repository.New: %v", err)
	}
	hasher, err := password.NewHasher(password.DefaultCost)
	if err != nil {
		t.Fatalf("password.NewHasher: %v", err)
	}
	tokens, err := auth.NewTokenManager(integrationSigningSecret, 15*time.Minute)
	if err != nil {
		t.Fatalf("auth.NewTokenManager: %v", err)
	}
	base, err := service.New(repo, hasher, tokens, service.Config{
		RefreshTTL: 7 * 24 * time.Hour,
		Lockout:    lockout.Policy{MaxFailures: 5, Duration: 15 * time.Minute},
	}, service.WithMerchantApproval(approvedMerchants{}))
	if err != nil {
		t.Fatalf("service.New: %v", err)
	}
	provider := &fakeGoogleProvider{profile: service.GoogleProfile{
		Subject: "google-sub-1", Email: "User@Example.com", EmailVerified: true, FullName: "Integration User",
	}}
	google, err := service.NewGoogleAuth(base, repo, provider, integrationOAuthStateTTL)
	if err != nil {
		t.Fatalf("service.NewGoogleAuth: %v", err)
	}
	return &googleFlow{google: google, base: base, repo: repo, pool: pool, provider: provider}
}

func (f *googleFlow) start(t *testing.T) service.GoogleLoginStart {
	t.Helper()
	start, err := f.google.StartGoogleLogin(context.Background(), "")
	if err != nil {
		t.Fatalf("StartGoogleLogin: %v", err)
	}
	return start
}

func stateFromAuthURL(t *testing.T, raw string) string {
	t.Helper()
	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse auth URL: %v", err)
	}
	state := parsed.Query().Get("state")
	if state == "" {
		t.Fatalf("auth URL has no state: %q", raw)
	}
	return state
}

func (f *googleFlow) complete(t *testing.T, start service.GoogleLoginStart) (service.GoogleLoginResult, error) {
	t.Helper()
	return f.google.CompleteGoogleLogin(context.Background(), service.GoogleCallbackInput{
		Code:           "authorization-code",
		State:          stateFromAuthURL(t, start.AuthURL),
		BrowserBinding: start.BindingValue,
		UserAgent:      "integration-agent",
		IP:             netip.MustParseAddr("203.0.113.5"),
	})
}

func TestGoogleFlow_createsCustomer(t *testing.T) {
	f := newGoogleFlow(t)
	start := f.start(t)

	result, err := f.complete(t, start)
	if err != nil {
		t.Fatalf("CompleteGoogleLogin: %v", err)
	}
	if result.Session.Account.Role != auth.RoleCustomer || result.Session.Account.Email != "user@example.com" {
		t.Fatalf("account = %+v", result.Session.Account)
	}

	var (
		provider      string
		hasPassword   bool
		emailVerified bool
		sub           string
		role          string
		count         int
	)
	if err := f.pool.QueryRow(context.Background(), `
		SELECT u.auth_provider, u.password_hash IS NOT NULL, u.email_verified, u.google_sub, r.name,
		       (SELECT count(*) FROM identity.users)
		FROM identity.users u JOIN identity.roles r ON r.id = u.role_id
		WHERE u.id = $1`, result.Session.Account.ID,
	).Scan(&provider, &hasPassword, &emailVerified, &sub, &role, &count); err != nil {
		t.Fatalf("read created user: %v", err)
	}
	if provider != "GOOGLE" || hasPassword || !emailVerified || sub != "google-sub-1" || role != "CUSTOMER" || count != 1 {
		t.Fatalf("created user: provider=%s hasPassword=%t verified=%t sub=%s role=%s count=%d",
			provider, hasPassword, emailVerified, sub, role, count)
	}
}

func TestGoogleFlow_persistsConfiguredOAuthStateTTL(t *testing.T) {
	f := newGoogleFlow(t)
	before := time.Now().UTC()
	start := f.start(t)
	state := stateFromAuthURL(t, start.AuthURL)

	var expires time.Time
	if err := f.pool.QueryRow(context.Background(),
		`SELECT expires_at FROM identity.oauth_states WHERE state = $1`, state,
	).Scan(&expires); err != nil {
		t.Fatalf("read oauth state: %v", err)
	}
	want := before.Add(integrationOAuthStateTTL)
	if expires.Before(want.Add(-10*time.Second)) || expires.After(want.Add(10*time.Second)) {
		t.Fatalf("oauth_states.expires_at = %v, want about %v", expires, want)
	}
	if start.BindingMaxAge != int(integrationOAuthStateTTL/time.Second) {
		t.Fatalf("binding MaxAge = %d, want %d", start.BindingMaxAge, int(integrationOAuthStateTTL/time.Second))
	}
}

func TestPasswordResetFlow_persistsConfiguredTTL(t *testing.T) {
	f := newGoogleFlow(t)
	hasher, _ := password.NewHasher(password.DefaultCost)
	hash, _ := hasher.Hash("OldPass1!")
	if _, err := f.repo.CreateUser(context.Background(), repository.CreateUserParams{
		Email: "reset@example.com", PasswordHash: hash, FullName: "Reset User",
		Phone: "+94771234567", Role: auth.RoleCustomer,
	}); err != nil {
		t.Fatalf("CreateUser: %v", err)
	}

	sender := &captureMailer{}
	reset, err := service.NewPasswordReset(f.base, f.repo, sender, "https://swiftdrop.example", integrationPasswordResetTTL, service.WithSynchronousEmail())
	if err != nil {
		t.Fatalf("NewPasswordReset: %v", err)
	}
	before := time.Now().UTC()
	if err := reset.ForgotPassword(context.Background(), service.ForgotPasswordInput{
		Email: "reset@example.com", IP: netip.MustParseAddr("203.0.113.5"),
	}); err != nil {
		t.Fatalf("ForgotPassword: %v", err)
	}
	if len(sender.messages) != 1 {
		t.Fatalf("%d reset emails sent, want 1", len(sender.messages))
	}

	var expires time.Time
	if err := f.pool.QueryRow(context.Background(), `
		SELECT t.expires_at
		FROM identity.password_reset_tokens t
		JOIN identity.users u ON u.id = t.user_id
		WHERE u.email = $1 AND t.used_at IS NULL`, "reset@example.com",
	).Scan(&expires); err != nil {
		t.Fatalf("read reset token: %v", err)
	}
	want := before.Add(integrationPasswordResetTTL)
	if expires.Before(want.Add(-10*time.Second)) || expires.After(want.Add(10*time.Second)) {
		t.Fatalf("password_reset_tokens.expires_at = %v, want about %v", expires, want)
	}
}

func TestGoogleFlow_existingGoogleIdentity(t *testing.T) {
	f := newGoogleFlow(t)
	existing, err := f.repo.CreateGoogleUser(context.Background(), repository.CreateGoogleUserParams{
		Email: "user@example.com", FullName: "Existing Google", GoogleSub: "google-sub-1", Role: auth.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("CreateGoogleUser: %v", err)
	}

	result, err := f.complete(t, f.start(t))
	if err != nil {
		t.Fatalf("CompleteGoogleLogin: %v", err)
	}
	if result.Session.Account.ID != existing.ID {
		t.Fatalf("signed in as %s, want %s", result.Session.Account.ID, existing.ID)
	}
	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("%d accounts exist, want 1", count)
	}
}

func TestGoogleFlow_linksUnverifiedEmailAndClearsPassword(t *testing.T) {
	f := newGoogleFlow(t)
	hasher, _ := password.NewHasher(password.DefaultCost)
	oldHash, err := hasher.Hash("OldPass1!")
	if err != nil {
		t.Fatalf("hash old password: %v", err)
	}
	local, err := f.repo.CreateUser(context.Background(), repository.CreateUserParams{
		Email: "user@example.com", PasswordHash: oldHash, FullName: "Local User",
		Phone: "+94771234567", Role: auth.RoleMerchant,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := f.repo.CreateRefreshToken(context.Background(), repository.NewRefreshToken{
		UserID: local.ID, TokenHash: sha256Hex("rt_before_link"), ExpiresAt: time.Now().Add(time.Hour),
	}); err != nil {
		t.Fatalf("CreateRefreshToken: %v", err)
	}

	result, err := f.complete(t, f.start(t))
	if err != nil {
		t.Fatalf("CompleteGoogleLogin: %v", err)
	}
	if result.Session.Account.ID != local.ID || result.Session.Account.Role != auth.RoleMerchant {
		t.Fatalf("account = %+v, want the existing user with its role", result.Session.Account)
	}

	var (
		hasPassword   bool
		emailVerified bool
		sub           string
		tokensRevoked bool
	)
	if err := f.pool.QueryRow(context.Background(), `
		SELECT u.password_hash IS NOT NULL, u.email_verified, u.google_sub,
		       EXISTS (
		           SELECT 1 FROM identity.refresh_tokens t
		           WHERE t.user_id = u.id
		             AND t.token_hash = $2
		             AND t.revoked_at IS NOT NULL
		       )
		FROM identity.users u WHERE u.id = $1`, local.ID, sha256Hex("rt_before_link"),
	).Scan(&hasPassword, &emailVerified, &sub, &tokensRevoked); err != nil {
		t.Fatalf("read linked user: %v", err)
	}
	if hasPassword || !emailVerified || sub != "google-sub-1" || !tokensRevoked {
		t.Fatalf("linked user: hasPassword=%t verified=%t sub=%s tokensRevoked=%t",
			hasPassword, emailVerified, sub, tokensRevoked)
	}

	// The attacker's old password can no longer be used.
	if _, err := f.base.Login(context.Background(), service.LoginInput{
		Email: "user@example.com", Password: "OldPass1!",
	}); !errors.Is(err, service.ErrInvalidCredentials) {
		t.Fatalf("old password login error = %v, want ErrInvalidCredentials", err)
	}
}

func TestGoogleFlow_passwordResetRestoresPasswordAfterLinking(t *testing.T) {
	f := newGoogleFlow(t)
	hasher, _ := password.NewHasher(password.DefaultCost)
	oldHash, _ := hasher.Hash("OldPass1!")
	local, err := f.repo.CreateUser(context.Background(), repository.CreateUserParams{
		Email: "user@example.com", PasswordHash: oldHash, FullName: "Local User",
		Phone: "+94771234567", Role: auth.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := f.complete(t, f.start(t)); err != nil {
		t.Fatalf("CompleteGoogleLogin: %v", err)
	}

	now := time.Now()
	const rawToken = "reset-after-linking"
	tokenHash := sha256Hex(rawToken)
	if err := f.repo.IssuePasswordResetToken(context.Background(), repository.NewPasswordResetToken{
		UserID: local.ID, TokenHash: tokenHash, ExpiresAt: now.Add(30 * time.Minute),
	}, now); err != nil {
		t.Fatalf("IssuePasswordResetToken: %v", err)
	}
	newHash, _ := hasher.Hash("NewPass1!")
	if _, err := f.repo.ResetPasswordWithToken(context.Background(), tokenHash, newHash, now.Add(time.Minute)); err != nil {
		t.Fatalf("ResetPasswordWithToken: %v", err)
	}

	if _, err := f.base.Login(context.Background(), service.LoginInput{
		Email: "user@example.com", Password: "NewPass1!",
	}); err != nil {
		t.Fatalf("new password login failed: %v", err)
	}
}

func TestGoogleFlow_disabledAccountIsNotLinked(t *testing.T) {
	f := newGoogleFlow(t)
	hasher, _ := password.NewHasher(password.DefaultCost)
	oldHash, _ := hasher.Hash("OldPass1!")
	local, err := f.repo.CreateUser(context.Background(), repository.CreateUserParams{
		Email: "user@example.com", PasswordHash: oldHash, FullName: "Disabled User",
		Phone: "+94771234567", Role: auth.RoleCustomer,
	})
	if err != nil {
		t.Fatalf("CreateUser: %v", err)
	}
	if _, err := f.pool.Exec(context.Background(), `UPDATE identity.users SET is_active = false WHERE id = $1`, local.ID); err != nil {
		t.Fatalf("deactivate user: %v", err)
	}

	if _, err := f.complete(t, f.start(t)); !errors.Is(err, service.ErrAccountDisabled) {
		t.Fatalf("error = %v, want ErrAccountDisabled", err)
	}
	var sub *string
	if err := f.pool.QueryRow(context.Background(), `SELECT google_sub FROM identity.users WHERE id = $1`, local.ID).Scan(&sub); err != nil {
		t.Fatalf("read google_sub: %v", err)
	}
	if sub != nil {
		t.Fatalf("disabled account was linked: %v", *sub)
	}
}

func TestGoogleFlow_bindingIsRequiredAndNotBurnedOnMismatch(t *testing.T) {
	f := newGoogleFlow(t)
	start := f.start(t)
	state := stateFromAuthURL(t, start.AuthURL)

	callback := func(binding string) error {
		_, err := f.google.CompleteGoogleLogin(context.Background(), service.GoogleCallbackInput{
			Code: "c", State: state, BrowserBinding: binding,
		})
		return err
	}
	if err := callback(""); !errors.Is(err, service.ErrOAuthStateInvalid) {
		t.Fatalf("missing binding error = %v", err)
	}
	if err := callback("wrong-binding"); !errors.Is(err, service.ErrOAuthStateInvalid) {
		t.Fatalf("mismatched binding error = %v", err)
	}
	if err := callback(start.BindingValue); err != nil {
		t.Fatalf("legitimate binding failed after a mismatch: %v", err)
	}
}

func TestGoogleFlow_replayRejected(t *testing.T) {
	f := newGoogleFlow(t)
	start := f.start(t)
	if _, err := f.complete(t, start); err != nil {
		t.Fatalf("first callback: %v", err)
	}
	if _, err := f.complete(t, start); !errors.Is(err, service.ErrOAuthStateInvalid) {
		t.Fatalf("replay error = %v, want ErrOAuthStateInvalid", err)
	}
}

func TestGoogleFlow_providerFailureBurnsState(t *testing.T) {
	f := newGoogleFlow(t)
	start := f.start(t)
	f.provider.err = errors.New("google is down")
	if _, err := f.complete(t, start); !errors.Is(err, service.ErrOAuthProvider) {
		t.Fatalf("error = %v, want ErrOAuthProvider", err)
	}
	f.provider.err = nil
	if _, err := f.complete(t, start); !errors.Is(err, service.ErrOAuthStateInvalid) {
		t.Fatalf("retry error = %v, want ErrOAuthStateInvalid", err)
	}
}

func TestGoogleFlow_concurrentFirstLogin(t *testing.T) {
	f := newGoogleFlow(t)
	first, second := f.start(t), f.start(t)
	states := []string{stateFromAuthURL(t, first.AuthURL), stateFromAuthURL(t, second.AuthURL)}
	starts := []service.GoogleLoginStart{first, second}

	var wg sync.WaitGroup
	results := make(chan error, 2)
	for i := range starts {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, err := f.google.CompleteGoogleLogin(context.Background(), service.GoogleCallbackInput{
				Code: "c", State: states[i], BrowserBinding: starts[i].BindingValue,
			})
			results <- err
		}(i)
	}
	wg.Wait()
	close(results)
	for err := range results {
		if err != nil {
			t.Fatalf("concurrent login error: %v", err)
		}
	}

	var count int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM identity.users`).Scan(&count); err != nil {
		t.Fatalf("count users: %v", err)
	}
	if count != 1 {
		t.Fatalf("%d accounts exist, want 1", count)
	}
}

func TestGoogleFlow_databaseError(t *testing.T) {
	f := newGoogleFlow(t)
	f.pool.Close()
	if _, err := f.google.StartGoogleLogin(context.Background(), ""); err == nil {
		t.Fatal("StartGoogleLogin succeeded with a closed database")
	}
}

func sha256Hex(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])
}

func randomHex(size int) string {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		panic("read random bytes: " + err.Error())
	}
	return hex.EncodeToString(buffer)
}

func openIsolatedDB(t *testing.T) *postgres.Pool {
	t.Helper()
	serverURL := os.Getenv("TEST_DATABASE_URL")
	if serverURL == "" {
		t.Skip("TEST_DATABASE_URL is not set; skipping PostgreSQL integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	t.Cleanup(cancel)

	admin, err := pgx.Connect(ctx, serverURL)
	if err != nil {
		t.Fatalf("connect test server: %v", err)
	}
	name := "swiftdrop_google_flow_" + randomHex(8)
	quoted := pgx.Identifier{name}.Sanitize()
	if _, err := admin.Exec(ctx, "CREATE DATABASE "+quoted); err != nil {
		_ = admin.Close(ctx)
		t.Fatalf("create isolated database: %v", err)
	}
	t.Cleanup(func() {
		cleanupCtx, cleanupCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cleanupCancel()
		_, _ = admin.Exec(cleanupCtx, "DROP DATABASE IF EXISTS "+quoted+" WITH (FORCE)")
		_ = admin.Close(cleanupCtx)
	})

	parsed, err := url.Parse(serverURL)
	if err != nil {
		t.Fatalf("parse TEST_DATABASE_URL: %v", err)
	}
	parsed.Path = "/" + name
	cfg := postgres.DefaultConfig()
	cfg.URL = parsed.String()
	cfg.MaxConns = 8
	cfg.MinConns = 0
	cfg.RetryAttempts = 1
	pool, err := postgres.Open(ctx, cfg)
	if err != nil {
		t.Fatalf("open isolated database: %v", err)
	}
	t.Cleanup(pool.Close)

	root := integrationRepoRoot(t)
	for _, file := range []string{
		"infra/docker/postgres/init/01-schemas.sql",
		"migrations/identity/000001_initial_schema.up.sql",
		"migrations/identity/000002_tables.up.sql",
		"migrations/identity/000003_oauth_states_browser_binding.up.sql",
		"migrations/identity/000004_email_verification.up.sql",
		"migrations/identity/000005_session_version.up.sql",
		"migrations/identity/000006_merchant_lifecycle_transitions.up.sql",
	} {
		script, err := os.ReadFile(filepath.Join(root, filepath.FromSlash(file)))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		if _, err := pool.Exec(ctx, string(script)); err != nil {
			t.Fatalf("apply %s: %v", file, err)
		}
	}
	return pool
}

func integrationRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("find working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find the repository root")
		}
		dir = parent
	}
}
