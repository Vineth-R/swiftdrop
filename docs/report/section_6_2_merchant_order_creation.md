## 6.2 Merchant API Order Creation

**Service:** Merchant Integration. **Endpoint:** `POST /api/v1/merchant/orders` (API 8.16). **Authentication:** merchant API key in `X-API-Key`; a merchant session is not accepted for creation.

The external merchant system creates a delivery order from its own store. Merchant Integration authenticates the key, validates the request, calls Parcel Operations for real pricing and parcel creation, then persists an immutable merchant order snapshot and the first timeline entry. Parcel status events later update the snapshot and timeline through `merchant.sync.queue`.

### 6.2.1 Step by Step

1. **Authenticate the key.** Merchant Integration parses `sdk_live_<lookup_id>_<secret>`, finds the key by its unique lookup ID, hashes the supplied key and compares the hash in constant time. A missing, malformed, unknown or revoked key returns `401 INVALID_API_KEY`.
2. **Resolve the merchant.** The merchant and owner identity come from the key record. The request body cannot carry `merchantId` or `createdByUserId`; unknown JSON fields are rejected.
3. **Require approval.** The merchant must be `APPROVED`. Pending, rejected and suspended merchants return `403 MERCHANT_NOT_APPROVED`.
4. **Validate the payload.** `externalOrderId`, recipient contact, weight and size are validated with the same field rules Parcel applies. An optional pickup may be supplied. The optional `Idempotency-Key` header is limited to 100 characters.
5. **Resolve default pickup.** When `pickup` is omitted, the merchant's saved business name, contact phone and pickup address are used. The pickup city is resolved from the merchant's default Pricing zone. A missing or unavailable zone returns `503 SERVICE_UNAVAILABLE`.
6. **Normalize and hash.** Merchant Integration builds a stable normalized request hash over the reference, pickup, recipient, two-decimal weight, size and description. The idempotency key is an identifier and is not part of the payload hash.
7. **Check duplicates.** The service checks the idempotency key first, then `externalOrderId`. An existing order with the same hash replays as `200` with the original order. A different hash returns `409 DUPLICATE_ORDER` before any Parcel call in the sequential case.
8. **Call Parcel Operations.** The real internal `POST /internal/parcels` contract is called with the verified merchant ID, verified owner user ID, external order ID, idempotency key and order details. The shared `X-Internal-Token` and request ID are forwarded. Parcel Operations applies Pricing and creates the parcel; Merchant Integration does not read the Parcel or Pricing schemas and does not duplicate pricing logic.
9. **Persist the snapshot.** In one local PostgreSQL transaction, Merchant Integration inserts the merchant order with the pickup/recipient contact snapshot, fee snapshot and tracking number, then inserts the initial `CREATED` timeline event.
10. **Respond.** A new order returns `201` with `orderId`, `externalOrderId`, `trackingNumber`, `status`, the fee breakdown and `createdAt`.

### 6.2.2 Idempotency and Duplicate Handling

- The same `externalOrderId` or `Idempotency-Key` with the same normalized payload returns the original order with `200`; no new parcel is created.
- A different normalized payload returns `409 DUPLICATE_ORDER`.
- A request without `Idempotency-Key` still deduplicates by `externalOrderId`.
- Parcel Operations serializes creation for `(merchantId, externalOrderId)` and replays the existing parcel, so a client timeout followed by a retry does not create a second parcel.
- If the local insert loses a uniqueness race, Merchant Integration re-reads the winner and replays it only when the request hash matches.
- **Known limitation:** two concurrent requests that reuse the same `Idempotency-Key` with _different_ payloads can both reach Parcel before either merchant order insert wins. The second merchant order is correctly rejected with `409 DUPLICATE_ORDER` and merchant order uniqueness is preserved, but the losing request can leave an unreferenced Parcel behind. Sequential conflicts are rejected before Parcel. This race is retained as an accepted limitation; fixing it would require a reservation or locking redesign before the external call.

### 6.2.3 Status Synchronization

`merchant.sync.queue` is bound to `parcel.status_changed.v1` on the shared `swiftdrop.events` topic exchange. The consumer:

1. Validates the message framing, event envelope, event type/version, producer, event ID and status payload.
2. Ignores customer parcels without `merchant_id`; they are not merchant-order work.
3. Locks the merchant order row for `(parcel_id, merchant_id)` and verifies the tracking number.
4. Ignores an event whose `occurred_at` is not newer than the stored `last_status_at`, so older events cannot regress the order.
5. Inserts the timeline event with `event_id` unique, so a redelivery is a no-op.
6. Updates `last_known_status` and `last_status_at` in the same transaction as the timeline insert.

Malformed events and tracking-number mismatches are permanent failures and are dead-lettered. Database failures remain retryable. A merchant-owned parcel without a merchant order snapshot is logged and acknowledged rather than poisoning the queue. The merchant dashboard and order detail read the same snapshot, so a synchronized status appears in both.

### 6.2.4 Outcomes and Error Handling

| Outcome                                          | HTTP | Code                    |
| :----------------------------------------------- | :--- | :---------------------- |
| New order                                        | 201  | —                       |
| Same payload replayed                            | 200  | —                       |
| Missing, malformed, unknown or revoked key       | 401  | `INVALID_API_KEY`       |
| Pending, rejected or suspended merchant          | 403  | `MERCHANT_NOT_APPROVED` |
| Unknown JSON field or malformed body             | 400  | `BAD_REQUEST`           |
| Duplicate reference/key with a different payload | 409  | `DUPLICATE_ORDER`       |
| Invalid field values                             | 422  | `VALIDATION_ERROR`      |
| Unpriced route                                   | 422  | `ROUTE_NOT_SERVICEABLE` |
| Weight outside every active band                 | 422  | `WEIGHT_NOT_PRICED`     |
| Too many requests for one key                    | 429  | `RATE_LIMITED`          |
| Parcel, Pricing or pickup-zone outage            | 503  | `SERVICE_UNAVAILABLE`   |

Merchant list and detail reads accept an approved `MERCHANT` session or an API key. Every query is scoped by the authenticated merchant, so another merchant's order returns `404 NOT_FOUND` with no tracking or recipient data.

### 6.2.5 Implementation Status

| Capability                                     | Status      | Evidence                                                    |
| :--------------------------------------------- | :---------- | :---------------------------------------------------------- |
| API-key-only order creation (8.16)             | Implemented | `POST /merchant/orders`; HTTP and access tests              |
| Optional `Idempotency-Key` and replay/conflict | Implemented | Order service and concurrent duplicate tests                |
| Real Parcel and Pricing integration            | Implemented | Internal Parcel client and Parcel/Pricing integration tests |
| Merchant order snapshot and fee snapshot       | Implemented | `merchant.merchant_orders`; migration `000010`              |
| Initial `CREATED` timeline                     | Implemented | Order creation transaction                                  |
| Merchant-scoped list and detail (8.17–8.18)    | Implemented | Repository, HTTP and access ownership tests                 |
| `merchant.sync.queue` consumer                 | Implemented | Merchant sync handler and event-flow integration test       |
| Duplicate/older event suppression              | Implemented | `event_id` uniqueness and `last_status_at` ordering tests   |
| Transactional status and timeline updates      | Implemented | Repository rollback and concurrent-event tests              |
| Dashboard status update                        | Implemented | Merchant dashboard and real-stack synchronization test      |

The real-stack browser journey registers a merchant, approves it, creates an API key in the portal, creates an order with the key, reads the snapshot and timeline, publishes a `parcel.status_changed.v1` event, and verifies the synchronized status in the portal and dashboard. A second merchant receives `404 NOT_FOUND` for the first merchant's order.
