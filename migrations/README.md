# Database migrations

SwiftDrop uses a single PostgreSQL 16 instance for local development, with one schema per service:

| Service      | Schema         | Migration directory       |
| ------------ | -------------- | ------------------------- |
| Identity     | `identity`     | `migrations/identity`     |
| Parcel       | `parcel`       | `migrations/parcel`       |
| Pricing      | `pricing`      | `migrations/pricing`      |
| Merchant     | `merchant`     | `migrations/merchant`     |
| Notification | `notification` | `migrations/notification` |

Each service owns its schema. A service must not read or write another service's schema directly. Cross-service data exchange belongs in HTTP APIs or RabbitMQ events.

## Naming convention

- `NNNNNN_short_description.up.sql`
- `NNNNNN_short_description.down.sql`

Use a zero-padded, monotonically increasing six-digit sequence. Applied migrations are immutable. A correction is a new migration, not an edit to an applied file.

`000001_initial_schema` creates the service schema. `000002_tables` is reserved by the platform foundation for the baseline tables from `docs/db.sql`. Other owners add new migrations from `000003`; do not renumber or reuse an applied version.

## Tool

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate). The Compose `migrate` service pins `migrate/migrate:v4.18.3`; no host installation is required.

## One-command workflow

Run everything from the repository root with PostgreSQL started:

```sh
docker compose up -d postgres
make migrate
```

`make migrate` bootstraps the shared `public.set_updated_at()` function (so pre-existing volumes work) and then applies every service's migrations in a safe order: identity, pricing, parcel, merchant, notification. Each service uses its own version table (`<service>_schema_migrations`).

On Windows with Git Bash and without GNU Make, use `bash scripts/migrate.sh`. To migrate one service, use `bash scripts/migrate.sh pricing`.

The runner uses Compose's configured database connection without printing credentials. It rejects dirty or ambiguous Merchant migration metadata rather than forcing a version.

## Merchant history reconciliation

The original Merchant contributions contained two different version-2 migrations. The canonical root retains `000001_initial_schema` and `000002_tables`; the original alternative versions 1–5 are preserved byte-for-byte under `merchant/legacy`. Do not apply both histories to one database.

`scripts/migrate.sh` detects a canonical fresh/version-1/version-2 schema or the original legacy version-2/version-3/version-4/version-5 schema. For a legacy database it assembles the matching historical migration source before upgrading. New migration `000006_reconcile_baseline` preserves rows and legacy constraints, restores UUID defaults, and adds the missing baseline webhook tables and triggers. Version 6 is the common continuation point. Future Merchant changes start at `000007`; other owners continue above their latest applied version (at least `000003`).

Migration 6 is forward-only. `make migrate-down` is disabled to prevent partial batch rollback or accidental data deletion. Investigate migration failures and add a forward correction; never edit an applied migration or use `force` to conceal a mismatch.

## Verification

Run the isolated migration scenarios against local PostgreSQL:

```sh
bash scripts/test-migrations.sh
```

The script checks historical SQL against the frozen Git blob hashes in `merchant/legacy/git-blob-hashes.txt`, so the check remains valid after the files move out of the canonical source. It upgrades canonical, legacy-2, and legacy-5 fixtures in uniquely named disposable databases, verifies version 6, table/default counts and preservation of an existing row, then checks repeat execution. Cleanup drops only databases created by that invocation. It never resets application volumes.

For direct inspection of an individual canonical service, the equivalent CLI pattern is:

```sh
DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable'

migrate \
  -path migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" \
  up

migrate \
  -path migrations/parcel \
  -database "${DATABASE_URL}&x-migrations-table=parcel_schema_migrations" \
  up

migrate \
  -path migrations/pricing \
  -database "${DATABASE_URL}&x-migrations-table=pricing_schema_migrations" \
  up

migrate \
  -path migrations/merchant \
  -database "${DATABASE_URL}&x-migrations-table=merchant_schema_migrations" \
  up
```

The migration version tables live in the default `public` schema. Service-owned tables must always use their service schema explicitly.

HTTP services open a PostgreSQL pool when `DATABASE_URL` is configured and report connectivity through `/ready`. Baseline migrations create business tables and reference seed data from `docs/db.sql`; Pricing and public tracking use their owning schemas.

Do not use a reset as a verification command. `make db-reset` deliberately deletes local PostgreSQL and RabbitMQ volumes and requires explicit authorization.

```sh
docker compose ps
```

Use the migration tests and readiness endpoints to verify setup without deleting data.

## Manual commands

```sh
DATABASE_URL='postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable'

docker compose run --rm migrate \
  -path /migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" up
```

The migration version tables live in the default `public` schema. Service-owned tables must always use their service schema explicitly.

## Startup policy

Services do not run migrations automatically at startup. Run migrations explicitly as part of local setup or a deployment pipeline so schema changes are observable, reviewable, and independent of application restarts.
