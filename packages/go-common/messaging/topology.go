package messaging

import (
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

// retryQueueSuffix names the retry queue for the current failure topology.
// Durable queue arguments are immutable, so changing the dead-letter target
// requires a new queue name. The v2 retry queue returns messages to its owning
// queue; the legacy .retry queue returned them to the shared domain exchange,
// which fanned a retry out to every worker bound to that event.
const retryQueueSuffix = ".retry.v2"

type topology struct {
	Exchange             string
	Queue                string
	RoutingKey           string
	RetryExchange        string
	RetryQueue           string
	RetryRoutingKey      string
	DeadLetterExchange   string
	DeadLetterQueue      string
	DeadLetterRoutingKey string
}

func (c ConsumerConfig) topology() topology {
	return topology{
		Exchange:             c.Exchange,
		Queue:                c.Queue,
		RoutingKey:           c.RoutingKey,
		RetryExchange:        c.Exchange + ".retry",
		RetryQueue:           c.Queue + retryQueueSuffix,
		RetryRoutingKey:      c.Queue + retryQueueSuffix,
		DeadLetterExchange:   c.Exchange + ".dlx",
		DeadLetterQueue:      c.Queue + ".dlq",
		DeadLetterRoutingKey: c.Queue + ".dlq",
	}
}

func declareConsumerTopology(channel *amqp.Channel, t topology) (amqp.Queue, error) {
	if err := channel.ExchangeDeclare(t.Exchange, "topic", true, false, false, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("declare exchange %s: %w", t.Exchange, err)
	}

	queue, err := channel.QueueDeclare(t.Queue, true, false, false, false, nil)
	if err != nil {
		return amqp.Queue{}, fmt.Errorf("declare queue %s: %w", t.Queue, err)
	}

	if err := channel.QueueBind(queue.Name, t.RoutingKey, t.Exchange, false, nil); err != nil {
		return amqp.Queue{}, fmt.Errorf("bind queue %s: %w", queue.Name, err)
	}

	if err := declareFailureTopology(channel, t); err != nil {
		return amqp.Queue{}, err
	}

	return queue, nil
}

func declareFailureTopology(channel *amqp.Channel, t topology) error {
	if err := channel.ExchangeDeclare(t.RetryExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare retry exchange %s: %w", t.RetryExchange, err)
	}

	retryQueue, err := channel.QueueDeclare(t.RetryQueue, true, false, false, false, amqp.Table{
		"x-queue-type": "classic",
		// Return the retry to the queue that owns it. The default exchange
		// routes by queue name, so another worker bound to the same domain
		// routing key never receives this retry. Returning through the shared
		// domain exchange would fan the retry out to every subscriber.
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": t.Queue,
	})
	if err != nil {
		return fmt.Errorf("declare retry queue %s: %w", t.RetryQueue, err)
	}

	if err := channel.QueueBind(retryQueue.Name, t.RetryRoutingKey, t.RetryExchange, false, nil); err != nil {
		return fmt.Errorf("bind retry queue %s: %w", retryQueue.Name, err)
	}

	if err := channel.ExchangeDeclare(t.DeadLetterExchange, "direct", true, false, false, false, nil); err != nil {
		return fmt.Errorf("declare dead-letter exchange %s: %w", t.DeadLetterExchange, err)
	}

	deadLetterQueue, err := channel.QueueDeclare(t.DeadLetterQueue, true, false, false, false, amqp.Table{
		"x-queue-type": "classic",
	})
	if err != nil {
		return fmt.Errorf("declare dead-letter queue %s: %w", t.DeadLetterQueue, err)
	}

	if err := channel.QueueBind(deadLetterQueue.Name, t.DeadLetterRoutingKey, t.DeadLetterExchange, false, nil); err != nil {
		return fmt.Errorf("bind dead-letter queue %s: %w", deadLetterQueue.Name, err)
	}

	return nil
}
