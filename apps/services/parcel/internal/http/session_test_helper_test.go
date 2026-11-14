package servicehttp

import (
	"context"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

type allowAllSessions struct{}

func (allowAllSessions) Check(context.Context, auth.Claims) error { return nil }

// allowSessions keeps handler contract tests focused on their own behaviour.
// The real session-version behaviour is tested in packages/go-common/sessioncheck.
func allowSessions() sessioncheck.Guard { return allowAllSessions{} }

type rejectingSessions struct{ err error }

func (r rejectingSessions) Check(context.Context, auth.Claims) error { return r.err }
