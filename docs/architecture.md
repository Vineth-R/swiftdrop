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
- **Merchant** owns merchant accounts, integration settings, and future API credentials.
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

Use events when work can happen independently and be retried, such as notification delivery or webhook dispatch. Producers publish versioned events to the durable topic exchange `swiftdrop.events`. Consumers declare durable queues bound to versioned routing keys.

Consumers are at-least-once: they must deduplicate by `event_id` and be idempotent. A message is acknowledged only after successful handling. Failed messages are rejected without requeue in the initial foundation; retry policy and dead-letter queues will be added with the business workflows.

See [`contracts/events/README.md`](../contracts/events/README.md).

## HTTP routing and prefix stripping

Nginx exposes `/api/v1/identity/`, `/api/v1/parcels/`, `/api/v1/pricing/`, `/api/v1/merchants/`, and `/api/v1/realtime/`. It strips the `/api/v1/<service>/` prefix with a rewrite before forwarding. Gin therefore registers root paths:

- `GET /healthz`
- `GET /readyz`

For example, `/api/v1/identity/healthz` becomes `/healthz` at the identity service. This keeps service routers independent of the edge topology. Any new route must be updated in the Gin router, the OpenAPI specification, and the Nginx mapping together.

The Next.js application serves `/` and its own routes, including `/api/hello`. Only the `/api/v1/` namespace is proxied to Go services.

## Configuration

Configuration is environment-based. Invalid required values fail startup clearly. The root `.env.example` documents local development values. Docker Compose passes container-network addresses such as `postgres` and `rabbitmq`; `localhost` is used only for host-to-container access.

HTTP services read:

- `APP_ENV`
- `LOG_LEVEL`
- `PORT`
- optional `DATABASE_URL` for future persistence

Workers read:

- `APP_ENV`, `LOG_LEVEL`
- required `RABBITMQ_URL`
- `RABBITMQ_EXCHANGE`, queue, routing key, prefetch, handler timeout, and reconnect bounds

Secrets and connection strings are never logged.

## Health and readiness

Every HTTP service exposes:

- `GET /healthz` — the process is alive.
- `GET /readyz` — the process is ready to accept traffic.

Readiness currently performs **no PostgreSQL or RabbitMQ dependency check**. It must not be interpreted as proof that downstream infrastructure is available. When a service starts depending on PostgreSQL, its readiness check should use a bounded-context ping and report failure accurately.

Workers do not expose public HTTP endpoints. Their lifecycle is log-driven: they connect, declare durable queue topology, consume with explicit acknowledgements, reconnect with capped backoff, and stop on process signals.

## Persistence and migrations

PostgreSQL is the primary relational store. SQL migrations live under `migrations/<service>/` and are applied with `golang-migrate`. Each service uses a distinct migration version table in the shared local database. Services never run migrations on startup.

The initial migrations create schemas only. Business tables are deferred until their workflows exist.

## Shared Go code

`packages/go-common` contains infrastructure only:

- environment loading and validation
- structured logging
- request IDs and HTTP response helpers
- HTTP server lifecycle
- RabbitMQ event envelope and consumer lifecycle

It contains no business entities, shared repositories, or cross-service business rules. All Go code belongs to the single root module `github.com/Vineth-R/swiftdrop`.

## Deliberate omissions

The foundation does not include authentication, JWT, parcel workflows, pricing formulas, merchant onboarding, API key management, notification delivery, webhook signing or retry scheduling, realtime subscriptions, payments, production TLS, Kubernetes, Terraform, or deployment automation.
