# Merchant Integration

This document records Member 8's merchant order and integration flows. The merchant service owns merchant accounts, API clients, order snapshots, status timelines, and administrative audit records. It does not read another service's tables; parcel creation and pricing are internal service integrations.

## Merchant use cases (Section 2.3)

| ID    | Use case                           | Primary actor            | Endpoint / integration |
| ----- | ---------------------------------- | ------------------------ | ---------------------- |
| UC-M1 | Register as merchant               | Merchant                 | 8.1                    |
| UC-M2 | Manage API keys                    | Merchant                 | 8.9–8.11               |
| UC-M3 | Create order through API           | External Merchant System | 8.16                   |
| UC-M4 | Configure webhook                  | Merchant                 | 8.12, 8.13             |
| UC-M5 | Receive status webhooks            | External Merchant System | Section 10             |
| UC-M6 | View order dashboard               | Merchant                 | 8.17, 8.18             |
| UC-M7 | Test webhook and view delivery log | Merchant                 | 8.14, 8.15             |
| UC-M8 | Manage business profile            | Merchant                 | 8.2, 8.3               |

The Admin actor approves, rejects, and suspends merchants through the administrative endpoints. The External Merchant System authenticates with `X-API-Key` and submits orders.

### Use-case responsibilities

- **UC-M1:** A merchant submits owner and business details for administrator approval.
- **UC-M2:** An approved merchant creates, lists, and revokes server-to-server API keys.
- **UC-M3:** An external store submits a delivery order and receives a tracking number and fee.
- **UC-M4:** A merchant configures webhook destinations and delivery preferences through M9.
- **UC-M5:** The external store receives parcel status updates through M9's webhook delivery flow.
- **UC-M6:** A merchant searches orders, filters by status, and views order history and details.
- **UC-M7:** A merchant sends a test webhook and reviews delivery attempts through M9.
- **UC-M8:** A merchant views and edits contact and default pickup information.

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

## Trust boundary with Parcel Operations

`POST /internal/parcels` is a trusted internal contract, not another
authentication authority. Parcel verifies the shared internal token, validates
the request, and keeps ownership stable when the same `(merchantId,
externalOrderId)` is replayed, but it cannot verify that the supplied
`createdByUserId` owns the merchant or that the merchant is approved. The
Merchant service owns API-key authentication, merchant status, and the
owner relationship; its order caller must enforce those checks before calling
Parcel and must never accept `createdByUserId` from an external client.

This integration is intentionally documented as pending: the Merchant order
route (UC-M3) is not implemented in the current tree, and no trusted internal
merchant lookup exists for Parcel to call. Until both exist and are reviewed,
Parcel's merchant creation path is safe only behind the internal network and
must not be described as fully resolving merchant ownership.

## Figures

The Mermaid sources for the report figures are in [`docs/diagrams/merchant-integration.md`](diagrams/merchant-integration.md).
