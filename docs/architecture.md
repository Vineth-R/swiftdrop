# SwiftDrop architecture

This document describes the initialized foundation. It intentionally stops short of business workflows.

## System shape

```text
Browser
  |
  v
Nginx :8080
  |-- /                 -> Next.js web :3000
  |-- /api/v1/identity  -> identity :8081
  |-- /api/v1/parcels   -> parcel :8082
  |-- /api/v1/pricing   -> pricing :8083
  |-- /api/v1/merchants -> merchant :8084
  |-- /api/v1/realtime  -> realtime :8085

Workers
  |-- notification-worker -> RabbitMQ
  |-- webhook-worker      -> RabbitMQ

Containers address infrastructure through Docker Compose service names; host-to-container access uses the published ports.
```

## Service boundaries

- **Identity** owns authentication, authorization, and identity profiles in a future phase. It does not issue tokens yet.
- **Parcel** owns parcel lifecycle, status, and tracking in a future phase.
- **Pricing** owns delivery fee calculation and pricing configuration in a future phase. No formulas are implemented.
- **Merchant** owns merchant accounts, integration settings, API client metadata, merchant order snapshots, status timelines, and administrative audit records. Parcel creation and pricing remain owned by their respective services.
- **Realtime** will own live tracking connections and subscriptions. It currently provides health endpoints and a structure for future WebSocket/SSE work.
- **Notification worker** will consume notification events and call external providers in a future phase.
- **Webhook worker** will deliver outgoing merchant webhooks and eventually manage retries.

A service owns its data. The local environment uses one PostgreSQL instance with separate schemas:

| Service  | Schema     |
| -------- | ---------- |
| Identity | `identity` |
| Parcel   | `parcel`   |
| Pricing  | `pricing`  |
| Merchant | `merchant` |

Services must not read or write another service's schema. Cross-service reads use HTTP; cross-service side effects use events.

## Communication

### Synchronous HTTP

Use HTTP when the caller needs an immediate response, such as validating a request, reading a resource, or determining a price before accepting an operation.

### Asynchronous RabbitMQ events

Use events when work can happen independently and be retried, such as notification delivery or webhook dispatch. When producers are added, they publish versioned events to the durable topic exchange `swiftdrop.events`. Consumers declare durable queues bound to versioned routing keys.

Consumers are at-least-once: they must deduplicate by `event_id` and be idempotent. Duplicate or redelivered messages are expected, including after reconnects and retries. A message is acknowledged only after successful handling or after a confirmed retry/dead-letter publication. Malformed envelopes and unsupported event types or versions are permanent and go to the dead-letter queue. Other handler failures are retried up to `RABBITMQ_MAX_RETRIES` with `RABBITMQ_RETRY_DELAY`, then dead-lettered. In-flight messages are requeued on shutdown.

Before a handler runs, the shared consumer validates message framing: the body must be at most 1 MiB and the content type must be absent or `application/json`. Current worker handlers then validate the shared envelope, including a canonical version suffix match and object-shaped `data`. Framing and envelope failures are permanent. Payload validation against a per-event JSON Schema is not implemented yet.

The main exchange and queues are unchanged. The shared consumer declares additional durable resources: `<exchange>.retry` and `<queue>.retry` for delayed retries, and `<exchange>.dlx` and `<queue>.dlq` for dead letters. Retry messages return to the original exchange and routing key after the delay; a worker routing key must not fan out to multiple worker queues. Retry and dead-letter publications use publisher confirms; the original delivery is acknowledged only after the broker confirms the publication.

When the broker or session fails, workers reconnect with capped exponential backoff and jitter between `RABBITMQ_RECONNECT_MIN` and `RABBITMQ_RECONNECT_MAX`. The backoff resets after a session reaches the consume phase, and reconnect waits and dials honor context cancellation so shutdown is prompt. A second termination signal forces exit if graceful shutdown does not finish.

Handler deadlines are cooperative: handlers must honor their context because Go cannot preempt arbitrary code. Processing remains serial; the configured prefetch is a broker buffer, not concurrency. Do not add concurrent handlers solely to make prefetch look effective; introduce concurrency only with an explicit ordering, acknowledgement, and shutdown design.

The webhook payload schema is an explicit open decision. The webhook worker validates only framing, the envelope, event type, and version; it does not interpret destination, signing, or retry fields.

See [`contracts/events/README.md`](../contracts/events/README.md).

## HTTP routing and prefix stripping

Nginx exposes `/api/v1/identity/`, `/api/v1/parcels/`, `/api/v1/pricing/`, `/api/v1/merchants/`, and `/api/v1/realtime/`. It strips the `/api/v1/<service>/` prefix with a rewrite before forwarding. Gin therefore registers root paths:

- `GET /healthz`
- `GET /readyz`

For example, `/api/v1/identity/healthz` becomes `/healthz` at the identity service. This keeps service routers independent of the edge topology. Any new route must be updated in the Gin router, the OpenAPI specification, and the Nginx mapping together.

All HTTP services share the same edge-facing behavior:

- Unmatched paths return a JSON `404` with code `not_found`.
- Disallowed methods on known paths return a JSON `405` with code `method_not_allowed` and an `Allow` header.
- Panics are recovered by the shared middleware. If no response has been written, the client receives a JSON `500` with code `internal_error`; the stack trace is logged server-side and never returned.
- Health and readiness probe requests are logged at debug level rather than as normal traffic.
- Servers set a 1 MiB header limit by default and route `http.Server.ErrorLog` through the structured logger.

The Next.js application serves `/` and its own routes, including `/api/hello`. Only the `/api/v1/` namespace is proxied to Go services.

## Configuration

Configuration is environment-based. Values are matched case-insensitively and trimmed by the shared loader. Invalid required values fail startup clearly. `RABBITMQ_URL` must use the `amqp` or `amqps` scheme and include a host; validation errors never include the URL. The root `.env.example` documents local development values. Docker Compose passes container-network addresses such as `postgres` and `rabbitmq`; `localhost` is used only for host-to-container access.

HTTP services read:

- `APP_ENV`
- `LOG_LEVEL`
- `PORT`
- optional `DATABASE_URL` for future persistence

Workers read:

- `APP_ENV`, `LOG_LEVEL`
- required `RABBITMQ_URL`
- `RABBITMQ_EXCHANGE`, queue, routing key, prefetch, handler timeout, retry limit, retry delay, and reconnect bounds

The maximum RabbitMQ message body size is 1 MiB and is fixed in `messaging.MaxMessageBytes`; it is not configurable in this phase. Secrets and connection strings are never logged.

## Health and readiness

Every HTTP service exposes:

- `GET /healthz` — the process is alive.
- `GET /readyz` — the process is ready to accept traffic.

Readiness currently performs **no PostgreSQL or RabbitMQ dependency check**. It must not be interpreted as proof that downstream infrastructure is available. When a service starts depending on PostgreSQL, its readiness check should use a bounded-context ping and report failure accurately.

Workers do not expose public HTTP endpoints. Their lifecycle is log-driven: they connect, declare the durable main and failure-routing topology, consume with explicit acknowledgements, retry retryable failures with a bounded delay, dead-letter permanent or exhausted failures, and reconnect with capped exponential backoff and jitter. In-flight messages are requeued on shutdown, and a second termination signal forces exit if graceful shutdown does not finish.

## Persistence and migrations

PostgreSQL is the primary relational store. SQL migrations live under `migrations/<service>/` and are applied with `golang-migrate`. Each service uses a distinct migration version table in the shared local database. Services never run migrations on startup.

The initial migrations create schemas only. Business tables are deferred until their workflows exist.

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
  go test -tags=integration ./tests/integration/...
```

CI runs the checks in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml): Go formatting, vet, race tests, build, and Compose config validation; a separate RabbitMQ integration job that fails on skipped tests; frontend lint, typecheck, and build; Chromium-only Playwright E2E; and representative `identity` and web Docker image builds. See the [README](../README.md#ci) for the job summary and [`CONTRIBUTING.md`](../CONTRIBUTING.md) for the contribution workflow.

## Deliberate omissions

The foundation does not include authentication, JWT, parcel workflows, pricing formulas, merchant onboarding, API key management, notification delivery, webhook signing or webhook delivery retries, realtime subscriptions, payments, production TLS, Kubernetes, Terraform, or deployment automation.
