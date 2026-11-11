# M8 merchant orders — final independent review report

## A. Executive summary

- PR: [#132](https://github.com/Vineth-R/swiftdrop/pull/132)
- Branch: `feature/m8-merchant-orders-portal`
- Target: `development`
- Final code commit reviewed: `aa79d97d9ddfbed537df716389cdca11f1c87d3f`
- Final review outcome: **Ready for merge; no known code, migration or CI blockers.**
- The PR remains **open and unmerged**.

## B. Development compatibility

- Latest `origin/development` reviewed: `50c652893fe7bd1816908857c72c188b507bd38b`.
- Original PR base: `de209dce712859adfebe5fabc9887a5f1904f1d8`.
- Development changes that affected M8:
  - Merchant migrations `000008_business_reg_no_optional` and `000009_restore_business_reg_no_required`.
  - Required business-registration validation in merchant registration.
  - Go toolchain 1.27.4, pooled/direct PostgreSQL configuration, Neon migration tooling.
  - S3-compatible/Neon object storage and proof-photo access.
  - M9 webhook delivery worker loop and M7 courier proof-upload UI.
  - Migration-validation CI job, storage/pooled test jobs and updated path filters.
- Conflicts resolved without reverting newer development work: `AGENTS.md`, `README.md`, `Makefile`, `docker-compose.yml` and `migrations/README.md`. Both development features and M8 features are retained.

## C. Migration review

- Merchant chain reviewed: `000001`, `000002`, `000006`, `000007`, `000008`, `000009`, `000010`.
- `000009` remains the development migration `000009_restore_business_reg_no_required`.
- The M8 snapshot migration was renumbered from the colliding `000009_merchant_order_snapshots` to **`000010_merchant_order_snapshots`**.
- The migration adds only nullable `VARCHAR`/`UUID` snapshot columns to `merchant.merchant_orders` and the `(merchant_id, updated_at)` index. It does not change existing constraints or data.
- Down migration drops only the columns and index introduced by the up migration.

Verification:

| Scenario                                                 | Result                                                      |
| -------------------------------------------------------- | ----------------------------------------------------------- |
| Fresh database: bootstrap + merchant `up` through 10     | PASS; all eight columns and index present, version 10 clean |
| Existing development database: `goto 9`, then `up` to 10 | PASS; snapshot columns absent at 9, present at 10           |
| Rollback `down 1` from 10                                | PASS; columns/index removed, version 9 clean                |
| Reapply `up`                                             | PASS; version 10 clean                                      |
| CI Migration validation after merge                      | PASS; merchant version 10, 30 tables                        |

## D. Backend review

### API 8.16

- API-key-only through `X-API-Key`; active key and APPROVED merchant required.
- Merchant and owner identity come from the key record; unknown body fields are rejected.
- Per-key 120/minute rate limit.
- Optional `Idempotency-Key` and independent `externalOrderId` deduplication.
- Same normalized payload returns 200; different payload returns 409 `DUPLICATE_ORDER`.
- Concurrent identical requests create one parcel and one order.
- Parcel timeout leaves no order; retry uses Parcel's stable external-order replay.
- Default pickup uses the merchant profile, with the city resolved from the Pricing zone.
- Parcel Operations owns pricing; Merchant stores the returned fee snapshot.
- Uses the real `POST /internal/parcels`; no Parcel database access.

### API 8.17 and 8.18

- Reads accept an approved MERCHANT session or active API key.
- Every query is merchant-scoped; another merchant's order returns 404.
- Filters, pagination metadata, whitelisted sorting and stable ordering verified.
- Detail uses one query with an ordered JSON timeline, fee and tracking snapshots.
- Malformed order IDs return 404 rather than a database error.

### Reliability and security

- Errors map to the documented 400/401/403/404/409/422/429/503 responses.
- Parcel client has a timeout, bounded response read and safe error classification.
- API keys/secrets are never logged or returned after creation.
- Sync logs contain event/parcel/merchant/status/correlation IDs only.

## E. Frontend review

- `/merchant/orders` and `/merchant/orders/{orderId}` work with the merchant session.
- Orders navigation, dashboard counts and recent orders use real API data.
- Loading, empty, error, retry, filters and pagination are covered.
- Timeline renders only synchronized merchant status events.
- PENDING/REJECTED/SUSPENDED merchants see the approval/suspension notice and no orders.
- The Create via API panel documents the endpoint, `X-API-Key`, optional
  `Idempotency-Key`, request/201/error examples, copyable cURL, API docs link and
  API-key shortcut.
- No browser order form, no session order-creation endpoint, and no merchant API
  key in browser storage or client code.

## F. Limitations and decisions

| Limitation                                                       | Status                    | Decision                                                                                           |
| ---------------------------------------------------------------- | ------------------------- | -------------------------------------------------------------------------------------------------- |
| No browser order creation                                        | By design                 | API 8.16 remains API-key-only; portal provides integration panel                                   |
| `externalOrderId` list filter is exact                           | Remains                   | Matches indexed merchant-scoped lookup                                                             |
| Snapshot columns nullable for pre-M8 rows                        | Remains                   | Forward-compatible; all M8-created orders populate them                                            |
| Unknown merchant-order event acknowledged                        | Remains                   | Safe no-op with a warning containing event/parcel/merchant IDs; malformed events still dead-letter |
| Missing `RABBITMQ_URL`                                           | Fixed for non-development | Development may omit it with a warning; staging/production fail startup                            |
| Concurrent conflicting idempotency key can leave an extra Parcel | Remains                   | Merchant order uniqueness stays correct; sequential conflicts are rejected before Parcel           |
| M9 webhook delivery/signing                                      | Unchanged                 | Owned by M9; M8 sync does not deliver webhooks                                                     |

## G. Defects fixed

| Severity          | Defect                                                                                                               | Fix                                                                                                            | Regression coverage                                               |
| ----------------- | -------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| Critical          | Merchant migration `000009` collided with development `000009_restore_business_reg_no_required`                      | Renumbered to `000010_merchant_order_snapshots`; updated all references and CI version expectation             | Fresh/existing-database migration checks; CI migration validation |
| High (CI blocker) | Gitleaks flagged the all-hex test JWT signing key                                                                    | Replaced it with a human-readable test-only value and added exact historical fingerprints to `.gitleaksignore` | Local Gitleaks range scan; GitHub secret scanning                 |
| Medium            | Reusing an idempotency key that already named another order could replay the external order instead of returning 409 | Check the idempotency key before external-order replay in both pre-check and insert-conflict paths             | `TestIdempotencyKeyConflictWinsOverExternalOrderReplay`           |
| Medium            | Malformed `{orderId}` reached the database and returned 500                                                          | Validate UUID in the handler and return 404                                                                    | HTTP integration malformed-ID test                                |
| Medium            | Staging/production could start without `RABBITMQ_URL` and silently skip sync                                         | Added `ValidateRuntime`; non-development startup fails without the broker                                      | Config runtime-validation test                                    |
| Low               | Sync consumer accepted short tracking numbers and FAILED events without a failure reason                             | Added schema-aligned validation and permanent-failure classification                                           | Sync handler tests                                                |
| Low               | Test fixtures used business-registration values rejected by the new development rule                                 | Updated fixtures to valid values                                                                               | Merchant repository/HTTP/access tests                             |
| Low               | Development migration compatibility test expected merchant version 9                                                 | Updated expectation to 10                                                                                      | `TestParcelServiceMigrationCompatibility`                         |

## H. Verification results

Local checks were run in the M8 worktree against `origin/development` `50c6528` after the final merge.

| Check                                             | Result    | Evidence                                                                 |
| ------------------------------------------------- | --------- | ------------------------------------------------------------------------ |
| `gofmt -l apps/services packages/go-common tests` | PASS      | no output                                                                |
| `go vet ./...`                                    | PASS      | exit 0                                                                   |
| `go test -race -count=1 ./...`                    | PASS      | exit 0                                                                   |
| `go build ./...`                                  | PASS      | exit 0                                                                   |
| `make test-db` (PostgreSQL integration)           | PASS      | exit 0                                                                   |
| Migration fresh DB                                | PASS      | merchant version 10; eight columns + index                               |
| Migration existing DB (`goto 9` → `up`)           | PASS      | version 10                                                               |
| Migration rollback/reapply                        | PASS      | version 9 clean → version 10 clean                                       |
| Merchant coverage                                 | **69.9%** | `make coverage-merchant`                                                 |
| Broker integration (`RABBITMQ_URL=...`)           | PASS      | `go test -tags=integration ./tests/integration`                          |
| Frontend lint/test/typecheck                      | PASS      | Nx run-many exit 0                                                       |
| Frontend build                                    | PASS      | `nx build @org/web`                                                      |
| Mocked Playwright Chromium                        | PASS      | 673 passed, 2 unrelated flaky retries                                    |
| Real-stack Playwright                             | PASS      | 5 passed                                                                 |
| Route checks                                      | PASS      | all gateway checks passed                                                |
| Access tests                                      | PASS      | exit 0, including `TestMerchantOrderOwnership`                           |
| Docker Compose config                             | PASS      | `docker compose config --quiet`                                          |
| `git diff --check`                                | PASS      | exit 0                                                                   |
| GitHub Actions at `aa79d97`                       | PASS      | Required CI gate, security gate, all jobs pass except release smoke skip |

## I. Final Git and PR state

- Review/fix commits added: `8f6b1e8` (development merge), `b09dff5`, `2b08a26`, `a72c063` (latest development merge), `aa79d97` (AGENTS formatting).
- Final code commit with observed green CI: `aa79d97d9ddfbed537df716389cdca11f1c87d3f`.
- Branch is synchronized with the latest `origin/development` (`50c6528`); PR mergeability is `MERGEABLE` with no conflicts.
- PR #132 is open, target `development`, and unmerged.
- A final documentation-only commit containing this report is the only later change; it does not alter code, contracts, migrations or tests.

## J. Remaining blockers

No known blockers remain.
