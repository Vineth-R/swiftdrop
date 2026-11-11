# Production migration report — Neon cutover

Status: **production database restored and verified; four proof photographs
transferred and reconciled; production read paths verified; PostgreSQL 18 write
paths rehearsed on a disposable database. Production traffic has not been
switched. Development CI is validated; GHCR publication and application deployment remain unverified.**

> **Repository note (2026-11-11):** this report records the production state at
> merchant migration 8. The repository now includes
> `000009_restore_business_reg_no_required`, which restores the mandatory
> business-registration constraint. It has **not** been applied to production;
> production remains at merchant 8 until a separately approved migration window.

## A. Source database

| Item                           | Value                                                                                                          |
| ------------------------------ | -------------------------------------------------------------------------------------------------------------- |
| Database                       | `swiftdrop`                                                                                                    |
| PostgreSQL                     | 16.15                                                                                                          |
| Size                           | 11 MB                                                                                                          |
| Encoding / collation           | UTF8 / `en_US.utf8`                                                                                            |
| Schemas                        | 5                                                                                                              |
| Tables                         | 30                                                                                                             |
| Proof records                  | 54 (4 with photos, 50 metadata-only)                                                                           |
| Migration versions             | identity 6, pricing 2, parcel 6, merchant 7 historically; merchant 8 after the forward compatibility migration |
| Extensions                     | `plpgsql` only                                                                                                 |
| Cross-schema foreign keys      | none                                                                                                           |
| Connected clients during audit | 0 after stopping a stray validation parcel container that was polling the outbox                               |

Exact source row counts:

| Table                                   | Rows |
| --------------------------------------- | ---: |
| identity.audit_logs                     |    1 |
| identity.courier_profiles               |    5 |
| identity.email_verification_tokens      |    8 |
| identity.merchant_lifecycle_transitions |   22 |
| identity.oauth_states                   |    0 |
| identity.password_reset_tokens          |    0 |
| identity.refresh_tokens                 |   75 |
| identity.roles                          |    5 |
| identity.users                          |   47 |
| merchant.audit_logs                     |   26 |
| merchant.merchant_api_clients           |    0 |
| merchant.merchant_order_status_events   |    0 |
| merchant.merchant_orders                |    0 |
| merchant.merchant_registration_attempts |   26 |
| merchant.merchant_status_transitions    |   27 |
| merchant.merchant_webhooks              |    0 |
| merchant.merchants                      |   26 |
| merchant.webhook_delivery_attempts      |    0 |
| notification.notification_logs          |   20 |
| notification.processed_events           |   20 |
| parcel.delivery_proofs                  |   54 |
| parcel.outbox_events                    |   36 |
| parcel.parcel_assignments               |   15 |
| parcel.parcel_status_history            |   25 |
| parcel.parcels                          |   22 |
| pricing.audit_logs                      |    0 |
| pricing.pricing_rules                   |   25 |
| pricing.size_surcharges                 |    4 |
| pricing.weight_bands                    |    4 |
| pricing.zones                           |    5 |

The four proof files match the four non-null `photo_path` rows:

| Proof ID                               | Parcel ID                              | Source key        |     Bytes | SHA-256         |
| -------------------------------------- | -------------------------------------- | ----------------- | --------: | --------------- |
| `12e5e098-362a-474e-b86a-c4f081bea0db` | `091d1c45-f200-4e53-8aef-afcfd3cb421d` | `50b96e4f…13.png` |        85 | `b58cd2df…78a1` |
| `89e40a90-9c2a-49d4-9369-039952812c3e` | `091d1c45-f200-4e53-8aef-afcfd3cb421d` | `27c90391…4a.jpg` |       759 | `88fd131b…72e8` |
| `1314fd5e-683d-411f-99a8-6cd8c7803c50` | `091d1c45-f200-4e53-8aef-afcfd3cb421d` | `3da73ea9…60.jpg` | 5,060,994 | `246e45c6…1467` |
| `c2b3c709-81f1-45b3-9b45-5fc200c3ce91` | `091d1c45-f200-4e53-8aef-afcfd3cb421d` | `f7093036…e8.jpg` | 5,242,880 | `3f1485ca…9b20` |

## B. Backup

| Item                             | Value                                                                                                                   |
| -------------------------------- | ----------------------------------------------------------------------------------------------------------------------- |
| Path                             | `/var/folders/s6/rkrnyxh51_ggggbvy5blmc2c0000gn/T/opencode/swiftdrop-cutover-backups/swiftdrop-20261110T203506Z.dump`   |
| Format                           | PostgreSQL custom                                                                                                       |
| SHA-256                          | `324d4a0d…fec0`                                                                                                         |
| `pg_restore --list`              | passed                                                                                                                  |
| Restored into                    | disposable PostgreSQL 16.15 database `final_backup_verify`                                                              |
| Row-count comparison             | 0 differences across all 30 tables                                                                                      |
| Schema comparison                | semantically identical; only PostgreSQL text formatting of one partial-index array predicate differs, not the predicate |
| Migration versions after restore | restored at identity 6, pricing 2, parcel 6, merchant 7, notification 2; merchant `000008` applied to reach 8 clean     |

Backups are outside the repository and are never committed.

## C. Migration history reconciliation

The source feature tree reported identity 6 and merchant 7 while that tree ended
at identity 3 and merchant 6. Development already contained the missing
migrations, unchanged. Investigation found them in Git commits `2f906f9` and
`621d733`:

| Version         | File                                    | Changes                                                                                                                                                                 |
| --------------- | --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| identity 000004 | `000004_email_verification`             | `identity.email_verification_tokens`, user/expiry indexes, one-unused unique index                                                                                      |
| identity 000005 | `000005_session_version`                | `identity.users.session_version INTEGER NOT NULL DEFAULT 0 CHECK >= 0`                                                                                                  |
| identity 000006 | `000006_merchant_lifecycle_transitions` | `identity.merchant_lifecycle_transitions`, user/sequence constraints                                                                                                    |
| merchant 000007 | `000007_registration_onboarding`        | `business_address`, `lifecycle_sequence`, registration attempt and status-transition tables, business-registration index/constraint, `rejection_reason` widened to 1000 |

Evidence:

- The changes are present in the source database.
- They are **not** in the bootstrap SQL (bootstrap creates only the five
  schemas and `public.set_updated_at()`).
- The current application code does not reference these tables or columns; the
  schema is ahead of this branch's app code.
- The exact up/down files were recovered verbatim from Git. No no-op or invented
  migration was added.
- A fresh PostgreSQL 16 database migrated with the recovered files produced an
  object inventory **identical to the source** (638/638 lines, zero diff).
- Rolling identity 6 and merchant 7 down and back up on a disposable database
  restored the identical inventory.
- Staging was migrated to identity 6 / merchant 8 clean.

Decision for this development branch: keep the existing identity 4-6 and merchant
7 files unchanged, add only `000008_business_reg_no_optional`, and keep the
ahead-of-local version guard as a safety net. The older feature tree used for the
production migration recovered the historical files for verification; that
recovery is not part of the development migration diff. Version rows were not
deleted or forced. The current Merchant integration suite passes on a fresh
database at merchant 8.

## D. Neon production preflight

| Item                | Value                                                       |
| ------------------- | ----------------------------------------------------------- |
| Project             | `proud-rice-63339713` (`swiftdrop`)                         |
| Region              | AWS US East 2 (`aws-us-east-2`)                             |
| Production branch   | `production` (`br-round-paper-beim4u2g`)                    |
| PostgreSQL          | 18.6                                                        |
| Database / role     | `neondb` / `neondb_owner`                                   |
| Schemas             | `public` only                                               |
| User tables         | none                                                        |
| Database size       | 7,536 kB                                                    |
| Extensions          | `plpgsql`                                                   |
| Object storage      | enabled, `us-east-2`, path-style                            |
| Bucket              | `delivery-proofs`, private, zero objects                    |
| Direct endpoint     | `ep-curly-rice-beol9l7b.c-9.us-east-2.aws.neon.tech`        |
| Pooled endpoint     | `ep-curly-rice-beol9l7b-pooler.c-9.us-east-2.aws.neon.tech` |
| Direct connectivity | verified read-only                                          |
| Pooled connectivity | verified read-only                                          |
| Unexpected data     | none                                                        |

**Collation difference:** source is `en_US.utf8`; Neon `neondb` is `C.UTF-8`,
and `en_US.utf8` is not available on the Neon server. This can change ordering of
non-ASCII text and dictionary-style name sorting. Uniqueness of `lower(email)`
and `lower(business_reg_no)` is unaffected for the ASCII addresses in the
source. Collation-sensitive queries will be compared after restore; no database
recreation or locale change will be attempted.

**PostgreSQL 16 → 18:** no extensions, no cross-schema foreign keys, no
serial/identity sequence ownership beyond UUID defaults, and no 16-only SQL.
The dump is custom format from 16; restore uses the existing script.

### PostgreSQL 18 rehearsal

A disposable PostgreSQL 18.6 container was created with a `C.UTF-8` database
(matching Neon's collation) and the verified backup was restored:

| Check                             | Result                                                                                                              |
| --------------------------------- | ------------------------------------------------------------------------------------------------------------------- |
| Restore                           | succeeded                                                                                                           |
| Row counts                        | 0 differences across all 30 tables                                                                                  |
| Semantic schema                   | identical; only the same parenthesized partial-index predicate text differs                                         |
| Migration status                  | restored at identity 6, pricing 2, parcel 6, merchant 7, notification 2; merchant `000008` applied to reach 8 clean |
| `make db-verify`                  | all checks passed                                                                                                   |
| Parcel/identity/pricing readiness | all `200` with database `ok`                                                                                        |
| Public tracking                   | `200`                                                                                                               |
| Customer parcel detail            | `200`; courier lookup through identity resolved                                                                     |
| Parcel history                    | `200`                                                                                                               |
| Proof stream                      | `200`; SHA-256 matched the source JPEG                                                                              |
| Courier detail                    | `200`                                                                                                               |
| Reports daily                     | `200`; delivered count reflected a rehearsal delivery                                                               |
| Foreign customer proof access     | `404`                                                                                                               |
| Registration + login              | `201` / `200`                                                                                                       |
| Proof upload                      | `201`; DB stored `<parcel>/<proof>.png`; latest download SHA-256 matched                                            |
| Status update                     | `200`; parcel became `DELIVERED`; history and PENDING outbox row written                                            |

All rehearsal writes were confined to the disposable PostgreSQL 18 database.
The source database and Neon production were not modified.

## E. Production restore — completed

Authorized explicitly before execution. Target preflight confirmed:

- project `proud-rice-63339713`, branch `br-round-paper-beim4u2g`, region
  `aws-us-east-2`, PostgreSQL 18.6;
- only `public`, zero user tables, empty `delivery-proofs` bucket;
- direct and pooled endpoints reachable read-only.

Restore command (secret omitted):

```sh
APP_ENV=production ALLOW_PRODUCTION_RESTORE=1 \
DATABASE_URL_DIRECT='postgresql://neondb_owner:<secret>@ep-curly-rice-beol9l7b.c-9.us-east-2.aws.neon.tech/neondb?sslmode=require' \
make db-restore BACKUP='/var/folders/.../swiftdrop-20261111T040514Z.dump'
```

Result and verification:

| Check                                | Result                                                                                                                                                                             |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Restore                              | succeeded with `--single-transaction --exit-on-error`                                                                                                                              |
| Schemas                              | 5                                                                                                                                                                                  |
| Tables                               | 30                                                                                                                                                                                 |
| Proof records                        | 54 (4 with photos, 50 metadata-only)                                                                                                                                               |
| Migration versions                   | identity 6, pricing 2, parcel 6, merchant 8, notification 2, all clean                                                                                                             |
| Row-count comparison vs source       | 0 differences across all 30 tables                                                                                                                                                 |
| Semantic schema comparison vs source | identical except PostgreSQL's text formatting of one partial-index predicate (`ANY (('PENDING'), ('COMPLETED'))` vs `ANY (('PENDING', 'COMPLETED'))`); the predicate is equivalent |
| Incompatible merchant constraint     | absent                                                                                                                                                                             |
| `make db-verify`                     | all checks passed                                                                                                                                                                  |
| `make migrate-status`                | clean                                                                                                                                                                              |

No `make migrate` was run against production after restore because the backup
already contains the reconciled merchant 8 schema.

## F. Object storage — completed

| Check                      | Result                                                                        |
| -------------------------- | ----------------------------------------------------------------------------- |
| Dry run                    | total 4, planned 4, missing 0, failed 0                                       |
| Apply                      | 4 transferred, 0 failed; size, content type and SHA-256 verified after upload |
| Database metadata          | 4 non-null `photo_path` rows point at `<parcel-uuid>/<proof-uuid>.jpg         | png` |
| Bucket                     | exactly 4 objects; no unexpected objects                                      |
| Reconciliation             | total 4, missing 0, size mismatch 0, MIME mismatch 0, orphans 0               |
| Idempotent rerun           | 0 planned, 4 skipped, 0 failed                                                |
| Presigned download         | all 4 returned `200`; downloaded SHA-256 matched the source files             |
| Expired presigned URL      | `403`                                                                         |
| Customer proof URL         | `200`                                                                         |
| Assigned courier proof URL | `200`                                                                         |
| Foreign customer proof URL | `404`                                                                         |

Transferred objects:

| Object key                  |     Bytes | SHA-256         |
| --------------------------- | --------: | --------------- |
| `091d1c45-…/12e5e098-….png` |        85 | `b58cd2df…78a1` |
| `091d1c45-…/89e40a90-….jpg` |       759 | `88fd131b…72e8` |
| `091d1c45-…/1314fd5e-….jpg` | 5,060,994 | `246e45c6…1467` |
| `091d1c45-…/c2b3c709-….jpg` | 5,242,880 | `3f1485ca…9b20` |

Source files and the source Docker volume were not deleted or modified.

## G. Application verification

### Production, read-only

Services were started against the production pooled connection and Neon storage:

| Check                                  | Result                                                                    |
| -------------------------------------- | ------------------------------------------------------------------------- |
| identity `/ready`                      | `200` database `ok`                                                       |
| pricing `/ready`                       | `200` database `ok`                                                       |
| parcel `/ready`                        | `200` database `ok`                                                       |
| merchant `/ready`                      | `200` database `ok`                                                       |
| identity `/users/me` with verified JWT | `200`                                                                     |
| pricing admin zones with ADMIN JWT     | `200`                                                                     |
| public tracking                        | `200`                                                                     |
| customer parcel detail                 | `200`; courier resolved through identity                                  |
| customer history                       | `200`                                                                     |
| courier delivery detail                | `200`                                                                     |
| reports daily                          | `200`                                                                     |
| merchant webhook settings              | `404 WEBHOOK_NOT_CONFIGURED`; session auth and merchant resolution worked |
| customer proof URL/download            | `200`; SHA-256 matched                                                    |
| courier proof URL/download             | `200`; SHA-256 matched                                                    |
| foreign customer proof                 | `404`                                                                     |

No production parcel creation, proof upload, status transition, password login
or background-worker message was executed, to avoid unauthorized production
writes. Those paths were exercised on the disposable PostgreSQL 18 rehearsal:

| Rehearsal check        | Result                                                                                |
| ---------------------- | ------------------------------------------------------------------------------------- |
| registration / login   | `201` / `200`                                                                         |
| proof upload           | `201`; nested key stored; latest download SHA-256 matched                             |
| courier status update  | `200`; parcel `DELIVERED`; history and PENDING outbox row written                     |
| reports after delivery | delivered count reflected the rehearsal                                               |
| background messaging   | covered by the existing RabbitMQ/outbox integration suites, not by production workers |

### Collation

The source is `en_US.utf8`; Neon is `C.UTF-8`. `en_US.utf8` is not available on
the Neon server. The source data is predominantly ASCII, and email/business
registration uniqueness is unaffected. Dictionary-style ordering of non-ASCII
names can differ. No locale change or database recreation was attempted.

## H. CI/CD

Development CI was validated on PR #129:

- 33 checks passed; release smoke is skipped by design on pull requests.

Production deployment remains isolated and unverified:

- `.github/workflows/deploy-production.yml` is a separate manual/tag workflow;
- no `production` GitHub environment or approval rules are configured;
- the five production secrets are not configured;
- no production workflow run has occurred;
- no GHCR images have been published from this change;
- the Go toolchain was updated to `1.27.4` after `GO-2026-6859` appeared in the
  vulnerability database; `govulncheck` is clean on the new toolchain.

Application rollout is still manual.

## I. Rollback readiness

- Source database remains available, with the same data and merchant 8 schema;
  it was not reset or deleted.
- Source proof volume `swiftdrop_parcel-proofs` remains intact; source files
  were copied, not moved.
- Final verified backup is outside the repository:
  `/var/folders/.../swiftdrop-production-final/swiftdrop-20261111T040514Z.dump`,
  SHA-256 `eae5c3b0…9618`.
- Neon production has point-in-time restore available, but no reset was used.
- Rollback after live writes requires reconciling writes accepted by Neon;
  changing the connection string alone is not a safe rollback.

## Outstanding

- Merge PR #129 after the required review/approval.
- Create the GitHub `production` environment, required reviewers and the five
  secrets before running the production workflow.
- Publish GHCR images from the approved commit.
- Deploy the application to its hosting target (not defined in this repo).
- Validate background workers against the production deployment.
- Perform the live traffic cutover only after explicit approval.
- Optionally validate collation-sensitive ordering with representative
  non-ASCII data.
- **Credential hygiene:** a real credential block was found at the end of the
  working-tree `.env.example` during review and removed; it was not committed. Rotate the exposed
  Neon role password, the default object-storage credential and the
  `swiftdrop-proof-storage` credential before deployment; update `.env` and the
  GitHub production secrets with the replacement values. No secret is present
  in tracked files after cleanup.
