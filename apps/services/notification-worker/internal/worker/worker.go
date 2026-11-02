package worker

import (
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
	Recipient string            `json:"recipient"`
	Channel   string            `json:"channel"`
	Template  string            `json:"template"`
	Variables map[string]string `json:"variables"`
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
	if err := validate(data); err != nil {
		return messaging.Permanent(err)
	}

	email := mailer.Message{
		To:      strings.TrimSpace(data.Recipient),
		Subject: strings.TrimSpace(data.Template),
		Text:    renderText(strings.TrimSpace(data.Template), data.Variables),
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
			"template", email.Subject,
			"error", safeErr.Error(),
		)
		return safeErr
	}

	h.logger.InfoContext(ctx, "notification delivered",
		"event_id", envelope.ID,
		"correlation_id", envelope.CorrelationID,
		"channel", data.Channel,
		"template", email.Subject,
	)
	return nil
}

func validate(data payload) error {
	recipient := strings.TrimSpace(data.Recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("recipient must be a plain email address")
	}

	switch data.Channel {
	case "email":
	case "sms":
		return errors.New("unsupported notification channel: sms")
	default:
		return fmt.Errorf("unsupported notification channel %q", data.Channel)
	}

	template := strings.TrimSpace(data.Template)
	if template == "" || len(template) > mailer.MaxSubjectLength || strings.ContainsAny(template, "\r\n") {
		return fmt.Errorf("template must be 1 to %d characters on one line", mailer.MaxSubjectLength)
	}

	if len(data.Variables) > maxVariables {
		return fmt.Errorf("variables must contain at most %d entries", maxVariables)
	}

	total := 0
	for key, value := range data.Variables {
		key = strings.TrimSpace(key)
		if key == "" || len(key) > maxVariableKey || strings.ContainsAny(key, "\r\n") {
			return errors.New("variable names must be 1 to 64 characters on one line")
		}
		if len(value) > maxVariableVal {
			return fmt.Errorf("variable %q must be at most %d bytes", key, maxVariableVal)
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
