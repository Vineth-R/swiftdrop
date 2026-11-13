# Neon PostgreSQL production integration

SwiftDrop keeps PostgreSQL 16 in Docker for local development and uses the
existing Neon project for staging and production. The same Go code supports
both; only environment configuration changes.

> **Repository note (2026-11-11):** this document records the validated cutover
> to merchant migration 8. The repository now also contains
> `000009_restore_business_reg_no_required`, which restores the mandatory
> business-registration constraint. It has **not** been applied to production;
> production remains at merchant 8 until a separately approved migration window.

## Verified Neon configuration

These values were read from the Neon API before this integration:

| Item            | Value                                                                            |
| --------------- | -------------------------------------------------------------------------------- |
| Project         | `proud-rice-63339713` (`swiftdrop`)                                              |
| Organization    | `org-proud-block-98827753`                                                       |
| Region          | AWS US East 2 (Ohio), `aws-us-east-2`                                            |
| Default branch  | `production` (`br-round-paper-beim4u2g`)                                         |
| PostgreSQL      | 18                                                                               |
| Database / role | `neondb` / `neondb_owner`                                                        |
| Direct host     | `ep-curly-rice-beol9l7b.c-9.us-east-2.aws.neon.tech`                             |
| Pooled host     | `ep-curly-rice-beol9l7b-pooler.c-9.us-east-2.aws.neon.tech`                      |
| Object storage  | enabled at `https://br-round-paper-beim4u2g.storage.c-9.us-east-2.aws.neon.tech` |

The production branch is not recreated or reset by any script in this
repository. The staging branch is created only through an approved Neon
operation.

## Connection architecture

```text
Application services  -> DATABASE_URL          (Neon -pooler host, sslmode=require)
Migration job / CI    -> DATABASE_URL_DIRECT   (Neon direct host, sslmode=require)
Local development      -> DATABASE_URL          (postgres:5432, sslmode=disable)
                          DATABASE_URL_DIRECT unset -> migration runner falls back
```

- Application code uses `pgx/v5` + `pgxpool`. There is no GORM.
- Neon uses PgBouncer in transaction mode. Protocol-level prepared statements
  are supported; SQL-level `SET`, session advisory locks, and temporary tables
  with session persistence are not. SwiftDrop uses parameterized queries,
  transactions, and `pg_advisory_xact_lock` (transaction-scoped), which are
  compatible.
- `packages/go-common/postgres` rejects Neon URLs whose `sslmode` is missing or
  is `disable`, `allow`, or `prefer`.
- Readiness remains a bounded `Ping`; shutdown closes the pool after the HTTP
  server and Parcel relay have drained.

### Pool budget

Neon's 0.25 CU compute exposes roughly 112 connections (105 usable). The Compose
defaults are intentionally conservative per instance:

| Service             | `DB_MAX_OPEN_CONNS` |
| ------------------- | ------------------: |
| identity            |                  10 |
| parcel              |                  15 |
| pricing             |                  10 |
| merchant            |                   8 |
| notification-worker |                   8 |
| realtime            |                   4 |
| **Total**           |              **55** |

Use `DB_MAX_IDLE_CONNS=0` in production to allow compute scale-to-zero. Override
individual budgets with `IDENTITY_DB_MAX_OPEN_CONNS`, `PARCEL_DB_MAX_OPEN_CONNS`,
`PRICING_DB_MAX_OPEN_CONNS`, `MERCHANT_DB_MAX_OPEN_CONNS`,
`NOTIFICATION_DB_MAX_OPEN_CONNS`, and `REALTIME_DB_MAX_OPEN_CONNS`.

## Environment

```env
DATABASE_URL=postgresql://neondb_owner:<secret>@ep-curly-rice-beol9l7b-pooler.c-9.us-east-2.aws.neon.tech/neondb?sslmode=require
DATABASE_URL_DIRECT=postgresql://neondb_owner:<secret>@ep-curly-rice-beol9l7b.c-9.us-east-2.aws.neon.tech/neondb?sslmode=require
DB_MAX_OPEN_CONNS=10
DB_MAX_IDLE_CONNS=0
DB_CONN_MAX_LIFETIME=30m
DB_CONN_MAX_IDLE_TIME=5m
DB_CONNECT_TIMEOUT=5s
```

Never commit either URL. Use GitHub environment secrets named
`NEON_DATABASE_URL` and `NEON_DATABASE_URL_DIRECT`; the production workflow only
uses the direct URL.

## Migrations

```sh
make migrate          # applies pending migrations; uses DATABASE_URL_DIRECT when set
make migrate-status   # prints each version; exits non-zero when a version is dirty
make migrate-down     # one version per service; refused with APP_ENV=production
make migrate-create SERVICE=parcel NAME=add_my_index
```

- One job, one environment. The GitHub `production` environment controls
  approval and the workflow concurrency group prevents overlapping runs.
- The pinned CLI is `migrate/migrate:v4.18.3`. Version tables are
  `<service>_schema_migrations` in `public`.
- `scripts/bootstrap-db.sh` creates the five schemas and
  `public.set_updated_at()` before migrations, locally or through
  `DATABASE_URL_DIRECT`.
- A dirty version fails `make migrate-status`. Fix the cause and use
  `migrate force <version>` only on an isolated database.
- Applied migrations are immutable. Add forward migrations; never renumber.
- Development contains identity `000004`-`000006` and merchant `000007`
  unchanged. Merchant `000008` was the new compatibility migration in this
  cutover and drops the database-level business-registration constraint for
  create paths that allowed the registration number to be omitted. The
  repository now also contains `000009_restore_business_reg_no_required`;
  production remains at 8 until that migration is separately approved and
  applied. See [Migration history reconciliation](#migration-history-reconciliation).
  The ahead-of-local version guard remains in place as a safety net.

### Staging validation

1. Create a Neon branch from `production` (for example `staging`) and wait until
   it is ready.
2. Run `DATABASE_URL_DIRECT=<staging direct URL> make migrate` and
   `make migrate-status`.
3. Run the application against the staging pooled URL and exercise login,
   tracking, reports, courier reads, proof upload, proof download, and outbox
   publication.
4. Do not run production cutover from a staging branch.

## Data migration from the local database

The source audit is read-only:

```sh
make db-audit
```

The source audit recorded PostgreSQL 16.15, 10 MB, five schemas, 30 tables, 54
delivery proofs (4 with photos), and all five migration versions clean
(identity 6, pricing 2, parcel 6, merchant 7 historically; merchant 8 after `000008` applied). Row counts change
with local activity. No extensions beyond `plpgsql`; no cross-schema foreign
keys. A backup was restored into a disposable PostgreSQL 16 database and
verified at 30 tables, 54 proofs, and the same clean versions.

### Backup

```sh
BACKUP_DIR="$HOME/swiftdrop-backups" make db-backup
```

- Custom-format `pg_dump`, `--no-owner --no-privileges`, written outside the
  repository with mode `0600`.
- The script verifies the archive with `pg_restore --list` and writes a SHA-256
  checksum.
- Backups are never committed.

### Restore into Neon

```sh
APP_ENV=production \
ALLOW_PRODUCTION_RESTORE=1 \
DATABASE_URL_DIRECT='postgresql://...' \
make db-restore BACKUP="$HOME/swiftdrop-backups/swiftdrop-<timestamp>.dump"
```

- The dump includes `public` (migration tables and `set_updated_at`) and all
  five service schemas.
- The script verifies the checksum, uses `--single-transaction
--exit-on-error`, and does not pass `--clean` unless `RESTORE_CLEAN=1`.
- Do **not** run `make migrate` after a successful restore unless
  `make migrate-status` reports a lower version. Reapplying migrations on a
  restored schema is not supported.
- Production restoration is a separate approval-gated operation.

### Proof files

Local proof files are transferred separately from the database:

```sh
PROOF_STORAGE_DIR=/path/to/proofs \
STORAGE_PROVIDER=neon STORAGE_ENDPOINT=... STORAGE_BUCKET=delivery-proofs \
STORAGE_ACCESS_KEY_ID=... STORAGE_SECRET_ACCESS_KEY=... \
make proof-transfer            # dry run

# after reviewing the report
... make proof-transfer-apply
```

The tool re-validates JPEG/PNG content, uploads
`<parcel-uuid>/<proof-uuid>.jpg|.png`, verifies size with `Stat`, and updates
`photo_path` only after verification. It is idempotent for already-migrated
keys, never deletes originals, and reports failures without secrets.

```sh
... make proof-reconcile
```

lists database keys missing from storage and stored proof keys absent from the
database. It is report-only.

## Production cutover checklist (approval-gated)

1. Final `make db-backup` from the source and verify the checksum.
2. Freeze or restrict source writes.
3. Perform the final dump/restore; if writes continued, repeat the dump and
   restore into a fresh target rather than merging.
4. Run `make migrate-status`; reconcile versions and row counts.
5. Run `make proof-transfer` (dry run), then apply, then `make proof-reconcile`.
6. Update production `DATABASE_URL`, `DATABASE_URL_DIRECT`, and `STORAGE_*`
   values through the approved secret manager.
7. Deploy the application images published by the production workflow.
8. Smoke test health/readiness, login, tracking, reports, courier reads, proof
   upload, authorized proof download, and notification/outbox flow.
9. Monitor connection counts, pool wait time, error rate, and storage errors.
10. Keep the old database and proof files until the new environment is verified.

No script in this repository performs the cutover automatically.

## GitHub Actions

- `.github/workflows/ci.yml` validates migrations on a fresh PostgreSQL 16,
  runs VersityGW S3-compatible storage integration tests, and runs a PgBouncer transaction-mode
  compatibility test. Pull requests never reach Neon.
- `.github/workflows/deploy-production.yml` runs on `workflow_dispatch` or a
  `v*`/`milestone-*` tag. It requires the `production` environment approval,
  applies migrations with `NEON_DATABASE_URL_DIRECT`, verifies versions, then
  publishes images to GHCR. Application rollout is manual.
- Required environment secrets: `NEON_DATABASE_URL_DIRECT`. Runtime secrets for
  the deployed application: `NEON_DATABASE_URL`, `NEON_STORAGE_ENDPOINT`,
  `NEON_STORAGE_ACCESS_KEY_ID`, `NEON_STORAGE_SECRET_ACCESS_KEY`.

## Migration history reconciliation

| Item                   | Finding                                                                                                                                                                                            |
| ---------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Missing versions       | Identity `000004`-`000006`; merchant `000007`                                                                                                                                                      |
| Sources                | Identity: commit `2f906f9`; merchant: commit `621d733`                                                                                                                                             |
| Identity changes       | `email_verification_tokens`; `users.session_version`; `merchant_lifecycle_transitions`                                                                                                             |
| Merchant changes       | `business_address`, `lifecycle_sequence`, `merchant_registration_attempts`, `merchant_status_transitions`, business-registration index/constraint, `rejection_reason` widened to 1000              |
| Present in source DB   | Yes; source inventory contains every table, column, index, constraint and trigger                                                                                                                  |
| In bootstrap SQL       | No; bootstrap creates only the five schemas and `public.set_updated_at()`                                                                                                                          |
| Current app dependency | No current `apps/` or `packages/` code references these tables or columns; the schema is ahead of this branch's application code                                                                   |
| Decision               | Recover the exact historical files, never synthesize no-op migrations, never rewrite applied versions                                                                                              |
| Verification           | A fresh PostgreSQL 16 database migrated with the recovered files produced the same 638-line object inventory as the source and clean versions 6/2/6/7/2; staging was migrated to the same versions |

Future migrations for identity and merchant start above `000006` and `000011`
respectively once `000010` is applied. Production currently remains at merchant
8, so it must apply `000009_restore_business_reg_no_required` and then
`000010_merchant_order_snapshots` before any later merchant migration. The
migration runner still refuses to replay history when a database version is
ahead of the local files.

## Known limitations

- Local integration tests assert PostgreSQL 16; Neon production is 18. The
  application queries are version-compatible, but do not point the test fixture
  at Neon.
- Neon Object Storage is branch-scoped. Production and staging credentials are
  valid only for their branch lineage.
- Source collation is `en_US.utf8`; Neon is `C.UTF-8`. Non-ASCII name ordering
  can differ. No en_US locale is available on the Neon server.
- Database and object storage are not atomic. Recovery relies on compensation
  and the reconciliation report.
- Production application hosting is not defined in this repository; the
  workflow publishes images only.

## Verification performed

These checks were run during this integration:

| Check                                 | Result                                                                                                                                                                                |
| ------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local source audit (`make db-audit`)  | Five schemas, 30 tables, 54 proofs (4 with photos), clean versions 6/2/6/8/2 after the compatibility migration, no extensions, no cross-schema FKs                                    |
| Local backup (`make db-backup`)       | Custom-format dump verified with `pg_restore --list`; SHA-256 written                                                                                                                 |
| Restore into disposable PostgreSQL 16 | 30 tables, 54 proofs and the same clean versions restored; script completed with checksum verification                                                                                |
| Staging branch                        | `staging` (`br-spring-violet-bej9mqq4`), compute `ep-autumn-star-beu5w6ob`, storage enabled                                                                                           |
| Staging migrations                    | identity 6, pricing 2, parcel 6, merchant 8, notification 2 after applying the recovered migrations and merchant `000008`                                                             |
| Pooled compatibility against Neon     | `TestPooledTransactionCompatibility` passed through the staging `-pooler` host (transactions, prepared statements, `pg_advisory_xact_lock`)                                           |
| Neon Object Storage against staging   | JPEG/PNG object lifecycle, `HeadObject`, presigned GET, expired signed URL, missing object and delete passed                                                                          |
| Parcel service against staging        | `/ready` returned `{"status":"ready","checks":{"database":"ok"}}` through the pooled URL                                                                                              |
| Proof end-to-end against staging      | Upload returned `201`; DB stored `<parcel-uuid>/<proof-uuid>.png`; customer presigned URL returned `200`; downloaded bytes matched; test row and object removed                       |
| Proof transfer tooling                | Dry-run and reconcile completed against staging with zero rows and zero orphans                                                                                                       |
| Local S3-compatible storage           | VersityGW v1.11.0 passed upload, download, metadata, deletion and presigned-expiry tests; MinIO images/binaries are no longer retrievable                                             |
| Playwright                            | Proof-photo journeys passed; the existing suite passed except for Next.js Dev Tools overlay assertions in local dev mode, which passed when run against the production build (`CI=1`) |
| Production restore                    | 5 schemas, 30 tables, 54 proofs, row counts identical, versions 6/2/6/8/2 clean, zero incompatible constraint                                                                         |
| Production proof transfer             | 4 objects uploaded with SHA-256 verification, 4 `photo_path` rows updated, reconcile 0 missing / 0 mismatch / 0 orphans                                                               |
| Production read verification          | identity/pricing/parcel/merchant readiness, `/users/me`, admin zones, tracking, customer/courier reads, reports, proof download, foreign denial                                       |

Production was restored from the verified merchant-8 backup and the four proof
photographs were transferred. Live traffic was **not** switched, and the
application has not been deployed to a production host. See
[`production-migration-report.md`](production-migration-report.md) for the
execution evidence, outstanding CI/CD work and rollback position.
