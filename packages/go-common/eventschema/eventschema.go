// Package eventschema validates published domain events against the checked-in
// JSON Schemas before a consumer handler interprets them.
//
// The schemas are copied from contracts/events so they can be embedded in the
// service binaries. A test compares the copies byte-for-byte with the contract
// source, so drift fails CI.
package eventschema

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"sync"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	// ErrInvalidEvent means the message body does not match the schema for its
	// event_type. Callers should classify it as permanent.
	ErrInvalidEvent = errors.New("event does not match its JSON schema")

	//go:embed schemas/*.schema.json
	schemaFiles embed.FS

	compileOnce sync.Once
	compiled    map[string]*jsonschema.Schema
	compileErr  error
)

type eventTypeEnvelope struct {
	EventType string `json:"event_type"`
}

// Validate checks a complete event envelope and payload against the schema
// selected by event_type. An event type with no checked-in schema is allowed
// through so the owning handler can apply its existing unsupported-type rule.
func Validate(body []byte) error {
	var envelope eventTypeEnvelope
	if err := json.Unmarshal(body, &envelope); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	schemas, err := schemas()
	if err != nil {
		return err
	}
	schema, ok := schemas[envelope.EventType]
	if !ok {
		return nil
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidEvent, err)
	}
	return nil
}

func schemas() (map[string]*jsonschema.Schema, error) {
	compileOnce.Do(func() {
		compiler := jsonschema.NewCompiler()
		compiled = make(map[string]*jsonschema.Schema)
		entries, err := fs.ReadDir(schemaFiles, "schemas")
		if err != nil {
			compileErr = err
			return
		}
		for _, entry := range entries {
			if entry.IsDir() {
				continue
			}
			path := "schemas/" + entry.Name()
			file, err := schemaFiles.Open(path)
			if err != nil {
				compileErr = err
				return
			}
			document, err := jsonschema.UnmarshalJSON(file)
			_ = file.Close()
			if err != nil {
				compileErr = err
				return
			}
			if err := compiler.AddResource(path, document); err != nil {
				compileErr = err
				return
			}
			schema, err := compiler.Compile(path)
			if err != nil {
				compileErr = err
				return
			}
			eventType, _ := document.(map[string]any)["properties"].(map[string]any)["event_type"].(map[string]any)["const"].(string)
			if eventType == "" {
				compileErr = fmt.Errorf("schema %s has no event_type const", path)
				return
			}
			compiled[eventType] = schema
		}
	})
	return compiled, compileErr
}
