package tools

import (
	"encoding/json"
	"math"
	"strconv"
	"strings"
	"unicode/utf8"
)

type argKind int

const (
	argString argKind = iota
	argNumber
)

// argSpec describes one accepted tool argument. It mirrors the provider-facing
// contract.Parameters so the model and validation cannot drift apart.
type argSpec struct {
	name     string
	kind     argKind
	required bool
	maxLen   int
	enum     []string
}

// validateArguments performs strict validation and returns a normalized copy.
// Validation messages are safe to return to the model: they name the field and
// the rule, never the submitted value.
func validateArguments(args map[string]any, specs []argSpec) (map[string]any, string) {
	known := make(map[string]argSpec, len(specs))
	for _, spec := range specs {
		known[spec.name] = spec
	}

	for key := range args {
		if _, ok := known[key]; !ok {
			return nil, "the request contains an unsupported argument"
		}
	}

	values := make(map[string]any, len(specs))
	for _, spec := range specs {
		raw, present := args[spec.name]
		if !present || raw == nil {
			if spec.required {
				return nil, spec.name + " is required"
			}
			continue
		}

		switch spec.kind {
		case argString:
			text, ok := raw.(string)
			if !ok {
				return nil, spec.name + " must be a string"
			}
			text = strings.TrimSpace(text)
			if text == "" {
				if spec.required {
					return nil, spec.name + " is required"
				}
				// An empty optional string is treated as absent so the
				// upstream filter is omitted.
				continue
			}
			if spec.maxLen > 0 && utf8.RuneCountInString(text) > spec.maxLen {
				return nil, spec.name + " is too long"
			}
			if len(spec.enum) > 0 && !containsString(spec.enum, text) {
				return nil, spec.name + " must be one of " + strings.Join(spec.enum, ", ")
			}
			values[spec.name] = text
		case argNumber:
			number, ok := canonicalNumber(raw)
			if !ok {
				return nil, spec.name + " must be a finite number"
			}
			values[spec.name] = number
		}
	}
	return values, ""
}

// canonicalNumber accepts every numeric shape a JSON provider can deliver and
// returns the shortest plain decimal representation.
func canonicalNumber(value any) (json.Number, bool) {
	var number float64
	switch typed := value.(type) {
	case json.Number:
		parsed, err := typed.Float64()
		if err != nil {
			return "", false
		}
		number = parsed
	case float64:
		number = typed
	case float32:
		number = float64(typed)
	case int:
		number = float64(typed)
	case int8:
		number = float64(typed)
	case int16:
		number = float64(typed)
	case int32:
		number = float64(typed)
	case int64:
		number = float64(typed)
	case uint:
		number = float64(typed)
	case uint8:
		number = float64(typed)
	case uint16:
		number = float64(typed)
	case uint32:
		number = float64(typed)
	case uint64:
		number = float64(typed)
	default:
		return "", false
	}
	if math.IsNaN(number) || math.IsInf(number, 0) {
		return "", false
	}
	return json.Number(strconv.FormatFloat(number, 'f', -1, 64)), true
}

// validWeight enforces the weight rules: greater than zero, at most 999999 and
// at most two decimal places.
func validWeight(number json.Number) bool {
	parsed, err := number.Float64()
	if err != nil || parsed <= 0 || parsed > 999999 {
		return false
	}
	text := number.String()
	if dot := strings.IndexByte(text, '.'); dot >= 0 && len(text)-dot-1 > 2 {
		return false
	}
	return true
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
