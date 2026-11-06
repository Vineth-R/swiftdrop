package delivery

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
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
	return &Sender{
		Client:   client,
		CheckURL: func(string) error { return nil },
		Now:      func() time.Time { return fixedNow },
	}
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
	ts, err := strconv.ParseInt(gotHeader.Get("X-SwiftDrop-Timestamp"), 10, 64)
	if err != nil {
		t.Fatalf("timestamp header is not Unix seconds: %v", err)
	}
	if !Verify(testSecret, ts, gotBody, gotHeader.Get("X-SwiftDrop-Signature")) {
		t.Fatalf("signature %q does not verify against the received raw body", gotHeader.Get("X-SwiftDrop-Signature"))
	}
	if Verify("wrong-secret", ts, gotBody, gotHeader.Get("X-SwiftDrop-Signature")) {
		t.Fatal("signature verified with the wrong secret")
	}
	wantBody, _ := ev.Body()
	if string(gotBody) != string(wantBody) {
		t.Errorf("body = %s, want %s", gotBody, wantBody)
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
	s.Client.Timeout = 150 * time.Millisecond
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
	s.Client = &http.Client{Timeout: 2 * time.Second} // default roots: does not trust the test CA
	res := s.Send(context.Background(), Target{URL: srv.URL, Secret: testSecret}, testEvent())
	if res.Success || res.ErrorMessage != errTLS {
		t.Fatalf("Result = %+v; want failure %q", res, errTLS)
	}
}

// A URL that fails the safety check is never contacted.
func TestSendRefusesUnsafeURLWithoutRequest(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewTLSServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { hits.Add(1) }))
	defer srv.Close()
	s := newTestSender(srv, time.Now())
	s.CheckURL = func(string) error { return http.ErrNotSupported }

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
	s.CheckURL = func(string) error { return nil } // skip the text check so only DialControl is tested
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
	if s.Client.Timeout != 10*time.Second {
		t.Errorf("timeout = %v, want 10s", s.Client.Timeout)
	}
	if s.Client.CheckRedirect == nil {
		t.Error("redirects must be disabled")
	}
	tr, ok := s.Client.Transport.(*http.Transport)
	if !ok || tr.Proxy != nil {
		t.Error("the transport must not use an HTTP proxy")
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
