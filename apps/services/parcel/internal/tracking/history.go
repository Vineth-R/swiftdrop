package tracking

import (
	"encoding/json"
	"fmt"
)

func decodeHistory(raw []byte, events *[]Event) error {
	if err := json.Unmarshal(raw, events); err != nil {
		return fmt.Errorf("decode public status history: %w", err)
	}
	if *events == nil {
		*events = []Event{}
	}
	return nil
}
