package auth

import (
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	// Issuer is the iss claim written by the identity service and required by
	// every verifier.
	Issuer = "swiftdrop-identity"

	// MinSecretBytes is the minimum recommended HS256 key length (RFC 7518
	// section 3.2). Callers enforce it with CheckSecretStrength.
	MinSecretBytes = 32
)

var (
	// ErrTokenExpired means the token is well formed and correctly signed but
	// past its expiry. Clients should call refresh (API TOKEN_EXPIRED).
	ErrTokenExpired = errors.New("access token expired")

	// ErrTokenInvalid covers every other verification failure: bad signature,
	// wrong algorithm or issuer, malformed token, or unusable claims.
	ErrTokenInvalid = errors.New("access token invalid")
)

// Claims is the verified content of an access token: sub, role, jti, iat and
// exp (API v1.1 section 1.2).
type Claims struct {
	UserID    string
	Role      Role
	TokenID   string
	IssuedAt  time.Time
	ExpiresAt time.Time
}

type jwtClaims struct {
	Role string `json:"role"`
	jwt.RegisteredClaims
}

// TokenManager issues and verifies HS256 access tokens. It is safe for
// concurrent use.
type TokenManager struct {
	secret []byte
	ttl    time.Duration
	now    func() time.Time
}

// Option customises a TokenManager.
type Option func(*TokenManager)

// WithClock replaces the time source. It exists so tests can control expiry.
func WithClock(now func() time.Time) Option {
	return func(m *TokenManager) {
		if now != nil {
			m.now = now
		}
	}
}

// CheckSecretStrength reports whether secret is long enough to sign HS256
// tokens. Services call it where the environment demands a strong secret.
func CheckSecretStrength(secret string) error {
	if len(secret) < MinSecretBytes {
		return fmt.Errorf("signing secret must be at least %d bytes", MinSecretBytes)
	}
	return nil
}

// NewTokenManager returns a manager that signs and verifies with secret.
// ttl is the lifetime given to issued tokens; verifiers may pass any positive
// value because they never issue.
func NewTokenManager(secret string, ttl time.Duration, opts ...Option) (*TokenManager, error) {
	if secret == "" {
		return nil, errors.New("signing secret is required")
	}
	if ttl <= 0 {
		return nil, errors.New("token lifetime must be greater than zero")
	}

	manager := &TokenManager{
		secret: []byte(secret),
		ttl:    ttl,
		now:    time.Now,
	}
	for _, opt := range opts {
		opt(manager)
	}
	return manager, nil
}

// TTL returns the lifetime of issued tokens.
func (m *TokenManager) TTL() time.Duration {
	return m.ttl
}

// Issue signs an access token for userID and role.
func (m *TokenManager) Issue(userID string, role Role) (string, Claims, error) {
	if userID == "" {
		return "", Claims{}, errors.New("user id is required")
	}
	if !role.Valid() {
		return "", Claims{}, errors.New("role is not a SwiftDrop role")
	}

	tokenID, err := newTokenID()
	if err != nil {
		return "", Claims{}, fmt.Errorf("generate token id: %w", err)
	}

	issuedAt := m.now().UTC().Truncate(time.Second)
	claims := Claims{
		UserID:    userID,
		Role:      role,
		TokenID:   tokenID,
		IssuedAt:  issuedAt,
		ExpiresAt: issuedAt.Add(m.ttl),
	}

	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, jwtClaims{
		Role: string(role),
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    Issuer,
			Subject:   userID,
			ID:        tokenID,
			IssuedAt:  jwt.NewNumericDate(claims.IssuedAt),
			ExpiresAt: jwt.NewNumericDate(claims.ExpiresAt),
		},
	}).SignedString(m.secret)
	if err != nil {
		return "", Claims{}, fmt.Errorf("sign access token: %w", err)
	}
	return signed, claims, nil
}

// Verify checks the signature, algorithm, issuer and expiry of token and
// returns its claims. It returns ErrTokenExpired or ErrTokenInvalid.
func (m *TokenManager) Verify(token string) (Claims, error) {
	parsed := jwtClaims{}
	_, err := jwt.ParseWithClaims(
		token,
		&parsed,
		func(*jwt.Token) (any, error) { return m.secret, nil },
		jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}),
		jwt.WithIssuer(Issuer),
		jwt.WithExpirationRequired(),
		jwt.WithIssuedAt(),
		jwt.WithTimeFunc(m.now),
	)
	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return Claims{}, ErrTokenExpired
		}
		return Claims{}, ErrTokenInvalid
	}

	role, ok := ParseRole(parsed.Role)
	if !ok || parsed.Subject == "" || parsed.ID == "" || parsed.IssuedAt == nil || parsed.ExpiresAt == nil {
		return Claims{}, ErrTokenInvalid
	}

	return Claims{
		UserID:    parsed.Subject,
		Role:      role,
		TokenID:   parsed.ID,
		IssuedAt:  parsed.IssuedAt.UTC(),
		ExpiresAt: parsed.ExpiresAt.UTC(),
	}, nil
}

func newTokenID() (string, error) {
	var buffer [16]byte
	if _, err := rand.Read(buffer[:]); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buffer[:]), nil
}
