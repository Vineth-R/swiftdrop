package main

import (
	"fmt"
	"log/slog"
	"net/url"
	"strings"

	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/notify"
	"github.com/Vineth-R/swiftdrop/apps/services/identity/internal/service"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

// emailSetup is everything the service needs to send email, built once from
// the environment. When email is not configured, Sender and Notifier are nil
// and DisabledReason says why; the service still starts, without the reset
// routes and without account emails.
type emailSetup struct {
	// Sender delivers a message over SMTP. Nil when email is disabled.
	Sender mailer.Mailer
	// Site holds the web app's address and the support address for the links
	// and the footer of every email.
	Site emails.Site
	// Notifier sends the account emails. Nil when email is disabled.
	Notifier service.Notifier
	// DisabledReason explains why email is off, in words that name settings and
	// never their values.
	DisabledReason string
	// Wait blocks until background emails are delivered. It is never nil.
	Wait func()
}

// setUpEmail reads SMTP_HOST, SMTP_PORT, SMTP_USER, SMTP_PASSWORD, SMTP_FROM,
// SMTP_FROM_NAME, SMTP_REPLY_TO, SMTP_SECURE, SUPPORT_EMAIL and
// FRONTEND_BASE_URL (docs/email.md). A missing or unusable setting turns email
// off with a warning instead of stopping the service, so CI and a developer
// without Mailpit can still start it.
func setUpEmail(logger *slog.Logger) (emailSetup, error) {
	loader := sharedconfig.Load()
	off := func(reason string) (emailSetup, error) {
		logger.Warn("Email is disabled: " + reason)
		return emailSetup{DisabledReason: reason, Wait: func() {}}, nil
	}

	mailConfig, err := mailer.LoadConfig(loader)
	if err != nil {
		return emailSetup{}, fmt.Errorf("load mailer config: %w", err)
	}
	// One support address answers every email: people reply to it and the footer
	// shows it.
	support := loader.String("SUPPORT_EMAIL", mailConfig.ReplyTo)
	if mailConfig.ReplyTo == "" {
		mailConfig.ReplyTo = support
	}
	if err := mailConfig.Validate(); err != nil {
		return off(err.Error())
	}

	frontend := strings.TrimRight(loader.String("FRONTEND_BASE_URL", ""), "/")
	if parsed, err := url.Parse(frontend); err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return off("FRONTEND_BASE_URL must be an absolute http or https URL")
	}

	sender, err := mailer.NewSMTP(mailConfig)
	if err != nil {
		return emailSetup{}, fmt.Errorf("create mailer: %w", err)
	}
	site := emails.Site{BaseURL: frontend, SupportEmail: support}
	notifier := notify.New(sender, site, logger)

	logger.Info("Email is enabled", "smtp_host", mailConfig.Host, "smtp_port", mailConfig.Port, "implicit_tls", mailConfig.ImplicitTLS)
	return emailSetup{Sender: sender, Site: site, Notifier: notifier, Wait: notifier.Wait}, nil
}
