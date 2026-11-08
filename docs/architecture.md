# SwiftDrop architecture

This document describes the shared foundation and implemented service boundaries. M5 customer parcel workflows are detailed in [the implementation record](m5-complete-parcel-flow.md).

## System shape

```text
Browser
  |
  v
Nginx :8080 (only published application port)
  |-- /                         -> Next.js web :3000
  |-- /api/v1/auth|users        -> identity :8081
  |-- /api/v1/couriers          -> identity :8081
  |-- /api/v1/admin/users       -> identity :8081
  |-- /api/v1/admin/notification-logs -> notification-worker :8086
  |-- /api/v1/parcels|tracking  -> parcel :8082
  |-- /api/v1/dispatch|courier  -> parcel :8082
  |-- /api/v1/reports           -> parcel :8082
  |-- /api/v1/pricing|zones     -> pricing :8083
  |-- /api/v1/admin/pricing     -> pricing :8083
  |-- /api/v1/admin/zones       -> pricing :8083
  |-- /api/v1/merchants|merchant -> merchant :8084
  |-- /api/v1/admin/merchants   -> merchant :8084
  |-- /api/v1/ai/chat           -> ai-assistant :8087
  |-- /ws                       -> realtime :8085
  |-- /internal                 -> 404 (never routed)
  |-- /healthz                  -> gateway health

Workers
  |-- notification-worker -> RabbitMQ, PostgreSQL, admin HTTP :8086
  |-- webhook-worker      -> RabbitMQ

PostgreSQL, RabbitMQ, and Mailpit are addressed through Compose service names. RabbitMQ (5672/15672) and Mailpit (1025/8025) publish host ports; PostgreSQL and service ports do not.
```

## Service boundaries

- **Identity** owns authentication, authorization, and user accounts. It issues the access and refresh tokens for email and password login and for Google sign-in, handles password reset, user management for administrators and the courier list, and exposes two service-to-service `/internal` lookups. Profile endpoints are still planned.
- **Parcel** owns parcel lifecycle, status, tracking, reports, and courier delivery reads and writes. Customer creation, owner-scoped reads/history/PDF receipts, internal merchant creation/summary, anonymous public tracking, DISPATCHER/ADMIN daily and outcome reports, and COURIER delivery list/detail reads and transactional status updates are implemented. Creation atomically stores the accepted real pricing quote, initial history and PENDING outbox data; the M6 relay publishes versioned events. Courier status updates lock the parcel, verify the current assignment and state transition, and commit the status, history row and PENDING `parcel.status_changed` outbox event together before the relay publishes `parcel.status_changed.v1`. Proof uploads (6.4) and the remaining lifecycle writes remain planned.
- **Pricing** owns delivery fee calculation and pricing configuration in a future phase. No formulas are implemented.
- **Merchant** owns merchant accounts, integration settings, API client metadata, merchant order snapshots, status timelines, and administrative audit records. Parcel creation and pricing remain owned by their respective services.
- **AI Assistant** owns the Gemini-backed conversational assistant. It validates chat requests, enforces optional JWT identity, retrieves approved embedded knowledge, and runs a bounded function-calling loop over exactly four read-only tools: pricing estimate, public tracking, owned parcel history and owned active deliveries. It never accepts identity or roles from the model, never exposes the Gemini key, and has no database access or write tools. Conversation history and rate limits are in-process and non-durable. See [`apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md`](../apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md) and [`docs/ai-assistant.md`](ai-assistant.md).
- **Realtime** owns the WebSocket `/ws` endpoint and the `parcel.status_changed.v1` consumer. It verifies access tokens (cookie or `?token=`), enforces per-parcel subscriptions and DISPATCHER/ADMIN `scope: all`, maintains an in-process hub, and forwards only public tracking fields. SSE, persistence/replay and multi-replica fan-out are not implemented; replicas sharing one queue each receive a subset of events. The web `useLiveUpdates` hook exists and is unit-tested, but no business screen consumes it yet.
- **Notification worker** consumes the legacy `notification.requested.v1` work item and the customer-owned `parcel.created.v1`/`parcel.status_changed.v1` domain events. It resolves customer contact through the identity internal endpoint and delivers the email channel through the shared SMTP mailer; merchant-owned parcel events are unsupported and dead-lettered. SMS has no provider yet and is a permanent failure. The worker records processed event IDs in `notification.processed_events` and delivery outcomes in `notification.notification_logs` (`SENT`, `PENDING`, `FAILED`). The success log and processed-event writes share one transaction; delivery is at-least-once, not exactly-once. A context-aware background scheduler deletes processed-event rows older than 30 days at startup and every 24 hours by default; cleanup failures are logged and retried without stopping consumption, cleanup never modifies delivery logs, and graceful shutdown waits for the scheduler to stop. The worker also exposes an internal ADMIN-only read API on port `8086` for `GET /api/v1/admin/notification-logs`; it returns masked recipients and is reachable only through the gateway. The customer-owned domain-event path is implemented; `notification.requested.v1` remains legacy. Merchant-owned parcels have no approved recipient-contact lookup, so merchant-owned parcel notifications remain unsupported until an authorized mechanism is approved.
- **Webhook worker** consumes `parcel.status_changed.v1` on its own durable queue and logs only. The target flow delivers the public merchant webhook projection; delivery, signing, and attempt logging are not implemented.

A service owns its data. The local environment uses one PostgreSQL instance with separate schemas:

| Service      | Schema         |
| ------------ | -------------- |
| Identity     | `identity`     |
| Parcel       | `parcel`       |
| Pricing      | `pricing`      |
| Merchant     | `merchant`     |
| Notification | `notification` |

Realtime is stateless and has no schema.

Services must not read or write another service's schema. Cross-service reads use HTTP; cross-service side effects use events.

## Communication

### Synchronous HTTP

Use HTTP when the caller needs an immediate response, such as validating a request, reading a resource, or determining a price before accepting an operation.

### Asynchronous RabbitMQ events

Use events when work can happen independently and be retried, such as notification delivery or webhook dispatch. When producers are added, they publish versioned events to the durable topic exchange `swiftdrop.events`. Consumers declare durable queues bound to versioned routing keys.

Consumers are at-least-once: they must deduplicate by `event_id` and be idempotent. Duplicate or redelivered messages are expected, including after reconnects and retries. A message is acknowledged only after successful handling or after a confirmed retry/dead-letter publication. Malformed envelopes and unsupported event types or versions are permanent and go to the dead-letter queue. Other handler failures are retried up to `RABBITMQ_MAX_RETRIES` with `RABBITMQ_RETRY_DELAY`, then dead-lettered. In-flight messages are requeued on shutdown.

Before a handler runs, the shared consumer validates message framing: the body must be at most 1 MiB and the content type must be absent or `application/json`. Current worker handlers then validate the shared envelope, including a canonical version suffix match and object-shaped `data`. Framing and envelope failures are permanent. Payload validation against a per-event JSON Schema is not implemented yet.

The main exchange is unchanged. The shared consumer declares additional durable resources: `<exchange>.retry` and `<queue>.retry.v2` for delayed retries, and `<exchange>.dlx` and `<queue>.dlq` for dead letters. A retry is dead-lettered to its owning queue through the default exchange (`x-dead-letter-exchange: ""`, `x-dead-letter-routing-key: <queue>`) after the delay, so a retry never reaches another worker bound to the same domain routing key. Multiple workers can therefore consume the same versioned event through separate queues without sharing retry delivery. The retry queue name carries a topology suffix because durable queue arguments are immutable; the legacy `<queue>.retry` queue is no longer written to and should be drained and deleted after upgrading. Retry and dead-letter publications use publisher confirms; the original delivery is acknowledged only after the broker confirms the publication.

The outbox storage labels remain `parcel.created`, `parcel.assigned`, and `parcel.status_changed`; the stored `routing_key` is legacy/internal. The relay maps those labels to `parcel.created.v1`, `parcel.assigned.v1`, and `parcel.status_changed.v1`, uses the versioned event name as the published routing key, and builds the canonical envelope. M5 creation writes the agreed outbox data in the same transaction as the parcel and initial history. The merged M6 relay publishes committed events with confirms and bounded retries. M7 courier status updates write the parcel row, history row and PENDING `parcel.status_changed` outbox row in one transaction after locking the parcel and verifying the current assignment. See [`contracts/events/README.md`](../contracts/events/README.md) for the canonical mapping and current implementation status.

When the broker or session fails, workers reconnect with capped exponential backoff and jitter between `RABBITMQ_RECONNECT_MIN` and `RABBITMQ_RECONNECT_MAX`. The backoff resets after a session reaches the consume phase, and reconnect waits and dials honor context cancellation so shutdown is prompt. A second termination signal forces exit if graceful shutdown does not finish.

Handler deadlines are cooperative: handlers must honor their context because Go cannot preempt arbitrary code. Processing remains serial; the configured prefetch is a broker buffer, not concurrency. Do not add concurrent handlers solely to make prefetch look effective; introduce concurrency only with an explicit ordering, acknowledgement, and shutdown design.

The public merchant webhook is a separate projection of `parcel.status_changed.v1`. It keeps the API v1.1 camelCase public body with `eventType: parcel.status_changed`, adds `schemaVersion: 1`, and uses the API v1.1 HMAC signing and six-attempt retry contract. The current worker does not implement HTTP delivery, signing, or delivery-attempt persistence; its `parcel.status_changed.v1` consumer only validates and logs. `externalOrderId` is not in the domain event; the authenticated internal parcel summary now supplies it to trusted callers. Webhook delivery remains planned. See [`contracts/events/README.md`](../contracts/events/README.md) and API v1.1 section 10.

See [`contracts/events/README.md`](../contracts/events/README.md).

## HTTP routing and prefix stripping

Nginx exposes API v1.1 path groups and strips the `/api/v1` prefix before forwarding:

| Public path group                                                                                                        | Upstream                     |
| ------------------------------------------------------------------------------------------------------------------------ | ---------------------------- |
| `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers`                                               | identity                     |
| `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/admin/pricing`, `/api/v1/admin/zones`                                       | pricing                      |
| `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports` | parcel                       |
| `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants`                                                       | merchant                     |
| `/api/v1/admin/notification-logs`                                                                                        | notification-worker          |
| `/api/v1/ai`                                                                                                             | ai-assistant                 |
| `/ws`                                                                                                                    | realtime (WebSocket upgrade) |

Gin services register root paths such as `/auth/...`, `/parcels/...`, `/health`, and `/ready`. For example, `/api/v1/parcels/123` becomes `/parcels/123` at the parcel service. This keeps service routers independent of the edge topology. Any new route must be updated in the Gin router, the OpenAPI specification, and the Nginx mapping together.

`/internal/*` is never routed and returns a JSON `404`. `/api/v1/ai/chat` is forwarded to the AI assistant on internal port `8087`; the browser never reaches Gemini directly. Only `/healthz` is public at the gateway; service `/health` and `/ready` are internal.

All HTTP services share the same edge-facing behavior:

- Unmatched paths return the API v1.1 error envelope with code `NOT_FOUND` and `meta.requestId`.
- Disallowed methods on known paths return `405` with code `METHOD_NOT_ALLOWED` and an `Allow` header.
- Panics are recovered by the shared middleware. If no response has been written, the client receives `INTERNAL_ERROR`; the stack trace is logged server-side and never returned.
- Health and readiness probes are logged at debug level rather than as normal traffic.
- Servers set a 1 MiB header limit by default and route `http.Server.ErrorLog` through the structured logger.
- `X-Request-Id` is echoed or generated on every response and appears as `meta.requestId` in errors.

The Next.js application serves `/` and its own routes. Nothing else outside `/api/v1/` and `/ws` is proxied to Go services.

Browser requests use same-origin credentials, `SameSite=Lax` cookies and a JSON content type. Together these narrow CSRF exposure, because a cross-site HTML form cannot send a JSON body without a CORS preflight. They are mitigations, not a complete CSRF defence, and no CSRF token exists until the API contract and backend implement one.

## Configuration

Configuration is environment-based. Values are matched case-insensitively and trimmed by the shared loader. Invalid required values fail startup clearly. `APP_ENV` must be explicitly set to `development`, `staging`, or `production`; all backend services and workers do not fall back to development when it is missing, empty, or whitespace. `RABBITMQ_URL` must use the `amqp` or `amqps` scheme and include a host; validation errors never include the URL. The root `.env.example` documents local development values. Docker Compose passes container-network addresses such as `postgres` and `rabbitmq`; `localhost` is used only for host-to-container access. PostgreSQL is internal to the Compose network and publishes no host port. Host-run services and host-run PostgreSQL integration tests need a separately running PostgreSQL instance; they must not depend on a Compose port mapping.

HTTP services read:

- `APP_ENV`
- `LOG_LEVEL`
- `PORT`
- optional `DATABASE_URL`; when set, services open the shared pgx pool and `/ready` reports `checks.database`. When unset, the service starts and `/ready` returns `503`.
- Pricing and parcel also read `JWT_ACCESS_SECRET` and `COOKIE_NAME` for authenticated admin/dispatcher routes. Outside development a configured secret must be at least 32 random bytes and must not be a placeholder; an unset secret leaves public endpoints running but makes those routes fail closed with `503`.
- The AI assistant reads `AI_*`, `GEMINI_API_KEY`, `JWT_ACCESS_SECRET`, `COOKIE_NAME`, `PRICING_BASE_URL`/`PRICING_TIMEOUT` and `PARCEL_BASE_URL`/`PARCEL_TIMEOUT`. It has no database. The Gemini key is injected only into that container and never reaches the browser or logs.
- Realtime reads `JWT_ACCESS_SECRET`, `COOKIE_NAME`, `REALTIME_ALLOWED_ORIGINS`, `REALTIME_QUEUE`, `REALTIME_ROUTING_KEY`, and the shared `RABBITMQ_*` settings. It is stateless and has no schema.

Workers read:

- `APP_ENV`, `LOG_LEVEL`
- required `RABBITMQ_URL`
- `RABBITMQ_EXCHANGE`, queue, routing key, prefetch, handler timeout, retry limit, retry delay, and reconnect bounds

The notification worker also requires `SMTP_HOST` and reads `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, and `SMTP_FROM_NAME`, plus the optional `SMTP_REPLY_TO`, `SMTP_SECURE` (implicit TLS, automatic on port 465) and `SUPPORT_EMAIL` (see [email.md](email.md)). It reads `PORT` (default `8086`), optional `JWT_ACCESS_SECRET`, and `COOKIE_NAME` (default `sd_access`) for its ADMIN-only read API. Outside development a configured secret must be at least 32 random bytes and must not be a placeholder; an unset secret leaves delivery running but makes the admin endpoint fail closed with `503`. Mailpit is the local sink (`SMTP_HOST=mailpit`, `SMTP_PORT=1025`); Gmail SMTP (`smtp.gmail.com`, port 587, STARTTLS, App Password) is the opt-in real-delivery provider. The Identity service sends password-reset email and the notification worker sends notification email through the shared mailer in `packages/go-common/mailer`, which reads this configuration. Identity also exposes the service-to-service `/internal` endpoints (`X-Internal-Token`), described in `docs/identity-internal-api.md`. The notification worker also reads `IDENTITY_BASE_URL` (default `http://identity:8081`), `IDENTITY_TIMEOUT` (default `3s`), and `INTERNAL_TOKEN` for identity contact lookups, `FRONTEND_BASE_URL` and `SUPPORT_EMAIL` for email links and the footer, and `NOTIFICATION_PARCEL_CREATED_QUEUE`/`NOTIFICATION_PARCEL_CREATED_ROUTING_KEY`/`NOTIFICATION_PARCEL_STATUS_QUEUE`/`NOTIFICATION_PARCEL_STATUS_ROUTING_KEY` for the two domain-event consumers. TLS certificate verification stays enabled; certificate revocation checking is not implemented.

The maximum RabbitMQ message body size is 1 MiB and is fixed in `messaging.MaxMessageBytes`; it is not configurable in this phase. Secrets and connection strings are never logged.

Demo users are created by a local operator tool, `make seed`, not by service startup or migrations. It reads `DATABASE_URL` and the ten `SEED_*` account variables, seeds only the five identity demo users and two courier profiles, and leaves merchant, API-key and parcel data untouched. It is not a runtime dependency between the identity and pricing services; its only cross-schema read is the lookup of active `pricing.zones` rows by code to resolve the two courier home zones. The seed command is outside the seven fail-closed backend services and workers; its `APP_ENV` development default only controls logging, so verify `DATABASE_URL` before running it.

## Health and readiness

Every HTTP service exposes:

- `GET /health` — liveness, `{"status":"ok"}`. `/healthz` remains a deprecated alias.
- `GET /ready` — readiness. Returns `200 {"status":"ready","checks":{"database":"ok"}}` after a bounded-context database ping, or `503` with `"database":"down"` when `DATABASE_URL` is unset or unreachable. `/readyz` remains a deprecated alias.

HTTP service healthchecks use `/ready`, so a service with no database connectivity is reported unhealthy. Mailpit's healthcheck uses `/readyz`, which also verifies its storage, rather than the liveness-only `/livez`. Worker healthchecks match the service process by executable path (`pgrep -x /usr/local/bin/service`), and the notification worker additionally checks its `/ready` endpoint, so a crashed or unready process cannot be reported healthy by the healthcheck shell's own command line. The gateway exposes only `/healthz` publicly. Readiness does not currently check RabbitMQ; workers remain log-driven.

Workers do not publish host ports and do not expose public HTTP endpoints, with one deliberate exception: the notification worker exposes an internal ADMIN-only read API on `8086`, reachable only through the gateway at `/api/v1/admin/notification-logs`. That route still executes the shared authentication and role middleware inside the worker. The worker's delivery lifecycle remains log-driven: it connects, declares the durable main and failure-routing topology, consumes with explicit acknowledgements, retries retryable failures with a bounded delay, dead-letters permanent or exhausted failures, and reconnects with capped exponential backoff and jitter. In-flight messages are requeued on shutdown, and a second termination signal forces exit if graceful shutdown does not finish.

## Persistence and migrations

PostgreSQL is the primary relational store. SQL migrations live under `migrations/<service>/` and are applied with `golang-migrate`. Each service uses a distinct migration version table in the shared local database. Services never run migrations at startup.

`000001` creates each service schema. `000002_tables` is reserved by the platform foundation for the baseline tables in [`db.sql`](db.sql) (26 tables across five schemas). Owners add new migrations from `000003`. Parcel `000003_dispatch_foundations` extends the baseline `parcel_assignments` and `outbox_events` tables without recreating them. Parcel `000004_price_breakdown_total` re-validates the price-breakdown total. Parcel `000005_outbox_event_types` reasserts the unversioned outbox event/routing-key pairs, widens `outbox_events.last_error` to `TEXT`, and adds the published-event cleanup and current-courier workload indexes; stored outbox labels stay `parcel.created`, `parcel.assigned`, and `parcel.status_changed`, and the relay still owns the mapping to the `.v1` bus names. Parcel `000006_customer_parcel_list_index` adds the additive `(customer_user_id, created_at, id)` index for the ordered customer list; no applied migration is rewritten. The Parcel relay is cancelled and awaited before the database and broker connections close, so an in-flight publish either completes and marks its row `PUBLISHED` or stays `PENDING` for redelivery. `make migrate` bootstraps the shared `public.set_updated_at()` function and applies all five services in order; `make migrate-down` rolls back one migration per service in reverse order.

Mailpit provides local SMTP (`1025`, published as `MAILPIT_SMTP_PORT`) and a web UI (`8025`). Gmail SMTP relay can replace it for real delivery; the mailbox provider is configuration, not part of the schema. Compose publishes only the gateway (`8080`), RabbitMQ (`5672`/`15672`), and Mailpit (`1025`/`8025`).

## Shared Go code

`packages/go-common` contains infrastructure only:

- environment loading and validation, including credential-safe AMQP URL checks
- structured logging
- request IDs and HTTP response helpers
- HTTP server lifecycle, structured panic recovery, and JSON 404/405 behavior
- RabbitMQ event envelope and message framing validation, consumer lifecycle, failure classification, reconnect backoff, and publisher-confirmed retry/dead-letter routing

It contains no business entities, shared repositories, or cross-service business rules. All Go code belongs to the single root module `github.com/Vineth-R/swiftdrop`.

## Verification

Local checks for the current foundation:

```sh
gofmt -l apps/services packages/go-common tests/integration
go vet ./...
go test -race ./...
go build ./...
docker compose config --quiet
```

Broker-backed messaging tests require RabbitMQ:

```sh
docker compose up -d rabbitmq
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -tags=integration ./tests/integration
```

Dispatch database tests require PostgreSQL and create isolated test databases. Compose PostgreSQL is not published, so point `TEST_DATABASE_URL` at a separately running local PostgreSQL instance (or a temporary published test instance):

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  make test-db
```

CI runs the checks in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml): Go formatting, vet, race tests, build, and Compose config validation; separate RabbitMQ and PostgreSQL integration jobs that fail on skipped tests; frontend lint, typecheck, and build; Chromium-only Playwright E2E; and representative `identity` and web Docker image builds. See the [README](../README.md#ci) for the job summary and [`CONTRIBUTING.md`](../CONTRIBUTING.md) for the contribution workflow.

## Deliberate omissions

The foundation does not include parcel workflows, merchant onboarding, API key management, merchant-owned parcel notification contact resolution, SMS notification delivery, webhook signing or webhook delivery retries, SSE or persistent realtime replay, payments, production TLS, Kubernetes, Terraform, or deployment automation.
