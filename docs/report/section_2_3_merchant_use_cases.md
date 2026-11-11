## 2.3 Merchant Use Cases

**Actors:** Merchant (`MERCHANT` role), External Merchant System, Administrator (`ADMIN` role).

A merchant applies for a business account, waits for an administrator decision, and then manages the business profile, API credentials and webhook settings. Orders are submitted by the merchant's own e-commerce system with an API key and are read back through the merchant portal. Administrators review applications and can approve, reject or suspend an account.

Table 2.3 lists the merchant use cases and their API v1.1 endpoints.

### Table 2.3: Merchant Use Cases

| ID    | Use case                           | Description                                                                                                                | API v1.1                                                                   |
| :---- | :--------------------------------- | :------------------------------------------------------------------------------------------------------------------------- | :------------------------------------------------------------------------- |
| UC-M1 | Register as merchant               | Submit owner and business details. The Identity account stays inactive and the merchant stays `PENDING` until approval.    | `8.1` (`POST /api/v1/merchants/register`)                                  |
| UC-M2 | Manage API keys                    | An approved merchant lists, creates and revokes server-to-server keys. The full key is shown once.                         | `8.9`–`8.11` (`/api/v1/merchants/me/keys`)                                 |
| UC-M3 | Create order through API           | The external store submits an order with `X-API-Key` and receives the tracking number and fee snapshot.                    | `8.16` (`POST /api/v1/merchant/orders`)                                    |
| UC-M4 | Configure webhook                  | An approved merchant stores an HTTPS destination, subscribed events and an active flag. A signing secret is returned once. | `8.12`, `8.13` (`/api/v1/merchants/me/webhook`)                            |
| UC-M5 | Receive status webhooks            | The external store receives signed `parcel.status_changed` events from the webhook worker.                                 | Section 10 outbound delivery (webhook worker)                              |
| UC-M6 | View order dashboard               | The merchant lists its own orders, filters them and opens the snapshot, fee and status timeline.                           | `8.17`, `8.18` (`/api/v1/merchant/orders`, `/api/v1/merchant/orders/{id}`) |
| UC-M7 | Test webhook and view delivery log | The merchant sends a signed sample event and reviews recorded delivery attempts, including retries and dead letters.       | `8.14`, `8.15` (`/api/v1/merchants/me/webhook/test`, `/deliveries`)        |
| UC-M8 | Manage business profile            | The merchant views and updates contact details and the default pickup address and zone.                                    | `8.2`, `8.3` (`GET`/`PATCH /api/v1/merchants/me`)                          |

The Administrator additionally manages the application and account lifecycle:

| Operation       | API v1.1                                                    |
| :-------------- | :---------------------------------------------------------- |
| List merchants  | `8.4` (`GET /api/v1/admin/merchants`)                       |
| Merchant detail | `8.5` (`GET /api/v1/admin/merchants/{merchantId}`)          |
| Approve         | `8.6` (`POST /api/v1/admin/merchants/{merchantId}/approve`) |
| Reject          | `8.7` (`POST /api/v1/admin/merchants/{merchantId}/reject`)  |
| Suspend         | `8.8` (`POST /api/v1/admin/merchants/{merchantId}/suspend`) |

### Merchant Lifecycle and Scoping Rules

1. **One merchant per owner.** `merchant.merchants.owner_user_id` is unique. A user cannot own two merchant records.
2. **Approval lifecycle.** A registration creates a `PENDING` application and an inactive Identity `MERCHANT` account. Approval is allowed from `PENDING` or `SUSPENDED`; rejection is allowed from `PENDING`; suspension is allowed from `APPROVED`. Other transitions return `409 INVALID_MERCHANT_STATE`.
3. **Identity synchronization.** Every actual approval, rejection, suspension and reapproval is a lifecycle transition with a unique transition ID and a per-merchant sequence. Identity applies each transition once and advances the session version. A failed Identity call leaves the merchant blocked and reports `identitySync: PENDING` to the administrator so the decision can be retried safely.
4. **Audit.** Approval, rejection and suspension write append-only rows to `merchant.audit_logs` with the administrator, action, merchant and reason metadata.
5. **API-key ownership.** Keys belong to exactly one merchant. Lookup IDs are unique. Only a SHA-256 hash of the full key is stored, and the full key is never returned after creation.
6. **Approval gate.** Only an `APPROVED` merchant can create API keys or use them. Pending, rejected and suspended merchants are refused with `403 MERCHANT_NOT_APPROVED`; revoked or unknown keys are refused with `401 INVALID_API_KEY`.
7. **Five-key limit.** At most five active keys exist per merchant. The limit is enforced by a database trigger and a merchant row lock, not by an application count alone.
8. **Merchant identity is never client-supplied.** Order creation and order reads resolve the merchant from the API key or the verified session. A request body cannot choose a merchant or owner.
9. **Strict merchant scoping.** List, detail and every write are filtered by the authenticated merchant ID. Another merchant's order is indistinguishable from a missing order and returns `404 NOT_FOUND`.
10. **API 8.16 is API-key-only.** The browser portal never handles a merchant API key. It documents the endpoint and links to API-key management instead of offering a browser order form.

### Implementation Status

| Capability                                   | Status      | Evidence                                                           |
| :------------------------------------------- | :---------- | :----------------------------------------------------------------- |
| Registration and pending application         | Implemented | `POST /merchants/register`; merchant onboarding tests              |
| Administrator list, detail and decisions     | Implemented | `/admin/merchants` routes; lifecycle integration and HTTP tests    |
| Approval, rejection and suspension audits    | Implemented | `merchant.audit_logs` writes in the lifecycle transaction          |
| API-key list, creation and revocation        | Implemented | `/merchants/me/keys`; key access, limit and concurrency tests      |
| API-key-created security notice              | Implemented | `emails.MerchantAPIKeyCreated`; asynchronous merchant notifier     |
| Order creation and merchant-scoped reads     | Implemented | API 8.16–8.18; order service and HTTP/PostgreSQL integration tests |
| Status synchronization                       | Implemented | `merchant.sync.queue` consumer; event-flow integration test        |
| Webhook settings, test send and delivery log | Implemented | API 8.12–8.15, shared merchant webhook tests                       |
| Browser order creation                       | Not offered | API 8.16 remains API-key-only by design                            |

The merchant portal exposes the order dashboard, order detail, business profile and API-key screens. A pending, rejected or suspended merchant sees the approval or suspension notice and no order data.
