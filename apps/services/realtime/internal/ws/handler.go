// Package ws serves the realtime WebSocket endpoint (API v1.1 section 9). It
// owns the connection: authentication, the ping and idle timeout, reading
// client messages and writing server messages. What a client may ask for and
// how updates are routed live in the live package.
package ws

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"

	"github.com/coder/websocket"

	"github.com/Vineth-R/swiftdrop/apps/services/realtime/internal/live"
	"github.com/Vineth-R/swiftdrop/packages/go-common/auth"
	"github.com/Vineth-R/swiftdrop/packages/go-common/sessioncheck"
)

const (
	// sendBuffer is how many server messages may wait for one connection. A
	// connection that falls further behind is closed and its client reconnects.
	sendBuffer = 32
	// defaultWriteTimeout bounds one write to a client.
	defaultWriteTimeout = 10 * time.Second
)

// Verifier checks an access token. *auth.TokenManager satisfies it.
type Verifier interface {
	Verify(token string) (auth.Claims, error)
}

// SessionValidator confirms that verified claims still match identity. It is
// optional: without one, the connection is only checked when it opens.
type SessionValidator interface {
	Check(ctx context.Context, claims auth.Claims) error
}

var (
	errTokenInvalid       = errors.New("invalid or expired token")
	errSessionRevoked     = errors.New("session revoked")
	errSessionUnavailable = errors.New("session state unavailable")
)

// Options configures the handler.
type Options struct {
	Hub *live.Hub
	// Verifier checks access tokens. Leave it nil (an untyped nil, not a nil
	// *auth.TokenManager) when no secret is configured: every connection is
	// then anonymous and may only follow single parcels.
	Verifier Verifier
	// CookieName is the access-token cookie, read when there is no ?token=.
	CookieName string
	// SessionValidator re-checks a verified session at connect time and
	// periodically. Nil keeps the token-expiry-only behaviour.
	SessionValidator SessionValidator
	// SessionCheckInterval is how often an open authenticated connection is
	// revalidated. Defaults to one minute; non-positive disables revalidation.
	SessionCheckInterval time.Duration
	// OriginPatterns are extra browser origins (host or host:port) allowed to
	// connect. Behind the gateway the Host header is not the public host, so list
	// every host users open.
	OriginPatterns []string
	Logger         *slog.Logger

	// The three timings default to the API v1.1 values. Tests shorten them.
	PingInterval time.Duration
	IdleTimeout  time.Duration
	WriteTimeout time.Duration
}

// Handler upgrades requests to WebSocket connections and serves them.
type Handler struct {
	ctx  context.Context
	opts Options
}

// NewHandler returns the handler. When ctx ends (the service is shutting down)
// every open connection is closed with "going away".
func NewHandler(ctx context.Context, opts Options) *Handler {
	if opts.PingInterval <= 0 {
		opts.PingInterval = live.PingInterval
	}
	if opts.IdleTimeout <= 0 {
		opts.IdleTimeout = live.IdleTimeout
	}
	if opts.WriteTimeout <= 0 {
		opts.WriteTimeout = defaultWriteTimeout
	}
	if opts.SessionCheckInterval <= 0 {
		opts.SessionCheckInterval = time.Minute
	}
	if opts.Logger == nil {
		opts.Logger = slog.Default()
	}
	return &Handler{ctx: ctx, opts: opts}
}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{
		OriginPatterns:  h.opts.OriginPatterns,
		CompressionMode: websocket.CompressionDisabled,
	})
	if err != nil {
		// Accept has already answered the request, for example 403 for an
		// origin that is not allowed.
		h.opts.Logger.Debug("websocket upgrade refused", "error", err)
		return
	}
	conn.SetReadLimit(live.MaxMessageBytes)

	c := &client{
		conn: conn,
		hub:  h.opts.Hub,
		opts: h.opts,
		out:  make(chan []byte, sendBuffer),
		done: make(chan struct{}),
	}
	c.touch()

	principal, expires, claims, authErr := h.authenticate(r)
	if authErr != nil {
		// A browser cannot read the HTTP status of a failed upgrade, so the
		// answer is a close code that tells the web client to renew its session.
		code := websocket.StatusCode(live.CloseTokenInvalid)
		if errors.Is(authErr, errSessionUnavailable) {
			code = websocket.StatusTryAgainLater
		}
		c.closeWith(code, authErr.Error())
		return
	}

	defer func() {
		h.opts.Hub.Remove(c)
		c.finish()
	}()

	h.opts.Logger.Debug("websocket opened", "authenticated", principal.Authenticated)
	go c.writeLoop()
	go c.pingLoop()
	go c.watch(h.ctx, expires)
	if principal.Authenticated {
		go c.watchSession(h.ctx, claims)
	}
	c.readLoop(principal)
	h.opts.Logger.Debug("websocket closed")
}

// authenticate reads the access token from ?token= or the cookie. No token, or
// no verifier configured, means an anonymous caller. A token that is present
// but invalid or expired returns ok = false. expires is when the token ends,
// or the zero time for an anonymous caller.
func (h *Handler) authenticate(r *http.Request) (live.Principal, time.Time, auth.Claims, error) {
	token := r.URL.Query().Get("token")
	if token == "" {
		if cookie, err := r.Cookie(h.opts.CookieName); err == nil {
			token = cookie.Value
		}
	}
	if token == "" || h.opts.Verifier == nil {
		return live.Principal{}, time.Time{}, auth.Claims{}, nil
	}

	claims, err := h.opts.Verifier.Verify(token)
	if err != nil {
		return live.Principal{}, time.Time{}, auth.Claims{}, errTokenInvalid
	}
	if h.opts.SessionValidator != nil {
		if err := h.opts.SessionValidator.Check(r.Context(), claims); err != nil {
			if errors.Is(err, sessioncheck.ErrRevoked) {
				return live.Principal{}, time.Time{}, auth.Claims{}, errSessionRevoked
			}
			return live.Principal{}, time.Time{}, auth.Claims{}, errSessionUnavailable
		}
	}
	return live.Principal{
		Authenticated: true,
		CanFollowAll:  claims.Role.Satisfies(auth.RoleDispatcher),
	}, claims.ExpiresAt, claims, nil
}

// client is one open connection. It is the hub's Subscriber.
type client struct {
	conn *websocket.Conn
	hub  *live.Hub
	opts Options

	out       chan []byte
	done      chan struct{}
	closeOnce sync.Once
	lastSeen  atomic.Int64
}

// Deliver queues an update without blocking (live.Subscriber).
func (c *client) Deliver(update live.Update) bool {
	return c.send(update.Message())
}

// DeliverAssignment queues an assignment notice without blocking
// (live.Subscriber). Only dispatch clients following scope all receive these.
func (c *client) DeliverAssignment(update live.AssignmentUpdate) bool {
	return c.send(update.Message())
}

// Overflow closes a connection that cannot keep up (live.Subscriber). The web
// client reconnects and fetches the current state again.
func (c *client) Overflow() {
	// Runs on the broker consumer's goroutine through Hub.Broadcast. A polite
	// close can wait for a client that is not reading, so do it elsewhere.
	go c.closeWith(websocket.StatusTryAgainLater, "cannot keep up")
}

func (c *client) send(message []byte) bool {
	select {
	case c.out <- message:
		return true
	default:
		return false
	}
}

func (c *client) touch() {
	c.lastSeen.Store(time.Now().UnixNano())
}

func (c *client) idleFor() time.Duration {
	return time.Since(time.Unix(0, c.lastSeen.Load()))
}

// closeWith performs the closing handshake with code and reason, once. The
// reason is short text of ours, never client input.
func (c *client) closeWith(code websocket.StatusCode, reason string) {
	c.closeOnce.Do(func() {
		_ = c.conn.Close(code, reason)
		close(c.done)
	})
}

// finish ends the connection without a handshake, once. The reader calls it
// when the connection has already gone.
func (c *client) finish() {
	c.closeOnce.Do(func() {
		_ = c.conn.CloseNow()
		close(c.done)
	})
}

// readLoop handles client messages until the connection ends.
func (c *client) readLoop(principal live.Principal) {
	var limiter live.MessageLimiter
	for {
		messageType, data, err := c.conn.Read(context.Background())
		if err != nil {
			return
		}
		c.touch()

		if !limiter.Allow(time.Now()) {
			c.closeWith(websocket.StatusCode(live.CloseTooMany), "too many messages")
			return
		}
		if messageType != websocket.MessageText {
			if !c.reject() {
				return
			}
			continue
		}

		message, err := live.ParseClientMessage(data)
		if err != nil {
			if !c.reject() {
				return
			}
			continue
		}
		if !c.handle(principal, message) {
			return
		}
	}
}

// reject answers a message that is not understood and keeps the connection.
func (c *client) reject() bool {
	if !c.send(live.ErrorMessage("INVALID_MESSAGE", "Message not understood.")) {
		c.closeWith(websocket.StatusTryAgainLater, "cannot keep up")
		return false
	}
	return true
}

// handle carries out one valid message. It returns false when the connection
// has been closed.
func (c *client) handle(principal live.Principal, message live.ClientMessage) bool {
	if message.Action == live.ActionPing {
		// Any message counts as a sign of life; nothing to answer.
		return true
	}

	if err := live.Authorize(principal, message); err != nil {
		if errors.Is(err, live.ErrUnauthenticated) {
			c.closeWith(websocket.StatusCode(live.CloseTokenInvalid), "authentication required")
			return false
		}
		c.writeNow(live.ErrorMessage("FORBIDDEN", "Not allowed to subscribe to this scope."))
		c.closeWith(websocket.StatusCode(live.CloseForbidden), "not allowed")
		return false
	}

	if err := c.hub.Apply(c, message); err != nil {
		c.closeWith(websocket.StatusCode(live.CloseTooMany), "too many subscriptions")
		return false
	}

	if message.Action == live.ActionSubscribe {
		// Sent after the subscription is in place and through the same queue as
		// updates, so the client fetches the current state only once nothing can
		// be missed and sees messages in order.
		if !c.send(live.SubscribedMessage(message)) {
			c.closeWith(websocket.StatusTryAgainLater, "cannot keep up")
			return false
		}
	}
	return true
}

// writeNow writes one message directly, for the moment just before a close.
func (c *client) writeNow(message []byte) {
	ctx, cancel := context.WithTimeout(context.Background(), c.opts.WriteTimeout)
	defer cancel()
	_ = c.conn.Write(ctx, websocket.MessageText, message)
}

// writeLoop sends queued messages in order.
func (c *client) writeLoop() {
	for {
		select {
		case <-c.done:
			return
		case message := <-c.out:
			ctx, cancel := context.WithTimeout(context.Background(), c.opts.WriteTimeout)
			err := c.conn.Write(ctx, websocket.MessageText, message)
			cancel()
			if err != nil {
				c.finish()
				return
			}
		}
	}
}

// pingLoop pings every PingInterval. A pong, like any client message, is a sign
// of life. A connection with no sign of life for IdleTimeout is closed.
func (c *client) pingLoop() {
	ticker := time.NewTicker(c.opts.PingInterval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-ticker.C:
		}

		ctx, cancel := context.WithTimeout(context.Background(), c.opts.PingInterval)
		err := c.conn.Ping(ctx)
		cancel()
		if err == nil {
			c.touch()
			continue
		}
		if c.idleFor() >= c.opts.IdleTimeout {
			// The peer has gone quiet, so a closing handshake would only wait.
			c.finish()
			return
		}
	}
}

// watch closes the connection when the service shuts down or the access token
// ends. Closing at token expiry stops a dispatcher's connection from going on
// receiving everything after the session is over; the web client renews its
// session and reconnects.
func (c *client) watch(shutdown context.Context, expires time.Time) {
	var expired <-chan time.Time
	if !expires.IsZero() {
		timer := time.NewTimer(time.Until(expires))
		defer timer.Stop()
		expired = timer.C
	}

	select {
	case <-c.done:
	case <-shutdown.Done():
		c.closeWith(websocket.StatusGoingAway, "server shutting down")
	case <-expired:
		c.closeWith(websocket.StatusCode(live.CloseTokenInvalid), "token expired")
	}
}

// watchSession revalidates an authenticated connection so a logout or
// suspension closes an already-open socket. A transient identity failure does
// not drop the connection; it is retried on the next tick, while new
// connections fail closed. The access token's own expiry remains the final
// bound.
func (c *client) watchSession(shutdown context.Context, claims auth.Claims) {
	interval := c.opts.SessionCheckInterval
	if c.opts.SessionValidator == nil || interval <= 0 {
		return
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-c.done:
			return
		case <-shutdown.Done():
			return
		case <-ticker.C:
		}

		timeout := interval
		if timeout > 5*time.Second {
			timeout = 5 * time.Second
		}
		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		err := c.opts.SessionValidator.Check(ctx, claims)
		cancel()
		switch {
		case err == nil:
		case errors.Is(err, sessioncheck.ErrRevoked):
			c.closeWith(websocket.StatusCode(live.CloseTokenInvalid), "session revoked")
			return
		default:
			c.opts.Logger.Debug("websocket session revalidation failed; keeping connection until next check", "reason", sessioncheck.Describe(err))
		}
	}
}
