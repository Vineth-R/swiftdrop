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
- `000006_merchant_constraints` is the merchant service's forward migration. It adds the stricter merchant CHECK constraints through additive `ALTER TABLE` statements.

**M7.1 baseline:** `parcel.delivery_proofs` is already provided by [`parcel/000002_tables.up.sql`](parcel/000002_tables.up.sql); `notification.notification_logs` and `notification.processed_events` are already provided by [`notification/000002_tables.up.sql`](notification/000002_tables.up.sql). These applied baseline files remain immutable. M7.1 adds verification rather than duplicate `000003` migrations for these tables.

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

This is **destructive** and applies to the database resolved by Compose; it does not create an isolated test database. Inspect the target and each service's current version first. The effect depends on which migration is being rolled back:

| Migration rolled back                                  | Effect                                                                                                                                                         |
| ------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `000002_tables` (version 2 → 1)                        | Drops that service's baseline tables and their data, plus explicitly declared migration-owned triggers/functions where applicable. The service schema remains. |
| `000001_initial_schema`                                | Drops the service schema with `CASCADE`, including any remaining objects. This is a separate rollback from version 1.                                          |
| `000006_merchant_constraints` (merchant version 6 → 2) | Removes the added CHECK constraints; retains the merchant schema and baseline tables. Version gaps do not imply extra rollback steps.                          |

For parcel, `000002 DOWN` drops `parcels`, `parcel_status_history`, `parcel_assignments`, `delivery_proofs` and `outbox_events`, together with the explicitly dropped parcel triggers/functions. For notification it drops `notification_logs` and `processed_events`. Neither `000002` DOWN file drops its schema; the corresponding `000001` DOWN files do. Reapplying `000002` recreates the baseline objects but cannot recover deleted rows. Use destructive rollback only on a disposable database, never the active development database.

### Isolated M7 rollback verification

From the repository root on Windows with Docker Desktop in Linux-container mode:

```powershell
powershell -NoProfile -File .\scripts\test-m7-database.ps1
```

This runner uses the existing cached `postgres:16-alpine` and `migrate/migrate:v4.18.3` images, a unique internal network and PostgreSQL container, and `tmpfs` storage with no existing project database volumes or published ports. It bypasses the Makefile/Unix wrapper, whose execution failed in the verified Windows environment. It does not read `.env` or target the active database.

The November 1, 2026 local run passed **95 SQL assertions before rollback**, rolled back only parcel/notification `000002` with both schemas retained at clean version 1, reapplied `000002` to clean version 2, and passed **the same 95 SQL assertions afterward**. It also passed **33 migration-state checks** across the initial, rolled-back and reapplied states. Version `000001` was never rolled back. Cleanup succeeded, including during a deliberate SQL-failure probe. See [M7.1 database verification](../tests/integration/README.md#m71-database-verification) for coverage, safety precautions and separate optional Go-validator results. These recorded checks cover parcel/notification baseline rollback, not every service or migration version.

### Reset local data (destructive)

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
