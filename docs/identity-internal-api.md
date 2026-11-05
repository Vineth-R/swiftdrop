# Identity internal APIs and shared mailer: handoff for M1 and M6

From M3. Source: API Documentation v1.1 section 13 and the Team Task Sheet (M3.3, M3.4).
Machine-readable contract: [`contracts/openapi/identity-internal.yaml`](../contracts/openapi/identity-internal.yaml).

| Who          | What they use                                                              | Section                                         |
| ------------ | -------------------------------------------------------------------------- | ----------------------------------------------- |
| M6 (Parcel)  | `GET /internal/couriers/{courierId}/eligibility` before assigning a parcel | [1](#1-the-two-endpoints)                       |
| M1 (Notif.)  | `GET /internal/users/{userId}/contact` to find the recipient               | [1](#1-the-two-endpoints)                       |
| M1 (Notif.)  | `packages/go-common/mailer` to send the email                              | [5](#5-m1-sending-email-with-the-shared-mailer) |
| M1 (Gateway) | Nothing to change for `/internal`; one gap for `/api/v1/admin/users`       | [6](#6-m1-gateway-notes)                        |

These endpoints are for service-to-service calls only. Nginx never routes `/internal`
(`infra/nginx/default.conf` returns a JSON `404 NOT_FOUND`), so only containers on the Compose
network can reach them.

## 1. The two endpoints

Base URL inside Compose: `http://identity:8081`. Both are `GET`, need both headers below, and
return the standard success envelope (`data` and `meta`, API section 1.5).

| Header             | Value                                                                                          |
| ------------------ | ---------------------------------------------------------------------------------------------- |
| `X-Internal-Token` | The shared secret from the `INTERNAL_TOKEN` environment variable.                              |
| `X-Request-Id`     | The current request ID, or a new one. Letters, digits and `- . _ :` only, 1 to 128 characters. |

### `GET /internal/couriers/{courierId}/eligibility` (M6)

```bash
curl -s "http://identity:8081/internal/couriers/3f1c7a52-0000-4000-8000-000000000001/eligibility" \
  -H "X-Internal-Token: $INTERNAL_TOKEN" -H "X-Request-Id: req_01HF-example"
```

```json
{
  "data": { "isActive": true, "isAvailable": true, "role": "COURIER" },
  "meta": {
    "requestId": "req_01HF-example",
    "timestamp": "2026-11-04T09:30:00Z"
  }
}
```

- Any existing user gets an answer with their real `role`. **A courier may be assigned only when
  `role == "COURIER"` and `isActive` and `isAvailable` are all true.** Identity reports facts;
  it does not decide for you (so you can pick your own error, for example
  `422 COURIER_NOT_ELIGIBLE`, API 5.2).
- A user with no courier profile row has `isAvailable: false`.
- Only these three fields are returned (API section 13).

### `GET /internal/users/{userId}/contact` (M1)

```bash
curl -s "http://identity:8081/internal/users/3f1c7a52-0000-4000-8000-000000000002/contact" \
  -H "X-Internal-Token: $INTERNAL_TOKEN" -H "X-Request-Id: req_01HF-example"
```

```json
{
  "data": {
    "fullName": "Nimal Perera",
    "email": "nimal@example.com",
    "phone": "+94771234567"
  },
  "meta": {
    "requestId": "req_01HF-example",
    "timestamp": "2026-11-04T09:30:00Z"
  }
}
```

- `phone` is an **empty string** when the user has none (for example a Google sign-up). Skip SMS
  in that case.
- Deactivated users are still returned, so a final notice can still be sent.
- This is personal data. Do not write it to logs.

### Errors

Standard envelope: `{"error": {"code": "...", "message": "..."}, "meta": {"requestId": "..."}}`.

| Status | Code                 | Meaning for the caller                                                                     |
| ------ | -------------------- | ------------------------------------------------------------------------------------------ |
| 401    | `UNAUTHENTICATED`    | `X-Internal-Token` missing or wrong, **or Identity itself has no valid `INTERNAL_TOKEN`**. |
| 400    | `BAD_REQUEST`        | `X-Request-Id` missing or malformed. The token is checked first.                           |
| 404    | `NOT_FOUND`          | No such user, or the ID is not a UUID. Treat as "not found".                               |
| 405    | `METHOD_NOT_ALLOWED` | Anything but `GET`.                                                                        |
| 500    | `INTERNAL_ERROR`     | Unexpected failure, such as the database being down. Safe to retry.                        |

**Treat `401` as a configuration fault, never as "courier not found".** If your 401s persist, the
token in your container differs from Identity's (see section 3).

## 2. Request timeouts and retries

Use a short client timeout (2 to 3 seconds). Retry only `500` and network errors, with a small
backoff. Never retry `401`, `400` or `404`: they will not change. For M1's notification worker, a
`404` for the user is a permanent failure (use `messaging.Permanent`); `500` and timeouts are
transient and should use the broker retry.

## 3. `INTERNAL_TOKEN` configuration (exact steps)

The variable name is **`INTERNAL_TOKEN`**, and every participant must use the same value.

**3.1 Create one secret, once, without showing it.** The project already has a placeholder in
`.env.example`; put a real value only in your untracked `.env`. To generate one:

```bash
# Linux, macOS or Git Bash: appends a new line to .env without printing the value
printf 'INTERNAL_TOKEN=%s\n' "$(openssl rand -hex 32)" >> .env
```

```powershell
# Windows PowerShell: appends to .env without printing the value
$b = New-Object byte[] 32; [Security.Cryptography.RandomNumberGenerator]::Create().GetBytes($b)
"INTERNAL_TOKEN=" + (($b | ForEach-Object { $_.ToString('x2') }) -join '') | Add-Content -Encoding ascii .env
```

If `.env` already has an `INTERNAL_TOKEN=` line, edit that line instead of appending a second one.
Share the value with the team privately (never in git, chat history for the repo, an issue or a PR).
Rules: development requires at least 16 characters; staging and production require at least 32 random bytes and must not use the example placeholder. One value applies to the whole Compose stack.
`APP_ENV` must be set explicitly to `development`, `staging`, or `production`; all backend services and workers refuse to start when it is missing, empty, or whitespace.

**3.2 Identity** already receives it (`docker-compose.yml`, `identity` service, line
`INTERNAL_TOKEN: ${INTERNAL_TOKEN:-}`). Nothing to do. In development, a missing or shorter-than-16
character value lets Identity start but refuses every internal call with 401 and logs this warning
(the log never contains the value):

```
Internal APIs are closed and refuse every request: INTERNAL_TOKEN is not set
```

Outside development, Identity refuses to start when the token is missing, shorter than 32 bytes, or
a known placeholder. The startup error names the setting but never includes the token value.

**3.3 M6, `parcel` service.** At the time of writing the block has no `INTERNAL_TOKEN`. Add two
lines to its `environment:` block in `docker-compose.yml`:

```yaml
parcel:
  environment:
    <<: *app-env
    PORT: '8082'
    DATABASE_URL: ... # unchanged
    INTERNAL_TOKEN: ${INTERNAL_TOKEN:-} # added
    IDENTITY_BASE_URL: http://identity:8081 # added (name is yours to choose)
  depends_on:
    postgres:
      condition: service_healthy
    identity: # added, so Identity is up first
      condition: service_healthy
```

**3.4 M1, `notification-worker` service.** Same two variables; it already has the SMTP variables
through `*mail-env`:

```yaml
notification-worker:
  environment:
    <<: [*app-env, *rabbitmq-env, *mail-env]
    NOTIFICATION_QUEUE: ... # unchanged
    NOTIFICATION_ROUTING_KEY: ... # unchanged
    INTERNAL_TOKEN: ${INTERNAL_TOKEN:-} # added
    IDENTITY_BASE_URL: http://identity:8081 # added
```

**3.5 Verify** (after `docker compose up -d --build identity`). Replace the user ID with a real
one from `SELECT id FROM identity.users LIMIT 1;`. `docker compose exec` reads the token from the
container's own environment, so it is never typed or printed:

```bash
docker compose exec identity sh -c \
  'wget -qO- --header="X-Internal-Token: $INTERNAL_TOKEN" --header="X-Request-Id: check-1" \
   http://127.0.0.1:8081/internal/users/<USER_ID>/contact'
# expect 200 and a JSON body; without the header expect 401

curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/internal/users/<USER_ID>/contact
# expect 404 from the gateway
```

## 4. M6: calling the eligibility API from Go

Everything below uses only the standard library plus the shared `requestid` and `config` packages.

```go
type eligibility struct {
	IsActive    bool   `json:"isActive"`
	IsAvailable bool   `json:"isAvailable"`
	Role        string `json:"role"`
}

var ErrCourierNotEligible = errors.New("courier is not eligible")

func (c *IdentityClient) CheckCourier(ctx context.Context, courierID string) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet,
		c.baseURL+"/internal/couriers/"+url.PathEscape(courierID)+"/eligibility", nil)
	if err != nil {
		return err
	}
	req.Header.Set("X-Internal-Token", c.token) // from INTERNAL_TOKEN; never log it
	req.Header.Set("X-Request-Id", requestid.FromContext(ctx))

	resp, err := c.http.Do(req) // http.Client{Timeout: 3 * time.Second}
	if err != nil {
		return fmt.Errorf("identity unavailable: %w", err) // maps to 503 SERVICE_UNAVAILABLE
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return ErrCourierNotEligible
	case http.StatusUnauthorized:
		return errors.New("identity rejected INTERNAL_TOKEN: check configuration") // 500, not 404
	default:
		return fmt.Errorf("identity returned %d", resp.StatusCode)
	}

	var body struct {
		Data eligibility `json:"data"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&body); err != nil {
		return err
	}
	if body.Data.Role != "COURIER" || !body.Data.IsActive || !body.Data.IsAvailable {
		return ErrCourierNotEligible // API 5.2: 422 COURIER_NOT_ELIGIBLE
	}
	return nil
}
```

If `requestid.FromContext(ctx)` is empty (a background job), call `requestid.New()` instead:
Identity rejects a call with no valid request ID.

## 5. M1: sending email with the shared mailer

Package: `packages/go-common/mailer`. It is a new package; no existing shared file changed.

```go
cfg, err := mailer.LoadConfig(config.Load())   // reads SMTP_HOST, SMTP_PORT, SMTP_USER,
if err != nil { return err }                   // SMTP_PASSWORD, SMTP_FROM, SMTP_FROM_NAME
if err := cfg.Validate(); err != nil { return err }
sender, err := mailer.NewSMTP(cfg)             // implements mailer.Mailer
if err != nil { return err }

err = sender.Send(ctx, mailer.Message{
	To:      contact.Email,                    // one plain address, no display name
	Subject: "Your parcel SDN4T8W2LC6 is out for delivery",
	Text:    "Hello Nimal, ...",               // required
	HTML:    "<p>Hello Nimal, ...</p>",        // optional; sent as multipart/alternative
})
```

- **The `notification-worker` container already has the SMTP variables** through `*mail-env`
  (`SMTP_HOST=mailpit`, `SMTP_PORT=1025` by default). Mailpit needs no login. View mail at
  <http://localhost:8025>. For Gmail, set `SMTP_HOST=smtp.gmail.com`, `SMTP_PORT=587`,
  `SMTP_USER`, `SMTP_PASSWORD` (a 16-character App Password) in `.env`.
- **Security built in:** the connection upgrades with STARTTLS and verifies the certificate;
  credentials are never sent without TLS (the send fails instead); recipient and subject with line
  breaks are rejected (header injection); errors never contain the password or body.
- **Errors:** `errors.Is(err, mailer.ErrInvalidMessage)` means the message itself is bad (permanent:
  use `messaging.Permanent`, do not retry). Any other error is a delivery problem (transient:
  retry). A failed email must never block the status update (M1.8 test).
- **Tests:** use `&mailer.Mock{}`. `Sent()` returns what was sent, `FailWith(err)` simulates an outage,
  `Reset()` clears it. It validates messages the same way the real sender does.
- **Do not log** the recipient address, the body, or `SMTP_PASSWORD`. Log the user ID instead.
- Wire it behind your own small interface so the worker's tests never need a network:
  `type emailSender interface { Send(context.Context, mailer.Message) error }`.

## 6. M1 gateway notes

1. `/internal` is already blocked correctly. No change needed.
2. **Gap:** `/api/v1/admin/users` **without a trailing slash** has no exact-match rule (the other
   groups, such as `users` and `couriers`, have `location = ... { return 308 ...; }`). It falls through
   to the web app. The slashed form `/api/v1/admin/users/` is routed correctly, and the frontend
   should call that form. If you add the rule, note that `return 308 /path/;` drops the query string;
   use `return 308 /api/v1/admin/users/$is_args$args;`.
3. `GOOGLE_REDIRECT_URI` must be `http://localhost:8080/api/v1/auth/google/callback`. That path is
   already routed to Identity by `location /api/v1/auth/`.

## 7. What has and has not been verified

Verified (unit tests, `go vet`, `go build`):

- Token check: missing, wrong, different case, prefix, extra characters, empty, and no configured
  token (fails closed). Request ID: missing, empty, blank, illegal characters, too long.
- Not found (unknown and malformed IDs), response fields and values, inactive, unavailable and
  non-courier cases, database errors reported as 500 and never as 404, no token or contact data in
  logs or error bodies.
- Mailer: plain server (Mailpit-style), STARTTLS with certificate verification, AUTH only over TLS,
  header-injection rejection, multipart messages.

**Not verified yet** (needs Docker/PostgreSQL, which were not available to M3):

- The SQL of both endpoints against a real PostgreSQL (integration tests exist and are skipped
  without `TEST_DATABASE_URL`).
- A real call from `parcel` or `notification-worker` inside Compose.
- The gateway answering `404` for `/internal`.
- A real email arriving in Mailpit or Gmail.
