// Package notify sends merchant application and lifecycle emails with the
// shared branded templates. Delivery is background best-effort SMTP.
package notify

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/Vineth-R/swiftdrop/packages/go-common/emails"
	"github.com/Vineth-R/swiftdrop/packages/go-common/mailer"
)

const (
	sendTimeout = 30 * time.Second
	maxAttempts = 2
)

type Notifier struct {
	mailer   mailer.Mailer
	site     emails.Site
	logger   *slog.Logger
	dispatch func(func())
	pending  sync.WaitGroup
}

func New(sender mailer.Mailer, site emails.Site, logger *slog.Logger) *Notifier {
	if logger == nil {
		logger = slog.New(slog.DiscardHandler)
	}
	n := &Notifier{mailer: sender, site: site, logger: logger}
	n.dispatch = func(job func()) {
		n.pending.Add(1)
		go func() { defer n.pending.Done(); job() }()
	}
	return n
}

func (n *Notifier) Wait() { n.pending.Wait() }

func (n *Notifier) Received(ctx context.Context, to, ownerName, businessName, contactEmail string, submittedAt time.Time) {
	n.send(ctx, "merchant_application_received", to, emails.MerchantApplicationReceived(emails.MerchantApplicationData{
		OwnerName: ownerName, BusinessName: businessName, ContactEmail: contactEmail,
		SubmittedAt: emails.FormatTime(submittedAt), LoginURL: n.site.URL("/login"),
	}))
}

func (n *Notifier) Approved(ctx context.Context, to, ownerName, businessName string, approvedAt time.Time) {
	n.send(ctx, "merchant_approved", to, emails.MerchantApproved(emails.MerchantApprovedData{
		OwnerName: ownerName, BusinessName: businessName,
		ApprovedAt: emails.FormatTime(approvedAt), LoginURL: n.site.URL("/login"),
	}))
}

func (n *Notifier) Rejected(ctx context.Context, to, ownerName, businessName string, reason *string) {
	text := ""
	if reason != nil {
		text = *reason
	}
	n.send(ctx, "merchant_rejected", to, emails.MerchantRejected(emails.MerchantDecisionData{
		OwnerName: ownerName, BusinessName: businessName, Reason: text,
	}))
}

func (n *Notifier) Suspended(ctx context.Context, to, ownerName, businessName string, reason *string) {
	text := ""
	if reason != nil {
		text = *reason
	}
	n.send(ctx, "merchant_suspended", to, emails.MerchantSuspended(emails.MerchantDecisionData{
		OwnerName: ownerName, BusinessName: businessName, Reason: text,
	}))
}

func (n *Notifier) send(ctx context.Context, kind, to string, email emails.Email) {
	message, err := email.Message(to, n.site)
	if err != nil {
		n.logger.ErrorContext(ctx, "merchant email could not be built", "email_kind", kind, "error", err.Error())
		return
	}
	n.dispatch(func() {
		sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTimeout)
		defer cancel()
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			if err := n.mailer.Send(sendCtx, message); err == nil {
				return
			} else {
				n.logger.ErrorContext(sendCtx, "merchant email was not sent", "email_kind", kind, "attempt", attempt, "error", err.Error())
			}
			if attempt < maxAttempts {
				select {
				case <-sendCtx.Done():
					return
				case <-time.After(time.Duration(attempt) * time.Second):
				}
			}
		}
	})
}
