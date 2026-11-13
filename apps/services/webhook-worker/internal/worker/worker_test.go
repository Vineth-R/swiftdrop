package worker

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/messaging"
	"github.com/Vineth-R/swiftdrop/packages/webhookdelivery"
)

var testNow = time.Date(2026, 11, 10, 12, 0, 0, 0, time.UTC)

// fakes record what the handler does, so tests can assert on it.

type fakeMerchants struct {
	webhook      Webhook
	webhookErr   error
	delivered    bool
	deliveredErr error
	recordErr    error
	attempts     []Attempt
	merchantID   string
}

func (f *fakeMerchants) Webhook(_ context.Context, merchantID string) (Webhook, error) {
	f.merchantID = merchantID
	return f.webhook, f.webhookErr
}

func (f *fakeMerchants) Delivered(_ context.Context, _, _ string) (bool, error) {
	return f.delivered, f.deliveredErr
}

func (f *fakeMerchants) RecordAttempt(_ context.Context, _ string, a Attempt) error {
	if f.recordErr != nil {
		return f.recordErr
	}
	f.attempts = append(f.attempts, a)
	return nil
}

type fakeParcels struct {
	order *string
	err   error
}

func (f fakeParcels) ExternalOrderID(context.Context, string) (*string, error) { return f.order, f.err }

type fakeSender struct {
	result webhookdelivery.Result
	calls  int
	target webhookdelivery.Target
	event  webhookdelivery.Event
	onSend func()
}

func (f *fakeSender) Send(_ context.Context, t webhookdelivery.Target, e webhookdelivery.Event) webhookdelivery.Result {
	f.calls++
	f.target, f.event = t, e
	if f.onSend != nil {
		f.onSend()
	}
	return f.result
}

type fakeScheduler struct {
	err         error
	calls       int
	nextAttempt int
	delay       time.Duration
	message     messaging.Message
}

func (f *fakeScheduler) Schedule(_ context.Context, m messaging.Message, next int, d time.Duration) error {
	f.calls++
	f.nextAttempt, f.delay, f.message = next, d, m
	return f.err
}

type rig struct {
	merchants *fakeMerchants
	parcels   fakeParcels
	sender    *fakeSender
	scheduler *fakeScheduler
}

func newRig() *rig {
	return &rig{
		merchants: &fakeMerchants{webhook: Webhook{
			URL: "https://merchant.example.com/hook", SigningSecret: "whsec_test",
			EventTypes: []string{"parcel.status_changed"}, IsActive: true, MerchantStatus: "APPROVED",
		}},
		sender:    &fakeSender{result: webhookdelivery.Result{StatusCode: 200, Success: true, Duration: 120 * time.Millisecond}},
		scheduler: &fakeScheduler{},
	}
}

func (r *rig) handler() *Handler {
	return New(slog.New(slog.NewTextHandler(io.Discard, nil)), Deps{
		Merchants: r.merchants, Parcels: r.parcels, Sender: r.sender, Scheduler: r.scheduler,
		Now: func() time.Time { return testNow },
	})
}

func statusBody(t *testing.T, data string) []byte {
	t.Helper()
	body, err := json.Marshal(messaging.Envelope{
		ID: "11111111-2222-4333-8444-555555555555", Type: EventType, Version: 1,
		OccurredAt: testNow.Add(-time.Minute), Producer: "parcel", Data: json.RawMessage(data),
	})
	if err != nil {
		t.Fatalf("marshal envelope: %v", err)
	}
	return body
}

const fullData = `{"parcel_id":"p-1","tracking_number":"SDN123","previous_status":"PICKED_UP",
"status":"IN_TRANSIT","merchant_id":"m-1","note":"on the way"}`

func message(t *testing.T, data string, headers map[string]any) messaging.Message {
	return messaging.Message{Body: statusBody(t, data), Headers: headers}
}

func TestHandleDelivers(t *testing.T) {
	r := newRig()
	order := "ORDER-9"
	r.parcels = fakeParcels{order: &order}

	if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if r.merchants.merchantID != "m-1" {
		t.Fatalf("merchant = %q", r.merchants.merchantID)
	}
	if r.sender.target.URL != "https://merchant.example.com/hook" || r.sender.target.Secret != "whsec_test" {
		t.Fatalf("target = %+v", r.sender.target)
	}
	d := r.sender.event.Data
	if r.sender.event.ID != "11111111-2222-4333-8444-555555555555" ||
		d.TrackingNumber != "SDN123" || d.Status != "IN_TRANSIT" ||
		d.ExternalOrderID == nil || *d.ExternalOrderID != "ORDER-9" ||
		d.PreviousStatus == nil || *d.PreviousStatus != "PICKED_UP" ||
		d.Note == nil || *d.Note != "on the way" {
		t.Fatalf("event data = %+v", d)
	}
	if len(r.merchants.attempts) != 1 {
		t.Fatalf("attempts = %d, want 1", len(r.merchants.attempts))
	}
	a := r.merchants.attempts[0]
	if !a.Success || a.AttemptNumber != 1 || a.NextRetryAt != nil || a.DeadLettered ||
		a.HTTPStatus == nil || *a.HTTPStatus != 200 || a.DurationMs != 120 || a.ParcelID != "p-1" {
		t.Fatalf("attempt = %+v", a)
	}
	if r.scheduler.calls != 0 {
		t.Fatal("a successful delivery must not schedule a retry")
	}
}

func TestHandleFailureSchedulesRetry(t *testing.T) {
	tests := []struct {
		attempt   int
		headers   map[string]any
		wantDelay time.Duration
	}{
		{1, nil, time.Minute},
		{2, map[string]any{attemptHeader: int32(2)}, 5 * time.Minute},
		{3, map[string]any{attemptHeader: int64(3)}, 30 * time.Minute},
		{4, map[string]any{attemptHeader: 4}, 2 * time.Hour},
		{5, map[string]any{attemptHeader: "5"}, 6 * time.Hour},
	}
	for _, tt := range tests {
		r := newRig()
		r.sender.result = webhookdelivery.Result{StatusCode: 503, ErrorMessage: "server error", Duration: time.Second}

		if err := r.handler().Handle(context.Background(), message(t, fullData, tt.headers)); err != nil {
			t.Fatalf("attempt %d: Handle() error: %v", tt.attempt, err)
		}
		a := r.merchants.attempts[0]
		wantNext := testNow.Add(tt.wantDelay)
		if a.Success || a.DeadLettered || a.AttemptNumber != tt.attempt ||
			a.NextRetryAt == nil || !a.NextRetryAt.Equal(wantNext) || a.ErrorMessage != "server error" {
			t.Fatalf("attempt %d: report = %+v", tt.attempt, a)
		}
		if r.scheduler.calls != 1 || r.scheduler.nextAttempt != tt.attempt+1 || r.scheduler.delay != tt.wantDelay {
			t.Fatalf("attempt %d: scheduler = %+v", tt.attempt, r.scheduler)
		}
		if string(r.scheduler.message.Body) == "" {
			t.Fatal("the retry must carry the original body")
		}
	}
}

func TestHandleSixthFailureIsDeadLettered(t *testing.T) {
	r := newRig()
	r.sender.result = webhookdelivery.Result{ErrorMessage: "request timed out"} // no status: timeout

	if err := r.handler().Handle(context.Background(), message(t, fullData, map[string]any{attemptHeader: int32(6)})); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	a := r.merchants.attempts[0]
	if !a.DeadLettered || a.NextRetryAt != nil || a.HTTPStatus != nil || a.AttemptNumber != 6 {
		t.Fatalf("attempt = %+v", a)
	}
	if r.scheduler.calls != 0 {
		t.Fatal("a dead-lettered event must not be scheduled again")
	}
}

func TestHandleFailureWithoutMessageGetsDefault(t *testing.T) {
	r := newRig()
	r.sender.result = webhookdelivery.Result{StatusCode: 500}
	if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if r.merchants.attempts[0].ErrorMessage == "" {
		t.Fatal("a failed attempt needs an error message")
	}
}

func TestHandleSkipsWithoutWebhookOrMerchant(t *testing.T) {
	tests := map[string]struct {
		data   string
		mutate func(*rig)
	}{
		"no merchant":    {data: `{"parcel_id":"p","tracking_number":"T","status":"DELIVERED"}`},
		"blank merchant": {data: `{"parcel_id":"p","tracking_number":"T","status":"DELIVERED","merchant_id":" "}`},
		"not configured": {data: fullData, mutate: func(r *rig) { r.merchants.webhookErr = ErrNotConfigured }},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := newRig()
			if tt.mutate != nil {
				tt.mutate(r)
			}
			if err := r.handler().Handle(context.Background(), message(t, tt.data, nil)); err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			if r.sender.calls != 0 || len(r.merchants.attempts) != 0 || r.scheduler.calls != 0 {
				t.Fatal("a skip without a webhook must not send, record or schedule")
			}
		})
	}
}

func TestHandleSuppressesInactiveOrUnsubscribed(t *testing.T) {
	tests := map[string]struct {
		mutate func(*rig)
		reason string
	}{
		"inactive":         {mutate: func(r *rig) { r.merchants.webhook.IsActive = false }, reason: "webhook_inactive"},
		"not subscribed":   {mutate: func(r *rig) { r.merchants.webhook.EventTypes = []string{"other"} }, reason: "webhook_not_subscribed"},
		"no subscriptions": {mutate: func(r *rig) { r.merchants.webhook.EventTypes = nil }, reason: "webhook_not_subscribed"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := newRig()
			tt.mutate(r)
			if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			if r.sender.calls != 0 || r.scheduler.calls != 0 {
				t.Fatal("a suppressed event must not send or schedule")
			}
			if len(r.merchants.attempts) != 1 {
				t.Fatalf("attempts = %d, want one suppression record", len(r.merchants.attempts))
			}
			a := r.merchants.attempts[0]
			if a.Success || !a.DeadLettered || a.NextRetryAt != nil || a.ErrorMessage != tt.reason {
				t.Fatalf("suppression attempt = %+v", a)
			}
		})
	}
}

func TestHandleSuppressesNonApprovedMerchant(t *testing.T) {
	for _, status := range []string{"PENDING", "REJECTED", "SUSPENDED"} {
		t.Run(status, func(t *testing.T) {
			r := newRig()
			r.merchants.webhook.MerchantStatus = status
			if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
				t.Fatalf("Handle() error: %v", err)
			}
			if r.sender.calls != 0 || r.scheduler.calls != 0 {
				t.Fatal("a non-approved merchant must not receive a delivery")
			}
			if len(r.merchants.attempts) != 1 {
				t.Fatalf("attempts = %d, want one suppression record", len(r.merchants.attempts))
			}
			a := r.merchants.attempts[0]
			wantReason := "merchant_" + strings.ToLower(status)
			if a.Success || !a.DeadLettered || a.NextRetryAt != nil || a.ErrorMessage != wantReason {
				t.Fatalf("suppression attempt = %+v", a)
			}
		})
	}
	t.Run("unknown status", func(t *testing.T) {
		r := newRig()
		r.merchants.webhook.MerchantStatus = ""
		if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
			t.Fatalf("Handle() error: %v", err)
		}
		if r.sender.calls != 0 || len(r.merchants.attempts) != 1 || r.merchants.attempts[0].ErrorMessage != "merchant_status_unknown" {
			t.Fatalf("unexpected suppression: sends=%d attempts=%+v", r.sender.calls, r.merchants.attempts)
		}
	})
}

func TestHandleRechecksSuspensionBeforeRetry(t *testing.T) {
	r := newRig()
	r.sender.result = webhookdelivery.Result{StatusCode: 503, ErrorMessage: "server error", Duration: time.Second}
	if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("first attempt: %v", err)
	}
	if r.scheduler.calls != 1 {
		t.Fatal("the first failure should schedule a retry")
	}

	r.merchants.webhook.MerchantStatus = "SUSPENDED"
	if err := r.handler().Handle(context.Background(), message(t, fullData, map[string]any{attemptHeader: int32(2)})); err != nil {
		t.Fatalf("retry after suspension: %v", err)
	}
	if r.sender.calls != 1 {
		t.Fatalf("sends = %d, want only the first attempt", r.sender.calls)
	}
	if r.scheduler.calls != 1 {
		t.Fatal("a suspended retry must not be scheduled again")
	}
	last := r.merchants.attempts[len(r.merchants.attempts)-1]
	if last.AttemptNumber != 2 || last.ErrorMessage != "merchant_suspended" || !last.DeadLettered {
		t.Fatalf("retry suppression = %+v", last)
	}
}

func TestHandleAllowsInFlightSuspensionToFinish(t *testing.T) {
	r := newRig()
	r.sender.onSend = func() { r.merchants.webhook.MerchantStatus = "SUSPENDED" }
	if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("Handle() error: %v", err)
	}
	if r.sender.calls != 1 {
		t.Fatal("a delivery already in progress may finish")
	}
	if len(r.merchants.attempts) != 1 || !r.merchants.attempts[0].Success {
		t.Fatalf("successful in-flight attempt = %+v", r.merchants.attempts)
	}
}

func TestSuppressedDeliveryIsNotReplayedOnReinstatement(t *testing.T) {
	r := newRig()
	r.merchants.webhook.MerchantStatus = "SUSPENDED"
	if err := r.handler().Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("suppressed delivery: %v", err)
	}
	r.merchants.webhook.MerchantStatus = "APPROVED"
	if r.scheduler.calls != 0 || r.sender.calls != 0 {
		t.Fatal("reinstatement must not replay a suppressed delivery")
	}
}

func TestHandleInfrastructureErrorsAreRetryable(t *testing.T) {
	boom := errors.New("boom")
	tests := map[string]struct {
		mutate    func(*rig)
		wantSends int
	}{
		"merchant lookup": {mutate: func(r *rig) { r.merchants.webhookErr = boom }},
		"parcel lookup":   {mutate: func(r *rig) { r.parcels.err = boom }},
		"record attempt":  {mutate: func(r *rig) { r.merchants.recordErr = boom }, wantSends: 1},
		"schedule": {
			mutate: func(r *rig) {
				r.sender.result = webhookdelivery.Result{StatusCode: 500}
				r.scheduler.err = boom
			},
			wantSends: 1,
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			r := newRig()
			tt.mutate(r)
			err := r.handler().Handle(context.Background(), message(t, fullData, nil))
			if err == nil {
				t.Fatal("Handle() should fail")
			}
			if messaging.IsPermanent(err) {
				t.Fatalf("error must be retryable: %v", err)
			}
			if r.sender.calls != tt.wantSends {
				t.Fatalf("sends = %d, want %d", r.sender.calls, tt.wantSends)
			}
		})
	}
}

func TestHandleRejectsInvalidMessages(t *testing.T) {
	envelope := func(mutate func(*messaging.Envelope)) []byte {
		e := messaging.Envelope{
			ID: "event-1", Type: EventType, Version: 1, OccurredAt: testNow,
			Producer: "parcel", Data: json.RawMessage(fullData),
		}
		mutate(&e)
		body, err := json.Marshal(e)
		if err != nil {
			t.Fatalf("marshal envelope: %v", err)
		}
		return body
	}

	tests := map[string]messaging.Message{
		"malformed JSON":     {Body: []byte(`{`)},
		"wrong event type":   {Body: envelope(func(e *messaging.Envelope) { e.Type = "parcel.created.v1" })},
		"wrong version":      {Body: envelope(func(e *messaging.Envelope) { e.Version = 2 })},
		"invalid envelope":   {Body: envelope(func(e *messaging.Envelope) { e.ID = "" })},
		"bad data":           {Body: envelope(func(e *messaging.Envelope) { e.Data = json.RawMessage(`[]`) })},
		"missing fields":     {Body: envelope(func(e *messaging.Envelope) { e.Data = json.RawMessage(`{"merchant_id":"m"}`) })},
		"attempt zero":       {Body: envelope(func(*messaging.Envelope) {}), Headers: map[string]any{attemptHeader: int32(0)}},
		"attempt too high":   {Body: envelope(func(*messaging.Envelope) {}), Headers: map[string]any{attemptHeader: int32(7)}},
		"attempt not number": {Body: envelope(func(*messaging.Envelope) {}), Headers: map[string]any{attemptHeader: "x"}},
		"attempt wrong type": {Body: envelope(func(*messaging.Envelope) {}), Headers: map[string]any{attemptHeader: true}},
	}
	for name, msg := range tests {
		t.Run(name, func(t *testing.T) {
			r := newRig()
			err := r.handler().Handle(context.Background(), msg)
			if err == nil || !messaging.IsPermanent(err) {
				t.Fatalf("Handle() error = %v, want a permanent error", err)
			}
			if r.sender.calls != 0 {
				t.Fatal("an invalid message must not be sent")
			}
		})
	}
}

func TestClampDuration(t *testing.T) {
	if got := clampDuration(-time.Second); got != 0 {
		t.Fatalf("negative = %d", got)
	}
	if got := clampDuration(2 * time.Hour); got != maxDurationMs {
		t.Fatalf("large = %d", got)
	}
	if got := clampDuration(1500 * time.Millisecond); got != 1500 {
		t.Fatalf("normal = %d", got)
	}
}

func TestHandleSkipsAlreadyDeliveredEvent(t *testing.T) {
	rig := newRig()
	rig.merchants.delivered = true
	handler := rig.handler()
	if err := handler.Handle(context.Background(), message(t, fullData, nil)); err != nil {
		t.Fatalf("Handle: %v", err)
	}
	if rig.sender.calls != 0 {
		t.Fatalf("sent %d deliveries for an already-delivered event", rig.sender.calls)
	}
	if len(rig.merchants.attempts) != 0 {
		t.Fatalf("recorded %d attempts for an already-delivered event", len(rig.merchants.attempts))
	}
}
