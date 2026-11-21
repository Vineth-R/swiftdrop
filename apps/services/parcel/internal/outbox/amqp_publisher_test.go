package outbox

import (
	"context"
	"errors"
	"testing"

	amqp "github.com/rabbitmq/amqp091-go"
)

func TestCheckReturn(t *testing.T) {
	t.Run("nil returns channel", func(t *testing.T) {
		if err := checkReturn(nil, "swiftdrop.events", "parcel.assigned.v1"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("empty returns channel", func(t *testing.T) {
		ch := make(chan amqp.Return, 1)
		if err := checkReturn(ch, "swiftdrop.events", "parcel.assigned.v1"); err != nil {
			t.Fatalf("expected nil error, got %v", err)
		}
	})

	t.Run("returned unroutable message", func(t *testing.T) {
		ch := make(chan amqp.Return, 1)
		ch <- amqp.Return{
			ReplyCode:  312,
			ReplyText:  "NO_ROUTE",
			Exchange:   "swiftdrop.events",
			RoutingKey: "unroutable.key",
		}
		err := checkReturn(ch, "swiftdrop.events", "unroutable.key")
		if err == nil {
			t.Fatal("expected error for returned message, got nil")
		}
		expected := "rabbitmq returned unroutable message from swiftdrop.events with routing key unroutable.key: 312 NO_ROUTE"
		if err.Error() != expected {
			t.Fatalf("expected %q, got %q", expected, err.Error())
		}
	})
}

func TestDrainReturns(t *testing.T) {
	ch := make(chan amqp.Return, 5)
	for i := 0; i < 3; i++ {
		ch <- amqp.Return{ReplyCode: uint16(300 + i)}
	}
	drainReturns(ch)

	select {
	case ret := <-ch:
		t.Fatalf("expected empty channel, got %+v", ret)
	default:
		// channel drained successfully
	}
}

func TestAMQPPublisherWithConfirmFn(t *testing.T) {
	pub := NewAMQPPublisher("amqp://localhost:5672", "swiftdrop.events")

	var publishedKey string
	pub.confirmFn = func(ctx context.Context, exchange, routingKey string, body []byte) error {
		publishedKey = routingKey
		return nil
	}

	err := pub.Publish(context.Background(), "swiftdrop.events", "parcel.created.v1", []byte(`{}`))
	if err != nil {
		t.Fatalf("unexpected publish error: %v", err)
	}
	if publishedKey != "parcel.created.v1" {
		t.Fatalf("expected parcel.created.v1, got %q", publishedKey)
	}

	pub.confirmFn = func(ctx context.Context, exchange, routingKey string, body []byte) error {
		return errors.New("confirm failed")
	}
	err = pub.Publish(context.Background(), "swiftdrop.events", "parcel.created.v1", []byte(`{}`))
	if err == nil || err.Error() != "confirm failed" {
		t.Fatalf("expected 'confirm failed', got %v", err)
	}
}
