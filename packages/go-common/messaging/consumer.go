package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type Handler interface {
	Handle(ctx context.Context, message Message) error
}

type Message struct {
	Body        []byte
	ContentType string
	MessageID   string
	RoutingKey  string
	Headers     map[string]any
}

type ConsumerConfig struct {
	URL            string
	Exchange       string
	Queue          string
	RoutingKey     string
	Prefetch       int
	HandlerTimeout time.Duration
	ReconnectMin   time.Duration
	ReconnectMax   time.Duration
}

func (c ConsumerConfig) validate() error {
	if c.URL == "" {
		return errors.New("rabbitmq URL is required")
	}
	if c.Exchange == "" {
		return errors.New("rabbitmq exchange is required")
	}
	if c.Queue == "" {
		return errors.New("rabbitmq queue is required")
	}
	if c.RoutingKey == "" {
		return errors.New("rabbitmq routing key is required")
	}
	if c.Prefetch < 1 {
		return errors.New("rabbitmq prefetch must be at least 1")
	}
	if c.HandlerTimeout <= 0 {
		return errors.New("rabbitmq handler timeout must be greater than zero")
	}
	if c.ReconnectMin <= 0 || c.ReconnectMax < c.ReconnectMin {
		return errors.New("rabbitmq reconnect bounds are invalid")
	}
	return nil
}

func Run(ctx context.Context, config ConsumerConfig, handler Handler, logger *slog.Logger) error {
	if err := config.validate(); err != nil {
		return err
	}

	backoff := config.ReconnectMin
	for {
		if ctx.Err() != nil {
			return nil
		}

		err := consume(ctx, config, handler, logger)
		if ctx.Err() != nil {
			return nil
		}
		if err != nil {
			logger.Error("rabbitmq consumer stopped", "error", err, "retry_in", backoff.String())
		}

		timer := time.NewTimer(backoff)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil
		case <-timer.C:
		}

		backoff = nextBackoff(backoff, config.ReconnectMax)
	}
}

func consume(ctx context.Context, config ConsumerConfig, handler Handler, logger *slog.Logger) error {
	connection, err := amqp.DialConfig(config.URL, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
	})
	if err != nil {
		return fmt.Errorf("connect to rabbitmq: %w", err)
	}
	defer connection.Close()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer channel.Close()

	if err := channel.ExchangeDeclare(config.Exchange, "topic", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare exchange %s: %w", config.Exchange, err)
	}

	queue, err := channel.QueueDeclare(config.Queue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("declare queue %s: %w", config.Queue, err)
	}

	if err := channel.QueueBind(queue.Name, config.RoutingKey, config.Exchange, false, nil); err != nil {
		return fmt.Errorf("bind queue %s: %w", queue.Name, err)
	}

	if err := channel.Qos(config.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set rabbitmq prefetch: %w", err)
	}

	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue %s: %w", queue.Name, err)
	}

	logger.Info("rabbitmq consumer ready",
		"exchange", config.Exchange,
		"queue", queue.Name,
		"routing_key", config.RoutingKey,
		"prefetch", config.Prefetch,
	)

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			handleDelivery(ctx, delivery, config.HandlerTimeout, handler, logger)
		}
	}
}

func handleDelivery(parent context.Context, delivery amqp.Delivery, timeout time.Duration, handler Handler, logger *slog.Logger) {
	ctx, cancel := context.WithTimeout(parent, timeout)
	defer cancel()

	message := Message{
		Body:        delivery.Body,
		ContentType: delivery.ContentType,
		MessageID:   delivery.MessageId,
		RoutingKey:  delivery.RoutingKey,
		Headers:     delivery.Headers,
	}

	if err := handler.Handle(ctx, message); err != nil {
		logger.Error("message handling failed",
			"message_id", message.MessageID,
			"routing_key", message.RoutingKey,
			"error", err,
		)
		if nackErr := delivery.Nack(false, false); nackErr != nil {
			logger.Error("message nack failed", "message_id", message.MessageID, "error", nackErr)
		}
		return
	}

	if err := delivery.Ack(false); err != nil {
		logger.Error("message ack failed", "message_id", message.MessageID, "error", err)
	}
}

func nextBackoff(current, maximum time.Duration) time.Duration {
	next := current * 2
	if next > maximum {
		return maximum
	}
	return next
}
