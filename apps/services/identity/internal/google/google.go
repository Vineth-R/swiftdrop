// Package google is the identity service's client for Google sign-in: it builds
// the consent URL, exchanges the authorization code and verifies the ID token.
// It implements service.GoogleProvider.
package google

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
)

const (
	issuer   = "https://accounts.google.com"
	certURL  = "https://www.googleapis.com/oauth2/v3/certs"
	authURL  = "https://accounts.google.com/o/oauth2/v2/auth"
	tokenURL = "https://oauth2.googleapis.com/token"

	// requestTimeout bounds each call to Google so a slow Google cannot hold a
	// login request open.
	requestTimeout = 10 * time.Second
)

// Config is the Google OAuth client registered for SwiftDrop.
type Config struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
}

// Client talks to Google. It makes no network call until a login is exchanged,
// so building one never fails because Google is unreachable.
type Client struct {
	oauth    *oauth2.Config
	verifier *oidc.IDTokenVerifier
}

var _ service.GoogleProvider = (*Client)(nil)

// New returns a Client for cfg.
func New(cfg Config) (*Client, error) {
	// The key set caches Google's signing keys for the life of the process, so it
	// must not use a request-scoped context.
	keySet := oidc.NewRemoteKeySet(context.Background(), certURL)
	endpoint := oauth2.Endpoint{AuthURL: authURL, TokenURL: tokenURL}
	return newClient(cfg, endpoint, keySet)
}

func newClient(cfg Config, endpoint oauth2.Endpoint, keySet oidc.KeySet) (*Client, error) {
	if cfg.ClientID == "" || cfg.ClientSecret == "" || cfg.RedirectURI == "" {
		return nil, errors.New("google client requires a client ID, a client secret and a redirect URI")
	}
	return &Client{
		oauth: &oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURI,
			Endpoint:     endpoint,
			Scopes:       []string{oidc.ScopeOpenID, "email", "profile"},
		},
		// The verifier checks the signature against Google's keys, the issuer, that
		// the token was issued to our client ID, and that it has not expired.
		verifier: oidc.NewVerifier(issuer, keySet, &oidc.Config{ClientID: cfg.ClientID}),
	}, nil
}

// AuthURL returns the Google consent URL. The PKCE challenge derived from
// codeVerifier means a stolen authorization code is useless without it.
func (c *Client) AuthURL(state, codeVerifier string) string {
	return c.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(codeVerifier))
}

// Exchange trades the authorization code for tokens and returns the profile
// from the verified ID token.
func (c *Client) Exchange(ctx context.Context, code, codeVerifier string) (service.GoogleProfile, error) {
	ctx, cancel := context.WithTimeout(ctx, requestTimeout)
	defer cancel()

	token, err := c.oauth.Exchange(ctx, code, oauth2.VerifierOption(codeVerifier))
	if err != nil {
		return service.GoogleProfile{}, fmt.Errorf("exchange authorization code: %w", err)
	}

	rawIDToken, ok := token.Extra("id_token").(string)
	if !ok || rawIDToken == "" {
		return service.GoogleProfile{}, errors.New("google response has no ID token")
	}
	idToken, err := c.verifier.Verify(ctx, rawIDToken)
	if err != nil {
		return service.GoogleProfile{}, fmt.Errorf("verify ID token: %w", err)
	}

	var claims struct {
		Email         string `json:"email"`
		EmailVerified bool   `json:"email_verified"`
		Name          string `json:"name"`
	}
	if err := idToken.Claims(&claims); err != nil {
		return service.GoogleProfile{}, fmt.Errorf("read ID token claims: %w", err)
	}
	return service.GoogleProfile{
		Subject:       idToken.Subject,
		Email:         claims.Email,
		EmailVerified: claims.EmailVerified,
		FullName:      claims.Name,
	}, nil
}
