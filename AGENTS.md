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

## Current phase: foundation with identity auth, customer email verification, merchant registration/admin approval, M4 pricing/tracking, M5 customer parcel creation, parcel reports and courier reads, notification reads, the AI delivery assistant and the shadcn UI foundation

M5 now implements customer parcel creation, list/detail/history/PDF receipts,
internal merchant creation/summary, and the `/new`, `/parcels` and
`/parcels/{id}` customer screens. Creation stores the real pricing quote, initial
history and PENDING outbox data atomically. See `docs/m5-complete-parcel-flow.md`
for verification of the merged M6 relay and the remaining M8 caller dependency.
Customer registration now sends a single-use email verification link and
unverified customers cannot obtain sessions. Merchant registration creates an
inactive Identity account and a PENDING application; ADMIN approval applies a
lifecycle-bound session-version transition before activation. Merchant and
Identity sessions use the `sv` JWT claim, and suspension/reapproval invalidates
old access tokens. The existing courier, dispatch and other-member
responsibilities below remain separate.

The repository is a production-oriented foundation. Production PostgreSQL and private proof storage use the existing Neon project in AWS US East 2; local Docker PostgreSQL and local filesystem storage remain the development defaults, with an opt-in VersityGW S3-compatible profile. Identity authentication, pricing quotes/administration, public parcel tracking, DISPATCHER/ADMIN parcel reports and the reports screen, the COURIER delivery list/detail read APIs, status updates and private proof uploads (API v1.1 sections 6.1-6.4), the COURIER delivery screens (today list, detail, status updates and proof upload/view), and the ADMIN notification-log read API and its ADMIN UI, and the public Gemini-backed AI Delivery Assistant (guest sessions, role-aware private tools, deterministic extraction guard, Markdown chat rendering) backend, chat widget, gateway route and Compose service are implemented. Merchant webhook settings (API v1.1 sections 8.12-8.15) are implemented with session authentication, owner-scoped access, encrypted secret storage and signed test sends, and the webhook worker's internal webhook read/attempt endpoints (API v1.1 section 13) are implemented with `X-Internal-Token` and `X-Request-Id` enforcement; creating a `MERCHANT` identity user and its merchant record remains an onboarding dependency. The remaining courier workflow end-to-end scenarios and other business features remain planned. The Realtime WebSocket `/ws` endpoint and its `parcel.status_changed.v1` consumer are implemented; SSE, persistence/replay and multi-replica fan-out remain planned. Do not describe planned features as if they already exist.

| Area           | Current state                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| -------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Frontend       | Next.js App Router with the shadcn/ui foundation, customer/merchant registration selection, email verification, public merchant onboarding, ADMIN merchant review and merchant profile/API-key screens, SwiftDrop design tokens, `/design` showcase, auth UI kit, app shell, profile/security pages, the COURIER delivery-history screen (`/history`, backed by the parcel courier list endpoint), the courier Today list, delivery-detail, status-update and proof-upload screens with photo preview, proof-photo view actions on courier and customer detail screens, the customer parcel creation wizard (`/new`) and owned list/detail screens (`/parcels`, `/parcels/{id}`), the DISPATCHER/ADMIN reports screen (`/reports`) with its daily chart/table and outcomes, the ADMIN users list, create-user and edit-user screens, the ADMIN notification-log screen, the public parcel tracking page (`/track`) and the AI Delivery Assistant chat widget. Other SwiftDrop business screens remain planned. |
| Frontend E2E   | Playwright covers the starter page, the `/design` token page, the auth UI kit, the app shell, profile/security, shadcn primitives, the COURIER delivery-history screen, the courier delivery detail status and proof upload/view flows including rate-limit and proof-quota states, the customer parcel wizard/list/detail including its fee-estimate error and rate-limit states, the DISPATCHER/ADMIN reports screen, ADMIN users, ADMIN create user, ADMIN edit user, ADMIN notification logs, public tracking and the mocked AI chat widget.                                                                                                                                                                                                                                                                                                                                                                                                                                                               |
| HTTP services  | Gin servers with configuration, structured logging, request IDs, health and readiness endpoints, timeouts, graceful shutdown, structured panic recovery, JSON 404/405 responses, a 1 MiB header limit, and debug-level probe logs. Identity auth with customer email verification, merchant account lifecycle and session versioning, pricing quotes/admin configuration, public parcel tracking, DISPATCHER/ADMIN parcel reports, COURIER delivery list/detail reads and status updates, customer parcel creation/owned reads/PDF receipts plus internal merchant creation/summary, merchant webhook settings, and the merchant webhook worker's internal webhook read/attempt endpoints are implemented. The AI assistant exposes `POST /ai/chat` with optional JWT identity, Gemini function calling, exactly four read-only tools, embedded approved knowledge and safe fallbacks.                                                                                                                         |
| Workers        | RabbitMQ consumers that validate message framing and the shared event envelope, log non-sensitive identifiers, reconnect with jittered backoff, force exit on a second termination signal, and route failures through bounded retry or dead-letter handling. The notification worker delivers email over SMTP and exposes an internal ADMIN-only notification-log read API on port 8086; webhook delivery is not implemented.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                  |
| Shared Go      | `packages/go-common` provides config and credential-safe AMQP URL validation, logging, HTTP helpers, request IDs, HTTP lifecycle, JWT access-token verification and role middleware, bounded HTTP rate limiting, internal-token middleware, event envelopes, message framing validation, failure classification, publisher-confirmed retry/dead-letter routing, a RabbitMQ consumer, and the provider-neutral `objectstore` package (local filesystem and S3-compatible adapters).                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                             |
| PostgreSQL     | Compose runs PostgreSQL 16 with Mailpit for local email. Migrations create the five service schemas and their baseline tables from `docs/db.sql`. HTTP services open a shared pgx pool and expose `/health` and `/ready`; `/ready` reports database connectivity.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| RabbitMQ       | Compose runs RabbitMQ 4.1 management. Workers declare the durable main exchange, queue, and binding plus retry and dead-letter exchanges and queues. Parcel creation writes PENDING transactional outbox data; the M6 relay publishes committed versioned events.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                              |
| Contracts      | OpenAPI specs cover health/readiness, shared errors, identity, pricing (estimate, zones, internal quote, admin configuration), parcel public tracking, customer/internal parcel creation and reads, DISPATCHER/ADMIN reports, COURIER delivery reads and status updates, the ADMIN notification-log read API, and the AI assistant chat endpoint. Event schemas cover `parcel.created.v1`, `parcel.assigned.v1`, and `parcel.status_changed.v1`; `notification.requested.v1` is legacy, and the public webhook projection is documented but not implemented; the realtime service consumes `parcel.status_changed.v1`.                                                                                                                                                                                                                                                                                                                                                                                         |
| AI Assistant   | `apps/services/ai-assistant` serves `POST /ai/chat` on internal port `8087` through Nginx `/api/v1/ai/chat` to guests and signed-in users. Gemini is the only provider (`google.golang.org/genai` v1.77.0), the model is configurable, and exactly four read-only tools are registered. Guests get an HttpOnly `sd_ai_guest` session bound to their conversation; CUSTOMER, DISPATCHER and ADMIN may use the private parcel tools, including ownership-scoped tracking (Parcel remains authoritative), while MERCHANT, COURIER and guests are denied. The assistant never uses the anonymous public tracking endpoint and never treats a tracking number as an identity credential. A deterministic extraction guard runs before Gemini, privileged lookups are audited, assistant replies render safe Markdown, and conversation/rate/concurrency state is in-process and non-durable. The key never leaves the service.                                                                                      |
| Realtime       | WebSocket `/ws` (API v1.1 section 9) with cookie or `?token=` access-token auth, per-parcel subscriptions, `scope: all` for DISPATCHER/ADMIN, ping/pong, token-expiry and shutdown handling, and an in-process hub. Consumes `parcel.status_changed.v1` on `realtime.parcel.status_changed.v1` and sends public tracking fields only. No SSE, persistence, replay or multi-replica fan-out; each queued event reaches one replica. `/ready` checks PostgreSQL when configured, not the broker consumer. The web `useLiveUpdates` hook exists and is unit-tested, but no business screen consumes it yet.                                                                                                                                                                                                                                                                                                                                                                                                       |
| Tests          | Go unit tests exist, including the parcel dispatch state machine; broker-backed messaging scenarios, isolated PostgreSQL dispatch/pricing/notification-log, parcel-report, customer parcel workflow, courier delivery-read and courier status transactional tests, and pricing HTTP integration tests live under `tests/integration` and the owning services. Playwright covers the starter page, the `/design` token page, the auth UI kit, the app shell, profile/security, shadcn primitives, the COURIER delivery-history screen, the booking wizard's fee-estimate error and rate-limit gating, the reports screen, ADMIN notification logs, public tracking and the AI assistant unit/orchestration/client/tool tests and realtime WebSocket/auth/hub/consumer tests.                                                                                                                                                                                                                                    |
| Infrastructure | Docker Compose, Go and web Dockerfiles, Nginx, migrations, event contracts, and CI exist. No Kubernetes, Terraform, cloud deployment, or production TLS.                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                                       |

Planned responsibilities:

- `identity`: email and password authentication (register, login, refresh, logout), Google login, password reset, admin users, the courier list, the internal courier-eligibility and user-contact lookups, and the signed-in user's own profile (`GET` and `PATCH /users/me`) and password change (`PUT /users/me/password`) are implemented.
- `parcel`: anonymous public tracking reads are implemented from real parcel/status-history rows, and authenticated tracking reads are ownership-scoped by `customer_user_id` (DISPATCHER/ADMIN may read operationally), DISPATCHER/ADMIN daily and outcome reports are served from the same parcel data, and the COURIER delivery list (API v1.1 section 6.1), assignment-scoped detail (section 6.2), transactional status updates (section 6.3) and private proof uploads (section 6.4) are implemented. Status updates lock the parcel, verify the current assignment, and commit the parcel row, history and pending outbox event together. Customer parcel creation, owned reads/history/PDF receipt and internal merchant creation/summary are also implemented. Proof uploads validate decoded JPEG/PNG content, enforce the 5 MiB photo limit, verify the current assignment before upload, re-check assignment and quota under the parcel row lock, and store private objects through `packages/go-common/objectstore`; the remaining lifecycle writes remain future work. Local filesystem storage is the development default, VersityGW is an opt-in local S3-compatible profile, and Neon Object Storage is the production provider. A committed proof row always references an object uploaded before commit; definite database failures delete the object, and `proof-transfer -reconcile` reports orphans and missing objects. `GET /parcels/{parcelId}/proof/photo` returns a short-lived presigned URL (or an authorized local stream) and is additive to API v1.1. The internal merchant route is a trusted-caller boundary: M8 must authenticate the API key/approval and supply the verified owner because Parcel has no Merchant internal lookup yet.
- `pricing`: public estimate and zones, internal quote/zone lookups, and ADMIN pricing configuration with audit logging are implemented.
- `merchant`: merchant profile/admin reads and transitions, API credentials, and the webhook settings endpoints (API v1.1 sections 8.12-8.15): `GET`/`PUT /merchants/me/webhook`, `POST /merchants/me/webhook/test`, and `GET /merchants/me/webhook/deliveries`. The settings routes require a session access token with the `MERCHANT` role and resolve the merchant from `merchant.merchants.owner_user_id`; a merchant ID is never read from the request. They answer `503 SERVICE_UNAVAILABLE` when `JWT_ACCESS_SECRET` or `WEBHOOK_SECRET_KEY` is unset. Signing secrets are encrypted with `packages/go-common/secretbox` (AES-256-GCM) and returned only once; `WEBHOOK_SECRET_KEY` has no key-rotation keyring and stored secrets are not bound to a webhook ID with AAD yet. Deliveries use `packages/webhookdelivery` for signed test sends. Creating a `MERCHANT` identity user and its merchant record is still an onboarding dependency owned by a later milestone, so the webhook endpoints are not production-usable until that lands. Two service-to-service endpoints serve the webhook worker (API v1.1 section 13): `GET /internal/merchants/{merchantId}/webhook` returns the webhook plus its decrypted signing secret (the only place the plaintext secret leaves the service), and `POST /internal/merchants/{merchantId}/webhook/attempts` records an idempotent delivery attempt. They require `X-Internal-Token` (shared `INTERNAL_TOKEN`) and a well-formed `X-Request-Id`; the token is checked first and an unset token fails closed with 401. They answer 503 when `WEBHOOK_SECRET_KEY` or the repository is unavailable, 404 when the merchant has no webhook, and 400/413/422 for malformed or invalid reports. Nginx never routes `/internal`, and responses are `no-store`. Trust boundaries: the shared token lets any internal service read a merchant's signing secret, `parcelId` is trusted caller-supplied data and is not verified against the merchant, and retry/dead-letter consistency is partly owned by the worker.
- `notification-worker`: consumes the legacy `notification.requested.v1` work item and the customer-owned `parcel.created.v1`/`parcel.status_changed.v1` domain events; resolves customer contact through identity; sends branded parcel email; merchant-owned events are dead-lettered as unsupported; SMS is a permanent failure; it records processed event IDs in `notification.processed_events` and delivery outcomes in `notification.notification_logs` (`SENT`, `PENDING`, `FAILED`). The success log and processed-event writes share one transaction. It also exposes the ADMIN-only `GET /api/v1/admin/notification-logs` read API on internal HTTP port `8086`; recipients are masked and status filtering is supported. A background scheduler deletes processed-event rows older than 30 days at startup and every 24 hours by default.
- `webhook-worker`: future outgoing webhook delivery and retries.
- `ai-assistant`: the public Gemini-backed SwiftDrop Delivery Assistant is implemented. It accepts an optional verified access token or a server-generated guest session, enforces a SwiftDrop-only scope plus a deterministic extraction/permission guard, and applies a 15/minute and `AI_MAX_CONCURRENT` limit per authenticated user or guest IP. It retrieves approved embedded Markdown and runs a bounded Gemini function-calling loop over `calculate_delivery_fee`, `get_public_parcel_status`, `get_my_parcel_history` and `get_my_active_deliveries`. All three parcel tools are private: CUSTOMER (own parcels, including tracking-number lookup), DISPATCHER and ADMIN (operational access); guests, MERCHANT and COURIER are denied before any upstream call. Tracking-number lookups call the ownership-scoped Parcel endpoint, never the anonymous public projection; a tracking number is not an authentication credential. It forwards only the caller's verified token, strips private fields, never accepts identity or role from the model, audits privileged lookups, has no database access and exposes no write tools. Guest conversations are bound to an HttpOnly `sd_ai_guest` session; conversation storage, rate limiting and concurrency are process-local; multi-replica coordination is future work.
- `realtime`: the WebSocket `/ws` endpoint (API v1.1 section 9) is implemented with JWT cookie or `?token=` auth, per-parcel subscriptions, DISPATCHER/ADMIN `scope: all`, ping/pong, token-expiry and shutdown handling, and a `parcel.status_changed.v1` consumer. SSE, persistence/replay and multi-replica fan-out remain future work.

## Implemented stack

- Go `1.27.4`, one root module: `github.com/Vineth-R/swiftdrop`.
- Gin `v1.12.0` for HTTP services.
- RabbitMQ client `github.com/rabbitmq/amqp091-go v1.16.0`.
- Next.js 16 App Router, React 19, TypeScript 6, and Tailwind CSS v4 with the SwiftDrop design tokens.
- shadcn/ui with Radix primitives, `class-variance-authority`, `clsx`, `tailwind-merge` and `tw-animate-css`; Lucide React is the only icon library.
- Nx `24.0.0` with pnpm `12.15.0`; CI uses Node.js 24.
- PostgreSQL 16, RabbitMQ 4.1 management, Nginx 1.27, Alpine 3.22 runtime.
- GitHub Actions CI in `.github/workflows/ci.yml`.

Do not add gRPC, Protocol Buffers, Kafka, Turborepo, Kubernetes, Terraform, or a new frontend framework without explicit approval.

## Repository map

| Path                                | Purpose                                                                                                                                                                                                                                                                                                                                                                                           |
| ----------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `apps/web`                          | Next.js App Router frontend with the shadcn/ui foundation, design tokens, `/design` showcase, auth UI kit, app shell, profile/security pages, the COURIER delivery-history screen, the DISPATCHER/ADMIN reports screen, the ADMIN users list, create-user and edit-user screens, the ADMIN notification-log screen and the AI Delivery Assistant chat widget. Other business screens are planned. |
| `apps/web-e2e`                      | Playwright end-to-end tests covering the starter page, `/design` token page, auth UI kit, app shell, profile/security, shadcn primitives, the COURIER delivery-history screen, the reports screen, ADMIN users, ADMIN create and edit user, ADMIN notification logs and the AI chat widget.                                                                                                       |
| `apps/services/identity`            | Identity HTTP service with authentication, admin users, couriers, internal lookups, the own-account endpoints `GET` and `PATCH /users/me` and `PUT /users/me/password`, and the local deterministic demo-user seed command.                                                                                                                                                                       |
| `apps/services/parcel`              | Parcel HTTP service with customer creation/owned reads/PDF receipts, internal merchant create/summary, anonymous public tracking, ownership-scoped authenticated tracking, DISPATCHER/ADMIN reports, COURIER delivery list/detail reads, status updates and private proof uploads, and M6 dispatch APIs/relay. The remaining lifecycle writes remain future work.                                 |
| `apps/services/pricing`             | Pricing HTTP service with estimate, zones, internal quote/zone lookups, and ADMIN configuration.                                                                                                                                                                                                                                                                                                  |
| `apps/services/merchant`            | Merchant HTTP service with profile/admin transitions, API credentials, session-authenticated webhook settings (API 8.12-8.15), and the webhook worker's internal webhook read/attempt endpoints (API section 13). Webhook secrets are AES-256-GCM encrypted; test sends use `packages/webhookdelivery`. Merchant account onboarding remains a future identity dependency.                         |
| `apps/services/ai-assistant`        | Public Gemini-backed AI Delivery Assistant HTTP service with optional JWT identity or guest session, embedded knowledge retrieval, a deterministic extraction guard, a bounded read-only tool registry with role-aware private tools, Pricing/Parcel clients, privileged-lookup auditing, in-process conversations/rate/concurrency state, and safe Markdown responses in the web widget.         |
| `apps/services/realtime`            | Realtime WebSocket `/ws` service with token auth, a subscription hub, and the `parcel.status_changed.v1` consumer; no SSE or persistence.                                                                                                                                                                                                                                                         |
| `apps/services/notification-worker` | RabbitMQ notification worker for the legacy work item and customer-owned parcel domain events, with identity contact resolution, SMTP email delivery, processed-event idempotency, delivery-outcome persistence in `notification.notification_logs`, and an internal ADMIN-only notification-log read API on port 8086.                                                                           |
| `apps/services/webhook-worker`      | RabbitMQ webhook worker skeleton.                                                                                                                                                                                                                                                                                                                                                                 |
| `packages/go-common`                | Shared infrastructure only: config and AMQP URL validation, logging, HTTP helpers, request IDs, server lifecycle, JWT verification/role middleware, rate limiting, internal-token middleware, messaging envelope and framing validation, retry/dead-letter routing, and a RabbitMQ consumer.                                                                                                      |
| `contracts/openapi`                 | Hand-maintained OpenAPI 3.0.3 specs for health/readiness, shared errors, identity, pricing, parcel tracking and reports, merchant, notification, AI assistant, and realtime.                                                                                                                                                                                                                      |
| `contracts/events`                  | Event naming, envelope, framing, versioning conventions, parcel event JSON Schemas, and the legacy notification schema.                                                                                                                                                                                                                                                                           |
| `migrations/<service>`              | golang-migrate SQL migrations. `000001` creates the schema; `000002_tables` is reserved for the platform baseline tables. Owners add new migrations from `000003`.                                                                                                                                                                                                                                |
| `infra/docker`                      | Parameterized Go Dockerfile and Next.js standalone Dockerfile.                                                                                                                                                                                                                                                                                                                                    |
| `infra/nginx`                       | Local reverse proxy configuration and routing notes.                                                                                                                                                                                                                                                                                                                                              |
| `tests/integration`                 | Broker-backed RabbitMQ tests, isolated PostgreSQL dispatch/pricing migration tests, parcel report query tests, and tagged pricing/tracking HTTP fixture tests. The notification-log integration test lives beside its repository package.                                                                                                                                                         |
| `docs`                              | Architecture documentation.                                                                                                                                                                                                                                                                                                                                                                       |
| `.github/workflows`                 | CI workflows.                                                                                                                                                                                                                                                                                                                                                                                     |
| `.agents/skills`                    | Third-party project skills pinned by `skills-lock.json`, plus local Nx skills.                                                                                                                                                                                                                                                                                                                    |
| `.opencode/skills`                  | Pre-existing local Nx skills used by OpenCode.                                                                                                                                                                                                                                                                                                                                                    |

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

- HTTP services read `APP_ENV`, `LOG_LEVEL`, `PORT`, and optional `DATABASE_URL` (pooled) plus `DATABASE_URL_DIRECT` for migrations. Pool settings are `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME`, `DB_CONNECT_TIMEOUT`, and `DB_HEALTH_CHECK_PERIOD`; `POSTGRES_MAX_CONNS`/`POSTGRES_MIN_CONNS` remain aliases. Neon hosts require `sslmode=require` or stronger. They open a shared pgx pool when it is set and report connectivity through `/ready`. Identity additionally reads `EMAIL_VERIFICATION_TOKEN_TTL` (default `30m`), `MERCHANT_BASE_URL` and `MERCHANT_TIMEOUT`; merchant login and refresh check the merchant's approval status and fail closed when it cannot be read. Merchant reads `FRONTEND_BASE_URL`, `SUPPORT_EMAIL`, `IDENTITY_BASE_URL`, `IDENTITY_TIMEOUT`, `PRICING_BASE_URL`, `PRICING_TIMEOUT`, `INTERNAL_TOKEN`, `JWT_ACCESS_SECRET`, `COOKIE_NAME` and the shared SMTP settings. Access tokens carry an `sv` session-version claim; merchant business routes compare it with Identity and require an APPROVED merchant. Pricing also reads `INTERNAL_TOKEN`, `JWT_ACCESS_SECRET`, `COOKIE_NAME` and `HTTP_TRUSTED_PROXIES`. Parcel reads `JWT_ACCESS_SECRET`, `COOKIE_NAME` (default `sd_access`) and `HTTP_TRUSTED_PROXIES`; an unset secret leaves the anonymous public tracking projection running but makes authenticated tracking, reports and courier delivery routes fail closed with `503`. Parcel also reads `PROOF_STORAGE_DIR`/`STORAGE_LOCAL_DIR` for local storage, `STORAGE_PROVIDER` (`local`, `neon`, `minio`, `s3`), `STORAGE_ENDPOINT`, `STORAGE_REGION`, `STORAGE_BUCKET`, `STORAGE_ACCESS_KEY_ID`, `STORAGE_SECRET_ACCESS_KEY`, `STORAGE_USE_PATH_STYLE`, `PROOF_MAX_PER_PARCEL` (default 20, allowed 1-100), `PROOF_MAX_SIZE_BYTES` (default and maximum 5 MiB), and `PROOF_PRESIGNED_URL_TTL_SECONDS` (default 300, allowed 30-3600). An unset provider keeps proof routes registered but fails closed with `503`. Local filesystem storage is the development default; VersityGW is an opt-in local S3-compatible profile and Neon Object Storage is the production provider. A single local replica or shared volume is required because independent local volumes on multiple replicas are unsafe. Merchant reads `JWT_ACCESS_SECRET`, `COOKIE_NAME` (default `sd_access`) and `WEBHOOK_SECRET_KEY`; an unset access secret or webhook key keeps the webhook settings routes registered but fail closed with `503 SERVICE_UNAVAILABLE`. `WEBHOOK_SECRET_KEY` may be empty, but when set it must be exactly 64 hexadecimal characters (32 bytes), must not be a placeholder, and is validated the same way in every environment; invalid values stop startup and the key is never logged. The AI assistant reads `AI_ENABLED`, `AI_PROVIDER` (`gemini` or `mock`), `GEMINI_API_KEY`, `AI_MODEL` (default `gemini-3.5-flash`), `AI_TEMPERATURE`, `AI_MAX_OUTPUT_TOKENS`, the `AI_*` timeout/limit/conversation/rate variables, `AI_MAX_CONCURRENT`, `AI_GUEST_COOKIE_NAME` (the guest cookie is always Secure), `FRONTEND_BASE_URL`, `JWT_ACCESS_SECRET`, `COOKIE_NAME`, `PRICING_BASE_URL`/`PRICING_TIMEOUT` and `PARCEL_BASE_URL`/`PARCEL_TIMEOUT`. It has no database. A missing Gemini key leaves the process running with an unconfigured provider and returns `503 AI_UNAVAILABLE`; it does not take down the gateway. Realtime reads `JWT_ACCESS_SECRET`, `COOKIE_NAME`, `REALTIME_ALLOWED_ORIGINS`, `REALTIME_QUEUE`, `REALTIME_ROUTING_KEY`, and the shared `RABBITMQ_*` settings; it is stateless and has no schema. `AI_PROVIDER=mock` is development-only. The notification worker reads `PORT` (default `8086`), `JWT_ACCESS_SECRET`, and `COOKIE_NAME` for its ADMIN-only read API; an unset secret leaves delivery running but makes that API fail closed. It also reads `IDENTITY_BASE_URL` (default `http://identity:8081`), `IDENTITY_TIMEOUT` (default `3s`), and `INTERNAL_TOKEN` for identity contact lookups, `FRONTEND_BASE_URL` and `SUPPORT_EMAIL` for email links/footer, and the four `NOTIFICATION_PARCEL_*` queue/routing-key variables for the domain-event consumers. `APP_ENV` must be explicitly set to `development`, `staging`, or `production`; all backend services and workers refuse to start when it is missing, empty, or whitespace. In development, an unset or shorter-than-16-character `INTERNAL_TOKEN` is treated as not configured so internal endpoints stay fail-closed. Outside development, identity and pricing refuse to start unless the token is at least 32 random bytes and is not a placeholder. Outside development, a configured `JWT_ACCESS_SECRET` must be at least 32 random bytes and not a placeholder; an unset value leaves pricing, parcel and notification-worker authenticated routes fail-closed with `503`.
- Workers require `RABBITMQ_URL` and read `RABBITMQ_EXCHANGE`, queue/routing-key variables, `RABBITMQ_PREFETCH`, `RABBITMQ_HANDLER_TIMEOUT`, `RABBITMQ_MAX_RETRIES`, `RABBITMQ_RETRY_DELAY`, `RABBITMQ_RECONNECT_MIN`, and `RABBITMQ_RECONNECT_MAX`. The notification worker also reads `NOTIFICATION_PROCESSED_EVENTS_RETENTION` (default `720h`) and `NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL` (default `24h`); both must be positive durations.
- `RABBITMQ_URL` must use the `amqp` or `amqps` scheme and include a host. Validation errors must not include the URL. Use the shared config loader for this check.
- Environment values are matched case-insensitively and trimmed by the shared loader. Keep new parsing consistent with it.
- The maximum accepted RabbitMQ message body is `messaging.MaxMessageBytes` (1 MiB) and is fixed in code. Do not add an environment variable for it without an explicit operational decision.
- Use the existing config loader for parsing and validation.
- Fail startup clearly on invalid or missing required configuration.
- Do not log connection strings, passwords, tokens, or other secrets.
- Go services read process environment variables directly; they do not load `.env`. Docker Compose reads `.env` automatically.
- `.env` is untracked. `.env.example` is the documented local template and must stay free of real credentials.
- Demo users are seeded only by the local `apps/services/identity/seed` command through `make seed`. It reads `DATABASE_URL` and the ten `SEED_*` account variables, is idempotent, never overwrites existing users or profiles, and seeds only the five identity demo users plus two courier profiles. Its only cross-schema read is the lookup of active `pricing.zones` rows by code to resolve the two courier home zones; it does not write pricing data or create a runtime dependency. Do not run it at service startup, Compose startup, or during migrations. The seed command is intentionally outside the seven fail-closed backend services and workers; its `APP_ENV` development default only controls logging and does not enforce production safety. Verify that `DATABASE_URL` points at a disposable local database before running it.

## HTTP API standards

- Use resource-oriented routes and explicit HTTP methods.
- Public routes follow API v1.1 path groups. Nginx strips the `/api/v1` prefix before forwarding, so Gin routers register root paths such as `/auth/...`, `/parcels/...`, `/health`, and `/ready`.
- Current public mapping:
  - `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers` -> identity on `8081`
  - `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/admin/pricing`, `/api/v1/admin/zones` -> pricing on `8083`
  - `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports` -> parcel on `8082` (including additive `/parcels/{parcelId}/proof/photo` and `/content`)
  - `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants` -> merchant on `8084`
  - `/api/v1/admin/notification-logs` -> notification-worker on `8086`
  - `/api/v1/ai` -> ai-assistant on `8087`
  - `/ws` -> realtime on `8085`
  - `/healthz` -> gateway only; `/internal` is never routed
  - `/` -> Next.js web on `3000`
- Do not add a second prefix inside a Gin router unless the routing architecture is deliberately changed.
- When adding or changing a public route, update the Gin route, `contracts/openapi/<service>.yaml`, and `infra/nginx/default.conf` together.
- Validate request bodies, query parameters, and path parameters. Reject malformed input with an appropriate 4xx response.
- Use appropriate status codes: 200/201/204 for success, 400 for malformed input, 401/403 for auth failures, 404 for missing resources, 409 for conflicts, 422 for semantic validation failures, and 500 for unexpected server errors. Pricing returns `422 WEIGHT_NOT_PRICED` for any weight outside every active band (including above the maximum) and `422 ROUTE_NOT_SERVICEABLE` for an unpriced route; the route error takes precedence.
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

Email and password authentication is implemented in the identity service, and `packages/go-common/auth` provides access-token verification and role middleware for every service. Pricing admin routes and the notification-worker notification-log route use `auth.Authenticate` and `auth.RequireRoles(auth.RoleAdmin)`; internal pricing routes use `X-Internal-Token` and fail closed when unset. The notification route also fails closed with `503` when `JWT_ACCESS_SECRET` is unset. Merchant webhook settings use `auth.Authenticate` and `auth.RequireRoles(auth.RoleMerchant)`, then resolve the merchant from the verified token subject through `merchant.merchants.owner_user_id`; they never accept a merchant ID from the request and fail closed with `503` when authentication or the webhook key is not configured. The merchant internal webhook endpoints use `httpx.InternalToken` (shared `INTERNAL_TOKEN`) plus a required `X-Request-Id`; the token is checked first and an unset token fails closed with 401. `MerchantAPIKey` remains the server-to-server credential for merchant API routes and now carries the merchant owner's user ID so its identity satisfies the shared middleware. Add authorization checks to each endpoint as it lands. For all authentication and authorization work:

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
  - `notification.parcel.created.v1`
  - `notification.parcel.status_changed.v1`
  - `webhook.parcel.status_changed.v1`
- Event names and routing keys are versioned. Use `<domain>.<past_tense_action>.v<major>`, for example `parcel.created.v1`.
- The parcel outbox keeps unversioned storage labels (`parcel.created`, `parcel.assigned`, `parcel.status_changed`) and a legacy/internal `routing_key`. The relay must map those labels to `parcel.created.v1`, `parcel.assigned.v1`, and `parcel.status_changed.v1` and use the versioned event name as the published routing key. Do not copy the stored routing key directly.
- Reuse the shared event envelope in `packages/go-common/messaging/envelope.go`. The envelope includes `event_id`, `event_type`, `event_version`, `occurred_at`, `producer`, optional `correlation_id`, and `data`. The outbox UUID is the event ID and `data` is a JSON object.
- Envelope validation is permanent-failure validation: `event_version` must match the version suffix of `event_type`, and `data` must be a JSON object. Do not accept a version mismatch, a non-canonical suffix, or non-object `data`.
- Document event schemas under `contracts/events` using JSON Schema draft 2020-12.
- The public merchant webhook is a separate projection of `parcel.status_changed.v1`; it keeps API v1.1's unversioned public `eventType: parcel.status_changed`, adds `schemaVersion: 1`, and uses the API v1.1 HMAC signing and six-attempt retry contract. The current worker consumes `parcel.status_changed.v1` on its own queue and logs only; do not claim webhook delivery/signing is implemented. `externalOrderId` is not in the domain event; the API v1.1 internal parcel summary is a required future dependency, not an implemented endpoint. See `contracts/events/README.md`.
- The shared consumer validates message framing before the handler runs: the body must be at most 1 MiB and the content type must be absent or `application/json`. Invalid framing is permanent and goes directly to the dead-letter queue.
- The notification worker validates the envelope, event type/version, and the payload fields it uses, then delivers email through the shared SMTP mailer. It supports the legacy `notification.requested.v1` work item and the customer-owned `parcel.created.v1`/`parcel.status_changed.v1` domain events; the domain path resolves customer contact through the identity internal endpoint. Merchant-owned parcel notifications have no approved recipient-contact lookup and are dead-lettered as unsupported. The webhook worker validates `parcel.status_changed.v1` and logs only. The shared consumer retries retryable failures and dead-letters permanent or exhausted failures. Workers own their persistence through service-scoped repositories: the notification worker writes `notification.processed_events` and `notification.notification_logs` and reads the latter for its admin API.
- The notification worker runs processed-event cleanup in its own context-aware goroutine at startup and every cleanup interval. Runs are serial within one worker; cleanup failures are logged and retried without stopping notification consumption. Shutdown cancels the scheduler and waits for it to exit before closing the database. A single cutoff-based `DELETE` is safe across replicas; no advisory lock is required. Cleanup never modifies `notification_logs`. Idempotency is guaranteed only while the processed-event row is retained, so a late redelivery after retention can be processed again.
- Customer/internal merchant creation writes canonical `parcel.created` outbox data in the parcel/history transaction. M6 relay publication, assignment writers and M7 courier status writers are implemented; status updates write their history row and PENDING outbox event in the same transaction as the parcel update. The relay must publish versioned `parcel.created.v1`, `parcel.assigned.v1` and `parcel.status_changed.v1` and validate payloads against `contracts/events`.
- Acknowledge a message only after processing succeeds or after a publisher-confirmed retry or dead-letter publication. Never acknowledge a failed message.
- Classify failures as permanent or retryable. Malformed JSON, invalid envelopes, unsupported event types or versions, invalid event data, unsupported content types, and oversize bodies are permanent. Unclassified handler errors are retryable.
- Retry at most `RABBITMQ_MAX_RETRIES` times (default `3`) with `RABBITMQ_RETRY_DELAY` (default `30s`), then dead-letter. Do not requeue failures indefinitely.
- Retry and dead-letter resources are derived from the main exchange and queue: `<exchange>.retry`, `<queue>.retry.v2`, `<exchange>.dlx`, `<queue>.dlq`. Do not change the arguments of existing main queues. The retry queue name carries a topology suffix because its dead-letter arguments are immutable; a changed retry topology uses a new queue name. Retries are dead-lettered to their owning queue through the default exchange (`x-dead-letter-exchange: ""`, `x-dead-letter-routing-key: <queue>`), so multiple workers may bind separate queues to the same domain routing key without sharing retries. The legacy `<queue>.retry` queue is no longer written to; drain and delete it after upgrading. The dead-letter queue has no consumer.
- Reconnect with capped exponential backoff and jitter between `RABBITMQ_RECONNECT_MIN` and `RABBITMQ_RECONNECT_MAX`. Reset backoff after a session reaches the consume phase. Reconnect waits and dials must honor context cancellation so shutdown is prompt.
- Handler deadlines are cooperative: handlers must honor their context because Go cannot preempt arbitrary code. Keep the serial consumer unless a concrete throughput requirement justifies concurrency; prefetch is a buffer, not concurrency.
- Design consumers for at-least-once delivery. Duplicate or redelivered messages are normal after reconnects and retries. Deduplicate by `event_id` and make side effects idempotent before adding state-changing handlers.
- Worker shutdown cancels the consumer and requeues in-flight messages. A second termination signal forces exit if graceful shutdown does not finish; preserve this behavior.
- Keep message handling independent of HTTP request lifecycles.
- Make failures observable through structured logs and, when added, metrics.
- Consider a transactional outbox when database changes and event publication must be consistent.
- Do not require RabbitMQ for ordinary unit tests. Reserve broker-backed tests for `tests/integration` with an explicit build tag.
- Workers do not publish host ports. The notification worker has one deliberate exception: an internal ADMIN-only read API on `8086` for `GET /api/v1/admin/notification-logs`, reachable only through the gateway. Do not add worker HTTP APIs solely for parity with other services.

## PostgreSQL and migrations

- Use PostgreSQL for persistence where configured.
- Use one local database with separate schemas per owning service: `identity`, `parcel`, `pricing`, `merchant`, and `notification`. Realtime is stateless.
- Services own their schema. Do not read or modify another service's schema.
- Use versioned migrations under `migrations/<service>/` with the `NNNNNN_description.up.sql` and `.down.sql` naming convention.
- Use golang-migrate as documented in `migrations/README.md`. Each service uses its own migration version table.
- `000001` creates the service schema. `000002_tables` is reserved for the platform baseline tables from `docs/db.sql`. Other owners add new migrations from `000003`; do not renumber or reuse an applied version.
- Parcel `000003_dispatch_foundations` extends the baseline `parcel_assignments` and `outbox_events` tables. It adds reassignment metadata, an assignment `updated_at` trigger, and outbox aggregate metadata without recreating tables.
- Parcel `000006_customer_parcel_list_index` adds the additive `(customer_user_id, created_at, id)` index for the ordered customer list; `000004` and `000005` remain unchanged.
- Applied migrations are immutable. Fix mistakes with a new migration.
- Do not run migrations automatically at service startup.
- Do not modify schemas manually without a migration.
- Production changes should be forward-only and use expand/contract patterns where needed. Down migrations remain useful for local development and review.
- Down migrations that drop schemas or tables are destructive. Confirm the target database before running them.
- HTTP services open a shared pgx pool when `DATABASE_URL` is set and report connectivity through `/ready`. Missing `DATABASE_URL` leaves the service running with `/ready` returning 503.
- Use local transactions where needed. Avoid distributed transactions.
- Baseline business tables come from `docs/db.sql`. Do not add speculative tables beyond it without a task requirement.

## Frontend standards

- Before you implement or change any frontend screen or component, read [`DESIGN.md`](DESIGN.md) in full and follow it. It is the design contract: the design language rules, the allowed tokens, the component kit, the app shell and role navigation, forms, API error handling, tests, and the checklist to finish with. Do not change the design foundation (tokens, kit components, shell, API client) unless the task is explicitly about it. ESLint enforces the checkable parts (raw colours, off-token classes, off-scale spacing, inline styles, banned libraries, direct `fetch`, `error.message`, a raw `<h1>`): see `DESIGN.md` section 16, and do not add an `eslint-disable` just to pass.
- Preserve the Next.js App Router structure in `apps/web/src/app`.
- Use TypeScript and preserve strict type checking. Do not loosen compiler strictness to bypass errors.
- Prefer React Server Components by default. Add `'use client'` only when browser APIs, event handlers, or client-side state require it.
- Keep API access consistent. Browser code should call public `/api/v1/...` routes through the edge, not internal service ports.
- Do not duplicate backend business logic in the frontend. The backend remains authoritative for validation, authorization, pricing, and state transitions.
- Validate user-facing forms and handle loading, error, empty, and success states.
- Follow accessibility practices and responsive design. The existing ESLint setup includes `jsx-a11y`.
- Style with Tailwind v4 utilities against the design tokens in `apps/web/src/app/globals.css`. Use tokens only: no raw hex values, off-scale spacing or second icon family (Lucide only). `/design` shows every token in light and dark. shadcn/ui is the approved component foundation; do not add another UI library or a state-management library without approval.
- Avoid unnecessary client-side JavaScript and dependencies.
- Browser code calls the Go API only through `/api/v1/...`; the Next.js app exposes no example API route. Do not confuse Next.js routes with the Go `/api/v1/` namespace.
- Signed-in screens go in `apps/web/src/app/(app)/<name>/page.tsx`. That group's layout already provides the session gate (splash, redirect to `/login`) and the shell for the person's role, so a page only renders its own content. Start every page with `PageHeader`, then `Section`s.
- Add a screen to the sidebar (or the courier's bottom tabs) in `apps/web/src/lib/nav.ts`. Hiding a link never protects a page: wrap a role-limited page's content in `RequireRole`, and the API checks again. A page that is not built yet shows the shell's 404.
- `apps/web` currently contains the shadcn/ui foundation (`components/ui`), the design system (`/design`), the auth screens, the app shell, the profile and security pages (`/profile`, `/profile/security`, a worked example of a form screen), the role-based dashboard (`components/dashboard`: customer, merchant and ADMIN homes, plus the courier's Today view; a dispatcher is sent to `/unassigned`), the COURIER delivery-history screen (`/history`, backed by the parcel courier list endpoint), the DISPATCHER/ADMIN reports screen (`/reports`), the ADMIN users list, create-user and edit-user screens (`/admin/users`), the ADMIN notification-log screen and the public parcel tracking page (`/track`). Other role screens are still to be built.
- `apps/web-e2e` currently covers the starter page, `/design` token page, auth UI kit, app shell, profile/security, shadcn primitives, the COURIER delivery-history screen, the reports screen, the ADMIN users list, create-user and edit-user screens, the ADMIN notification-log screen, the booking wizard's fee-estimate error and rate-limit coverage, and public tracking. Test a screen without a backend with `mockApi` and `signInAs` from `apps/web-e2e/src/support`, and extend the suite for meaningful user journeys when those journeys exist.
- The shadcn foundation lives in `apps/web/components.json`, `apps/web/src/components/ui`, and `apps/web/src/lib/utils.ts`. The generated primitives are bridged to SwiftDrop tokens in `globals.css`; `accent` remains the lime brand token, so hover and selected states use `surface-2`, not shadcn accent. `lib/utils.ts` registers the SwiftDrop type scale with tailwind-merge so `text-body-sm text-text` keeps both classes. Keep the SwiftDrop public component APIs (`Button`, `Alert`, `Field`, `PasswordField`, `EmptyState`, `Drawer`, `UserMenu`, `Select`, `Textarea`, `Modal`, `DataTable`, `StatusChip`, `Toast`, `Timeline`) intact. Radix imports are allowed only inside `components/ui`; screens use those wrappers. `SelectMenu` is the shadcn Radix listbox; `Select` remains the native labelled form control. `Dialog` and `Sheet` are the shadcn overlays; `Modal` remains the native decision dialog. Do not add generic shadcn defaults to screens or a second theme system.

## Testing requirements

Add or update tests for every meaningful change.

### Go

- Unit tests for business logic when it is introduced.
- HTTP handler and route tests.
- Configuration and validation tests.
- Repository tests when a database layer is added.
- Integration tests for PostgreSQL and broker-backed RabbitMQ tests under `tests/integration`. PostgreSQL tests create and drop uniquely named databases and require `TEST_DATABASE_URL`. Pricing migration tests live in `tests/integration/database`; the pricing HTTP integration test lives beside the handler and is run with the integration tag. The notification-log repository integration test lives in `apps/services/notification-worker/internal/notificationlog` and also runs with the integration tag. The processed-event cleanup integration test lives in `apps/services/notification-worker/internal/idempotency` and uses its own disposable database. The demo-user seed integration test lives in `apps/services/identity/internal/seed` and also runs with the integration tag.
- Tests for validation, authorization, and failure paths.
- Existing tests use the Go standard library testing package. Follow that style unless there is a clear reason to add another tool.
- Unit tests must not require PostgreSQL or RabbitMQ. Use `//go:build integration` for infrastructure-backed tests under `tests/integration`.
- Broker-backed messaging tests cover acknowledgement, retry-then-success, permanent dead-lettering, retry exhaustion, and panic recovery. Coverage that must not depend on a live broker stays in `packages/go-common/messaging`.
- Use `go test -race ./...` for concurrency-sensitive changes when feasible.
- CI sets `RABBITMQ_URL` for the integration job and fails when any integration test is skipped. Do not rely on silent skips.

### Frontend

- Test meaningful component and application behavior when the UI is implemented.
- Pure frontend logic is unit-tested with Vitest under `apps/web/src/**/*.spec.ts`; run `pnpm exec nx test @org/web`. API client and date-helper tests live beside their module as `apps/web/src/lib/*.test.mjs` and run with `node --test "apps/web/src/lib/*.test.mjs"`. Component and flow behavior stays in Playwright.
- Use the existing Nx Playwright target for important end-to-end workflows.
- The design lint rules in `apps/web/eslint-rules` have their own tests: `node --test "apps/web/eslint-rules/*.test.mjs"`. CI runs them, and you run them when you change a rule.
- Event schemas in `contracts/events` are validated with `node --test "tests/contracts/*.test.mjs"` using Ajv 2020-12. CI runs this in the frontend job; update the tests when a schema or example changes.
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
- `@org/web` targets: `build`, `dev`, `start`, `serve-static`, `build-deps`, `watch-deps`, `lint`, `test`, `typecheck`. `test` runs the Vitest unit suite; `typecheck` checks the unit tests and Vitest config; the Next build performs application type checking.
- `@org/web-e2e` targets: `lint`, `typecheck`, `e2e`, and Playwright CI targets. It has no `build` target.
- Go projects have `build`, `test`, `lint`, and `serve` targets. `packages/go-common/objectstore` also has S3-compatible (VersityGW-backed) integration tests behind `integration,storage`. `go-common` has `build`, `test`, and `lint`.
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
  pnpm exec nx test @org/web
  pnpm exec nx typecheck @org/web
  pnpm exec nx lint @org/web-e2e
  pnpm exec nx typecheck @org/web-e2e
  pnpm exec nx e2e @org/web-e2e
  pnpm exec nx run-many -t lint test build -p identity,parcel,pricing,merchant,notification-worker,webhook-worker,realtime,go-common
  RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' go test -tags=integration ./tests/integration
  STORAGE_TEST_ENDPOINT='http://localhost:7070' STORAGE_TEST_ACCESS_KEY='swiftdrop' STORAGE_TEST_SECRET_KEY='swiftdrop-s3' make test-storage
  TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' make test-db
  TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' go test -tags=integration ./tests/integration/database/... ./apps/services/pricing/internal/http/... ./apps/services/notification-worker/internal/idempotency/... ./apps/services/notification-worker/internal/notificationlog/... ./apps/services/identity/internal/seed/...
  ```
- `pnpm exec nx lint @org/web` and `pnpm exec nx build @org/web` are valid. `pnpm exec nx serve @org/web` is not; use `dev` or `start`.
- The CI frontend job runs the design-lint and API-client Node tests, `pnpm exec nx test @org/web`, `pnpm exec nx typecheck @org/web`, `pnpm exec nx run-many -t lint build --projects=@org/web,@org/web-e2e`, and `pnpm exec nx typecheck @org/web-e2e`. Nx runs only targets that are configured for each project.
- CI runs Playwright E2E for Chromium only with `pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"`. The Playwright config still defines Firefox and WebKit projects for local use.
- Run `pnpm exec prettier --write` and `pnpm exec prettier --check` on changed documentation and frontend files. Prettier ignores the third-party skill directories.

## Docker and infrastructure

- Keep the existing `docker-compose.yml` filename. Do not rename it to `compose.yaml` solely because a general skill prefers that name.
- Compose services:
  - `postgres` on 5432, persistent volume, `pg_isready` health check, schema/function init script.
  - `rabbitmq` on 5672 and management on 15672, persistent volume, diagnostics health check.
  - `mailpit` on 1025 (SMTP) and 8025 (UI), health check.
  - `identity` 8081, `parcel` 8082, `pricing` 8083, `merchant` 8084, `realtime` 8085.
  - `notification-worker` on internal HTTP `8086` for its ADMIN read API and no published port; `webhook-worker`, no ports.
  - `web` on 3000.
  - `nginx` on 8080.
  - `migrate` tools-profile service for golang-migrate.
- Only the gateway, RabbitMQ (5672/15672), and Mailpit (1025/8025) publish host ports. Service, worker, web, and Postgres ports are internal to the Compose network. Do not carry any of this exposure into production architecture without review.
- The Go Dockerfile builds from the repository root context and accepts a `SERVICE` build argument. It uses a multi-stage build, a static binary, a minimal Alpine runtime, and a non-root user.
- The web Dockerfile uses Node 24, pnpm, Nx build, Next standalone output, and a non-root user.
- Do not bake secrets into images. `.dockerignore` excludes `.env` files and agent/tooling directories.
- Use Compose service names for container-to-container addresses. Do not use `localhost` for another container.
- Compose starts Nginx only after its web and HTTP service dependencies report healthy, including the notification worker's `/ready` check. Workers depend on a healthy RabbitMQ and use a 35-second stop grace period so in-flight messages can be requeued.
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
- The CI workflow file is the source of truth for CI. It runs Go formatting, vet, race tests with a coverage artifact, build, Compose config validation, and a worker-healthcheck guard; a separate RabbitMQ integration job that fails on skipped tests; frontend lint/build and E2E typecheck; Chromium-only Playwright E2E; changed-service Docker image builds and scans; Compose-backed access tests with their own path filter and access output (triggered by Compose, Nginx, PostgreSQL initialization, migrations, access tests, the Makefile, migration scripts, `.env.example`, or any Docker-service change); a stable `Required CI gate` aggregate; and a `v*`/`milestone-*` tag release job that prepares `.env` from `.env.example`, builds every image, starts the Compose stack with `--wait`, applies migrations with `make migrate`, runs route checks, and stops the stack. Security jobs use pinned `govulncheck`, `pnpm audit --prod`, Gitleaks, CodeQL, and Trivy/Syft; a `security-gate` is required by `Required CI gate`. Dependency updates are managed manually; Dependabot is not enabled. Do not repeat stale CI summaries, and do not claim a CI run passed unless its result was observed.

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
- Security-sensitive changes pass `govulncheck ./...`, `pnpm audit --prod --audit-level=high`, Gitleaks, and the container scan/SBOM gate when images change.
- Relevant Nx targets pass, such as `pnpm exec nx test identity` or an `nx run-many` command for the affected Go projects.
- Frontend changes pass `pnpm exec nx lint @org/web` and `pnpm exec nx build @org/web`; E2E changes pass `pnpm exec nx typecheck @org/web-e2e` and the relevant `pnpm exec nx e2e @org/web-e2e` target.
- Messaging changes run the broker-backed integration tests when RabbitMQ is available and report any coverage that could not run.
- `docker compose config` passes for Compose changes.
- Object storage changes run `make test-storage` when the local S3-compatible service is available, or report the gap.
- `pnpm exec prettier --check` passes on changed documentation and frontend files.
- API, event, migration, environment, and documentation changes are updated together.
- `git diff --check` passes.
- No secrets, generated build output, unrelated rewrites, or unverified claims are included.

If a check cannot run because a tool, service, credential, or browser is unavailable, say so explicitly and explain the gap.
