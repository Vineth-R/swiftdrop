package mailer

import (
	"bufio"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"errors"
	"io"
	"math/big"
	"mime"
	"mime/multipart"
	"mime/quotedprintable"
	"net"
	"net/mail"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/config"
)

// fakeServer is a one-connection-at-a-time SMTP server that records what the
// client sent.
type fakeServer struct {
	listener net.Listener
	cert     tls.Certificate
	starttls bool // advertise STARTTLS

	mu       sync.Mutex
	commands []string
	data     string
	authLine string
	rcpt     string
	failRcpt bool
}

func newFakeServer(t *testing.T, starttls bool) *fakeServer {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	s := &fakeServer{listener: listener, starttls: starttls, cert: selfSignedCert(t)}
	t.Cleanup(func() { _ = listener.Close() })
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go s.serve(connection)
		}
	}()
	return s
}

func (s *fakeServer) port() int { return s.listener.Addr().(*net.TCPAddr).Port }

func (s *fakeServer) record(line string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.commands = append(s.commands, line)
}

func (s *fakeServer) snapshot() (commands []string, data, auth, rcpt string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.commands...), s.data, s.authLine, s.rcpt
}

func (s *fakeServer) serve(connection net.Conn) {
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(10 * time.Second))
	reader := bufio.NewReader(connection)
	write := func(line string) { _, _ = io.WriteString(connection, line+"\r\n") }
	secure := false

	write("220 fake ESMTP")
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		verb := strings.ToUpper(strings.SplitN(line, " ", 2)[0])
		if verb != "AUTH" { // never record credentials, even in tests
			s.record(line)
		}

		switch verb {
		case "EHLO":
			write("250-fake")
			if s.starttls && !secure {
				write("250-STARTTLS")
			}
			write("250 AUTH PLAIN")
		case "STARTTLS":
			write("220 go ahead")
			tlsConnection := tls.Server(connection, &tls.Config{Certificates: []tls.Certificate{s.cert}})
			if err := tlsConnection.Handshake(); err != nil {
				return
			}
			connection, reader, secure = tlsConnection, bufio.NewReader(tlsConnection), true
			write = func(line string) { _, _ = io.WriteString(tlsConnection, line+"\r\n") }
		case "AUTH":
			s.mu.Lock()
			s.authLine = line
			s.mu.Unlock()
			write("235 ok")
		case "MAIL":
			write("250 ok")
		case "RCPT":
			if s.failRcpt {
				write("550 no such user")
				continue
			}
			s.mu.Lock()
			s.rcpt = line
			s.mu.Unlock()
			write("250 ok")
		case "DATA":
			write("354 go")
			var body strings.Builder
			for {
				part, err := reader.ReadString('\n')
				if err != nil {
					return
				}
				if part == ".\r\n" {
					break
				}
				body.WriteString(part)
			}
			s.mu.Lock()
			s.data = body.String()
			s.mu.Unlock()
			write("250 queued")
		case "QUIT":
			write("221 bye")
			return
		default:
			write("502 not implemented")
		}
	}
}

func selfSignedCert(t *testing.T) tls.Certificate {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := &x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "127.0.0.1"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
		KeyUsage:     x509.KeyUsageDigitalSignature,
		ExtKeyUsage:  []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
		IPAddresses:  []net.IP{net.ParseIP("127.0.0.1")},
	}
	der, err := x509.CreateCertificate(rand.Reader, template, template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	return tls.Certificate{Certificate: [][]byte{der}, PrivateKey: key}
}

func (s *fakeServer) mailer(t *testing.T, user, password string, trustServer bool) *SMTP {
	t.Helper()
	sender, err := NewSMTP(Config{
		Host: "127.0.0.1", Port: s.port(), User: user, Password: password,
		From: "no-reply@swiftdrop.local", FromName: "Swift Drop Support",
	})
	if err != nil {
		t.Fatalf("NewSMTP returned error: %v", err)
	}
	if trustServer {
		pool := x509.NewCertPool()
		parsed, _ := x509.ParseCertificate(s.cert.Certificate[0])
		pool.AddCert(parsed)
		sender.tlsConfig.RootCAs = pool
	}
	return sender
}

func resetMessage() Message {
	return Message{
		To:      "nimal@example.com",
		Subject: "Reset your SwiftDrop password — ශ්‍රී",
		Text:    "Open https://swiftdrop.example/reset-password?token=abc\r\n.\r\nline starting with a dot above",
	}
}

func TestSMTP_Send_plainServerLikeMailpit(t *testing.T) {
	server := newFakeServer(t, false)

	if err := server.mailer(t, "", "", false).Send(context.Background(), resetMessage()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	commands, data, authLine, rcpt := server.snapshot()
	if authLine != "" {
		t.Fatal("no credentials are configured, none must be sent")
	}
	for _, command := range commands {
		if strings.EqualFold(command, "STARTTLS") {
			t.Fatal("STARTTLS was used although the server did not offer it")
		}
	}
	if !strings.Contains(rcpt, "<nimal@example.com>") {
		t.Fatalf("RCPT = %q", rcpt)
	}

	parsed, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("delivered data is not an email: %v", err)
	}
	decoder := new(mime.WordDecoder)
	subject, err := decoder.DecodeHeader(parsed.Header.Get("Subject"))
	if err != nil || subject != resetMessage().Subject {
		t.Fatalf("Subject = %q (%v), want the original", subject, err)
	}
	if from := parsed.Header.Get("From"); !strings.Contains(from, "no-reply@swiftdrop.local") || !strings.Contains(from, "Swift Drop Support") {
		t.Fatalf("From = %q", from)
	}
	if parsed.Header.Get("To") != "nimal@example.com" || parsed.Header.Get("Message-ID") == "" || parsed.Header.Get("Date") == "" {
		t.Fatalf("missing standard headers: %v", parsed.Header)
	}
	body, _ := io.ReadAll(quotedprintable.NewReader(parsed.Body))
	if !strings.Contains(string(body), "reset-password?token=abc") || !strings.Contains(string(body), "line starting with a dot above") {
		t.Fatalf("body = %q", body)
	}
}

func TestSMTP_Send_multipartWithHTML(t *testing.T) {
	server := newFakeServer(t, false)
	message := resetMessage()
	message.HTML = `<p>Reset: <a href="https://swiftdrop.example/reset-password?token=abc">link</a></p>`

	if err := server.mailer(t, "", "", false).Send(context.Background(), message); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	_, data, _, _ := server.snapshot()
	parsed, err := mail.ReadMessage(strings.NewReader(data))
	if err != nil {
		t.Fatalf("not an email: %v", err)
	}
	mediaType, params, err := mime.ParseMediaType(parsed.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/alternative" {
		t.Fatalf("Content-Type = %q (%v), want multipart/alternative", mediaType, err)
	}
	reader := multipart.NewReader(parsed.Body, params["boundary"])
	var types []string
	for {
		part, err := reader.NextPart()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			t.Fatalf("read part: %v", err)
		}
		types = append(types, strings.SplitN(part.Header.Get("Content-Type"), ";", 2)[0])
	}
	if strings.Join(types, ",") != "text/plain,text/html" {
		t.Fatalf("parts = %v, want text/plain then text/html", types)
	}
}

func TestSMTP_Send_startTLSAndAuth(t *testing.T) {
	server := newFakeServer(t, true)

	if err := server.mailer(t, "user@example.com", "app-password", true).Send(context.Background(), resetMessage()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}

	commands, _, authLine, _ := server.snapshot()
	starttlsAt, mailAt := -1, -1
	for i, command := range commands {
		switch strings.ToUpper(strings.SplitN(command, " ", 2)[0]) {
		case "STARTTLS":
			starttlsAt = i
		case "MAIL":
			mailAt = i
		}
	}
	if starttlsAt == -1 || starttlsAt > mailAt {
		t.Fatalf("commands = %v, want STARTTLS before MAIL", commands)
	}
	encoded := strings.TrimPrefix(authLine, "AUTH PLAIN ")
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil || string(raw) != "\x00user@example.com\x00app-password" {
		t.Fatalf("AUTH line did not carry the credentials (err %v)", err)
	}
}

func TestSMTP_Send_verifiesTheServerCertificate(t *testing.T) {
	server := newFakeServer(t, true)

	// trustServer=false: the self-signed certificate is not in the trust pool.
	err := server.mailer(t, "user@example.com", "app-password", false).Send(context.Background(), resetMessage())
	if err == nil {
		t.Fatal("Send accepted an untrusted certificate")
	}
	_, _, authLine, _ := server.snapshot()
	if authLine != "" {
		t.Fatal("credentials were sent over an unverified connection")
	}
}

func TestSMTP_Send_neverSendsCredentialsInClearText(t *testing.T) {
	server := newFakeServer(t, false) // no STARTTLS

	err := server.mailer(t, "user@example.com", "app-password", false).Send(context.Background(), resetMessage())
	if err == nil {
		t.Fatal("Send authenticated over a connection without TLS")
	}
	if _, _, authLine, _ := server.snapshot(); authLine != "" {
		t.Fatal("credentials crossed the network in clear text")
	}
	if strings.Contains(err.Error(), "app-password") {
		t.Fatal("the password leaked into the error")
	}
}

func TestSMTP_Send_serverErrors(t *testing.T) {
	t.Run("recipient rejected", func(t *testing.T) {
		server := newFakeServer(t, false)
		server.failRcpt = true
		if err := server.mailer(t, "", "", false).Send(context.Background(), resetMessage()); err == nil {
			t.Fatal("Send hid a rejected recipient")
		}
	})

	t.Run("nothing is listening", func(t *testing.T) {
		server := newFakeServer(t, false)
		sender := server.mailer(t, "", "", false)
		_ = server.listener.Close()
		if err := sender.Send(context.Background(), resetMessage()); err == nil {
			t.Fatal("Send succeeded without a server")
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		server := newFakeServer(t, false)
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := server.mailer(t, "", "", false).Send(ctx, resetMessage()); err == nil {
			t.Fatal("Send ignored a cancelled context")
		}
	})
}

func TestMessage_rejectsUnsafeInput(t *testing.T) {
	server := newFakeServer(t, false)
	sender := server.mailer(t, "", "", false)

	tests := []struct {
		name   string
		change func(*Message)
	}{
		{name: "recipient with header injection", change: func(m *Message) { m.To = "a@example.com\r\nBcc: victim@example.com" }},
		{name: "recipient with display name", change: func(m *Message) { m.To = "Evil <a@example.com>" }},
		{name: "empty recipient", change: func(m *Message) { m.To = "" }},
		{name: "subject with header injection", change: func(m *Message) { m.Subject = "Hi\r\nBcc: victim@example.com" }},
		{name: "empty subject", change: func(m *Message) { m.Subject = "  " }},
		{name: "overlong subject", change: func(m *Message) { m.Subject = strings.Repeat("a", MaxSubjectLength+1) }},
		{name: "empty body", change: func(m *Message) { m.Text = " " }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			message := resetMessage()
			test.change(&message)

			for name, mailer := range map[string]Mailer{"smtp": sender, "mock": &Mock{}} {
				if err := mailer.Send(context.Background(), message); !errors.Is(err, ErrInvalidMessage) {
					t.Fatalf("%s: Send() error = %v, want ErrInvalidMessage", name, err)
				}
			}
		})
	}
	if commands, _, _, _ := server.snapshot(); len(commands) != 0 {
		t.Fatalf("an invalid message reached the server: %v", commands)
	}
}

func TestMock(t *testing.T) {
	mock := &Mock{}
	ctx := context.Background()

	if err := mock.Send(ctx, resetMessage()); err != nil {
		t.Fatalf("Send returned error: %v", err)
	}
	sent := mock.Sent()
	if len(sent) != 1 || sent[0].To != "nimal@example.com" {
		t.Fatalf("Sent() = %+v", sent)
	}
	sent[0].To = "changed@example.com"
	if mock.Sent()[0].To != "nimal@example.com" {
		t.Fatal("Sent() exposes internal state")
	}

	outage := errors.New("smtp down")
	mock.FailWith(outage)
	if err := mock.Send(ctx, resetMessage()); !errors.Is(err, outage) {
		t.Fatalf("Send() error = %v, want the injected failure", err)
	}
	if len(mock.Sent()) != 1 {
		t.Fatal("a failed send was recorded")
	}

	mock.Reset()
	if len(mock.Sent()) != 0 || mock.Send(ctx, resetMessage()) != nil {
		t.Fatal("Reset did not clear the mock")
	}
}

func TestLoadConfig(t *testing.T) {
	lookup := func(values map[string]string) config.Loader {
		return config.NewLoader(func(key string) (string, bool) {
			value, ok := values[key]
			return value, ok
		})
	}

	t.Run("compose defaults for Mailpit", func(t *testing.T) {
		cfg, err := LoadConfig(lookup(map[string]string{"SMTP_HOST": "mailpit", "SMTP_PORT": "1025"}))
		if err != nil {
			t.Fatalf("LoadConfig returned error: %v", err)
		}
		if !cfg.Enabled() || cfg.Port != 1025 || cfg.From != defaultFrom || cfg.FromName != defaultFromName || cfg.Validate() != nil {
			t.Fatalf("config = %+v", cfg)
		}
	})

	t.Run("nothing set means disabled", func(t *testing.T) {
		cfg, err := LoadConfig(lookup(nil))
		if err != nil || cfg.Enabled() {
			t.Fatalf("config = %+v, err = %v, want disabled", cfg, err)
		}
		if cfg.Validate() == nil {
			t.Fatal("a disabled config must not validate")
		}
	})

	t.Run("bad port", func(t *testing.T) {
		if _, err := LoadConfig(lookup(map[string]string{"SMTP_PORT": "not-a-port"})); err == nil {
			t.Fatal("LoadConfig accepted a bad port")
		}
	})
}

func TestConfig_Validate(t *testing.T) {
	valid := Config{Host: "smtp.gmail.com", Port: 587, User: "me@gmail.com", Password: "pw", From: "me@gmail.com", FromName: "Swift Drop Support"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := map[string]func(*Config){
		"no host":                func(c *Config) { c.Host = "" },
		"port zero":              func(c *Config) { c.Port = 0 },
		"port too high":          func(c *Config) { c.Port = 70000 },
		"sender with name":       func(c *Config) { c.From = "Support <me@gmail.com>" },
		"sender not an address":  func(c *Config) { c.From = "nope" },
		"from name with newline": func(c *Config) { c.FromName = "Evil\r\nBcc: x@example.com" },
		"user without password":  func(c *Config) { c.Password = "" },
		"password without user":  func(c *Config) { c.User = "" },
	}
	for name, change := range tests {
		t.Run(name, func(t *testing.T) {
			cfg := valid
			change(&cfg)
			if err := cfg.Validate(); err == nil {
				t.Fatal("Validate accepted a bad config")
			}
		})
	}
	if _, err := NewSMTP(Config{}); err == nil {
		t.Fatal("NewSMTP accepted an empty config")
	}
}
