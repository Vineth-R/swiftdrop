package messaging

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net"
	"net/url"
	"runtime/debug"
	"strings"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

var errHandlerPanic = errors.New("handler panicked")

const dialTimeout = 10 * time.Second

type Handler interface {
	Handle(ctx context.Context, message Message) error
}

type Message struct {
	Body          []byte
	ContentType   string
	MessageID     string
	CorrelationID string
	RoutingKey    string
	Redelivered   bool
	Headers       map[string]any
}

// Observer receives low-cardinality processing facts. Implementations must be
// safe for concurrent use and must not put message content in a metric label.
type Observer struct {
	Processed func(queue string, duration time.Duration)
	Failed    func(queue string, permanent bool)
	Retried   func(queue string)
}

type ConsumerConfig struct {
	URL            string
	Exchange       string
	Queue          string
	RoutingKey     string
	Prefetch       int
	HandlerTimeout time.Duration
	MaxRetries     int
	RetryDelay     time.Duration
	ReconnectMin   time.Duration
	ReconnectMax   time.Duration
	// Health optionally records the consumer lifecycle so a readiness endpoint
	// can report whether this consumer can process events. A nil value is a
	// no-op and leaves existing callers unchanged.
	Health *Health
	// Observer optionally records processing outcomes for metrics.
	Observer *Observer
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
	if c.MaxRetries < 0 {
		return errors.New("rabbitmq max retries must not be negative")
	}
	if c.RetryDelay < time.Millisecond {
		return errors.New("rabbitmq retry delay must be at least 1ms")
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

	c := consumer{
		config:  config,
		handler: handler,
		logger:  logger,
		consume: consumeOnce,
		wait:    waitContext,
		randN:   rand.Int64N,
	}
	return c.run(ctx)
}

type consumeFunc func(ctx context.Context, config ConsumerConfig, handler Handler, logger *slog.Logger, onReady func()) error

type consumer struct {
	config  ConsumerConfig
	handler Handler
	logger  *slog.Logger
	consume consumeFunc
	wait    func(context.Context, time.Duration) error
	randN   func(int64) int64
}

func (c consumer) run(ctx context.Context) error {
	backoff := c.config.ReconnectMin
	c.config.Health.MarkConnecting()

	for {
		if ctx.Err() != nil {
			c.config.Health.MarkStopped()
			return nil
		}

		ready := false
		err := c.consume(ctx, c.config, c.handler, c.logger, func() {
			ready = true
			c.config.Health.MarkReady()
		})
		if ctx.Err() != nil {
			c.config.Health.MarkStopped()
			return nil
		}

		c.config.Health.MarkReconnecting(err)

		// A session that reached the consume phase proves the broker and
		// topology are reachable, so the next failure starts from the floor
		// again instead of inheriting an old outage's backoff.
		if ready {
			backoff = c.config.ReconnectMin
		}

		delay := jitter(backoff, c.randN)
		if err != nil {
			c.logger.Error("rabbitmq consumer stopped", "error", err, "retry_in", delay.String())
		}

		if err := c.wait(ctx, delay); err != nil {
			c.config.Health.MarkStopped()
			return nil
		}

		backoff = nextBackoff(backoff, c.config.ReconnectMax)
	}
}

func consumeOnce(ctx context.Context, config ConsumerConfig, handler Handler, logger *slog.Logger, onReady func()) error {
	connection, err := dialAMQP(ctx, config.URL)
	if err != nil {
		return fmt.Errorf("connect to rabbitmq: %w", err)
	}
	defer connection.Close()

	channel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq channel: %w", err)
	}
	defer channel.Close()

	t := config.topology()
	queue, err := declareConsumerTopology(channel, t)
	if err != nil {
		return err
	}

	publisherChannel, err := connection.Channel()
	if err != nil {
		return fmt.Errorf("open rabbitmq publisher channel: %w", err)
	}
	defer publisherChannel.Close()

	publisher, err := newAMQPPublisher(publisherChannel, t)
	if err != nil {
		return err
	}

	if err := channel.Qos(config.Prefetch, 0, false); err != nil {
		return fmt.Errorf("set rabbitmq prefetch: %w", err)
	}

	deliveries, err := channel.Consume(queue.Name, "", false, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("consume queue %s: %w", queue.Name, err)
	}

	if onReady != nil {
		onReady()
	}

	logger.Info("rabbitmq consumer ready",
		"exchange", config.Exchange,
		"queue", queue.Name,
		"routing_key", config.RoutingKey,
		"prefetch", config.Prefetch,
		"max_retries", config.MaxRetries,
		"retry_delay", config.RetryDelay.String(),
	)

	processor := deliveryProcessor{
		handler:    handler,
		publisher:  publisher,
		timeout:    config.HandlerTimeout,
		maxRetries: config.MaxRetries,
		retryDelay: config.RetryDelay,
		logger:     logger,
		queue:      queue.Name,
		observer:   config.Observer,
	}

	for {
		select {
		case <-ctx.Done():
			return nil
		case delivery, ok := <-deliveries:
			if !ok {
				return errors.New("rabbitmq delivery channel closed")
			}
			processor.handle(ctx, delivery)
		}
	}
}

func dialAMQP(ctx context.Context, url string) (*amqp.Connection, error) {
	dialer := &net.Dialer{
		Timeout:   dialTimeout,
		KeepAlive: 30 * time.Second,
	}

	connection, err := amqp.DialConfig(url, amqp.Config{
		Heartbeat: 10 * time.Second,
		Locale:    "en_US",
		Dial: func(network, addr string) (net.Conn, error) {
			conn, err := dialer.DialContext(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			// amqp091-go cannot cancel TLS/AMQP handshaking with a context, so a
			// deadline bounds it. The library clears it once the connection opens.
			if err := conn.SetDeadline(time.Now().Add(dialTimeout)); err != nil {
				_ = conn.Close()
				return nil, err
			}
			return conn, nil
		},
	})
	if err != nil {
		return nil, redactAMQPError(err, url)
	}
	return connection, nil
}

// redactAMQPError removes connection URL credentials from errors returned by
// the AMQP client. Some URL parse errors embed the raw URL, including its
// userinfo, so the error is replaced with a generic message in that case.
func redactAMQPError(err error, rawURL string) error {
	if err == nil {
		return nil
	}

	message := err.Error()
	if rawURL != "" && strings.Contains(message, rawURL) {
		return errors.New("connect to rabbitmq: invalid connection URL")
	}

	parsed, parseErr := url.Parse(rawURL)
	if parseErr == nil && parsed.User != nil {
		if password, ok := parsed.User.Password(); ok && password != "" && strings.Contains(message, password) {
			return errors.New("connect to rabbitmq: connection URL credentials must not be logged")
		}
	}

	return err
}

func waitContext(ctx context.Context, delay time.Duration) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if delay <= 0 {
		return nil
	}

	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

type deliveryProcessor struct {
	handler    Handler
	publisher  failurePublisher
	timeout    time.Duration
	maxRetries int
	retryDelay time.Duration
	logger     *slog.Logger
	queue      string
	observer   *Observer
}

func (p deliveryProcessor) handle(parent context.Context, delivery amqp.Delivery) {
	ctx, cancel := context.WithTimeout(parent, p.timeout)
	defer cancel()

	message := messageFromDelivery(delivery)
	started := time.Now()
	err := invokeHandler(ctx, p.handler, message, delivery, p.logger)

	switch {
	case err == nil:
		p.observeProcessed(started)
		ackDelivery(delivery, message, p.logger)
	case parent.Err() != nil:
		p.logger.Warn("message handling interrupted by shutdown",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"routing_key", message.RoutingKey,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"requeue", true,
			"error", err,
		)
		nackDelivery(delivery, true, message, p.logger)
	case errors.Is(err, errHandlerPanic):
		p.routeFailure(parent, delivery, message, err, "panic")
	case IsPermanent(err):
		p.routeFailure(parent, delivery, message, err, "permanent")
	default:
		p.routeFailure(parent, delivery, message, err, "retryable")
	}
}

func (p deliveryProcessor) routeFailure(parent context.Context, delivery amqp.Delivery, message Message, handlerErr error, failureClass string) {
	attempts := retryCountFromHeaders(delivery.Headers)

	if failureClass == "permanent" {
		p.observeFailed(true)
		p.deadLetter(parent, delivery, message, handlerErr, "permanent")
		return
	}

	if attempts >= p.maxRetries {
		p.observeFailed(false)
		p.deadLetter(parent, delivery, message, handlerErr, "retry_exhausted")
		return
	}

	if p.publisher == nil {
		p.logger.Error("message failure publisher is not configured",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"routing_key", message.RoutingKey,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"failure_class", failureClass,
			"requeue", true,
			"error", handlerErr,
		)
		nackDelivery(delivery, true, message, p.logger)
		return
	}

	if err := p.publisher.PublishRetry(parent, delivery, attempts+1, p.retryDelay, failureClass); err != nil {
		p.logger.Error("message retry scheduling failed",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"routing_key", message.RoutingKey,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"failure_class", failureClass,
			"requeue", true,
			"error", err,
		)
		nackDelivery(delivery, true, message, p.logger)
		return
	}

	if p.observer != nil && p.observer.Retried != nil {
		p.observer.Retried(p.queue)
	}
	p.logger.Warn("message scheduled for retry",
		"message_id", message.MessageID,
		"correlation_id", message.CorrelationID,
		"routing_key", message.RoutingKey,
		"delivery_tag", delivery.DeliveryTag,
		"redelivered", message.Redelivered,
		"attempt", attempts+1,
		"max_retries", p.maxRetries,
		"retry_delay", p.retryDelay.String(),
		"failure_class", failureClass,
		"error", handlerErr,
	)
	ackDelivery(delivery, message, p.logger)
}

func (p deliveryProcessor) observeProcessed(started time.Time) {
	if p.observer == nil || p.observer.Processed == nil {
		return
	}
	p.observer.Processed(p.queue, time.Since(started))
}

func (p deliveryProcessor) observeFailed(permanent bool) {
	if p.observer == nil || p.observer.Failed == nil {
		return
	}
	p.observer.Failed(p.queue, permanent)
}

func (p deliveryProcessor) deadLetter(parent context.Context, delivery amqp.Delivery, message Message, handlerErr error, reason string) {
	if p.publisher == nil {
		p.logger.Error("message dead-letter publisher is not configured",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"routing_key", message.RoutingKey,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"reason", reason,
			"requeue", true,
			"error", handlerErr,
		)
		nackDelivery(delivery, true, message, p.logger)
		return
	}

	if err := p.publisher.PublishDeadLetter(parent, delivery, reason); err != nil {
		p.logger.Error("message dead-letter publication failed",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"routing_key", message.RoutingKey,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"reason", reason,
			"requeue", true,
			"error", err,
		)
		nackDelivery(delivery, true, message, p.logger)
		return
	}

	p.logger.Error("message dead-lettered",
		"message_id", message.MessageID,
		"correlation_id", message.CorrelationID,
		"routing_key", message.RoutingKey,
		"delivery_tag", delivery.DeliveryTag,
		"redelivered", message.Redelivered,
		"reason", reason,
		"error", handlerErr,
	)
	ackDelivery(delivery, message, p.logger)
}

func messageFromDelivery(delivery amqp.Delivery) Message {
	return Message{
		Body:          delivery.Body,
		ContentType:   delivery.ContentType,
		MessageID:     delivery.MessageId,
		CorrelationID: delivery.CorrelationId,
		RoutingKey:    delivery.RoutingKey,
		Redelivered:   delivery.Redelivered,
		Headers:       delivery.Headers,
	}
}

func retryCountFromHeaders(headers amqp.Table) int {
	value, ok := headers[retryCountHeader]
	if !ok {
		return 0
	}

	switch count := value.(type) {
	case int:
		if count > 0 {
			return count
		}
	case int32:
		if count > 0 {
			return int(count)
		}
	case int64:
		if count > 0 {
			return int(count)
		}
	}

	return 0
}

func invokeHandler(ctx context.Context, handler Handler, message Message, delivery amqp.Delivery, logger *slog.Logger) (err error) {
	if err := ValidateMessage(message.ContentType, message.Body); err != nil {
		return Permanent(err)
	}

	defer func() {
		if recovered := recover(); recovered != nil {
			logger.Error("message handler panicked",
				"message_id", message.MessageID,
				"correlation_id", message.CorrelationID,
				"routing_key", message.RoutingKey,
				"delivery_tag", delivery.DeliveryTag,
				"redelivered", message.Redelivered,
				"panic", recovered,
				"stack", string(debug.Stack()),
			)
			err = errHandlerPanic
		}
	}()

	return handler.Handle(ctx, message)
}

func ackDelivery(delivery amqp.Delivery, message Message, logger *slog.Logger) {
	if err := delivery.Ack(false); err != nil {
		logger.Error("message ack failed",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"error", err,
		)
	}
}

func nackDelivery(delivery amqp.Delivery, requeue bool, message Message, logger *slog.Logger) {
	if err := delivery.Nack(false, requeue); err != nil {
		logger.Error("message nack failed",
			"message_id", message.MessageID,
			"correlation_id", message.CorrelationID,
			"delivery_tag", delivery.DeliveryTag,
			"redelivered", message.Redelivered,
			"requeue", requeue,
			"error", err,
		)
	}
}

func nextBackoff(current, maximum time.Duration) time.Duration {
	if current >= maximum {
		return maximum
	}

	next := current * 2
	if next < current || next > maximum {
		return maximum
	}
	return next
}

func jitter(base time.Duration, randN func(int64) int64) time.Duration {
	if base <= 0 {
		return 0
	}

	half := base / 2
	if half <= 0 {
		return base
	}

	return half + time.Duration(randN(int64(half)+1))
}
