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

	// ReplyTo overrides Config.ReplyTo for this message. Empty uses the config's.
	ReplyTo string

	// Inline are images the HTML refers to as src="cid:<ContentID>", such as the
	// logo. They travel inside the message, so they show even where remote
	// images are blocked. They need an HTML part.
	Inline []InlineImage
}

// InlineImage is an image embedded in the message.
type InlineImage struct {
	// ContentID is the id the HTML uses: src="cid:logo-mark".
	ContentID string
	// ContentType is image/png, image/jpeg or image/gif.
	ContentType string
	Data        []byte
}

const (
	maxInlineImages = 4
	// maxInlineBytes bounds all inline images of one message together.
	maxInlineBytes = 512 << 10
)

// Mailer sends email. Implementations must be safe for concurrent use.
type Mailer interface {
	Send(ctx context.Context, message Message) error
}

// Config is the SMTP connection, loaded from SMTP_HOST, SMTP_PORT, SMTP_USER,
// SMTP_PASSWORD, SMTP_FROM, SMTP_FROM_NAME, SMTP_REPLY_TO and SMTP_SECURE
// (docs/architecture.md, docs/email.md).
type Config struct {
	Host     string
	Port     int
	User     string
	Password string
	From     string
	FromName string

	// ReplyTo is where a person's reply goes, when it should not go to From
	// (for example a no-reply sender and a support mailbox). Empty adds no header.
	ReplyTo string

	// ImplicitTLS connects with TLS from the first byte (SMTPS, usually port
	// 465). Otherwise the connection starts in clear text and is upgraded with
	// STARTTLS when the server offers it (port 587, and Mailpit locally).
	ImplicitTLS bool
}

// LoadConfig reads the SMTP settings. A blank SMTP_HOST is allowed and means
// email is not configured (see Config.Enabled), so a service can start without
// it.
func LoadConfig(loader config.Loader) (Config, error) {
	port, err := loader.Port("SMTP_PORT", defaultPort)
	if err != nil {
		return Config{}, err
	}
	// auto means implicit TLS on port 465 and STARTTLS everywhere else.
	secure, err := loader.EnumFold("SMTP_SECURE", "auto", "auto", "true", "false")
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
		ReplyTo:  loader.String("SMTP_REPLY_TO", ""),

		ImplicitTLS: secure == "true" || (secure == "auto" && port == 465),
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
	if c.ReplyTo != "" {
		if address, err := mail.ParseAddress(c.ReplyTo); err != nil || address.Address != c.ReplyTo {
			return errors.New("SMTP_REPLY_TO must be a plain email address")
		}
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
	if m.ReplyTo != "" {
		reply, err := mail.ParseAddress(m.ReplyTo)
		if err != nil || reply.Address != m.ReplyTo || strings.ContainsAny(m.ReplyTo, "\r\n") {
			return "", fmt.Errorf("%w: reply-to is not a plain email address", ErrInvalidMessage)
		}
	}
	if err := m.validateInline(); err != nil {
		return "", err
	}
	return address.Address, nil
}

func (m Message) validateInline() error {
	if len(m.Inline) == 0 {
		return nil
	}
	if m.HTML == "" {
		return fmt.Errorf("%w: inline images need an HTML part", ErrInvalidMessage)
	}
	if len(m.Inline) > maxInlineImages {
		return fmt.Errorf("%w: at most %d inline images", ErrInvalidMessage, maxInlineImages)
	}
	total := 0
	for _, image := range m.Inline {
		total += len(image.Data)
		switch image.ContentType {
		case "image/png", "image/jpeg", "image/gif":
		default:
			return fmt.Errorf("%w: inline image type must be image/png, image/jpeg or image/gif", ErrInvalidMessage)
		}
		if len(image.Data) == 0 || !validContentID(image.ContentID) {
			return fmt.Errorf("%w: inline image needs data and a simple content id", ErrInvalidMessage)
		}
	}
	if total > maxInlineBytes {
		return fmt.Errorf("%w: inline images are larger than %d bytes", ErrInvalidMessage, maxInlineBytes)
	}
	return nil
}

// validContentID allows letters, digits and . _ - only, so an id can never
// break out of a header.
func validContentID(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for _, r := range id {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '.', r == '_', r == '-':
		default:
			return false
		}
	}
	return true
}
