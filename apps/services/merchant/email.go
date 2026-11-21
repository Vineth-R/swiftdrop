package main

import (
	"fmt"
	"log/slog"
	"strings"

	serviceconfig "github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/config"
	"github.com/Vineth-R/swiftdrop/apps/services/merchant/internal/notify"
	sharedconfig "github.com/Vineth-R/swiftdrop/packages/go-common/config"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

type emailSetup struct {
	Sender   mailer.Mailer
	Site     emails.Site
	Notifier *notify.Notifier
	Wait     func()
}

func setUpEmail(config serviceconfig.Config, logger *slog.Logger) (emailSetup, error) {
	loader := sharedconfig.Load()
	mailConfig, err := mailer.LoadConfig(loader)
	if err != nil {
		return emailSetup{}, fmt.Errorf("load mailer config: %w", err)
	}
	support := config.SupportEmail
	if support == "" {
		support = mailConfig.ReplyTo
	}
	if mailConfig.ReplyTo == "" {
		mailConfig.ReplyTo = support
	}
	if err := mailConfig.Validate(); err != nil {
		logger.Warn("Merchant email is disabled: " + err.Error())
		return emailSetup{Wait: func() {}}, nil
	}
	sender, err := mailer.NewSMTP(mailConfig)
	if err != nil {
		return emailSetup{}, fmt.Errorf("create mailer: %w", err)
	}
	site := emails.Site{BaseURL: strings.TrimRight(config.FrontendBaseURL, "/"), SupportEmail: support}
	notifier := notify.New(sender, site, logger)
	logger.Info("Merchant email is enabled", "smtp_host", mailConfig.Host, "smtp_port", mailConfig.Port)
	return emailSetup{Sender: sender, Site: site, Notifier: notifier, Wait: notifier.Wait}, nil
}
