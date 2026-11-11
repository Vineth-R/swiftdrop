# M8 merchant orders and portal — completion report

Branch: `feature/m8-merchant-orders-portal`  
Base: `origin/development` (`de209dc`)  
Worktree: isolated from the dirty M7 worktree; registration/approval work was already merged.

## 1. Initial audit

The registration and approval implementation was already merged. Merchant profiles, API keys, webhook settings, and the registration/approval screens were present. The merchant order UI components (`orders-view.tsx`, `order-detail-view.tsx`) existed but had no routes and no backend.

| Task                           | Existing status                                                                                                                                 | Action taken                                                                             |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------- |
| M8.1 database and registration | Registration/approval merged; merchant tables, API-key trigger and order tables existed; order snapshot lacked pickup/recipient contact columns | Reused registration; added migration 000009                                              |
| M8.2 report sections           | `docs/merchant-integration.md` described the target but said the order route was pending                                                        | Updated to the implemented workflow                                                      |
| M8.3 API keys                  | Format, SHA-256, constant-time compare, revoke, approval gate and 5-key lock/trigger existed                                                    | Preserved; added validation, trigger-error mapping and concurrency/one-time-secret tests |
| M8.4 order APIs and sync       | No order handlers, repository, Parcel client, OpenAPI paths or consumer                                                                         | Implemented API 8.16-8.18 and `merchant.sync.queue`                                      |
| M8.5 portal and tests          | Dashboard/profile/keys existed; order views had no routes/nav; no order tests                                                                   | Added routes, nav, dashboard overview, Create via API panel, mocked and real-stack tests |

## 2. Completion matrix

| Item                           | Status   | Evidence                                                       |
| ------------------------------ | -------- | -------------------------------------------------------------- |
| M8.1 database and registration | Complete | Registration reused; `000009_merchant_order_snapshots`         |
| M8.2 report sections 2.3/6.2   | Complete | `docs/merchant-integration.md`, figures                        |
| M8.3 API-key security          | Complete | Existing implementation preserved; new limit/concurrency tests |
| M8.4 order APIs                | Complete | `POST/GET /merchant/orders`, `GET /merchant/orders/{orderId}`  |
| M8.4 sync consumer             | Complete | `merchant.sync.queue` consumer and status transaction          |
| M8.5 portal                    | Complete | `/merchant/orders`, detail, dashboard, Create via API panel    |
| M8.5 tests                     | Complete | Unit, HTTP/DB integration, mocked Playwright, real-stack       |
| M8.5 coverage                  | Complete | 68.9% merchant service with PostgreSQL tests                   |

## 3. API endpoints

| Method | Path                                | Auth                        | Behavior                                                                                          |
| ------ | ----------------------------------- | --------------------------- | ------------------------------------------------------------------------------------------------- |
| `POST` | `/api/v1/merchant/orders`           | `X-API-Key` only            | approved merchant, optional `Idempotency-Key`, real Parcel/Pricing creation, 201 new / 200 replay |
| `GET`  | `/api/v1/merchant/orders`           | MERCHANT session or API key | merchant-scoped list, filters, pagination, safe sort                                              |
| `GET`  | `/api/v1/merchant/orders/{orderId}` | MERCHANT session or API key | snapshot, fee, status and ordered timeline; cross-merchant 404                                    |

Errors: `400 BAD_REQUEST` for unknown/invalid body fields, `401 INVALID_API_KEY`,
`403 MERCHANT_NOT_APPROVED`, `404 NOT_FOUND`, `409 DUPLICATE_ORDER`,
`422 VALIDATION_ERROR` / `ROUTE_NOT_SERVICEABLE` / `WEIGHT_NOT_PRICED`,
`429 RATE_LIMITED`, `503 SERVICE_UNAVAILABLE`.

## 4. API-key security and lifecycle

- Keys remain `sdk_live_<lookup_id>_<secret>`.
- Only a SHA-256 hash of the full key is stored; comparison is constant time.
- The full key is returned exactly once at creation and never in list/detail.
- Revoked keys return 401; unapproved, rejected and suspended merchants return 403.
- The active-key limit is five, enforced by a row lock plus a database trigger.
- Concurrent creation was tested with 10 goroutines: exactly five succeed and five
  receive `ErrKeyLimit`.
- Order creation resolves the merchant and owner from the key record and rejects
  unknown fields, so a request body cannot supply a merchant identity.

## 5. Order creation workflow

1. Authenticate `X-API-Key`; resolve the active merchant and require APPROVED.
2. Decode the documented body with unknown fields rejected (including any
   `merchantId`/`createdByUserId`).
3. Validate recipient and pickup fields with the same phone, length, UUID, weight
   and size rules Parcel applies.
4. If pickup is omitted, use the merchant's business name, contact phone, pickup
   address and zone; the pickup city is the Pricing zone name.
5. Compute a stable normalized SHA-256 request hash.
6. Replay an existing order by `externalOrderId` or `Idempotency-Key` when the
   hash matches; otherwise return `409 DUPLICATE_ORDER`.
7. Call the real internal Parcel endpoint `POST /internal/parcels` with the
   verified merchant and owner, `X-Internal-Token` and a forwarded request ID.
8. Persist the order snapshot and initial `CREATED` timeline row in one
   transaction.
9. Return the documented 201 body (or 200 for a replay).

## 6. Idempotency and duplicate handling

- Same reference/key + same normalized payload → 200, original order, no new parcel.
- Same reference/key + different payload → 409 `DUPLICATE_ORDER`.
- No `Idempotency-Key` → still deduplicates by `externalOrderId`.
- Two concurrent identical requests → one 201, one 200, one Parcel call, one order.
- A Parcel timeout leaves no merchant order; the retry uses Parcel's
  `(merchantId, externalOrderId)` replay and then persists the order.
- A unique-conflict loser re-reads the winner and returns it only when the hash matches.

Known limitation: concurrent requests that use the same idempotency key with
different external order references can create the second Parcel before the
merchant unique-key conflict is detected. Sequential conflicting requests are
rejected before Parcel is called; the merchant order uniqueness remains correct.

## 7. Parcel Service integration

- Real HTTP client in `internal/parcelclient`, not a mock or direct database write.
- Sends the trusted internal contract: merchant ID, owner user ID, external
  order ID, idempotency key, pickup, recipient, weight, size and description.
- Preserves Pricing's fee breakdown and total; no duplicate pricing logic.
- Maps Parcel 409 to `DUPLICATE_ORDER`, 422 to the documented validation/pricing
  codes, and 401/5xx/timeouts to `SERVICE_UNAVAILABLE`.
- Does not expose Parcel internals or recipient data in errors/logs.

## 8. List and detail

- Every query is scoped by the authenticated merchant ID.
- Another merchant's order returns 404 with no order data.
- Filters: status, from/to date, exact external order ID.
- Pagination: page/pageSize (maximum 100), total items and total pages.
- Sort: `createdAt`, `updatedAt` or `externalOrderId`, both directions, stable `id` tie-breaker.
- Detail is one SQL query with an ordered JSON timeline, avoiding a status/timeline read race.

## 9. Status synchronization consumer

- Queue `merchant.sync.queue`, routing key `parcel.status_changed.v1`, durable
  exchange and shared retry/DLQ topology.
- Validates content type/body size, envelope, type, version, producer, UUIDs,
  tracking number and status enum.
- Ignores events without `merchant_id` (customer parcels).
- Locks the merchant order row and applies only `occurred_at > last_status_at`.
- `merchant_order_status_events.event_id UNIQUE` makes redelivery a no-op.
- Timeline insert and status update share one transaction; failures roll back.
- Tracking mismatch and malformed events are permanent; database failures retry.
- A merchant parcel without an order snapshot is logged and acknowledged; it is
  not applicable merchant-order work.
- The consumer logs event, parcel, merchant, status and correlation IDs only, never
  recipients or secrets.

## 10. Event and RabbitMQ configuration

- Exchange: `swiftdrop.events`.
- Queue: `merchant.sync.queue`; retry `merchant.sync.queue.retry.v2`; DLQ
  `merchant.sync.queue.dlq`.
- No producer contract change was needed: `parcel.status_changed.v1` already
  carries `parcel_id`, `tracking_number`, `previous_status`, `status`,
  `merchant_id`, and the envelope carries `event_id`/`occurred_at`.
- Merchant starts the consumer when `RABBITMQ_URL` is set. Without it, the HTTP
  service starts and logs a warning; synchronization is disabled.

## 11. Merchant portal

- `/merchant/orders`: merchant session list with filters, pagination and detail links.
- `/merchant/orders/{orderId}`: real snapshot, fee, current status and synchronized timeline.
- Dashboard: real total/in-progress/delivered/failed counts and recent orders.
- Nav: Dashboard, Orders, API keys, Business, Profile.
- Pending/rejected/suspended merchants see the approval/suspension notice and no orders.
- **Create via API** panel: endpoint, `X-API-Key`, optional `Idempotency-Key`,
  request body, 201 example, error codes, copyable cURL, API documentation link,
  and a shortcut to API key management.
- No browser order form and no API key in browser storage or client requests.
- API 8.16 remains API-key-only; no new session order endpoint was added.

## 12. Database migration and indexes

`000009_merchant_order_snapshots` adds:

- `pickup_name`, `pickup_phone`, `pickup_address`, `pickup_city`, `pickup_zone_id`
- `recipient_phone`, `recipient_address`, `recipient_zone_id`
- `(merchant_id, updated_at)` index

Existing constraints remain: unique `(merchant_id, external_order_id)`, partial
unique `(merchant_id, idempotency_key)`, unique `parcel_id`, unique
`tracking_number`, and the status timeline `event_id` unique index. Version
`000008` is skipped because another worktree carries an uncommitted candidate
with that version; the migration runner accepts the gap.

## 13. M5 / M7 / M9 integration

- M5: Merchant uses the existing internal Parcel creation and pricing path.
- M7: Merchant consumes the existing `parcel.status_changed.v1` producer without
  changing it.
- M9: webhook settings, secret handling, test sends and internal worker endpoints
  are untouched. The merchant sync consumer only updates merchant-owned order
  data and does not deliver webhooks, sign payloads or expose webhook secrets.
- M9's own `parcel.status_changed.v1` consumer continues independently.

## 14. Ownership and authorization

- API key OR MERCHANT session is required for reads; only an API key may create.
- The merchant is always resolved from the credential, never from the request body.
- Every list/detail/sync query is merchant-scoped.
- HTTP integration tests and the real-stack journey prove another merchant gets
  404 with no tracking/order data.
- The access suite now has `TestMerchantOrderOwnership` using the live gateway.

## 15. Tests

Backend:

- `internal/orders`: validation, hash stability, default pickup, replay/conflict,
  concurrent winner, Parcel failures, response DTOs.
- `internal/parcelclient`: contract, headers, success, validation, conflict,
  outage, timeout, invalid response.
- `internal/merchantsync`: valid, duplicate/older, customer parcel, unknown order,
  tracking mismatch, retryable failure, malformed events.
- `internal/repository`: order create/detail/list/sync, duplicate constraints,
  concurrent events, rollback, merchant lifecycle, API clients, registration,
  webhooks and delivery attempts.
- `internal/http`: API-key and JWT reads, approval refusals, validation, default
  pickup, replay/conflict, concurrent duplicates, timeout retry, rate limit,
  cross-merchant 404, key-limit concurrency.
- `identityclient`, `pricingclient`, `notify`, `config` and go-common auth helpers.

Frontend:

- Mocked Playwright: order list/pagination/filters, detail timeline, Create via
  API panel, pending/suspended denial, API key create/one-time secret/revoke,
  409 conflict, dashboard overview.
- Real-stack Playwright: register → approve → login → create key in portal →
  API-key order → SDN tracking/fee → list/detail → publish
  `parcel.status_changed.v1` → synchronized `IN_TRANSIT` timeline → second
  merchant 404.

## 16. Actual coverage

`TEST_DATABASE_URL=... make coverage-merchant` (race detector, PostgreSQL integration tests):

```
merchant                                               16.4%
merchant/internal/apikey                               80.8%
merchant/internal/config                               78.5%
merchant/internal/http                                 68.2%
merchant/internal/identityclient                       79.7%
merchant/internal/merchantsync                         86.7%
merchant/internal/notify                               73.5%
merchant/internal/onboarding                           76.2%
merchant/internal/orders                               75.4%
merchant/internal/parcelclient                         89.1%
merchant/internal/pricingclient                        90.9%
merchant/internal/repository                           69.7%
total                                                  68.9%
```

The gate is 60%.

## 17. Verification results

| Command                                                                        | Result                              |
| ------------------------------------------------------------------------------ | ----------------------------------- |
| `gofmt -l apps/services packages/go-common tests`                              | clean                               |
| `go vet ./...`                                                                 | pass                                |
| `go test -count=1 ./...`                                                       | pass                                |
| `go build ./...`                                                               | pass                                |
| `TEST_DATABASE_URL=... make coverage-merchant`                                 | 68.9% pass                          |
| `TEST_DATABASE_URL=... make test-db`                                           | pass                                |
| `RABBITMQ_URL=... go test -tags=integration ./tests/integration`               | pass                                |
| `make check-routes` against a fresh Compose stack                              | all routes pass                     |
| `make test-access` against a fresh Compose stack                               | pass                                |
| Real-stack Playwright (`--project=real-stack`, 5 tests)                        | 5 passed                            |
| Mocked Chromium Playwright                                                     | 665 passed, 1 unrelated flaky retry |
| `pnpm exec nx lint/test/typecheck/build @org/web`                              | pass                                |
| `pnpm exec nx lint/typecheck @org/web-e2e`                                     | pass                                |
| `docker compose config --quiet`                                                | pass                                |
| `TEST_DATABASE_URL=... go test -tags=integration ./apps/services/merchant/...` | pass                                |

The full existing real-stack onboarding suspension test needed a larger local
timeout because it stops and restarts Identity; with `--timeout=120000` it passed.
CI already runs that suite.

## 18. Remaining limitations

- No browser order form by design; API 8.16 is API-key-only.
- `externalOrderId` list filtering is an exact match, not a fuzzy search.
- Contact snapshot columns are nullable for pre-M8 rows; all M8-created orders fill them.
- The consumer is disabled when `RABBITMQ_URL` is unset (logged).
- A merchant-owned parcel without an order snapshot is acknowledged, not dead-lettered.
- A conflicting concurrent idempotency key can leave an extra Parcel before the
  merchant conflict is detected (merchant order uniqueness remains correct).
- Webhook delivery/signing remains M9-owned and unchanged.
- No per-event metrics; structured logs are the observability surface.

## 19. Branch and commits

Branch: `feature/m8-merchant-orders-portal`  
Implementation commit: `7dc9b16` (`feat(merchant): complete M8 orders and merchant portal`)  
PR: https://github.com/Vineth-R/swiftdrop/pull/132 (into `development`, not merged)
