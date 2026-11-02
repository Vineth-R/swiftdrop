package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"sort"
	"strings"

	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const EventType = "notification.requested.v1"

const (
	maxVariables    = 64
	maxVariableKey  = 64
	maxVariableVal  = 1024
	maxVariableSize = 8 * 1024
)

type Handler struct {
	logger *slog.Logger
	sender mailer.Mailer
}

func New(logger *slog.Logger, sender mailer.Mailer) *Handler {
	return &Handler{logger: logger, sender: sender}
}

type payload struct {
	Recipient string          `json:"recipient"`
	Channel   string          `json:"channel"`
	Template  string          `json:"template"`
	Variables json.RawMessage `json:"variables"`
}

func (h *Handler) Handle(ctx context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode event envelope: %w", err))
	}

	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("validate event envelope: %w", err))
	}

	if envelope.Type != EventType {
		return messaging.Permanent(fmt.Errorf("unsupported event type %q", envelope.Type))
	}

	if envelope.Version != 1 {
		return messaging.Permanent(fmt.Errorf("unsupported event version %d", envelope.Version))
	}

	var data payload
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode notification data: %w", err))
	}
	variables, err := parseVariables(data.Variables)
	if err != nil {
		return messaging.Permanent(fmt.Errorf("decode notification data: %w", err))
	}
	if err := validate(data, variables); err != nil {
		return messaging.Permanent(err)
	}

	email := mailer.Message{
		To:      strings.TrimSpace(data.Recipient),
		Subject: strings.TrimSpace(data.Template),
		Text:    renderText(strings.TrimSpace(data.Template), variables),
	}

	if err := h.sender.Send(ctx, email); err != nil {
		if errors.Is(err, mailer.ErrInvalidMessage) {
			return messaging.Permanent(fmt.Errorf("invalid notification message: %w", err))
		}

		safeErr := safeDeliveryError(err, email.To)
		h.logger.ErrorContext(ctx, "notification delivery failed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"channel", data.Channel,
			"error", safeErr.Error(),
		)
		return safeErr
	}

	h.logger.InfoContext(ctx, "notification delivered",
		"event_id", envelope.ID,
		"correlation_id", envelope.CorrelationID,
		"channel", data.Channel,
	)
	return nil
}

func parseVariables(raw json.RawMessage) (map[string]string, error) {
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) == 0 {
		return nil, nil
	}
	if bytes.Equal(trimmed, []byte("null")) {
		return nil, errors.New("variables must be an object with string values")
	}

	var variables map[string]string
	if err := json.Unmarshal(trimmed, &variables); err != nil {
		return nil, errors.New("variables must be an object with string values")
	}
	return variables, nil
}

func validate(data payload, variables map[string]string) error {
	recipient := strings.TrimSpace(data.Recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("recipient must be a plain email address")
	}

	switch data.Channel {
	case "email":
	case "sms":
		return errors.New("unsupported notification channel")
	default:
		return errors.New("unsupported notification channel")
	}

	template := strings.TrimSpace(data.Template)
	if template == "" || len(template) > mailer.MaxSubjectLength || strings.ContainsAny(template, "\r\n") {
		return fmt.Errorf("template must be 1 to %d characters on one line", mailer.MaxSubjectLength)
	}

	if len(variables) > maxVariables {
		return fmt.Errorf("variables must contain at most %d entries", maxVariables)
	}

	total := 0
	for key, value := range variables {
		if key != strings.TrimSpace(key) {
			return errors.New("variable names must not have leading or trailing whitespace")
		}
		if key == "" || len(key) > maxVariableKey || strings.ContainsAny(key, "\r\n") {
			return errors.New("variable names must be 1 to 64 characters on one line")
		}
		if len(value) > maxVariableVal {
			return fmt.Errorf("variable values must be at most %d bytes", maxVariableVal)
		}
		total += len(key) + len(value)
	}
	if total > maxVariableSize {
		return fmt.Errorf("variables must total at most %d bytes", maxVariableSize)
	}

	return nil
}

func renderText(template string, variables map[string]string) string {
	if len(variables) == 0 {
		return template
	}

	keys := make([]string, 0, len(variables))
	for key := range variables {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var text strings.Builder
	text.WriteString(template)
	text.WriteString("\n\n")
	for _, key := range keys {
		text.WriteString(key)
		text.WriteString(": ")
		text.WriteString(variables[key])
		text.WriteString("\n")
	}
	return strings.TrimRight(text.String(), "\n")
}

// safeDeliveryError keeps server failures useful without echoing the recipient
// address into logs when an SMTP server quotes it back.
func safeDeliveryError(err error, recipient string) error {
	message := err.Error()
	if recipient != "" {
		message = strings.ReplaceAll(message, recipient, "[REDACTED]")
	}
	return errors.New(message)
}
