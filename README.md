# SwiftDrop

SwiftDrop is a parcel delivery and courier management platform. This repository is a production-oriented foundation: an Nx monorepo with a Next.js frontend, independently runnable Go HTTP services, RabbitMQ workers, a shared Go infrastructure package, PostgreSQL migrations, event and API contracts, Nginx routing, local Docker Compose infrastructure, and GitHub Actions CI.

Identity authentication, pricing estimates/zones/internal quote and admin configuration, anonymous public parcel tracking, DISPATCHER/ADMIN parcel reports and the reports screen, and ADMIN notification-log reads and UI are implemented. Customer parcel creation, owned reads, internal merchant creation/summaries and customer screens are implemented. Courier/dispatch status writes, merchant onboarding, SMS notification delivery, webhook delivery and realtime subscriptions remain planned. See [Current scope](#current-scope), [`docs/pricing-and-tracking.md`](docs/pricing-and-tracking.md) and [`docs/architecture.md`](docs/architecture.md).

## Technology

- **Frontend:** Next.js 16 App Router, React 19, TypeScript 6, pnpm, Nx, Tailwind CSS v4, shadcn/ui with Radix primitives, and Lucide icons.
- **Backend:** Go 1.27.3 with Gin, one root Go module.
- **Data:** PostgreSQL 16 with service-owned schemas and versioned SQL migrations.
- **Messaging:** RabbitMQ 4.1 with a durable topic exchange, versioned JSON event contracts, bounded retries, and dead-lettering.
- **Edge:** Nginx reverse proxy.
- **Local orchestration:** Docker Compose.
- **CI:** GitHub Actions.

## Architecture

| Service               | Responsibility                                                                                      | Internal port |
| --------------------- | --------------------------------------------------------------------------------------------------- | ------------- |
| `identity`            | Authentication, admin users, couriers, internal lookups                                             | 8081          |
| `parcel`              | Anonymous tracking and DISPATCHER/ADMIN reports; writes planned                                     | 8082          |
| `pricing`             | Delivery quotes, zones, and ADMIN pricing configuration                                             | 8083          |
| `merchant`            | Future merchant accounts, settings, and API credentials                                             | 8084          |
| `realtime`            | Future live tracking over WebSocket or Server-Sent Events                                           | 8085          |
| `notification-worker` | Notification delivery (legacy + customer parcel domain events), email, ADMIN notification-log reads | 8086          |
| `webhook-worker`      | Future outgoing webhook delivery                                                                    | no HTTP port  |
| `@org/web`            | Next.js frontend                                                                                    | 3000          |
| Nginx                 | Public HTTP entry point                                                                             | 8080          |

Application access is through the gateway (`http://localhost:8080`). The Compose file also publishes development and infrastructure ports: RabbitMQ AMQP on `5672` and its management UI on `15672`, and Mailpit SMTP on `1025` and its web UI on `8025`. The service ports in the table above are internal to the Compose network and are not published to the host. Override any published port with the matching `.env` variable (`NGINX_PORT`, `RABBITMQ_PORT`, `RABBITMQ_MANAGEMENT_PORT`, `MAILPIT_SMTP_PORT`, `MAILPIT_UI_PORT`). Services communicate synchronously over HTTP when a caller needs an immediate result. Asynchronous work uses RabbitMQ. Each persisting service owns its PostgreSQL schema and must not modify another service's tables. Realtime currently has no schema or persistence. Read [`docs/architecture.md`](docs/architecture.md) for service boundaries, routing, event conventions, lifecycle behavior, and readiness semantics; this README does not repeat the full architecture.

## Repository structure

```text
apps/
  web/                      Next.js App Router application
  web-e2e/                  Playwright end-to-end tests
  services/
    identity/
    parcel/
    pricing/
    merchant/
    notification-worker/
    webhook-worker/
    realtime/
packages/
  go-common/                Shared Go infrastructure
contracts/
  openapi/                  Per-service OpenAPI specifications
  events/                   Versioned event contracts and conventions
migrations/                 Service-owned SQL migrations
infra/
  docker/                   Go and Next.js Dockerfiles
  nginx/                    Local reverse proxy configuration
docs/                       Architecture documentation
tests/integration/          RabbitMQ and PostgreSQL integration tests
.github/workflows/          CI
```

Each Go service has `main.go`, `internal/config`, and either `internal/http` or `internal/worker`, with tests beside the code.

## Prerequisites

- Go **1.27.3** or a compatible 1.27 toolchain (`go.mod` is the source of truth).
- Node.js **24**, the supported and CI-tested version. Other Node.js versions are not officially supported.
- pnpm **12.15.0** through Corepack.
- Docker with Docker Compose v2.
- `curl`, used by the gateway route verification.
- Optional: the `golang-migrate` CLI for database migrations.
- Optional: `psql` to inspect the local database.
- Optional: Chromium if you run Playwright E2E tests.

```sh
corepack enable
corepack prepare pnpm@12.15.0 --activate
```

`make` drives the database helpers and local demo seed (`make migrate`, `make migrate-down`, `make db-verify`, `make db-reset`, `make seed`, `make check-routes`). Nx and Go commands remain available for everything else.

## Install and configure

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
pnpm install --frozen-lockfile
```

`.env.example` contains safe local placeholders only. Set `APP_ENV` explicitly to `development`, `staging`, or `production`; all backend services and workers refuse to start without it. Do not commit `.env`, credentials, private keys, or production secrets. Docker Compose reads `.env` automatically. Go services read process environment variables directly and do not load `.env`; export it before running a service or worker on the host.

The template documents every variable currently used: PostgreSQL, RabbitMQ, gateway, Mailpit, Gmail SMTP (`SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_FROM_NAME`), pool overrides, and the M4 pricing/tracking settings (`HTTP_TRUSTED_PROXIES` for Pricing and Parcel, plus `INTERNAL_TOKEN`, `JWT_ACCESS_SECRET` and `COOKIE_NAME` for Pricing). It also includes identity settings (`JWT_REFRESH_SECRET`, token TTLs, cookie names and flags, Google OAuth client settings, `FRONTEND_BASE_URL`) and the notification worker's identity contact lookup (`IDENTITY_BASE_URL`, `IDENTITY_TIMEOUT`, `INTERNAL_TOKEN`), email links/footer (`FRONTEND_BASE_URL`, `SUPPORT_EMAIL`) and legacy plus parcel domain-event queues (`NOTIFICATION_QUEUE`/`NOTIFICATION_ROUTING_KEY`, `NOTIFICATION_PARCEL_CREATED_*`, `NOTIFICATION_PARCEL_STATUS_*`). In development, an unset or shorter-than-16-character `INTERNAL_TOKEN` keeps the internal endpoints fail-closed. Outside development, identity and pricing refuse to start unless the token is at least 32 random bytes and is not a placeholder. `JWT_ACCESS_SECRET` must be at least 32 random bytes and not a placeholder outside development; an unset value leaves pricing, parcel and notification-worker authenticated routes fail-closed with `503`. Never commit real credentials.

```sh
set -a
. ./.env
set +a
```

### Run a second clone safely

Compose namespaces containers, networks, and volumes by project name. The default project name is `swiftdrop`; set `COMPOSE_PROJECT_NAME` to give a second checkout its own resources:

```sh
COMPOSE_PROJECT_NAME=swiftdrop-verify make up-build
COMPOSE_PROJECT_NAME=swiftdrop-verify make down
```

Use `make project-name` to print the resolved project name. Compose resources are prefixed with it (for example `swiftdrop-verify-postgres-1` and `swiftdrop-verify_postgres-data`). Project isolation does not change published host ports, so if two stacks run at the same time, choose different ports in the second clone's `.env` (or export them), for example `NGINX_PORT=8180`, `RABBITMQ_PORT=5772`, `RABBITMQ_MANAGEMENT_PORT=15673`, `MAILPIT_SMTP_PORT=1125`, and `MAILPIT_UI_PORT=8125`. `make down` keeps that project's volumes; only `make db-reset` deletes them, and only for the selected project.

The integration branch is `development`. Create feature branches from it and open pull requests into `development`. Do not push directly to `development` or `main`. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the branch, pull request, review, and merge workflow.

## Quick start

Prerequisites: Docker with Compose v2, `make`, and `curl`. Go, Node.js, pnpm, and Playwright are only needed for host-side development and tests.

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
# edit .env only if your local configuration differs
make up-build
```

`make up-build` builds all images and then runs the full startup workflow. `make up`:

1. validates configuration (`preflight`);
2. starts the existing images and waits for health checks;
3. applies all five service migrations;
4. runs `scripts/check-routes.sh` against the actual published Nginx port.

Open <http://localhost:8080/>. Mailpit is at <http://localhost:8025/> and RabbitMQ management at <http://localhost:15672/>. The first build can take several minutes.

**Subsequent startups:** use `make up`. It starts existing images with `--no-build` and does not rebuild them.

**After changing application source code:** use `make up-build` so the images are rebuilt before startup.

Running `make up` again is safe: Compose is a no-op, migrations report `no change`, and routes are re-verified. If an application image is missing, `make up` fails before starting and tells you to run `make build` or `make up-build`.

If `.env` is missing and required variables are not exported, or `JWT_ACCESS_SECRET` is empty, `make up` fails during preflight with a clear error instead of waiting for unhealthy containers.

### Command reference

| Command                        | What it does                                                     |
| ------------------------------ | ---------------------------------------------------------------- |
| `make build`                   | Build all application images; does not start containers          |
| `make up`                      | Start existing images, wait for health, migrate, verify routes   |
| `make up-build`                | Build images, then run the full `make up` workflow               |
| `docker compose up --build -d` | Start only; **builds but does not migrate**                      |
| `make migrate`                 | Apply all service migrations                                     |
| `make db-verify`               | Verify the documented database baseline (read-only)              |
| `make project-name`            | Print the resolved Compose project name                          |
| `make seed`                    | Create the five deterministic demo users                         |
| `make migrate-down`            | Roll back **one migration per service** (destructive; see below) |
| `make check-routes`            | Verify gateway routing against the running stack                 |
| `make logs`                    | Follow logs                                                      |
| `make down`                    | Stop containers; keeps volumes and data                          |
| `make db-reset`                | Delete local volumes and rebuild (destructive)                   |

Start only what you need:

```sh
docker compose up -d postgres        # database and migrations
docker compose up -d rabbitmq        # workers and broker-backed integration tests
docker compose up -d mailpit         # local email UI
```

PostgreSQL is internal to the Compose network and does not publish a host port. Containers connect to it at `postgres:5432`; Compose builds that URL from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB`, so services and `make migrate` never use the `localhost` URL. The `DATABASE_URL` in `.env.example` is a **host-side** default for tools such as `psql`, `go run`, and `make seed`. It points at `localhost:5432` and works only when a PostgreSQL instance is reachable there (for example a separate local installation). It cannot reach the Compose database. PostgreSQL integration tests use a separate `TEST_DATABASE_URL` instead; see [PostgreSQL integration tests](#postgresql-integration-tests) below. Do not use `localhost` from inside a container, and do not add a host port to `docker-compose.yml` just for local tests.

## Sending email

Mailpit is the local email sink and is always available at <http://localhost:8025> (SMTP on `1025`). `.env.example` defaults to Mailpit, so no credentials are required for local email.

### Gmail SMTP (opt-in)

1. Enable 2-Step Verification on the Google account.
2. Create an App Password at Google Account → Security → App Passwords (choose "Mail"). Google shows a 16-character password; the normal account password will not work.
3. Create `.env` from the template and set your own credentials in the untracked file:
   ```sh
   cp .env.example .env
   # edit .env and uncomment the Gmail block:
   # SMTP_HOST=smtp.gmail.com
   # SMTP_PORT=587
   # SMTP_USER=your-account@gmail.com
   # SMTP_PASSWORD=your-16-character-app-password
   # SMTP_FROM=your-account@gmail.com
   # SMTP_FROM_NAME=Swift Drop Support
   ```
4. Recreate the services that receive mail configuration:
   ```sh
   docker compose up -d --force-recreate identity notification-worker
   ```

Gmail settings used by the template:

| Setting   | Value                            |
| --------- | -------------------------------- |
| Host      | `smtp.gmail.com`                 |
| Port      | `587` (STARTTLS)                 |
| Username  | the Gmail address                |
| Password  | 16-character Google App Password |
| From      | the same Gmail address           |
| From name | `Swift Drop Support`             |

Mailpit publishes `${MAILPIT_SMTP_PORT:-1025}` and its UI on `8025`. To switch back from Gmail, set `SMTP_HOST=mailpit`, `SMTP_PORT=1025`, and leave `SMTP_USER`/`SMTP_PASSWORD` empty.

**Security and scope**

- The App Password belongs only in `.env`, which is untracked. Never commit it, paste it into chat, or log it. Revoke it from the Google account if exposed.
- TLS certificate verification stays enabled. Certificate revocation checking (`CheckCertificateRevocation`) is not implemented in the sender.
- The identity service sends its account emails (password reset, password changed, welcome, account locked and so on) and the notification worker sends notification email through the shared mailer in `packages/go-common/mailer`, which reads this configuration. The emails themselves, their branded templates, how to preview them with `make email-preview` and `make email-send-preview`, and real delivery with a provider are described in [docs/email.md](docs/email.md). `SUPPORT_EMAIL`, `SMTP_REPLY_TO` and `SMTP_SECURE` are optional. The notification worker requires `SMTP_HOST` and fails startup when SMTP is not configured. Delivery is at-least-once; a duplicate email is possible if a crash or connection loss happens between an SMTP send and the broker acknowledgement.
- Gmail SMTP is suitable for development and small volumes. Production email should use a transactional provider with SPF/DKIM/DMARC.

## Database migrations

Migrations live under `migrations/<service>/` and use `NNNNNN_description.up.sql` / `.down.sql`. Applied migrations are immutable.

- `000001_initial_schema` creates the schema.
- `000002_tables` is the M1 baseline from [`docs/db.sql`](docs/db.sql) (26 tables, triggers, seeds).
- `000006_merchant_constraints` is the merchant-specific forward migration that preserves the merchant checks after the duplicate merchant migration files were removed. Versions `000003`–`000005` are intentionally absent for merchant; the runner accepts gaps.
- New migrations start at the next unused version (merchant: `000007`).

Apply migrations (PostgreSQL must be running):

```sh
docker compose up -d postgres
make migrate
```

`make migrate` runs `scripts/migrate.sh` inside the pinned `migrate/migrate:v4.18.3` container. The migrate service receives a `DATABASE_URL` that Compose builds from `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` (from `.env` or exported variables), so custom credentials are honored, and it never prints the URL. It bootstraps `public.set_updated_at()` and applies identity → pricing → parcel → merchant → notification, each with its own `<service>_schema_migrations` version table.

Expected result: 26 tables and seeds `5/5/25/4/4`.

Verify the baseline without writing to the database:

```sh
make db-verify
```

`make db-verify` runs read-only queries through the Compose `postgres` service with `default_transaction_read_only=on`. It checks the five schemas, 26 base tables, five identity roles, five pricing zones, 25 pricing rules, four weight bands, and four size surcharges, then prints `PASS`/`FAIL` for each check and exits non-zero if any expectation is not met. It does not run migrations, reset data, or print credentials.

**Windows and line endings.** Git for Windows may convert shell scripts to CRLF, which breaks execution in the Linux migration container. `.gitattributes` forces LF for `*.sh` and `Makefile`; do not override it. If an existing clone still fails with `bad interpreter: /bin/sh^M` or `$'\r': command not found`, preserve any local edits and re-checkout only the affected scripts:

```sh
git stash push -- scripts/migrate.sh scripts/check-routes.sh
rm scripts/migrate.sh scripts/check-routes.sh
git checkout -- scripts/migrate.sh scripts/check-routes.sh
git stash pop
```

A fresh clone also works. Do not run `git checkout -- scripts/` to fix this: it rewrites every file under `scripts/` and can discard unrelated local changes.

Roll back:

```sh
make migrate-down
```

This rolls back **one migration per service** in reverse order. On services currently at version 2 this drops their schema and tables. Use it only on a disposable database. Migrations never run automatically at application startup.

Destructive reset (local only, explicit use):

```sh
make db-reset
```

`make db-reset` runs `docker compose down -v`, deleting local PostgreSQL and RabbitMQ volumes. Never use it on shared data.

### Custom database credentials

Set `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` in `.env` (or export them). Migrations and services use the same values. Passwords containing URL-reserved characters (`@ : / ? # [ ] %`) are a known limitation because Compose builds the connection URL from these values; use URL-safe credentials for now.

`make migrate` fails clearly if the database is unreachable or credentials are wrong. Inspect with `docker compose ps postgres` and `docker compose logs postgres`. The migration runner never prints the connection URL.

## Run the frontend

```sh
pnpm exec nx dev @org/web
```

The application is available at <http://localhost:3000>. Run the frontend unit tests with `pnpm exec nx test @org/web`; browser flows run through Playwright with `pnpm exec nx e2e @org/web-e2e`.

The browser only ever calls `/api/v1/...` on its own origin. In development, Next proxies those calls to the gateway at `http://127.0.0.1:8080` (set `API_PROXY_TARGET` to use another address), so the session cookies stay first-party. Start the backend first with `make up`, otherwise sign-in, register and password reset fail with "We could not reach SwiftDrop". Google sign-in only completes from <http://localhost:8080>, because its redirect address is the gateway.

The screens are `/login`, `/register`, `/forgot-password`, `/reset-password`, `/signed-out`, a placeholder `/dashboard`, `/profile`, `/profile/security`, the COURIER `/history` delivery-history screen, the DISPATCHER/ADMIN `/reports` screen, the ADMIN `/admin/users` and `/admin/notification-logs`, and the public `/track` parcel tracking page. `/design` shows the tokens, the SwiftDrop components and the shadcn/ui primitives.

## Run an individual HTTP service

```sh
set -a
. ./.env
set +a

go run ./apps/services/identity
```

Or use Nx:

```sh
pnpm exec nx serve identity
```

Service health endpoints are internal to the Compose network (they are not published to the host):

- `GET /health` — liveness, `{"status":"ok"}`. `/healthz` remains a deprecated alias.
- `GET /ready` — readiness, `{"status":"ready","checks":{"database":"ok"}}`, or `503` when `DATABASE_URL` is unset or the database does not answer a ping. `/readyz` remains a deprecated alias.

Inspect them with `docker compose exec`, for example `docker compose exec identity wget -qO- http://127.0.0.1:8081/ready`. Through the gateway, only `/healthz` is public.

Unmatched routes return the API v1.1 error envelope with code `NOT_FOUND` and `meta.requestId`; disallowed methods return `METHOD_NOT_ALLOWED` with an `Allow` header. Panics are converted to `INTERNAL_ERROR`; stack traces, SQL, and credentials never reach clients.

## Run a worker

Workers require RabbitMQ. Start it with Docker Compose:

```sh
docker compose up -d rabbitmq
```

Then:

```sh
set -a
. ./.env
set +a

go run ./apps/services/notification-worker
```

The notification worker also requires SMTP configuration. `.env.example` defaults to Mailpit (`SMTP_HOST=mailpit`, `SMTP_PORT=1025`), so start Mailpit with `docker compose up -d mailpit` and view delivered mail at <http://localhost:8025>. A missing or invalid SMTP configuration fails startup.

Workers reconnect with capped exponential backoff and jitter when the broker is unavailable, and reset the backoff after a session reaches the consume phase. They acknowledge a message only after the handler succeeds or after a retry/dead-letter publication is confirmed. Unclassified handler failures are retried up to `RABBITMQ_MAX_RETRIES` (default `3`) with `RABBITMQ_RETRY_DELAY` (default `30s`) and then dead-lettered; malformed or unsupported messages are dead-lettered immediately. A second termination signal forces exit if graceful shutdown does not finish.

Notification delivery is at-least-once. The worker acknowledges only after the SMTP send succeeds, but delivery and acknowledgement cannot be atomic, so a duplicate email is possible after a crash or connection loss between them. A delivery failure is retried and eventually dead-lettered. Invalid payloads and unsupported channels such as `sms` are permanent failures. Workers accept message bodies up to 1 MiB with an absent or `application/json` content type. Workers do not expose a public HTTP API, with one deliberate exception: the notification worker exposes the ADMIN-only `GET /api/v1/admin/notification-logs` read API on internal port `8086`, reachable only through the gateway. Worker health checks verify the process and, for the notification worker, database connectivity; they do not prove an active RabbitMQ consumer or successful event delivery. Event-flow verification remains a future task.

Processed-event records are retained for 30 days by default. The notification worker runs cleanup immediately at startup and then every 24 hours by default, deleting `notification.processed_events` rows whose `processed_at` is strictly older than the cutoff. Configure the policy with `NOTIFICATION_PROCESSED_EVENTS_RETENTION` (default `720h`) and `NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL` (default `24h`); both values must be positive. Cleanup never modifies `notification_logs`, and a failed cleanup is logged and retried on the next interval without stopping notification consumption. Graceful shutdown cancels the scheduler and waits for it to stop before the database closes. Idempotency is guaranteed only while the processed-event row is retained, so a sufficiently late redelivery after cleanup can be processed again.

## Tests and checks

### Go unit tests and builds

```sh
gofmt -l apps/services packages/go-common tests/integration
go vet ./...
go test ./...
go test -race ./...
go build ./...
```

Unit tests do not require PostgreSQL or RabbitMQ.

### RabbitMQ integration tests

Broker-backed messaging integration tests live in [`tests/integration`](tests/integration) behind the `integration` build tag. They cover acknowledgement, retry-then-success, permanent dead-lettering, retry exhaustion, and panic recovery.

```sh
docker compose up -d rabbitmq
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -tags=integration ./tests/integration
```

Tests skip when `RABBITMQ_URL` is unset. CI sets it and fails the job if any integration test is skipped. Each test creates uniquely named exchanges and queues and removes them during cleanup. See [`tests/integration/README.md`](tests/integration/README.md).

### PostgreSQL integration tests

Dispatch and pricing migration tests live in [`tests/integration/database`](tests/integration/database) behind the `integration` build tag. They apply the parcel/pricing baseline and dispatch migrations to a uniquely named, isolated database, then verify constraints, indexes, triggers, seed data, upgrade paths, and rollback/reapply. The pricing HTTP integration test in `apps/services/pricing/internal/http` exercises the real router, repository and seeded PostgreSQL data. The parcel report query integration test in `apps/services/parcel/internal/reports` uses the parcel fixture and verifies UTC boundaries, zero-day filling, outcomes rates, top-five failure reasons and 90-day index use. The notification-log integration test in `apps/services/notification-worker/internal/notificationlog` applies the real notification migrations to an isolated database and verifies filtering, pagination, ordering and empty results. The processed-event cleanup integration test in `apps/services/notification-worker/internal/idempotency` uses a separate disposable database and verifies cutoff behavior, repeatability, cancellation, concurrency and delivery-log preservation.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  make test-db
```

`TEST_DATABASE_URL` must allow `CREATE DATABASE`. The tests drop their isolated databases during cleanup and never touch application data outside them. `make test-db` runs the migration suite, the pricing HTTP integration tests, the notification-worker idempotency/cleanup tests, the notification-log repository integration tests and the demo-seed integration tests. Compose PostgreSQL is not published to the host, so use a separately running local PostgreSQL or a temporary published test instance. CI runs the same tests against PostgreSQL 16.

### Nx and frontend checks

```sh
pnpm exec nx show projects

pnpm exec nx run-many -t lint test build \
  -p identity,parcel,pricing,merchant,notification-worker,webhook-worker,realtime,go-common

pnpm exec nx lint @org/web
pnpm exec nx build @org/web
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e
```

`@org/web` has no separate `test` or `typecheck` target; the Next build performs type checking.

To run Playwright E2E locally:

```sh
pnpm exec playwright install chromium
pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"
```

The Playwright configuration also defines Firefox and WebKit projects, but CI runs Chromium only in this phase.

The E2E suite includes `track.spec.ts`, which covers the public `/track` page without a backend: found, delivered, failed, not-found, invalid-format, rate-limited, server-fault and offline states, shared links, refresh behaviour, public-field privacy, and responsive and accessibility checks. It also includes `admin-users.spec.ts`, which covers the ADMIN `/admin/users` screen without a backend: the user list, search, the role and account filters, pagination, activate and deactivate with confirmation, the self-deactivation guard, and the loading, empty, error and no-permission states.

Go targets run from the repository root so the single root module and shared packages resolve correctly. `go.mod`, `go.sum`, and `packages/go-common` are part of every Go target's Nx cache key.

## Run the integrated environment

```sh
make up-build    # first run or after changing source code
make up          # subsequent runs, reusing built images
```

`make up-build` builds the images and then runs the full startup workflow. `make up` starts the existing images, waits for health checks, applies migrations, and verifies gateway routes without rebuilding. Plain `docker compose up --build -d` starts the same stack but does **not** migrate; run `make migrate` afterwards. Nginx waits for its web and HTTP service dependencies to become healthy before starting. Endpoints:

- Gateway (only public application port): <http://localhost:8080/>
- Frontend through the gateway: <http://localhost:8080/>
- Gateway health: <http://localhost:8080/healthz>
- Mailpit UI: <http://localhost:8025/>
- RabbitMQ management: <http://localhost:15672/>
- Public API groups: `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers`, `/api/v1/admin/notification-logs`, `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports`, `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants`
- WebSocket: `ws://localhost:8080/ws`
- `/internal/*` is blocked at the gateway (JSON `404`); `/api/v1/ai` is a TODO and returns JSON `404`.

### Docker build cache

The Go image copies only `go.mod`, `go.sum`, `packages/`, and the selected service. The web image copies the pnpm/Nx manifests, `nx.json`, TypeScript configs, `apps/web`, `apps/web-e2e`, and `packages`, instead of the whole repository. Backend-only changes therefore do not invalidate the web build, and documentation, migration, Makefile, and Compose changes do not invalidate Go builds. If a JavaScript package is added under `packages/`, add its manifest to the `pnpm install` layer in `infra/docker/web.Dockerfile`.

## Demo login

Email and password authentication is implemented in the identity service (API v1.1 sections 2.1 to 2.4): register, login, refresh and logout. Google login, password reset, admin user management, the courier list and two internal lookup APIs are also implemented in the identity service (sections 2.5, 2.6, 2.10 to 2.17 and 13); profiles are still planned. Demo users are not created by migrations; seed them with `make seed` below, or register an account instead:

```sh
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"Str0ngPass!","fullName":"Your Name","phone":"+94771234567"}'

curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -H 'X-Token-Mode: body' \
  -d '{"email":"you@example.com","password":"Str0ngPass!"}'
```

Login sets the HttpOnly `sd_access` (15 minutes) and `sd_refresh` (7 days) cookies and, in token mode, also returns `accessToken` and `refreshToken` in the body. Five wrong passwords lock the account for 15 minutes (`423 ACCOUNT_LOCKED`), and login and register are limited to 10 requests per minute per client address. `JWT_ACCESS_SECRET` is required; outside development it must be at least 32 random bytes. Admin endpoints require an ADMIN account; use the demo ADMIN created by `make seed` or another ADMIN account.

### Google OAuth (opt-in)

Google sign-in is implemented. It stays disabled until you configure your own OAuth client; SwiftDrop does not ship credentials. Keep real client IDs and secrets in your untracked `.env` only.

1. In Google Cloud Console, create an OAuth 2.0 Client ID of type **Web application**.
2. Add the authorized redirect URI exactly: `http://localhost:8080/api/v1/auth/google/callback`. The browser must reach the callback through the gateway, not through the Next.js development origin.
3. Set these values in `.env` (or export them before starting the service on the host), then recreate the identity container:
   ```sh
   GOOGLE_CLIENT_ID=your-client-id
   GOOGLE_CLIENT_SECRET=your-client-secret
   GOOGLE_REDIRECT_URI=http://localhost:8080/api/v1/auth/google/callback
   ```
   ```sh
   docker compose up -d --force-recreate identity
   ```
4. Start the flow at <http://localhost:8080/api/v1/auth/google/login> (the login screen links to it) and complete consent in a browser.

The identity service reads `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET`, and `GOOGLE_REDIRECT_URI` from its process environment. If any is missing, or the redirect URI is not an absolute HTTP(S) URL, the service logs a warning naming the missing settings (never their values) and does not mount `/auth/google/login` or `/auth/google/callback`; those paths return `404 NOT_FOUND`. Email/password authentication continues to work. See [`docs/identity-m2-handoff.md`](docs/identity-m2-handoff.md) for the browser flow and error behaviour.

### Demo seed

`make seed` creates one ADMIN, one DISPATCHER, two COURIER accounts with valid courier profiles, and one CUSTOMER. It is a local development tool and never runs during service startup, Compose startup, or migrations. It seeds identity users only; merchants, API keys, parcels, orders, notifications and delivery logs are intentionally excluded until their dependencies are ready.

The command reads process environment variables; it does not load `.env` automatically. Export `DATABASE_URL` and all ten values before running it:

```sh
export DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable'
export SEED_ADMIN_EMAIL='seed.admin@example.com'
export SEED_ADMIN_PASSWORD='<your-admin-password>'
# ...the remaining SEED_DISPATCHER, SEED_COURIER_1, SEED_COURIER_2 and SEED_CUSTOMER pairs
make seed
```

The complete required set is `SEED_ADMIN_EMAIL`, `SEED_ADMIN_PASSWORD`, `SEED_DISPATCHER_EMAIL`, `SEED_DISPATCHER_PASSWORD`, `SEED_COURIER_1_EMAIL`, `SEED_COURIER_1_PASSWORD`, `SEED_COURIER_2_EMAIL`, `SEED_COURIER_2_PASSWORD`, `SEED_CUSTOMER_EMAIL`, and `SEED_CUSTOMER_PASSWORD`.

The addresses in `.env.example` use `example.com` and are placeholders, not real mailboxes. The seed is idempotent: existing emails are skipped, including accounts with a different role, and existing passwords, roles, user details and courier profiles are never overwritten. A rerun therefore creates zero users and skips the five existing accounts.

Compose PostgreSQL is internal and does not publish a host port. To seed from the host, point `DATABASE_URL` at a reachable PostgreSQL instance. For isolated local testing, use a temporary disposable PostgreSQL database, apply all five service migrations to it, export that URL and the seed variables, then run `make seed`. Do not point the seed at production or a shared database.

Stop containers without deleting data:

```sh
docker compose down
```

`docker compose down -v` also deletes the PostgreSQL and RabbitMQ volumes. That is destructive; use it only when you intend to reset local data.

## CI

GitHub Actions runs [`.github/workflows/ci.yml`](.github/workflows/ci.yml) on pull requests to `development` and `main`, on pushes to those branches, and on `v*` and `milestone-*` tags. The current jobs are:

| Job                           | What it runs                                                                                                                                                                                                                                                                                                                 |
| ----------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Go checks**                 | `gofmt` verification, `go vet ./...`, `go test -race -covermode=atomic -coverprofile=coverage.out ./...`, uploads `coverage.out`, `go build ./...`, `docker compose config --quiet`, and the worker-healthcheck guard.                                                                                                       |
| **Go vulnerability scan**     | Installs pinned `govulncheck@v1.9.0` and fails on reachable symbol-level vulnerabilities.                                                                                                                                                                                                                                    |
| **RabbitMQ integration**      | Starts a RabbitMQ service container, requires `RABBITMQ_URL`, waits for the broker port, runs `go test -race -count=1 -tags=integration -v ./tests/integration`, and fails if any test is skipped.                                                                                                                           |
| **PostgreSQL integration**    | Starts a PostgreSQL 16 service container, requires `TEST_DATABASE_URL`, and runs the isolated dispatch, processed-event cleanup, notification-log, pricing and demo-seed database tests; fails if any test is skipped.                                                                                                       |
| **Frontend checks**           | Installs dependencies with pnpm, validates `contracts/events`, runs the design-lint and API-client Node tests, runs `pnpm exec nx test @org/web`, `pnpm exec nx typecheck @org/web`, `pnpm exec nx run-many -t lint build --projects=@org/web,@org/web-e2e`, and typechecks `@org/web-e2e`. Nx runs only configured targets. |
| **Frontend production audit** | Installs the frozen lockfile and runs `pnpm audit --prod --audit-level=high`; high/critical production findings fail.                                                                                                                                                                                                        |
| **Playwright E2E**            | Installs Chromium with dependencies and runs the `@org/web-e2e` E2E target with `--project=chromium` only.                                                                                                                                                                                                                   |
| **Secret scanning**           | Installs checksum-verified Gitleaks and scans full Git history with redacted output.                                                                                                                                                                                                                                         |
| **Docker builds and scans**   | Detects changed projects with `dorny/paths-filter`, builds only affected images, verifies each image exists, scans it with Trivy (HIGH/CRITICAL), and uploads a Syft CycloneDX SBOM plus the Trivy report.                                                                                                                   |
| **Access tests**              | Builds and starts the full Compose stack and runs the cross-service access suite when a Docker service changes, or when Compose, Nginx, PostgreSQL initialization, migrations, access tests, the Makefile, migration scripts, or `.env.example` change.                                                                      |
| **SAST (CodeQL)**             | Runs advanced CodeQL analysis for Go and JavaScript/TypeScript on pull requests and development/main pushes.                                                                                                                                                                                                                 |
| **Security gate**             | Aggregates Go, frontend, secret, SAST, and container security results; Docker is allowed to be skipped when change detection finds no affected images, but a failed Docker/scan job fails the gate.                                                                                                                          |
| **Required CI gate**          | Always runs and fails if any core check, security gate, `changes`, Docker build, or access test fails or is cancelled; Docker and access may be skipped when change detection finds no affected projects.                                                                                                                    |
| **Release smoke (tags)**      | On `v*` and `milestone-*` tags, prepares `.env` from `.env.example`, builds every image, starts the full Compose stack with `--wait`, applies migrations with `make migrate`, runs `scripts/check-routes.sh`, and stops the stack.                                                                                           |

The path filters, the separate access output, and the required-gate result handling are covered by `tests/scripts/ci_path_filter_test.go`. The workflow does not deploy, publish images, or use deployment credentials. This documentation does not claim that CI has passed for a given revision; check the Actions run for the branch or pull request.

Dependency updates are managed manually by the development team; Dependabot is not enabled.

See [Security scanning](docs/security-scanning.md) for scanner versions, thresholds, local commands, SBOM artifacts, exception policy, and security-gate troubleshooting.

## Troubleshooting

- **Port already in use:** change `NGINX_PORT`, `RABBITMQ_PORT`, `RABBITMQ_MANAGEMENT_PORT`, `MAILPIT_SMTP_PORT`, or `MAILPIT_UI_PORT` in `.env` and restart Compose. `SMTP_PORT` is the outbound provider port, not a published port. Service and web ports are internal and are not published.
- **Environment variables missing for a host process:** Go services do not load `.env`. Export it with `set -a; . ./.env; set +a` before `go run` or `nx serve`.
- **Migrations cannot connect:** run `docker compose up -d postgres`, then `docker compose ps postgres`. Confirm `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` match your `.env`. `make migrate` reads the Compose-resolved `DATABASE_URL`, not a hardcoded one. URL-reserved password characters are a known limitation (use URL-safe credentials).
- **Nginx returns 502 or will not start:** Nginx resolves upstreams at startup. Start the full stack with `make up` rather than starting Nginx alone. Check `docker compose ps` for unhealthy services.
- **Images are missing or stale (`make up` starts old code):** run `make up-build` to rebuild before starting.
- **Worker keeps reconnecting:** confirm RabbitMQ is healthy and `RABBITMQ_URL` points at the broker. For containers use the `rabbitmq` service name; use `localhost` only from the host. Reconnect attempts are logged with the next delay.
- **Integration tests skip:** `RABBITMQ_URL` is unset or empty. Set it to the Compose broker URL. CI treats skips as failures.
- **Playwright cannot launch a browser:** run `pnpm exec playwright install chromium` (add `--with-deps` on Linux if system libraries are missing).
- **Docker runs out of memory while building every Go image:** the Go Dockerfile bounds compiler memory (`GOFLAGS=-p=1`, `GOMEMLIMIT`, `GOGC`), and `make build` builds the application images one service at a time in a fixed order. If a build still exhausts memory, build one service at a time with `docker compose build <service>` and then run `make up`; do not run global Docker cleanup.

  ```sh
  docker compose build identity
  docker compose build parcel
  docker compose build pricing
  docker compose build merchant
  docker compose build realtime
  docker compose build notification-worker
  docker compose build webhook-worker
  docker compose build web

  docker compose up -d
  ```

- **Migration fails with a duplicate version:** check `migrations/<service>/` for repeated six-digit versions. Applied migrations are immutable; add a new forward migration instead of renumbering an applied one.
- **Merchant migration history:** the duplicate merchant files (`000002_merchants`–`000005_audit_logs`) were removed before ever being applied to the baseline database. Merchant schema changes now live in `000006_merchant_constraints`. Databases that somehow applied the merchant variant as version 2 must be rebuilt by the merchant owner.
- **CI fails on formatting:** run `gofmt -w` on the changed Go files and `pnpm exec prettier --write` on changed documentation or frontend files.

## Branching and pull requests

Start feature branches from `development` and open pull requests into `development`. Keep changes focused, include tests and documentation with implementation changes, and use the pull request template. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full workflow and required checks.

## Adding a new service

1. Create `apps/services/<name>/` with `main.go`, `internal/config`, and either `internal/http` or `internal/worker` plus tests.
2. Import shared code through `github.com/Vineth-R/swiftdrop/packages/go-common/...`.
3. Add a `project.json` with `build`, `test`, `lint`, and `serve` targets using `nx:run-commands`.
4. Add a service-owned schema migration under `migrations/<name>/` if the service persists data.
5. Add an OpenAPI specification under `contracts/openapi/`.
6. If the service is public, add an Nginx route and matching Gin paths, then update the Compose service and documentation.
7. Run `gofmt`, `go test ./...`, `go build ./...`, and the Nx targets.

Do not create a nested `go.mod` or `go.work`. Do not give services a shared business-logic package or direct access to another service's tables.

## Documentation

- [`DESIGN.md`](DESIGN.md): the frontend design guide. Read it before building or changing any screen in `apps/web`.
- [`docs/architecture.md`](docs/architecture.md): service boundaries, routing, lifecycle, messaging, and readiness.
- [`docs/db.sql`](docs/db.sql): canonical database definition (26 tables, five schemas, triggers, seeds).
- [`docs/API_Documentation_v1.1.md`](docs/API_Documentation_v1.1.md) and [`docs/Database_Design_Document_v1.1.md`](docs/Database_Design_Document_v1.1.md): product API and database design sources.
- [`docs/how-to-error-logging.md`](docs/how-to-error-logging.md): adopting the shared error envelope and safe logging.
- [`docs/security-scanning.md`](docs/security-scanning.md): automated security scanners, thresholds, SBOMs, exceptions, and gate troubleshooting.
- [`docs/follow-ups/merchant-database-integration.md`](docs/follow-ups/merchant-database-integration.md): merchant-owner database integration review.
- [`contracts/events/README.md`](contracts/events/README.md): event naming, envelope, framing, and failure rules.
- [`contracts/openapi/README.md`](contracts/openapi/README.md): HTTP contracts and shared error behavior.
- [`migrations/README.md`](migrations/README.md): migration conventions and commands.
- [`infra/nginx/README.md`](infra/nginx/README.md): local edge routing.
- [`tests/integration/README.md`](tests/integration/README.md): RabbitMQ and PostgreSQL integration test setup.
- [`CONTRIBUTING.md`](CONTRIBUTING.md): development and review workflow.

## Current scope

Implemented: repository structure, service entry points, health and readiness endpoints, environment configuration with case-insensitive and trimmed values, structured logging, request IDs, graceful shutdown, structured panic recovery, JSON 404/405 responses, a 1 MiB HTTP header limit, shared infrastructure, event envelope and message framing validation, bounded RabbitMQ retry and dead-letter routing with publisher confirms, reconnect backoff with jitter, unit and broker-backed messaging integration tests, one Playwright example test run Chromium-only in CI, Dockerfiles, Compose infrastructure, Nginx routing, migration and event conventions, Nx targets, CI, and documentation.

Platform foundation implemented: shared PostgreSQL pool and database-backed readiness, five-schema baseline migrations (26 tables and reference seeds), forward merchant constraints, Mailpit, published gateway and development infrastructure ports, API v1.1 error envelope with request IDs, redacting logs, the API v1.1 gateway route groups, and `make up` one-command startup with container-based migrations.

M4 implemented: `POST /pricing/estimate`, `GET /zones`, authenticated internal quote/zone lookups, ADMIN pricing configuration with transactional audit logging, exact decimal LKR calculations, bounded public rate limiting, trusted-proxy handling, and anonymous `GET /tracking/{trackingNumber}` backed by real parcel/status-history rows. M5 consumes the real internal quote and persists its exact LKR total and snake_case `price_breakdown` atomically with the parcel, initial history and PENDING outbox data. Customer creation/read/history/PDF APIs, internal merchant creation/summary and the `/new`, `/parcels`, `/parcels/{id}` screens are implemented. See [M5 evidence](docs/m5-complete-parcel-flow.md) for verification and remaining dependencies.

M10.1 reports implemented in the parcel service: `GET /reports/daily` returns UTC event-date counts of created, delivered and failed parcels with zero-filled days, and `GET /reports/outcomes` returns totals, success/failure rates and the five most common failure reasons for parcels created in the range. Both require DISPATCHER or ADMIN access, default to 7 and 30 inclusive UTC dates respectively, reject ranges longer than 90 days, and set `Cache-Control: no-store`. The report routes fail closed with `503` when `JWT_ACCESS_SECRET` is not configured.

M10.4 adds the DISPATCHER/ADMIN `/reports` screen. It filters a UTC date range of up to 90 days, shows outcome totals, success and failure rates, top failure reasons, and a daily chart and table. Loading, empty, server-fault, rate-limit, validation, no-permission and network states are handled; the screen calls the two M10.1 report endpoints through the shared API client. Its Playwright suite covers the report states, role access, range validation, touch targets and dark mode.

Notification email delivery is implemented in the notification worker for the legacy `notification.requested.v1` work item and the customer-owned `parcel.created.v1`/`parcel.status_changed.v1` domain events. The legacy path validates the required `parcel_id` and `recipient`; the domain path resolves the customer's contact through the identity internal endpoint and renders a branded parcel email. `sms` is a permanent failure, and delivery failures follow the existing retry/dead-letter policy. The worker records processed event IDs in `notification.processed_events` and delivery outcomes in `notification.notification_logs` (`SENT`, `PENDING`, `FAILED`); the success log and processed-event writes share one transaction. A background scheduler deletes processed-event records older than 30 days at startup and every 24 hours by default. The worker also exposes the ADMIN-only `GET /api/v1/admin/notification-logs` read API through its internal HTTP port `8086`; recipients are masked and `status` can filter `PENDING`, `SENT` or `FAILED`. Merchant-owned parcel notifications are unsupported and dead-lettered because no approved recipient-contact mechanism exists. Delivery remains at-least-once, not exactly-once, and M5 creation now writes transactional outbox data; M6 relay publication is implemented and integrated with creation.

Merchant-owner follow-up: the merchant repository uses `database/sql` and is not wired to the shared pgx pool or HTTP handlers yet; no schema change is required for that wiring.

Implemented by identity (M2/M3): email/password authentication, Google login, password reset, admin users, couriers, internal lookups, and the signed-in user's own `GET` and `PATCH /users/me` and `PUT /users/me/password`. Implemented by M4: pricing estimates/zones/internal quote, ADMIN pricing configuration and public tracking reads. Implemented by M1: the legacy notification worker, the customer-owned `parcel.created.v1`/`parcel.status_changed.v1` consumer with identity contact resolution, ADMIN notification delivery-log reads and the ADMIN notification-log screen, with masked recipients and status filtering, plus deterministic demo-user seeding (`make seed`). Implemented by M10: the DISPATCHER/ADMIN reports screen (`/reports`). The COURIER `/history` delivery-history screen is implemented in the web app against the planned parcel `GET /courier/deliveries` contract; until that endpoint lands, the screen shows its endpoint-unavailable state.

Not implemented: the parcel courier delivery APIs, courier lifecycle writes, merchant-owned parcel notification contact resolution, merchant onboarding and API keys, SMS notification delivery, webhook signing and delivery retries, realtime subscriptions, the pricing administration UI, the AI assistant, payments, external integrations, production TLS, Kubernetes, Terraform, and production deployment automation. Report Sections 6.1/8 and Figures 8/9 remain outstanding.
