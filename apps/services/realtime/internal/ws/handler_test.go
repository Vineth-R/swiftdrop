package ws

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

const (
	trackingNumber = "SDK7M2Q9XP4"
	cookieName     = "sd_access"
)

// newTestSigningKey returns a fresh HS256 signing key for one test. It is
// generated per test so no credential-like value is stored in the repository.
func newTestSigningKey(t *testing.T) string {
	t.Helper()
	key := make([]byte, auth.MinSecretBytes)
	if _, err := rand.Read(key); err != nil {
		t.Fatalf("generate test signing key: %v", err)
	}
	return hex.EncodeToString(key)
}

type testServer struct {
	*httptest.Server
	hub     *live.Hub
	manager *auth.TokenManager
	stop    context.CancelFunc
}

func newTestServer(t *testing.T, edit func(*Options)) *testServer {
	t.Helper()
	manager, err := auth.NewTokenManager(newTestSigningKey(t), time.Minute)
	if err != nil {
		t.Fatalf("NewTokenManager() = %v", err)
	}

	hub := live.NewHub()
	options := Options{
		Hub:        hub,
		Verifier:   manager,
		CookieName: cookieName,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}
	if edit != nil {
		edit(&options)
	}

	ctx, stop := context.WithCancel(context.Background())
	mux := http.NewServeMux()
	mux.Handle("/ws", NewHandler(ctx, options))
	server := httptest.NewServer(mux)
	t.Cleanup(func() {
		stop()
		server.Close()
	})
	return &testServer{Server: server, hub: hub, manager: manager, stop: stop}
}

func (s *testServer) token(t *testing.T, role auth.Role) string {
	t.Helper()
	token, _, err := s.manager.Issue("11111111-1111-4111-8111-111111111111", role)
	if err != nil {
		t.Fatalf("Issue() = %v", err)
	}
	return token
}

func (s *testServer) url(query string) string {
	return "ws" + strings.TrimPrefix(s.URL, "http") + "/ws" + query
}

func (s *testServer) dial(t *testing.T, query string, header http.Header) *websocket.Conn {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(ctx, s.url(query), &websocket.DialOptions{HTTPHeader: header})
	if err != nil {
		t.Fatalf("Dial() = %v", err)
	}
	t.Cleanup(func() { _ = conn.CloseNow() })
	return conn
}

func send(t *testing.T, conn *websocket.Conn, message string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := conn.Write(ctx, websocket.MessageText, []byte(message)); err != nil {
		t.Fatalf("Write() = %v", err)
	}
}

func read(t *testing.T, conn *websocket.Conn) map[string]any {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	_, data, err := conn.Read(ctx)
	if err != nil {
		t.Fatalf("Read() = %v", err)
	}
	var message map[string]any
	if err := json.Unmarshal(data, &message); err != nil {
		t.Fatalf("not JSON: %s", data)
	}
	return message
}

// expectClose reads until the connection closes and checks the close code.
func expectClose(t *testing.T, conn *websocket.Conn, want websocket.StatusCode, wait time.Duration) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), wait)
	defer cancel()
	for {
		if _, _, err := conn.Read(ctx); err != nil {
			if got := websocket.CloseStatus(err); got != want {
				t.Fatalf("close code = %d (%v), want %d", got, err, want)
			}
			return
		}
	}
}

func subscribe(t *testing.T, conn *websocket.Conn, number string) {
	t.Helper()
	send(t, conn, fmt.Sprintf(`{"type":"subscribe","trackingNumber":%q}`, number))
	got := read(t, conn)
	want := map[string]any{"type": "subscribed", "trackingNumber": number}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("subscribed = %v, want %v", got, want)
	}
}

var statusUpdate = live.Update{
	TrackingNumber: trackingNumber,
	PreviousStatus: "IN_TRANSIT",
	Status:         "OUT_FOR_DELIVERY",
	UpdatedAt:      time.Date(2026, 11, 2, 15, 40, 0, 0, time.UTC),
}

func wantUpdate() map[string]any {
	return map[string]any{
		"type": "parcel.status.updated",
		"data": map[string]any{
			"trackingNumber": trackingNumber,
			"previousStatus": "IN_TRANSIT",
			"status":         "OUT_FOR_DELIVERY",
			"updatedAt":      "2026-11-02T15:40:00Z",
		},
	}
}

func TestAnonymousVisitorFollowsOneParcel(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)

	subscribe(t, conn, trackingNumber)
	server.hub.Broadcast(statusUpdate)
	if got := read(t, conn); !reflect.DeepEqual(got, wantUpdate()) {
		t.Fatalf("update = %v, want %v", got, wantUpdate())
	}
}

func TestUpdateForAnotherParcelIsNotSent(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)
	subscribe(t, conn, trackingNumber)

	other := statusUpdate
	other.TrackingNumber = "SDN4T8W2LC6"
	server.hub.Broadcast(other)
	server.hub.Broadcast(statusUpdate)

	// The first message the client sees is for its own parcel.
	if got := read(t, conn); !reflect.DeepEqual(got, wantUpdate()) {
		t.Fatalf("update = %v, want %v", got, wantUpdate())
	}
}

func TestAnonymousVisitorCannotFollowEverything(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)

	send(t, conn, `{"type":"subscribe","scope":"all"}`)
	// 4401 tells the web client to renew its session and try again.
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 3*time.Second)
}

func TestCustomerCannotFollowEverything(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "?token="+server.token(t, auth.RoleCustomer), nil)

	send(t, conn, `{"type":"subscribe","scope":"all"}`)
	got := read(t, conn)
	if got["type"] != "error" || got["code"] != "FORBIDDEN" {
		t.Fatalf("message = %v, want a FORBIDDEN error", got)
	}
	expectClose(t, conn, websocket.StatusCode(live.CloseForbidden), 3*time.Second)
}

func TestDispatcherAndAdminFollowEverything(t *testing.T) {
	for _, role := range []auth.Role{auth.RoleDispatcher, auth.RoleAdmin} {
		t.Run(string(role), func(t *testing.T) {
			server := newTestServer(t, nil)
			conn := server.dial(t, "?token="+server.token(t, role), nil)

			send(t, conn, `{"type":"subscribe","scope":"all"}`)
			if got := read(t, conn); !reflect.DeepEqual(got, map[string]any{"type": "subscribed", "scope": "all"}) {
				t.Fatalf("subscribed = %v", got)
			}
			server.hub.Broadcast(statusUpdate)
			if got := read(t, conn); !reflect.DeepEqual(got, wantUpdate()) {
				t.Fatalf("update = %v, want %v", got, wantUpdate())
			}
		})
	}
}

func TestTokenFromTheCookie(t *testing.T) {
	server := newTestServer(t, nil)
	header := http.Header{"Cookie": {cookieName + "=" + server.token(t, auth.RoleDispatcher)}}
	conn := server.dial(t, "", header)

	send(t, conn, `{"type":"subscribe","scope":"all"}`)
	if got := read(t, conn); got["type"] != "subscribed" {
		t.Fatalf("message = %v, want subscribed", got)
	}
}

type sessionValidatorFunc func(context.Context, auth.Claims) error

func (f sessionValidatorFunc) Check(ctx context.Context, claims auth.Claims) error {
	return f(ctx, claims)
}

func TestRevokedSessionIsRefusedAtConnect(t *testing.T) {
	server := newTestServer(t, func(options *Options) {
		options.SessionValidator = sessionValidatorFunc(func(context.Context, auth.Claims) error {
			return sessioncheck.ErrRevoked
		})
	})
	conn := server.dial(t, "?token="+server.token(t, auth.RoleDispatcher), nil)
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 3*time.Second)
}

func TestUnavailableSessionCheckFailsClosedAtConnect(t *testing.T) {
	server := newTestServer(t, func(options *Options) {
		options.SessionValidator = sessionValidatorFunc(func(context.Context, auth.Claims) error {
			return sessioncheck.ErrUnavailable
		})
	})
	conn := server.dial(t, "?token="+server.token(t, auth.RoleDispatcher), nil)
	expectClose(t, conn, websocket.StatusTryAgainLater, 3*time.Second)
}

func TestCurrentSessionConnects(t *testing.T) {
	server := newTestServer(t, func(options *Options) {
		options.SessionValidator = sessionValidatorFunc(func(context.Context, auth.Claims) error { return nil })
	})
	conn := server.dial(t, "?token="+server.token(t, auth.RoleDispatcher), nil)
	subscribe(t, conn, trackingNumber)
}

func TestOpenConnectionIsClosedWhenSessionIsRevoked(t *testing.T) {
	var calls int
	server := newTestServer(t, func(options *Options) {
		options.SessionCheckInterval = 50 * time.Millisecond
		options.SessionValidator = sessionValidatorFunc(func(context.Context, auth.Claims) error {
			calls++
			if calls == 1 {
				return nil
			}
			return sessioncheck.ErrRevoked
		})
	})
	conn := server.dial(t, "?token="+server.token(t, auth.RoleCustomer), nil)
	subscribe(t, conn, trackingNumber)
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 3*time.Second)
}

func TestInvalidTokenIsClosedWith4401(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "?token=not-a-token", nil)
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 3*time.Second)
}

func TestWithoutAVerifierEveryoneIsAnonymous(t *testing.T) {
	server := newTestServer(t, func(options *Options) { options.Verifier = nil })
	token := server.token(t, auth.RoleDispatcher)
	conn := server.dial(t, "?token="+token, nil)

	subscribe(t, conn, trackingNumber)
	send(t, conn, `{"type":"subscribe","scope":"all"}`)
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 3*time.Second)
}

func TestTokenEndsTheConnection(t *testing.T) {
	short, err := auth.NewTokenManager(newTestSigningKey(t), 2*time.Second)
	if err != nil {
		t.Fatalf("NewTokenManager() = %v", err)
	}
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	mux := http.NewServeMux()
	mux.Handle("/ws", NewHandler(ctx, Options{
		Hub:        live.NewHub(),
		Verifier:   short,
		CookieName: cookieName,
		Logger:     slog.New(slog.NewTextHandler(io.Discard, nil)),
	}))
	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	token, _, err := short.Issue("11111111-1111-4111-8111-111111111111", auth.RoleDispatcher)
	if err != nil {
		t.Fatalf("Issue() = %v", err)
	}
	dialCtx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	conn, _, err := websocket.Dial(dialCtx, "ws"+strings.TrimPrefix(httpServer.URL, "http")+"/ws?token="+token, nil)
	if err != nil {
		t.Fatalf("Dial() = %v", err)
	}
	defer conn.CloseNow()

	// The token lasts about two seconds; the server then asks the client to
	// renew its session.
	expectClose(t, conn, websocket.StatusCode(live.CloseTokenInvalid), 6*time.Second)
}

func TestSubscriptionLimit(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)

	for i := 0; i < live.MaxSubscriptions; i++ {
		subscribe(t, conn, fmt.Sprintf("SDK%08d", i))
	}
	send(t, conn, `{"type":"subscribe","trackingNumber":"SDKZZZZZZZZ"}`)
	expectClose(t, conn, websocket.StatusCode(live.CloseTooMany), 3*time.Second)
}

func TestMessageThatIsNotUnderstoodKeepsTheConnection(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)

	send(t, conn, `not json`)
	if got := read(t, conn); got["type"] != "error" || got["code"] != "INVALID_MESSAGE" {
		t.Fatalf("message = %v, want an INVALID_MESSAGE error", got)
	}
	send(t, conn, `{"type":"subscribe","trackingNumber":"nope"}`)
	if got := read(t, conn); got["code"] != "INVALID_MESSAGE" {
		t.Fatalf("message = %v, want an INVALID_MESSAGE error", got)
	}

	subscribe(t, conn, trackingNumber)
}

func TestPingKeepsTheConnectionAndGetsNoReply(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)

	send(t, conn, `{"type":"ping"}`)
	subscribe(t, conn, trackingNumber) // the next message is subscribed, not a pong
}

func TestUnsubscribeStopsUpdates(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)
	subscribe(t, conn, trackingNumber)

	send(t, conn, fmt.Sprintf(`{"type":"unsubscribe","trackingNumber":%q}`, trackingNumber))
	// Subscribing to a second parcel proves the unsubscribe was processed
	// first; the update for the first parcel is then not delivered.
	other := "SDN4T8W2LC6"
	subscribe(t, conn, other)
	server.hub.Broadcast(statusUpdate)
	next := statusUpdate
	next.TrackingNumber = other
	server.hub.Broadcast(next)

	got := read(t, conn)
	data, _ := got["data"].(map[string]any)
	if data["trackingNumber"] != other {
		t.Fatalf("update = %v, want the one for %s", got, other)
	}
}

func TestBrowserOrigins(t *testing.T) {
	server := newTestServer(t, func(options *Options) {
		options.OriginPatterns = []string{"localhost:3000"}
	})

	dialWith := func(origin string) error {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer cancel()
		conn, _, err := websocket.Dial(ctx, server.url(""), &websocket.DialOptions{
			HTTPHeader: http.Header{"Origin": {origin}},
		})
		if err == nil {
			_ = conn.CloseNow()
		}
		return err
	}

	if err := dialWith("http://localhost:3000"); err != nil {
		t.Fatalf("an allowed origin was refused: %v", err)
	}
	if err := dialWith("https://evil.example"); err == nil {
		t.Fatal("an origin that is not allowed was accepted")
	}
}

func TestShutdownClosesConnections(t *testing.T) {
	server := newTestServer(t, nil)
	conn := server.dial(t, "", nil)
	subscribe(t, conn, trackingNumber)

	server.stop()
	expectClose(t, conn, websocket.StatusGoingAway, 3*time.Second)
}

func TestDeliverDoesNotBlockWhenTheQueueIsFull(t *testing.T) {
	c := &client{out: make(chan []byte, 2)}
	if !c.Deliver(statusUpdate) || !c.Deliver(statusUpdate) {
		t.Fatal("Deliver() should queue while there is room")
	}
	if c.Deliver(statusUpdate) {
		t.Fatal("Deliver() should report false when the queue is full, not block")
	}
}

func TestSilentClientIsDroppedByTheIdleTimeout(t *testing.T) {
	server := newTestServer(t, func(options *Options) {
		options.PingInterval = 30 * time.Millisecond
		options.IdleTimeout = 100 * time.Millisecond
	})
	conn := server.dial(t, "", nil)
	send(t, conn, fmt.Sprintf(`{"type":"subscribe","trackingNumber":%q}`, trackingNumber))
	// This client never reads again, so it never answers a ping.
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if server.hub.Connections() == 0 {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("a silent client was not dropped by the idle timeout")
}
