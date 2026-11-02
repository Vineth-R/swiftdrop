# SwiftDrop

SwiftDrop is a parcel delivery and courier management platform. This repository is a production-oriented foundation: an Nx monorepo with a Next.js frontend, independently runnable Go HTTP services, RabbitMQ workers, a shared Go infrastructure package, PostgreSQL migrations, event and API contracts, Nginx routing, local Docker Compose infrastructure, and GitHub Actions CI.

Business functionality such as parcel lifecycles, pricing rules, merchant onboarding, live tracking, SMS notification delivery, and webhook delivery is intentionally not implemented yet. See [Current scope](#current-scope) and [`docs/architecture.md`](docs/architecture.md).

## Technology

- **Frontend:** Next.js 16 App Router, React 19, TypeScript 6, pnpm, Nx.
- **Backend:** Go 1.27.3 with Gin, one root Go module.
- **Data:** PostgreSQL 16 with service-owned schemas and versioned SQL migrations.
- **Messaging:** RabbitMQ 4.1 with a durable topic exchange, versioned JSON event contracts, bounded retries, and dead-lettering.
- **Edge:** Nginx reverse proxy.
- **Local orchestration:** Docker Compose.
- **CI:** GitHub Actions.

## Architecture

| Service               | Responsibility                                              | Internal port |
| --------------------- | ----------------------------------------------------------- | ------------- |
| `identity`            | Future authentication, authorization, and identity profiles | 8081          |
| `parcel`              | Future parcel lifecycle, status, and tracking               | 8082          |
| `pricing`             | Future delivery fee calculation and pricing configuration   | 8083          |
| `merchant`            | Future merchant accounts, settings, and API credentials     | 8084          |
| `realtime`            | Future live tracking over WebSocket or Server-Sent Events   | 8085          |
| `notification-worker` | Notification event consumption and email delivery           | no HTTP port  |
| `webhook-worker`      | Future outgoing webhook delivery                            | no HTTP port  |
| `@org/web`            | Next.js frontend                                            | 3000          |
| Nginx                 | Public HTTP entry point                                     | 8080          |

Only the gateway (`http://localhost:8080`) is published. Service ports below are internal to the Compose network. Services communicate synchronously over HTTP when a caller needs an immediate result. Asynchronous work uses RabbitMQ. Each persisting service owns its PostgreSQL schema and must not modify another service's tables. Realtime currently has no schema or persistence. Read [`docs/architecture.md`](docs/architecture.md) for service boundaries, routing, event conventions, lifecycle behavior, and readiness semantics; this README does not repeat the full architecture.

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
- Node.js **24**, the CI version.
- pnpm **12.15.0** through Corepack.
- Docker with Docker Compose v2.
- Optional: the `golang-migrate` CLI for database migrations.
- Optional: `psql` to inspect the local database.
- Optional: Chromium if you run Playwright E2E tests.

```sh
corepack enable
corepack prepare pnpm@12.15.0 --activate
```

`make` drives the database helpers (`make migrate`, `make migrate-down`, `make db-reset`, `make check-routes`). Nx and Go commands remain available for everything else.

## Install and configure

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
pnpm install --frozen-lockfile
```

`.env.example` contains safe local placeholders only. Do not commit `.env`, credentials, private keys, or production secrets. Docker Compose reads `.env` automatically. Go services read process environment variables directly and do not load `.env`; export it before running a service or worker on the host.

The template documents every variable currently used: PostgreSQL, RabbitMQ, gateway, Mailpit, Gmail SMTP (`SMTP_HOST`, `SMTP_PORT`, `SMTP_USER`, `SMTP_PASSWORD`, `SMTP_FROM`, `SMTP_FROM_NAME`), and pool overrides. It also includes placeholders reserved for M2's identity work (`JWT_ACCESS_SECRET`, `JWT_REFRESH_SECRET`, token TTLs, cookie names and flags, Google OAuth client settings, `FRONTEND_BASE_URL`) and `INTERNAL_TOKEN` for internal service calls. Placeholders do not enable authentication.

```sh
set -a
. ./.env
set +a
```

The integration branch is `development`. Create feature branches from it and open pull requests into `development`. Do not push directly to `development` or `main`. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the branch, pull request, review, and merge workflow.

## Quick start

Prerequisites: Docker with Compose v2 and `make`. Go, Node.js, pnpm, and Playwright are only needed for host-side development and tests.

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

**Subsequent startups:** use `make up`. It reuses the existing images and does not rebuild them.

**After changing application source code:** use `make up-build` so the images are rebuilt before startup.

Running `make up` again is safe: Compose is a no-op, migrations report `no change`, and routes are re-verified.

If `.env` is missing and `POSTGRES_USER`/`POSTGRES_DB` are not exported, `make up` fails with a clear error instead of silently using defaults.

### Command reference

| Command                        | What it does                                                     |
| ------------------------------ | ---------------------------------------------------------------- |
| `make build`                   | Build all application images; does not start containers          |
| `make up`                      | Start existing images, wait for health, migrate, verify routes   |
| `make up-build`                | Build images, then run the full `make up` workflow               |
| `docker compose up --build -d` | Start only; **builds but does not migrate**                      |
| `make migrate`                 | Apply all service migrations                                     |
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

PostgreSQL is internal to the Compose network and does not publish a host port. `make migrate` runs inside the Compose network. To run Go services or PostgreSQL integration tests on the host, point `DATABASE_URL`/`TEST_DATABASE_URL` at a PostgreSQL instance you run separately (for example a local PostgreSQL install on `localhost:5432`); do not add a host port to `docker-compose.yml` just for local tests.

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
- The identity service sends password-reset email and the notification worker sends notification email through the shared mailer in `packages/go-common/mailer`, which reads this configuration. The notification worker requires `SMTP_HOST` and fails startup when SMTP is not configured. Delivery is at-least-once; a duplicate email is possible if a crash or connection loss happens between an SMTP send and the broker acknowledgement.
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

`make migrate` runs `scripts/migrate.sh` inside the pinned `migrate/migrate:v4.18.3` container. It uses the Compose-resolved `DATABASE_URL` (from `.env` or exported variables), so custom credentials are honored, and it never prints the URL. It bootstraps `public.set_updated_at()` and applies identity → pricing → parcel → merchant → notification, each with its own `<service>_schema_migrations` version table.

Expected result: 26 tables and seeds `5/5/25/4/4`.

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

The application is available at <http://localhost:3000>. The existing `/api/hello` route remains available.

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

Notification delivery is at-least-once. The worker acknowledges only after the SMTP send succeeds, but delivery and acknowledgement cannot be atomic, so a duplicate email is possible after a crash or connection loss between them. A delivery failure is retried and eventually dead-lettered. Invalid payloads and unsupported channels such as `sms` are permanent failures. Workers accept message bodies up to 1 MiB with an absent or `application/json` content type. Workers do not expose a public HTTP API.

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

Dispatch database tests live in [`tests/integration/database`](tests/integration/database) behind the `integration` build tag. They apply the parcel baseline and dispatch migrations to a uniquely named, isolated database, then verify assignments, outbox behavior, constraints, indexes, triggers, upgrade paths, and rollback/reapply.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  make test-db
```

`TEST_DATABASE_URL` must allow `CREATE DATABASE`. The tests drop their isolated databases during cleanup and never touch application data outside them. Compose PostgreSQL is not published to the host, so use a separately running local PostgreSQL or a temporary published test instance. CI runs the same tests against PostgreSQL 16.

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
- Public API groups: `/api/v1/auth`, `/api/v1/users`, `/api/v1/admin/users`, `/api/v1/couriers`, `/api/v1/pricing`, `/api/v1/zones`, `/api/v1/parcels`, `/api/v1/tracking`, `/api/v1/dispatch`, `/api/v1/admin/parcels`, `/api/v1/courier`, `/api/v1/reports`, `/api/v1/merchants`, `/api/v1/merchant`, `/api/v1/admin/merchants`
- WebSocket: `ws://localhost:8080/ws`
- `/internal/*` is blocked at the gateway (JSON `404`); `/api/v1/ai` is a TODO and returns JSON `404`.

### Docker build cache

The Go image copies only `go.mod`, `go.sum`, `packages/`, and the selected service. The web image copies the pnpm/Nx manifests, `nx.json`, TypeScript configs, `apps/web`, `apps/web-e2e`, and `packages`, instead of the whole repository. Backend-only changes therefore do not invalidate the web build, and documentation, migration, Makefile, and Compose changes do not invalidate Go builds. If a JavaScript package is added under `packages/`, add its manifest to the `pnpm install` layer in `infra/docker/web.Dockerfile`.

## Demo login

Email and password authentication is implemented in the identity service (API v1.1 sections 2.1 to 2.4): register, login, refresh and logout. Google login, password reset, admin user management, the courier list and two internal lookup APIs are also implemented in the identity service (sections 2.5, 2.6, 2.10 to 2.17 and 13); profiles are still planned. Demo users and password seeding are also pending, so register an account first:

```sh
curl -i -X POST http://localhost:8080/api/v1/auth/register \
  -H 'Content-Type: application/json' \
  -d '{"email":"you@example.com","password":"Str0ngPass!","fullName":"Your Name","phone":"+94771234567"}'

curl -i -X POST http://localhost:8080/api/v1/auth/login \
  -H 'Content-Type: application/json' \
  -H 'X-Token-Mode: body' \
  -d '{"email":"you@example.com","password":"Str0ngPass!"}'
```

Login sets the HttpOnly `sd_access` (15 minutes) and `sd_refresh` (7 days) cookies and, in token mode, also returns `accessToken` and `refreshToken` in the body. Five wrong passwords lock the account for 15 minutes (`423 ACCOUNT_LOCKED`), and login and register are limited to 10 requests per minute per client address. `JWT_ACCESS_SECRET` is required; outside development it must be at least 32 random bytes.

Stop containers without deleting data:

```sh
docker compose down
```

`docker compose down -v` also deletes the PostgreSQL and RabbitMQ volumes. That is destructive; use it only when you intend to reset local data.

## CI

GitHub Actions runs [`.github/workflows/ci.yml`](.github/workflows/ci.yml) on pull requests to `development` and `main`, on pushes to those branches, and on `v*` tags. The current jobs are:

| Job                        | What it runs                                                                                                                                                                                                           |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| **Go checks**              | `gofmt` verification, `go vet ./...`, `go test -race -covermode=atomic -coverprofile=coverage.out ./...`, uploads `coverage.out`, `go build ./...`, `docker compose config --quiet`, and the worker-healthcheck guard. |
| **RabbitMQ integration**   | Starts a RabbitMQ service container, requires `RABBITMQ_URL`, waits for the broker port, runs `go test -race -count=1 -tags=integration -v ./tests/integration`, and fails if any test is skipped.                     |
| **PostgreSQL integration** | Starts a PostgreSQL 16 service container, requires `TEST_DATABASE_URL`, and runs the isolated dispatch database tests under `tests/integration/database`; fails if any test is skipped.                                |
| **Frontend checks**        | Installs dependencies with pnpm, runs `pnpm exec nx run-many -t lint build --projects=@org/web,@org/web-e2e`, and typechecks `@org/web-e2e`. Nx runs only configured targets.                                          |
| **Playwright E2E**         | Installs Chromium with dependencies and runs the `@org/web-e2e` E2E target with `--project=chromium` only.                                                                                                             |
| **Docker builds**          | Detects changed projects with `dorny/paths-filter` and builds only the affected Go service images and the web image.                                                                                                   |
| **Release smoke (tags)**   | On `v*` tags, prepares `.env` from `.env.example`, builds every image, starts the full Compose stack with `--wait`, applies migrations with `make migrate`, runs `scripts/check-routes.sh`, and stops the stack.       |

The workflow does not deploy, publish images, or use deployment credentials. This documentation does not claim that CI has passed for a given revision; check the Actions run for the branch or pull request.

## Troubleshooting

- **Port already in use:** change `NGINX_PORT`, `RABBITMQ_PORT`, `RABBITMQ_MANAGEMENT_PORT`, `MAILPIT_SMTP_PORT`, or `MAILPIT_UI_PORT` in `.env` and restart Compose. `SMTP_PORT` is the outbound provider port, not a published port. Service and web ports are internal and are not published.
- **Environment variables missing for a host process:** Go services do not load `.env`. Export it with `set -a; . ./.env; set +a` before `go run` or `nx serve`.
- **Migrations cannot connect:** run `docker compose up -d postgres`, then `docker compose ps postgres`. Confirm `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` match your `.env`. `make migrate` reads the Compose-resolved `DATABASE_URL`, not a hardcoded one. URL-reserved password characters are a known limitation (use URL-safe credentials).
- **Nginx returns 502 or will not start:** Nginx resolves upstreams at startup. Start the full stack with `make up` rather than starting Nginx alone. Check `docker compose ps` for unhealthy services.
- **Images are missing or stale (`make up` starts old code):** run `make up-build` to rebuild before starting.
- **Worker keeps reconnecting:** confirm RabbitMQ is healthy and `RABBITMQ_URL` points at the broker. For containers use the `rabbitmq` service name; use `localhost` only from the host. Reconnect attempts are logged with the next delay.
- **Integration tests skip:** `RABBITMQ_URL` is unset or empty. Set it to the Compose broker URL. CI treats skips as failures.
- **Playwright cannot launch a browser:** run `pnpm exec playwright install chromium` (add `--with-deps` on Linux if system libraries are missing).
- **Docker runs out of memory while building every Go image:** the Go Dockerfile bounds compiler memory (`GOFLAGS=-p=1`, `GOMEMLIMIT`, `GOGC`) and the Makefile serializes builds with `COMPOSE_PARALLEL_LIMIT=1`. Build sequentially with `make build`, then start with `make up`.

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

- [`docs/architecture.md`](docs/architecture.md): service boundaries, routing, lifecycle, messaging, and readiness.
- [`docs/db.sql`](docs/db.sql): canonical database definition (26 tables, five schemas, triggers, seeds).
- [`docs/API_Documentation_v1.1.md`](docs/API_Documentation_v1.1.md) and [`docs/Database_Design_Document_v1.1.md`](docs/Database_Design_Document_v1.1.md): product API and database design sources.
- [`docs/how-to-error-logging.md`](docs/how-to-error-logging.md): adopting the shared error envelope and safe logging.
- [`docs/follow-ups/merchant-database-integration.md`](docs/follow-ups/merchant-database-integration.md): merchant-owner database integration review.
- [`contracts/events/README.md`](contracts/events/README.md): event naming, envelope, framing, and failure rules.
- [`contracts/openapi/README.md`](contracts/openapi/README.md): HTTP contracts and shared error behavior.
- [`migrations/README.md`](migrations/README.md): migration conventions and commands.
- [`infra/nginx/README.md`](infra/nginx/README.md): local edge routing.
- [`tests/integration/README.md`](tests/integration/README.md): RabbitMQ and PostgreSQL integration test setup.
- [`CONTRIBUTING.md`](CONTRIBUTING.md): development and review workflow.

## Current scope

Implemented: repository structure, service entry points, health and readiness endpoints, environment configuration with case-insensitive and trimmed values, structured logging, request IDs, graceful shutdown, structured panic recovery, JSON 404/405 responses, a 1 MiB HTTP header limit, shared infrastructure, event envelope and message framing validation, bounded RabbitMQ retry and dead-letter routing with publisher confirms, reconnect backoff with jitter, unit and broker-backed messaging integration tests, one Playwright example test run Chromium-only in CI, Dockerfiles, Compose infrastructure, Nginx routing, migration and event conventions, Nx targets, CI, and documentation.

Platform foundation implemented: shared PostgreSQL pool and database-backed readiness, five-schema baseline migrations (26 tables and reference seeds), forward merchant constraints, Mailpit, gateway-only published ports, API v1.1 error envelope with request IDs, redacting logs, the API v1.1 gateway route groups, and `make up` one-command startup with container-based migrations.

Notification email delivery is implemented in the notification worker: `notification.requested.v1` is validated, the `email` channel is sent through the shared SMTP mailer, `sms` is a permanent failure, and delivery failures follow the existing retry/dead-letter policy. The worker is stateless and does not persist notification status.

Merchant-owner follow-up: the merchant repository uses `database/sql` and is not wired to the shared pgx pool or HTTP handlers yet; no schema change is required for that wiring.

Not implemented: authentication (owned by M2), JWT, cookies/sessions, parcels, pricing formulas, merchant onboarding, API keys, SMS notification delivery, webhook signing and delivery retries, realtime subscriptions, the AI assistant, payments, external integrations, production TLS, Kubernetes, Terraform, and production deployment automation. Business tables exist, but no business endpoints or event producers are implemented yet.
