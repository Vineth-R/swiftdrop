package messaging

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type handlerFunc func(context.Context, Message) error

func (f handlerFunc) Handle(ctx context.Context, message Message) error {
	return f(ctx, message)
}

type ackCall struct {
	tag      uint64
	multiple bool
}

type nackCall struct {
	tag      uint64
	multiple bool
	requeue  bool
}

type recordingAcknowledger struct {
	acks  []ackCall
	nacks []nackCall
}

func (a *recordingAcknowledger) Ack(tag uint64, multiple bool) error {
	a.acks = append(a.acks, ackCall{tag: tag, multiple: multiple})
	return nil
}

func (a *recordingAcknowledger) Nack(tag uint64, multiple, requeue bool) error {
	a.nacks = append(a.nacks, nackCall{tag: tag, multiple: multiple, requeue: requeue})
	return nil
}

func (*recordingAcknowledger) Reject(uint64, bool) error {
	return nil
}

type publishCall struct {
	kind       string
	retryCount int
	delay      time.Duration
	reason     string
}

type recordingPublisher struct {
	calls         []publishCall
	retryErr      error
	deadLetterErr error
}

func (p *recordingPublisher) PublishRetry(_ context.Context, _ amqp.Delivery, retryCount int, delay time.Duration, reason string) error {
	p.calls = append(p.calls, publishCall{
		kind:       "retry",
		retryCount: retryCount,
		delay:      delay,
		reason:     reason,
	})
	return p.retryErr
}

func (p *recordingPublisher) PublishDeadLetter(_ context.Context, _ amqp.Delivery, reason string) error {
	p.calls = append(p.calls, publishCall{kind: "dead_letter", reason: reason})
	return p.deadLetterErr
}

func testDelivery(acker amqp.Acknowledger, body []byte) amqp.Delivery {
	return amqp.Delivery{
		Acknowledger:  acker,
		DeliveryTag:   42,
		MessageId:     "message-1",
		CorrelationId: "correlation-1",
		RoutingKey:    "notification.requested.v1",
		Redelivered:   true,
		Body:          body,
	}
}

func testLogger(w io.Writer) *slog.Logger {
	return slog.New(slog.NewTextHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug}))
}

func testProcessor(handler Handler, publisher failurePublisher, maxRetries int) deliveryProcessor {
	return deliveryProcessor{
		handler:    handler,
		publisher:  publisher,
		timeout:    time.Minute,
		maxRetries: maxRetries,
		retryDelay: 100 * time.Millisecond,
		logger:     testLogger(io.Discard),
	}
}

func requireSingleAck(t *testing.T, acker *recordingAcknowledger) ackCall {
	t.Helper()

	if len(acker.acks) != 1 {
		t.Fatalf("Ack calls = %d, want 1", len(acker.acks))
	}
	if len(acker.nacks) != 0 {
		t.Fatalf("Nack calls = %d, want 0", len(acker.nacks))
	}
	if acker.acks[0].tag != 42 {
		t.Fatalf("Ack tag = %d, want 42", acker.acks[0].tag)
	}
	if acker.acks[0].multiple {
		t.Fatal("Ack multiple = true, want false")
	}

	return acker.acks[0]
}

func requireSingleNack(t *testing.T, acker *recordingAcknowledger, requeue bool) nackCall {
	t.Helper()

	if len(acker.nacks) != 1 {
		t.Fatalf("Nack calls = %d, want 1", len(acker.nacks))
	}
	if len(acker.acks) != 0 {
		t.Fatalf("Ack calls = %d, want 0", len(acker.acks))
	}
	if acker.nacks[0].tag != 42 {
		t.Fatalf("Nack tag = %d, want 42", acker.nacks[0].tag)
	}
	if acker.nacks[0].multiple {
		t.Fatal("Nack multiple = true, want false")
	}
	if acker.nacks[0].requeue != requeue {
		t.Fatalf("Nack requeue = %t, want %t", acker.nacks[0].requeue, requeue)
	}

	return acker.nacks[0]
}

func TestConsumerConfigValidate(t *testing.T) {
	valid := ConsumerConfig{
		URL:            "amqp://guest:guest@localhost:5672/",
		Exchange:       "swiftdrop.events",
		Queue:          "notification.requested.v1",
		RoutingKey:     "notification.requested.v1",
		Prefetch:       10,
		HandlerTimeout: 30 * time.Second,
		MaxRetries:     3,
		RetryDelay:     30 * time.Second,
		ReconnectMin:   time.Second,
		ReconnectMax:   30 * time.Second,
	}

	if err := valid.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	zeroRetries := valid
	zeroRetries.MaxRetries = 0
	if err := zeroRetries.validate(); err != nil {
		t.Fatalf("zero-retry config rejected: %v", err)
	}

	tests := map[string]func(*ConsumerConfig){
		"missing url":             func(c *ConsumerConfig) { c.URL = "" },
		"missing exchange":        func(c *ConsumerConfig) { c.Exchange = "" },
		"missing queue":           func(c *ConsumerConfig) { c.Queue = "" },
		"missing routing key":     func(c *ConsumerConfig) { c.RoutingKey = "" },
		"invalid prefetch":        func(c *ConsumerConfig) { c.Prefetch = 0 },
		"invalid handler timeout": func(c *ConsumerConfig) { c.HandlerTimeout = 0 },
		"negative max retries":    func(c *ConsumerConfig) { c.MaxRetries = -1 },
		"retry delay too small":   func(c *ConsumerConfig) { c.RetryDelay = time.Microsecond },
		"invalid reconnect bounds": func(c *ConsumerConfig) {
			c.ReconnectMin = 10 * time.Second
			c.ReconnectMax = time.Second
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			config := valid
			mutate(&config)
			if err := config.validate(); err == nil {
				t.Fatal("validate() should have failed")
			}
		})
	}
}

func TestDeliveryProcessorAcknowledgesSuccess(t *testing.T) {
	acker := &recordingAcknowledger{}
	var received Message

	handler := handlerFunc(func(_ context.Context, message Message) error {
		received = message
		return nil
	})

	testProcessor(handler, nil, 3).handle(
		context.Background(),
		testDelivery(acker, []byte(`{"secret":"not-logged"}`)),
	)

	requireSingleAck(t, acker)
	if received.MessageID != "message-1" {
		t.Fatalf("MessageID = %q, want message-1", received.MessageID)
	}
	if received.CorrelationID != "correlation-1" {
		t.Fatalf("CorrelationID = %q, want correlation-1", received.CorrelationID)
	}
	if !received.Redelivered {
		t.Fatal("Redelivered = false, want true")
	}
}

func TestDeliveryProcessorRoutesRetryableError(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		return errors.New("handler failed")
	})

	testProcessor(handler, publisher, 3).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:       "retry",
		retryCount: 1,
		delay:      100 * time.Millisecond,
		reason:     "retryable",
	})
}

func TestDeliveryProcessorRoutesWrappedPermanentError(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		return fmt.Errorf("wrapped: %w", Permanent(errors.New("bad event")))
	})

	testProcessor(handler, publisher, 3).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "permanent",
	})
}

func TestDeliveryProcessorDeadLettersAfterRetryExhaustion(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		return errors.New("handler failed")
	})

	delivery := testDelivery(acker, []byte(`{}`))
	delivery.Headers = amqp.Table{retryCountHeader: int32(3)}

	testProcessor(handler, publisher, 3).handle(context.Background(), delivery)

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "retry_exhausted",
	})
}

func TestDeliveryProcessorDeadLettersWhenMaxRetriesIsZero(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		return errors.New("handler failed")
	})

	testProcessor(handler, publisher, 0).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "retry_exhausted",
	})
}

func TestDeliveryProcessorRequeuesWhenRetryPublicationFails(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{retryErr: errors.New("confirm failed")}
	handler := handlerFunc(func(context.Context, Message) error {
		return errors.New("handler failed")
	})

	testProcessor(handler, publisher, 3).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleNack(t, acker, true)
	requirePublishCall(t, publisher, publishCall{
		kind:       "retry",
		retryCount: 1,
		delay:      100 * time.Millisecond,
		reason:     "retryable",
	})
}

func TestDeliveryProcessorRequeuesWhenDeadLetterPublicationFails(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{deadLetterErr: errors.New("confirm failed")}
	handler := handlerFunc(func(context.Context, Message) error {
		return Permanent(errors.New("bad event"))
	})

	testProcessor(handler, publisher, 3).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleNack(t, acker, true)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "permanent",
	})
}

func TestDeliveryProcessorRequeuesOnShutdownBeforeRouting(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	cancel()

	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		return Permanent(errors.New("bad event"))
	})

	testProcessor(handler, publisher, 3).handle(parent, testDelivery(acker, []byte(`{}`)))

	requireSingleNack(t, acker, true)
	if len(publisher.calls) != 0 {
		t.Fatalf("publisher calls = %d, want 0", len(publisher.calls))
	}
}

func TestDeliveryProcessorRecoversPanicAndSchedulesRetry(t *testing.T) {
	var logs bytes.Buffer
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		panic("handler boom")
	})
	processor := testProcessor(handler, publisher, 3)
	processor.logger = testLogger(&logs)

	func() {
		defer func() {
			if recovered := recover(); recovered != nil {
				t.Fatalf("panic escaped delivery handling: %v", recovered)
			}
		}()

		processor.handle(
			context.Background(),
			testDelivery(acker, []byte(`{"secret":"do-not-log"}`)),
		)
	}()

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:       "retry",
		retryCount: 1,
		delay:      100 * time.Millisecond,
		reason:     "panic",
	})

	output := logs.String()
	for _, want := range []string{
		"message handler panicked",
		"handler boom",
		"message_id=message-1",
		"correlation_id=correlation-1",
		"delivery_tag=42",
		"redelivered=true",
		"stack=",
	} {
		if !strings.Contains(output, want) {
			t.Errorf("panic log missing %q:\n%s", want, output)
		}
	}
	if strings.Contains(output, "do-not-log") {
		t.Errorf("panic log contains message body:\n%s", output)
	}
}

func TestDeliveryProcessorPanicAtRetryLimitDeadLetters(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handler := handlerFunc(func(context.Context, Message) error {
		panic("handler boom")
	})

	testProcessor(handler, publisher, 0).handle(
		context.Background(),
		testDelivery(acker, []byte(`{}`)),
	)

	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "retry_exhausted",
	})
}

func TestDeliveryProcessorDeadLettersOversizedMessage(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handlerCalled := false
	handler := handlerFunc(func(context.Context, Message) error {
		handlerCalled = true
		return nil
	})

	body := make([]byte, MaxMessageBytes+1)
	testProcessor(handler, publisher, 3).handle(
		context.Background(),
		testDelivery(acker, body),
	)

	if handlerCalled {
		t.Fatal("handler was called for an oversized message")
	}
	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "permanent",
	})
}

func TestDeliveryProcessorDeadLettersUnsupportedContentType(t *testing.T) {
	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	handlerCalled := false
	handler := handlerFunc(func(context.Context, Message) error {
		handlerCalled = true
		return nil
	})

	delivery := testDelivery(acker, []byte(`{}`))
	delivery.ContentType = "text/plain"

	testProcessor(handler, publisher, 3).handle(context.Background(), delivery)

	if handlerCalled {
		t.Fatal("handler was called for an unsupported content type")
	}
	requireSingleAck(t, acker)
	requirePublishCall(t, publisher, publishCall{
		kind:   "dead_letter",
		reason: "permanent",
	})
}

func TestRetryCountFromHeaders(t *testing.T) {
	tests := []struct {
		name    string
		headers amqp.Table
		want    int
	}{
		{name: "missing", headers: nil, want: 0},
		{name: "int", headers: amqp.Table{retryCountHeader: 2}, want: 2},
		{name: "int32", headers: amqp.Table{retryCountHeader: int32(3)}, want: 3},
		{name: "int64", headers: amqp.Table{retryCountHeader: int64(4)}, want: 4},
		{name: "negative", headers: amqp.Table{retryCountHeader: int32(-1)}, want: 0},
		{name: "wrong type", headers: amqp.Table{retryCountHeader: "3"}, want: 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := retryCountFromHeaders(tt.headers); got != tt.want {
				t.Fatalf("retryCountFromHeaders() = %d, want %d", got, tt.want)
			}
		})
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(time.Second, 30*time.Second); got != 2*time.Second {
		t.Fatalf("nextBackoff() = %v, want 2s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff() = %v, want 30s", got)
	}
	if got := nextBackoff(30*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff() = %v, want 30s", got)
	}
}

func requirePublishCall(t *testing.T, publisher *recordingPublisher, want publishCall) {
	t.Helper()

	if len(publisher.calls) != 1 {
		t.Fatalf("publisher calls = %d, want 1", len(publisher.calls))
	}
	if publisher.calls[0] != want {
		t.Fatalf("publisher call = %+v, want %+v", publisher.calls[0], want)
	}
}

func TestJitterBounds(t *testing.T) {
	const base = 10 * time.Second

	if got := jitter(0, func(int64) int64 {
		t.Fatal("randN called for a zero base")
		return 0
	}); got != 0 {
		t.Fatalf("jitter(0) = %v, want 0", got)
	}

	if got := jitter(time.Nanosecond, func(int64) int64 {
		t.Fatal("randN called for a sub-half base")
		return 0
	}); got != time.Nanosecond {
		t.Fatalf("jitter(1ns) = %v, want 1ns", got)
	}

	half := base / 2
	if got := jitter(base, func(int64) int64 { return 0 }); got != half {
		t.Fatalf("lower-bound jitter = %v, want %v", got, half)
	}

	if got := jitter(base, func(n int64) int64 {
		if want := int64(half) + 1; n != want {
			t.Fatalf("randN argument = %d, want %d", n, want)
		}
		return n - 1
	}); got != base {
		t.Fatalf("upper-bound jitter = %v, want %v", got, base)
	}
}

func TestConsumerBackoffCapsAndResetsAfterReady(t *testing.T) {
	type step struct {
		ready bool
		err   error
	}

	steps := []step{
		{err: errors.New("dial failed")},
		{err: errors.New("dial failed")},
		{err: errors.New("dial failed")},
		{err: errors.New("dial failed")},
		{ready: true, err: errors.New("connection lost")},
		{err: errors.New("dial failed")},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	calls := 0
	var waits []time.Duration

	c := consumer{
		config:  ConsumerConfig{ReconnectMin: time.Second, ReconnectMax: 3 * time.Second},
		handler: handlerFunc(func(context.Context, Message) error { return nil }),
		logger:  testLogger(io.Discard),
		randN:   func(int64) int64 { return 0 },
		consume: func(_ context.Context, _ ConsumerConfig, _ Handler, _ *slog.Logger, onReady func()) error {
			if calls >= len(steps) {
				t.Fatalf("consume called %d times, want at most %d", calls+1, len(steps))
			}
			current := steps[calls]
			calls++
			if current.ready {
				onReady()
			}
			return current.err
		},
		wait: func(_ context.Context, delay time.Duration) error {
			waits = append(waits, delay)
			if len(waits) == len(steps) {
				cancel()
				return context.Canceled
			}
			return nil
		},
	}

	if err := c.run(ctx); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}

	want := []time.Duration{
		500 * time.Millisecond,
		time.Second,
		1500 * time.Millisecond,
		1500 * time.Millisecond,
		500 * time.Millisecond,
		time.Second,
	}
	if !slices.Equal(waits, want) {
		t.Fatalf("waits = %v, want %v", waits, want)
	}
}

func TestConsumerRunStopsWithoutReconnectingAfterShutdown(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	waitCalled := false
	c := consumer{
		config:  ConsumerConfig{ReconnectMin: time.Second, ReconnectMax: time.Second},
		handler: handlerFunc(func(context.Context, Message) error { return nil }),
		logger:  testLogger(io.Discard),
		randN:   func(int64) int64 { return 0 },
		consume: func(ctx context.Context, _ ConsumerConfig, _ Handler, _ *slog.Logger, _ func()) error {
			cancel()
			<-ctx.Done()
			return ctx.Err()
		},
		wait: func(context.Context, time.Duration) error {
			waitCalled = true
			return nil
		},
	}

	if err := c.run(ctx); err != nil {
		t.Fatalf("run() error = %v, want nil", err)
	}
	if waitCalled {
		t.Fatal("run() waited after shutdown was requested")
	}
}

func TestWaitContextHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	start := time.Now()
	err := waitContext(ctx, time.Hour)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitContext() error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Fatalf("waitContext() took %v after cancellation", elapsed)
	}

	if err := waitContext(context.Background(), 0); err != nil {
		t.Fatalf("waitContext() with zero delay = %v, want nil", err)
	}
}

func TestDialAMQPHonorsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	connection, err := dialAMQP(ctx, "amqp://guest:guest@127.0.0.1:1/")
	if connection != nil {
		_ = connection.Close()
		t.Fatal("dialAMQP() returned a connection with a canceled context")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("dialAMQP() error = %v, want context.Canceled", err)
	}
}

func TestDeliveryProcessorRequeuesWhenHandlerInterruptedByShutdown(t *testing.T) {
	parent, cancel := context.WithCancel(context.Background())
	defer cancel()

	acker := &recordingAcknowledger{}
	publisher := &recordingPublisher{}
	started := make(chan struct{})

	handler := handlerFunc(func(ctx context.Context, _ Message) error {
		close(started)
		<-ctx.Done()
		return errors.New("handler interrupted")
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		testProcessor(handler, publisher, 3).handle(parent, testDelivery(acker, []byte(`{}`)))
	}()

	<-started
	cancel()
	<-done

	requireSingleNack(t, acker, true)
	if len(publisher.calls) != 0 {
		t.Fatalf("publisher calls = %d, want 0", len(publisher.calls))
	}
}
