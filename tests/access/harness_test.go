//go:build access

package access

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
)

// stack is the running local Compose stack the access tests talk to. It is
// created once, on first use, and its fixtures are removed after the last test.
type stack struct {
	base   string
	root   string
	secret string
	client *http.Client

	// tokens and userIDs hold one real, active account per role.
	tokens  map[auth.Role]string
	userIDs map[auth.Role]string

	mu      sync.Mutex
	created []string // every user ID this run made, for cleanup
}

var (
	setup     sync.Once
	theStack  *stack
	setupFail error
)

func TestMain(m *testing.M) {
	code := m.Run()
	if theStack != nil {
		theStack.cleanup()
	}
	os.Exit(code)
}

// useStack returns the shared stack, preparing it on first use.
func useStack(t *testing.T) *stack {
	t.Helper()
	setup.Do(func() { theStack, setupFail = newStack() })
	if setupFail != nil {
		t.Fatal(setupFail)
	}
	return theStack
}

func newStack() (*stack, error) {
	base := strings.TrimRight(os.Getenv("TEST_BASE_URL"), "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	parsed, err := url.Parse(base)
	if err != nil || parsed.Scheme != "http" {
		return nil, fmt.Errorf("TEST_BASE_URL must be the local HTTP gateway, got %q", base)
	}
	switch parsed.Hostname() {
	case "localhost", "127.0.0.1", "::1":
	default:
		return nil, fmt.Errorf("the access tests create and delete accounts, so they only run against a local gateway, not %q", parsed.Host)
	}
	root, err := filepath.Abs("../..")
	if err != nil {
		return nil, err
	}

	s := &stack{
		base:    base,
		root:    root,
		client:  &http.Client{Timeout: 20 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }},
		tokens:  map[auth.Role]string{},
		userIDs: map[auth.Role]string{},
	}
	if s.secret, err = accessSecret(root); err != nil {
		return nil, err
	}

	manager, err := auth.NewTokenManager(s.secret, 15*time.Minute)
	if err != nil {
		return nil, fmt.Errorf("JWT_ACCESS_SECRET is not usable: %w", err)
	}
	runID := randomHex(4)
	for _, role := range []auth.Role{auth.RoleCustomer, auth.RoleMerchant, auth.RoleCourier, auth.RoleDispatcher, auth.RoleAdmin} {
		email := fmt.Sprintf("access.%s.%s@example.test", runID, strings.ToLower(string(role)))
		id, err := s.register(email, "Str0ngPass!")
		if err != nil {
			return s, fmt.Errorf("register the %s fixture: %w", role, err)
		}
		if err := s.sql(fmt.Sprintf(
			`UPDATE identity.users SET role_id = (SELECT id FROM identity.roles WHERE name = '%s'), email_verified = true WHERE id = '%s';`, role, id)); err != nil {
			return s, fmt.Errorf("give the fixture the %s role: %w", role, err)
		}
		if role == auth.RoleMerchant {
			if err := s.sql(fmt.Sprintf(
				`INSERT INTO merchant.merchants (owner_user_id, business_name, business_reg_no, business_address, contact_email, contact_phone, pickup_address, pickup_zone_id, status, approved_by, approved_at)
				 VALUES ('%s', 'Access Merchant', 'ACCESS-%s', '1 Test Road', 'merchant@example.test', '+94771234567', '1 Test Road', '00000000-0000-4000-8000-000000000001', 'APPROVED', '%s', now());`, id, runID, id)); err != nil {
				return s, fmt.Errorf("create the merchant fixture: %w", err)
			}
		}
		token, _, err := manager.Issue(id, role)
		if err != nil {
			return s, fmt.Errorf("sign a %s token: %w", role, err)
		}
		s.userIDs[role], s.tokens[role] = id, token
	}
	return s, nil
}

// accessSecret reads the signing secret the way the Compose services see it.
func accessSecret(root string) (string, error) {
	if secret := os.Getenv("JWT_ACCESS_SECRET"); secret != "" {
		return secret, nil
	}
	command := exec.Command("docker", "compose", "config", "--format", "json")
	command.Dir = root
	raw, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("cannot read the Compose configuration (is Docker running and .env present?): %w", err)
	}
	var config struct {
		Services map[string]struct {
			Environment map[string]string `json:"environment"`
		} `json:"services"`
	}
	if err := json.Unmarshal(raw, &config); err != nil {
		return "", fmt.Errorf("cannot parse the Compose configuration: %w", err)
	}
	secret := config.Services["identity"].Environment["JWT_ACCESS_SECRET"]
	if secret == "" {
		return "", fmt.Errorf("the identity service has no JWT_ACCESS_SECRET; set it in .env or export it")
	}
	return secret, nil
}

// sql runs statements in the Compose database as the fixture owner. It is only
// used to give fixture accounts a role and to remove them again.
func (s *stack) sql(statement string) error {
	command := exec.Command("docker", "compose", "exec", "-T", "postgres", "sh", "-c",
		`exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)
	command.Dir = s.root
	command.Stdin = strings.NewReader(statement)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("fixture SQL failed: %w: %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func (s *stack) cleanup() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if len(s.created) == 0 {
		return
	}
	ids := "'" + strings.Join(s.created, "','") + "'"
	if err := s.sql(fmt.Sprintf(
		`DELETE FROM merchant.merchant_order_status_events WHERE order_id IN (SELECT id FROM merchant.merchant_orders WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s)));
		 DELETE FROM merchant.merchant_orders WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s));
		 DELETE FROM merchant.webhook_delivery_attempts WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s));
		 DELETE FROM merchant.merchant_webhooks WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s));
		 DELETE FROM merchant.merchant_api_clients WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s));
		 DELETE FROM merchant.merchant_status_transitions WHERE merchant_id IN (SELECT id FROM merchant.merchants WHERE owner_user_id IN (%[1]s));
		 DELETE FROM merchant.merchant_registration_attempts WHERE user_id IN (%[1]s);
		 DELETE FROM merchant.merchants WHERE owner_user_id IN (%[1]s);
		 DELETE FROM identity.courier_profiles WHERE user_id IN (%[1]s);
		 DELETE FROM identity.refresh_tokens WHERE user_id IN (%[1]s);
		 DELETE FROM identity.email_verification_tokens WHERE user_id IN (%[1]s);
		 DELETE FROM identity.merchant_lifecycle_transitions WHERE user_id IN (%[1]s);
		 DELETE FROM identity.users WHERE id IN (%[1]s);`, ids)); err != nil {
		fmt.Fprintln(os.Stderr, "access tests: could not remove the fixture accounts:", err)
	}
}

// reply is one HTTP answer.
type reply struct {
	status int
	header http.Header
	body   []byte
}

// code returns error.code from the standard error envelope, or "".
func (r reply) code() string {
	var envelope struct {
		Error struct {
			Code string `json:"code"`
		} `json:"error"`
	}
	_ = json.Unmarshal(r.body, &envelope)
	return envelope.Error.Code
}

// data decodes the "data" member of the success envelope into out.
func (r reply) data(out any) error {
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if err := json.Unmarshal(r.body, &envelope); err != nil {
		return err
	}
	return json.Unmarshal(envelope.Data, out)
}

// do sends one request. A 429 is waited out, because login and registration
// share a limit of 10 requests a minute per address and the suite makes more.
func (s *stack) do(method, path, body string, headers map[string]string) (reply, error) {
	for attempt := 0; ; attempt++ {
		var reader io.Reader
		if body != "" {
			reader = strings.NewReader(body)
		}
		request, err := http.NewRequest(method, s.base+path, reader)
		if err != nil {
			return reply{}, err
		}
		if body != "" {
			request.Header.Set("Content-Type", "application/json")
		}
		for name, value := range headers {
			request.Header.Set(name, value)
		}
		response, err := s.client.Do(request)
		if err != nil {
			return reply{}, fmt.Errorf("%s %s: %w", method, path, err)
		}
		data, err := io.ReadAll(io.LimitReader(response.Body, 2<<20))
		_ = response.Body.Close()
		if err != nil {
			return reply{}, err
		}
		if response.StatusCode == http.StatusTooManyRequests && attempt < 3 {
			wait, _ := strconv.Atoi(response.Header.Get("Retry-After"))
			time.Sleep(time.Duration(min(max(wait, 1), 65)) * time.Second)
			continue
		}
		return reply{status: response.StatusCode, header: response.Header, body: bytes.TrimSpace(data)}, nil
	}
}

func (s *stack) mustDo(t *testing.T, method, path, body string, headers map[string]string) reply {
	t.Helper()
	r, err := s.do(method, path, body, headers)
	if err != nil {
		t.Fatal(err)
	}
	return r
}

// register creates a customer through the public API and remembers it for cleanup.
func (s *stack) register(email, password string) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"email": email, "password": password, "fullName": "Access Test", "phone": "+94771234567",
	})
	r, err := s.do(http.MethodPost, "/api/v1/auth/register", string(body), nil)
	if err != nil {
		return "", err
	}
	if r.status != http.StatusCreated {
		return "", fmt.Errorf("register answered %d: %s", r.status, r.body)
	}
	var account struct {
		ID string `json:"id"`
	}
	if err := r.data(&account); err != nil || account.ID == "" {
		return "", fmt.Errorf("register returned no account ID: %s", r.body)
	}
	s.mu.Lock()
	s.created = append(s.created, account.ID)
	s.mu.Unlock()
	return account.ID, nil
}

// customer is a fresh account for a test that must not share state.
type customer struct {
	id, email, password string
}

func (s *stack) newCustomer(t *testing.T) customer {
	t.Helper()
	c := customer{email: fmt.Sprintf("access.%s@example.test", randomHex(6)), password: "Str0ngPass!"}
	id, err := s.register(c.email, c.password)
	if err != nil {
		t.Fatal(err)
	}
	if err := s.sql(fmt.Sprintf(`UPDATE identity.users SET email_verified = true WHERE id = '%s';`, id)); err != nil {
		t.Fatal(err)
	}
	c.id = id
	return c
}

// newCourier registers a fresh account, gives it the COURIER role and an
// available courier profile, and returns it. The profile is required for the
// dispatch eligibility lookup; the status endpoint only trusts the token.
func (s *stack) newCourier(t *testing.T) customer {
	t.Helper()
	c := s.newCustomer(t)
	if err := s.sql(fmt.Sprintf(
		`UPDATE identity.users SET role_id = (SELECT id FROM identity.roles WHERE name = 'COURIER') WHERE id = '%[1]s';
		 INSERT INTO identity.courier_profiles (user_id, is_available) VALUES ('%[1]s', true)
		 ON CONFLICT (user_id) DO UPDATE SET is_available = true;`, c.id)); err != nil {
		t.Fatal(err)
	}
	return c
}

// createParcel creates a real parcel through the public gateway for the given
// customer session and returns its ID and tracking number.
func (s *stack) createParcel(t *testing.T, accessToken, description string) (id, tracking string) {
	t.Helper()
	zonesReply := s.mustDo(t, http.MethodGet, "/api/v1/zones", "", nil)
	var zones []struct{ ID, Code, Name string }
	if err := zonesReply.data(&zones); err != nil || zonesReply.status != http.StatusOK {
		t.Fatalf("read real pricing zones: HTTP %d; decode %v", zonesReply.status, err)
	}
	var origin, destination string
	for _, zone := range zones {
		switch zone.Code {
		case "CMB":
			origin = zone.ID
		case "KDY":
			destination = zone.ID
		}
	}
	if origin == "" || destination == "" {
		t.Fatal("the disposable access stack needs the migration-seeded CMB and KDY pricing zones")
	}
	estimateBody, err := json.Marshal(map[string]any{"originZoneId": origin, "destinationZoneId": destination, "weightKg": 2, "sizeCategory": "SMALL"})
	if err != nil {
		t.Fatal(err)
	}
	estimate := s.mustDo(t, http.MethodPost, "/api/v1/pricing/estimate", string(estimateBody), nil)
	var fee struct{ Total json.Number }
	if err := estimate.data(&fee); err != nil || estimate.status != http.StatusOK || fee.Total == "" {
		t.Fatalf("obtain real pricing estimate: HTTP %d; decode %v", estimate.status, err)
	}
	createBody, err := json.Marshal(map[string]any{
		"sender":    map[string]string{"name": "Ownership Sender", "phone": "+94771234567", "address": "12 Test Road", "city": "Colombo", "zoneId": origin},
		"recipient": map[string]string{"name": "Ownership Recipient", "phone": "+94712345678", "address": "45 Test Street", "city": "Kandy", "zoneId": destination},
		"weightKg":  2, "sizeCategory": "SMALL", "description": description, "expectedFee": fee.Total,
	})
	if err != nil {
		t.Fatal(err)
	}
	created := s.mustDo(t, http.MethodPost, "/api/v1/parcels", string(createBody), bearer(accessToken))
	var parcel struct{ ID, TrackingNumber string }
	if err := created.data(&parcel); err != nil || created.status != http.StatusCreated || parcel.ID == "" || parcel.TrackingNumber == "" {
		t.Fatalf("create parcel fixture: HTTP %d; decode %v", created.status, err)
	}
	return parcel.ID, parcel.TrackingNumber
}

// sqlQuery runs a single-column SQL query and returns its trimmed output.
func (s *stack) sqlQuery(t *testing.T, query string) string {
	t.Helper()
	command := exec.Command("docker", "compose", "exec", "-T", "postgres", "sh", "-c",
		`exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$POSTGRES_DB"`)
	command.Dir = s.root
	command.Stdin = strings.NewReader(query)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("fixture SQL query failed: %v: %s", err, strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output))
}

// session is what a login or refresh in token mode returns.
type session struct {
	AccessToken  string `json:"accessToken"`
	RefreshToken string `json:"refreshToken"`
}

var tokenMode = map[string]string{"X-Token-Mode": "body"}

func (s *stack) login(t *testing.T, email, password string) (reply, session) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"email": email, "password": password})
	r := s.mustDo(t, http.MethodPost, "/api/v1/auth/login", string(body), tokenMode)
	var out session
	if r.status == http.StatusOK {
		if err := r.data(&out); err != nil {
			t.Fatalf("login returned an unreadable session: %v: %s", err, r.body)
		}
	}
	return r, out
}

func (s *stack) refresh(t *testing.T, refreshToken string) (reply, session) {
	t.Helper()
	body, _ := json.Marshal(map[string]string{"refreshToken": refreshToken})
	r := s.mustDo(t, http.MethodPost, "/api/v1/auth/refresh", string(body), tokenMode)
	var out session
	if r.status == http.StatusOK {
		if err := r.data(&out); err != nil {
			t.Fatalf("refresh returned an unreadable session: %v: %s", err, r.body)
		}
	}
	return r, out
}

func bearer(token string) map[string]string {
	return map[string]string{"Authorization": "Bearer " + token}
}

func randomHex(size int) string {
	raw := make([]byte, size)
	if _, err := rand.Read(raw); err != nil {
		panic(err)
	}
	return hex.EncodeToString(raw)
}
