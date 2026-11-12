# M8 remaining work — completion report

## Baseline

- Branch created from `origin/development` at `0a05f7a`.
- PR #132 (M8 merchant orders) and PR #135 were already merged.
- M8.4 order creation, Parcel/Pricing integration and `merchant.sync.queue` synchronization were already implemented and were not rewritten.

## Completion matrix

| Task | Previous state                                                                                               | Remaining work completed                                                                                                                    | Status                                |
| :--- | :----------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------------------------------------------------------------------------ | :------------------------------------ |
| M8.1 | Registration, admin lifecycle, audit, snapshots and constraints implemented                                  | UUID validation for admin detail/approve/reject/suspend, API-key revocation and internal approval; malformed IDs now return `404 NOT_FOUND` | Complete                              |
| M8.2 | Only the working integration note existed; DOCX headings were empty and Markdown sections were missing       | Added report sections 2.3 and 6.2; corrected UC-M5; linked the new sections; reconciled API documentation; added this report                | Complete                              |
| M8.3 | Key format, hashing, constant-time comparison, five-key DB limit, revocation and approval gate implemented   | API 8.10 `createdAt`; OpenAPI key schemas; API-key-created security email; secret-leak, limit and malformed-ID regression tests             | Complete                              |
| M8.4 | Order creation, idempotency, Parcel/Pricing integration, snapshots, timeline and synchronization implemented | Revalidated only; no code rewrite. The concurrent same-key/different-payload Parcel race remains a documented limitation                    | Complete (with documented limitation) |
| M8.5 | Merchant portal and mocked/real-stack journey implemented                                                    | Real-stack journey now verifies `IN TRANSIT` on `/dashboard` and `/merchant/orders` after synchronization                                   | Complete                              |

## Changed areas

### Merchant API

- `apps/services/merchant/internal/http/merchant.go`
  - validates merchant, key and internal owner UUIDs before repository calls;
  - returns `createdAt` in the API 8.10 response;
  - sends the key-created security notice with the prefix only.
- `apps/services/merchant/internal/repository/api_clients.go`
  - `CreateAPIClient` returns the inserted `APIClient` using `RETURNING`.
- `apps/services/merchant/internal/http/router.go` and `main.go`
  - pass the existing mail notifier into the merchant HTTP options.
- `apps/services/merchant/internal/notify/notify.go`
  - adds `APIKeyCreated`, using the existing asynchronous best-effort sender.

### Contracts and documentation

- `contracts/openapi/merchant.yaml`: key list, creation and revocation schemas, including `createdAt`, `lastUsedAt`, `revokedAt` and `409 KEY_LIMIT_REACHED`.
- `docs/report/section_2_3_merchant_use_cases.md`
- `docs/report/section_6_2_merchant_order_creation.md`
- `docs/merchant-integration.md`, `docs/API_Documentation_v1.1.md`, `docs/email.md`
- No DOCX changes and no new migration.

### Tests

- `apps/services/merchant/internal/http/key_management_integration_test.go`
- `apps/services/merchant/internal/http/merchant_identifier_integration_test.go`
- `apps/services/merchant/internal/http/orders_integration_test.go` — concurrent same-key, different-payload, many-payload, different-key, cross-merchant and Parcel-failure cases
- `apps/services/merchant/internal/repository/order_reservation_integration_test.go` — reservation lifecycle and recovery after a database failure following Parcel
- `tests/integration/database/merchant_order_snapshot_migration_test.go`
- `tests/integration/database/merchant_order_reservation_migration_test.go`
- `apps/web-e2e/src/merchant-orders.real.spec.ts`

## Verification

| Check                                                    | Result                                                                                                 |
| :------------------------------------------------------- | :----------------------------------------------------------------------------------------------------- |
| `gofmt` on changed Go files                              | Pass                                                                                                   |
| `go vet ./...`                                           | Pass                                                                                                   |
| `go build ./...`                                         | Pass                                                                                                   |
| `go test -race -count=1 ./...`                           | Pass                                                                                                   |
| Merchant PostgreSQL integration                          | Pass                                                                                                   |
| `make coverage-merchant`                                 | Pass, **71.1%** (minimum 60%)                                                                          |
| Migration `000010` fresh, existing, rollback and reapply | Pass                                                                                                   |
| Event-flow integration with RabbitMQ                     | Pass                                                                                                   |
| Contract tests                                           | Pass (11 tests)                                                                                        |
| `docker compose config --quiet`                          | Pass                                                                                                   |
| Web lint, unit tests, typecheck and build                | Pass                                                                                                   |
| E2E lint and typecheck                                   | Pass                                                                                                   |
| Mocked Chromium Playwright                               | 701 passed; one tooltip test failed under full-suite load and passed in isolation (pre-existing flake) |
| Real-stack merchant journey                              | Pass (1 test)                                                                                          |
| Prettier on changed report/frontend/contract files       | Pass                                                                                                   |
| `git diff --check`                                       | Pass                                                                                                   |

The local real-stack run needed `IDENTITY_TIMEOUT=10s` because the local Docker Desktop identity service exceeded the default 3-second timeout during bcrypt work. This was an environment constraint; the test itself passed and no code default was changed.

## Concurrent idempotency fix

Same-key order attempts are now serialized per `(merchant, Idempotency-Key)` with a PostgreSQL session advisory lock. Before Parcel is called, a durable row in `merchant.merchant_order_reservations` records the normalized request hash. A concurrent or subsequent request with a different payload is rejected with `409 DUPLICATE_ORDER` without reaching Parcel. A same-payload retry proceeds and Parcel replays by `(merchantId, externalOrderId)`. The merchant order, initial timeline row and reservation completion commit in one local transaction.

A failed attempt leaves a `FAILED` reservation with the same hash, so a retry with the same payload can proceed while a different payload remains rejected. The advisory lock is released automatically if the process or database session ends.

**Remaining distributed window:** Parcel creation and the Merchant database commit are not one atomic transaction. If Merchant's database remains unavailable after Parcel succeeds, the parcel can exist without a merchant order until a same-payload retry persists it; that retry uses Parcel's replay and does not create a second parcel. Failed reservations are retained to bind the key and can be cleaned up by a future retention job.

## Remote CI

Remote CI and the PR URL are recorded in the pull request after the branch is pushed.

## Merchant webhook portal and lifecycle

The merchant portal already contained `/webhook`, the merchant navigation entry, settings, test-send and delivery-log components, and the API v1.1 8.12–8.15 endpoints. The remaining M8/M9 work was the real-stack browser proof, the test-send audit row and the worker lifecycle policy.

| Requirement                                       | Implementation                                                                               | Evidence                                                                      |
| :------------------------------------------------ | :------------------------------------------------------------------------------------------- | :---------------------------------------------------------------------------- |
| Webhook settings, one-time secret and rotation    | `apps/web/src/components/merchant/webhook-settings.tsx`; `lib/webhook-api.ts`; API 8.12/8.13 | `webhook.spec.ts`; real-stack `merchant-webhooks.real.spec.ts`                |
| Send test event                                   | `webhook-test-event.tsx`; API 8.14; test attempts recorded through `RecordDeliveryAttempt`   | Merchant handler unit tests; real-stack delivery-log assertion                |
| Delivery history, filters and pagination          | `webhook-deliveries.tsx`; API 8.15                                                           | Mocked webhook suite; real-stack tracking filter                              |
| Merchant isolation                                | Session-resolved merchant in `webhook.go`; second merchant returns 404 / empty log           | `webhook.spec.ts`; real-stack cross-merchant assertion                        |
| Approved-only configuration and test sends        | `webhookHandler.merchant` rejects non-APPROVED merchants                                     | Webhook handler tests                                                         |
| Worker re-checks approval on every attempt        | `webhook-worker` reads `merchantStatus`; suppresses non-APPROVED merchants                   | Worker tests for pending, rejected, suspended, retry and in-flight suspension |
| Revoked/inactive webhook suppression              | Internal 404 / `isActive` / event subscription checks                                        | Worker skip and suppression tests                                             |
| Auditable suppression, no replay on reinstatement | Dead-lettered attempt with a stable reason; no retry scheduled                               | Worker suppression and reinstatement tests                                    |
| Test attempt visible in delivery log              | API 8.14 records attempt with `SDNTEST0001`, dead-lettered on failure                        | Handler unit tests; real-stack webhook journey                                |

The webhook lifecycle policy is also documented in `docs/webhook-worker.md` and `docs/merchant-internal-api.md`.

### Webhook verification

- Mocked webhook Playwright suite: 19 passed, covering settings, one-time secret, rotation, validation, test send, delivery log filters, pagination, error states and role access.
- Real-stack `merchant-webhooks.real.spec.ts`: passed. It saves an HTTPS destination, rotates the secret once, sends a test event, sees `SDNTEST0001` in the delivery log, filters by tracking number, and proves a second merchant gets `404 WEBHOOK_NOT_CONFIGURED` and an empty delivery log.
- Webhook worker unit tests cover approved, pending, rejected and suspended merchants, suspension before retry, in-flight suspension, inactive/unsubscribed/deleted webhooks, auditable suppression and no replay after reinstatement.
- Event-flow integration passes with the internal webhook response carrying `merchantStatus`.
