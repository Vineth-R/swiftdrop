package google

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/go-jose/go-jose/v4"
	"github.com/go-jose/go-jose/v4/jwt"
	"golang.org/x/oauth2"
)

const (
	testClientID    = "client-id.apps.example"
	testRedirectURI = "http://localhost:8080/api/v1/auth/google/callback"
	testVerifier    = "verifier-0123456789-0123456789-0123456789-abcdef"
)

// fakeGoogle is a token endpoint that returns whatever ID token the test set,
// and records what the client sent.
type fakeGoogle struct {
	server  *httptest.Server
	key     *rsa.PrivateKey
	idToken string
	status  int

	form url.Values
}

func newFakeGoogle(t *testing.T) *fakeGoogle {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	g := &fakeGoogle{key: key, status: http.StatusOK}
	g.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = r.ParseForm()
		g.form = r.PostForm
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(g.status)
		if g.status != http.StatusOK {
			_, _ = w.Write([]byte(`{"error":"invalid_grant"}`))
			return
		}
		body := map[string]string{"access_token": "opaque-access-token", "token_type": "Bearer"}
		if g.idToken != "" {
			body["id_token"] = g.idToken
		}
		_ = json.NewEncoder(w).Encode(body)
	}))
	t.Cleanup(g.server.Close)
	return g
}

func (g *fakeGoogle) client(t *testing.T) *Client {
	t.Helper()
	client, err := newClient(
		Config{ClientID: testClientID, ClientSecret: "test-only-secret", RedirectURI: testRedirectURI},
		oauth2.Endpoint{AuthURL: "https://accounts.example/auth", TokenURL: g.server.URL},
		&oidc.StaticKeySet{PublicKeys: []crypto.PublicKey{&g.key.PublicKey}},
	)
	if err != nil {
		t.Fatalf("newClient returned error: %v", err)
	}
	return client
}

type idClaims struct {
	Issuer        string           `json:"iss"`
	Subject       string           `json:"sub"`
	Audience      string           `json:"aud"`
	Expiry        *jwt.NumericDate `json:"exp"`
	IssuedAt      *jwt.NumericDate `json:"iat"`
	Email         string           `json:"email"`
	EmailVerified bool             `json:"email_verified"`
	Name          string           `json:"name"`
}

func validClaims() idClaims {
	return idClaims{
		Issuer: issuer, Subject: "google-sub-1", Audience: testClientID,
		Expiry:   jwt.NewNumericDate(time.Now().Add(time.Hour)),
		IssuedAt: jwt.NewNumericDate(time.Now()),
		Email:    "nimal@example.com", EmailVerified: true, Name: "Nimal Perera",
	}
}

func sign(t *testing.T, key *rsa.PrivateKey, claims idClaims) string {
	t.Helper()
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key},
		(&jose.SignerOptions{}).WithType("JWT"))
	if err != nil {
		t.Fatalf("new signer: %v", err)
	}
	token, err := jwt.Signed(signer).Claims(claims).Serialize()
	if err != nil {
		t.Fatalf("sign token: %v", err)
	}
	return token
}

func TestNew_requiresAllSettings(t *testing.T) {
	for _, cfg := range []Config{
		{},
		{ClientID: "id", ClientSecret: "secret"},
		{ClientID: "id", RedirectURI: testRedirectURI},
		{ClientSecret: "secret", RedirectURI: testRedirectURI},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatalf("New(%+v) accepted an incomplete configuration", cfg)
		}
	}
	if _, err := New(Config{ClientID: "id", ClientSecret: "secret", RedirectURI: testRedirectURI}); err != nil {
		t.Fatalf("New returned error for a complete configuration: %v", err)
	}
}

func TestClient_AuthURL(t *testing.T) {
	client := newFakeGoogle(t).client(t)

	parsed, err := url.Parse(client.AuthURL("state-123", testVerifier))
	if err != nil {
		t.Fatalf("AuthURL is not a URL: %v", err)
	}
	query := parsed.Query()

	sum := sha256.Sum256([]byte(testVerifier))
	want := map[string]string{
		"client_id":             testClientID,
		"redirect_uri":          testRedirectURI,
		"response_type":         "code",
		"scope":                 "openid email profile",
		"state":                 "state-123",
		"code_challenge_method": "S256",
		"code_challenge":        base64.RawURLEncoding.EncodeToString(sum[:]),
	}
	for key, value := range want {
		if got := query.Get(key); got != value {
			t.Errorf("%s = %q, want %q", key, got, value)
		}
	}
	if strings.Contains(parsed.RawQuery, testVerifier) {
		t.Error("the PKCE verifier itself must never appear in the URL")
	}
	if strings.Contains(parsed.RawQuery, "client_secret") {
		t.Error("the client secret must never appear in the URL")
	}
}

func TestClient_Exchange_success(t *testing.T) {
	google := newFakeGoogle(t)
	google.idToken = sign(t, google.key, validClaims())

	profile, err := google.client(t).Exchange(context.Background(), "the-code", testVerifier)
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if profile.Subject != "google-sub-1" || profile.Email != "nimal@example.com" ||
		!profile.EmailVerified || profile.FullName != "Nimal Perera" {
		t.Fatalf("profile = %+v", profile)
	}

	if google.form.Get("code") != "the-code" || google.form.Get("grant_type") != "authorization_code" ||
		google.form.Get("redirect_uri") != testRedirectURI {
		t.Fatalf("token request form = %v", google.form)
	}
	if google.form.Get("code_verifier") != testVerifier {
		t.Fatal("the PKCE verifier was not sent to the token endpoint")
	}
}

func TestClient_Exchange_acceptsIssuerWithoutScheme(t *testing.T) {
	google := newFakeGoogle(t)
	claims := validClaims()
	claims.Issuer = "accounts.google.com"
	google.idToken = sign(t, google.key, claims)

	if _, err := google.client(t).Exchange(context.Background(), "code", testVerifier); err != nil {
		t.Fatalf("Exchange rejected Google's scheme-less issuer: %v", err)
	}
}

func TestClient_Exchange_unverifiedEmailIsReportedNotHidden(t *testing.T) {
	google := newFakeGoogle(t)
	claims := validClaims()
	claims.EmailVerified = false
	google.idToken = sign(t, google.key, claims)

	profile, err := google.client(t).Exchange(context.Background(), "code", testVerifier)
	if err != nil {
		t.Fatalf("Exchange returned error: %v", err)
	}
	if profile.EmailVerified {
		t.Fatal("email_verified=false was reported as verified")
	}
}

func TestClient_Exchange_rejectsBadIDTokens(t *testing.T) {
	otherKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}

	tests := []struct {
		name  string
		token func(g *fakeGoogle) string
	}{
		{name: "token issued to another client", token: func(g *fakeGoogle) string {
			c := validClaims()
			c.Audience = "someone-elses-client-id"
			return sign(t, g.key, c)
		}},
		{name: "wrong issuer", token: func(g *fakeGoogle) string {
			c := validClaims()
			c.Issuer = "https://evil.example"
			return sign(t, g.key, c)
		}},
		{name: "expired", token: func(g *fakeGoogle) string {
			c := validClaims()
			c.Expiry = jwt.NewNumericDate(time.Now().Add(-time.Hour))
			return sign(t, g.key, c)
		}},
		{name: "signed with an unknown key", token: func(g *fakeGoogle) string {
			return sign(t, otherKey, validClaims())
		}},
		{name: "not a token at all", token: func(*fakeGoogle) string { return "garbage" }},
		{name: "no ID token in the response", token: func(*fakeGoogle) string { return "" }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			google := newFakeGoogle(t)
			google.idToken = test.token(google)

			if _, err := google.client(t).Exchange(context.Background(), "code", testVerifier); err == nil {
				t.Fatal("Exchange accepted an unusable ID token")
			}
		})
	}
}

func TestClient_Exchange_googleRejectsTheCode(t *testing.T) {
	google := newFakeGoogle(t)
	google.status = http.StatusBadRequest

	_, err := google.client(t).Exchange(context.Background(), "used-code", testVerifier)
	if err == nil {
		t.Fatal("Exchange succeeded although Google refused the code")
	}
	if strings.Contains(err.Error(), "test-only-secret") {
		t.Fatal("the client secret leaked into the error")
	}
}
