# SwiftDrop — Demo Readiness

This is the reproducible local demonstration path. It assumes the repository is
checked out and Docker/Compose v2 is available.

## 1. Prerequisites

| Tool     | Version                     |
| -------- | --------------------------- |
| Go       | 1.27.4 (or compatible 1.27) |
| Node.js  | 24                          |
| pnpm     | 12.15.0 via Corepack        |
| Docker   | Compose v2                  |
| Optional | `k6`, Chromium, `psql`      |

```sh
corepack enable
corepack prepare pnpm@12.15.0 --activate
pnpm install --frozen-lockfile
```

## 2. Required environment

Copy the documented template and fill the local values. `.env` is untracked and
must never be committed.

```sh
cp .env.example .env
```

At minimum the demo stack needs:

| Variable                                                            | Purpose                   | Local default / note                                                |
| ------------------------------------------------------------------- | ------------------------- | ------------------------------------------------------------------- |
| `APP_ENV`                                                           | Runtime mode              | `development`                                                       |
| `POSTGRES_USER` / `POSTGRES_PASSWORD` / `POSTGRES_DB`               | Compose PostgreSQL        | `swiftdrop` / `swiftdrop` / `swiftdrop`                             |
| `JWT_ACCESS_SECRET`                                                 | Access-token signing      | Set a strong local value (32+ random bytes)                         |
| `INTERNAL_TOKEN`                                                    | Service-to-service auth   | Set a strong local value                                            |
| `WEBHOOK_SECRET_KEY`                                                | Webhook secret encryption | Generate with `openssl rand -hex 32`                                |
| `COOKIE_SECURE`                                                     | Cookie Secure flag        | `false` for plain local HTTP                                        |
| `FRONTEND_BASE_URL`                                                 | Email links               | `http://localhost:8080`                                             |
| `SMTP_HOST` / `SMTP_PORT`                                           | Mailpit                   | `mailpit` / `1025`                                                  |
| `STORAGE_PROVIDER`                                                  | Proof storage             | `local` for the default demo                                        |
| `GEMINI_API_KEY`                                                    | AI assistant              | Optional; use `AI_PROVIDER=mock` without a key                      |
| `GOOGLE_CLIENT_ID` / `GOOGLE_CLIENT_SECRET` / `GOOGLE_REDIRECT_URI` | Google login              | Optional; routes are absent/disabled without them                   |
| `SEED_*_PASSWORD`                                                   | Demo accounts             | Required by `make seed-demo`                                        |
| `SEED_MERCHANT_PASSWORD`                                            | Demo merchant             | Required by `make seed-demo`                                        |
| `SEED_MERCHANT_API_KEY`                                             | Demo merchant key         | Required by `make seed-demo`; generate `sdk_live_<16 hex>_<48 hex>` |

For a no-key local AI demo:

```sh
AI_PROVIDER=mock
```

`AI_PROVIDER=mock` is rejected outside `APP_ENV=development`.

## 3. Start the stack

```sh
make up-build
```

This builds the images, starts PostgreSQL/RabbitMQ/Mailpit, applies all
migrations, starts the services/workers/web/Nginx, waits for readiness, and
verifies the gateway routes.

Useful checks:

```sh
make logs
docker compose ps
curl -s http://localhost:8080/healthz
curl -s http://localhost:8080/api/v1/zones
```

- Gateway: `http://localhost:8080`
- Mailpit UI: `http://localhost:8025`
- RabbitMQ management: `http://localhost:15672`
- Internal services are not published on the host.

## 4. Seed deterministic demo data

`make seed-demo` creates:

- five identity demo accounts (ADMIN, DISPATCHER, two COURIERs, CUSTOMER);
- an APPROVED merchant with an Identity owner and a supplied API key;
- one parcel for every canonical status with deterministic tracking numbers,
  history and assignments.

```sh
export SEED_ADMIN_PASSWORD='...'
export SEED_DISPATCHER_PASSWORD='...'
export SEED_COURIER_1_PASSWORD='...'
export SEED_COURIER_2_PASSWORD='...'
export SEED_CUSTOMER_PASSWORD='...'
export SEED_MERCHANT_PASSWORD='...'
export SEED_MERCHANT_API_KEY="sdk_live_$(openssl rand -hex 8)_$(openssl rand -hex 24)"

make seed-demo
```

The seed is idempotent: existing users are not overwritten and a missing courier
profile is repaired. It refuses `APP_ENV=production` and a non-local database
unless `ALLOW_REMOTE_SEED=1` is explicitly set for a disposable target.

Demo account emails come from `.env`:

| Role       | Variable                | Example                       |
| ---------- | ----------------------- | ----------------------------- |
| ADMIN      | `SEED_ADMIN_EMAIL`      | `seed.admin@example.com`      |
| DISPATCHER | `SEED_DISPATCHER_EMAIL` | `seed.dispatcher@example.com` |
| COURIER 1  | `SEED_COURIER_1_EMAIL`  | `seed.courier.1@example.com`  |
| COURIER 2  | `SEED_COURIER_2_EMAIL`  | `seed.courier.2@example.com`  |
| CUSTOMER   | `SEED_CUSTOMER_EMAIL`   | `seed.customer@example.com`   |
| MERCHANT   | `SEED_MERCHANT_EMAIL`   | `seed.merchant@example.com`   |

Passwords are the values you exported; the seed never prints them.

## 5. Complete demonstration workflow

### 5.1 Customer creates a parcel

1. Open `http://localhost:8080/login`.
2. Sign in with the seeded CUSTOMER email/password.
3. Go to **New delivery**.
4. Choose pickup/destination zones, weight and size; confirm the real quote.
5. Fill sender/recipient details and create the delivery.
6. Note the tracking number (`SDK…`/`SDN…`).
7. Open **My parcels**; the new parcel and its PDF receipt are available.

What to verify:

- the quote is a real pricing-service result;
- the parcel detail shows `CREATED` and an initial history row;
- the receipt downloads as a PDF;
- `/track?number=...` shows the public projection without private fields.

### 5.2 Dispatcher assigns a courier

1. Sign in as the seeded DISPATCHER in a second browser profile.
2. Open **All parcels** (or **Unassigned**).
3. Find the tracking number.
4. Open the assignment dialog and choose COURIER 1.
5. Confirm.

What to verify:

- the parcel leaves the unassigned queue;
- the workload count increases;
- the assignment history records the dispatcher and courier;
- a live `parcel.assigned.updated` notice reaches the dispatch screen.

### 5.3 Courier picks up and delivers

1. Sign in as COURIER 1 in a third browser profile.
2. The parcel appears in **Today**.
3. Open the delivery detail.
4. Upload a pickup photo and confirm pickup.
5. Mark **Out for delivery**.
6. Enter the recipient name, upload a delivery photo, add a note and confirm
   delivery.

What to verify:

- pickup commits `PICKED_UP` and `IN_TRANSIT` history;
- delivery requires recipient + photo;
- the activity section shows both proof photos;
- the status is `DELIVERED` and cannot be changed again.

### 5.4 Customer, dispatcher and admin observe the result

- Customer **My parcels** detail shows `DELIVERED`, history, proof and receipt.
- Public `/track?number=...` shows the final status and timeline.
- Dispatcher **All parcels** shows `DELIVERED` (live or after refresh).
- ADMIN **Notification logs** shows email records for the parcel.
- Mailpit (`http://localhost:8025`) shows the delivered emails.

### 5.5 Merchant demo

1. Sign in as the seeded merchant.
2. **Orders** shows the merchant order dashboard.
3. Use **Create via API** with the supplied `SEED_MERCHANT_API_KEY` to create an
   order; the panel never stores the key.
4. **Webhook** lets you save an HTTPS endpoint, rotate the signing secret, send
   a signed test event and inspect delivery attempts.
5. **API keys** creates/revokes keys; the full key is shown once.

For a local webhook receiver, use the `webhook-test` Compose profile and the
`tests/webhook-sink` receiver, or any public HTTPS endpoint that verifies
`X-SwiftDrop-Signature`.

### 5.6 Admin demo

1. Sign in as ADMIN.
2. **Users**: create/edit/deactivate a staff account; verify a deactivated account
   cannot log in.
3. **Merchants**: approve/reject/suspend and send an administrative email.
4. **Zones** / **Pricing**: change a zone, rule, band or surcharge; create a new
   parcel and confirm the new quote.
5. **Reports**: daily/outcomes reflect the created/delivered parcels.
6. **Notification logs**: masked recipients and status filters.

### 5.7 AI assistant

1. Open the chat widget as a guest.
2. Ask a public question (services, pricing).
3. Sign in as CUSTOMER and ask about your own parcel.
4. Try a private request as a guest; the assistant asks you to sign in.

Without a Gemini key, set `AI_PROVIDER=mock` first.

## 6. Troubleshooting

| Symptom                                       | Cause / fix                                                                                                                                                     |
| --------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Nginx returns 502                             | An upstream is not healthy. `docker compose ps`, then `make logs`. Rebuild the changed service if the image is older than the code.                             |
| `/ready` is 503                               | Check the named dependency. Parcel/merchant/realtime need PostgreSQL and/or RabbitMQ consumers. AI is 503 without a provider, which is expected in no-key mode. |
| Login succeeds but pages redirect to `/login` | Cookies are Secure on plain HTTP. Set `COOKIE_SECURE=false` for local HTTP.                                                                                     |
| Login says email verification is required     | Open Mailpit, click the verification link, or use the seeded verified accounts.                                                                                 |
| Password reset routes are 404                 | SMTP/`FRONTEND_BASE_URL` is not configured; identity starts without reset routes.                                                                               |
| New customer cannot verify                    | SMTP is disabled. Configure Mailpit/SMTP before registering a new customer.                                                                                     |
| Merchant login refused                        | The merchant is PENDING/REJECTED/SUSPENDED. Approve it as ADMIN.                                                                                                |
| Merchant order API 401                        | Use `X-API-Key`, not a session cookie, for `POST /merchant/orders`.                                                                                             |
| Parcel creation 429                           | The per-customer limiter (30/min) was reached; wait for `Retry-After`.                                                                                          |
| Webhook test 429                              | The per-merchant limiter (10/min) was reached.                                                                                                                  |
| Webhook test 422 `WEBHOOK_URL_NOT_ALLOWED`    | The URL is not public HTTPS or resolves to a blocked address.                                                                                                   |
| Proof upload 413                              | The photo exceeds 5 MiB or the gateway body limit.                                                                                                              |
| Proof upload 422                              | The file is not a valid JPEG/PNG or the declared MIME type does not match.                                                                                      |
| Realtime stays “Reconnecting”                 | Check `REALTIME_ALLOWED_ORIGINS` and the access token. Behind `nx dev`, set `NEXT_PUBLIC_WS_URL=ws://localhost:8080/ws`.                                        |
| AI chat 503                                   | No Gemini key and `AI_PROVIDER` is not `mock`.                                                                                                                  |
| Outbox failed backlog                         | `make outbox-failed`; replay or expire with the documented windows.                                                                                             |
| Production DB migration error                 | The repository head is ahead of the documented production snapshot; apply the approved migration window first.                                                  |

## 7. Useful commands

```sh
make up-build                 # build + start + migrate + route checks
make migrate                  # apply migrations
make migrate-status           # versions and dirty state
make seed-demo                # all deterministic demo data
make test                     # Go unit tests with race detector
make test-db                  # PostgreSQL integration tests
make test-access              # Compose gateway access matrix
pnpm exec nx e2e @org/web-e2e # Playwright
pnpm exec nx run @org/web-e2e:e2e --args="--project=real-stack"
make outbox-failed            # failed outbox backlog
make prune-retention          # audit/webhook retention dry run
make db-backup                # verified backup outside the repo
```

## 8. Demo readiness decision

The demo was executed in this acceptance pass: `make up-build`, migrations,
`make seed-demo`, the route checks, the 63-endpoint access matrix and the real-stack Playwright suite all passed. Use the troubleshooting rows above if
the local `.env` differs from the Mailpit defaults.

---

## 9. Non-deployment finalization additions

### Audit logs

1. Sign in as ADMIN.
2. Open **Administration → Audit logs**.
3. Choose Identity, Merchant or Pricing; filter by actor, action, resource type
   or date range.
4. Open **Show details**. Credential-like keys are shown as `[redacted]`.

### Failed-outbox recovery

```sh
make outbox-failed
make outbox-replay OUTBOX_ARGS="-older-than 720h"
make outbox-expire OUTBOX_ARGS="-older-than 720h"
```

Replay/expire are dry runs unless the Make targets pass `-apply`.

### Retention

```sh
make prune-retention          # dry run
make prune-retention-apply    # bounded batches
BATCH_SIZE=2000 MAX_BATCHES=50 make prune-retention-apply
```

### Webhook reliability demo

```sh
docker compose --profile webhook-test up -d --wait webhook-sink
WEBHOOK_RETRY_DELAYS=2s,2s,2s,2s,2s \
WEBHOOK_TEST_ORIGIN=https://webhook-sink:8098 \
WEBHOOK_TEST_CA_FILE=/test-certs/cert.pem \
docker compose up -d --no-deps --force-recreate merchant webhook-worker
```

Run `webhook-reliability.real.spec.ts`. Afterwards recreate merchant/webhook-worker
without the test env so normal webhook validation applies.

### S3-compatible storage demo

```sh
make storage-up
docker run --rm --network swiftdrop_default \
  -e AWS_ACCESS_KEY_ID=... -e AWS_SECRET_ACCESS_KEY=... \
  -e AWS_DEFAULT_REGION=us-east-2 amazon/aws-cli:2.28.0 \
  --endpoint-url http://s3:7070 s3api create-bucket \
  --bucket delivery-proofs-test \
  --create-bucket-configuration LocationConstraint=us-east-2
STORAGE_TEST_ENDPOINT=http://localhost:7070 \
STORAGE_TEST_ACCESS_KEY=... STORAGE_TEST_SECRET_KEY=... \
STORAGE_TEST_BUCKET=delivery-proofs-test STORAGE_TEST_REGION=us-east-2 \
make test-storage
```

### Live Gemini

```sh
AI_LIVE_SMOKE=1 BASE_URL=http://localhost:8080 node scripts/ai-live-smoke.mjs
```

### Google OAuth

**Status: PASS** — automated tests pass, Google accepted the configured client ID
and redirect URI, and the project owner completed interactive consent and
account linking. The exact manual verification date was not recorded. This
remains a manual, non-CI check.

To re-verify after a configuration change, inspect the redirect locally:

```sh
curl -s -o /dev/null -D - http://localhost:8080/api/v1/auth/google/login
```

Then open the returned URL, complete consent with an authorized Google account,
and confirm the callback creates/links the account and issues a session. Never
log the authorization code, tokens or client secret.

### Dedicated notification-flow suite

The suite starts its own disposable Compose project and uses Mailpit, so it must
not reuse the developer stack's `.env` SMTP settings.

```sh
export COMPOSE_PROJECT_NAME=swiftdrop-notificationflow
export COMPOSE_FILE='docker-compose.yml:tests/integration/notificationflow/docker-compose.identity.yml'
export RABBITMQ_PORT=25672 RABBITMQ_MANAGEMENT_PORT=25673
export MAILPIT_SMTP_PORT=21025 MAILPIT_UI_PORT=28025 NGINX_PORT=18080
export SMTP_HOST=mailpit SMTP_PORT=1025 SMTP_USER= SMTP_PASSWORD= SMTP_SECURE=auto
export RABBITMQ_RETRY_DELAY=5s RABBITMQ_MAX_RETRIES=2

docker compose down -v
docker compose up -d --wait --wait-timeout 300 \
  postgres rabbitmq mailpit identity merchant notification-worker
make migrate
NOTIFICATIONFLOW_IDENTITY_URL=http://127.0.0.1:18081 \
NOTIFICATIONFLOW_MERCHANT_URL=http://127.0.0.1:18084 \
NOTIFICATIONFLOW_RABBITMQ_URL='amqp://swiftdrop:swiftdrop@127.0.0.1:25672/' \
NOTIFICATIONFLOW_RABBITMQ_MANAGEMENT_URL='http://127.0.0.1:25673' \
NOTIFICATIONFLOW_MAILPIT_URL='http://127.0.0.1:28025' \
RABBITMQ_RETRY_DELAY=5s RABBITMQ_MAX_RETRIES=2 NOTIFICATIONFLOW_RETRY_WAIT=90s \
go test -race -tags=notificationflow -count=1 -v ./tests/integration/notificationflow/...

docker compose down -v
```

`RABBITMQ_RETRY_DELAY` and `RABBITMQ_MAX_RETRIES` must be exported before
`docker compose up`: the worker container reads them, not the test process. With
the default 30s/3-retry worker settings, the identity-unavailable dead-letter
test's 60s wait expires. CI sets them at job level. Full details are in
`tests/integration/notificationflow/README.md`.
