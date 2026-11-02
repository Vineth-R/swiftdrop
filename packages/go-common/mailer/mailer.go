// Package mailer sends transactional email over SMTP. Services depend on the
// Mailer interface: SMTP talks to Mailpit locally or Gmail for real delivery,
// and Mock records messages in tests. The identity service uses it for password
// reset emails and the notification worker uses it for status emails.
package mailer

import (
	"context"
	"errors"
	"fmt"
	"net/mail"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

const (
	defaultPort     = 1025
	defaultFrom     = "no-reply@swiftdrop.local"
	defaultFromName = "Swift Drop Support"

	// MaxSubjectLength keeps subjects readable and bounds header size.
	MaxSubjectLength = 200
)

// ErrInvalidMessage is wrapped by every error caused by the message itself
// (bad recipient, header injection, empty body) rather than by the server.
var ErrInvalidMessage = errors.New("invalid email message")

// Message is one email to one recipient. Text is required; HTML is optional and
// is sent as an alternative to the plain text part.
type Message struct {
	To      string
	Subject string
	Text    string
	HTML    string
}

// Mailer sends email. Implementations must be safe for concurrent use.
type Mailer interface {
	Send(ctx context.Context, message Message) error
}

// Config is the SMTP connection, loaded from SMTP_HOST, SMTP_PORT, SMTP_USER,
// SMTP_PASSWORD, SMTP_FROM and SMTP_FROM_NAME (docs/architecture.md).
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string
}

// LoadConfig reads the SMTP settings. A blank SMTP_HOST is allowed and means
// email is not configured (see Config.Enabled), so a service can start without
// it.
func LoadConfig(loader config.Loader) (Config, error) {
	port, err := loader.Port("SMTP_PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}
	return Config{
		Host:     loader.String("SMTP_HOST", ""),
		Port:     port,
		User:     loader.String("SMTP_USER", ""),
		Password: loader.String("SMTP_PASSWORD", ""),
		From:     loader.String("SMTP_FROM", defaultFrom),
		FromName: loader.String("SMTP_FROM_NAME", defaultFromName),
	}, nil
}

// Enabled reports whether an SMTP server is configured.
func (c Config) Enabled() bool { return c.Host != "" }

// Validate checks a configuration that is meant to be used. Its messages name
// settings, never their values.
func (c Config) Validate() error {
	if c.Host == "" {
		return errors.New("SMTP_HOST is not set")
	}
	if c.Port < 1 || c.Port > 65535 {
		return errors.New("SMTP_PORT must be between 1 and 65535")
	}
	if address, err := mail.ParseAddress(c.From); err != nil || address.Address != c.From {
		return errors.New("SMTP_FROM must be a plain email address")
	}
	if strings.ContainsAny(c.FromName, "\r\n") {
		return errors.New("SMTP_FROM_NAME must not contain line breaks")
	}
	if (c.User == "") != (c.Password == "") {
		return errors.New("SMTP_USER and SMTP_PASSWORD must be set together")
	}
	return nil
}

// validate checks a message before any network traffic. It returns the parsed
// recipient address.
func (m Message) validate() (string, error) {
	address, err := mail.ParseAddress(m.To)
	if err != nil || address.Address != m.To || strings.ContainsAny(m.To, "\r\n") {
		return "", fmt.Errorf("%w: recipient is not a plain email address", ErrInvalidMessage)
	}
	if strings.TrimSpace(m.Subject) == "" || len(m.Subject) > MaxSubjectLength || strings.ContainsAny(m.Subject, "\r\n") {
		return "", fmt.Errorf("%w: subject must be 1 to %d characters on one line", ErrInvalidMessage, MaxSubjectLength)
	}
	if strings.TrimSpace(m.Text) == "" {
		return "", fmt.Errorf("%w: text body is required", ErrInvalidMessage)
	}
	return address.Address, nil
}
