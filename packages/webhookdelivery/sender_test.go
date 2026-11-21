package webhookdelivery

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/webhookurl"
)

const testSecret = "whsec_test_secret"

// newTestSender points the sender at a local fake merchant. A TLS test server
// listens on 127.0.0.1, which the production URL check and DialControl would
// refuse, so these tests swap both out. Blocking is tested separately, with
// the real NewSender, in TestProductionSenderRefusesLoopback.
func newTestSender(srv *httptest.Server, fixedNow time.Time) *Sender {
	client := srv.Client()
	client.Timeout = 2 * time.Second
	client.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	return newSender(client, func(string) error { return nil }, func() time.Time { return fixedNow })
}

func testEvent() Event {
	return SampleEvent("11111111-2222-4333-8444-555555555555", time.Date(2026, 11, 4, 13, 0, 0, 0, time.UTC))
}

// The merchant must be able to verify every request exactly as documented:
// raw body, timestamp header, signature header.
func TestSendSignsAndSendsRequiredHeaders(t *testing.T) {
	now := time.Unix(1762000000, 0)
	var gotBody []byte
	var gotHeader http.Header
	var gotMethod string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotMethod, gotHeader = r.Method, r.Header.Clone()
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	ev := testEvent()
	res := newTestSender(srv, now).Send(context.Background(), Target{URL: srv.URL + "/hook", Secret: testSecret}, ev)

	if !res.Success || res.StatusCode != http.StatusNoContent || res.ErrorMessage != "" {
		t.Fatalf("Result = %+v; want success with status 204", res)
	}
	if gotMethod != http.MethodPost {
		t.Errorf("method = %s, want POST", gotMethod)
	}
	want := map[string]string{
		"Content-Type":          "application/json",
		"X-Swiftdrop-Event":     "parcel.status_changed",
		"X-Swiftdrop-Event-Id":  ev.ID,
		"X-Swiftdrop-Timestamp": "1762000000",
	}
	for k, v := range want {
		if gotHeader.Get(k) != v {
			t.Errorf("header %s = %q, want %q", k, gotHeader.Get(k), v)
		}
	}
	tsHeader := gotHeader.Get("X-SwiftDrop-Timestamp")
	if _, err := strconv.ParseInt(tsHeader, 10, 64); err != nil {
		t.Fatalf("timestamp header is not Unix seconds: %v", err)
	}
	if !VerifyAt(testSecret, tsHeader, gotBody, gotHeader.Get("X-SwiftDrop-Signature"), now) {
		t.Fatalf("signature %q does not verify against the received raw body", gotHeader.Get("X-SwiftDrop-Signature"))
	}
	if VerifyAt("wrong-secret", tsHeader, gotBody, gotHeader.Get("X-SwiftDrop-Signature"), now) {
		t.Fatal("signature verified with the wrong secret")
	}
	wantBody, _ := ev.Body()
	if string(gotBody) != string(wantBody) {
		t.Errorf("body = %s, want %s", gotBody, wantBody)
	}
	if !strings.Contains(string(gotBody), `"schemaVersion":1`) {
		t.Errorf("body has no schemaVersion 1: %s", gotBody)
	}
}

// Section 10.2: any 2xx is success, anything else is a failure.
func TestSendStatusCodes(t *testing.T) {
	tests := []struct {
		status  int
		success bool
	}{
		{200, true}, {201, true}, {204, true}, {299, true},
		{301, false}, {302, false}, {400, false}, {401, false},
		{404, false}, {410, false}, {429, false}, {500, false}, {503, false},
	}
	for _, tc := range tests {
		t.Run(strconv.Itoa(tc.status), func(t *testing.T) {
			srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
			}))
			defer srv.Close()
			res := newTestSender(srv, time.Now()).Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())
			if res.Success != tc.success {
				t.Fatalf("status %d: Success = %v, want %v (%+v)", tc.status, res.Success, tc.success, res)
			}
			if !tc.success && res.ErrorMessage == "" {
				t.Error("a failed attempt must carry an error message")
			}
			if tc.success && res.ErrorMessage != "" {
				t.Errorf("successful attempt has error message %q", res.ErrorMessage)
			}
		})
	}
}

// A redirect is not followed: it could lead to an internal address, and a
// merchant's URL must answer directly.
func TestSendDoesNotFollowRedirects(t *testing.T) {
	var targetHits atomic.Int32
	target := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { targetHits.Add(1) }))
	defer target.Close()
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, target.URL, http.StatusFound)
	}))
	defer srv.Close()

	res := newTestSender(srv, time.Now()).Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != http.StatusFound {
		t.Fatalf("Result = %+v; want failure with status 302", res)
	}
	if targetHits.Load() != 0 {
		t.Fatal("the redirect was followed")
	}
}

func TestSendTimeoutRecordsNoStatus(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)

	s := newTestSender(srv, time.Now())
	s.client.Timeout = 150 * time.Millisecond
	start := time.Now()
	res := s.Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())

	if res.Success || res.StatusCode != 0 {
		t.Fatalf("Result = %+v; want failure with no status (http_status NULL)", res)
	}
	if res.ErrorMessage != errTimeout {
		t.Errorf("ErrorMessage = %q, want %q", res.ErrorMessage, errTimeout)
	}
	if time.Since(start) > 2*time.Second {
		t.Error("the timeout did not stop the request promptly")
	}
}

func TestSendCancelledContext(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	res := newTestSender(srv, time.Now()).Send(ctx, Target{URL: srv.URL, Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != 0 {
		t.Fatalf("Result = %+v; want failure with no status", res)
	}
	if res.ErrorMessage != errCancelled {
		t.Errorf("ErrorMessage = %q, want %q", res.ErrorMessage, errCancelled)
	}
}

func TestSendConnectionRefused(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	s := newTestSender(srv, time.Now())
	srv.Close() // nothing is listening any more
	res := s.Send(context.Background(), Target{URL: url, Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != 0 || res.ErrorMessage != errConnect {
		t.Fatalf("Result = %+v; want failure %q", res, errConnect)
	}
}

// An untrusted certificate must fail, not be skipped.
func TestSendRejectsUntrustedCertificate(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	defer srv.Close()
	s := newTestSender(srv, time.Now())
	// Default roots: does not trust the test CA. Use the production timeout so
	// a loaded CI machine cannot turn the expected TLS error into a timeout.
	s.client = &http.Client{Timeout: RequestTimeout}
	res := s.Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())
	if res.Success || res.ErrorMessage != errTLS {
		t.Fatalf("Result = %+v; want failure %q", res, errTLS)
	}
}

// Classification is deterministic and independent of network timing.
func TestClassify(t *testing.T) {
	certErr := &tls.CertificateVerificationError{Err: errors.New("x509: certificate signed by unknown authority")}
	tests := []struct {
		name string
		err  error
		want string
	}{
		{"cancelled", context.Canceled, errCancelled},
		{"deadline", context.DeadlineExceeded, errTimeout},
		{"blocked address", fmt.Errorf("dial: %w", webhookurl.ErrBlockedIP), errAddrNotAllowed},
		{"untrusted certificate", fmt.Errorf("request: %w", certErr), errTLS},
		{"connection failure", errors.New("connection refused"), errConnect},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := classify(tc.err); got != tc.want {
				t.Fatalf("classify(%v) = %q, want %q", tc.err, got, tc.want)
			}
		})
	}
}

// A URL that fails the safety check is never contacted.
func TestSendRefusesUnsafeURLWithoutRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	s := newTestSender(srv, time.Now())
	s.checkURL = func(string) error { return http.ErrNotSupported }

	res := s.Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != 0 || res.ErrorMessage != errURLNotAllowed {
		t.Fatalf("Result = %+v; want failure %q", res, errURLNotAllowed)
	}
	if hits.Load() != 0 {
		t.Fatal("the server was contacted")
	}
}

// With the real NewSender, a loopback server must be unreachable. This is the
// end-to-end proof that DialControl is wired into the HTTP client.
func TestProductionSenderRefusesLoopback(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()

	s := NewSender()
	s.checkURL = func(string) error { return nil } // skip the text check so only DialControl is tested
	res := s.Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())

	if res.Success || res.StatusCode != 0 || res.ErrorMessage != errAddrNotAllowed {
		t.Fatalf("Result = %+v; want failure %q", res, errAddrNotAllowed)
	}
	if hits.Load() != 0 {
		t.Fatal("the loopback server was contacted")
	}
}

func TestProductionSenderChecksURLText(t *testing.T) {
	s := NewSender()
	for _, url := range []string{"http://shop.example.com/h", "https://localhost/h", "https://169.254.169.254/x", "https://postgres/h"} {
		res := s.Send(context.Background(), Target{URL: url, Secret: testSecret}, testEvent())
		if res.Success || res.ErrorMessage != errURLNotAllowed {
			t.Errorf("%s: Result = %+v; want failure %q", url, res, errURLNotAllowed)
		}
	}
}

func TestNewSenderSettings(t *testing.T) {
	s := NewSender()
	if RequestTimeout != 10*time.Second {
		t.Errorf("RequestTimeout = %v, want 10s", RequestTimeout)
	}
	if s.client.Timeout != RequestTimeout {
		t.Errorf("timeout = %v, want %v", s.client.Timeout, RequestTimeout)
	}
	if s.client.CheckRedirect == nil {
		t.Error("redirects must be disabled")
	}
	tr, ok := s.client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("transport = %T, want *http.Transport", s.client.Transport)
	}
	if tr.Proxy != nil {
		t.Error("the transport must not use an HTTP proxy")
	}
	if tr.TLSClientConfig == nil {
		t.Fatal("TLS config must be set")
	}
	if tr.TLSClientConfig.MinVersion != tls.VersionTLS12 {
		t.Errorf("MinVersion = %x, want TLS 1.2", tr.TLSClientConfig.MinVersion)
	}
	if tr.TLSClientConfig.InsecureSkipVerify {
		t.Error("certificate verification must stay enabled")
	}
}

// An uninitialised Sender must report a safe failure, not panic on a nil
// client.
func TestZeroValueSenderFailsSafely(t *testing.T) {
	var s Sender
	res := s.Send(context.Background(), Target{URL: "https://shop.example.com/hook", Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != 0 || res.ErrorMessage != errNotConfigured {
		t.Fatalf("Result = %+v; want failure %q", res, errNotConfigured)
	}

	var nilSender *Sender
	res = nilSender.Send(context.Background(), Target{URL: "https://shop.example.com/hook", Secret: testSecret}, testEvent())
	if res.Success || res.StatusCode != 0 || res.ErrorMessage != errNotConfigured {
		t.Fatalf("nil Sender Result = %+v; want failure %q", res, errNotConfigured)
	}
}

// roundTripFunc lets a test return a response without a network call.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// trackingBody records how much was read and whether it was closed.
type trackingBody struct {
	remaining int
	read      int
	closed    bool
}

func (b *trackingBody) Read(p []byte) (int, error) {
	if b.remaining == 0 {
		return 0, io.EOF
	}
	n := len(p)
	if n > b.remaining {
		n = b.remaining
	}
	b.read += n
	b.remaining -= n
	return n, nil
}

func (b *trackingBody) Close() error {
	b.closed = true
	return nil
}

// The merchant response body is bounded, drained and always closed.
func TestSendBoundsAndClosesResponseBody(t *testing.T) {
	body := &trackingBody{remaining: 1 << 20}
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       body,
			Header:     make(http.Header),
		}, nil
	})}
	s := newSender(client, func(string) error { return nil }, time.Now)

	res := s.Send(context.Background(), Target{URL: "https://shop.example.com/hook", Secret: testSecret}, testEvent())
	if !res.Success || res.StatusCode != http.StatusOK {
		t.Fatalf("Result = %+v; want success", res)
	}
	if !body.closed {
		t.Fatal("the response body was not closed")
	}
	if body.read > maxResponseDrain {
		t.Fatalf("read %d bytes; want at most %d", body.read, maxResponseDrain)
	}
}

// Stored error messages must not leak the URL (it may hold a token), the
// signing secret, or anything the merchant's server replied.
func TestResultNeverLeaksSecretsOrURL(t *testing.T) {
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal stack trace with " + testSecret))
	}))
	defer srv.Close()

	url := srv.URL + "/hook?token=URLTOKEN123"
	res := newTestSender(srv, time.Now()).Send(context.Background(), Target{URL: url, Secret: testSecret}, testEvent())
	for _, leaked := range []string{"URLTOKEN123", testSecret, "stack trace", "127.0.0.1"} {
		if strings.Contains(res.ErrorMessage, leaked) {
			t.Errorf("ErrorMessage %q leaks %q", res.ErrorMessage, leaked)
		}
	}
	if len(res.ErrorMessage) > 255 {
		t.Errorf("ErrorMessage is %d chars; the column holds 255", len(res.ErrorMessage))
	}
}
