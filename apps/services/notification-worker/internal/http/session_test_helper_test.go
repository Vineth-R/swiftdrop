package servicehttp

import (
	"context"

	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

type allowNotificationSessions struct{}

func (allowNotificationSessions) Check(context.Context, auth.Claims) error { return nil }

func notificationSessions() sessioncheck.Guard { return allowNotificationSessions{} }

type revokeNotificationSessions struct{ err error }

func (r revokeNotificationSessions) Check(context.Context, auth.Claims) error { return r.err }
