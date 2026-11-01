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

## Naming and versioning

- `NNNNNN_short_description.up.sql`
- `NNNNNN_short_description.down.sql`

Use a zero-padded, monotonically increasing six-digit sequence. Applied migrations are immutable: fix mistakes with a new migration, never by editing or renumbering an applied file. Versions must be unique per service directory; gaps are allowed.

Current chain:

- `000001_initial_schema` creates the service schema.
- `000002_tables` is the M1 baseline from [`docs/db.sql`](../docs/db.sql): 26 tables, triggers, indexes, and reference seeds.
- `000003_dispatch_foundations` (parcel) extends the baseline `parcel_assignments` and `parcel.outbox_events` tables with reassignment metadata, an assignment `updated_at` trigger, and outbox aggregate metadata. It uses additive `ALTER TABLE` statements and does not recreate the baseline tables.
- `000006_merchant_constraints` is the merchant service's forward migration. It adds the stricter merchant CHECK constraints through additive `ALTER TABLE` statements.

**Parcel history:** a dispatch feature branch briefly proposed `000002_parcels` and `000003_parcel_assignments_and_outbox`. Both duplicated tables already created by the applied `000002_tables` baseline, and the second reused the `000003` version. They were never applied to the shared baseline database and were replaced by `000003_dispatch_foundations` before merge. Parcel migrations continue from `000004`.

**Merchant history:** `000002_merchants`, `000003_api_clients`, `000004_orders_and_events`, and `000005_audit_logs` were removed. They duplicated the applied `000002_tables` baseline (including its version number) and re-created existing tables without `IF NOT EXISTS`, which made the migration source invalid. They were never applied to the shared baseline database. The useful constraints were preserved in `000006_merchant_constraints`; redundant index variants were not re-added. Merchant migrations continue from `000007`.

A database that somehow applied the merchant variant as version 2 instead of the baseline is not supported; it lacks baseline-only tables such as `merchant_webhooks` and `webhook_delivery_attempts` and must be rebuilt by its owner. The shared and fresh databases follow the baseline-first chain.

## Tool

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate) through the Compose `migrate` service, which pins `migrate/migrate:v4.18.3`. No host installation is required. `scripts/migrate.sh` runs inside that container.

## Commands

Apply every service's migrations (PostgreSQL must be running):

```sh
docker compose up -d postgres
make migrate
```

`make migrate`:

1. loads the Compose-resolved `DATABASE_URL` (built from `.env` or exported `POSTGRES_*` variables) inside the migrate container;
2. bootstraps `public.set_updated_at()` in the postgres container;
3. applies identity → pricing → parcel → merchant → notification, each with its own `<service>_schema_migrations` version table.

The runner never prints the connection URL or password. A missing `.env` and no exported `POSTGRES_USER`/`POSTGRES_DB` fails preflight with a clear error.

Roll back one migration per service (reverse order):

```sh
make migrate-down
```

This is **destructive**: on services currently at version 2 it drops their schema and tables. Use it only on a disposable database.

Reset local data and rebuild:

```sh
make db-reset
```

`make db-reset` runs `docker compose down -v`, which deletes the PostgreSQL and RabbitMQ volumes. Do not run it against shared data.

## Custom credentials

Set `POSTGRES_USER`, `POSTGRES_PASSWORD`, and `POSTGRES_DB` in `.env` or export them. The same values are used by the postgres container, the services, and the migration runner. Passwords containing URL-reserved characters (`@ : / ? # [ ] %`) are a known limitation because Compose builds the URL from these values; use URL-safe credentials for now. See the root README section “Custom database credentials” for the same restriction.

## Troubleshooting

- **Duplicate migration version:** two files share a six-digit prefix. Applied versions are immutable; delete only files that were never applied, or add a new forward migration. Never renumber an applied migration.
- **Dirty database version:** a migration failed mid-run. Inspect `docker compose logs migrate` and the service's `<service>_schema_migrations` row. On an isolated database, fix the cause and use `migrate force <version>`; never force the shared database without review.
- **Authentication failure:** verify `POSTGRES_USER`/`POSTGRES_PASSWORD` match the initialized volume. Changing them does not alter an existing volume; create an isolated project or reset only disposable data.
- **Missing `set_updated_at()`:** run `make migrate`; it bootstraps the function before applying migrations.

## Startup policy

Services never run migrations at startup. `make up` starts existing images, waits for health, migrates, and verifies gateway routes; `make up-build` builds the images first. Plain `docker compose up` does not migrate.
