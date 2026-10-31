# Merchant Integration

This document records Member 8's merchant order and integration flows. The merchant service owns merchant accounts, API clients, order snapshots, status timelines, and administrative audit records. It does not read another service's tables; parcel creation and pricing are internal service integrations.

## Merchant use cases (Section 2.3)

| ID | Use case | Primary actor | Endpoint / integration |
| --- | --- | --- | --- |
| UC-M1 | Register as merchant | Merchant | `POST /api/v1/merchants/register` |
| UC-M2 | Manage API keys | Merchant | `GET/POST/DELETE /api/v1/merchants/me/keys` |
| UC-M3 | Create order through API | External Merchant System | `POST /api/v1/merchant/orders` |
| UC-M4 | Configure webhook | Merchant | M9 webhook APIs |
| UC-M5 | Receive status webhooks | External Merchant System | M9 webhook delivery |
| UC-M6 | View order dashboard | Merchant | `GET /api/v1/merchant/orders` and `/{orderId}` |
| UC-M7 | Test webhook and view delivery log | Merchant | M9 webhook APIs |
| UC-M8 | Manage business profile | Merchant | `GET/PATCH /api/v1/merchants/me` |

The Admin actor approves, rejects, and suspends merchants through the administrative endpoints. The External Merchant System authenticates with `X-API-Key` and submits orders.

## Merchant API order creation (Section 6.2)

1. The external merchant system calls `POST /api/v1/merchant/orders` with `X-API-Key`.
2. Merchant Integration extracts the lookup identifier, verifies the key hash, and confirms that the merchant is approved.
3. It validates the payload and checks `externalOrderId`, `Idempotency-Key`, and the normalized `request_hash`.
4. If pickup details are omitted, the saved merchant pickup address and zone are used.
5. Merchant Integration calls Parcel Operations through the authenticated internal `POST /internal/parcels` contract.
6. Parcel Operations applies pricing and creates the parcel.
7. Parcel Operations returns the tracking number and fee breakdown.
8. Merchant Integration stores the order snapshot and inserts the first `CREATED` status event in one local transaction.
9. The service returns HTTP 201 with the order, tracking number, status, and fee.

Identical duplicate requests return the original order. A reused external order identifier or idempotency key with a different normalized payload returns `409 DUPLICATE_ORDER`. Invalid credentials return `401`, unapproved merchants return `403`, invalid payloads return `422`, and unavailable Parcel Operations returns `503`.

## Figures

The Mermaid sources for the report figures are in [`docs/diagrams/merchant-integration.md`](diagrams/merchant-integration.md).
