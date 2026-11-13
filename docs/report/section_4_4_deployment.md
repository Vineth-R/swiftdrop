# 4.4 Deployment

SwiftDrop is deployed locally as a Docker Compose stack. The same images are
built in CI for `linux/amd64` and published from a tag by the production
workflow. There is no Kubernetes deployment in the current phase.

## Compose topology

`docker-compose.yml` defines:

| Service                   | Image / build                    | Published port       | Health check                |
| ------------------------- | -------------------------------- | -------------------- | --------------------------- |
| `postgres`                | `postgres:16-alpine`             | none (internal)      | `pg_isready`                |
| `rabbitmq`                | `rabbitmq:4.1-management-alpine` | `5672`, `15672`      | `rabbitmq-diagnostics ping` |
| `mailpit`                 | `axllent/mailpit:v1.27`          | `1025`, `8025`       | `/readyz`                   |
| `s3` (storage profile)    | `versity/versitygw:v1.11.0`      | `7070`               | TCP 7070                    |
| `identity`                | `infra/docker/go.Dockerfile`     | none                 | `/ready`                    |
| `parcel`                  | same                             | none                 | `/ready`                    |
| `pricing`                 | same                             | none                 | `/ready`                    |
| `merchant`                | same                             | none                 | `/ready`                    |
| `realtime`                | same                             | none                 | `/ready`                    |
| `ai-assistant`            | same                             | none                 | `/ready`                    |
| `notification-worker`     | same                             | internal `8086` only | process + `/ready`          |
| `webhook-worker`          | same                             | none                 | process                     |
| `web`                     | `infra/docker/web.Dockerfile`    | none                 | `/`                         |
| `nginx`                   | `nginx:1.27-alpine`              | `8080`               | `/nginx-health`             |
| `migrate` (tools profile) | `migrate/migrate:v4.18.3`        | none                 | one-shot                    |

Only the gateway, RabbitMQ management and Mailpit publish host ports. The
database, services and workers stay on the Compose network. The storage profile
is opt-in.

## Startup and migrations

```sh
cp .env.example .env
make up-build
```

`make up-build` validates the environment, builds every image, starts the stack,
waits for health checks, applies all five service migrations and runs the route
checks. `make up` starts existing images, migrates and verifies routes.
`docker compose up --build -d` starts containers but does not migrate.

Migrations are versioned per service and applied with golang-migrate. A first
install creates the five schemas through
`infra/docker/postgres/init/01-schemas.sql`; migrations then create the baseline
tables and reference data. `make migrate-status` reports the version and dirty
state for each service.

## Configuration

Configuration is environment-based and documented in `.env.example`. Backend
services require an explicit `APP_ENV`; they do not load `.env` themselves.
Compose supplies service names as hostnames (`postgres`, `rabbitmq`, `identity`,
and so on). Secrets such as `JWT_ACCESS_SECRET`, `INTERNAL_TOKEN`,
`WEBHOOK_SECRET_KEY` and `GEMINI_API_KEY` are injected as environment values and
are never baked into images.

## Images and runtime hardening

- Go images use a multi-stage build, a static binary and an Alpine runtime.
- The web image uses Next.js standalone output on Node 24 Alpine.
- Both run as non-root users.
- CI builds and scans every image for `linux/amd64`, uploads an SBOM and runs
  Trivy for HIGH/CRITICAL findings.
- The release workflow builds every image, starts the stack with `--wait`,
  applies migrations, verifies the database baseline, checks routes and runs the
  registration/login/refresh/logout smoke test.

## Delivery and event infrastructure

Parcel creation and courier status updates write a transactional outbox row.
The relay publishes versioned events to the durable `swiftdrop.events` topic
exchange. Consumers use durable queues, bounded retries through
`<queue>.retry.v2` and a dead-letter queue. The notification worker sends email
through the configured SMTP provider (Mailpit locally). The webhook worker
delivers signed HTTPS webhooks with the six API v1.1 business retries.

## Data and storage

- PostgreSQL 16 stores the five service schemas. Local data is kept in the
  `postgres-data` volume.
- Private proof photos use local filesystem storage by default, VersityGW for
  the local S3-compatible profile, or Neon Object Storage in production.
- Backups are written outside the repository with `make db-backup`; restore
  requires an explicit target and environment.

## CI/CD

GitHub Actions runs formatting, vet, race tests, coverage artifacts, build,
PostgreSQL and RabbitMQ integration tests, the event-flow suite, frontend
lint/test/typecheck/build, Chromium and real-stack Playwright, migration
validation, Docker builds, SBOM/Trivy, CodeQL and Gitleaks. The required gate
aggregates the mandatory jobs. The tag release job builds all images, starts
Compose, migrates, verifies the database and routes, and runs the auth smoke
test.

## Operations

See [`docs/operations.md`](../operations.md) for startup, shutdown, migrations,
safe seeding and reset, logs, readiness, RabbitMQ and dead-letter queues,
notification and webhook troubleshooting, database troubleshooting, configuration
and backup/recovery.
