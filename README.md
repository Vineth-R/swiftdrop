# SwiftDrop

SwiftDrop is a parcel delivery and courier management platform for customers,
merchants, couriers, dispatchers and administrators. It covers the whole delivery
lifecycle — registration and sign-in, fee estimation, parcel creation, dispatch,
courier pickup and delivery with photo proof, public tracking, merchant orders and
signed webhooks, notifications, reports, and an AI delivery assistant.

The repository is a production-oriented Nx monorepo: a Next.js App Router frontend,
independently deployable Go HTTP services, RabbitMQ workers, one shared Go
infrastructure package, PostgreSQL migrations with one schema per service, OpenAPI
and event contracts, Docker Compose infrastructure, and Nginx routing. GitHub
Actions verifies the project on every pull request and on pushes to `development`
and `main`.

This README is the entry point. Deeper material is linked rather than duplicated:
[`docs/architecture.md`](docs/architecture.md),
[`docs/API_Documentation_v1.1.md`](docs/API_Documentation_v1.1.md),
[`docs/Database_Design_Document_v1.1.md`](docs/Database_Design_Document_v1.1.md),
[`docs/operations.md`](docs/operations.md) and [`AGENTS.md`](AGENTS.md).

## 1. Project overview

SwiftDrop replaces ad-hoc courier coordination with one traceable system:

- **Customers** estimate a fee, create a parcel, download a receipt and follow it on
  the public tracking page.
- **Merchants** register for approval, create orders through an authenticated API,
  manage webhook subscriptions, and watch order status synchronize automatically.
- **Couriers** see the deliveries assigned to them, confirm pickup, move a parcel to
  out-for-delivery, and confirm delivery with a mandatory photo and recipient name.
- **Dispatchers** assign and reassign parcels, watch live updates, and review daily
  and outcome reports.
- **Administrators** manage users, merchants, pricing, notification logs and audit
  logs.
- **Everyone** can ask the Gemini-backed SwiftDrop Delivery Assistant a question, and
  signed-in customers, dispatchers and administrators can use its private parcel tools.

The design is deliberately service-oriented. Synchronous work uses HTTP/JSON. Work
that can be retried independently — notifications, merchant webhooks, merchant order
status synchronization and realtime fan-out — uses RabbitMQ events with a
transactional outbox in the Parcel service. Each service owns its schema and never
reads another service's tables.

For the current implementation boundary, see [Key features](#2-key-features) and
[Known limitations](#known-limitations). Planned work is labelled as planned and is
not presented as implemented.

## 2. Key features

### Identity and access

- Customer registration with single-use email verification; unverified customers
  cannot obtain a session.
- Email/password login, refresh-token rotation with reuse detection, logout with
  refresh-token revocation, and idempotent logout after access-token expiry.
- Google OAuth 2.0 sign-in (opt-in; requires your own Google client).
- Password reset by email, password change, account lockout after repeated failures.
- Merchant registration and ADMIN approval, rejection, suspension and re-approval.
- Session versioning (`sv` JWT claim) so suspension and re-approval invalidate old
  access tokens; a password change revokes every refresh token.
- Role-based access for `CUSTOMER`, `MERCHANT`, `COURIER`, `DISPATCHER` and `ADMIN`
  enforced by the API, not only by the UI.
- Profile read/update, ADMIN user management, courier directory and ADMIN audit-log
  reads for identity, merchant and pricing.

### Pricing and public tracking

- Exact decimal LKR estimation by zone, weight band and size surcharge.
- Public zone listing and public parcel tracking with an allowlisted projection (no
  names, phone numbers, addresses, prices or internal IDs).
- ADMIN pricing and zone configuration with transactional audit logging.
- Live tracking updates over WebSocket.

### Parcels and dispatching

- Customer parcel creation that stores the accepted real pricing quote, the initial
  history row and a pending outbox event atomically.
- Customer-owned list, detail, history and PDF receipt.
- Internal merchant parcel creation and summary for trusted callers.
- Dispatcher assignment and reassignment with courier eligibility checks, one active
  assignment per parcel, and terminal-state protection.
- Dispatcher All parcels and Unassigned queues with live status and assignment
  notices.
- DISPATCHER/ADMIN daily and outcome reports with a chart, table and top failure
  reasons.

### Courier workflow

- Today list and delivery history for the signed-in courier.
- Assignment-scoped delivery detail with sender/recipient and parcel information.
- Proof-backed pickup (`CREATED → PICKED_UP → IN_TRANSIT`) and delivery
  (`OUT_FOR_DELIVERY → DELIVERED`) confirmations that commit status, history, proof
  metadata and outbox data in one transaction.
- Mandatory delivery photo and recipient name; failure reason for failed deliveries.
- Private proof-photo storage with short-lived presigned reads and authorization.

### Merchants, webhooks and notifications

- Merchant order API (API v1.1 sections 8.16–8.18) authenticated by merchant API key,
  with idempotency, snapshots and an initial timeline.
- Merchant portal: order dashboard, list, detail, receipts, API-key management,
  integration guide and Create via API panel.
- Webhook settings with encrypted signing secrets, signed test sends, delivery
  history and the six API v1.1 business retries.
- Asynchronous merchant order status synchronization on `merchant.sync.queue`.
- Branded transactional email for identity and parcel events; merchant-owned events
  resolve the approved merchant contact through an internal API.
- ADMIN notification-log read API and screen with masked recipients.

### Realtime

- WebSocket `/ws` with cookie or `?token=` authentication.
- Per-parcel subscriptions for anonymous and customer clients; `scope: all` for
  dispatchers and administrators.
- Consumers for `parcel.status_changed.v1` and `parcel.assigned.v1`.

### AI Delivery Assistant

- Public floating chat widget on every page, backed by Gemini.
- Server-side guest sessions and optional verified JWT identity.
- Exactly four read-only tools: fee estimate, ownership-scoped parcel status, owned
  parcel history and owned active deliveries.
- Deterministic extraction/permission guard, approved embedded knowledge, safe
  Markdown rendering, privileged-lookup auditing and safe fallbacks.
- `AI_PROVIDER=mock` for deterministic development and tests.

### Platform

- Health and readiness endpoints, structured logging, request IDs, panic recovery,
  JSON error envelopes, graceful shutdown and Prometheus metrics.
- PostgreSQL migrations with rollback, verification, and restore/rehearsal tooling.
- RabbitMQ publisher confirms, bounded retries, dead-lettering and idempotent
  consumers.
- CSRF origin enforcement, a Content Security Policy, security headers, rate limits,
  private object storage and credential redaction.
- CI covering Go, frontend, integration, event-flow, access, real-stack, security and
  container scans.

## 3. Application screenshots

No screenshots are committed to the repository, so none are shown here rather than
presenting stale or unverified images. The implemented screens are:

| Area       | Routes                                                                                                                                                                                                        |
| ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Public     | `/`, `/login`, `/register`, `/register/customer`, `/register/merchant`, `/verify-email`, `/forgot-password`, `/reset-password`, `/signed-out`, `/track`, `/merchant/pending`, `/design`                       |
| Customer   | `/dashboard`, `/new`, `/parcels`, `/parcels/{id}`, `/profile`, `/profile/security`                                                                                                                            |
| Merchant   | `/dashboard`, `/merchant/orders`, `/merchant/orders/{orderId}`, `/merchant/integration`, `/merchant/profile`, `/api-keys`, `/webhook`, `/profile`                                                             |
| Courier    | `/dashboard` (Today), `/history`, `/workload`, `/deliveries/{parcelId}`                                                                                                                                       |
| Dispatcher | `/dashboard`, `/parcels`, `/unassigned`, `/reports`, `/profile`                                                                                                                                               |
| Admin      | `/dashboard`, `/admin/users`, `/admin/users/new`, `/admin/users/{userId}/edit`, `/admin/merchants`, `/admin/notification-logs`, `/admin/audit-logs`, `/admin/pricing`, `/admin/zones`, `/reports`, `/profile` |

`/design` is the running style guide: it shows every design token and component in
light and dark mode. To capture screenshots for a report, start the stack
(`make up-build`), open <http://localhost:8080>, and use a browser screenshot tool.
Editable architecture and ER diagram sources live in [`docs/diagrams`](docs/diagrams).

## 4. System architecture

```text
Browser
  |
  v
Nginx :8080 (only published application port)
  |-- /                              -> Next.js web :3000
  |-- /api/v1/auth|users|couriers    -> identity :8081
  |-- /api/v1/admin/users            -> identity :8081
  |-- /api/v1/parcels|tracking       -> parcel :8082
  |-- /api/v1/dispatch|courier       -> parcel :8082
  |-- /api/v1/reports                -> parcel :8082
  |-- /api/v1/pricing|zones          -> pricing :8083
  |-- /api/v1/admin/pricing|zones    -> pricing :8083
  |-- /api/v1/merchants|merchant     -> merchant :8084
  |-- /api/v1/admin/merchants        -> merchant :8084
  |-- /api/v1/admin/notification-logs-> notification-worker :8086
  |-- /api/v1/ai/chat                -> ai-assistant :8087
  |-- /ws                            -> realtime :8085
  |-- /healthz                       -> gateway health
  `-- /internal                      -> 404 (never routed)

Workers and services
  |-- merchant            -> RabbitMQ (merchant.sync.queue), PostgreSQL, internal HTTP
  |-- notification-worker -> RabbitMQ, PostgreSQL, admin HTTP :8086, Identity + Merchant HTTP
  |-- webhook-worker      -> RabbitMQ, Merchant + Parcel internal HTTP
  |-- realtime            -> RabbitMQ, PostgreSQL, WebSocket
  `-- ai-assistant        -> Pricing + Parcel internal HTTP, Gemini

PostgreSQL, RabbitMQ and Mailpit are addressed by Compose service names.
RabbitMQ (5672/15672) and Mailpit (1025/8025) publish host ports;
PostgreSQL, application services and workers do not.
```

```mermaid
flowchart LR
  B[Browser] --> N[Nginx gateway]
  N --> W[Next.js web]
  N --> ID[identity]
  N --> P[parcel]
  N --> PR[pricing]
  N --> M[merchant]
  N --> R[realtime]
  N --> A[ai-assistant]
  N --> NW[notification-worker]
  P --> PG[(PostgreSQL)]
  ID --> PG
  PR --> PG
  M --> PG
  NW --> PG
  R --> PG
  P -->|outbox relay| Q[[RabbitMQ]]
  Q --> NW
  Q --> WH[webhook-worker]
  Q --> R
  Q --> M
  A --> PR
  A --> P
  A --> G[Gemini API]
  WH --> M
  WH --> P
  NW --> ID
  NW --> M
```

| Service               | Responsibility                                                                   | Internal port |
| --------------------- | -------------------------------------------------------------------------------- | ------------- |
| `identity`            | Authentication, users, couriers, internal identity lookups                       | 8081          |
| `parcel`              | Parcels, tracking, dispatch, courier workflow, proofs, reports, receipts, outbox | 8082          |
| `pricing`             | Estimates, zones, pricing rules and ADMIN configuration                          | 8083          |
| `merchant`            | Merchant accounts, API keys, orders, webhooks, status sync, internal lookups     | 8084          |
| `realtime`            | WebSocket `/ws` and status/assignment consumers                                  | 8085          |
| `notification-worker` | Email delivery, notification log, ADMIN notification-log read API                | 8086          |
| `ai-assistant`        | Gemini-backed Delivery Assistant and its four read-only tools                    | 8087          |
| `webhook-worker`      | Signed merchant webhook delivery and business retries                            | 8088          |
| `web`                 | Next.js frontend                                                                 | 3000          |
| `nginx`               | Public gateway                                                                   | 8080 (host)   |

Each persisting service owns one PostgreSQL schema: `identity`, `parcel`, `pricing`,
`merchant` and `notification`. Realtime is stateless. Services never read another
service's schema; cross-service reads use HTTP and side effects use events. See
[`docs/architecture.md`](docs/architecture.md) for lifecycle, readiness and messaging
details.

## 5. Technology stack

| Layer         | Choice                                                                                                            |
| ------------- | ----------------------------------------------------------------------------------------------------------------- |
| Frontend      | Next.js 16 App Router, React 19, TypeScript 6, Tailwind CSS v4, shadcn/ui (Radix), Lucide icons, `react-markdown` |
| Monorepo      | Nx 24, pnpm 12.15.0, Vitest, Playwright                                                                           |
| Backend       | Go 1.27.4, Gin v1.12.0, one root Go module                                                                        |
| Data          | PostgreSQL 16, pgx, golang-migrate v4.18.3                                                                        |
| Messaging     | RabbitMQ 4.1 management (`amqp091-go`)                                                                            |
| AI            | Google Gemini through `google.golang.org/genai`                                                                   |
| Storage       | Local filesystem (development), VersityGW S3-compatible (opt-in), Neon Object Storage (production)                |
| Edge          | Nginx 1.27                                                                                                        |
| Local runtime | Docker Compose v2                                                                                                 |
| CI/CD         | GitHub Actions, GHCR image publication, CodeQL, Gitleaks, Trivy, Syft                                             |
| Observability | `log/slog`, request IDs, Prometheus `/metrics`                                                                    |

## 6. Repository structure

```text
apps/
  web/                     Next.js App Router application
  web-e2e/                 Playwright end-to-end and real-stack tests
  services/
    identity/              Authentication, users, couriers
    parcel/                Parcels, dispatch, courier, proofs, reports, outbox relay
    pricing/               Fees, zones, pricing administration
    merchant/              Merchants, orders, API keys, webhooks, status sync
    realtime/              WebSocket gateway and consumers
    ai-assistant/          Gemini assistant, tools and knowledge base
    notification-worker/   Email delivery and notification log
    webhook-worker/        Signed merchant webhook delivery
packages/
  go-common/               Shared infrastructure (no business entities)
  webhookdelivery/         Webhook signing and delivery helper
contracts/
  openapi/                 Per-service OpenAPI specifications
  events/                  Versioned event JSON Schemas and conventions
  routes/                  Runtime Gin route inventories
migrations/
  identity/ parcel/ pricing/ merchant/ notification/
infra/
  docker/                  Go and Next.js Dockerfiles
  nginx/                   Gateway configuration
docs/                      Architecture, API, database, operations and report sources
tests/
  integration/             RabbitMQ, PostgreSQL and event-flow suites
  access/                  Cross-service authorization matrix
  contracts/               Contract and route-inventory tests
  perf/                    k6 baseline scenarios
  scripts/                 CI helper tests
scripts/                   Operational and release scripts
.github/workflows/         CI, deployment and security workflows
```

Each Go service has a `main.go`, `internal/config` and either `internal/http` or
`internal/worker`, with tests beside the code. Do not create a nested `go.mod`, a
`go.work`, a shared business package, or direct cross-schema access.

## 7. Prerequisites

- **Go 1.27.4** or a compatible 1.27 toolchain (`go.mod` is authoritative).
- **Node.js 24** — the supported and CI-tested version.
- **pnpm 12.15.0** through Corepack.
- **Docker** with **Docker Compose v2**.
- **make** and **curl**.
- Optional: `golang-migrate` CLI, `psql`, Chromium for Playwright, `k6` for the
  performance scenarios, a Gemini API key for live AI answers.

```sh
corepack enable
corepack prepare pnpm@12.15.0 --activate
```

Docker must be running before any Compose command. The quick start needs only
Docker, Compose, `make`, `curl` and `openssl`; Go and Node.js are for host-side
development and tests.

## 8. Environment setup

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
pnpm install --frozen-lockfile
```

`.env` is untracked and is the only place for local secrets. Docker Compose reads it
automatically. Go services read process environment variables directly and do **not**
load `.env`; export it before running a service or worker on the host:

```sh
set -a
. ./.env
set +a
```

### Required values

`APP_ENV` must be exactly `development`, `staging` or `production`; every backend
service and worker refuses to start without it. Generate local secrets rather than
copying placeholders:

```sh
# JWT_ACCESS_SECRET must be at least 32 random bytes outside development.
printf 'JWT_ACCESS_SECRET=%s\n' "$(openssl rand -hex 32)"
printf 'JWT_REFRESH_SECRET=%s\n' "$(openssl rand -hex 32)"
printf 'INTERNAL_TOKEN=%s\n' "$(openssl rand -hex 32)"
printf 'WEBHOOK_SECRET_KEY=%s\n' "$(openssl rand -hex 32)"
```

| Group          | Variables                                                                                                                                                                                 | Notes                                                                        |
| -------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- |
| Runtime        | `APP_ENV`, `LOG_LEVEL`, `NGINX_PORT`                                                                                                                                                      | `APP_ENV` is required; gateway defaults to `8080`                            |
| PostgreSQL     | `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, `DATABASE_URL`, `DATABASE_URL_DIRECT`                                                                                                | Compose builds the container URL; `DATABASE_URL` is for host-side tools only |
| Pool tuning    | `DB_MAX_OPEN_CONNS`, `DB_MAX_IDLE_CONNS`, `DB_CONN_MAX_LIFETIME`, `DB_CONN_MAX_IDLE_TIME`, `DB_CONNECT_TIMEOUT`, `DB_HEALTH_CHECK_PERIOD`                                                 | `POSTGRES_MAX_CONNS` / `POSTGRES_MIN_CONNS` remain aliases                   |
| RabbitMQ       | `RABBITMQ_URL`, `RABBITMQ_DEFAULT_USER`, `RABBITMQ_DEFAULT_PASS`, `RABBITMQ_EXCHANGE`, `RABBITMQ_MAX_RETRIES`, `RABBITMQ_RETRY_DELAY`, `RABBITMQ_RECONNECT_MIN`, `RABBITMQ_RECONNECT_MAX` | Defaults work with Compose                                                   |
| Auth           | `JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`, `COOKIE_NAME`, token TTLs, cookie flags                                                                                                        | Secrets must be at least 32 bytes outside development                        |
| Internal calls | `INTERNAL_TOKEN`, `IDENTITY_BASE_URL`, `MERCHANT_BASE_URL`, `PRICING_BASE_URL`, `PARCEL_BASE_URL` and timeouts                                                                            | Development accepts Compose service names                                    |
| Email          | `SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_FROM_NAME`, `SUPPORT_EMAIL`                                                                                    | Defaults to Mailpit; Gmail is opt-in                                         |
| Google OAuth   | `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, `GOOGLE_REDIRECT_URI`                                                                                                                         | Optional; the routes are not mounted until configured                        |
| Proof storage  | `STORAGE_PROVIDER`, `STORAGE_*`, `PROOF_STORAGE_DIR`, `PROOF_MAX_PER_PARCEL`, `PROOF_MAX_SIZE_BYTES`, `PROOF_PRESIGNED_URL_TTL_SECONDS`                                                   | Local filesystem is the development default                                  |
| Webhooks       | `WEBHOOK_SECRET_KEY`, `WEBHOOK_SECRET_KEY_ID`, `WEBHOOK_SECRET_KEYS`                                                                                                                      | 64 hex characters; keyring supports rotation                                 |
| AI             | `AI_ENABLED`, `AI_PROVIDER`, `GEMINI_API_KEY`, `AI_MODEL`, timeouts, rate and concurrency limits                                                                                          | Key is read only by `ai-assistant`                                           |
| Seeds          | `SEED_*` and `SEED_MERCHANT_*`                                                                                                                                                            | Passwords have no defaults                                                   |

`.env.example` documents every variable with safe placeholders. Never commit real
credentials, and never expose `GEMINI_API_KEY` through a `NEXT_PUBLIC_` name, the web
image, a build argument or source control.

Outside development, a configured `JWT_ACCESS_SECRET` must be at least 32 random
bytes and not a placeholder, and identity/pricing refuse to start unless
`INTERNAL_TOKEN` is at least 32 random bytes. In development, an unset or
shorter-than-16-character `INTERNAL_TOKEN` keeps internal endpoints fail-closed.

## 9. Docker Compose quick start

```sh
cp .env.example .env
# set at least APP_ENV=development and a real JWT_ACCESS_SECRET
make up-build
```

`make up-build` builds every image and then runs the startup workflow: validate
configuration, start PostgreSQL/RabbitMQ/Mailpit and wait for health, apply all five
service migrations, start the application images and wait for readiness, then verify
gateway routes with `scripts/check-routes.sh`.

Open:

| Endpoint                          | URL                                                         |
| --------------------------------- | ----------------------------------------------------------- |
| Application (through the gateway) | <http://localhost:8080/>                                    |
| Mailpit (captured email)          | <http://localhost:8025/>                                    |
| RabbitMQ management               | <http://localhost:15672/> (Compose credentials from `.env`) |
| Gateway health                    | <http://localhost:8080/healthz>                             |
| PostgreSQL                        | Internal only (`postgres:5432`)                             |
| Service and worker ports          | Internal only                                               |

Subsequent starts with existing images:

```sh
make up
```

After changing application source, rebuild:

```sh
make up-build
```

`make up` is safe to repeat: Compose is a no-op, migrations report no change, routes
are re-verified, and a missing image fails before startup with instructions.

### Common commands

| Command                                                                     | Purpose                                                                           |
| --------------------------------------------------------------------------- | --------------------------------------------------------------------------------- |
| `make help`                                                                 | List all targets                                                                  |
| `make build`                                                                | Build all application images without starting                                     |
| `make up`                                                                   | Start dependencies, migrate, start services, verify routes                        |
| `make up-build`                                                             | Build images, then run `make up`                                                  |
| `make down`                                                                 | Stop containers and keep volumes                                                  |
| `make logs`                                                                 | Follow logs                                                                       |
| `make migrate`                                                              | Apply all service migrations                                                      |
| `make migrate-status`                                                       | Print each migration version and fail on dirty state                              |
| `make migrate-create SERVICE=parcel NAME=description`                       | Create a migration pair                                                           |
| `make db-verify`                                                            | Read-only baseline verification                                                   |
| `make seed` / `make seed-merchant` / `make seed-parcels` / `make seed-demo` | Seed demo data                                                                    |
| `make check-routes`                                                         | Verify gateway routing against the running stack                                  |
| `make test`                                                                 | Go race tests                                                                     |
| `make test-db`                                                              | PostgreSQL integration tests                                                      |
| `make test-storage`                                                         | S3-compatible object-store integration tests                                      |
| `make test-access`                                                          | Cross-service access tests against the gateway                                    |
| `make check-openapi`                                                        | Contract and route-inventory validation                                           |
| `make db-reset`                                                             | Delete local volumes and rebuild (**destructive**, requires `CONFIRM_DB_RESET=1`) |

Compose namespaces resources by project name. To run a second checkout safely:

```sh
COMPOSE_PROJECT_NAME=swiftdrop-verify make up-build
COMPOSE_PROJECT_NAME=swiftdrop-verify make down
```

Choose different published ports in the second checkout's `.env` (for example
`NGINX_PORT=8180`, `RABBITMQ_PORT=5772`, `RABBITMQ_MANAGEMENT_PORT=15673`,
`MAILPIT_SMTP_PORT=1125`, `MAILPIT_UI_PORT=8125`).

## 10. Database migrations

Migrations live under `migrations/<service>/` and use
`NNNNNN_description.up.sql` / `.down.sql`. Each service has its own version table.
Applied migrations are immutable; fix mistakes with a new forward migration.

| Service      | Current head | Notes                                                                                    |
| ------------ | ------------ | ---------------------------------------------------------------------------------------- |
| Identity     | `000006`     | OAuth state binding, email verification, session version, merchant lifecycle             |
| Pricing      | `000002`     | Baseline tables and seeds                                                                |
| Parcel       | `000008`     | Dispatch, price breakdown, outbox types/claim, customer list index, proof kind           |
| Merchant     | `000011`     | Constraints, onboarding, registration-number compatibility, order snapshots, idempotency |
| Notification | `000003`     | Delivery-log `created_at` index                                                          |

The complete chain creates **31 tables**: the 26-table `docs/db.sql` baseline plus
five additive tables. See the addendum in
[`docs/Database_Design_Document_v1.1.md`](docs/Database_Design_Document_v1.1.md).

```sh
docker compose up -d postgres
make migrate
make migrate-status
make db-verify
```

`make migrate` runs the pinned `migrate/migrate:v4.18.3` image through
`scripts/migrate.sh`, bootstraps `public.set_updated_at()`, and applies identity →
pricing → parcel → merchant → notification. It never prints the connection URL.

`make db-verify` runs read-only checks through the Compose PostgreSQL service with
`default_transaction_read_only=on`. A development database that has extra
admin-created pricing data will correctly report a mismatch against the baseline
counts; verify against a freshly migrated database for release evidence.

Roll back one migration per service (disposable databases only):

```sh
make migrate-down
```

Never run `make migrate-down` or `make db-reset` against shared or production data.
Migrations are not applied automatically at service startup.

To rehearse backup, restore and migration rollback:

```sh
make db-rehearse-restore REHEARSAL_DATABASE_URL='postgres://.../disposable?sslmode=disable'
```

See [`docs/operations.md`](docs/operations.md) and
[`migrations/README.md`](migrations/README.md).

## 11. Demo data and seed instructions

Demo users are never created by migrations, Compose or service startup. Seeds are
operator-run, idempotent, and refuse `APP_ENV=production`.

```sh
export SEED_ADMIN_PASSWORD='<your-password>'
export SEED_DISPATCHER_PASSWORD='<your-password>'
export SEED_COURIER_1_PASSWORD='<your-password>'
export SEED_COURIER_2_PASSWORD='<your-password>'
export SEED_CUSTOMER_PASSWORD='<your-password>'
export SEED_MERCHANT_PASSWORD='<your-password>'

make seed            # five demo users + courier profiles
make seed-merchant   # approved merchant owner and merchant record
make seed-parcels    # one parcel per canonical status
make seed-demo       # all of the above through the Compose network
```

`make seed-demo` can also create a known merchant API key:

```sh
export SEED_MERCHANT_API_KEY="$(printf 'sdk_live_%s_%s' "$(openssl rand -hex 8)" "$(openssl rand -hex 24)")"
make seed-demo
```

Only the key hash is stored; the full value is the one you supplied. The email
addresses in `.env.example` use `example.com` and are placeholders, not mailboxes.
Seeds require a reachable database and, for the merchant path, a reachable Identity
service. Verify that any `DATABASE_URL` points at a disposable database before
seeding.

## 12. Application access and service endpoints

Everything a browser uses goes through Nginx on `http://localhost:8080`. Nginx strips
`/api/v1` before forwarding, so Gin registers root paths such as `/auth/...`,
`/parcels/...`, `/health` and `/ready`.

| Public prefix                                                                                                            | Upstream service                  |
| ------------------------------------------------------------------------------------------------------------------------ | --------------------------------- |
| `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers`, `/api/v1/admin/audit-logs/identity`          | identity                          |
| `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports` | parcel                            |
| `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/admin/pricing`, `/api/v1/admin/zones`, `/api/v1/admin/audit-logs/pricing`   | pricing                           |
| `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants`, `/api/v1/admin/audit-logs/merchant`                  | merchant                          |
| `/api/v1/admin/notification-logs`                                                                                        | notification-worker               |
| `/api/v1/ai`                                                                                                             | ai-assistant                      |
| `/ws`                                                                                                                    | realtime                          |
| `/healthz`                                                                                                               | gateway only                      |
| `/internal`                                                                                                              | blocked at the gateway (JSON 404) |

Health probes are internal:

```sh
docker compose exec identity wget -qO- http://127.0.0.1:8081/ready
docker compose exec parcel wget -qO- http://127.0.0.1:8082/health
```

`/health` is liveness. `/ready` reports required dependencies; for example, realtime
requires PostgreSQL and both consumers, the notification worker requires PostgreSQL
and all three consumers, and `ai-assistant` returns `503` when its provider is
disabled or unconfigured. `/healthz` and `/readyz` remain deprecated aliases.

The browser-only Next.js development server runs on `http://localhost:3000` and
proxies `/api/v1` to the gateway at `http://127.0.0.1:8080` (`API_PROXY_TARGET`
overrides it). `/ws` is not proxied by Next, so for `nx dev` copy
`apps/web/.env.example` to the untracked `apps/web/.env.local`.

## 13. User roles and demo accounts

| Role         | Main capabilities                                                                                                     |
| ------------ | --------------------------------------------------------------------------------------------------------------------- |
| `CUSTOMER`   | Register/verify, estimate, create and track own parcels, download receipts, edit profile, use private AI parcel tools |
| `MERCHANT`   | Approved merchant portal, orders, API keys, webhooks, receipts, integration guide                                     |
| `COURIER`    | Today list, delivery history, pickups, out-for-delivery, delivery with proof, failures                                |
| `DISPATCHER` | All parcels, unassigned queue, assignment/reassignment, reports, live updates, operational AI access                  |
| `ADMIN`      | Users, merchants, pricing, notification logs, audit logs, reports, operational AI access                              |

Demo accounts created by `make seed` / `make seed-demo`:

| Role       | Email (placeholder)           | Password                   |
| ---------- | ----------------------------- | -------------------------- |
| ADMIN      | `seed.admin@example.com`      | `SEED_ADMIN_PASSWORD`      |
| DISPATCHER | `seed.dispatcher@example.com` | `SEED_DISPATCHER_PASSWORD` |
| COURIER 1  | `seed.courier.1@example.com`  | `SEED_COURIER_1_PASSWORD`  |
| COURIER 2  | `seed.courier.2@example.com`  | `SEED_COURIER_2_PASSWORD`  |
| CUSTOMER   | `seed.customer@example.com`   | `SEED_CUSTOMER_PASSWORD`   |
| MERCHANT   | `seed.merchant@example.com`   | `SEED_MERCHANT_PASSWORD`   |

Passwords have no default and are never committed. You can also register a customer
at `/register/customer`; merchants apply at `/register/merchant` and cannot sign in
until an administrator approves them at `/admin/merchants`.

A development login example:

```sh
curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -H 'X-Token-Mode: body' \
  -d '{"email":"seed.customer@example.com","password":"<your-password>"}'
```

Five wrong passwords lock the account for 15 minutes. Login and register are limited
to 10 requests per minute per client address.

## 14. API documentation

- Product specification: [`docs/API_Documentation_v1.1.md`](docs/API_Documentation_v1.1.md)
- Machine-readable contracts: [`contracts/openapi`](contracts/openapi)
- Event schemas and conventions: [`contracts/events`](contracts/events)
- Runtime route inventories: [`contracts/routes`](contracts/routes)
- Merchant integration guide: [`docs/merchant-api-guide.html`](docs/merchant-api-guide.html)
  and [`docs/merchant-api-guide.pdf`](docs/merchant-api-guide.pdf)
- Merchant API guide source: `make merchant-api-pdf`

Errors use the API v1.1 envelope and never expose stack traces, SQL or credentials:

```json
{
  "error": { "code": "NOT_FOUND", "message": "route not found" },
  "meta": { "requestId": "..." }
}
```

Status codes follow REST conventions: `200/201/204` success, `400` malformed input,
`401/403` authentication/authorization, `404` missing, `409` conflict, `422` semantic
validation and `500` unexpected failure. Unmatched routes return JSON `NOT_FOUND`;
disallowed methods return `METHOD_NOT_ALLOWED` with an `Allow` header.

Most browser routes authenticate with the `sd_access` HttpOnly cookie (15 minutes)
and rotate the `sd_refresh` cookie (7 days). Merchant order creation (API 8.16) is
authenticated only by `X-API-Key`; the portal never handles a merchant key. Internal
service-to-service routes require `X-Internal-Token` and, for merchant internal
routes, a well-formed `X-Request-Id`, and are never proxied by Nginx.

Validate contracts and route coverage:

```sh
make check-openapi
```

After an intentional route change, regenerate the inventories and review the diff
with the OpenAPI and Nginx changes:

```sh
UPDATE_ROUTES=1 go test -run TestRouteInventory ./apps/services/...
```

## 15. Event-driven architecture

The durable topic exchange is **`swiftdrop.events`**. Event names are versioned as
`<domain>.<past_tense_action>.v<major>`.

| Event                      | Producers           | Consumers                                                                       |
| -------------------------- | ------------------- | ------------------------------------------------------------------------------- |
| `parcel.created.v1`        | Parcel outbox relay | notification-worker                                                             |
| `parcel.assigned.v1`       | Parcel outbox relay | realtime                                                                        |
| `parcel.status_changed.v1` | Parcel outbox relay | notification-worker, webhook-worker, realtime, merchant (`merchant.sync.queue`) |

Durable queues include `notification.requested.v1` (legacy work item),
`notification.parcel.created.v1`, `notification.parcel.status_changed.v1`,
`webhook.parcel.status_changed.v1`, `realtime.parcel.status_changed.v1`,
`realtime.parcel.assigned.v1` and `merchant.sync.queue`.

- **Transactional outbox.** Parcel creation, assignment and status writes store a
  PENDING outbox row in the same transaction as the business change. The relay claims
  rows atomically (`FOR UPDATE SKIP LOCKED`, `claim_token`/`claimed_at`) before
  publishing with publisher confirms, so replicas do not publish the same row.
  Publication is at-least-once; consumers are idempotent.
- **Retries and dead letters.** Resources are derived from the exchange and queue:
  `<exchange>.retry`, `<queue>.retry.v2`, `<exchange>.dlx`, `<queue>.dlq`. Failures
  are classified as permanent or retryable; retryable failures are retried up to
  `RABBITMQ_MAX_RETRIES` (default 3) with `RABBITMQ_RETRY_DELAY` (default 30s), then
  dead-lettered. Malformed or unsupported messages are dead-lettered immediately.
- **Framing and schemas.** Message bodies are limited to 1 MiB and must be absent or
  `application/json`. Envelopes and the full event payload are validated against the
  embedded JSON Schemas before handling.
- **Merchant webhooks.** The webhook worker delivers the API v1.1 public projection
  with HMAC signing, records attempts, and schedules the 1m/5m/30m/2h/6h business
  retries. Suspended, pending or rejected merchants and inactive webhooks are
  suppressed before the HTTP request.
- **Realtime.** Status events project only public tracking fields. Assignment notices
  reach dispatcher/administrator `scope: all` clients only and tell them to refetch
  authoritative REST state.

See [`contracts/events/README.md`](contracts/events/README.md) and
[`docs/architecture.md`](docs/architecture.md).

## 16. AI assistant setup and usage

The Delivery Assistant is part of the release, not an optional extra branch. It is a
floating chat widget on every page and a Go service at `apps/services/ai-assistant`.
The browser calls only `POST /api/v1/ai/chat` through Nginx; the Gemini key exists
only in the `ai-assistant` container.

### Live Gemini

```sh
# in .env
AI_ENABLED=true
AI_PROVIDER=gemini
GEMINI_API_KEY=your-key
AI_MODEL=gemini-3.5-flash
```

```sh
make up-build
curl -s http://localhost:8080/api/v1/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"What is SwiftDrop?","conversationId":null}'
```

### Deterministic local provider

```sh
AI_PROVIDER=mock
GEMINI_API_KEY=
```

`mock` is development-only. A missing or invalid Gemini key does not stop the
gateway: the assistant returns `503 AI_UNAVAILABLE` with a safe message.

### Permissions and tools

| Caller                | Tool access                                                                   |
| --------------------- | ----------------------------------------------------------------------------- |
| Guest                 | No private tools; approved knowledge only; HttpOnly `sd_ai_guest` session     |
| `CUSTOMER`            | Own parcel status/history/active deliveries, including tracking-number lookup |
| `DISPATCHER`, `ADMIN` | Operational parcel access                                                     |
| `MERCHANT`, `COURIER` | No parcel tools                                                               |

Exactly four read-only tools exist: `calculate_delivery_fee`,
`get_public_parcel_status` (ownership-scoped; despite the name it never uses the
anonymous public endpoint), `get_my_parcel_history` and `get_my_active_deliveries`.
The assistant never accepts identity or role from the model, never accesses the
database, exposes no write tools, and treats a tracking number as data — not as an
authentication credential.

Conversation history, rate limiting (15/minute) and concurrency limits are
process-local and non-durable. Multi-replica coordination is future work. See
[`docs/ai-assistant.md`](docs/ai-assistant.md),
[`docs/ai-release-checklist.md`](docs/ai-release-checklist.md) and the service's
[`IMPLEMENTATION_CONTRACT.md`](apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md).

## 17. Testing and verification

Run checks from the repository root so the single Go module and shared packages
resolve.

### Backend

```sh
gofmt -l apps/services packages/go-common tests/integration
go vet ./...
go test -race -count=1 ./...
go build ./...
```

`make test` runs the race suite. Unit tests do not require PostgreSQL or RabbitMQ.

### PostgreSQL integration

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  make test-db
```

`TEST_DATABASE_URL` must allow `CREATE DATABASE`; tests create and drop uniquely named
databases and never touch application data. The suite covers migrations
(up/down/reapply), pricing HTTP, notification idempotency/retention, the
notification-log repository, demo seeds, merchant HTTP/repository and the Parcel
PostgreSQL tests. The Parcel tests require the `migrate` CLI on `PATH`.

### RabbitMQ integration and event flow

```sh
docker compose up -d rabbitmq
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -tags=integration ./tests/integration
```

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -race -count=1 -tags=integration ./tests/integration/eventflow/...
```

The event-flow suite writes real outbox rows, runs the real relay and AMQP publisher,
and processes events with the real notification, webhook, realtime and merchant-sync
handlers. CI fails on skipped integration tests.

### Frontend

```sh
pnpm exec nx test @org/web
pnpm exec nx typecheck @org/web
pnpm exec nx lint @org/web
pnpm exec nx build @org/web
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e
pnpm exec playwright install chromium
pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"
```

The Playwright config also defines Firefox and WebKit for local use; CI runs Chromium
only. Mocked journeys use the production Next build on purpose, matching CI and the
deployed application. Real-stack journeys (`*.real.spec.ts`) run against Compose and
need the seeded accounts through `REAL_STACK_*` variables; see
`apps/web-e2e` and [`CONTRIBUTING.md`](CONTRIBUTING.md).

### Contracts, design lint and schemas

```sh
make check-openapi
node --test "tests/contracts/*.test.mjs"
node --test "apps/web/eslint-rules/*.test.mjs"
node --test "apps/web/src/lib/*.test.mjs"
pnpm exec prettier --check .
```

### Security and infrastructure

```sh
go run golang.org/x/vuln/cmd/govulncheck@v1.9.0 ./...
pnpm audit --prod --audit-level=high
docker compose config --quiet
make test-access          # against a running Compose gateway
make test-storage         # when the local S3-compatible service is running
```

CI runs gofmt, `go vet`, the race suite with coverage, build, Compose validation, Go
vulnerability scanning, RabbitMQ/PostgreSQL/event-flow integration, frontend
Prettier/lint/test/typecheck/build, Chromium Playwright, Gitleaks, CodeQL, Trivy image
scans and SBOM generation, access tests, migration validation, object-storage tests,
pooled PostgreSQL, real-stack onboarding, notification-worker E2E and a release smoke
on `v*`/`milestone-*` tags. `Required CI gate` and `Security gate` are the required
status checks. See the final acceptance evidence in
[`docs/implementation/FINAL_ACCEPTANCE_REPORT.md`](docs/implementation/FINAL_ACCEPTANCE_REPORT.md),
[`docs/implementation/FINAL_TEST_REPORT.md`](docs/implementation/FINAL_TEST_REPORT.md)
and [`docs/implementation/SECURITY_REVIEW.md`](docs/implementation/SECURITY_REVIEW.md).

A check is evidence only when it was actually run. The final release report records
what ran, what passed, and any environment limitation.

## 18. Security considerations

- **Authentication.** Passwords are bcrypt-hashed; access and refresh tokens are
  validated for signature, allowed algorithm, expiry and issuer. Refresh tokens
  rotate and reuse is detected. Merchants must be approved to obtain a session.
- **Authorization.** The backend enforces roles and ownership on every protected
  route. Client-side guards are convenience only. Cross-customer reads return `404`.
- **Session revocation.** The `sv` claim is checked against Identity; suspension and
  re-approval invalidate old access tokens, and a password change revokes every
  refresh token. Checks fail closed when Identity is unavailable.
- **Secrets.** `.env` is untracked; `.env.example` contains placeholders only.
  Webhook signing secrets use AES-256-GCM with AAD binding and a rotation keyring.
  Secrets, tokens and passwords never appear in logs or error responses; request
  query strings are redacted at the gateway.
- **CSRF and headers.** State-changing cross-site requests are rejected by origin
  checks. Nginx adds a CSP and security headers; the script/style `'unsafe-inline'`
  allowances are required by Next hydration and React style attributes.
- **Limits.** HTTP headers are limited to 1 MiB, request and upload sizes are bounded,
  photos are validated as decoded JPEG/PNG up to 5 MiB, and login, registration,
  parcel creation, proof upload, webhook tests and AI chat are rate limited.
- **Private proofs.** Proof photos are stored outside the database and served through
  short-lived presigned URLs or an authorized stream. A committed proof always
  references an object uploaded before commit; `proof-reconcile` reports orphans and
  missing objects.
- **Internal endpoints.** `/internal` is never routed publicly. Shared
  `INTERNAL_TOKEN` routes fail closed when the token is unset; merchant internal
  routes also require `X-Request-Id`.
- **Retention.** Notification delivery logs are retained 180 days, processed events
  30 days, audit logs 365 days and webhook attempts 90 days by default; merchant order
  reservations are retained indefinitely. See [`docs/data-retention.md`](docs/data-retention.md).
- **Supply chain.** CI runs `govulncheck`, `pnpm audit`, Gitleaks, CodeQL, Trivy and
  Syft. Dependencies are updated manually; Dependabot is disabled.
- **Known limitations.** No production TLS, Kubernetes, Terraform or shared
  rate-limit/conversation state. SMS is not implemented. AI conversation state is
  process-local. Dispatcher-initiated status writes are outside API v1.1. Local proof
  storage requires a single Parcel replica or a shared volume. See
  [Known limitations](#known-limitations).

## 19. Troubleshooting

| Symptom                              | Fix                                                                                                                       |
| ------------------------------------ | ------------------------------------------------------------------------------------------------------------------------- |
| Port already in use                  | Change `NGINX_PORT`, `RABBITMQ_PORT`, `RABBITMQ_MANAGEMENT_PORT`, `MAILPIT_SMTP_PORT` or `MAILPIT_UI_PORT` in `.env`      |
| `make up` fails preflight            | Ensure `.env` exists with `APP_ENV` and a non-empty `JWT_ACCESS_SECRET`                                                   |
| Host process cannot see variables    | Go services do not load `.env`; run `set -a; . ./.env; set +a` first                                                      |
| Nginx returns 502 or will not start  | Nginx resolves upstreams at startup; run `make up` for the full stack, then check `docker compose ps`                     |
| `make up` starts old code            | Rebuild with `make up-build`                                                                                              |
| Migrations cannot connect            | `docker compose up -d postgres`; confirm `POSTGRES_USER`, `POSTGRES_PASSWORD` and `POSTGRES_DB`; use URL-safe credentials |
| Migration duplicate version          | Never renumber an applied migration; add a forward migration                                                              |
| Worker keeps reconnecting            | Check RabbitMQ health and `RABBITMQ_URL`; use the `rabbitmq` service name inside Compose                                  |
| Integration tests skip               | Set `RABBITMQ_URL`; CI treats skips as failures                                                                           |
| Playwright cannot launch Chromium    | `pnpm exec playwright install chromium` (add `--with-deps` on Linux)                                                      |
| AI chat returns `503 AI_UNAVAILABLE` | Check `AI_ENABLED`, `AI_PROVIDER` and `GEMINI_API_KEY`; use `mock` for local tests                                        |
| Email does not arrive                | Open Mailpit at <http://localhost:8025>; confirm `SMTP_HOST=mailpit` and `SMTP_PORT=1025`                                 |
| db-verify reports extra pricing rows | The development database has extra admin-created data; verify against a freshly migrated database                         |
| Docker build runs out of memory      | Build one service at a time (`docker compose build parcel`) then `make up`; do not run global Docker cleanup              |
| Formatting check fails               | `gofmt -w` changed Go files and `pnpm exec prettier --write` changed docs/frontend files                                  |

More procedures are in [`docs/operations.md`](docs/operations.md).

## 20. Team members and contributions

SwiftDrop was built by a ten-person team. The areas below summarize each
contributor's primary work, derived from repository history and
[`docs/implementation/MEMBER_WISE_STATUS.md`](docs/implementation/MEMBER_WISE_STATUS.md).

| Member | Name                 | Primary contributions                                                                                                       |
| ------ | -------------------- | --------------------------------------------------------------------------------------------------------------------------- |
| M1     | Vineth Ranathunga    | Platform foundation, notification worker, merchant service, parcel service, web application, Docker/CI, release integration |
| M2     | Nihan Kuruppu        | Identity service, authentication screens, email templates, web application and E2E                                          |
| M3     | Manuthi Perera       | Identity, Google OAuth, password reset, profile and admin flows                                                             |
| M4     | Thisal Kokuhennadi   | Pricing service, zones, public tracking and booking fee flow                                                                |
| M5     | Supesh Asalya        | Customer parcel creation, pricing integration, receipts, M5 database verification                                           |
| M6     | Achira Sandaruwan    | Dispatch assignment, parcel state machine, outbox relay and realtime assignment                                             |
| M7     | Nawamini Keragala    | Courier delivery reads, status updates, proof upload/storage and courier screens                                            |
| M8     | Rangana Amarathunge  | Merchant integration, merchant orders, snapshots and merchant portal                                                        |
| M9     | Minimuthu Thennakoon | Webhook worker, signed delivery, retries and webhook settings                                                               |
| M10    | Sandunike Kulawala   | Reports, realtime WebSocket, live-update hooks and realtime verification                                                    |

Cross-cutting work (CI, security hardening, contracts, integration suites, final
acceptance) was shared across the team; the member mapping above reflects each
member's primary ownership, not exclusive contribution.

## Known limitations

These are deliberate boundaries, not hidden gaps:

- No production TLS termination, Kubernetes, Terraform or cloud deployment
  automation. Compose is the supported local and CI runtime.
- No SMS delivery. SMS is a documented permanent failure for the notification worker.
- Realtime has no SSE, persistence/replay or multi-replica fan-out; replicas sharing
  one queue each receive a subset of events.
- AI conversation storage, rate limiting and concurrency are process-local.
- Shared distributed rate limiting and a Prometheus/Grafana deployment are deferred.
- Dispatcher-initiated parcel status writes and customer edit/cancel are outside API
  v1.1.
- Payment processing and third-party carrier integrations are not implemented.
- Proof storage on independent local volumes across multiple Parcel replicas is
  unsafe; use one replica, a shared volume, or object storage.
- The academic report has outstanding source material: Section 6.1 and Section 8
  headings and Figures 8/9 have no source in `docs/report` or `docs/diagrams`. The
  report owner must supply them; the repository does not invent report content.

## Further documentation

| Document                                                                                           | Purpose                                                |
| -------------------------------------------------------------------------------------------------- | ------------------------------------------------------ |
| [`AGENTS.md`](AGENTS.md)                                                                           | Repository rules for human and AI contributors         |
| [`DESIGN.md`](DESIGN.md)                                                                           | Frontend design contract; read before changing screens |
| [`docs/architecture.md`](docs/architecture.md)                                                     | Service boundaries, routing, messaging, readiness      |
| [`docs/API_Documentation_v1.1.md`](docs/API_Documentation_v1.1.md)                                 | Product API specification                              |
| [`docs/Database_Design_Document_v1.1.md`](docs/Database_Design_Document_v1.1.md)                   | Logical database design and final migration addendum   |
| [`docs/operations.md`](docs/operations.md)                                                         | Operational runbook                                    |
| [`docs/neon-production.md`](docs/neon-production.md)                                               | Neon PostgreSQL, pool budgets and cutover              |
| [`docs/data-retention.md`](docs/data-retention.md)                                                 | Retention policy and cleanup jobs                      |
| [`docs/delivery-proof-storage.md`](docs/delivery-proof-storage.md)                                 | Proof storage and reconciliation                       |
| [`docs/security-scanning.md`](docs/security-scanning.md)                                           | Scanners, thresholds and exceptions                    |
| [`docs/email.md`](docs/email.md)                                                                   | Branded email, templates and previews                  |
| [`docs/merchant-integration.md`](docs/merchant-integration.md)                                     | Merchant order and integration flows                   |
| [`docs/merchant-internal-api.md`](docs/merchant-internal-api.md)                                   | Merchant internal API                                  |
| [`docs/identity-internal-api.md`](docs/identity-internal-api.md)                                   | Identity internal API                                  |
| [`docs/webhook-worker.md`](docs/webhook-worker.md)                                                 | Webhook worker flow                                    |
| [`docs/realtime-verify.md`](docs/realtime-verify.md)                                               | WebSocket verification                                 |
| [`docs/phase-2-security-reliability.md`](docs/phase-2-security-reliability.md)                     | Phase 2 hardening record                               |
| [`docs/phase-3-testing-documentation-qa.md`](docs/phase-3-testing-documentation-qa.md)             | Phase 3 QA record                                      |
| [`docs/phase-4-completion-report.md`](docs/phase-4-completion-report.md)                           | Phase 4 completion record                              |
| [`docs/implementation/FINAL_ACCEPTANCE_REPORT.md`](docs/implementation/FINAL_ACCEPTANCE_REPORT.md) | Final acceptance evidence                              |
| [`docs/implementation/FINAL_TEST_REPORT.md`](docs/implementation/FINAL_TEST_REPORT.md)             | Final test evidence                                    |
| [`docs/implementation/SECURITY_REVIEW.md`](docs/implementation/SECURITY_REVIEW.md)                 | Security review                                        |
| [`docs/report`](docs/report)                                                                       | Academic report sources and editable diagrams          |
| [`CONTRIBUTING.md`](CONTRIBUTING.md)                                                               | Branch, pull request and review workflow               |
| [`contracts/events/README.md`](contracts/events/README.md)                                         | Event naming, envelope and failure rules               |
| [`contracts/openapi/README.md`](contracts/openapi/README.md)                                       | HTTP contracts and error behavior                      |
| [`contracts/routes/README.md`](contracts/routes/README.md)                                         | Route inventory regeneration                           |
| [`migrations/README.md`](migrations/README.md)                                                     | Migration conventions and commands                     |
| [`infra/nginx/README.md`](infra/nginx/README.md)                                                   | Local edge routing                                     |
| [`tests/integration/README.md`](tests/integration/README.md)                                       | Integration test setup                                 |
