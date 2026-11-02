<!-- nx configuration start-->
<!-- Leave the start & end comments to automatically receive updates. -->

# General Guidelines for working with Nx

- For navigating/exploring the workspace, invoke the `nx-workspace` skill first - it has patterns for querying projects, targets, and dependencies
- When running tasks (for example build, lint, test, e2e, etc.), always prefer running the task through `nx` (i.e. `nx run`, `nx run-many`, `nx affected`) instead of using the underlying tooling directly
- Prefix nx commands with the workspace's package manager (e.g., `pnpm nx build`, `npm exec nx test`) - avoids using globally installed CLI
- You have access to the Nx MCP server and its tools, use them to help the user
- For Nx plugin best practices, check `node_modules/@nx/<plugin>/PLUGIN.md`. Not all plugins have this file - proceed without it if unavailable.
- NEVER guess CLI flags - always check nx_docs or `--help` first when unsure

## Scaffolding & Generators

- For scaffolding tasks (creating apps, libs, project structure, setup), ALWAYS invoke the `nx-generate` skill FIRST before exploring or calling MCP tools

## When to use nx_docs

- USE for: advanced config options, unfamiliar flags, migration guides, plugin configuration, edge cases
- DON'T USE for: basic generator syntax (`nx g @nx/react:app`), standard commands, things you already know
- The `nx-generate` skill handles generator discovery internally - don't call nx_docs just to look up generator syntax

<!-- nx configuration end-->

# SwiftDrop Agent Guide

SwiftDrop is a parcel delivery and courier management platform. The repository is an Nx monorepo with a Next.js frontend, independently buildable Go services, a shared Go infrastructure package, PostgreSQL migrations, RabbitMQ event contracts, local Docker Compose infrastructure, and Nginx routing.

This file is the project-specific source of guidance for AI agents. The Nx block above is maintained by Nx and must stay intact. The code and configuration in this repository take precedence over README summaries, generic skill advice, and assumptions from other projects.

## Current phase: foundation, not business functionality

The repository is intentionally a production-oriented foundation. Do not describe or implement planned features as if they already exist.

| Area           | Current state                                                                                                                                                                                                                                                                                                                                       |
| -------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Frontend       | Next.js App Router starter page and `/api/hello` example route. No SwiftDrop business UI.                                                                                                                                                                                                                                                           |
| Frontend E2E   | Playwright covers the starter page, the `/design` token page, and the auth UI kit.                                                                                                                                                                                                                                                                  |
| HTTP services  | Gin servers with configuration, structured logging, request IDs, health and readiness endpoints, timeouts, graceful shutdown, structured panic recovery, JSON 404/405 responses, a 1 MiB header limit, and debug-level probe logs. Identity auth and own-profile endpoints only.                                                                    |
| Workers        | RabbitMQ consumers that validate message framing and the shared event envelope, log non-sensitive identifiers, reconnect with jittered backoff, force exit on a second termination signal, and route failures through bounded retry or dead-letter handling. The notification worker delivers email over SMTP; webhook delivery is not implemented. |
| Shared Go      | `packages/go-common` provides config and credential-safe AMQP URL validation, logging, HTTP helpers, request IDs, HTTP lifecycle, event envelopes, message framing validation, failure classification, publisher-confirmed retry/dead-letter routing, and a RabbitMQ consumer.                                                                      |
| PostgreSQL     | Compose runs PostgreSQL 16 with Mailpit for local email. Migrations create the five service schemas and their baseline tables from `docs/db.sql`. HTTP services open a shared pgx pool and expose `/health` and `/ready`; `/ready` reports database connectivity.                                                                                   |
| RabbitMQ       | Compose runs RabbitMQ 4.1 management. Workers declare the durable main exchange, queue, and binding plus retry and dead-letter exchanges and queues. No event producers or outbox implementation yet.                                                                                                                                               |
| Contracts      | OpenAPI specs cover health and readiness plus shared 404/405/500 error responses. One example event schema exists for `notification.requested.v1`; the webhook payload schema is an open decision.                                                                                                                                                  |
| Realtime       | Health endpoints and Nginx WebSocket/SSE settings exist. No WebSocket or SSE handler or subscription engine exists.                                                                                                                                                                                                                                 |
| Tests          | Go unit tests exist, including the parcel dispatch state machine; eight broker-backed messaging scenarios and isolated PostgreSQL dispatch migration tests live under `tests/integration`. Playwright covers the starter page, the `/design` token page, and the auth UI kit.                                                                       |
| Infrastructure | Docker Compose, Go and web Dockerfiles, Nginx, migrations, event contracts, and CI exist. No Kubernetes, Terraform, cloud deployment, or production TLS.                                                                                                                                                                                            |

Planned responsibilities:

- `identity`: email and password authentication (register, login, refresh, logout), Google login, password reset, admin users, the courier list, the internal courier-eligibility and user-contact lookups, and the signed-in user's own profile (`GET /users/me`) are implemented; profile updates and password changes are planned.
- `parcel`: future parcel lifecycle, status, and tracking.
- `pricing`: future delivery fee calculation and pricing configuration.
- `merchant`: future merchant accounts, integration settings, and API credentials.
- `notification-worker`: consumes `notification.requested.v1` and delivers email over SMTP; SMS is a permanent failure, and delivery status is not persisted.
- `webhook-worker`: future outgoing webhook delivery and retries.
- `realtime`: future live parcel tracking over WebSocket or Server-Sent Events.

## Implemented stack

- Go `1.27.3`, one root module: `github.com/Vineth-R/swiftdrop`.
- Gin `v1.12.0` for HTTP services.
- RabbitMQ client `github.com/rabbitmq/amqp091-go v1.16.0`.
- Next.js 16 App Router, React 19, TypeScript 6, and Tailwind CSS v4 with the SwiftDrop design tokens.
- Nx `24.0.0` with pnpm `12.15.0`; CI uses Node.js 24.
- PostgreSQL 16, RabbitMQ 4.1 management, Nginx 1.27, Alpine 3.22 runtime.
- GitHub Actions CI in `.github/workflows/ci.yml`.

Do not add gRPC, Protocol Buffers, Kafka, Turborepo, Kubernetes, Terraform, or a new frontend framework without explicit approval.

## Repository map

| Path                                | Purpose                                                                                                                                                                                                          |
| ----------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps/web`                          | Next.js App Router frontend. Currently the design-token foundation, `/design` token page, and auth UI kit; no SwiftDrop business screens.                                                                        |
| `apps/web-e2e`                      | Playwright end-to-end tests covering the starter page, `/design` token page, and auth UI kit.                                                                                                                    |
| `apps/services/identity`            | Identity HTTP service with authentication, admin users, couriers, internal lookups, and `GET /users/me`. Profile updates and password changes are planned.                                                       |
| `apps/services/parcel`              | Parcel HTTP service skeleton. Future parcels and tracking.                                                                                                                                                       |
| `apps/services/pricing`             | Pricing HTTP service skeleton. Future fee calculation.                                                                                                                                                           |
| `apps/services/merchant`            | Merchant HTTP service skeleton. Future accounts and credentials.                                                                                                                                                 |
| `apps/services/realtime`            | Realtime HTTP service skeleton. Future WebSocket/SSE tracking.                                                                                                                                                   |
| `apps/services/notification-worker` | RabbitMQ notification worker with SMTP email delivery and stateless delivery logging.                                                                                                                            |
| `apps/services/webhook-worker`      | RabbitMQ webhook worker skeleton.                                                                                                                                                                                |
| `packages/go-common`                | Shared infrastructure only: config and AMQP URL validation, logging, HTTP helpers, request IDs, server lifecycle, messaging envelope and framing validation, retry/dead-letter routing, and a RabbitMQ consumer. |
| `contracts/openapi`                 | Hand-maintained OpenAPI 3.0.3 specs. Currently health, readiness, and shared error responses.                                                                                                                    |
| `contracts/events`                  | Event naming, envelope, framing, versioning conventions, and one JSON Schema example.                                                                                                                            |
| `migrations/<service>`              | golang-migrate SQL migrations. `000001` creates the schema; `000002_tables` is reserved for the platform baseline tables. Owners add new migrations from `000003`.                                               |
| `infra/docker`                      | Parameterized Go Dockerfile and Next.js standalone Dockerfile.                                                                                                                                                   |
| `infra/nginx`                       | Local reverse proxy configuration and routing notes.                                                                                                                                                             |
| `tests/integration`                 | Broker-backed RabbitMQ tests covering acknowledgement, retries, dead-lettering, and panic recovery, plus isolated PostgreSQL dispatch migration tests in `tests/integration/database`.                           |
| `docs`                              | Architecture documentation.                                                                                                                                                                                      |
| `.github/workflows`                 | CI workflows.                                                                                                                                                                                                    |
| `.agents/skills`                    | Third-party project skills pinned by `skills-lock.json`, plus local Nx skills.                                                                                                                                   |
| `.opencode/skills`                  | Pre-existing local Nx skills used by OpenCode.                                                                                                                                                                   |

`packages/.gitkeep` is only a placeholder. Do not add a shared JavaScript package unless the task requires one.

## Architecture and service boundaries

### Monorepo

- Nx orchestrates JavaScript projects and Go project targets.
- pnpm manages JavaScript dependencies and the lockfile.
- Go services are independently buildable and deployable but share one root `go.mod`.
- Never create a nested `go.mod` or a `go.work` file without explicit approval.
- Do not replace Nx with another monorepo tool.
- Do not move Go services into a new `cmd/` or `pkg/` layout without an approved architectural change.

### Ownership

- Each service owns its business logic, data access, and database schema.
- A service may import its own `internal/` packages and `packages/go-common`.
- A service must not import another service's `internal/` packages.
- A service must not read or write another service's database tables or schema.
- Cross-service dependencies must use explicit HTTP APIs or asynchronous events.
- Keep shared packages small. Do not put business entities, repositories, or cross-service rules in `packages/go-common`.

### Communication

- Use HTTP/REST with JSON for synchronous communication and immediate responses.
- Use Gin for Go HTTP APIs, matching the existing implementation.
- Use RabbitMQ for asynchronous events and background jobs.
- Do not introduce gRPC or Protocol Buffers.
- Do not introduce Kafka or another message broker without explicit approval.
- Public browser-facing traffic goes through Nginx. Do not expose internal service ports or internal-only endpoints to browsers.
- Internal service-to-service HTTP endpoints must be explicit, authenticated where required, documented, and kept separate from public routes.

### Data

- PostgreSQL is the persistence choice where configured.
- Local development uses one PostgreSQL instance with one schema per service: `identity`, `parcel`, `pricing`, `merchant`, and `notification`. Realtime has no tables and is stateless.
- Production may use separate databases later, but service ownership rules still apply.
- Use transactions for local consistency.
- Avoid distributed transactions. When database changes and event publication must remain consistent, design an outbox or equivalent explicit mechanism before implementing it.

## Go standards

### Layout

- `main.go` handles startup, configuration loading, logging setup, router construction, server timeouts, signals, and graceful shutdown. Keep business logic out of `main.go`.
- Service-specific code belongs under `apps/services/<service>/internal/`.
- HTTP services currently use `internal/config` and `internal/http/router.go`.
- Workers currently use `internal/config` and `internal/worker`.
- Add handlers, services, and repositories only when a real responsibility justifies them. Do not create empty layers or identical directory trees for every service.
- `packages/go-common` must remain infrastructure only.

### Code

- Follow idiomatic Go and `gofmt`.
- Pass `context.Context` through blocking, cancellable, or I/O-bound operations. Use `c.Request.Context()` in Gin handlers.
- Handle errors explicitly and wrap them with useful context. Do not ignore errors or use panics for normal control flow.
- Avoid global mutable state. Prefer constructor injection and explicit dependencies.
- Keep interfaces small and consumer-defined where practical. Do not add interfaces solely for mocking without a real seam.
- Use `log/slog` through `packages/go-common/logging`. Do not use `fmt.Println` or `log.Println` for service logging.
- Preserve HTTP server timeouts and graceful shutdown. Do not start naked `http.ListenAndServe` calls in new services.
- Do not add dependencies when the standard library or existing dependencies are sufficient.
- If a dependency is added, update `go.mod` and `go.sum`, explain why, and test the affected behavior.
- Do not add a shared repository layer or shared business types to `go-common`.

### Configuration

- HTTP services read `APP_ENV`, `LOG_LEVEL`, `PORT`, and optional `DATABASE_URL`. They open a shared pgx pool when it is set and report connectivity through `/ready`.
- Workers require `RABBITMQ_URL` and read `RABBITMQ_EXCHANGE`, queue/routing-key variables, `RABBITMQ_PREFETCH`, `RABBITMQ_HANDLER_TIMEOUT`, `RABBITMQ_MAX_RETRIES`, `RABBITMQ_RETRY_DELAY`, `RABBITMQ_RECONNECT_MIN`, and `RABBITMQ_RECONNECT_MAX`.
- `RABBITMQ_URL` must use the `amqp` or `amqps` scheme and include a host. Validation errors must not include the URL. Use the shared config loader for this check.
- Environment values are matched case-insensitively and trimmed by the shared loader. Keep new parsing consistent with it.
- The maximum accepted RabbitMQ message body is `messaging.MaxMessageBytes` (1 MiB) and is fixed in code. Do not add an environment variable for it without an explicit operational decision.
- Use the existing config loader for parsing and validation.
- Fail startup clearly on invalid or missing required configuration.
- Do not log connection strings, passwords, tokens, or other secrets.
- Go services read process environment variables directly; they do not load `.env`. Docker Compose reads `.env` automatically.
- `.env` is untracked. `.env.example` is the documented local template and must stay free of real credentials.

## HTTP API standards

- Use resource-oriented routes and explicit HTTP methods.
- Public routes follow API v1.1 path groups. Nginx strips the `/api/v1` prefix before forwarding, so Gin routers register root paths such as `/auth/...`, `/parcels/...`, `/health`, and `/ready`.
- Current public mapping:
  - `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers` -> identity on `8081`
  - `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/admin/pricing`, `/api/v1/admin/zones` -> pricing on `8083`
  - `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports` -> parcel on `8082`
  - `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants` -> merchant on `8084`
  - `/ws` -> realtime on `8085`
  - `/healthz` -> gateway only; `/internal` is never routed
  - `/` -> Next.js web on `3000`
- Do not add a second prefix inside a Gin router unless the routing architecture is deliberately changed.
- When adding or changing a public route, update the Gin route, `contracts/openapi/<service>.yaml`, and `infra/nginx/default.conf` together.
- Validate request bodies, query parameters, and path parameters. Reject malformed input with an appropriate 4xx response.
- Use appropriate status codes: 200/201/204 for success, 400 for malformed input, 401/403 for auth failures, 404 for missing resources, 409 for conflicts, 422 for semantic validation failures, and 500 for unexpected server errors.
- Use the shared response helpers. The error shape follows API v1.1:
  ```json
  {
    "error": { "code": "NOT_FOUND", "message": "route not found" },
    "meta": { "requestId": "..." }
  }
  ```
- Health returns `{"status":"ok"}`. Readiness returns `{"status":"ready","checks":{"database":"ok"}}` or 503 with the failing check set to `down`. `/health` and `/ready` are primary; `/healthz` and `/readyz` remain deprecated aliases.
- The shared router returns JSON for unmatched paths (`404`, code `NOT_FOUND`) and disallowed methods (`405`, code `METHOD_NOT_ALLOWED`, with an `Allow` header). Do not add a separate ad hoc error shape for these cases.
- Panics are recovered by the shared middleware and converted to a JSON `500` (`INTERNAL_ERROR`) when no response has been written. The recovery log includes request ID, method, path, panic value, and stack trace; clients never receive the stack trace or internal details.
- Probe requests to `/health`, `/ready`, `/healthz`, and `/readyz` are logged at debug level. Do not log them as normal traffic.
- `httpx.NewServer` sets a 1 MiB header limit by default and routes `http.Server.ErrorLog` through `slog`. Preserve both unless an approved change requires otherwise.
- Do not expose stack traces, SQL errors, credentials, tokens, or internal implementation details to clients. Log safe details server-side.
- Propagate request cancellation and timeouts to downstream calls.
- Preserve backward compatibility unless an API change explicitly requires otherwise. Document breaking changes.
- Do not invent endpoint names, response schemas, or auth behavior for functionality that has not been designed.

## Security and authentication

Email and password authentication is implemented in the identity service, and `packages/go-common/auth` provides access-token verification and role middleware for every service. Add authorization checks to each endpoint as it lands. For all authentication and authorization work:

- Never store plaintext passwords. Use a current, vetted password-hashing algorithm with an appropriate work factor.
- Validate JWT signatures, allowed algorithms, expiry, issuer, and audience where applicable. Do not hand-roll JWT validation.
- Enforce authorization on the backend. Client-side checks are not authorization.
- Never trust user IDs, roles, prices, permissions, or ownership claims supplied by a client without authoritative validation.
- Keep access tokens, refresh tokens, passwords, and sensitive personal data out of logs and error messages.
- Keep secrets out of source control and Docker images. Use environment variables or an approved secret-management mechanism.
- Validate all external input and handle errors safely.
- Review dependency vulnerabilities when adding or changing dependencies.
- Apply least privilege. Protect internal service endpoints from unauthorized access.
- Do not run destructive database operations without explicit authorization.
- Security-sensitive changes must include tests for the relevant failure and authorization paths.
- Keep local development on plain HTTP. Do not add invented TLS certificates. Production TLS termination is out of scope for this phase.

## RabbitMQ and background workers

- The durable topic exchange is `swiftdrop.events`.
- The current workers declare durable queues:
  - `notification.requested.v1`
  - `webhook.delivery.requested.v1`
- Event names and routing keys are versioned. Use `<domain>.<past_tense_action>.v<major>`, for example `parcel.created.v1`.
- Reuse the shared event envelope in `packages/go-common/messaging/envelope.go`. The envelope includes `event_id`, `event_type`, `event_version`, `occurred_at`, `producer`, optional `correlation_id`, and `data`.
- Envelope validation is permanent-failure validation: `event_version` must match the version suffix of `event_type`, and `data` must be a JSON object. Do not accept a version mismatch, a non-canonical suffix, or non-object `data`.
- Document event schemas under `contracts/events` using JSON Schema draft 2020-12.
- The webhook payload schema is an open decision. Do not invent webhook destination, signing, or retry fields until a schema is added under `contracts/events/`.
- The shared consumer validates message framing before the handler runs: the body must be at most 1 MiB and the content type must be absent or `application/json`. Invalid framing is permanent and goes directly to the dead-letter queue.
- The notification worker validates the envelope, event type/version, and the payload fields it uses, then delivers email through the shared SMTP mailer. The webhook worker validates and logs only. The shared consumer retries retryable failures and dead-letters permanent or exhausted failures. Workers do not write to a database.
- Producers are not implemented yet. When adding one, publish to the versioned routing key and validate the payload against its contract.
- Acknowledge a message only after processing succeeds or after a publisher-confirmed retry or dead-letter publication. Never acknowledge a failed message.
- Classify failures as permanent or retryable. Malformed JSON, invalid envelopes, unsupported event types or versions, invalid event data, unsupported content types, and oversize bodies are permanent. Unclassified handler errors are retryable.
- Retry at most `RABBITMQ_MAX_RETRIES` times (default `3`) with `RABBITMQ_RETRY_DELAY` (default `30s`), then dead-letter. Do not requeue failures indefinitely.
- Retry and dead-letter resources are derived from the main exchange and queue: `<exchange>.retry`, `<queue>.retry`, `<exchange>.dlx`, `<queue>.dlq`. Do not change the arguments of existing main queues. Retry messages return to the original exchange and routing key; a worker routing key must not fan out to multiple worker queues. The dead-letter queue has no consumer.
- Reconnect with capped exponential backoff and jitter between `RABBITMQ_RECONNECT_MIN` and `RABBITMQ_RECONNECT_MAX`. Reset backoff after a session reaches the consume phase. Reconnect waits and dials must honor context cancellation so shutdown is prompt.
- Handler deadlines are cooperative: handlers must honor their context because Go cannot preempt arbitrary code. Keep the serial consumer unless a concrete throughput requirement justifies concurrency; prefetch is a buffer, not concurrency.
- Design consumers for at-least-once delivery. Duplicate or redelivered messages are normal after reconnects and retries. Deduplicate by `event_id` and make side effects idempotent before adding state-changing handlers.
- Worker shutdown cancels the consumer and requeues in-flight messages. A second termination signal forces exit if graceful shutdown does not finish; preserve this behavior.
- Keep message handling independent of HTTP request lifecycles.
- Make failures observable through structured logs and, when added, metrics.
- Consider a transactional outbox when database changes and event publication must be consistent.
- Do not require RabbitMQ for ordinary unit tests. Reserve broker-backed tests for `tests/integration` with an explicit build tag.
- Workers do not expose public HTTP ports. Do not add an HTTP API solely for parity with other services.

## PostgreSQL and migrations

- Use PostgreSQL for persistence where configured.
- Use one local database with separate schemas per owning service: `identity`, `parcel`, `pricing`, `merchant`, and `notification`. Realtime is stateless.
- Services own their schema. Do not read or modify another service's schema.
- Use versioned migrations under `migrations/<service>/` with the `NNNNNN_description.up.sql` and `.down.sql` naming convention.
- Use golang-migrate as documented in `migrations/README.md`. Each service uses its own migration version table.
- `000001` creates the service schema. `000002_tables` is reserved for the platform baseline tables from `docs/db.sql`. Other owners add new migrations from `000003`; do not renumber or reuse an applied version.
- Parcel `000003_dispatch_foundations` extends the baseline `parcel_assignments` and `outbox_events` tables. It adds reassignment metadata, an assignment `updated_at` trigger, and outbox aggregate metadata without recreating tables.
- Applied migrations are immutable. Fix mistakes with a new migration.
- Do not run migrations automatically at service startup.
- Do not modify schemas manually without a migration.
- Production changes should be forward-only and use expand/contract patterns where needed. Down migrations remain useful for local development and review.
- Down migrations that drop schemas or tables are destructive. Confirm the target database before running them.
- HTTP services open a shared pgx pool when `DATABASE_URL` is set and report connectivity through `/ready`. Missing `DATABASE_URL` leaves the service running with `/ready` returning 503.
- Use local transactions where needed. Avoid distributed transactions.
- Baseline business tables come from `docs/db.sql`. Do not add speculative tables beyond it without a task requirement.

## Frontend standards

- Preserve the Next.js App Router structure in `apps/web/src/app`.
- Use TypeScript and preserve strict type checking. Do not loosen compiler strictness to bypass errors.
- Prefer React Server Components by default. Add `'use client'` only when browser APIs, event handlers, or client-side state require it.
- Keep API access consistent. Browser code should call public `/api/v1/...` routes through the edge, not internal service ports.
- Do not duplicate backend business logic in the frontend. The backend remains authoritative for validation, authorization, pricing, and state transitions.
- Validate user-facing forms and handle loading, error, empty, and success states.
- Follow accessibility practices and responsive design. The existing ESLint setup includes `jsx-a11y`.
- Style with Tailwind v4 utilities against the design tokens in `apps/web/src/app/globals.css`. Use tokens only: no raw hex values, off-scale spacing or second icon family (Lucide only). `/design` shows every token in light and dark. Do not introduce a component library or state-management library without approval.
- Avoid unnecessary client-side JavaScript and dependencies.
- The existing `/api/hello` route is an example. Do not confuse it with the Go `/api/v1/` namespace.
- `apps/web` currently contains the design-token foundation and the `/design` token page, not SwiftDrop business screens.
- `apps/web-e2e` currently covers the starter page, `/design` token page, and auth UI kit. Extend it for meaningful user journeys when those journeys exist.

## Testing requirements

Add or update tests for every meaningful change.

### Go

- Unit tests for business logic when it is introduced.
- HTTP handler and route tests.
- Configuration and validation tests.
- Repository tests when a database layer is added.
- Integration tests for PostgreSQL and broker-backed RabbitMQ tests under `tests/integration`. PostgreSQL tests create and drop uniquely named databases and require `TEST_DATABASE_URL`.
- Tests for validation, authorization, and failure paths.
- Existing tests use the Go standard library testing package. Follow that style unless there is a clear reason to add another tool.
- Unit tests must not require PostgreSQL or RabbitMQ. Use `//go:build integration` for infrastructure-backed tests under `tests/integration`.
- Broker-backed messaging tests cover acknowledgement, retry-then-success, permanent dead-lettering, retry exhaustion, and panic recovery. Coverage that must not depend on a live broker stays in `packages/go-common/messaging`.
- Use `go test -race ./...` for concurrency-sensitive changes when feasible.
- CI sets `RABBITMQ_URL` for the integration job and fails when any integration test is skipped. Do not rely on silent skips.

### Frontend

- Test meaningful component and application behavior when the UI is implemented.
- Use the existing Nx Playwright target for important end-to-end workflows.
- Do not add a new test framework without approval.

### Distributed behavior

When messaging and persistence are implemented, test:

- Duplicate message delivery and idempotency.
- Timeouts and downstream failures.
- Retry and dead-letter paths.
- Consistency between database changes and published events.

Never claim tests passed unless they were actually run and passed. Report any check that could not run and why.

## Nx, pnpm, and build standards

- Use pnpm only for JavaScript dependencies. Do not use npm or Yarn.
- Do not modify dependency versions or lockfiles unless the task requires it.
- Use Nx for configured tasks. Inspect available targets before invoking them.
- `@org/web` targets: `build`, `dev`, `start`, `serve-static`, `build-deps`, `watch-deps`, `lint`. It has no `test` target and no `typecheck` target; the Next build performs type checking.
- `@org/web-e2e` targets: `lint`, `typecheck`, `e2e`, and Playwright CI targets. It has no `build` target.
- Go projects have `build`, `test`, `lint`, and `serve` targets. `go-common` has `build`, `test`, and `lint`.
- Run Go targets from the repository root. They depend on the root module and `packages/go-common`.
- Do not remove `go.mod`, `go.sum`, or `packages/go-common` from Nx cache inputs.
- Build output goes to ignored directories such as `dist`, `.next`, `out-tsc`, and `test-output`. Do not commit generated output unless the repository explicitly requires it.
- Useful commands:
  ```sh
  pnpm install --frozen-lockfile
  pnpm exec nx show projects
  pnpm exec nx build identity
  pnpm exec nx test identity
  pnpm exec nx lint identity
  pnpm exec nx serve identity
  pnpm exec nx dev @org/web
  pnpm exec nx build @org/web
  pnpm exec nx lint @org/web
  pnpm exec nx lint @org/web-e2e
  pnpm exec nx typecheck @org/web-e2e
  pnpm exec nx e2e @org/web-e2e
  pnpm exec nx run-many -t lint test build -p identity,parcel,pricing,merchant,notification-worker,webhook-worker,realtime,go-common
  RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' go test -tags=integration ./tests/integration
  TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' go test -tags=integration ./tests/integration/database/...
  ```
- `pnpm exec nx lint @org/web` and `pnpm exec nx build @org/web` are valid. `pnpm exec nx serve @org/web` is not; use `dev` or `start`.
- The CI frontend command is `pnpm exec nx run-many -t lint typecheck build --projects=@org/web,@org/web-e2e`. Nx runs only targets that are configured for each project.
- CI runs Playwright E2E for Chromium only with `pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"`. The Playwright config still defines Firefox and WebKit projects for local use.
- Run `pnpm exec prettier --write` and `pnpm exec prettier --check` on changed documentation and frontend files. Prettier ignores the third-party skill directories.

## Docker and infrastructure

- Keep the existing `docker-compose.yml` filename. Do not rename it to `compose.yaml` solely because a general skill prefers that name.
- Compose services:
  - `postgres` on 5432, persistent volume, `pg_isready` health check, schema/function init script.
  - `rabbitmq` on 5672 and management on 15672, persistent volume, diagnostics health check.
  - `mailpit` on 1025 (SMTP) and 8025 (UI), health check.
  - `identity` 8081, `parcel` 8082, `pricing` 8083, `merchant` 8084, `realtime` 8085.
  - `notification-worker` and `webhook-worker`, no public ports.
  - `web` on 3000.
  - `nginx` on 8080.
  - `migrate` tools-profile service for golang-migrate.
- Only the gateway, RabbitMQ (5672/15672), and Mailpit (1025/8025) publish host ports. Service, worker, web, and Postgres ports are internal to the Compose network. Do not carry any of this exposure into production architecture without review.
- The Go Dockerfile builds from the repository root context and accepts a `SERVICE` build argument. It uses a multi-stage build, a static binary, a minimal Alpine runtime, and a non-root user.
- The web Dockerfile uses Node 24, pnpm, Nx build, Next standalone output, and a non-root user.
- Do not bake secrets into images. `.dockerignore` excludes `.env` files and agent/tooling directories.
- Use Compose service names for container-to-container addresses. Do not use `localhost` for another container.
- Compose starts Nginx only after its web and HTTP service dependencies report healthy. Workers depend on a healthy RabbitMQ and use a 35-second stop grace period so in-flight messages can be requeued.
- Nginx resolves upstreams at startup. Work on a single service through its published port, or start the full Compose stack. Starting Nginx alone with missing upstreams is not supported.
- Preserve Nginx prefix stripping, request ID forwarding, security headers, timeouts, and realtime upgrade settings.
- Do not add production TLS certificates. Production TLS termination is a future deployment concern.
- Validate infrastructure changes with `docker compose config`, `nginx -t` when applicable, and health checks.
- Do not run `docker compose down -v` or remove volumes without explicit user authorization.
- Do not assume Kubernetes, Terraform, cloud deployment, or managed observability exists.

## Git and change management

- Inspect `git status` before editing.
- The current integration branch is `development`. Create feature branches from it and open pull requests into `development`; do not assume branch protection or required reviews exist until the team establishes a permanent mainline.
- Preserve existing user changes and untracked files.
- Do not reset, clean, rebase, force-push, or discard changes without explicit permission.
- Do not commit or push unless explicitly instructed.
- Do not modify unrelated files.
- Keep changes focused and explain significant architectural trade-offs.
- Review the final diff before reporting completion.
- Run `git diff --check` before finishing.
- Never claim a workflow or command passed without checking its actual result.

## Documentation

- Update documentation when architecture, APIs, events, environment variables, migrations, or operational procedures change.
- Keep README, `docs/architecture.md`, `contracts/openapi`, `contracts/events`, `migrations/README.md`, `.env.example`, and this file consistent with the implementation.
- Prefer editing the relevant existing section over appending duplicate guidance.
- Keep project-wide agent rules in this file. Do not create competing root instruction files that duplicate this guidance.
- Document environment variables in `.env.example`.
- Mark planned features as planned. Do not describe future business behavior as implemented.
- The CI workflow file is the source of truth for CI. It runs Go formatting, vet, race tests with a coverage artifact, build, Compose config validation, and a worker-healthcheck guard; a separate RabbitMQ integration job that fails on skipped tests; frontend lint/build and E2E typecheck; Chromium-only Playwright E2E; changed-service Docker image builds; and a `v*` tag release job that prepares `.env` from `.env.example`, builds every image, starts the Compose stack with `--wait`, applies migrations with `make migrate`, runs route checks, and stops the stack. Do not repeat stale CI summaries, and do not claim a CI run passed unless its result was observed.

## Agent workflow

1. Understand the task and acceptance criteria.
2. Inspect the relevant code, configuration, and existing tests.
3. Identify affected services, shared packages, contracts, migrations, and infrastructure.
4. For complex changes, propose a concise plan before editing.
5. Implement the smallest complete solution.
6. Add or update tests.
7. Run relevant formatting, linting, tests, and builds.
8. Review the resulting diff.
9. Update documentation and contracts where necessary.
10. Summarize changes, verification evidence, and outstanding issues.

Ask for clarification when requirements are ambiguous in ways that could cause significant architectural changes, data loss, security problems, or incompatible APIs. Do not ask unnecessary questions for routine, reversible implementation details.

## Installed skills and agent configuration

- Third-party project skills are installed under `.agents/skills` and pinned by `skills-lock.json`.
- The pre-existing local Nx skills are also available in `.opencode/skills` and `.agents/skills`, with agent-specific copies under `.cursor/skills` and `.github/skills`.
- Leave those pre-existing Nx copies in place. Do not copy skill files into other agent directories, and do not install additional skills as part of a coding task unless explicitly requested.
- Do not edit or reformat files under `.agents/skills`, `.opencode/skills`, `.cursor/skills`, or `.github/skills`. They are third-party and excluded from Prettier.
- Use the relevant retained skill for the task:
  - Go work: `golang-code-style`, `golang-error-handling`, `golang-testing`, `golang-database`, `golang-security`.
  - Gin work: `golang-gin-api`.
  - Frontend work: `vercel-react-best-practices`, `accessibility`.
  - Interface and design review: `better-interface` routes to the full `better-accessibility`, `better-layout`, `better-writing`, `better-typography`, `better-colors`, and `better-ui` suite; `interface-review` reviews a branch, PR, or uncommitted change.
  - Motion polish: `transitions-polish` for existing animation timing and easing.
  - Architecture and domain: `codebase-design`, `domain-modeling`.
  - Review and debugging: `code-review`, `diagnosing-bugs`.
  - Planning and research: `grilling`, `grill-me`, `grill-with-docs`, `research`.
  - Delivery workflow: `implement`, `prototype`, `tdd`, `to-spec`, `to-tickets`, `triage`, `wayfinder`.
  - Teaching and writing: `teach`, `writing-great-skills`.
  - Email work: `email-best-practices`.
  - Monorepo and packages: the official Nx skills and pnpm project commands.
  - Database: `supabase-postgres-best-practices`; use `migrations/README.md` for golang-migrate specifics. Do not add Drizzle or generic Postgres skills; this repository uses Go and golang-migrate.
  - Infrastructure: `docker-build-strategies`, `docker-compose-patterns`, `reverse-proxy`.
  - Messaging: `rabbitmq-development`; use this file for service boundaries and communication rules.
  - Testing and verification: `playwright-best-practices`, `verification-before-completion`.
- Skills are guidance, not authority. If a skill conflicts with this file, follow this file and note the conflict. In particular:
  - Keep `docker-compose.yml`; do not rename it to `compose.yaml`.
  - Keep the existing `apps/services/<service>` layout; do not restructure into `cmd/` and `pkg/`.
  - Use schema-per-service on the local shared PostgreSQL instance; do not add a database container per service.
  - Keep the existing `swiftdrop.events` topic exchange and durable classic queue topology unless a queue migration is explicitly designed.
  - Keep local HTTP; do not add TLS or gRPC examples from the Nginx or messaging skills.
  - Treat Supabase advice as general PostgreSQL guidance; ignore Supabase hosting, RLS, and product-specific recommendations unless Supabase is explicitly adopted.
  - `code-review` can run its standards review without an issue tracker. Do not run `setup-matt-pocock-skills` or install the tracker workflow without explicit approval.
  - `codebase-design` is analysis vocabulary, not permission for speculative abstractions, unnecessary interfaces, or service restructuring.
  - `domain-modeling` may create `GLOSSARY.md` and `docs/adr/` lazily when domain documentation is actually needed. Follow the existing documentation conventions.
  - `research` may write a Markdown note; place it under `docs/` unless an established convention says otherwise, and cite primary sources.
  - `grilling` is for major, ambiguous design decisions, not routine reversible changes.
  - Keep the full `better-*` suite together. `better-interface` and `interface-review` route to `better-accessibility` and `better-layout`; a partial install leaves dead references.
  - `interface-review` and `writing-great-skills` are user-invoked upstream. Do not auto-run them; invoke them explicitly when requested.
  - `better-writing` governs interface copy. `writing-great-skills` governs authoring agent skills, not product writing.
  - `resolving-merge-conflicts` is pinned to `mattpocock/skills@v1.2.3` because upstream removed it on 2026-09-24. Keep the pinned version rather than substituting an unvetted community fork.
  - `writing-great-skills` is pinned to `mattpocock/skills@v1.1.0` because upstream renamed it to `writing-for-agents` on 2026-07-31. Keep the requested name unless a rename is explicitly approved.
  - Do not rewrite Git history or force-push for secret remediation without explicit approval.
- The Nx block at the top is auto-maintained. Preserve its start and end comments.

## Definition of done

Before reporting a task complete:

- `gofmt` is clean for changed Go code.
- `go vet ./...` passes.
- `go test ./...` passes for the affected scope.
- `go test -race ./...` passes for concurrency-sensitive changes when feasible.
- `go build ./...` passes.
- Relevant Nx targets pass, such as `pnpm exec nx test identity` or an `nx run-many` command for the affected Go projects.
- Frontend changes pass `pnpm exec nx lint @org/web` and `pnpm exec nx build @org/web`; E2E changes pass `pnpm exec nx typecheck @org/web-e2e` and the relevant `pnpm exec nx e2e @org/web-e2e` target.
- Messaging changes run the broker-backed integration tests when RabbitMQ is available and report any coverage that could not run.
- `docker compose config` passes for Compose changes.
- `pnpm exec prettier --check` passes on changed documentation and frontend files.
- API, event, migration, environment, and documentation changes are updated together.
- `git diff --check` passes.
- No secrets, generated build output, unrelated rewrites, or unverified claims are included.

If a check cannot run because a tool, service, credential, or browser is unavailable, say so explicitly and explain the gap.
