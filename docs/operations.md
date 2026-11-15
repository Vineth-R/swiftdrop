# Operations runbook

Short, copy-pasteable operational procedures for local Compose and the
database. This file does not replace `README.md` or
[`docs/architecture.md`](architecture.md); it records the exact commands and
the expected result for routine operations.

## Clean-clone Compose startup

Prerequisites: Docker with Compose v2, `make`, `curl`, and (for the seed steps)
Go on the host.

```sh
git clone https://github.com/Vineth-R/swiftdrop.git
cd swiftdrop
cp .env.example .env
make up-build
make migrate
make seed
make seed-merchant
```

Expected results:

1. `cp .env.example .env` creates a local, untracked file with safe
   placeholders. Edit it only if your ports or local configuration differ.
2. `make up-build` validates the Compose configuration, builds every image,
   starts the stack, waits up to 300 seconds for health checks, applies all
   service migrations, and runs `scripts/check-routes.sh`. The final lines are
   the route checks passing. The web app is at <http://localhost:8080/>, Mailpit
   at <http://localhost:8025/>, and RabbitMQ management at
   <http://localhost:15672/>.
3. `make migrate` is safe to re-run and reports `no change` when every service
   is already current. `make up-build` already ran it.
4. `make seed` creates the five deterministic identity demo users. A rerun
   reports zero created and five skipped and never overwrites existing data.
5. `make seed-merchant` creates an APPROVED merchant and its MERCHANT identity
   owner. A rerun reports `already applied` and makes no changes.

### Seed prerequisites on a Compose checkout

The seed commands are host-side Go programs. They read process environment
variables directly and do **not** load `.env`. Compose PostgreSQL and Identity
are internal to the Compose network and do not publish host ports, so the
default `.env` `DATABASE_URL` (`localhost:5432`) cannot reach the Compose
database. Before either seed:

```sh
set -a
. ./.env
set +a
# Override the host-side values after sourcing .env.
export DATABASE_URL='postgres://swiftdrop:swiftdrop@<host-reachable-db>:5432/swiftdrop?sslmode=disable'
export IDENTITY_BASE_URL='http://<host-reachable-identity>:8081'
export INTERNAL_TOKEN='<the same local internal token the services use>'
```

For `make seed`, also export the ten `SEED_*` identity variables. For
`make seed-merchant`, export the `SEED_MERCHANT_*` variables; there is no
password default:

```sh
export SEED_MERCHANT_PASSWORD='<your-local-password>'
make seed-merchant
```

`make seed-merchant` refuses to run when `APP_ENV=production`. It also refuses a
non-local database host unless `ALLOW_REMOTE_SEED=1` is set, and it never prints
the password, `INTERNAL_TOKEN`, or `DATABASE_URL`.

## Database and backup safety

- Compose PostgreSQL data lives in the `postgres-data` volume. `make down` keeps
  it; `make db-reset` deletes it and is destructive.
- Never run `docker compose down -v`, `make db-reset`, `make migrate-down`, or a
  restore against a database that holds data you need. Confirm the target host
  and project name first with `make project-name`.
- `make migrate-down` rolls back one migration per service and requires
  `APP_ENV`; it refuses `APP_ENV=production` unless
  `ALLOW_PRODUCTION_ROLLBACK=1` is set as part of an approved plan.
- Backups are written outside the repository, are verified with
  `pg_restore --list`, and get a SHA-256 checksum:

  ```sh
  make db-backup
  # or, for a host-reachable URL:
  BACKUP_SOURCE=url DATABASE_URL='postgres://...' make db-backup-remote
  ```

  Expected: a path under `${HOME}/swiftdrop-backups` (or `BACKUP_DIR`),
  `verified: pg_restore --list succeeded`, and a `.sha256` file. Never commit
  dumps or checksums.

- A restore overwrites a target and needs an explicit target and environment:

  ```sh
  APP_ENV=development \
  DATABASE_URL_DIRECT='postgres://...' \
  make db-restore BACKUP=/path/to/swiftdrop-<timestamp>.dump
  ```

  The checksum is verified when the `.sha256` file is present. After a restore,
  run `make migrate-status` and do not run `make migrate` again unless the
  restored database is behind the local migrations.

- `RESTORE_CLEAN=1` additionally drops pre-existing objects. Use it only on a
  disposable target.
- Migrations are forward-only in practice. Applied migrations are immutable;
  fix mistakes with a new migration, not by editing history.

## Migration status

```sh
make migrate-status
```

Expected for a healthy stack: one line per service with a plain numeric
version, for example:

```text
identity: 6
pricing: 2
parcel: 7
merchant: 11
notification: 2
```

A database with no applied migrations may report
`no applied migrations or unreadable version`. That is not a clean status and
exits non-zero.

A dirty migration state prints the version followed by `(dirty)`:

```text
merchant: 11 (dirty)
```

and exits non-zero. Do not apply more migrations until the dirty version is
investigated. golang-migrate's `version` command may also emit
`Dirty database version <n>. Fix and force version.`; the script reports that
as `dirty migration state` instead of a clean version. Resolve the failed
statement manually, then use an approved `migrate force <version>` operation —
never guess a version or delete a version table row.

To create a new migration pair:

```sh
make migrate-create SERVICE=merchant NAME=description_of_change
```

## Readiness limits

`GET /health` and `/ready` are the primary probes; `/healthz` and `/readyz` are
deprecated aliases. Through the gateway only `/healthz` is public. Inspect a
service directly with Compose:

```sh
docker compose exec identity wget -qO- http://127.0.0.1:8081/ready
```

`/ready` currently means:

- **`identity` and `pricing`**: ready when the PostgreSQL ping succeeds.
- **`parcel`**: ready when PostgreSQL answers and, when publishing is configured,
  its outbox relay has a usable broker connection.
- **`merchant`**: ready when PostgreSQL answers and, when status sync is enabled,
  its merchant-sync consumer is healthy.
- **`realtime`**: ready when PostgreSQL answers and both the status and
  assignment consumers are healthy.
- **`notification-worker`**: ready when PostgreSQL answers and all three
  consumers are healthy. Its internal ADMIN read API is on port `8086` and is
  reachable only through the gateway; an unset `JWT_ACCESS_SECRET` leaves
  delivery running but makes that API fail closed.
- **`webhook-worker`**: exposes internal `/health` and `/ready` on port `8088`;
  readiness requires its consumer. The port is not published or routed by
  Nginx.
- **`ai-assistant`**: `/ready` returns `503` when its provider is disabled or
  unconfigured; Compose uses `/health` because the AI capability is optional.

A `503` includes the failing checks. `/health` remains liveness so a reconnecting
consumer or a temporary dependency outage does not trigger a restart loop.

## Logs

```sh
make logs                                  # every service, follow, last 100 lines
docker compose logs -f --tail=200 parcel   # one service
docker compose logs --since=15m notification-worker
```

Services log structured `slog` output. Request logs include method, path, status,
latency and request ID; they never include query strings, bodies, tokens or
passwords. Panic logs include a redacted panic value and type. If a log line
contains `[REDACTED]`, that is expected for sensitive attributes.

## Health and readiness

```sh
docker compose ps
docker compose exec identity wget -qO- http://127.0.0.1:8081/ready
docker compose exec parcel   wget -qO- http://127.0.0.1:8082/ready
docker compose exec web      wget -qO- http://127.0.0.1:3000/
curl -s http://localhost:8080/healthz
```

`/ready` returns `200 {"status":"ready","checks":{...}}` when all required
checks succeed and `503` with the failing checks when they do not.
`webhook-worker` exposes internal `/health` and `/ready` on port `8088`; it is
not published or routed by Nginx.

## RabbitMQ queues and dead letters

Management UI: <http://localhost:15672/> (default `swiftdrop` / `swiftdrop` in
the Compose template).

```sh
docker compose exec rabbitmq rabbitmqctl list_queues name messages consumers
docker compose exec rabbitmq rabbitmqctl list_bindings
```

Expected durable queues:

- `notification.requested.v1`
- `notification.parcel.created.v1`
- `notification.parcel.status_changed.v1`
- `webhook.parcel.status_changed.v1`
- `realtime.parcel.status_changed.v1`
- `merchant.sync.queue`

Retry and dead-letter resources are derived from each queue:
`<exchange>.retry`, `<queue>.retry.v2`, `<exchange>.dlx`, `<queue>.dlq`. Inspect
a dead-letter queue in the UI before discarding messages. A message in a DLQ is
a failed event, not a transient broker problem; fix the cause and republish or
discard deliberately.

## Notification troubleshooting

1. Check `docker compose logs notification-worker`.
2. Confirm `notification.parcel.created.v1` and
   `notification.parcel.status_changed.v1` have consumers.
3. Confirm the customer or approved merchant contact resolves through the
   internal APIs.
4. Check Mailpit at <http://localhost:8025/> for the email.
5. Inspect `/api/v1/admin/notification-logs` as ADMIN, or the
   `/admin/notification-logs` screen.

Expected outcomes: a `SENT` log with a masked recipient on success; `PENDING`
during a retryable SMTP failure; `FAILED` for an invalid message. A permanent
failure (unapproved merchant, unknown contact, SMS) dead-letters without an
email. `processed_events` prevents a successful event from being sent twice.

## Webhook troubleshooting

1. `docker compose logs webhook-worker`.
2. Inspect `merchant.webhook_delivery_attempts` through the merchant delivery
   log (`GET /api/v1/merchants/me/webhook/deliveries`) or the merchant webhook
   screen.
3. Confirm the URL is HTTPS, public and not a loopback/private address.
4. Confirm `WEBHOOK_SECRET_KEY` is valid, and that the key named by a stored
   `v2:<keyId>:...` value is present in `WEBHOOK_SECRET_KEYS`; the worker reads
   the decrypted secret through the Merchant internal API.
5. Check the six attempts (1m, 5m, 30m, 2h, 6h). Attempt 6 failure is dead
   lettered.

The worker suppresses an event that already has a successful attempt. This is
durable, but it cannot close the window where the receiver accepted the request
and the worker crashed before recording it. Receivers must deduplicate on
`eventId`; delivery is at-least-once, not exactly-once.

## Metrics and readiness

Every HTTP service exposes `GET /metrics` on its internal port (for example
`docker compose exec parcel wget -qO- http://127.0.0.1:8082/metrics`). The
gateway does not route `/metrics`.

Readiness includes required workload dependencies:

- Realtime requires PostgreSQL and both consumers.
- Notification worker requires PostgreSQL and all three consumers.
- Merchant includes the sync consumer when enabled.
- Parcel includes the outbox relay when publishing is configured.
- Webhook worker exposes internal `/health` and `/ready` on `8088`.
- AI `/ready` is `503` when its provider is disabled or unconfigured; Compose
  uses `/health` because AI is optional.

Useful checks:

```sh
docker compose exec realtime wget -qO- http://127.0.0.1:8085/ready
docker compose exec notification-worker wget -qO- http://127.0.0.1:8086/ready
docker compose exec webhook-worker wget -qO- http://127.0.0.1:8088/ready
docker compose exec parcel wget -qO- http://127.0.0.1:8082/metrics | grep swiftdrop_outbox
```

Identity deletes expired refresh/reset/verification tokens after
`IDENTITY_TOKEN_RETENTION` (default 30 days) at startup and every
`IDENTITY_TOKEN_CLEANUP_INTERVAL` (default 24 hours).

Notification processed events are retained for 30 days. Published outbox rows are
retained for 14 days. Merchant order reservations and notification delivery logs
currently have no automatic deletion; their retention policy needs owner
approval because deleting them can weaken duplicate-order protection or delivery
auditability.

Alert thresholds and the deferred monitoring stack are listed in
[phase-2-security-reliability.md](phase-2-security-reliability.md).

## Performance checks

Local k6 smoke/load scenarios live in `tests/perf`. They use the public stack and
do not define production SLOs:

```sh
k6 run tests/perf/public.js
k6 run tests/perf/websocket.js
PERF_EMAIL=... PERF_PASSWORD=... k6 run tests/perf/auth.js
```

The recorded baseline is in
[phase-3-performance-baseline.md](phase-3-performance-baseline.md).

## Database troubleshooting

```sh
docker compose exec postgres pg_isready -U "$POSTGRES_USER" -d "$POSTGRES_DB"
make migrate-status
make db-verify
docker compose exec postgres psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c '\dn'
```

Common causes:

- `dirty` migration state: investigate the failed statement, then use an
  approved `migrate force` operation. Never delete a version row by guesswork.
- `no applied migrations or unreadable version`: apply migrations.
- `database: down` on `/ready`: the pool cannot reach PostgreSQL; check the
  container, credentials and network.
- `too many connections`: lower per-service pool sizes or raise PostgreSQL
  limits; see the `DB_*` pool variables.

## Configuration problems

- `APP_ENV is required`: set `APP_ENV=development`, `staging` or `production`.
- `JWT_ACCESS_SECRET is required`: set at least 32 random bytes outside
  development.
- `INTERNAL_TOKEN` shorter than 32 bytes outside development: use a strong
  random token; internal routes otherwise stay fail-closed.
- Webhook routes answer `503`: set `JWT_ACCESS_SECRET` and a valid
  `WEBHOOK_SECRET_KEY` (64 hex characters).
- Webhook decryption fails after a rotation: keep the previous key in
  `WEBHOOK_SECRET_KEYS` as `id=hex64` until every stored secret has been
  re-saved. Legacy `v1` values upgrade automatically on the next test send or
  worker read; see [phase-2-security-reliability.md](phase-2-security-reliability.md).
- Authenticated parcel/pricing/realtime requests answer `503`: identity session
  state cannot be read. Check `IDENTITY_BASE_URL`, `INTERNAL_TOKEN` and the
  identity `/ready`; the check fails closed by design.
- AI returns `503 AI_UNAVAILABLE`: set a valid `GEMINI_API_KEY` or use
  `AI_PROVIDER=mock` for a local stack.
- Compose cannot resolve an upstream: restart Nginx after service recreated;
  Compose depends on health with `restart: true`, but a manual partial start may
  need `docker compose restart nginx`.

## Backup and recovery

```sh
make db-backup
BACKUP_SOURCE=url DATABASE_URL='postgres://...' make db-backup-remote
APP_ENV=development DATABASE_URL_DIRECT='postgres://...' \
  make db-restore BACKUP=/path/to/swiftdrop-<timestamp>.dump
```

Backups are written outside the repository, verified with `pg_restore --list`,
and accompanied by a SHA-256 checksum. A restore overwrites the target; confirm
the host and project first with `make project-name`. After a restore, run
`make migrate-status`. Proof objects are backed up separately from PostgreSQL.
