package messaging

import (
	"testing"
	"time"
)

func TestConsumerConfigValidate(t *testing.T) {
	valid := ConsumerConfig{
		URL:            "amqp://guest:guest@localhost:5672/",
		Exchange:       "swiftdrop.events",
		Queue:          "notification.requested.v1",
		RoutingKey:     "notification.requested.v1",
		Prefetch:       10,
		HandlerTimeout: 30 * time.Second,
		ReconnectMin:   time.Second,
		ReconnectMax:   30 * time.Second,
	}

	if err := valid.validate(); err != nil {
		t.Fatalf("valid config rejected: %v", err)
	}

	tests := map[string]func(*ConsumerConfig){
		"missing url":             func(c *ConsumerConfig) { c.URL = "" },
		"missing exchange":        func(c *ConsumerConfig) { c.Exchange = "" },
		"missing queue":           func(c *ConsumerConfig) { c.Queue = "" },
		"missing routing key":     func(c *ConsumerConfig) { c.RoutingKey = "" },
		"invalid prefetch":        func(c *ConsumerConfig) { c.Prefetch = 0 },
		"invalid handler timeout": func(c *ConsumerConfig) { c.HandlerTimeout = 0 },
		"invalid reconnect bounds": func(c *ConsumerConfig) {
			c.ReconnectMin = 10 * time.Second
			c.ReconnectMax = time.Second
		},
	}

	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			config := valid
			mutate(&config)
			if err := config.validate(); err == nil {
				t.Fatal("validate() should have failed")
			}
		})
	}
}

func TestNextBackoff(t *testing.T) {
	if got := nextBackoff(time.Second, 30*time.Second); got != 2*time.Second {
		t.Fatalf("nextBackoff() = %v, want 2s", got)
	}
	if got := nextBackoff(20*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff() = %v, want 30s", got)
	}
	if got := nextBackoff(30*time.Second, 30*time.Second); got != 30*time.Second {
		t.Fatalf("nextBackoff() = %v, want 30s", got)
	}
}
