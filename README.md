# SwiftDrop

SwiftDrop is a parcel delivery and courier management platform. This repository contains the initial production-oriented foundation: a Next.js frontend, independently runnable Go services, shared Go infrastructure, PostgreSQL migrations, RabbitMQ event contracts, Nginx routing, local Docker Compose infrastructure, Nx targets, and CI.

Business functionality such as authentication, parcel lifecycles, pricing rules, merchant onboarding, live tracking, notification delivery, and webhook retries is intentionally not implemented yet.

## Technology

- **Frontend:** Next.js 16 App Router, React 19, TypeScript, pnpm, Nx.
- **Backend:** Go 1.27, Gin, one root Go module.
- **Data:** PostgreSQL 16 with service-owned schemas and versioned SQL migrations.
- **Messaging:** RabbitMQ 4.x with durable topic exchanges and versioned JSON event contracts.
- **Edge:** Nginx local reverse proxy.
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

Services communicate synchronously over HTTP when a caller needs an immediate result. Asynchronous work is published to RabbitMQ. Each service owns its PostgreSQL schema and must not modify another service's tables. See [`docs/architecture.md`](docs/architecture.md) for routing, data ownership, event conventions, and readiness semantics.

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
tests/integration/          Future integration-test conventions
.github/workflows/          CI
```

Each Go service has `main.go`, `internal/config`, and either `internal/http` or `internal/worker`, with tests beside the code.

## Prerequisites

- Go **1.27.3** or a compatible 1.27 toolchain.
- Node.js **24** (the CI version) and pnpm **12.15.0** through Corepack.
- Docker with Docker Compose.
- GNU Make is not required; Nx and Go commands are used directly.
- Optional: `golang-migrate` CLI for database migrations.

## Environment setup

```sh
cp .env.example .env
pnpm install --frozen-lockfile
```

`.env.example` contains safe local placeholders only. Do not commit `.env`, credentials, private keys, or production secrets.

Docker Compose reads `.env` automatically. Go services read process environment variables and do not load `.env` themselves. For direct local execution, export the file first:

```sh
set -a
. ./.env
set +a
```

## Local development

### Run the frontend

```sh
pnpm exec nx dev @org/web
```

The application is available at <http://localhost:3000>. The existing `/api/hello` route remains available.

### Run an individual Go service

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

Replace `identity` and `8081` with another service and its port. Readiness currently reports process readiness only; it does not claim that PostgreSQL or RabbitMQ is reachable.

### Run a worker

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

Workers reconnect with capped exponential backoff when the broker is unavailable. They acknowledge a message only after the handler succeeds and reject it without requeue on handler failure. They do not expose a public HTTP API.

### Run Go tests and builds

```sh
go test ./...
go build ./...
gofmt -w apps/services packages/go-common
```

Unit tests do not require PostgreSQL or RabbitMQ.

### Run the integrated environment

```sh
docker compose up --build
```

Endpoints:

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

`docker compose down -v` also deletes PostgreSQL and RabbitMQ volumes. That is destructive; use it only when you intend to reset local data.

If Docker runs out of memory while building every Go image in parallel, build services individually and then start the stack:

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

## Database migrations

Migrations are organized by owning service under `migrations/<service>/` and use `NNNNNN_description.up.sql` and `.down.sql`. The initial migrations create each service schema only.

Install the migration CLI and run migrations explicitly:

```sh
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3

DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable'

migrate -path migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" up
```

Repeat for `parcel`, `pricing`, and `merchant`. Roll back with `down 1`. Full conventions and destructive-operation warnings are in [`migrations/README.md`](migrations/README.md). Services never run migrations automatically at startup.

## Messaging

Events use the durable topic exchange `swiftdrop.events`. Routing keys and event names are versioned, for example:

- `parcel.created.v1`
- `parcel.status_changed.v1`
- `notification.requested.v1`
- `webhook.delivery.requested.v1`

Consumers use explicit acknowledgements and must be idempotent. The initial workers validate the shared event envelope and log non-sensitive identifiers; no external providers or retry engine are implemented. See [`contracts/events/README.md`](contracts/events/README.md).

## API contracts

OpenAPI specifications live in [`contracts/openapi`](contracts/openapi). Nginx routes `/api/v1/<service>/` and strips that prefix before forwarding, so Gin exposes `/healthz` and `/readyz` at the service root. See [`infra/nginx/README.md`](infra/nginx/README.md).

## Nx commands

Verified project names include `identity`, `parcel`, `pricing`, `merchant`, `notification-worker`, `webhook-worker`, `realtime`, `go-common`, `@org/web`, and `@org/web-e2e`.

```sh
pnpm exec nx show projects

pnpm exec nx build identity
pnpm exec nx test identity
pnpm exec nx lint identity
pnpm exec nx serve identity

pnpm exec nx run-many -t lint test build \
  -p identity,parcel,pricing,merchant,notification-worker,webhook-worker,realtime,go-common

pnpm exec nx lint @org/web
pnpm exec nx build @org/web
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e
pnpm exec nx e2e @org/web-e2e
```

Go targets run from the repository root so the single root module and shared packages resolve correctly. Shared inputs (`go.mod`, `go.sum`, and `packages/go-common`) are part of every Go target's Nx cache key.

## Adding a new service

1. Create `apps/services/<name>/` with `main.go`, `internal/config`, and either `internal/http` or `internal/worker` plus tests.
2. Import shared code through `github.com/Vineth-R/swiftdrop/packages/go-common/...`.
3. Add `project.json` with `build`, `test`, `lint`, and `serve` targets using `nx:run-commands`.
4. Add a service-owned schema migration under `migrations/<name>/` if the service persists data.
5. Add an OpenAPI specification under `contracts/openapi/`.
6. If the service is public, add an Nginx route and matching Gin paths, then update the Compose service and documentation.
7. Run `gofmt`, `go test ./...`, `go build ./...`, and the Nx targets.

Do not create a nested `go.mod` or `go.work`. Do not give services a shared business-logic package or direct access to another service's tables.

## CI

`.github/workflows/ci.yml` runs:

- Go formatting, vet, tests, and builds with the root module.
- Frontend lint, typecheck, and build through Nx.
- Playwright E2E tests.
- Docker Compose configuration validation and representative image builds.

No deployment credentials or automatic deployments are configured.

## Current scope

Implemented: repository structure, service entry points, health endpoints, configuration, structured logging, graceful shutdown, shared infrastructure, unit tests, Dockerfiles, Compose infrastructure, Nginx routing, migration and event conventions, Nx targets, CI, and documentation.

Not implemented: authentication, JWT, parcels, pricing formulas, merchant onboarding, API keys, notification delivery, webhook signing/retry, realtime subscriptions, payments, external integrations, production TLS, Kubernetes, Terraform, and production deployment automation.
