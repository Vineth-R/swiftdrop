# Merchant internal API

Service-to-service endpoints of the Merchant service for the webhook and
notification workers (API v1.1 section 13). The machine-readable contract is
[`contracts/openapi/merchant-internal.yaml`](../contracts/openapi/merchant-internal.yaml).

| Method and path                                          | Caller              | Purpose                                                                                                                                    |
| -------------------------------------------------------- | ------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| `GET /internal/merchants/{merchantId}/webhook`           | Webhook worker      | Returns `url`, `signingSecret`, `eventTypes`, `isActive` and `merchantStatus`. The only place the plain signing secret leaves the service. |
| `POST /internal/merchants/{merchantId}/webhook/attempts` | Webhook worker      | Records one delivery attempt for the merchant's delivery log (API 8.15). Idempotent.                                                       |
| `GET /internal/merchants/{merchantId}/contact`           | Notification worker | Returns `ownerUserId`, `businessName`, `contactEmail` and `status` so merchant-owned parcel events can be emailed.                         |

## Access rules

- Nginx never routes `/internal`, so only services on the Compose network can
  reach these endpoints (`scripts/check-routes.sh` already proves the gateway
  answers 404).
- Every call needs `X-Internal-Token` (the shared `INTERNAL_TOKEN`) and a
  well-formed `X-Request-Id`. The token is checked first, so a caller without it
  learns nothing else.
- Fail closed: without a valid `INTERNAL_TOKEN` every call answers 401; without
  `WEBHOOK_SECRET_KEY` (or a database) a call with the right token answers 503.
- In development `INTERNAL_TOKEN` must be at least 16 characters, otherwise the
  endpoints stay closed. Outside development a set but weak token (placeholder,
  under 32 bytes) stops the service at startup, the same rule as the other
  services.
- Responses carry `Cache-Control: no-store`. The signing secret is never logged.

## Merchant contact lookup

`GET /internal/merchants/{merchantId}/contact` returns the merchant's own
business contact, not a shopper's details:

```json
{
  "data": {
    "merchantId": "4f1c7a52-0000-4000-8000-000000000002",
    "ownerUserId": "5f1c7a52-0000-4000-8000-000000000003",
    "businessName": "ShopZone",
    "contactEmail": "ops@shopzone.lk",
    "status": "APPROVED"
  },
  "meta": {
    "requestId": "req_01HF-example",
    "timestamp": "2026-11-11T09:30:00Z"
  }
}
```

- The recipient is `contactEmail`. Phone and signing secrets are never returned.
- A `PENDING`, `SUSPENDED` or `REJECTED` merchant is returned with its real
  status; the notification worker applies its own policy and does not send.
- Unknown merchants answer `404`; a missing token answers `401` first; a
  service without a database answers `503`.
- The notification worker treats `404`, `401`/`403` and malformed responses as
  permanent, and `5xx`/timeouts as retryable.

## Recording attempts

`POST .../webhook/attempts` body:

```json
{
  "eventId": "e1a2b3c4-5c6d-4e7f-8a9b-0c1d2e3f4a5b",
  "parcelId": "e5a1b7c2-6d3f-4a8b-9c10-2d3e4f5a6b7c",
  "trackingNumber": "SDN4T8W2LC6",
  "attemptNumber": 2,
  "httpStatus": 503,
  "success": false,
  "errorMessage": "upstream unavailable",
  "nextRetryAt": "2026-11-04T12:35:00Z",
  "deadLettered": false,
  "durationMs": 182
}
```

- `parcelId` is required beyond the fields of API 8.15, because the table stores
  it (database design 6.5).
- `success` is required; a missing field is rejected with
  `422 VALIDATION_ERROR` instead of being treated as `false`.

## Webhook delivery lifecycle

The webhook worker re-reads this endpoint on every attempt. It only sends when
`merchantStatus` is `APPROVED`, `isActive` is true and
`parcel.status_changed` is subscribed. A suspended, pending or rejected
merchant is suppressed before the HTTP request, including on retries. A webhook
deleted or made inactive between attempts is skipped.

A suppression is recorded as a dead-lettered delivery attempt with a stable
reason (`merchant_suspended`, `merchant_pending`, `merchant_rejected`,
`webhook_inactive` or `webhook_not_subscribed`) and is never scheduled for a
retry. Reinstating a merchant therefore does not replay previously suppressed
deliveries. A delivery already in progress is allowed to finish.

- `(webhook, eventId, attemptNumber)` is unique. A repeated report answers
  `200` with `recorded: false` and changes nothing; a new attempt answers `201`.
- Rules: `attemptNumber` 1 to 6; `httpStatus` 100 to 599 or omitted on a timeout;
  a successful attempt carries no `nextRetryAt` and is not dead lettered; a
  dead-lettered attempt (attempt 6 failed) carries no `nextRetryAt`.
- `errorMessage` is safe text only (never a payload or a secret) and is cut to
  255 characters.
- `createdAt` in the delivery log (8.15) is response-only. It is generated by
  the Merchant service when the attempt is stored and represents the time the
  service recorded the attempt; the worker does not supply it.

## Checking delivered events

`GET /internal/merchants/{merchantId}/webhook/deliveries/{eventId}` answers
`{"data":{"delivered":true|false}}`. It reads the durable
`merchant.webhook_delivery_attempts` log and reports whether a successful
attempt already exists for this merchant and event. The webhook worker calls it
before sending, so a broker redelivery does not create a second successful
delivery after the first one was recorded.

The endpoint requires `X-Internal-Token` and a well-formed `X-Request-Id`, and
its responses are `no-store`. A malformed event UUID answers `400`.

This check cannot guarantee exactly-once delivery across an HTTP boundary. If a
receiver accepted a webhook and the worker crashed before recording success, a
redelivery can send it again. Receivers must deduplicate on `eventId`.

## Trust boundaries

- `INTERNAL_TOKEN` is shared by every internal caller. Any service that holds it
  can call the internal GET and read a merchant's signing secret, not only the
  webhook worker. This is the existing internal-authentication model, not a
  per-service identity.
- `parcelId` is trusted caller-supplied data. The Merchant service does not
  verify that the parcel belongs to the merchant; the webhook worker is the
  trusted source.
- Retry and dead-letter consistency is partly owned by the worker. The endpoint
  rejects contradictory combinations, but it does not enforce the full retry
  schedule (for example, that attempt 6 is dead-lettered).

## Configuration

| Variable             | Used for                                                                 |
| -------------------- | ------------------------------------------------------------------------ |
| `INTERNAL_TOKEN`     | Guards both endpoints (shared with the other internal APIs).             |
| `WEBHOOK_SECRET_KEY` | Decrypts the stored signing secret (same key as the settings endpoints). |
