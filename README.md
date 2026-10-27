# SwiftDrop

SwiftDrop is a parcel delivery and courier management platform. This repository is a production-oriented foundation: an Nx monorepo with a Next.js frontend, independently runnable Go HTTP services, RabbitMQ workers, a shared Go infrastructure package, PostgreSQL migrations, event and API contracts, Nginx routing, local Docker Compose infrastructure, and GitHub Actions CI.

Business functionality such as authentication, parcel lifecycles, pricing rules, merchant onboarding, live tracking, notification delivery, and webhook delivery is intentionally not implemented yet. See [Current scope](#current-scope) and [`docs/architecture.md`](docs/architecture.md).

## Technology

- **Frontend:** Next.js 16 App Router, React 19, TypeScript 6, pnpm, Nx.
- **Backend:** Go 1.27.3 with Gin, one root Go module.
- **Data:** PostgreSQL 16 with service-owned schemas and versioned SQL migrations.
- **Messaging:** RabbitMQ 4.1 with a durable topic exchange, versioned JSON event contracts, bounded retries, and dead-lettering.
- **Edge:** Nginx reverse proxy.
- **Local orchestration:** Docker Compose.
- **CI:** GitHub Actions.

## Architecture

| Service               | Responsibility                                              | Local port   |
| --------------------- | ----------------------------------------------------------- | ------------ |
| `identity`            | Future authentication, authorization, and identity profiles | 8081         |
| `parcel`              | Future parcel lifecycle, status, and tracking               | 8082         |
| `pricing`             | Future delivery fee calculation and pricing configuration   | 8083         |
| `merchant`            | Future merchant accounts, settings, and API credentials     | 8084         |
| `realtime`            | Future live tracking over WebSocket or Server-Sent Events   | 8085         |
| `notification-worker` | Future notification event consumption and delivery          | no HTTP port |
| `webhook-worker`      | Future outgoing webhook delivery                            | no HTTP port |
| `@org/web`            | Next.js frontend                                            | 3000         |
| Nginx                 | Public HTTP entry point                                     | 8080         |

Services communicate synchronously over HTTP when a caller needs an immediate result. Asynchronous work uses RabbitMQ. Each persisting service owns its PostgreSQL schema and must not modify another service's tables. Realtime currently has no schema or persistence. Read [`docs/architecture.md`](docs/architecture.md) for service boundaries, routing, event conventions, lifecycle behavior, and readiness semantics; this README does not repeat the full architecture.

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
tests/integration/          Broker-backed RabbitMQ integration tests
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

GNU Make is not required; use Nx and Go commands directly.

## Install and configure

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
pnpm install --frozen-lockfile
```

`.env.example` contains safe local placeholders only. Do not commit `.env`, credentials, private keys, or production secrets. Docker Compose reads `.env` automatically. Go services read process environment variables directly and do not load `.env`; export it before running a service or worker on the host:

```sh
set -a
. ./.env
set +a
```

The default development branch is currently `chore/initial-workspace` (the remote default). Create feature branches from it. Do not push directly to the shared branch. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the branch, pull request, review, and merge workflow.

## Local dependencies

Start only what you need:

```sh
# Required for database migrations. No service connects to PostgreSQL yet.
docker compose up -d postgres

# Required for workers and broker-backed integration tests.
docker compose up -d rabbitmq
```

Check health with `docker compose ps`. The PostgreSQL container uses `pg_isready`; RabbitMQ uses `rabbitmq-diagnostics -q ping`. RabbitMQ management is available at <http://localhost:15672/> with the credentials from `.env`.

HTTP services and the frontend do not require PostgreSQL or RabbitMQ to start. Workers require a reachable RabbitMQ.

## Database migrations

Migrations are organized by owning service under `migrations/<service>/` and use `NNNNNN_description.up.sql` and `.down.sql`. The initial migrations create each service schema only. There are no business tables or seed data.

Start PostgreSQL first and wait for it to report healthy:

```sh
docker compose up -d postgres
docker compose ps postgres
```

Install the migration CLI and run migrations explicitly:

```sh
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable'

migrate -path migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" up
```

Repeat for `parcel`, `pricing`, and `merchant`, using a distinct migration version table for each service. Roll back with `down 1`.

No service connects to PostgreSQL yet. `DATABASE_URL` is loaded for future use but application code does not open a database connection. Services never run migrations automatically at startup. Full conventions and destructive-operation warnings are in [`migrations/README.md`](migrations/README.md).

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

Direct service endpoints:

- <http://localhost:8081/healthz>
- <http://localhost:8081/readyz>

Replace `identity` and `8081` with another service and its port. Readiness currently reports process readiness only; it does not claim that PostgreSQL or RabbitMQ is reachable. Unmatched routes return a JSON `404`; disallowed methods return a JSON `405` with an `Allow` header. Panics are converted to a JSON `500` without exposing internal details.

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

Workers reconnect with capped exponential backoff and jitter when the broker is unavailable, and reset the backoff after a session reaches the consume phase. They acknowledge a message only after the handler succeeds or after a retry/dead-letter publication is confirmed. Unclassified handler failures are retried up to `RABBITMQ_MAX_RETRIES` (default `3`) with `RABBITMQ_RETRY_DELAY` (default `30s`) and then dead-lettered; malformed or unsupported messages are dead-lettered immediately. A second termination signal forces exit if graceful shutdown does not finish.

Delivery is at-least-once. Duplicate or redelivered messages are normal, including after reconnects and retries; future state-changing handlers must deduplicate by `event_id` and be idempotent. Workers accept message bodies up to 1 MiB with an absent or `application/json` content type. Workers do not expose a public HTTP API.

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
  go test -tags=integration ./tests/integration/...
```

Tests skip when `RABBITMQ_URL` is unset. CI sets it and fails the job if any integration test is skipped. Each test creates uniquely named exchanges and queues and removes them during cleanup. See [`tests/integration/README.md`](tests/integration/README.md).

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
docker compose up --build
```

Nginx waits for its web and HTTP service dependencies to become healthy before starting. Endpoints:

- Frontend through Nginx: <http://localhost:8080/>
- Frontend directly: <http://localhost:3000/>
- Identity through Nginx: <http://localhost:8080/api/v1/identity/healthz>
- Parcels through Nginx: <http://localhost:8080/api/v1/parcels/healthz>
- Pricing through Nginx: <http://localhost:8080/api/v1/pricing/healthz>
- Merchants through Nginx: <http://localhost:8080/api/v1/merchants/healthz>
- Realtime through Nginx: <http://localhost:8080/api/v1/realtime/healthz>
- RabbitMQ management: <http://localhost:15672/>

Stop containers without deleting data:

```sh
docker compose down
```

`docker compose down -v` also deletes the PostgreSQL and RabbitMQ volumes. That is destructive; use it only when you intend to reset local data.

## CI

GitHub Actions runs the workflow in [`.github/workflows/ci.yml`](.github/workflows/ci.yml) on every push and pull request. The current jobs are:

| Job                      | What it runs                                                                                                                                                                                           |
| ------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| **Go checks**            | `gofmt` verification, `go vet ./...`, `go test -race ./...`, `go build ./...`, and `docker compose config --quiet`.                                                                                    |
| **RabbitMQ integration** | Starts a RabbitMQ service container, requires `RABBITMQ_URL`, waits for the broker port, runs `go test -race -count=1 -tags=integration -v ./tests/integration/...`, and fails if any test is skipped. |
| **Frontend checks**      | Installs dependencies with pnpm and runs `pnpm exec nx run-many -t lint typecheck build --projects=@org/web,@org/web-e2e`. Nx runs only configured targets.                                            |
| **Playwright E2E**       | Installs Chromium with dependencies and runs the `@org/web-e2e` E2E target with `--project=chromium` only.                                                                                             |
| **Docker builds**        | Validates Compose configuration and builds representative images: the Go image for `identity` and the Next.js web image. It does not build every service image.                                        |

The workflow does not deploy, publish images, or use deployment credentials. This documentation does not claim that CI has passed for a given revision; check the Actions run for the branch or pull request.

## Troubleshooting

- **Port already in use:** change the matching port in `.env` (`IDENTITY_PORT`, `WEB_PORT`, `NGINX_PORT`, and so on) and restart Compose.
- **Environment variables missing for a host process:** Go services do not load `.env`. Export it with `set -a; . ./.env; set +a` before `go run` or `nx serve`.
- **Migrations cannot connect:** run `docker compose up -d postgres`, then `docker compose ps postgres`. Confirm `DATABASE_URL` matches the configured user, password, database, and host port.
- **Nginx returns 502 or will not start:** Nginx resolves upstreams at startup. Start the full stack with `docker compose up --build` rather than starting Nginx alone. Check `docker compose ps` for unhealthy services.
- **Worker keeps reconnecting:** confirm RabbitMQ is healthy and `RABBITMQ_URL` points at the broker. For containers use the `rabbitmq` service name; use `localhost` only from the host. Reconnect attempts are logged with the next delay.
- **Integration tests skip:** `RABBITMQ_URL` is unset or empty. Set it to the Compose broker URL. CI treats skips as failures.
- **Playwright cannot launch a browser:** run `pnpm exec playwright install chromium` (add `--with-deps` on Linux if system libraries are missing).
- **Docker runs out of memory while building every Go image:** build services individually, then start the stack.

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

- **CI fails on formatting:** run `gofmt -w` on the changed Go files and `pnpm exec prettier --write` on changed documentation or frontend files.

## Branching and pull requests

Start feature branches from `chore/initial-workspace`. Keep changes focused, include tests and documentation with implementation changes, and use the pull request template. See [`CONTRIBUTING.md`](CONTRIBUTING.md) for the full workflow and required checks.

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
- [`contracts/events/README.md`](contracts/events/README.md): event naming, envelope, framing, and failure rules.
- [`contracts/openapi/README.md`](contracts/openapi/README.md): HTTP contracts and shared error behavior.
- [`migrations/README.md`](migrations/README.md): migration conventions and commands.
- [`infra/nginx/README.md`](infra/nginx/README.md): local edge routing.
- [`tests/integration/README.md`](tests/integration/README.md): broker-backed test setup.
- [`CONTRIBUTING.md`](CONTRIBUTING.md): development and review workflow.

## Current scope

Implemented: repository structure, service entry points, health and readiness endpoints, environment configuration with case-insensitive and trimmed values, structured logging, request IDs, graceful shutdown, structured panic recovery, JSON 404/405 responses, a 1 MiB HTTP header limit, shared infrastructure, event envelope and message framing validation, bounded RabbitMQ retry and dead-letter routing with publisher confirms, reconnect backoff with jitter, unit and broker-backed messaging integration tests, one Playwright example test run Chromium-only in CI, Dockerfiles, Compose infrastructure, Nginx routing, migration and event conventions, Nx targets, CI, and documentation.

Not implemented: authentication, JWT, parcels, pricing formulas, merchant onboarding, API keys, notification delivery, webhook signing and delivery retries, realtime subscriptions, payments, external integrations, production TLS, Kubernetes, Terraform, and production deployment automation. No service connects to PostgreSQL, and no event producers or outbox exist yet.
