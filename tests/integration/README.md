# Integration tests

## Pricing and tracking backend

The backend HTTP fixture suite uses both `integration` and `backendintegration` tags, so it is separate from the broker-only CI job. Start a disposable local Compose stack, configure `JWT_ACCESS_SECRET`, apply migrations, and run:

```sh
TEST_BASE_URL=http://localhost:8080 go test -tags=integration,backendintegration -run TestPricingTrackingBackend -count=1 ./tests/integration/...
```

Use your configured gateway port. This suite requires a loopback HTTP URL and local Docker. It creates UUID-scoped fixtures and deletes only those fixtures; it does not reset the application database. It assumes baseline pricing bands and surcharges and should run on a disposable stack. It covers quote boundaries and sizes, admin authentication/audits, configuration conflicts, stored fee preservation, all six tracking states, privacy, and gateway rate enforcement. Lifecycle transitions themselves belong to the Parcel owner's write APIs.

This directory contains tests that require running infrastructure. Broker-backed RabbitMQ tests are guarded by the `integration` build tag, so `go test ./...` never requires a broker.

## Prerequisites

Start RabbitMQ and wait for it to become healthy:

```sh
docker compose up -d rabbitmq
docker compose ps rabbitmq
```

Use the connection details from `.env.example` unless you changed them. From the host, the default URL is:

```text
amqp://swiftdrop:swiftdrop@localhost:5672/
```

## Run the tests

```sh
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -race -count=1 -tags=integration -v ./tests/integration/...
```

Tests call `t.Skip` when `RABBITMQ_URL` is unset. A skip is expected for local runs that do not need a broker, but CI treats any skip as a failure. Run from the repository root so the root Go module resolves.

## Messaging scenarios

`messaging_test.go` currently exercises five scenarios:

| Scenario                   | Expected behavior                                                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Successful acknowledgement | A handler succeeds, the message is acknowledged, and the main queue drains.                                                                |
| Retry then success         | A retryable handler failure is republished through the retry queue; the next attempt succeeds and no dead letter is produced.              |
| Permanent failure          | A `messaging.Permanent` failure goes directly to the dead-letter queue with the failure reason and original message metadata preserved.    |
| Retry exhaustion           | A handler that keeps failing is attempted up to the configured retry limit, then dead-lettered with `retry_exhausted` and the retry count. |
| Panic recovery             | A handler panic is recovered, treated as retryable, retried, and can succeed without producing a dead letter.                              |

Reconnect behavior is not exercised here. Forcing broker disconnects against shared infrastructure would be disruptive; bounded backoff, jitter, backoff reset, and context-cancelled waits and dials are covered by deterministic unit tests in `packages/go-common/messaging`.

## Isolated topology and cleanup

Each test builds a unique prefix from `swiftdrop.test.<UnixNano>` and declares:

- a main topic exchange, queue, and routing key
- a retry exchange and retry queue
- a dead-letter exchange and dead-letter queue

Cleanup runs through `t.Cleanup`: it deletes the queues, then the exchanges, and closes the channel and connection. Tests do not reuse the production `swiftdrop.events` exchange or worker queues.

## CI behavior

The `integration` job in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) starts a RabbitMQ service container, sets `RABBITMQ_URL`, waits for port 5672, and runs:

```sh
go test -race -count=1 -tags=integration -v ./tests/integration/...
```

The job fails if the broker is unreachable or if any integration test is skipped. This documentation does not assert that CI passed for a given revision; check the Actions run.

## Guidelines for new tests

- Keep infrastructure-backed tests behind `//go:build integration`.
- Read connection details from environment variables such as `RABBITMQ_URL`. Do not hard-code credentials.
- Use unique names per run and remove created queues, exchanges, and other resources during cleanup.
- Do not make unit tests depend on this directory or on a live broker.
- Do not require a real external notification or webhook provider.
- Document any additional container or setup requirement here and in the test file.
