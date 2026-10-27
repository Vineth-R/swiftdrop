package messaging

import (
	"bytes"
	"strings"
	"testing"
)

func TestValidateMessageAcceptsJSON(t *testing.T) {
	tests := map[string]string{
		"empty content type":            "",
		"whitespace content type":       "   ",
		"application json":              "application/json",
		"application json with charset": "application/json; charset=utf-8",
		"application json mixed case":   "Application/JSON",
	}

	for name, contentType := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMessage(contentType, []byte(`{"ok":true}`)); err != nil {
				t.Fatalf("ValidateMessage() rejected compatible message: %v", err)
			}
		})
	}
}

func TestValidateMessageRejectsOtherContentTypes(t *testing.T) {
	tests := map[string]string{
		"text plain":         "text/plain",
		"application xml":    "application/xml",
		"problem json":       "application/problem+json",
		"json patch":         "application/json-patch+json",
		"malformed":          "application/json; charset",
		"missing media type": "charset=utf-8",
	}

	for name, contentType := range tests {
		t.Run(name, func(t *testing.T) {
			if err := ValidateMessage(contentType, []byte(`{"ok":true}`)); err == nil {
				t.Fatal("ValidateMessage() should have failed")
			}
		})
	}
}

func TestValidateMessageEnforcesMaxSize(t *testing.T) {
	atLimit := bytes.Repeat([]byte("a"), MaxMessageBytes)
	if err := ValidateMessage("application/json", atLimit); err != nil {
		t.Fatalf("message at size limit rejected: %v", err)
	}

	overLimit := bytes.Repeat([]byte("a"), MaxMessageBytes+1)
	err := ValidateMessage("application/json", overLimit)
	if err == nil {
		t.Fatal("message over size limit accepted")
	}
	if !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("unexpected size error: %v", err)
	}
}

func TestValidateMessageRejectsOversizeBeforeContentType(t *testing.T) {
	overLimit := bytes.Repeat([]byte("a"), MaxMessageBytes+1)

	if err := ValidateMessage("text/plain", overLimit); err == nil {
		t.Fatal("oversize message accepted")
	}
}
