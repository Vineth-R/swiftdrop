# Merchant Integration

This document records Member 8's merchant order and integration flows. The merchant service owns merchant accounts, API clients, order snapshots, status timelines, and administrative audit records. It does not read another service's tables; parcel creation and pricing are internal service integrations.

The report deliverables are [`section 2.3`](report/section_2_3_merchant_use_cases.md) and [`section 6.2`](report/section_6_2_merchant_order_creation.md).

## Merchant use cases (Section 2.3)

| ID    | Use case                           | Primary actor            | Endpoint / integration |
| ----- | ---------------------------------- | ------------------------ | ---------------------- |
| UC-M1 | Register as merchant               | Merchant                 | 8.1                    |
| UC-M2 | Manage API keys                    | Merchant                 | 8.9–8.11               |
| UC-M3 | Create order through API           | External Merchant System | 8.16                   |
| UC-M4 | Configure webhook                  | Merchant                 | 8.12, 8.13             |
| UC-M5 | Receive status webhooks            | External Merchant System | 10 (outbound delivery) |
| UC-M6 | View order dashboard               | Merchant                 | 8.17, 8.18             |
| UC-M7 | Test webhook and view delivery log | Merchant                 | 8.14, 8.15             |
| UC-M8 | Manage business profile            | Merchant                 | 8.2, 8.3               |

The Admin actor approves, rejects, and suspends merchants through the administrative endpoints. The External Merchant System authenticates with `X-API-Key` and submits orders.

### Use-case responsibilities

- **UC-M1:** A merchant submits owner and business details for administrator approval.
- **UC-M2:** An approved merchant creates, lists, and revokes server-to-server API keys.
- **UC-M3:** An external store submits a delivery order and receives a tracking number and fee.
- **UC-M4:** A merchant configures webhook destinations and delivery preferences through M9.
- **UC-M5:** The external store receives signed `parcel.status_changed` events at its configured HTTPS endpoint through the webhook worker (API v1.1 section 10).
- **UC-M6:** A merchant searches orders, filters by status, and views order history and details.
- **UC-M7:** A merchant sends a test webhook and reviews delivery attempts through M9.
- **UC-M8:** A merchant views and edits contact and default pickup information.

## Merchant API order creation (Section 6.2)

1. The external merchant system calls `POST /api/v1/merchant/orders` with `X-API-Key`.
2. Merchant Integration extracts the lookup identifier, verifies the key hash, and confirms that the merchant is approved.
3. It validates the payload and checks `externalOrderId`, `Idempotency-Key`, and the normalized `request_hash`.
4. If pickup details are omitted, the saved merchant pickup address, contact phone, business name and zone are used; the pickup city is the Pricing zone name.
5. Merchant Integration calls Parcel Operations through the authenticated internal `POST /internal/parcels` contract.
6. Parcel Operations applies pricing and creates the parcel.
7. Parcel Operations returns the tracking number and fee breakdown.
8. Merchant Integration stores the order, pickup/recipient snapshots, fee snapshot and the first `CREATED` status event in one local transaction.
9. The service returns HTTP 201 with the order, tracking number, status, and fee.

Identical duplicate requests return the original order. A reused external order identifier or idempotency key with a different normalized payload returns `409 DUPLICATE_ORDER`. Invalid credentials return `401`, unapproved merchants return `403`, invalid payloads return `422`, and unavailable Parcel Operations returns `503`.

## Trust boundary with Parcel Operations

`POST /internal/parcels` is a trusted internal contract, not another
authentication authority. Parcel verifies the shared internal token, validates
the request, and keeps ownership stable when the same `(merchantId,
externalOrderId)` is replayed, but it cannot verify that the supplied
`createdByUserId` owns the merchant or that the merchant is approved. The
Merchant service owns API-key authentication, merchant status, and the
owner relationship; its order caller enforces those checks before calling
Parcel and never accepts `createdByUserId` from an external client.

The implemented caller is API 8.16. It authenticates `X-API-Key`, resolves the
merchant and owner from the key record, requires `APPROVED`, validates the
request, and sends the verified `merchantId` and `createdByUserId` to Parcel.
Parcel applies the real Pricing quote and creates the parcel; the Merchant
service does not duplicate parcel creation or pricing logic.

## Idempotency and concurrency

The normalized request hash is SHA-256 over a fixed-order structure with a
canonical two-decimal weight. The same `externalOrderId` or `Idempotency-Key`
with the same hash returns the original order with `200`; a different hash
returns `409 DUPLICATE_ORDER`. A request without `Idempotency-Key` still
deduplicates by `externalOrderId`.

Parcel serializes creation for `(merchantId, externalOrderId)` and replays the
existing parcel, so a client timeout followed by a retry cannot create a second
parcel. The Merchant service persists the order and its initial `CREATED`
timeline row in one transaction; a unique-conflict loser re-reads the winner and
returns it only when the request hash matches.

## Status synchronization

`merchant.sync.queue` is bound to `parcel.status_changed.v1`. The service requires
`RABBITMQ_URL` outside development; local development may omit it and will log a
warning that synchronization is disabled. The consumer
validates the envelope, type, version, producer and status fields, then locks the
merchant order row and applies the event only when `occurred_at` is newer than
`last_status_at`. The event ID is unique, so a redelivery is a no-op, and the
timeline insert and status update share one transaction. Customer parcels have
no `merchant_id` and are ignored. Merchant-owned parcels without an order
snapshot are logged and acknowledged; malformed events and tracking-number
mismatches are dead-lettered.

## Figures

The Mermaid sources for the report figures are in [`docs/diagrams/merchant-integration.md`](diagrams/merchant-integration.md).

## Merchant portal access

The Next.js portal exposes `/merchant/orders` and `/merchant/orders/{orderId}`
to a signed-in `MERCHANT`. The list and detail screens use the merchant session
cookie, never an API key, and the API scopes every query to the authenticated
merchant. A PENDING, REJECTED or SUSPENDED merchant sees the approval/suspension
notice and no order data. The dashboard shows real total, in-progress, delivered
and failed counts plus recent orders from the same API.

The portal does not implement a browser order-creation form. API 8.16 is
API-key-only and the key must stay on the merchant's server. Instead,
`/merchant/orders` includes a Create via API panel with the endpoint, required
headers, request/response examples, an API documentation link and a shortcut to
API key management. This keeps API v1.1 unchanged and prevents a merchant secret
from entering browser storage or client-side requests.
