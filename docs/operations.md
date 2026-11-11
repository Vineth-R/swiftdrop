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
parcel: 6
merchant: 9
notification: 2
```

A database with no applied migrations may report
`no applied migrations or unreadable version`. That is not a clean status and
exits non-zero.

A dirty migration state prints the version followed by `(dirty)`:

```text
merchant: 9 (dirty)
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

- **Database-backed services and workers** (`identity`, `pricing`, `parcel`,
  `merchant`, `notification-worker`, and `realtime` when configured): ready only
  when the PostgreSQL ping succeeds. `/ready` returns
  `{"status":"ready","checks":{"database":"ok"}}`, or `503` with the database
  check set to `down` when `DATABASE_URL` is unset or unreachable.
- **`webhook-worker`**: the Compose health check is process health only. It has
  no HTTP readiness endpoint and no published port; it does not prove broker
  connectivity or that messages are being consumed.
- **`ai-assistant`**: readiness is process health; a missing Gemini key leaves
  the process running with an unconfigured provider and returns
  `503 AI_UNAVAILABLE` on chat instead of taking down the gateway.
- **`realtime`**: readiness checks PostgreSQL when configured, not the RabbitMQ
  consumer. A ready pod can still be reconnecting to the broker.
- **`notification-worker`**: its internal ADMIN read API is on port `8086` and
  is reachable only through the gateway; an unset `JWT_ACCESS_SECRET` leaves
  delivery running but makes that API fail closed.

RabbitMQ failures are visible in structured logs and through the management UI,
not through most `/ready` endpoints. Treat a green `/ready` as "the service can
serve database-backed requests", not "every dependency is healthy".
