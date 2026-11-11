# Email

Every email SwiftDrop sends is a branded HTML message with a plain text version. They share one layout (the web app's palette, logo and route motif, with a dark theme) and are written in one place: `packages/go-common/emails`. A service picks a scenario, gives it the data, and sends the result with `packages/go-common/mailer`.

## See them

```sh
make email-preview
```

Renders all of them to `tmp/email-preview/` (open `index.html`: a page with every email, each with its trigger and subject, plus the `.html` and `.txt` files).

```sh
make email-send-preview
```

Sends all of them to Mailpit, so you see them the way a mail app does, at <http://localhost:8025>. Run `make up` first.

## The emails

| Email                                                                      | Sent when                                                | Sent by             | State                                                                      |
| -------------------------------------------------------------------------- | -------------------------------------------------------- | ------------------- | -------------------------------------------------------------------------- |
| Email verification                                                         | A customer registers (2.1)                               | identity            | Live                                                                       |
| Password reset                                                             | Someone asks to reset a password (API 2.16)              | identity            | Live                                                                       |
| Password changed                                                           | A password is reset (2.17) or changed (2.9)              | identity            | Live                                                                       |
| Welcome                                                                    | A customer registers (2.1) or signs up with Google       | identity            | Live                                                                       |
| Account created by an administrator                                        | An admin creates a staff account (2.11)                  | identity            | Live                                                                       |
| Account locked                                                             | The failed login that locks an account (2.2)             | identity            | Live                                                                       |
| Account deactivated, reactivated                                           | An admin changes a user's status (2.14)                  | identity            | Live                                                                       |
| Merchant application received (pending)                                    | A merchant registers (8.1)                               | merchant            | Live                                                                       |
| Merchant approved                                                          | An admin approves (8.6)                                  | merchant            | Live                                                                       |
| Merchant rejected, with or without a reason                                | An admin rejects (8.7)                                   | merchant            | Live; the reason is optional                                               |
| Merchant suspended, with the reason                                        | An admin suspends (8.8)                                  | merchant            | Live                                                                       |
| API key created (security notice)                                          | A merchant creates a key (8.10)                          | merchant            | Template ready; M8 sends it                                                |
| Parcel created, picked up, in transit, out for delivery, delivered, failed | The parcel's status changes (`parcel.status_changed.v1`) | notification worker | Templates ready; the worker renders them once producers publish the events |

"Live" means a person gets the email today when they do the action. "Template ready" means the email exists, is tested and previews, and the owning service calls it.

## How an email is built

An email is a list of blocks: `Paragraph`, `Button`, `Details`, `Notice` and `Steps`. The layout turns the same blocks into the HTML and into the plain text, so the two never disagree.

```go
email := emails.MerchantApproved(emails.MerchantApprovedData{
    OwnerName:    "Ruwan Jayasuriya",
    BusinessName: "ShopZone Lanka",
    ApprovedAt:   emails.FormatTime(time.Now()),
    LoginURL:     site.URL("/login"),
})
message, err := email.Message("owner@shopzone.lk", site) // site: emails.Site{BaseURL, SupportEmail}
if err == nil {
    err = sender.Send(ctx, message) // sender: a mailer.Mailer
}
```

Rules the code enforces, so a mistake cannot reach a customer:

- Text from people (names, reasons, notes) is escaped in the HTML and shown as written in the text.
- Every link is absolute `http` or `https`. A `javascript:` or relative link is an error.
- At most one `Button` per email: one main action.
- The logo travels inside the message (`cid:`), so it shows even where remote images are blocked.
- Times are written in Sri Lanka time (`emails.FormatTime`).
- No email contains a password, a token other than the reset link, or an API key.

### Add an email

1. Add a data struct and a constructor in `scenarios_account.go`, `scenarios_merchant.go` or `scenarios_parcel.go`. Write plain, direct sentences in the product's voice: sentence case, what happened, then what to do, no exclamation marks.
2. Add an entry with sample data to `Catalog` in `catalog.go`. The tests and the preview only see what is in the catalog, and a test fails if the count changes without an update.
3. Run `go test ./packages/go-common/emails/` and `make email-preview`, and look at the result in light and dark.
4. Call it from the service that owns the event, in the background: an email that fails must never fail the action that caused it (see `apps/services/identity/internal/notify`).

## Real delivery

Locally the services send to Mailpit. For real email, set these in your untracked `.env` (see `.env.example`):

| Setting                       | Meaning                                                                                                                                                      |
| ----------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `SMTP_HOST`, `SMTP_PORT`      | The provider's SMTP server. Port 587 upgrades with STARTTLS; port 465 uses TLS from the first byte.                                                          |
| `SMTP_USER`, `SMTP_PASSWORD`  | The provider's login. They must be set together. They are only sent over TLS.                                                                                |
| `SMTP_FROM`, `SMTP_FROM_NAME` | The sender. The address must be one the provider lets you send from.                                                                                         |
| `SUPPORT_EMAIL`               | The address people may write to: shown in every footer and used as Reply-To.                                                                                 |
| `SMTP_REPLY_TO`               | Overrides the Reply-To header only.                                                                                                                          |
| `SMTP_SECURE`                 | `auto` (default: implicit TLS on 465), `true` or `false`.                                                                                                    |
| `FRONTEND_BASE_URL`           | The public address of the web app. Every link in an email is built from it, so it must be what people type in a browser, such as `https://app.swiftdrop.lk`. |

To prove a provider works without starting the stack, send all the emails to an inbox you own:

```sh
SMTP_HOST=smtp.gmail.com SMTP_PORT=587 SMTP_USER=you@gmail.com \
SMTP_PASSWORD=your-app-password SMTP_FROM=you@gmail.com \
  go run ./packages/go-common/emails/cmd/preview -env -send -to you@gmail.com
```

### Gmail

Enable 2-Step Verification on the account, create a 16-character App Password, and use it as `SMTP_PASSWORD` with `SMTP_HOST=smtp.gmail.com` and `SMTP_PORT=587`. Gmail rewrites the sender to the account's own address, so set `SMTP_FROM` to that address. It is fine for a demo; Google limits how many messages one account may send in a day.

### A transactional provider

Brevo, SendGrid, Mailgun, Amazon SES and Postmark all offer SMTP. Verify your sending domain with them (the SPF and DKIM records they give you) and send from an address on that domain. That is what keeps messages out of the spam folder; no code can do it for you.

### When nothing arrives

The identity service logs one line at startup. `Email is enabled` means it will send. `Email is disabled: <reason>` names the setting to fix (`SMTP_HOST is not set`, `FRONTEND_BASE_URL must be an absolute http or https URL`, and so on), and password reset is switched off with it. A failed delivery logs `account email was not sent` with the kind of email and the user ID, never the address or the message. Check the spam folder, then the provider's own log.

## Why they look the way they do

Email clients ignore most modern CSS, so the layout is tables with inline styles, 600 pixels wide, one column on a phone. The dark theme uses `prefers-color-scheme` where the client supports it (Apple Mail is one) and falls back to the light theme, or to the client's own dark mode, elsewhere. The brand fonts cannot be loaded in an email, so the text uses the system sans-serif. The colours are the web app's tokens, copied into `palette.go`; change them there if a token changes.
