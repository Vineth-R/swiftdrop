package logging

import (
	"fmt"
	"io"
	"log/slog"
	"os"
	"strings"
)

func New(service, env, level string) (*slog.Logger, error) {
	parsed, err := ParseLevel(level)
	if err != nil {
		return nil, err
	}

	options := &slog.HandlerOptions{Level: parsed}

	var handler slog.Handler
	if strings.EqualFold(strings.TrimSpace(env), "development") {
		handler = slog.NewTextHandler(os.Stdout, options)
	} else {
		handler = slog.NewJSONHandler(os.Stdout, options)
	}

	return slog.New(NewRedactingHandler(handler)).With("service", service), nil
}

// NewBootstrap returns a redacting JSON logger for failures that happen before
// configuration and normal logging are available. It always uses JSON so
// startup failures stay machine-readable in every environment.
func NewBootstrap(service string) *slog.Logger {
	return newBootstrap(os.Stderr, service)
}

func newBootstrap(writer io.Writer, service string) *slog.Logger {
	handler := NewRedactingHandler(slog.NewJSONHandler(writer, nil))
	return slog.New(handler).With("service", service)
}

func ParseLevel(value string) (slog.Level, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "debug":
		return slog.LevelDebug, nil
	case "", "info":
		return slog.LevelInfo, nil
	case "warn", "warning":
		return slog.LevelWarn, nil
	case "error":
		return slog.LevelError, nil
	default:
		return 0, fmt.Errorf("unsupported log level %q", value)
	}
}
