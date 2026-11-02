package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
)

const EventType = "notification.requested.v1"

const (
	maxRecipientLength   = 255
	maxErrorMessageBytes = 255
)

var uuidPattern = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// DeliveryStore records idempotency and delivery outcomes. The worker checks it
// before delivery and records an event only after a delivery attempt.
type DeliveryStore interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	RecordDelivered(ctx context.Context, entry idempotency.LogEntry) error
	RecordFailure(ctx context.Context, entry idempotency.LogEntry) error
}

const (
	maxVariables    = 64
	maxVariableKey  = 64
	maxVariableVal  = 1024
	maxVariableSize = 8 * 1024
)

type Handler struct {
	logger *slog.Logger
	sender mailer.Mailer
	store  DeliveryStore
}

func New(logger *slog.Logger, sender mailer.Mailer, store DeliveryStore) *Handler {
	return &Handler{logger: logger, sender: sender, store: store}
}

type payload struct {
	ParcelID  string          `json:"parcel_id"`
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

	if !uuidPattern.MatchString(envelope.ID) {
		return messaging.Permanent(errors.New("event_id must be a UUID"))
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

	if h.store == nil {
		return errors.New("processed-event store is not configured")
	}

	processed, err := h.store.IsProcessed(ctx, envelope.ID)
	if err != nil {
		h.logger.ErrorContext(ctx, "processed-event lookup failed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"error", err,
		)
		return fmt.Errorf("check processed event: %w", err)
	}
	if processed {
		h.logger.InfoContext(ctx, "notification already processed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"channel", data.Channel,
		)
		return nil
	}

	entry := idempotency.LogEntry{
		EventID:   envelope.ID,
		ParcelID:  strings.TrimSpace(data.ParcelID),
		Channel:   strings.ToUpper(data.Channel),
		Recipient: strings.TrimSpace(data.Recipient),
		Subject:   strings.TrimSpace(data.Template),
	}

	if entry.Channel == "SMS" {
		entry.Status = idempotency.StatusFailed
		entry.ErrorMessage = "unsupported notification channel"
		if err := h.recordFailure(ctx, entry, envelope.CorrelationID); err != nil {
			return err
		}
		return messaging.Permanent(errors.New("unsupported notification channel"))
	}

	email := mailer.Message{
		To:      entry.Recipient,
		Subject: entry.Subject,
		Text:    renderText(entry.Subject, variables),
	}

	if err := h.sender.Send(ctx, email); err != nil {
		if errors.Is(err, mailer.ErrInvalidMessage) {
			entry.Status = idempotency.StatusFailed
			entry.ErrorMessage = sanitizeErrorMessage(err.Error(), email.To)
			if logErr := h.recordFailure(ctx, entry, envelope.CorrelationID); logErr != nil {
				return logErr
			}
			return messaging.Permanent(fmt.Errorf("invalid notification message: %w", err))
		}

		safeErr := safeDeliveryError(err, email.To)
		entry.Status = idempotency.StatusPending
		entry.ErrorMessage = safeErr.Error()
		if logErr := h.recordFailure(ctx, entry, envelope.CorrelationID); logErr != nil {
			return logErr
		}
		return safeErr
	}

	if err := h.store.RecordDelivered(ctx, entry); err != nil {
		h.logger.ErrorContext(ctx, "record delivered notification failed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"error", err,
		)
		return fmt.Errorf("record delivered notification: %w", err)
	}

	h.logger.InfoContext(ctx, "notification delivered",
		"event_id", envelope.ID,
		"correlation_id", envelope.CorrelationID,
		"channel", entry.Channel,
	)
	return nil
}

func (h *Handler) recordFailure(ctx context.Context, entry idempotency.LogEntry, correlationID string) error {
	if err := h.store.RecordFailure(ctx, entry); err != nil {
		h.logger.ErrorContext(ctx, "record failed notification failed",
			"event_id", entry.EventID,
			"correlation_id", correlationID,
			"status", entry.Status,
			"error", err,
		)
		return fmt.Errorf("record failed notification: %w", err)
	}
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
	if !uuidPattern.MatchString(strings.TrimSpace(data.ParcelID)) {
		return errors.New("parcel_id must be a UUID")
	}

	recipient := strings.TrimSpace(data.Recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("recipient must be a plain email address")
	}
	if utf8.RuneCountInString(recipient) > maxRecipientLength {
		return fmt.Errorf("recipient must be at most %d characters", maxRecipientLength)
	}

	switch data.Channel {
	case "email", "sms":
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
	return errors.New(sanitizeErrorMessage(err.Error(), recipient))
}

// sanitizeErrorMessage redacts the recipient, removes control characters,
// collapses whitespace and truncates to the notification_logs.error_message
// limit without splitting a UTF-8 character.
func sanitizeErrorMessage(message, recipient string) string {
	if recipient != "" {
		message = strings.ReplaceAll(message, recipient, "[REDACTED]")
	}

	var builder strings.Builder
	for _, char := range message {
		switch {
		case char == '\r' || char == '\n' || char == '\t':
			builder.WriteByte(' ')
		case char < 0x20 || char == 0x7f:
			continue
		default:
			builder.WriteRune(char)
		}
	}

	return truncateUTF8(strings.Join(strings.Fields(builder.String()), " "), maxErrorMessageBytes)
}

func truncateUTF8(value string, maximum int) string {
	if len(value) <= maximum {
		return value
	}

	end := maximum
	for end > 0 && !utf8.RuneStart(value[end]) {
		end--
	}
	return value[:end]
}
