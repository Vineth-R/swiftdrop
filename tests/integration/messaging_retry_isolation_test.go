//go:build integration

package integration

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// retryIsolationQueueSuffix mirrors the shared topology's retry queue suffix.
// The retry queue name changes when its dead-letter arguments change, because
// RabbitMQ refuses to redeclare a durable queue with different arguments.
const retryIsolationQueueSuffix = ".retry.v2"

type retryIsolationFixture struct {
	url        string
	conn       *amqp.Connection
	channel    *amqp.Channel
	exchange   string
	routingKey string
	queueA     string
	queueB     string
	retryA     string
	retryB     string
	dlqA       string
	dlqB       string
}

func newRetryIsolationFixture(t *testing.T) *retryIsolationFixture {
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

	base := fmt.Sprintf("swiftdrop.test.retry.%d", time.Now().UnixNano())
	f := &retryIsolationFixture{
		url:        url,
		conn:       conn,
		channel:    channel,
		exchange:   base + ".events",
		routingKey: base + ".event.v1",
		queueA:     base + ".worker-a",
		queueB:     base + ".worker-b",
		retryA:     base + ".worker-a" + retryIsolationQueueSuffix,
		retryB:     base + ".worker-b" + retryIsolationQueueSuffix,
		dlqA:       base + ".worker-a.dlq",
		dlqB:       base + ".worker-b.dlq",
	}

	if err := channel.ExchangeDeclare(f.exchange, "topic", true, false, false, false, nil); err != nil {
		t.Fatalf("declare main exchange: %v", err)
	}
	for _, queue := range []string{f.queueA, f.queueB} {
		if _, err := channel.QueueDeclare(queue, true, false, false, false, nil); err != nil {
			t.Fatalf("declare queue %s: %v", queue, err)
		}
		if err := channel.QueueBind(queue, f.routingKey, f.exchange, false, nil); err != nil {
			t.Fatalf("bind queue %s: %v", queue, err)
		}
	}

	t.Cleanup(f.cleanup)
	return f
}

func (f *retryIsolationFixture) cleanup() {
	for _, queue := range []string{f.queueA, f.queueB, f.retryA, f.retryB, f.dlqA, f.dlqB} {
		_, _ = f.channel.QueueDelete(queue, false, false, false)
	}
	for _, exchange := range []string{f.exchange + ".retry", f.exchange + ".dlx", f.exchange} {
		_ = f.channel.ExchangeDelete(exchange, false, false)
	}
	_ = f.channel.Close()
	_ = f.conn.Close()
}

func (f *retryIsolationFixture) runWorker(t *testing.T, queue string, handler messaging.Handler) {
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
			Queue:          queue,
			RoutingKey:     f.routingKey,
			Prefetch:       1,
			HandlerTimeout: 5 * time.Second,
			MaxRetries:     3,
			RetryDelay:     100 * time.Millisecond,
			ReconnectMin:   50 * time.Millisecond,
			ReconnectMax:   500 * time.Millisecond,
		}, handler, logger)
	}()

	var stopOnce sync.Once
	t.Cleanup(func() {
		stopOnce.Do(func() {
			cancel()
			select {
			case <-stopped:
			case <-time.After(5 * time.Second):
				t.Errorf("consumer for %s did not stop; logs:\n%s", queue, logs.String())
			}
		})
	})

	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		info, err := f.channel.QueueInspect(queue)
		if err == nil && info.Consumers > 0 {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("consumer for %s did not become ready; logs:\n%s", queue, logs.String())
}

func (f *retryIsolationFixture) publish(t *testing.T, messageID string) {
	t.Helper()

	err := f.channel.PublishWithContext(context.Background(), f.exchange, f.routingKey, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     messageID,
		CorrelationId: "correlation-" + messageID,
		Body:          []byte(fmt.Sprintf(`{"event_id":%q}`, messageID)),
	})
	if err != nil {
		t.Fatalf("publish initial message: %v", err)
	}
}

// publishRetry puts a message directly on one worker's retry queue. The delay
// is the per-message TTL that eventually dead-letters it back to that worker.
func (f *retryIsolationFixture) publishRetry(t *testing.T, retryQueue, messageID string, delay time.Duration) {
	t.Helper()

	err := f.channel.PublishWithContext(context.Background(), f.exchange+".retry", retryQueue, false, false, amqp.Publishing{
		ContentType:   "application/json",
		DeliveryMode:  amqp.Persistent,
		MessageId:     messageID,
		CorrelationId: "correlation-" + messageID,
		Expiration:    fmt.Sprintf("%d", delay.Milliseconds()),
		Headers: amqp.Table{
			retryCountHeader:             int32(1),
			"x-swiftdrop-failure-reason": "retryable",
		},
		Body: []byte(fmt.Sprintf(`{"event_id":%q}`, messageID)),
	})
	if err != nil {
		t.Fatalf("publish retry to %s: %v", retryQueue, err)
	}
}

func (f *retryIsolationFixture) waitForEmpty(t *testing.T, queue string, timeout time.Duration) {
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

func retryCountValue(value any) (int, bool) {
	switch count := value.(type) {
	case int:
		return count, true
	case int32:
		return int(count), true
	case int64:
		return int(count), true
	default:
		return 0, false
	}
}

// TestRetryReturnsOnlyToOwningWorkerQueue proves that two workers can bind
// separate queues to the same domain event while keeping retries isolated.
func TestRetryReturnsOnlyToOwningWorkerQueue(t *testing.T) {
	tests := []struct {
		name   string
		retryA bool
	}{
		{name: "worker A retry stays on worker A", retryA: true},
		{name: "worker B retry stays on worker B", retryA: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			f := newRetryIsolationFixture(t)

			var aCalls, bCalls atomic.Int32
			aAttempts := make(chan struct{}, 4)
			bAttempts := make(chan struct{}, 4)
			aRetryCounts := make(chan int, 4)
			bRetryCounts := make(chan int, 4)

			f.runWorker(t, f.queueA, handlerFunc(func(_ context.Context, message messaging.Message) error {
				aCalls.Add(1)
				if count, ok := retryCountValue(message.Headers[retryCountHeader]); ok {
					aRetryCounts <- count
				}
				aAttempts <- struct{}{}
				return nil
			}))
			f.runWorker(t, f.queueB, handlerFunc(func(_ context.Context, message messaging.Message) error {
				bCalls.Add(1)
				if count, ok := retryCountValue(message.Headers[retryCountHeader]); ok {
					bRetryCounts <- count
				}
				bAttempts <- struct{}{}
				return nil
			}))

			// Both queues are bound to the same domain key, so both receive the
			// initial event. This proves the fan-out is intentional.
			f.publish(t, "initial-"+test.name)
			waitForAttempt(t, aAttempts, 5*time.Second)
			waitForAttempt(t, bAttempts, 5*time.Second)

			var (
				targetRetryQueue string
				targetAttempts   chan struct{}
				targetCalls      *atomic.Int32
				targetCounts     chan int
				bystanderCalls   *atomic.Int32
			)
			if test.retryA {
				targetRetryQueue = f.retryA
				targetAttempts = aAttempts
				targetCalls = &aCalls
				targetCounts = aRetryCounts
				bystanderCalls = &bCalls
			} else {
				targetRetryQueue = f.retryB
				targetAttempts = bAttempts
				targetCalls = &bCalls
				targetCounts = bRetryCounts
				bystanderCalls = &aCalls
			}

			f.publishRetry(t, targetRetryQueue, "retry-"+test.name, 150*time.Millisecond)
			waitForAttempt(t, targetAttempts, 5*time.Second)

			// If the retry still returned through the shared domain exchange,
			// the bystander queue would receive a second copy in this window.
			time.Sleep(750 * time.Millisecond)

			if got := targetCalls.Load(); got != 2 {
				t.Fatalf("target worker calls = %d, want 2 (initial + retry)", got)
			}
			if got := bystanderCalls.Load(); got != 1 {
				t.Fatalf("bystander worker calls = %d, want 1 (initial only); retry leaked across queues", got)
			}

			select {
			case count := <-targetCounts:
				if count != 1 {
					t.Fatalf("retry count header = %d, want 1", count)
				}
			default:
				t.Fatal("retried message did not reach the target handler with a retry-count header")
			}

			f.waitForEmpty(t, f.queueA, 5*time.Second)
			f.waitForEmpty(t, f.queueB, 5*time.Second)
			f.waitForEmpty(t, f.retryA, 5*time.Second)
			f.waitForEmpty(t, f.retryB, 5*time.Second)
		})
	}
}
