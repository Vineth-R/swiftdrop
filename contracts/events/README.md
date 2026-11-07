# Event contracts

SwiftDrop uses RabbitMQ for asynchronous communication. Events are JSON messages published to the durable topic exchange `swiftdrop.events`.

## Naming and versioning

Event names use `<domain>.<past_tense_action>.v<major>`:

- `parcel.created.v1`
- `parcel.assigned.v1`
- `parcel.status_changed.v1`
- `notification.requested.v1` (legacy; see below)
- `webhook.delivery.requested.v1` (retired legacy work item; see below)

The major version is part of both the event type and its routing key. Breaking changes require a new major version and a new queue binding. A producer must not silently change the meaning of an existing versioned event.

Each event file follows `<event_name>.schema.json` and uses JSON Schema draft 2020-12.

## Source of truth and compatibility layers

| Layer                     | Contract                                                                                                                                                                                                                                                                                                                                                                                                  |
| ------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Database outbox storage   | Unversioned labels `parcel.created`, `parcel.assigned`, and `parcel.status_changed` with the exact stored routing keys below. The database rejects `.v1` labels and mixed pairs; parcel `000005_outbox_event_types` reasserts the contract. The stored `routing_key` is a legacy/internal value and is not the published routing key. Creation and M6 dispatch writers plus the M6 relay are implemented. |
| Published RabbitMQ events | Versioned event names with the same versioned routing key. This is the canonical bus contract. Creation outbox data is implemented; bus publication is not.                                                                                                                                                                                                                                               |
| Internal envelope         | `packages/go-common/messaging.Envelope`: snake_case metadata and a nested object in `data`. Envelope validation is implemented.                                                                                                                                                                                                                                                                           |
| Public merchant webhook   | The API v1.1 camelCase projection with public `eventType: parcel.status_changed` and `schemaVersion: 1`. Delivery and signing are not implemented.                                                                                                                                                                                                                                                        |
| Legacy work items         | `notification.requested.v1` has a consumer but no producer. `webhook.delivery.requested.v1` is retired: the webhook worker now consumes `parcel.status_changed.v1` on its own queue. Neither is a target domain-event contract.                                                                                                                                                                           |

Do not use the database `routing_key` column directly as a RabbitMQ routing key. The relay owns the storage-to-bus mapping.

## Envelope

Every event uses the same envelope fields:

| Field            | Type               | Required | Purpose                                                        |
| ---------------- | ------------------ | -------- | -------------------------------------------------------------- |
| `event_id`       | string             | yes      | Unique event identifier used for idempotency.                  |
| `event_type`     | string             | yes      | Versioned event name, for example `notification.requested.v1`. |
| `event_version`  | integer            | yes      | Major schema version. Must match the event type suffix.        |
| `occurred_at`    | RFC 3339 timestamp | yes      | Time the event occurred.                                       |
| `producer`       | string             | yes      | Service that produced the event.                               |
| `correlation_id` | string             | no       | Request or workflow correlation identifier.                    |
| `data`           | object             | yes      | Event-specific payload defined by the schema.                  |

An example implementation lives in `packages/go-common/messaging/envelope.go`.

The internal envelope uses `event_version` (an integer), not `schemaVersion`. `schemaVersion` belongs only to the public merchant webhook projection, which keeps the unversioned public `eventType: parcel.status_changed` and adds `schemaVersion: 1` for API v1.1 compatibility. Do not add `schemaVersion` to the internal envelope or store it in the outbox.

## Envelope validation (implemented)

`Envelope.Validate` enforces the documented contract without adding product semantics:

- `event_id`, `event_type`, and `producer` are required and must not be blank.
- `event_version` must be at least `1`.
- `event_version` must equal the major version in the `.v<major>` suffix of `event_type`. The suffix must be present, numeric, greater than zero, and canonical (no leading zeros). For example, `notification.requested.v1` requires `event_version: 1`.
- `occurred_at` must be set; the zero time is rejected.
- `data` must be a JSON object. `null`, arrays, scalars, empty values, and malformed JSON are rejected.
- Unknown JSON fields are ignored by Go unmarshalling so additive changes remain compatible. See [Evolution](#evolution).
- Validation errors name the failing field, event type, or version. They never include payload values, recipient data, or other message contents.

Envelope validation is permanent-failure validation: a message that fails it is not retried. The current worker handlers apply it, then require the event type and version they consume.

## Message framing

Before a handler unmarshals a message body, the consumer applies the shared framing checks:

- The body must be at most `messaging.MaxMessageBytes` (1 MiB). The limit is fixed in code; there is no environment variable for it in this phase. A configurable limit requires an explicit operational decision.
- The content type must be absent or `application/json`. Optional parameters such as `charset=utf-8` are accepted. Any other media type, including `application/json-patch+json` and `application/xml`, is rejected.
- An absent content type is tolerated for compatibility with producers that do not set one.

`messaging.ValidateMessage` implements these checks and returns a plain error for the caller to classify as permanent. The constant and helper live in `packages/go-common/messaging/validation.go`.

The shared consumer invokes `messaging.ValidateMessage` before calling a handler. An invalid content type or oversized body is therefore a permanent failure and is dead-lettered without retrying.

## Outbox storage and published mapping

The transactional outbox is the source record for parcel events. The unversioned storage labels and exact pairs below are enforced by the database; parcel `000005_outbox_event_types` reasserts them and rejects `.v1` labels and mixed pairs:

| Storage label           | Stored routing key (legacy) | Published event type       | Published routing key      |
| ----------------------- | --------------------------- | -------------------------- | -------------------------- |
| `parcel.created`        | `parcel.created`            | `parcel.created.v1`        | `parcel.created.v1`        |
| `parcel.assigned`       | `parcel.assigned`           | `parcel.assigned.v1`       | `parcel.assigned.v1`       |
| `parcel.status_changed` | `parcel.status.changed`     | `parcel.status_changed.v1` | `parcel.status_changed.v1` |

The relay must validate the storage label, build the canonical envelope, and publish with the mapped event type and routing key. It must not copy the stored legacy routing key. The event ID is the outbox row UUID; the payload is the versioned event's `data` object, not the full envelope.

The Parcel creation writer inserts `parcel.created` with aggregate metadata and
the canonical event's `data` object in the same transaction as the parcel and
initial history. The merged M6 relay publishes committed events with confirms
and bounded retries.
The relay must preserve this committed boundary and needs a documented claim/lock
strategy before it can safely run with more than one instance.

## Producers

- Publish to the `swiftdrop.events` topic exchange with the versioned event name as the routing key.
- Reuse the event envelope and validate the payload against the versioned JSON schema.
- Prefer asynchronous events when the caller does not need an immediate result and the work can be retried independently.
- Use synchronous HTTP when the caller needs an immediate response, such as validation or a read.

## Consumers

- Declare a durable queue, bind it to the versioned routing key, and set an explicit prefetch limit.
- Acknowledge a message only after processing succeeds or after a retry/dead-letter publication is confirmed by the broker.
- Treat delivery as at-least-once: deduplicate by `event_id` and make handlers idempotent. Duplicate or redelivered messages are normal, including after reconnects and retries.
- Retry unclassified handler failures up to `RABBITMQ_MAX_RETRIES` (default `3`) with `RABBITMQ_RETRY_DELAY` (default `30s`). After exhaustion, route the message to the dead-letter queue. Shutdown requeues in-flight messages.
- Retry and dead-letter resources are derived from the main exchange and queue: `<exchange>.retry`, `<queue>.retry.v2`, `<exchange>.dlx`, and `<queue>.dlq`. A retry is dead-lettered to its owning queue through the default exchange (`x-dead-letter-exchange: ""`, `x-dead-letter-routing-key: <queue>`) after the delay.
- Multiple workers may bind separate durable queues to the same versioned routing key; queue-isolated retry routing means a retry never reaches another worker's queue.
- Retry count, original exchange and routing key, failure reason, and failure time are carried in message headers. Do not use these headers as authorization or trust them as business data.
- Connection URLs, passwords, and other credentials are never logged. AMQP URL parse and dial errors are redacted before they reach structured logs.
- The dead-letter queue is for inspection and replay; no consumer is implemented in the foundation.
- Do not require RabbitMQ to run ordinary unit tests; test handlers directly and reserve broker tests for integration tests.

### Permanent and retryable failures

Permanent failures are not retried and go directly to the dead-letter queue:

- malformed JSON that cannot be unmarshalled into the envelope
- an envelope that fails `Envelope.Validate`, including a version suffix mismatch or non-object `data`
- an unsupported event type or event version
- invalid event data when a worker validates payload data
- an unsupported message content type or an oversize message body

Unclassified handler errors are retryable and use the bounded retry policy. Only successful handling or a confirmed retry/dead-letter publication acknowledges the original delivery. A failed retry or dead-letter publication requeues the message instead of acknowledging it.

## Event payload schemas

Parcel domain events use the following versioned schemas:

- [`parcel.created.v1.schema.json`](parcel.created.v1.schema.json)
- [`parcel.assigned.v1.schema.json`](parcel.assigned.v1.schema.json)
- [`parcel.status_changed.v1.schema.json`](parcel.status_changed.v1.schema.json)

Each schema validates the canonical envelope (`event_id`, `event_type`, `event_version`, `occurred_at`, `producer`, optional `correlation_id`, and object-shaped `data`), requires `producer` to be `parcel`, and closes the `data` object with `additionalProperties: false`.

`parcel.created.v1` data contains `parcel_id`, `tracking_number`, the initial `status` (`CREATED`), and exactly one of `customer_user_id` or `merchant_id`. The schema rejects both missing owners and both owners, matching the parcel database constraint.

`parcel.assigned.v1` data contains `parcel_id`, `tracking_number`, `courier_id`, `assigned_by`, and optional `previous_courier_id`.

`parcel.status_changed.v1` data contains `parcel_id`, `tracking_number`, `previous_status`, `status`, optional `merchant_id`, optional `customer_user_id`, optional `actor_user_id`, optional `note`, and `failure_reason` (required when `status` is `FAILED`, 5 to 255 characters). These match the parcel database and state-machine rules.

Domain event payloads must not contain phone numbers, email addresses, physical addresses, passwords, JWTs or access tokens, webhook signing secrets, or raw authentication credentials. Contact details are resolved through the appropriate internal service. Parcel event schemas therefore expose identifiers and lifecycle attributes, not contact data.

Customer and internal merchant creation now produce the `parcel.created` outbox
data described above. The notification consumer is implemented for customer-owned
published events. M6 relay publication and assignment producers are implemented;
M7 courier status producers remain pending.

### Notification rendering (implemented)

The notification worker supports the legacy `notification.requested.v1` work item and the customer-owned `parcel.created.v1` and `parcel.status_changed.v1` domain events. It validates the envelope, the event type and version, and the `data` fields it uses before delivering anything. Invalid `data` is a permanent failure. For domain events it resolves the customer's contact through the identity internal endpoint and sends the branded parcel email; merchant-owned events are unsupported and are dead-lettered. The legacy path is unchanged.

The notification worker requires `data.parcel_id` as a UUID. It is the parcel the notification belongs to and is stored in `notification.notification_logs.parcel_id`, which is `NOT NULL`. A missing or malformed `parcel_id` is a permanent failure and does not trigger delivery or a delivery-log write.

The notification worker delivers the `email` channel through the shared SMTP mailer. It treats `data.template` as the email subject and builds a deterministic plain-text body:

- If `variables` is absent or empty, the body is the template text.
- Otherwise the body is the template, a blank line, then one `key: value` line per variable sorted by key.

`variables`, when present, must be a JSON object whose values are strings. Explicit `null`, arrays, scalars, and objects with non-string values are permanent failures. Validation errors name the failing field only; they do not echo channel values, variable keys, recipients, or other payload data.

`sms` is accepted by the schema but has no provider in this phase, so it is a permanent failure and is dead-lettered without retrying. Recipient addresses, message bodies, templates, and SMTP credentials are never logged.

Dispatches are at-least-once. A message is acknowledged only after the SMTP send succeeds and the delivery transaction commits. The transaction upserts `notification.notification_logs` with `status = 'SENT'`, sets `sent_at`, and inserts the event into `notification.processed_events`; both writes are atomic. A duplicate `event_id` is skipped before delivery and does not create another log row or send another email.

Delivery outcomes use the database's uppercase values:

- `SENT` after a successful SMTP delivery.
- `PENDING` for a retryable delivery failure; the shared consumer retries it.
- `FAILED` for a permanent delivery failure, such as an invalid message or unsupported channel.

A retryable failure is recorded as `PENDING`, but the handler cannot observe final retry exhaustion or dead-lettering. A `PENDING` row can therefore remain after retries are exhausted.

A retryable failure is not recorded as processed. If the failure log cannot be written, the worker returns a retryable database error because the outcome could not be recorded; this can retry an event whose original delivery error was permanent.

A later failure for the same `(event_id, channel, recipient)` preserves an existing `sent_at`; a first-time failure leaves it `NULL`. Processed-event records are retained for 30 days by default. The notification worker deletes rows whose `processed_at` is strictly earlier than the cutoff at startup and then every 24 hours by default. `NOTIFICATION_PROCESSED_EVENTS_RETENTION` (default `720h`) and `NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL` (default `24h`) configure the policy; both must be positive durations. Cleanup never modifies `notification_logs`, and cleanup failures are logged and retried on the next interval without stopping notification consumption. Idempotency is guaranteed only while the processed-event row is retained. After cleanup, a sufficiently late redelivery of the same event can be processed again.

SMTP delivery and the database transaction are separate operations. A duplicate email is still possible after a crash between SMTP acceptance and the transaction commit, when the database write fails after delivery, or when two instances process the same event concurrently. This is at-least-once delivery, not exactly-once delivery.

### Notification v1 direction

`notification.requested.v1` is legacy. It has a consumer but no producer. Its required `recipient` field is raw contact data and conflicts with the domain-event privacy rule above; it must not be exposed to merchants and must not receive new producers without a separately approved internal-command contract.

The target direction is for the notification worker to consume `parcel.created.v1` and `parcel.status_changed.v1`, resolve contact details through the identity internal `GET /internal/users/{userId}/contact` endpoint, and render notifications from non-contact parcel data. Parcel domain events must not contain email addresses or phone numbers.

**Current limitation:** this contact-resolution path only covers a parcel with a `customer_user_id`. A merchant-owned parcel has `merchant_id` instead of a user ID, and the identity endpoint cannot resolve a merchant-order recipient. The API v1.1 internal parcel summary returns `recipientName` but no email or phone, and `merchant.merchant_orders` does not store recipient contact details. Merchant-owned parcel notifications are therefore **unsupported in the current notification target** until an authorized merchant/parcel contact-resolution mechanism is approved. Do not read another service's database, add contact fields to parcel events, or invent a notification producer or endpoint.

Architectural options requiring approval:

- an authorized internal parcel endpoint that returns notification contact data;
- an authorized merchant-order contact lookup;
- an explicitly approved internal notification-command contract;
- or limiting the notification target to customer-owned parcels until one of the above is approved.

The customer-owned domain-event path is implemented. Merchant-owned parcel notifications remain unsupported and are dead-lettered until an authorized contact-resolution mechanism is approved. The legacy `notification.requested.v1` consumer remains.

Per-event schemas may be closed for their specific payload, as the notification example is. Additive changes are still compatible for consumers that ignore unknown fields; update the schema within the same major version when adding optional fields. Adding a required field, removing a field, changing a type, or changing semantics is a breaking change that requires a new major version.

### Public merchant webhook projection (v1 contract; not implemented)

The webhook is a separate public projection. Do not reuse the internal RabbitMQ envelope as the merchant body. Merchants receive only `parcel.status_changed`; `parcel.created` and `parcel.assigned` remain internal events unless a separate public contract is approved.

- Internal trigger: `parcel.status_changed.v1`.
- Public event type: `parcel.status_changed` (unversioned in the API v1.1 public contract).
- Public body is camelCase and additive `schemaVersion: 1` is included.
- Public body fields: `eventId`, `eventType`, `schemaVersion`, `occurredAt`, and `data.trackingNumber`, `data.externalOrderId`, `data.previousStatus`, `data.status`, `data.note`, `data.failureReason`.
- `externalOrderId` is not part of `parcel.status_changed.v1`. The webhook worker must not read a merchant database directly. API v1.1 documents a target `GET /internal/parcels/{parcelId}/summary` response containing `externalOrderId`, but that endpoint is not implemented or present in the parcel OpenAPI contract. It is a required future dependency unless the team approves adding `external_order_id` to the domain event.
- `eventId` is the stable outbox UUID. A webhook delivery-attempt ID is internal and must not replace the merchant idempotency key.
- Signing: HMAC-SHA-256 over `<timestamp>.<raw request body>`, header `X-SwiftDrop-Signature: sha256=<lowercase hex>`, five-minute replay window, constant-time comparison.
- Retry: six attempts following API v1.1: immediate, 1 minute, 5 minutes, 30 minutes, 2 hours, 6 hours; then dead-letter and record the attempt. Business retries must not reuse the shared RabbitMQ consumer retry loop and multiply attempts.
- Delivery logs store outcome metadata, not request or response bodies. Never log payloads, signing secrets, recipient data, or raw responses.

Current state: `webhook-worker` consumes `parcel.status_changed.v1` on its own durable queue, validates the envelope/type/version, and logs only. It does not deliver HTTP requests, sign, or write delivery attempts. The merchant webhook settings, signing-secret lookup, and delivery-attempt recording endpoints are not implemented. `docs/API_Documentation_v1.1.md` section 10 is the public webhook contract; this repository's current runtime does not yet implement it.

The internal trigger for the target flow is `parcel.status_changed.v1`, produced by the parcel service: the M7 courier status writer records a PENDING `parcel.status_changed` outbox row in the status transaction, and the M6 relay publishes `parcel.status_changed.v1` to `swiftdrop.events`. A dedicated `webhook.delivery.requested.v1` dispatcher is not used; it would require a separately approved payload schema and must not duplicate the domain-event publisher.

## Queue and routing topology

- Exchange: `swiftdrop.events` (durable topic exchange).
- Published routing key: the versioned event name.
- Current implemented worker queues:
  - `notification.requested.v1` (legacy notification work item; no producer)
  - `notification.parcel.created.v1` (bound to `parcel.created.v1`)
  - `notification.parcel.status_changed.v1` (bound to `parcel.status_changed.v1`)
  - `webhook.parcel.status_changed.v1` (bound to `parcel.status_changed.v1`)
- Consumer bindings:
  - Notification worker: `parcel.created.v1`, `parcel.status_changed.v1` (implemented for customer-owned parcels; merchant-owned unsupported)
  - Webhook worker: `parcel.status_changed.v1` (implemented; validates and logs only)
  - Realtime: `parcel.assigned.v1`, `parcel.status_changed.v1` (target; not implemented)
  - Merchant integration: `parcel.status_changed.v1` (target; not implemented)
- Queue names are deployment details. Do not rename or redeclare an existing durable queue with different arguments. The notification worker's parcel consumers use separate durable queues; future consumers should add new durable queues or an explicit queue migration rather than changing an existing queue's arguments.
- Failure resources use the shared implementation: retry exchange `<exchange>.retry`, retry queue `<queue>.retry.v2`, dead-letter exchange `<exchange>.dlx`, and dead-letter queue `<queue>.dlq`. The retry queue dead-letters to its owning queue through the default exchange, so retries stay queue-isolated. The API v1.1 legacy name `swiftdrop.dlx` is not the implemented exchange.
- Multiple workers may consume the same versioned event through separate queues; each queue has its own retry queue, so a retry is never delivered to another worker. The retry queue name carries a topology suffix because durable queue arguments are immutable; the legacy `<queue>.retry` retry queue is no longer written to, and should be drained and deleted after upgrading.

## Implementation status

The following are contracts and target architecture, not evidence of working functionality:

- Parcel creation outbox writer: implemented atomically with parcel and initial history. Relay: not implemented.
- Parcel creation bus publication and assignment writers: implemented through the merged M6 relay. M7 courier status writers remain planned.
- Notification domain-event consumer: implemented for customer-owned `parcel.created.v1` and `parcel.status_changed.v1`; merchant-owned events are dead-lettered as unsupported. The legacy `notification.requested.v1` consumer remains.
- Webhook delivery, signing, and attempt logging: not implemented; the current worker only logs a legacy work item.
- Realtime and merchant event consumers: not implemented.

Do not claim an event is published or delivered merely because a schema, queue name, or database table exists.

## Schema validation

The event schemas are checked by `tests/contracts/event-schemas.test.mjs` using Ajv 2020-12:

```sh
node --test "tests/contracts/*.test.mjs"
```

The test compiles every schema, validates every documented example, and covers negative cases for owner cardinality, producer identity, event type/version, unexpected fields, invalid statuses, and the `FAILED` failure-reason rule. The legacy `notification.requested.v1` schema is compiled and checked with a valid legacy payload for compatibility. CI runs this command in the frontend job.

## Evolution

Additive fields are backward-compatible within a major version. Consumers should ignore unknown envelope and payload fields, tolerate duplicate delivery, and tolerate out-of-order delivery unless a business workflow explicitly requires ordering. Removing fields, changing types, or changing semantics requires a new major version.
