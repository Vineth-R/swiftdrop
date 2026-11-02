# Event contracts

SwiftDrop uses RabbitMQ for asynchronous communication. Events are JSON messages published to the durable topic exchange `swiftdrop.events`.

## Naming and versioning

Event names use `<domain>.<past_tense_action>.v<major>`:

- `parcel.created.v1`
- `parcel.status_changed.v1`
- `notification.requested.v1`
- `webhook.delivery.requested.v1`

The major version is part of both the event type and its routing key. Breaking changes require a new major version and a new queue binding. A producer must not silently change the meaning of an existing versioned event.

Each event file follows `<event_name>.schema.json` and uses JSON Schema draft 2020-12.

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
- Retry and dead-letter resources are derived from the main exchange and queue: `<exchange>.retry`, `<queue>.retry`, `<exchange>.dlx`, and `<queue>.dlq`. Retry messages return to the original exchange and routing key after the delay.
- A worker routing key must not fan out to multiple worker queues; retry re-entry routes through the original routing key.
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

An example schema exists at `contracts/events/notification.requested.v1.schema.json`. It is the documented shape for that event, but the current notification worker does not validate `data` against it yet; it validates the envelope and the event type/version only. Payload validation is required before a worker acts on payload fields.

Per-event schemas may be closed for their specific payload, as the notification example is. Additive changes are still compatible for consumers that ignore unknown fields; update the schema within the same major version when adding optional fields. Adding a required field, removing a field, changing a type, or changing semantics is a breaking change that requires a new major version.

### Webhook payload schema: open decision

No consumer currently validates a webhook payload schema because none is defined. The webhook worker consumes `webhook.delivery.requested.v1` and validates only the envelope, event type, and version. It does not interpret destination URLs, signing details, retry configuration, or any other webhook payload field.

Choosing the webhook payload shape and whether it needs its own JSON Schema is an open contract decision. Do not assume or invent webhook fields until a schema is added under `contracts/events/`.

## Evolution

Additive fields are backward-compatible within a major version. Consumers should ignore unknown envelope and payload fields, tolerate duplicate delivery, and tolerate out-of-order delivery unless a business workflow explicitly requires ordering. Removing fields, changing types, or changing semantics requires a new major version.
