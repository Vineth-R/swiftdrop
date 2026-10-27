package messaging

import (
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestPermanentErrorClassification(t *testing.T) {
	base := errors.New("bad event")
	permanent := Permanent(base)

	if !IsPermanent(permanent) {
		t.Fatal("Permanent() error was not detected as permanent")
	}
	if IsPermanent(base) {
		t.Fatal("unclassified error was detected as permanent")
	}
	if IsPermanent(nil) {
		t.Fatal("nil error was detected as permanent")
	}

	wrapped := fmt.Errorf("handler: %w", permanent)
	if !IsPermanent(wrapped) {
		t.Fatal("wrapped permanent error was not detected")
	}
	if !errors.Is(wrapped, base) {
		t.Fatal("wrapped permanent error does not preserve the original error chain")
	}
	if !strings.Contains(wrapped.Error(), "bad event") {
		t.Fatalf("wrapped error lost context: %q", wrapped.Error())
	}

	if Permanent(nil) != nil {
		t.Fatal("Permanent(nil) returned a non-nil error")
	}
	if got := Permanent(permanent); got != permanent {
		t.Fatal("Permanent() wrapped an already permanent error")
	}
}
