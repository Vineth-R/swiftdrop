package messaging

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	retryCountHeader         = "x-swiftdrop-retry-count"
	originalExchangeHeader   = "x-swiftdrop-original-exchange"
	originalRoutingKeyHeader = "x-swiftdrop-original-routing-key"
	failureReasonHeader      = "x-swiftdrop-failure-reason"
	failedAtHeader           = "x-swiftdrop-failed-at"
	publishConfirmTimeout    = 10 * time.Second
)

type failurePublisher interface {
	PublishRetry(ctx context.Context, delivery amqp.Delivery, retryCount int, delay time.Duration, reason string) error
	PublishDeadLetter(ctx context.Context, delivery amqp.Delivery, reason string) error
}

type publishConfirmFunc func(ctx context.Context, exchange, key string, mandatory bool, message amqp.Publishing) (bool, error)

type amqpPublisher struct {
	topology topology
	confirm  publishConfirmFunc
}

func newAMQPPublisher(channel *amqp.Channel, t topology) (*amqpPublisher, error) {
	returns := channel.NotifyReturn(make(chan amqp.Return, 1))
	if err := channel.Confirm(false); err != nil {
		return nil, fmt.Errorf("enable publisher confirms: %w", err)
	}

	return &amqpPublisher{
		topology: t,
		confirm: func(ctx context.Context, exchange, key string, mandatory bool, message amqp.Publishing) (bool, error) {
			return publishWithConfirm(ctx, channel, returns, exchange, key, mandatory, message)
		},
	}, nil
}

func (p *amqpPublisher) PublishRetry(ctx context.Context, delivery amqp.Delivery, retryCount int, delay time.Duration, reason string) error {
	headers := failureHeaders(delivery, p.topology, reason)
	headers[retryCountHeader] = int32(retryCount)

	message := publishingFromDelivery(delivery, headers)
	message.Expiration = strconv.FormatInt(delay.Milliseconds(), 10)

	return p.publish(ctx, p.topology.RetryExchange, p.topology.RetryRoutingKey, message)
}

func (p *amqpPublisher) PublishDeadLetter(ctx context.Context, delivery amqp.Delivery, reason string) error {
	headers := failureHeaders(delivery, p.topology, reason)
	message := publishingFromDelivery(delivery, headers)

	return p.publish(ctx, p.topology.DeadLetterExchange, p.topology.DeadLetterRoutingKey, message)
}

func (p *amqpPublisher) publish(ctx context.Context, exchange, key string, message amqp.Publishing) error {
	if p.confirm == nil {
		return errors.New("publisher confirms are not configured")
	}

	confirmed, err := p.confirm(ctx, exchange, key, true, message)
	if err != nil {
		return err
	}
	if !confirmed {
		return fmt.Errorf("rabbitmq did not confirm publication to %s with routing key %s", exchange, key)
	}

	return nil
}

func publishWithConfirm(ctx context.Context, channel *amqp.Channel, returns <-chan amqp.Return, exchange, key string, mandatory bool, message amqp.Publishing) (bool, error) {
	publishCtx, cancel := context.WithTimeout(ctx, publishConfirmTimeout)
	defer cancel()

	drainReturns(returns)

	confirmation, err := channel.PublishWithDeferredConfirmWithContext(publishCtx, exchange, key, mandatory, false, message)
	if err != nil {
		return false, fmt.Errorf("publish to %s: %w", exchange, err)
	}

	confirmed, err := confirmation.WaitContext(publishCtx)
	if err != nil {
		return false, fmt.Errorf("wait for publisher confirm: %w", err)
	}
	if !confirmed {
		return false, nil
	}

	select {
	case returned, ok := <-returns:
		if ok {
			return false, fmt.Errorf("rabbitmq returned message from %s with routing key %s: %d %s", exchange, key, returned.ReplyCode, returned.ReplyText)
		}
	default:
	}

	return true, nil
}

func drainReturns(returns <-chan amqp.Return) {
	for {
		select {
		case _, ok := <-returns:
			if !ok {
				return
			}
		default:
			return
		}
	}
}

func failureHeaders(delivery amqp.Delivery, t topology, reason string) amqp.Table {
	headers := make(amqp.Table, len(delivery.Headers)+5)
	for key, value := range delivery.Headers {
		headers[key] = value
	}

	originalExchange := delivery.Exchange
	if originalExchange == "" {
		originalExchange = t.Exchange
	}

	originalRoutingKey := delivery.RoutingKey
	if originalRoutingKey == "" {
		originalRoutingKey = t.RoutingKey
	}

	headers[originalExchangeHeader] = originalExchange
	headers[originalRoutingKeyHeader] = originalRoutingKey
	headers[failureReasonHeader] = reason
	headers[failedAtHeader] = time.Now().UTC().Format(time.RFC3339Nano)

	return headers
}

func publishingFromDelivery(delivery amqp.Delivery, headers amqp.Table) amqp.Publishing {
	return amqp.Publishing{
		Headers:         headers,
		ContentType:     delivery.ContentType,
		ContentEncoding: delivery.ContentEncoding,
		DeliveryMode:    amqp.Persistent,
		Priority:        delivery.Priority,
		CorrelationId:   delivery.CorrelationId,
		MessageId:       delivery.MessageId,
		Timestamp:       time.Now().UTC(),
		Type:            delivery.Type,
		AppId:           delivery.AppId,
		Body:            delivery.Body,
	}
}
