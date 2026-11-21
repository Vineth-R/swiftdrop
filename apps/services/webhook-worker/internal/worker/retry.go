package worker

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

// attemptHeader carries the number of the attempt a message will make. A
// message from the exchange has no header, which means attempt 1.
const attemptHeader = "x-webhook-attempt"

const (
	dialTimeout    = 10 * time.Second
	publishTimeout = 10 * time.Second
)

// AMQPScheduler schedules a later attempt with RabbitMQ delay queues, so the
// 1 minute to 6 hour waits (API v1.1 section 10.3) survive a restart of the
// worker. Each distinct delay gets its own queue with a fixed message TTL, so a
// short delay never waits behind a long one. When the TTL expires, RabbitMQ
// dead-letters the message through the default exchange back into the worker's
// own queue, where the normal consumer picks it up.
type AMQPScheduler struct {
	url   string
	queue string
}

func NewAMQPScheduler(url, queue string) *AMQPScheduler {
	return &AMQPScheduler{url: url, queue: queue}
}

// delayQueueName names the delay queue for one delay, e.g.
// webhook.parcel.status_changed.v1.delay.300s.
func delayQueueName(queue string, delay time.Duration) string {
	return fmt.Sprintf("%s.delay.%ds", queue, int64(delay/time.Second))
}

// delayQueueArgs are the arguments of a delay queue. They never change for a
// given queue name, because RabbitMQ rejects a redeclare with other arguments.
func delayQueueArgs(queue string, delay time.Duration) amqp.Table {
	return amqp.Table{
		"x-queue-type":              "classic",
		"x-message-ttl":             int64(delay / time.Millisecond),
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": queue,
	}
}

// Schedule publishes message again, to be consumed as attempt nextAttempt after
// delay. The body, message ID and correlation ID are kept, so the merchant sees
// the same eventId on every attempt.
func (s *AMQPScheduler) Schedule(ctx context.Context, message messaging.Message, nextAttempt int, delay time.Duration) error {
	if delay < time.Second {
		return errors.New("schedule retry: delay must be at least 1s")
	}

	conn, err := amqp.DialConfig(s.url, amqp.Config{Dial: amqp.DefaultDial(dialTimeout)})
	if err != nil {
		// The error can contain the broker URL with its password; say only
		// that the connection failed.
		return errors.New("schedule retry: could not connect to RabbitMQ")
	}
	defer func() { _ = conn.Close() }()

	channel, err := conn.Channel()
	if err != nil {
		return fmt.Errorf("schedule retry: open channel: %w", err)
	}
	defer func() { _ = channel.Close() }()

	if err := channel.Confirm(false); err != nil {
		return fmt.Errorf("schedule retry: enable confirms: %w", err)
	}

	name := delayQueueName(s.queue, delay)
	if _, err := channel.QueueDeclare(name, true, false, false, false, delayQueueArgs(s.queue, delay)); err != nil {
		return fmt.Errorf("schedule retry: declare %s: %w", name, err)
	}

	pubCtx, cancel := context.WithTimeout(ctx, publishTimeout)
	defer cancel()

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(pubCtx, "", name, false, false, amqp.Publishing{
		ContentType:   message.ContentType,
		DeliveryMode:  amqp.Persistent,
		MessageId:     message.MessageID,
		CorrelationId: message.CorrelationID,
		Timestamp:     time.Now().UTC(),
		Headers:       amqp.Table{attemptHeader: int32(nextAttempt)},
		Body:          message.Body,
	})
	if err != nil {
		return fmt.Errorf("schedule retry: publish: %w", err)
	}
	acked, err := confirmation.WaitContext(pubCtx)
	if err != nil {
		return fmt.Errorf("schedule retry: wait for confirm: %w", err)
	}
	if !acked {
		return errors.New("schedule retry: broker did not confirm the message")
	}
	return nil
}

// attemptFromHeaders reads the attempt number of a message. No header means a
// first delivery from the exchange.
func attemptFromHeaders(headers map[string]any) (int, error) {
	raw, ok := headers[attemptHeader]
	if !ok {
		return 1, nil
	}
	switch v := raw.(type) {
	case int:
		return v, nil
	case int8:
		return int(v), nil
	case int16:
		return int(v), nil
	case int32:
		return int(v), nil
	case int64:
		return int(v), nil
	case uint8:
		return int(v), nil
	case uint16:
		return int(v), nil
	case uint32:
		return int(v), nil
	case string:
		n, err := strconv.Atoi(v)
		if err != nil {
			return 0, fmt.Errorf("invalid %s header", attemptHeader)
		}
		return n, nil
	default:
		return 0, fmt.Errorf("invalid %s header", attemptHeader)
	}
}
