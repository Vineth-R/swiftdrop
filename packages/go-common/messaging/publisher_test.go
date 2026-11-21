package messaging

import (
	"context"
	"errors"
	"testing"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

type confirmCall struct {
	exchange  string
	key       string
	mandatory bool
	message   amqp.Publishing
}

func recordingConfirm(calls *[]confirmCall, confirmed bool, err error) publishConfirmFunc {
	return func(_ context.Context, exchange, key string, mandatory bool, message amqp.Publishing) (bool, error) {
		*calls = append(*calls, confirmCall{
			exchange:  exchange,
			key:       key,
			mandatory: mandatory,
			message:   message,
		})
		return confirmed, err
	}
}

func testTopology() topology {
	return ConsumerConfig{
		Exchange:   "swiftdrop.events",
		Queue:      "notification.requested.v1",
		RoutingKey: "notification.requested.v1",
	}.topology()
}

func TestPublisherPublishesRetry(t *testing.T) {
	delivery := amqp.Delivery{
		Exchange:        "source.exchange",
		RoutingKey:      "source.routing.key",
		Headers:         amqp.Table{"existing": "value"},
		ContentType:     "application/json",
		ContentEncoding: "utf-8",
		DeliveryMode:    amqp.Persistent,
		Priority:        5,
		CorrelationId:   "correlation-1",
		MessageId:       "message-1",
		Type:            "notification.requested.v1",
		AppId:           "test",
		Body:            []byte(`{"ok":true}`),
	}

	publisher := amqpPublisher{
		topology: testTopology(),
	}
	calls := &[]confirmCall{}
	publisher.confirm = recordingConfirm(calls, true, nil)

	if err := publisher.PublishRetry(context.Background(), delivery, 2, 150*time.Millisecond, "retryable"); err != nil {
		t.Fatalf("PublishRetry() returned error: %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("confirm calls = %d, want 1", len(*calls))
	}

	call := (*calls)[0]
	if call.exchange != "swiftdrop.events.retry" {
		t.Fatalf("exchange = %q, want swiftdrop.events.retry", call.exchange)
	}
	if call.key != "notification.requested.v1.retry.v2" {
		t.Fatalf("routing key = %q, want notification.requested.v1.retry.v2", call.key)
	}
	if !call.mandatory {
		t.Fatal("mandatory = false, want true")
	}
	if call.message.Expiration != "150" {
		t.Fatalf("Expiration = %q, want 150", call.message.Expiration)
	}
	if call.message.DeliveryMode != amqp.Persistent {
		t.Fatalf("DeliveryMode = %d, want %d", call.message.DeliveryMode, amqp.Persistent)
	}
	if got := call.message.Headers[retryCountHeader]; got != int32(2) {
		t.Fatalf("%s = %v, want 2", retryCountHeader, got)
	}
	if got := call.message.Headers[originalExchangeHeader]; got != "source.exchange" {
		t.Fatalf("%s = %v", originalExchangeHeader, got)
	}
	if got := call.message.Headers[originalRoutingKeyHeader]; got != "source.routing.key" {
		t.Fatalf("%s = %v", originalRoutingKeyHeader, got)
	}
	if got := call.message.Headers[failureReasonHeader]; got != "retryable" {
		t.Fatalf("%s = %v", failureReasonHeader, got)
	}
	if got := call.message.Headers[failedAtHeader]; got == "" || got == nil {
		t.Fatalf("%s is empty", failedAtHeader)
	}
	if got := call.message.Headers["existing"]; got != "value" {
		t.Fatalf("existing header = %v, want value", got)
	}
	if delivery.Headers[retryCountHeader] != nil {
		t.Fatal("PublishRetry mutated the original delivery headers")
	}

	if call.message.ContentType != delivery.ContentType {
		t.Fatalf("ContentType = %q", call.message.ContentType)
	}
	if call.message.ContentEncoding != delivery.ContentEncoding {
		t.Fatalf("ContentEncoding = %q", call.message.ContentEncoding)
	}
	if call.message.Priority != delivery.Priority {
		t.Fatalf("Priority = %d", call.message.Priority)
	}
	if call.message.CorrelationId != delivery.CorrelationId {
		t.Fatalf("CorrelationId = %q", call.message.CorrelationId)
	}
	if call.message.MessageId != delivery.MessageId {
		t.Fatalf("MessageId = %q", call.message.MessageId)
	}
	if call.message.Type != delivery.Type {
		t.Fatalf("Type = %q", call.message.Type)
	}
	if call.message.AppId != delivery.AppId {
		t.Fatalf("AppId = %q", call.message.AppId)
	}
	if string(call.message.Body) != string(delivery.Body) {
		t.Fatalf("Body = %q", call.message.Body)
	}
	if call.message.Timestamp.IsZero() {
		t.Fatal("Timestamp is zero")
	}
}

func TestPublisherPublishesDeadLetter(t *testing.T) {
	delivery := amqp.Delivery{
		Headers:       amqp.Table{retryCountHeader: int32(3)},
		ContentType:   "application/json",
		CorrelationId: "correlation-1",
		MessageId:     "message-1",
		Body:          []byte(`{"ok":true}`),
	}

	publisher := amqpPublisher{
		topology: testTopology(),
	}
	calls := &[]confirmCall{}
	publisher.confirm = recordingConfirm(calls, true, nil)

	if err := publisher.PublishDeadLetter(context.Background(), delivery, "permanent"); err != nil {
		t.Fatalf("PublishDeadLetter() returned error: %v", err)
	}

	if len(*calls) != 1 {
		t.Fatalf("confirm calls = %d, want 1", len(*calls))
	}

	call := (*calls)[0]
	if call.exchange != "swiftdrop.events.dlx" {
		t.Fatalf("exchange = %q, want swiftdrop.events.dlx", call.exchange)
	}
	if call.key != "notification.requested.v1.dlq" {
		t.Fatalf("routing key = %q, want notification.requested.v1.dlq", call.key)
	}
	if call.message.Expiration != "" {
		t.Fatalf("Expiration = %q, want empty", call.message.Expiration)
	}
	if got := call.message.Headers[retryCountHeader]; got != int32(3) {
		t.Fatalf("%s = %v, want 3", retryCountHeader, got)
	}
	if got := call.message.Headers[failureReasonHeader]; got != "permanent" {
		t.Fatalf("%s = %v", failureReasonHeader, got)
	}
}

func TestPublisherRejectsUnconfirmedPublications(t *testing.T) {
	tests := []struct {
		name      string
		confirmed bool
		err       error
	}{
		{name: "not confirmed", confirmed: false},
		{name: "confirm error", confirmed: false, err: errors.New("channel closed")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			calls := &[]confirmCall{}
			publisher := amqpPublisher{
				topology: testTopology(),
				confirm:  recordingConfirm(calls, tt.confirmed, tt.err),
			}

			if err := publisher.PublishRetry(context.Background(), amqp.Delivery{}, 1, time.Second, "retryable"); err == nil {
				t.Fatal("PublishRetry() should have failed")
			}
		})
	}
}

func TestPublisherRequiresConfirms(t *testing.T) {
	publisher := amqpPublisher{topology: testTopology()}

	if err := publisher.PublishDeadLetter(context.Background(), amqp.Delivery{}, "permanent"); err == nil {
		t.Fatal("PublishDeadLetter() should fail without publisher confirms")
	}
}
