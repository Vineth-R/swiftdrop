package outbox

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	defaultPublishTimeout = 10 * time.Second
)

// AMQPPublisher connects to RabbitMQ and publishes messages with publisher confirms.
type AMQPPublisher struct {
	url      string
	exchange string

	mu      sync.Mutex
	conn    *amqp.Connection
	channel *amqp.Channel
}

// NewAMQPPublisher creates an AMQPPublisher.
func NewAMQPPublisher(amqpURL, exchange string) *AMQPPublisher {
	if exchange == "" {
		exchange = "swiftdrop.events"
	}
	return &AMQPPublisher{
		url:      amqpURL,
		exchange: exchange,
	}
}

// Close closes the underlying AMQP channel and connection.
func (p *AMQPPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error
	if p.channel != nil {
		if err := p.channel.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, err)
		}
		p.channel = nil
	}
	if p.conn != nil {
		if err := p.conn.Close(); err != nil && !errors.Is(err, amqp.ErrClosed) {
			errs = append(errs, err)
		}
		p.conn = nil
	}
	if len(errs) > 0 {
		return fmt.Errorf("closing amqp publisher: %v", errs)
	}
	return nil
}

// ensureChannel verifies the connection and channel are open and publisher confirms are enabled.
func (p *AMQPPublisher) ensureChannel() (*amqp.Channel, error) {
	if p.url == "" {
		return nil, errors.New("amqp url is not configured")
	}

	if p.conn != nil && !p.conn.IsClosed() && p.channel != nil && !p.channel.IsClosed() {
		return p.channel, nil
	}

	// Clean up stale handles
	if p.channel != nil {
		_ = p.channel.Close()
		p.channel = nil
	}
	if p.conn != nil {
		_ = p.conn.Close()
		p.conn = nil
	}

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return nil, fmt.Errorf("dial rabbitmq: %w", err)
	}

	channel, err := conn.Channel()
	if err != nil {
		_ = conn.Close()
		return nil, fmt.Errorf("open rabbitmq channel: %w", err)
	}

	// Declare topic exchange as durable
	if err := channel.ExchangeDeclare(
		p.exchange,
		"topic",
		true,  // durable
		false, // auto-delete
		false, // internal
		false, // no-wait
		nil,
	); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("declare exchange %s: %w", p.exchange, err)
	}

	// Enable publisher confirms
	if err := channel.Confirm(false); err != nil {
		_ = channel.Close()
		_ = conn.Close()
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	p.conn = conn
	p.channel = channel
	return p.channel, nil
}

// Publish publishes a message with publisher confirms.
func (p *AMQPPublisher) Publish(ctx context.Context, exchange, routingKey string, body []byte) error {
	p.mu.Lock()
	channel, err := p.ensureChannel()
	p.mu.Unlock()
	if err != nil {
		return fmt.Errorf("ensure amqp channel: %w", err)
	}

	publishCtx, cancel := context.WithTimeout(ctx, defaultPublishTimeout)
	defer cancel()

	if exchange == "" {
		exchange = p.exchange
	}

	msg := amqp.Publishing{
		DeliveryMode: amqp.Persistent,
		ContentType:  "application/json",
		Timestamp:    time.Now().UTC(),
		Body:         body,
	}

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(publishCtx, exchange, routingKey, true, false, msg)
	if err != nil {
		return fmt.Errorf("publish message: %w", err)
	}

	confirmed, err := confirmation.WaitContext(publishCtx)
	if err != nil {
		return fmt.Errorf("wait for confirm: %w", err)
	}
	if !confirmed {
		return fmt.Errorf("broker nacked publication to %s/%s", exchange, routingKey)
	}

	return nil
}
