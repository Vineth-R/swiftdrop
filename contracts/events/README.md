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

## Producers

- Publish to the `swiftdrop.events` topic exchange with the versioned event name as the routing key.
- Reuse the event envelope and validate the payload against the versioned JSON schema.
- Prefer asynchronous events when the caller does not need an immediate result and the work can be retried independently.
- Use synchronous HTTP when the caller needs an immediate response, such as validation or a read.

## Consumers

- Declare a durable queue, bind it to the versioned routing key, and set an explicit prefetch limit.
- Acknowledge a message only after processing succeeds.
- Reject a message without requeue when processing fails. Retry and dead-letter policies will be introduced with the business workflows rather than hidden in the initial foundation.
- Treat delivery as at-least-once: deduplicate by `event_id` and make handlers idempotent.
- Ignore unknown fields for additive schema changes. Reject unsupported major versions instead of guessing.
- Do not require RabbitMQ to run ordinary unit tests; test handlers directly and reserve broker tests for integration tests.

## Evolution

Additive fields are backward-compatible within a major version. Removing fields, changing types, or changing semantics requires a new major version. Consumers should tolerate duplicate delivery and out-of-order delivery unless a business workflow explicitly requires ordering.
