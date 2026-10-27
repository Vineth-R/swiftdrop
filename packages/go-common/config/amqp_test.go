package config

import (
	"strings"
	"testing"
)

func TestValidateAMQPURL(t *testing.T) {
	valid := []string{
		"amqp://swiftdrop:swiftdrop@rabbitmq:5672/",
		"amqps://user:password@rabbitmq:5671/vhost",
		"AMQP://rabbitmq:5672",
	}

	for _, value := range valid {
		if err := ValidateAMQPURL(value); err != nil {
			t.Fatalf("ValidateAMQPURL(%q) returned error: %v", value, err)
		}
	}

	invalid := map[string]string{
		"empty":        "",
		"whitespace":   "   ",
		"wrong scheme": "http://rabbitmq:5672",
		"missing host": "amqp:///vhost",
		"malformed":    "amqp://user:password@rabbitmq:5672/%zz",
	}

	for name, value := range invalid {
		t.Run(name, func(t *testing.T) {
			if err := ValidateAMQPURL(value); err == nil {
				t.Fatal("ValidateAMQPURL() should have failed")
			}
		})
	}
}

func TestValidateAMQPURLRedactsCredentials(t *testing.T) {
	tests := []string{
		"amqp://swiftdrop:s3cr3t-password@rabbitmq:5672/%zz",
		"http://swiftdrop:s3cr3t-password@rabbitmq:5672",
		"amqp://swiftdrop:s3cr3t-password@/vhost",
	}

	for _, value := range tests {
		err := ValidateAMQPURL(value)
		if err == nil {
			t.Fatalf("ValidateAMQPURL(%q) should have failed", value)
		}
		if strings.Contains(err.Error(), "s3cr3t-password") {
			t.Fatalf("error leaked the password: %q", err.Error())
		}
		if strings.Contains(err.Error(), "swiftdrop") {
			t.Fatalf("error leaked the username: %q", err.Error())
		}
	}
}
