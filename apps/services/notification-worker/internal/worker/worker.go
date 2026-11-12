package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/mail"
	"net/url"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/idempotency"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/identityclient"
	"github.com/Vineth-R/swiftdrop/apps/services/notification-worker/internal/merchantclient"
	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/logging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/go-common/validation"
)

const (
	// EventType is the legacy notification work item. It remains supported.
	EventType = "notification.requested.v1"
	// ParcelCreatedEventType is the D6.1 parcel-created domain event.
	ParcelCreatedEventType = "parcel.created.v1"
	// ParcelStatusChangedEventType is the D6.1 status-change domain event.
	ParcelStatusChangedEventType = "parcel.status_changed.v1"
)

const (
	maxRecipientLength   = 255
	maxErrorMessageBytes = 255
)

// DeliveryStore records idempotency and delivery outcomes. The worker checks it
// before delivery and records an event only after a delivery attempt.
type DeliveryStore interface {
	IsProcessed(ctx context.Context, eventID string) (bool, error)
	RecordDelivered(ctx context.Context, entry idempotency.LogEntry) error
	RecordFailure(ctx context.Context, entry idempotency.LogEntry) error
}

// ContactResolver resolves a customer's contact details through identity.
type ContactResolver interface {
	Contact(ctx context.Context, userID string) (identityclient.Contact, error)
}

// MerchantResolver resolves a merchant's contact details through the merchant
// service. The notification address is the merchant's approved contact email.
type MerchantResolver interface {
	Merchant(ctx context.Context, merchantID string) (merchantclient.Merchant, error)
}

const (
	maxVariables    = 64
	maxVariableKey  = 64
	maxVariableVal  = 1024
	maxVariableSize = 8 * 1024
)

type Handler struct {
	logger    *slog.Logger
	sender    mailer.Mailer
	store     DeliveryStore
	contacts  ContactResolver
	merchants MerchantResolver
	site      emails.Site
}

func New(logger *slog.Logger, sender mailer.Mailer, store DeliveryStore, contacts ContactResolver, merchants MerchantResolver, site emails.Site) *Handler {
	return &Handler{logger: logger, sender: sender, store: store, contacts: contacts, merchants: merchants, site: site}
}

type payload struct {
	ParcelID  string          `json:"parcel_id"`
	Recipient string          `json:"recipient"`
	Channel   string          `json:"channel"`
	Template  string          `json:"template"`
	Variables json.RawMessage `json:"variables"`
}

type parcelCreatedData struct {
	ParcelID       string  `json:"parcel_id"`
	TrackingNumber string  `json:"tracking_number"`
	Status         string  `json:"status"`
	CustomerUserID *string `json:"customer_user_id"`
	MerchantID     *string `json:"merchant_id"`
}

type parcelStatusChangedData struct {
	ParcelID       string  `json:"parcel_id"`
	TrackingNumber string  `json:"tracking_number"`
	PreviousStatus string  `json:"previous_status"`
	Status         string  `json:"status"`
	MerchantID     *string `json:"merchant_id"`
	CustomerUserID *string `json:"customer_user_id"`
	ActorUserID    *string `json:"actor_user_id"`
	Note           *string `json:"note"`
	FailureReason  *string `json:"failure_reason"`
}

func (h *Handler) Handle(ctx context.Context, message messaging.Message) error {
	var envelope messaging.Envelope
	if err := json.Unmarshal(message.Body, &envelope); err != nil {
		return messaging.Permanent(fmt.Errorf("decode event envelope: %w", err))
	}

	if err := envelope.Validate(); err != nil {
		return messaging.Permanent(fmt.Errorf("validate event envelope: %w", err))
	}

	if !validation.UUID(envelope.ID) {
		return messaging.Permanent(errors.New("event_id must be a UUID"))
	}

	if envelope.Type == ParcelCreatedEventType || envelope.Type == ParcelStatusChangedEventType {
		if envelope.Producer != "parcel" {
			return messaging.Permanent(fmt.Errorf("%s producer must be %q", envelope.Type, "parcel"))
		}
	}

	switch envelope.Type {
	case EventType:
		return h.handleLegacy(ctx, envelope)
	case ParcelCreatedEventType:
		return h.handleParcelCreated(ctx, envelope)
	case ParcelStatusChangedEventType:
		return h.handleParcelStatusChanged(ctx, envelope)
	default:
		return messaging.Permanent(fmt.Errorf("unsupported event type %q", envelope.Type))
	}
}

func (h *Handler) handleLegacy(ctx context.Context, envelope messaging.Envelope) error {
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
			"operation", "lookup_processed_event",
		)
		return errors.New("processed-event lookup failed")
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
	return h.deliver(ctx, envelope, entry, email)
}

func (h *Handler) handleParcelCreated(ctx context.Context, envelope messaging.Envelope) error {
	var data parcelCreatedData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode parcel.created.v1 data: %w", err))
	}
	owner, err := validateParcelCreated(data)
	if err != nil {
		return messaging.Permanent(err)
	}
	if owner.merchantID != "" {
		return h.notifyMerchant(
			ctx,
			envelope,
			strings.TrimSpace(data.ParcelID),
			strings.TrimSpace(data.TrackingNumber),
			owner.merchantID,
			"CREATED",
			"",
			"",
		)
	}
	return h.notifyCustomer(
		ctx,
		envelope,
		strings.TrimSpace(data.ParcelID),
		strings.TrimSpace(data.TrackingNumber),
		owner.customerUserID,
		"CREATED",
		"",
		"",
	)
}

func (h *Handler) handleParcelStatusChanged(ctx context.Context, envelope messaging.Envelope) error {
	var data parcelStatusChangedData
	if err := json.Unmarshal(envelope.Data, &data); err != nil {
		return messaging.Permanent(fmt.Errorf("decode parcel.status_changed.v1 data: %w", err))
	}
	if err := validateParcelStatusChanged(data); err != nil {
		return messaging.Permanent(err)
	}

	hasCustomer := data.CustomerUserID != nil && strings.TrimSpace(*data.CustomerUserID) != ""
	hasMerchant := data.MerchantID != nil && strings.TrimSpace(*data.MerchantID) != ""
	switch {
	case hasCustomer && hasMerchant:
		return messaging.Permanent(errors.New("parcel.status_changed.v1 has an ambiguous notification owner"))
	case hasMerchant:
		failureReason := ""
		if data.FailureReason != nil {
			failureReason = strings.TrimSpace(*data.FailureReason)
		}
		return h.notifyMerchant(
			ctx,
			envelope,
			strings.TrimSpace(data.ParcelID),
			strings.TrimSpace(data.TrackingNumber),
			strings.TrimSpace(*data.MerchantID),
			data.Status,
			data.PreviousStatus,
			failureReason,
		)
	case !hasCustomer:
		return messaging.Permanent(errors.New("parcel.status_changed.v1 has no notification owner"))
	}

	failureReason := ""
	if data.FailureReason != nil {
		failureReason = strings.TrimSpace(*data.FailureReason)
	}
	return h.notifyCustomer(
		ctx,
		envelope,
		strings.TrimSpace(data.ParcelID),
		strings.TrimSpace(data.TrackingNumber),
		strings.TrimSpace(*data.CustomerUserID),
		data.Status,
		data.PreviousStatus,
		failureReason,
	)
}

// notifyMerchant delivers a merchant-owned parcel update to the merchant's
// approved contact email. Unapproved merchants and missing contacts are
// permanent failures: the event is preserved in the dead-letter queue rather
// than silently dropped.
func (h *Handler) notifyMerchant(
	ctx context.Context,
	envelope messaging.Envelope,
	parcelID string,
	trackingNumber string,
	merchantID string,
	status string,
	previousStatus string,
	failureReason string,
) error {
	if h.store == nil {
		return errors.New("processed-event store is not configured")
	}

	processed, err := h.store.IsProcessed(ctx, envelope.ID)
	if err != nil {
		h.logger.ErrorContext(ctx, "processed-event lookup failed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"operation", "lookup_processed_event",
		)
		return errors.New("processed-event lookup failed")
	}
	if processed {
		h.logger.InfoContext(ctx, "notification already processed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"channel", "email",
		)
		return nil
	}

	if h.merchants == nil {
		return errors.New("merchant contact resolver is not configured")
	}
	merchant, err := h.merchants.Merchant(ctx, merchantID)
	if err != nil {
		if errors.Is(err, merchantclient.ErrNotFound) ||
			errors.Is(err, merchantclient.ErrUnauthorized) ||
			errors.Is(err, merchantclient.ErrInvalidResponse) {
			h.logger.WarnContext(ctx, "notification merchant lookup failed permanently",
				"event_id", envelope.ID,
				"parcel_id", parcelID,
				"merchant_id", merchantID,
				"correlation_id", envelope.CorrelationID,
				"operation", "merchant_contact",
				"error", err, "error_type", fmt.Sprintf("%T", err),
			)
			return messaging.Permanent(err)
		}
		h.logger.WarnContext(ctx, "notification merchant lookup failed; retrying",
			"event_id", envelope.ID,
			"parcel_id", parcelID,
			"merchant_id", merchantID,
			"correlation_id", envelope.CorrelationID,
			"operation", "merchant_contact",
			"error", err, "error_type", fmt.Sprintf("%T", err),
		)
		return err
	}

	if merchant.Status != "APPROVED" {
		h.logger.WarnContext(ctx, "merchant-owned parcel notification skipped",
			"event_id", envelope.ID,
			"parcel_id", parcelID,
			"merchant_id", merchantID,
			"merchant_status", merchant.Status,
			"correlation_id", envelope.CorrelationID,
		)
		return messaging.Permanent(errors.New("merchant is not approved for parcel notifications"))
	}

	contactEmail := strings.TrimSpace(merchant.ContactEmail)
	if contactEmail == "" {
		h.logger.WarnContext(ctx, "merchant-owned parcel notification skipped",
			"event_id", envelope.ID,
			"parcel_id", parcelID,
			"merchant_id", merchantID,
			"correlation_id", envelope.CorrelationID,
			"reason", "missing_contact_email",
		)
		return messaging.Permanent(errors.New("merchant contact email is empty"))
	}

	contact := identityclient.Contact{
		FullName: strings.TrimSpace(merchant.BusinessName),
		Email:    contactEmail,
	}
	message, err := h.parcelEmail(contact, trackingNumber, status, previousStatus, failureReason)
	if err != nil {
		return messaging.Permanent(fmt.Errorf("build merchant notification: %w", err))
	}
	entry := idempotency.LogEntry{
		EventID:   envelope.ID,
		ParcelID:  parcelID,
		Channel:   "EMAIL",
		Recipient: contactEmail,
		Subject:   message.Subject,
	}
	return h.deliver(ctx, envelope, entry, message)
}

func (h *Handler) notifyCustomer(
	ctx context.Context,
	envelope messaging.Envelope,
	parcelID string,
	trackingNumber string,
	customerUserID string,
	status string,
	previousStatus string,
	failureReason string,
) error {
	if h.store == nil {
		return errors.New("processed-event store is not configured")
	}

	processed, err := h.store.IsProcessed(ctx, envelope.ID)
	if err != nil {
		h.logger.ErrorContext(ctx, "processed-event lookup failed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"operation", "lookup_processed_event",
		)
		return errors.New("processed-event lookup failed")
	}
	if processed {
		h.logger.InfoContext(ctx, "notification already processed",
			"event_id", envelope.ID,
			"correlation_id", envelope.CorrelationID,
			"channel", "email",
		)
		return nil
	}

	if h.contacts == nil {
		return errors.New("identity contact resolver is not configured")
	}
	contact, err := h.contacts.Contact(ctx, customerUserID)
	if err != nil {
		if errors.Is(err, identityclient.ErrNotFound) ||
			errors.Is(err, identityclient.ErrUnauthorized) ||
			errors.Is(err, identityclient.ErrInvalidResponse) {
			h.logger.WarnContext(ctx, "notification contact lookup failed permanently",
				"event_id", envelope.ID,
				"parcel_id", parcelID,
				"correlation_id", envelope.CorrelationID,
				"operation", "identity_contact",
				"error", err, "error_type", fmt.Sprintf("%T", err),
			)
			return messaging.Permanent(err)
		}
		h.logger.WarnContext(ctx, "notification contact lookup failed; retrying",
			"event_id", envelope.ID,
			"parcel_id", parcelID,
			"correlation_id", envelope.CorrelationID,
			"operation", "identity_contact",
			"error", err, "error_type", fmt.Sprintf("%T", err),
		)
		return err
	}

	message, err := h.parcelEmail(contact, trackingNumber, status, previousStatus, failureReason)
	if err != nil {
		return messaging.Permanent(fmt.Errorf("build parcel notification: %w", err))
	}
	entry := idempotency.LogEntry{
		EventID:   envelope.ID,
		ParcelID:  parcelID,
		Channel:   "EMAIL",
		Recipient: contact.Email,
		Subject:   message.Subject,
	}
	return h.deliver(ctx, envelope, entry, message)
}

func (h *Handler) parcelEmail(
	contact identityclient.Contact,
	trackingNumber string,
	status string,
	previousStatus string,
	failureReason string,
) (mailer.Message, error) {
	subject, heading, lead := parcelWording(status, trackingNumber)

	rows := []emails.Row{
		{Label: "Tracking number", Value: trackingNumber, IsMono: true},
		{Label: "Current status", Value: statusLabel(status)},
	}
	if previousStatus != "" {
		rows = append(rows, emails.Row{Label: "Previous status", Value: statusLabel(previousStatus)})
	}
	if status == "FAILED" && failureReason != "" {
		rows = append(rows, emails.Row{Label: "Reason", Value: failureReason})
	}

	greeting := "Hello,"
	if name := strings.TrimSpace(contact.FullName); name != "" {
		greeting = "Hello " + name + ","
	}

	email := emails.Email{
		Subject:   subject,
		Preheader: heading + ": " + trackingNumber + ".",
		Eyebrow:   "Parcel update",
		Heading:   heading,
		Blocks: []emails.Block{
			emails.Paragraph(greeting),
			emails.Paragraph(lead),
			emails.Details(rows...),
			emails.Button("Track your parcel", h.site.URL("/track/"+url.PathEscape(trackingNumber))),
		},
		Reason: "You are receiving this because you sent parcel " + trackingNumber + " with SwiftDrop.",
	}
	return email.Message(contact.Email, h.site)
}

func (h *Handler) deliver(ctx context.Context, envelope messaging.Envelope, entry idempotency.LogEntry, message mailer.Message) error {
	if err := h.sender.Send(ctx, message); err != nil {
		if errors.Is(err, mailer.ErrInvalidMessage) {
			entry.Status = idempotency.StatusFailed
			entry.ErrorMessage = sanitizeErrorMessage(err.Error(), message.To)
			if logErr := h.recordFailure(ctx, entry, envelope.CorrelationID); logErr != nil {
				return logErr
			}
			return messaging.Permanent(fmt.Errorf("invalid notification message: %w", err))
		}

		safeErr := safeDeliveryError(err, message.To)
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
			"operation", "record_delivery",
		)
		return errors.New("record delivered notification failed")
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
			"operation", "record_failure",
		)
		return errors.New("record failed notification failed")
	}
	return nil
}

type parcelOwner struct {
	customerUserID string
	merchantID     string
}

func validateParcelCreated(data parcelCreatedData) (parcelOwner, error) {
	if !validation.UUID(strings.TrimSpace(data.ParcelID)) {
		return parcelOwner{}, errors.New("parcel_id must be a UUID")
	}
	tracking := strings.TrimSpace(data.TrackingNumber)
	if length := validation.RuneLength(tracking); length < 11 || length > 20 {
		return parcelOwner{}, errors.New("tracking_number must be 11 to 20 characters")
	}
	if data.Status != "CREATED" {
		return parcelOwner{}, errors.New("parcel.created.v1 status must be CREATED")
	}

	hasCustomer := data.CustomerUserID != nil && strings.TrimSpace(*data.CustomerUserID) != ""
	hasMerchant := data.MerchantID != nil && strings.TrimSpace(*data.MerchantID) != ""
	if hasCustomer == hasMerchant {
		return parcelOwner{}, errors.New("parcel.created.v1 must have exactly one owner")
	}
	if hasCustomer {
		customerUserID := strings.TrimSpace(*data.CustomerUserID)
		if !validation.UUID(customerUserID) {
			return parcelOwner{}, errors.New("customer_user_id must be a UUID")
		}
		return parcelOwner{customerUserID: customerUserID}, nil
	}
	merchantID := strings.TrimSpace(*data.MerchantID)
	if !validation.UUID(merchantID) {
		return parcelOwner{}, errors.New("merchant_id must be a UUID")
	}
	return parcelOwner{merchantID: merchantID}, nil
}

func validateParcelStatusChanged(data parcelStatusChangedData) error {
	if !validation.UUID(strings.TrimSpace(data.ParcelID)) {
		return errors.New("parcel_id must be a UUID")
	}
	tracking := strings.TrimSpace(data.TrackingNumber)
	if length := validation.RuneLength(tracking); length < 11 || length > 20 {
		return errors.New("tracking_number must be 11 to 20 characters")
	}
	if !validStatus(data.PreviousStatus) {
		return errors.New("previous_status is not a parcel status")
	}
	if !validStatus(data.Status) {
		return errors.New("status is not a parcel status")
	}
	if data.Status == "FAILED" && data.FailureReason == nil {
		return errors.New("failure_reason is required when status is FAILED")
	}
	if data.FailureReason != nil {
		reason := strings.TrimSpace(*data.FailureReason)
		if length := validation.RuneLength(reason); length < 5 || length > 255 {
			return errors.New("failure_reason must be 5 to 255 characters")
		}
	}
	if data.CustomerUserID != nil && strings.TrimSpace(*data.CustomerUserID) != "" &&
		!validation.UUID(strings.TrimSpace(*data.CustomerUserID)) {
		return errors.New("customer_user_id must be a UUID")
	}
	if data.MerchantID != nil && strings.TrimSpace(*data.MerchantID) != "" &&
		!validation.UUID(strings.TrimSpace(*data.MerchantID)) {
		return errors.New("merchant_id must be a UUID")
	}
	if data.ActorUserID != nil && !validation.UUID(strings.TrimSpace(*data.ActorUserID)) {
		return errors.New("actor_user_id must be a UUID")
	}
	if data.Note != nil && validation.RuneLength(*data.Note) > 500 {
		return errors.New("note must be at most 500 characters")
	}
	return nil
}

func validStatus(status string) bool {
	switch status {
	case "CREATED", "PICKED_UP", "IN_TRANSIT", "OUT_FOR_DELIVERY", "DELIVERED", "FAILED":
		return true
	default:
		return false
	}
}

func parcelWording(status, trackingNumber string) (subject, heading, lead string) {
	switch status {
	case "CREATED":
		return fmt.Sprintf("Your parcel %s was created", trackingNumber),
			"Your parcel is booked",
			fmt.Sprintf("Your parcel %s has been created and is ready for collection.", trackingNumber)
	case "PICKED_UP":
		return fmt.Sprintf("Your parcel %s was picked up", trackingNumber),
			"Your parcel was picked up",
			fmt.Sprintf("Your parcel %s has been picked up by our courier.", trackingNumber)
	case "IN_TRANSIT":
		return fmt.Sprintf("Your parcel %s is in transit", trackingNumber),
			"Your parcel is on its way",
			fmt.Sprintf("Your parcel %s is moving through our network.", trackingNumber)
	case "OUT_FOR_DELIVERY":
		return fmt.Sprintf("Your parcel %s is out for delivery", trackingNumber),
			"Your parcel is out for delivery",
			fmt.Sprintf("A courier is on the way with your parcel %s.", trackingNumber)
	case "DELIVERED":
		return fmt.Sprintf("Your parcel %s was delivered", trackingNumber),
			"Your parcel was delivered",
			fmt.Sprintf("Your parcel %s has been delivered.", trackingNumber)
	case "FAILED":
		return fmt.Sprintf("We could not deliver parcel %s", trackingNumber),
			"We could not deliver your parcel",
			fmt.Sprintf("We could not deliver your parcel %s.", trackingNumber)
	default:
		return fmt.Sprintf("Your parcel %s was updated", trackingNumber),
			"Your parcel was updated",
			fmt.Sprintf("Your parcel %s has a new status.", trackingNumber)
	}
}

func statusLabel(status string) string {
	switch status {
	case "CREATED":
		return "Created"
	case "PICKED_UP":
		return "Picked up"
	case "IN_TRANSIT":
		return "In transit"
	case "OUT_FOR_DELIVERY":
		return "Out for delivery"
	case "DELIVERED":
		return "Delivered"
	case "FAILED":
		return "Failed"
	default:
		return status
	}
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
	if !validation.UUID(strings.TrimSpace(data.ParcelID)) {
		return errors.New("parcel_id must be a UUID")
	}

	recipient := strings.TrimSpace(data.Recipient)
	address, err := mail.ParseAddress(recipient)
	if err != nil || address.Address != recipient || strings.ContainsAny(recipient, "\r\n") {
		return errors.New("recipient must be a plain email address")
	}
	if validation.RuneLength(recipient) > maxRecipientLength {
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
	// Redact credential-shaped words and any embedded email addresses before
	// trimming control characters. The exact recipient is redacted separately
	// because a formatted mask can differ from the address in an SMTP error.
	message = logging.RedactText(message)
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
