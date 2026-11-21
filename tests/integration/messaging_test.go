//go:build integration

package integration

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// Reconnect behavior is intentionally not exercised here. Forcing a broker-side
// disconnect without restarting RabbitMQ or using management API privileges
// would risk disrupting shared broker resources. Bounded backoff, jitter,
// backoff reset, and context-canceled reconnect dials/waits are covered by
// deterministic unit tests in packages/go-common/messaging.

const (
	retryCountHeader    = "x-swiftdrop-retry-count"
	failureReasonHeader = "x-swiftdrop-failure-reason"
)

type handlerFunc func(context.Context, messaging.Message) error

func (f handlerFunc) Handle(ctx context.Context, message messaging.Message) error {
	return f(ctx, message)
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

type fixture struct {
	url           string
	conn          *amqp.Connection
	channel       *amqp.Channel
	exchange      string
	queue         string
	routingKey    string
	retryExchange string
	retryQueue    string
	dlx           string
	dlq           string
}

func newFixture(t *testing.T) *fixture {
	t.Helper()

	url := os.Getenv("RABBITMQ_URL")
	if url == "" {
		t.Skip("RABBITMQ_URL is not set")
	}

	conn, err := amqp.Dial(url)
	if err != nil {
		t.Fatalf("connect to rabbitmq: %v", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		conn.Close()
		t.Fatalf("open rabbitmq channel: %v", err)
	}

	base := fmt.Sprintf("swiftdrop.test.%d", time.Now().UnixNano())
	f := &fixture{
		url:           url,
		conn:          conn,
		channel:       channel,
		exchange:      base + ".events",
		queue:         base + ".queue",
		routingKey:    base + ".event.v1",
		retryExchange: base + ".events.retry",
		retryQueue:    base + ".queue.retry.v2",
		dlx:           base + ".events.dlx",
		dlq:           base + ".queue.dlq",
	}

	if err := channel.ExchangeDeclare(f.exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("declare main exchange: %v", err)
	}
	if _, err := channel.QueueDeclare(f.queue, true, false, false, false, nil); err != nil {
		t.Fatalf("declare main queue: %v", err)
	}
	if err := channel.QueueBind(f.queue, f.routingKey, f.exchange, false, nil); err != nil {
		t.Fatalf("bind main queue: %v", err)
	}

	t.Cleanup(f.cleanup)
	return f
}

func (f *fixture) cleanup() {
	for _, queue := range []string{f.queue, f.retryQueue, f.dlq} {
		_, _ = f.channel.QueueDelete(queue, false, false, false)
	}
	for _, exchange := range []string{f.retryExchange, f.dlx, f.exchange} {
		_ = f.channel.ExchangeDelete(exchange, false, false)
	}
	_ = f.channel.Close()
	_ = f.conn.Close()
}

func (f *fixture) runConsumer(t *testing.T, handler messaging.Handler, maxRetries int, retryDelay time.Duration) func() {
	t.Helper()

	logs := &lockedBuffer{}
	logger := slog.New(slog.NewTextHandler(logs, &slog.HandlerOptions{Level: slog.LevelDebug}))

	ctx, cancel := context.WithCancel(context.Background())
	stopped := make(chan struct{})

	go func() {
		defer close(stopped)
		_ = messaging.Run(ctx, messaging.ConsumerConfig{
			URL:            f.url,
			Exchange:       f.exchange,
			Queue:          f.queue,
			RoutingKey:     f.routingKey,
			Prefetch:       1,
			HandlerTimeout: 5 * time.Second,
			MaxRetries:     maxRetries,
			RetryDelay:     retryDelay,
			ReconnectMin:   50 * time.Millisecond,
			ReconnectMax:   500 * time.Millisecond,
		}, handler, logger)
	}()

	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			cancel()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Errorf("consumer did not stop; logs:\n%s", logs.String())
			}
		})
	}
	t.Cleanup(stop)

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		select {
		case <-stopped:
			t.Fatalf("consumer stopped before becoming ready; logs:\n%s", logs.String())
		default:
		}

		queue, err := f.channel.QueueInspect(f.queue)
		if err == nil && queue.Consumers > 0 {
			return stop
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("consumer did not become ready; logs:\n%s", logs.String())
	return stop
}

func (f *fixture) publish(t *testing.T, messageID, body string) {
	t.Helper()

	err := f.channel.PublishWithContext(context.Background(), f.exchange, f.routingKey, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     messageID,
		CorrelationId: "correlation-" + messageID,
		Body:          []byte(body),
	})
	if err != nil {
		t.Fatalf("publish message: %v", err)
	}
}

func (f *fixture) waitForEmpty(t *testing.T, queue string, timeout time.Duration) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		info, err := f.channel.QueueInspect(queue)
		if err != nil {
			t.Fatalf("inspect queue %s: %v", queue, err)
		}
		if info.Messages == 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for queue %s to drain", queue)
}

func (f *fixture) waitForMessage(t *testing.T, queue string, timeout time.Duration) amqp.Delivery {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		delivery, ok, err := f.channel.Get(queue, true)
		if err != nil {
			t.Fatalf("get from queue %s: %v", queue, err)
		}
		if ok {
			return delivery
		}
		time.Sleep(25 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for a message in queue %s", queue)
	return amqp.Delivery{}
}

func waitForAttempt(t *testing.T, attempts <-chan struct{}, timeout time.Duration) {
	t.Helper()

	select {
	case <-attempts:
	case <-time.After(timeout):
		t.Fatal("timed out waiting for a handler attempt")
	}
}

func headerString(t *testing.T, headers amqp.Table, key string) string {
	t.Helper()

	value, ok := headers[key]
	if !ok {
		t.Fatalf("header %s is missing", key)
	}
	text, ok := value.(string)
	if !ok {
		t.Fatalf("header %s = %T, want string", key, value)
	}
	return text
}

func headerInt(t *testing.T, headers amqp.Table, key string) int {
	t.Helper()

	value, ok := headers[key]
	if !ok {
		t.Fatalf("header %s is missing", key)
	}

	switch count := value.(type) {
	case int:
		return count
	case int32:
		return int(count)
	case int64:
		return int(count)
	default:
		t.Fatalf("header %s = %T, want an integer", key, value)
		return 0
	}
}

func TestConsumerAcknowledgesSuccessfulMessage(t *testing.T) {
	f := newFixture(t)
	attempts := make(chan struct{}, 1)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		attempts <- struct{}{}
		return nil
	}), 2, 100*time.Millisecond)

	f.publish(t, "success-1", `{"event_id":"success-1"}`)
	waitForAttempt(t, attempts, 5*time.Second)
	f.waitForEmpty(t, f.queue, 5*time.Second)
}

func TestConsumerRetriesRetryableFailureThenSucceeds(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	attempts := make(chan struct{}, 3)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		call := calls.Add(1)
		attempts <- struct{}{}
		if call == 1 {
			return errors.New("transient failure")
		}
		return nil
	}), 2, 100*time.Millisecond)

	f.publish(t, "retry-success-1", `{"event_id":"retry-success-1"}`)
	waitForAttempt(t, attempts, 5*time.Second)
	waitForAttempt(t, attempts, 5*time.Second)

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)

	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}

	info, err := f.channel.QueueInspect(f.dlq)
	if err != nil {
		t.Fatalf("inspect dead-letter queue: %v", err)
	}
	if info.Messages != 0 {
		t.Fatalf("dead-letter queue messages = %d, want 0", info.Messages)
	}
}

func TestConsumerDeadLettersPermanentFailure(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	attempts := make(chan struct{}, 1)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		calls.Add(1)
		attempts <- struct{}{}
		return messaging.Permanent(errors.New("invalid event"))
	}), 2, 100*time.Millisecond)

	f.publish(t, "permanent-1", `{"event_id":"permanent-1"}`)
	waitForAttempt(t, attempts, 5*time.Second)

	delivery := f.waitForMessage(t, f.dlq, 5*time.Second)
	if string(delivery.Body) != `{"event_id":"permanent-1"}` {
		t.Fatalf("dead-letter body = %q", delivery.Body)
	}
	if delivery.MessageId != "permanent-1" {
		t.Fatalf("dead-letter message ID = %q", delivery.MessageId)
	}
	if delivery.CorrelationId != "correlation-permanent-1" {
		t.Fatalf("dead-letter correlation ID = %q", delivery.CorrelationId)
	}
	if got := headerString(t, delivery.Headers, failureReasonHeader); got != "permanent" {
		t.Fatalf("%s = %q, want permanent", failureReasonHeader, got)
	}

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)

	if calls.Load() != 1 {
		t.Fatalf("handler calls = %d, want 1", calls.Load())
	}
}

func TestConsumerDeadLettersAfterRetryExhaustion(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	attempts := make(chan struct{}, 4)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		calls.Add(1)
		attempts <- struct{}{}
		return errors.New("transient failure")
	}), 1, 100*time.Millisecond)

	f.publish(t, "exhausted-1", `{"event_id":"exhausted-1"}`)
	waitForAttempt(t, attempts, 5*time.Second)
	waitForAttempt(t, attempts, 5*time.Second)

	delivery := f.waitForMessage(t, f.dlq, 5*time.Second)
	if got := headerString(t, delivery.Headers, failureReasonHeader); got != "retry_exhausted" {
		t.Fatalf("%s = %q, want retry_exhausted", failureReasonHeader, got)
	}
	if got := headerInt(t, delivery.Headers, retryCountHeader); got != 1 {
		t.Fatalf("%s = %d, want 1", retryCountHeader, got)
	}

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)

	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}
}

func TestConsumerRecoversPanicAndRetries(t *testing.T) {
	f := newFixture(t)
	var calls atomic.Int32
	attempts := make(chan struct{}, 3)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		call := calls.Add(1)
		attempts <- struct{}{}
		if call == 1 {
			panic("transient panic")
		}
		return nil
	}), 2, 100*time.Millisecond)

	f.publish(t, "panic-1", `{"event_id":"panic-1"}`)
	waitForAttempt(t, attempts, 5*time.Second)
	waitForAttempt(t, attempts, 5*time.Second)

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)

	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want 2", calls.Load())
	}

	info, err := f.channel.QueueInspect(f.dlq)
	if err != nil {
		t.Fatalf("inspect dead-letter queue: %v", err)
	}
	if info.Messages != 0 {
		t.Fatalf("dead-letter queue messages = %d, want 0", info.Messages)
	}
}

func (f *fixture) publishWithContentType(t *testing.T, messageID, contentType, body string) {
	t.Helper()

	err := f.channel.PublishWithContext(context.Background(), f.exchange, f.routingKey, false, false, amqp.Publishing{
		ContentType:   contentType,
		DeliveryMode:  amqp.Persistent,
		MessageId:     messageID,
		CorrelationId: "correlation-" + messageID,
		Body:          []byte(body),
	})
	if err != nil {
		t.Fatalf("publish message: %v", err)
	}
}

func TestConsumerRequeuesInFlightMessageOnShutdown(t *testing.T) {
	f := newFixture(t)

	started := make(chan struct{})
	stopFirst := f.runConsumer(t, handlerFunc(func(ctx context.Context, _ messaging.Message) error {
		close(started)
		<-ctx.Done()
		return errors.New("handler interrupted by shutdown")
	}), 2, 100*time.Millisecond)

	f.publish(t, "shutdown-requeue-1", `{"event_id":"shutdown-requeue-1"}`)
	waitForAttempt(t, started, 5*time.Second)
	stopFirst()

	var calls atomic.Int32
	processed := make(chan struct{}, 1)
	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		calls.Add(1)
		processed <- struct{}{}
		return nil
	}), 2, 100*time.Millisecond)

	select {
	case <-processed:
	case <-time.After(5 * time.Second):
		t.Fatal("the requeued message was not processed by the second consumer")
	}

	f.waitForEmpty(t, f.queue, 5*time.Second)
	if calls.Load() != 1 {
		t.Fatalf("second handler calls = %d, want 1", calls.Load())
	}

	info, err := f.channel.QueueInspect(f.dlq)
	if err != nil {
		t.Fatalf("inspect dead-letter queue: %v", err)
	}
	if info.Messages != 0 {
		t.Fatalf("dead-letter queue messages = %d, want 0", info.Messages)
	}
}

func TestConsumerDeadLettersOversizedMessage(t *testing.T) {
	f := newFixture(t)
	handlerCalled := make(chan struct{}, 1)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		handlerCalled <- struct{}{}
		return nil
	}), 2, 100*time.Millisecond)

	body := strings.Repeat("a", messaging.MaxMessageBytes+1)
	f.publishWithContentType(t, "oversized-1", "application/json", body)

	delivery := f.waitForMessage(t, f.dlq, 10*time.Second)
	if got := headerString(t, delivery.Headers, failureReasonHeader); got != "permanent" {
		t.Fatalf("%s = %q, want permanent", failureReasonHeader, got)
	}
	if len(delivery.Body) != len(body) {
		t.Fatalf("dead-letter body length = %d, want %d", len(delivery.Body), len(body))
	}

	select {
	case <-handlerCalled:
		t.Fatal("handler was called for an oversized message")
	default:
	}

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)
}

func TestConsumerDeadLettersUnsupportedContentType(t *testing.T) {
	f := newFixture(t)
	handlerCalled := make(chan struct{}, 1)

	f.runConsumer(t, handlerFunc(func(context.Context, messaging.Message) error {
		handlerCalled <- struct{}{}
		return nil
	}), 2, 100*time.Millisecond)

	f.publishWithContentType(t, "content-type-1", "text/plain", `{"event_id":"content-type-1"}`)

	delivery := f.waitForMessage(t, f.dlq, 5*time.Second)
	if got := headerString(t, delivery.Headers, failureReasonHeader); got != "permanent" {
		t.Fatalf("%s = %q, want permanent", failureReasonHeader, got)
	}

	select {
	case <-handlerCalled:
		t.Fatal("handler was called for an unsupported content type")
	default:
	}

	f.waitForEmpty(t, f.queue, 5*time.Second)
	f.waitForEmpty(t, f.retryQueue, 5*time.Second)
}
