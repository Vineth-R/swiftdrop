# Webhook worker

Delivers `parcel.status_changed.v1` events to merchants' webhooks (API v1.1
sections 10 and 13).

## Flow for one message

1. Read the event from queue `webhook.parcel.status_changed.v1`.
2. Skip it when the parcel has no `merchant_id` or the merchant has no webhook
   (`404 WEBHOOK_NOT_CONFIGURED`).
3. Re-check the merchant lifecycle and webhook configuration on every attempt.
   It sends only when `merchantStatus` is `APPROVED`, `isActive` is true and
   `parcel.status_changed` is subscribed. A suspended, pending or rejected
   merchant, or an inactive/unsubscribed webhook, is suppressed before the HTTP
   request and recorded as a dead-lettered attempt with a stable reason. A
   suppressed delivery is not retried or replayed on reinstatement.
4. Read `externalOrderId` from the Parcel service
   (`GET /internal/parcels/{id}/summary`).
5. Send one signed POST (`packages/webhookdelivery`, 10 s timeout, no redirects).
6. Report the attempt to the Merchant service
   (`POST /internal/merchants/{id}/webhook/attempts`) so it shows in the
   merchant's delivery log.
7. On failure, schedule the next attempt; after the 6th failure the attempt is
   reported with `deadLettered: true` and nothing more is sent.

| Attempt | Wait before it                      |
| ------- | ----------------------------------- |
| 1       | none (as soon as the event arrives) |
| 2       | 1 minute after attempt 1 failed     |
| 3       | 5 minutes                           |
| 4       | 30 minutes                          |
| 5       | 2 hours                             |
| 6       | 6 hours                             |

## Retries survive a restart

The wait is held by RabbitMQ, not by the worker. For each delay the worker
declares a queue `webhook.parcel.status_changed.v1.delay.<seconds>s` with a fixed
message TTL. When the TTL ends, RabbitMQ dead-letters the message through the
default exchange back into the worker's own queue. The attempt number travels in
the `x-webhook-attempt` header (no header means attempt 1). The body and message
ID are unchanged, so the merchant sees the same `eventId` on every attempt.

## Failure handling

- A failing merchant endpoint is a normal outcome: recorded and retried by the
  schedule above. It is never returned to the shared consumer as an error.
- A failure of our own infrastructure (Merchant or Parcel service, RabbitMQ) is
  returned as an error, and the shared consumer retries the message
  (`RABBITMQ_MAX_RETRIES`, `RABBITMQ_RETRY_DELAY`).
- If a failure happens after the send (recording or scheduling), the retry may
  send the same event again. Receivers de-duplicate on `eventId`; recording an
  attempt is idempotent.
- Malformed messages (bad JSON, wrong type or version, bad attempt header) are
  permanent errors and go to the dead-letter queue.
- A suppressed delivery is recorded as `deadLettered: true` with a stable
  lifecycle reason. If that audit record cannot be written, the message is
  dead-lettered rather than retried into a later delivery after reinstatement.

## Configuration

| Variable                  | Default                | Purpose                                       |
| ------------------------- | ---------------------- | --------------------------------------------- |
| `INTERNAL_TOKEN`          | none                   | Sent as `X-Internal-Token` on internal calls. |
| `MERCHANT_BASE_URL`       | `http://merchant:8084` | Merchant service origin.                      |
| `PARCEL_BASE_URL`         | `http://parcel:8082`   | Parcel service origin.                        |
| `INTERNAL_CLIENT_TIMEOUT` | `5s`                   | Timeout of one internal call.                 |

Outside development a missing, placeholder or short `INTERNAL_TOKEN` stops the
service. In development it only logs a warning and deliveries fail until the
token is set. The signing secret is never logged.
