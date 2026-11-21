package repository

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// ErrOAuthStateInvalid means the OAuth state is unknown, expired or already
// used. The caller cannot tell these apart (API v1.1 section 2.6,
// 400 OAUTH_STATE_INVALID).
var ErrOAuthStateInvalid = errors.New("oauth state is not valid")

// NewOAuthState describes a Google login attempt to remember (DB v1.1 section
// 3.5). RedirectTo, when set, must be a relative path; the table rejects
// anything else.
type NewOAuthState struct {
	State              string
	CodeVerifier       string
	RedirectTo         string
	BrowserBindingHash string
	ExpiresAt          time.Time
}

// OAuthState is what a callback gets back when it presents a valid state.
type OAuthState struct {
	CodeVerifier string
	RedirectTo   string
}

// CreateOAuthState stores a state value for a Google login attempt.
func (r *Repository) CreateOAuthState(ctx context.Context, state NewOAuthState) error {
	if _, err := r.db.Exec(ctx, `
		INSERT INTO identity.oauth_states (state, code_verifier, redirect_to, browser_binding_hash, expires_at)
		VALUES ($1, NULLIF($2, ''), NULLIF($3, ''), NULLIF($4, ''), $5)`,
		state.State, state.CodeVerifier, state.RedirectTo, state.BrowserBindingHash, state.ExpiresAt.UTC(),
	); err != nil {
		return fmt.Errorf("creating oauth state: %w", err)
	}
	return nil
}

// ConsumeOAuthState deletes the state and returns what was stored with it. A
// state works exactly once: the DELETE is a single statement, so two callbacks
// presenting the same state cannot both succeed. The browser-binding hash is
// part of the WHERE clause, so a missing or mismatched binding is rejected
// without burning the legitimate state. It returns ErrOAuthStateInvalid for an
// unknown, expired, already used or wrongly bound state.
func (r *Repository) ConsumeOAuthState(ctx context.Context, state, browserBindingHash string, now time.Time) (OAuthState, error) {
	var (
		verifier   *string
		redirectTo *string
	)
	err := r.db.QueryRow(ctx, `
		DELETE FROM identity.oauth_states
		WHERE state = $1
		  AND browser_binding_hash = $2
		  AND expires_at > $3
		RETURNING code_verifier, redirect_to`,
		state, browserBindingHash, now.UTC(),
	).Scan(&verifier, &redirectTo)
	if errors.Is(err, pgx.ErrNoRows) {
		return OAuthState{}, ErrOAuthStateInvalid
	}
	if err != nil {
		return OAuthState{}, fmt.Errorf("consuming oauth state: %w", err)
	}

	consumed := OAuthState{}
	if verifier != nil {
		consumed.CodeVerifier = *verifier
	}
	if redirectTo != nil {
		consumed.RedirectTo = *redirectTo
	}
	return consumed, nil
}
