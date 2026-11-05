package mailer

import (
	"bytes"
	"context"
	"crypto/rand"
	"crypto/tls"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"fmt"
	"mime"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"net/smtp"
	"strconv"
	"strings"
	"time"
)

const (
	// sendTimeout bounds one whole delivery (connect, TLS, auth, send).
	sendTimeout = 15 * time.Second

	ehloName = "swiftdrop"
)

// SMTP sends mail through an SMTP server. When the server offers STARTTLS the
// connection is upgraded with certificate verification on. Credentials are sent
// only over TLS: a server that cannot do STARTTLS is refused if a user is
// configured, so a password never crosses the network in clear text.
type SMTP struct {
	config    Config
	tlsConfig *tls.Config
	now       func() time.Time
}

var _ Mailer = (*SMTP)(nil)

// NewSMTP returns an SMTP mailer for config.
func NewSMTP(config Config) (*SMTP, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("smtp mailer: %w", err)
	}
	return &SMTP{
		config:    config,
		tlsConfig: &tls.Config{ServerName: config.Host, MinVersion: tls.VersionTLS12},
		now:       time.Now,
	}, nil
}

// Send delivers message. Errors never include the password or message body.
func (s *SMTP) Send(ctx context.Context, message Message) error {
	recipient, err := message.validate()
	if err != nil {
		return err
	}
	body, err := s.build(message)
	if err != nil {
		return fmt.Errorf("smtp send: %w", err)
	}

	ctx, cancel := context.WithTimeout(ctx, sendTimeout)
	defer cancel()

	address := net.JoinHostPort(s.config.Host, strconv.Itoa(s.config.Port))
	var connection net.Conn
	if s.config.ImplicitTLS {
		connection, err = (&tls.Dialer{NetDialer: &net.Dialer{}, Config: s.tlsConfig}).DialContext(ctx, "tcp", address)
	} else {
		connection, err = (&net.Dialer{}).DialContext(ctx, "tcp", address)
	}
	if err != nil {
		return fmt.Errorf("smtp send: connect to %s: %w", address, err)
	}
	defer connection.Close()
	if deadline, ok := ctx.Deadline(); ok {
		_ = connection.SetDeadline(deadline)
	}

	client, err := smtp.NewClient(connection, s.config.Host)
	if err != nil {
		return fmt.Errorf("smtp send: greeting: %w", err)
	}
	defer client.Close()

	if err := client.Hello(ehloName); err != nil {
		return fmt.Errorf("smtp send: hello: %w", err)
	}
	if s.config.ImplicitTLS {
		// The connection is encrypted already; there is nothing to upgrade.
	} else if offered, _ := client.Extension("STARTTLS"); offered {
		if err := client.StartTLS(s.tlsConfig); err != nil {
			return fmt.Errorf("smtp send: starttls: %w", err)
		}
	} else if s.config.User != "" {
		return errors.New("smtp send: server does not offer STARTTLS; refusing to send credentials")
	}
	if s.config.User != "" {
		auth := smtp.PlainAuth("", s.config.User, s.config.Password, s.config.Host)
		if err := client.Auth(auth); err != nil {
			return errors.New("smtp send: authentication failed")
		}
	}

	if err := client.Mail(s.config.From); err != nil {
		return fmt.Errorf("smtp send: sender rejected: %w", err)
	}
	if err := client.Rcpt(recipient); err != nil {
		return fmt.Errorf("smtp send: recipient rejected: %w", err)
	}
	writer, err := client.Data()
	if err != nil {
		return fmt.Errorf("smtp send: data: %w", err)
	}
	if _, err := writer.Write(body); err != nil {
		return fmt.Errorf("smtp send: write message: %w", err)
	}
	if err := writer.Close(); err != nil {
		return fmt.Errorf("smtp send: finish message: %w", err)
	}
	// The message is accepted at this point; a failed QUIT changes nothing.
	_ = client.Quit()
	return nil
}

// build renders the RFC 5322 message: encoded headers and a quoted-printable
// UTF-8 body, multipart when an HTML part is present.
func (s *SMTP) build(message Message) ([]byte, error) {
	messageID, err := randomHex(16)
	if err != nil {
		return nil, err
	}
	from := (&mail.Address{Name: s.config.FromName, Address: s.config.From}).String()
	_, domain, _ := strings.Cut(s.config.From, "@")

	var out bytes.Buffer
	header := func(name, value string) { out.WriteString(name + ": " + value + "\r\n") }
	header("From", from)
	header("To", message.To)
	header("Subject", mime.QEncoding.Encode("utf-8", message.Subject))
	header("Date", s.now().UTC().Format(time.RFC1123Z))
	header("Message-ID", "<"+messageID+"@"+domain+">")
	header("MIME-Version", "1.0")
	if replyTo := firstNonEmpty(message.ReplyTo, s.config.ReplyTo); replyTo != "" {
		header("Reply-To", replyTo)
	}

	if message.HTML == "" {
		header("Content-Type", `text/plain; charset="utf-8"`)
		header("Content-Transfer-Encoding", "quoted-printable")
		out.WriteString("\r\n")
		if err := writeQuoted(&out, message.Text); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}

	alternative, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	if len(message.Inline) == 0 {
		header("Content-Type", `multipart/alternative; boundary="`+alternative+`"`)
		out.WriteString("\r\n")
		if err := writeAlternative(&out, alternative, message); err != nil {
			return nil, err
		}
		return out.Bytes(), nil
	}

	// Inline images ride in multipart/related around the alternative part, which
	// is what makes src="cid:..." resolve in the HTML.
	related, err := randomHex(12)
	if err != nil {
		return nil, err
	}
	header("Content-Type", `multipart/related; type="multipart/alternative"; boundary="`+related+`"`)
	out.WriteString("\r\n--" + related + "\r\n")
	out.WriteString(`Content-Type: multipart/alternative; boundary="` + alternative + `"` + "\r\n\r\n")
	if err := writeAlternative(&out, alternative, message); err != nil {
		return nil, err
	}
	for _, image := range message.Inline {
		out.WriteString("--" + related + "\r\n")
		out.WriteString("Content-Type: " + image.ContentType + `; name="` + image.ContentID + `"` + "\r\n")
		out.WriteString("Content-Transfer-Encoding: base64\r\n")
		out.WriteString("Content-ID: <" + image.ContentID + ">\r\n")
		out.WriteString(`Content-Disposition: inline; filename="` + image.ContentID + `"` + "\r\n\r\n")
		writeBase64(&out, image.Data)
	}
	out.WriteString("--" + related + "--\r\n")
	return out.Bytes(), nil
}

// writeAlternative writes the plain text part and then the HTML part, the order
// a mail client reads them in (the last one it can show wins), and the closing
// boundary.
func writeAlternative(out *bytes.Buffer, boundary string, message Message) error {
	for _, part := range []struct{ contentType, body string }{
		{`text/plain; charset="utf-8"`, message.Text},
		{`text/html; charset="utf-8"`, message.HTML},
	} {
		out.WriteString("--" + boundary + "\r\n")
		out.WriteString("Content-Type: " + part.contentType + "\r\n")
		out.WriteString("Content-Transfer-Encoding: quoted-printable\r\n\r\n")
		if err := writeQuoted(out, part.body); err != nil {
			return err
		}
		out.WriteString("\r\n")
	}
	out.WriteString("--" + boundary + "--\r\n")
	return nil
}

// writeBase64 writes data as base64 in lines of 76 characters.
func writeBase64(out *bytes.Buffer, data []byte) {
	encoded := base64.StdEncoding.EncodeToString(data)
	for len(encoded) > 76 {
		out.WriteString(encoded[:76] + "\r\n")
		encoded = encoded[76:]
	}
	out.WriteString(encoded + "\r\n")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func writeQuoted(out *bytes.Buffer, text string) error {
	writer := quotedprintable.NewWriter(out)
	if _, err := writer.Write([]byte(text)); err != nil {
		return err
	}
	return writer.Close()
}

func randomHex(size int) (string, error) {
	buffer := make([]byte, size)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate random value: %w", err)
	}
	return hex.EncodeToString(buffer), nil
}
