package messaging

import (
	"fmt"
	"mime"
	"strings"
)

// MaxMessageBytes is the maximum accepted RabbitMQ message body size (1 MiB).
const MaxMessageBytes = 1 << 20

// ValidateMessage enforces transport-level constraints that must be checked
// before a handler unmarshals a message body. An empty content type is allowed
// for compatibility; otherwise only application/json is accepted, including
// optional parameters such as charset. Rejected messages should be classified
// as permanent failures.
func ValidateMessage(contentType string, body []byte) error {
	if len(body) > MaxMessageBytes {
		return fmt.Errorf("message body exceeds %d bytes", MaxMessageBytes)
	}

	if strings.TrimSpace(contentType) == "" {
		return nil
	}

	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return fmt.Errorf("invalid content type: %w", err)
	}
	if mediaType != "application/json" {
		return fmt.Errorf("unsupported content type %q", mediaType)
	}

	return nil
}
