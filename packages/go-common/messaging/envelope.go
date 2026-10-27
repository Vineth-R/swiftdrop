package messaging

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"
)

type Envelope struct {
	ID            string          `json:"event_id"`
	Type          string          `json:"event_type"`
	Version       int             `json:"event_version"`
	OccurredAt    time.Time       `json:"occurred_at"`
	Producer      string          `json:"producer"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Data          json.RawMessage `json:"data"`
}

func (e Envelope) Validate() error {
	if strings.TrimSpace(e.ID) == "" {
		return errors.New("event_id is required")
	}
	if strings.TrimSpace(e.Type) == "" {
		return errors.New("event_type is required")
	}
	if e.Version < 1 {
		return errors.New("event_version must be at least 1")
	}
	if e.OccurredAt.IsZero() {
		return errors.New("occurred_at is required")
	}
	if strings.TrimSpace(e.Producer) == "" {
		return errors.New("producer is required")
	}

	typeVersion, err := versionFromType(e.Type)
	if err != nil {
		return err
	}
	if e.Version != typeVersion {
		return fmt.Errorf("event_version %d does not match event_type %q", e.Version, e.Type)
	}

	data := bytes.TrimSpace(e.Data)
	if len(data) == 0 || data[0] != '{' {
		return errors.New("data must be a JSON object")
	}
	if !json.Valid(data) {
		return errors.New("data must be valid JSON")
	}

	return nil
}

// versionFromType extracts the positive major version from the .v<major>
// suffix of a versioned event type. It rejects a missing suffix, a non-numeric
// or zero version, and non-canonical versions with leading zeros.
func versionFromType(eventType string) (int, error) {
	separator := strings.LastIndexByte(eventType, '.')
	if separator < 0 || separator == len(eventType)-1 {
		return 0, fmt.Errorf("event_type %q must end with .v<major>", eventType)
	}

	suffix := eventType[separator+1:]
	if len(suffix) < 2 || suffix[0] != 'v' {
		return 0, fmt.Errorf("event_type %q must end with .v<major>", eventType)
	}

	digits := suffix[1:]
	if len(digits) == 0 || digits[0] == '0' {
		return 0, fmt.Errorf("event_type %q must end with .v<major>", eventType)
	}
	for _, digit := range digits {
		if digit < '0' || digit > '9' {
			return 0, fmt.Errorf("event_type %q must end with .v<major>", eventType)
		}
	}

	version, err := strconv.Atoi(digits)
	if err != nil || version < 1 {
		return 0, fmt.Errorf("event_type %q must end with .v<major>", eventType)
	}

	return version, nil
}
