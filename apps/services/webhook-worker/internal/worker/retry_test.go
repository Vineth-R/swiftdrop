package worker

import (
	"testing"
	"time"
)

func TestDelayQueueName(t *testing.T) {
	got := delayQueueName("webhook.parcel.status_changed.v1", 5*time.Minute)
	if got != "webhook.parcel.status_changed.v1.delay.300s" {
		t.Fatalf("name = %q", got)
	}
}

func TestDelayQueueArgs(t *testing.T) {
	args := delayQueueArgs("q", 2*time.Hour)
	if args["x-message-ttl"] != int64(7200000) {
		t.Fatalf("ttl = %v", args["x-message-ttl"])
	}
	if args["x-dead-letter-exchange"] != "" || args["x-dead-letter-routing-key"] != "q" {
		t.Fatalf("dead-letter args = %v", args)
	}
}

func TestAttemptFromHeaders(t *testing.T) {
	if n, err := attemptFromHeaders(nil); err != nil || n != 1 {
		t.Fatalf("no header = %d, %v", n, err)
	}
	if n, err := attemptFromHeaders(map[string]any{attemptHeader: int32(4)}); err != nil || n != 4 {
		t.Fatalf("int32 = %d, %v", n, err)
	}
	if _, err := attemptFromHeaders(map[string]any{attemptHeader: 1.5}); err == nil {
		t.Fatal("float must be rejected")
	}
}

func TestScheduleRejectsShortDelay(t *testing.T) {
	s := NewAMQPScheduler("amqp://guest:guest@localhost:5672/", "q")
	if err := s.Schedule(t.Context(), messageStub(), 2, time.Millisecond); err == nil {
		t.Fatal("a delay under 1s must be rejected")
	}
}
