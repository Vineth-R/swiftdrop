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
- `000003_oauth_states_browser_binding` (identity) adds the nullable `oauth_states.browser_binding_hash` column used to bind a Google login attempt to the browser that started it. Existing rows keep `NULL` and expire normally.
- `000004_email_verification` (identity) adds `identity.email_verification_tokens` and its user/expiry/one-unused indexes.
- `000005_session_version` (identity) adds `identity.users.session_version` (`INTEGER NOT NULL DEFAULT 0`, `>= 0`).
- `000006_merchant_lifecycle_transitions` (identity) adds `identity.merchant_lifecycle_transitions` and its user/sequence constraints.
- `000007_registration_onboarding` (merchant) adds `business_address`, `lifecycle_sequence`, the registration attempt and status-transition tables, the business registration index/constraint, and widens `rejection_reason` to 1000 characters.
- `000008_business_reg_no_optional` (merchant) is a forward compatibility migration: the public onboarding flow validates `registrationNumber`, but the merchant create/repository path also permits it to be omitted, and `000007` added `merchants_business_reg_no_present` (NOT VALID still rejects new rows). `000008` drops only that database constraint; the columns, indexes and registration/status tables from `000007` remain.
- `000003_dispatch_foundations` (parcel) extends the baseline `parcel_assignments` and `parcel.outbox_events` tables with reassignment metadata, an assignment `updated_at` trigger, and outbox aggregate metadata. It uses additive `ALTER TABLE` statements and does not recreate the baseline tables.
- `000004_price_breakdown_total` (parcel) re-validates `parcels.price_breakdown` so `price_amount` equals the three components and JSON nulls cannot bypass the total. It replaces the baseline constraint with `NOT VALID` + `VALIDATE CONSTRAINT` and does not change stored data.
- `000005_outbox_event_types` (parcel) reasserts the approved unversioned outbox contract, widens `outbox_events.last_error` to `TEXT`, and adds the published-event cleanup and current-courier workload indexes. It does not store `.v1` labels; the relay maps storage labels to versioned bus names.
- `000006_customer_parcel_list_index` (parcel) adds the additive `idx_parcels_customer_created_id` btree on `(customer_user_id, created_at, id)` for the ordered customer list. The existing `parcels_customer_idx` only covers the owner predicate; the new index supports both sort directions with a backward scan and does not modify any applied migration.
- `000006_merchant_constraints` is the merchant service's forward migration. It adds the stricter merchant CHECK constraints through additive `ALTER TABLE` statements.
- The baseline remains 26 tables; identity `000004`-`000006` and merchant `000007` add four additive tables, so the complete chain is 30 tables.

**Parcel outbox contract (000005):** the outbox stores only the unversioned labels and exact pairs `parcel.created` → `parcel.created`, `parcel.assigned` → `parcel.assigned`, and `parcel.status_changed` → `parcel.status.changed`. The underscore in the event type and the dots in the status routing key are intentional. `000005` reasserts these constraints, rejects `.v1` labels and mixed pairs at the database, and does not change the relay's responsibility to map storage labels to the versioned bus events (`parcel.created.v1`, `parcel.assigned.v1`, `parcel.status_changed.v1`). It widens `last_error` to `TEXT` and adds `idx_outbox_events_published_cleanup` for published-event retention and `idx_parcel_assignments_courier_workload` for the current-assignment workload query. The existing `outbox_status_created (status, created_at)` index already serves the pending FIFO relay poll; a representative 100k-row local `EXPLAIN (ANALYZE, BUFFERS)` returned the first 100 pending rows in about 1.4 ms, so `000005` intentionally does not add a duplicate partial pending index. The workload index reduced the grouped current-assignment scan from about 6.9 ms and 53 index buffers to about 5.2 ms and 18 index buffers on the same data, so it is retained.

**Pricing and tracking (M4):** no new migrations were required. `pricing/000002_tables` already creates `zones`, `pricing_rules`, `weight_bands`, `size_surcharges` and `audit_logs`, including indexes, checks, the active-route unique index, the `(min, max]` exclusion constraint, and the reference seed (CMB/KDY and the 550.00 LKR example). Public tracking reads the existing `parcel.parcels` and `parcel.parcel_status_history` tables and the existing unique tracking-number index. The feature branch's proposed merchant reconciliation and legacy merchant migrations were not adopted: the canonical `000002_tables` already contains the webhook tables, UUID defaults and key-limit trigger, and the shared database is at canonical merchant version 6.

**M7.1 baseline:** `parcel.delivery_proofs` is already provided by [`parcel/000002_tables.up.sql`](parcel/000002_tables.up.sql); `notification.notification_logs` and `notification.processed_events` are already provided by [`notification/000002_tables.up.sql`](notification/000002_tables.up.sql). These applied baseline files remain immutable. M7.1 adds verification rather than duplicate `000003` migrations for these tables.

**Parcel history:** a dispatch feature branch briefly proposed `000002_parcels` and `000003_parcel_assignments_and_outbox`. Both duplicated tables already created by the applied `000002_tables` baseline, and the second reused the `000003` version. They were never applied to the shared baseline database and were replaced by `000003_dispatch_foundations` before merge. Parcel migrations continue from `000004`; the current parcel head is `000006_customer_parcel_list_index`.

**Merchant history:** `000002_merchants`, `000003_api_clients`, `000004_orders_and_events`, and `000005_audit_logs` were removed. They duplicated the applied `000002_tables` baseline (including its version number) and re-created existing tables without `IF NOT EXISTS`, which made the migration source invalid. They were never applied to the shared baseline database. The useful constraints were preserved in `000006_merchant_constraints`; redundant index variants were not re-added. Merchant `000008_business_reg_no_optional` is the current head; the next merchant migration starts at `000009`.

A database that somehow applied the merchant variant as version 2 instead of the baseline is not supported; it lacks baseline-only tables such as `merchant_webhooks` and `webhook_delivery_attempts` and must be rebuilt by its owner. The shared and fresh databases follow the baseline-first chain.

## Tool

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate) through the Compose `migrate` service, which pins `migrate/migrate:v4.18.3`. No host installation is required. `scripts/migrate.sh` runs inside that container.

## Neon and direct connections

Application services use `DATABASE_URL` (Neon pooled host, `sslmode=require`).
Migrations use `DATABASE_URL_DIRECT` (Neon direct host) and fall back to
`DATABASE_URL` when it is unset, so local development keeps working without
change. The runner never prints either URL.

```sh
make migrate-status   # versions and dirty state
make migrate          # apply pending migrations
make migrate-down     # one version per service; refused when APP_ENV=production
make migrate-create SERVICE=parcel NAME=add_my_index
```

`scripts/bootstrap-db.sh` creates the five schemas and
`public.set_updated_at()` before migrations. It uses the local Compose service
or `DATABASE_URL_DIRECT` for a remote database. See
[`docs/neon-production.md`](../docs/neon-production.md).

**Development history.** Development already contains identity `000004`-`000006`
and merchant `000007`; they are unchanged by this branch. Merchant `000008` is
the only new migration. The production migration work was based on an older
feature tree that lacked those files; they were recovered there for verification
and produced an inventory identical to the source database. Development did not
need that recovery. The ahead-of-local version guard remains in place as a
safety net; no version rows were deleted or forced.

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

| Migration rolled back                                  | Effect                                                                                                                                                                                                                                                                                |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `000002_tables` (version 2 → 1)                        | Drops that service's baseline tables and their data, plus explicitly declared migration-owned triggers/functions where applicable. The service schema remains.                                                                                                                        |
| `000005_outbox_event_types` (parcel version 5 → 4)     | Restores `outbox_events.last_error` to `VARCHAR(255)`, reasserts the baseline outbox CHECK constraints, and drops the two indexes added by `000005`. Fails atomically with a clear error if any `last_error` value is longer than 255 characters; it never truncates diagnostic data. |
| `000001_initial_schema`                                | Drops the service schema with `CASCADE`, including any remaining objects. This is a separate rollback from version 1.                                                                                                                                                                 |
| `000006_merchant_constraints` (merchant version 6 → 2) | Removes the added CHECK constraints; retains the merchant schema and baseline tables. Version gaps do not imply extra rollback steps.                                                                                                                                                 |

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
