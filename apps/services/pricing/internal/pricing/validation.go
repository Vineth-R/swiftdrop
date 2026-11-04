package pricing

import (
	"encoding/json"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

func AllowedFields(kind string, creating bool) map[string]bool {
	fields := ""
	switch kind {
	case "zones":
		fields = "name districts isActive"
		if creating {
			fields += " code"
		}
	case "rules":
		fields = "baseFee isActive"
		if creating {
			fields += " originZoneId destinationZoneId"
		}
	case "bands":
		fields = "minWeightKg maxWeightKg charge isActive"
	case "surcharges":
		fields = "surcharge"
	}
	allowed := make(map[string]bool)
	for _, field := range strings.Fields(fields) {
		allowed[field] = true
	}
	return allowed
}

func (m Mutation) Validate(kind string, creating bool) error {
	raw, err := json.Marshal(m)
	if err != nil {
		return err
	}
	fields := make(map[string]json.RawMessage)
	if err := json.Unmarshal(raw, &fields); err != nil {
		return err
	}
	if len(fields) == 0 {
		return &Invalid{Field: "request", Issue: "must contain a change"}
	}
	allowed := AllowedFields(kind, creating)
	for field := range fields {
		if !allowed[field] {
			return &Invalid{Field: field, Issue: "field is not allowed"}
		}
	}
	required := []string{}
	if creating {
		switch kind {
		case "zones":
			required = []string{"code", "name"}
		case "rules":
			required = []string{"originZoneId", "destinationZoneId", "baseFee"}
		case "bands":
			required = []string{"minWeightKg", "maxWeightKg", "charge"}
		}
	}
	if kind == "surcharges" {
		required = []string{"surcharge"}
	}
	for _, field := range required {
		if _, ok := fields[field]; !ok {
			return &Invalid{Field: field, Issue: "is required"}
		}
	}
	for _, field := range []struct {
		name    string
		value   *string
		maximum int
	}{
		{name: "code", value: m.Code, maximum: 20}, {name: "name", value: m.Name, maximum: 100},
	} {
		if field.value != nil && (!validation.Required(*field.value) || validation.RuneLength(*field.value) > field.maximum) {
			return &Invalid{Field: field.name, Issue: "is empty or too long"}
		}
	}
	for _, field := range []struct {
		name  string
		value *string
	}{
		{name: "originZoneId", value: m.OriginZoneID}, {name: "destinationZoneId", value: m.DestinationZoneID},
	} {
		if field.value != nil && !validation.UUID(*field.value) {
			return &Invalid{Field: field.name, Issue: "must be a UUID"}
		}
	}
	for _, field := range []struct {
		name  string
		value *Decimal
	}{
		{name: "baseFee", value: m.BaseFee}, {name: "charge", value: m.Charge}, {name: "surcharge", value: m.Surcharge},
	} {
		if field.value != nil && *field.value < 0 {
			return &Invalid{Field: field.name, Issue: "must be nonnegative"}
		}
	}
	if m.MinWeightKg != nil && (*m.MinWeightKg < 0 || *m.MinWeightKg >= 999999) {
		return &Invalid{Field: "minWeightKg", Issue: "is outside supported precision"}
	}
	if m.MaxWeightKg != nil && (*m.MaxWeightKg <= 0 || *m.MaxWeightKg > 999999) {
		return &Invalid{Field: "maxWeightKg", Issue: "is outside supported precision"}
	}
	if m.MinWeightKg != nil && m.MaxWeightKg != nil && *m.MaxWeightKg <= *m.MinWeightKg {
		return &Invalid{Field: "maxWeightKg", Issue: "must exceed minWeightKg"}
	}
	return nil
}
