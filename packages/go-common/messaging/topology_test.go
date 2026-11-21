package messaging

import "testing"

func TestConsumerConfigTopology(t *testing.T) {
	config := ConsumerConfig{
		Exchange:   "swiftdrop.events",
		Queue:      "notification.requested.v1",
		RoutingKey: "notification.requested.v1",
	}

	got := config.topology()
	want := topology{
		Exchange:             "swiftdrop.events",
		Queue:                "notification.requested.v1",
		RoutingKey:           "notification.requested.v1",
		RetryExchange:        "swiftdrop.events.retry",
		RetryQueue:           "notification.requested.v1.retry.v2",
		RetryRoutingKey:      "notification.requested.v1.retry.v2",
		DeadLetterExchange:   "swiftdrop.events.dlx",
		DeadLetterQueue:      "notification.requested.v1.dlq",
		DeadLetterRoutingKey: "notification.requested.v1.dlq",
	}

	if got != want {
		t.Fatalf("topology() = %+v, want %+v", got, want)
	}
}
