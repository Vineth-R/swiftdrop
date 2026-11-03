# Integration tests

This directory contains the M7.1 PostgreSQL verification script, the isolated PostgreSQL dispatch tests in `database/`, and broker-backed RabbitMQ tests. Each suite has separate prerequisites and coverage. The optional Go proof-photo validator tests live in the parcel service and require neither PostgreSQL nor RabbitMQ.

## M7.1 database verification

[`m7_database.sql`](m7_database.sql) verifies the existing database constraints against [API v1.1](../../docs/API_Documentation_v1.1.md), sections 6.3–6.4, and [Database Design v1.1](../../docs/Database_Design_Document_v1.1.md), sections 5.1, 5.4, 7.1 and 7.2.

All three M7.1 tables already exist in the immutable platform baseline migrations:

| Table                            | Existing UP migration                                                                     |
| -------------------------------- | ----------------------------------------------------------------------------------------- |
| `parcel.delivery_proofs`         | [`parcel/000002_tables.up.sql`](../../migrations/parcel/000002_tables.up.sql)             |
| `notification.notification_logs` | [`notification/000002_tables.up.sql`](../../migrations/notification/000002_tables.up.sql) |
| `notification.processed_events`  | [`notification/000002_tables.up.sql`](../../migrations/notification/000002_tables.up.sql) |

No duplicate `000003` migrations were created for M7.1. The applied `000001` and `000002` migrations were preserved. See the [migration guide](../../migrations/README.md) for versioning and rollback behavior.

### Coverage

| Area                  | Verified behavior                                                                                                                                                                                                                                                                          |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Columns and keys      | Required columns, PostgreSQL types, character limits, nullability, primary keys, the proof-to-parcel foreign key, and the `(parcel_id, created_at)` proof index.                                                                                                                           |
| Delivery proofs       | Required fields and defaults; recipient-only, note-only and photo proofs; multiple proofs per parcel; JPEG/PNG MIME metadata; sizes from 1 through 5,242,880 bytes inclusive; missing photo metadata, invalid MIME types, nonpositive/oversized sizes and all-NULL proof content rejected. |
| Notification logs     | Required fields, defaults, valid statuses, channel/status checks, primary-key rejection and unique `(event_id, channel, recipient)` deduplication. Distinct channels, recipients or events remain permitted.                                                                               |
| Processed events      | Required fields and timestamp default; composite primary key `(consumer, event_id)` rejects duplicates for the same consumer while allowing another consumer to process that event.                                                                                                        |
| Parcel outcome fields | `FAILED` requires `failed_at` and a 5–255-character `failure_reason`; `DELIVERED` requires `delivered_at` and `delivered_to_name`, whose maximum length is 150. Tests use valid lifecycle transitions with triggers enabled.                                                               |

The SQL tests validate photo metadata, not image bytes. The baseline's empty-proof rejection means `recipient_name`, `note` and `photo_path` are all NULL; it does not establish whitespace validation. File signatures, image decoding, generated storage names, assignment authorization and HTTP upload behavior are separate application responsibilities.

### Run on Windows

Run from the repository root with Windows PowerShell 5.1 or later and Docker Desktop running Linux containers. These existing images must already be cached locally:

- `postgres:16-alpine`
- `migrate/migrate:v4.18.3`, matching the repository's pinned migration image

```powershell
powershell -NoProfile -File .\scripts\test-m7-database.ps1
```

The runner uses `--pull never`; a missing cached image fails preflight. It invokes the migration image directly, bypassing the Makefile and Unix migration wrapper that failed in the verified Windows environment. It needs no host PostgreSQL, `psql` or golang-migrate installation. `psql` runs inside the disposable PostgreSQL container with `-X` and `ON_ERROR_STOP=1`. Exit code 0 means verification and cleanup succeeded; preflight, migration, assertion or cleanup failures produce a nonzero exit code. An optional `-StartupTimeoutSeconds` parameter changes the default 60-second startup timeout.

### Isolation and safety

- Each invocation creates GUID-based container/network names and a database named `swiftdrop_m7_test_<runId>` on a unique internal bridge network, with no published ports.
- PostgreSQL data uses a 256 MiB `tmpfs`; no existing project database volumes, named volumes or anonymous PGDATA volume are attached. SQL/bootstrap files and migration directories are mounted read-only.
- The runner does not use the active Compose project, `.env`, `DATABASE_URL` or project credentials. Password-free trust authentication is confined to its private test network. Connection URLs are redacted from diagnostic output.
- The existing bootstrap creates the schemas and `public.set_updated_at()`. Only the parcel and notification baseline migrations are applied by this runner; other service business tables are not required.
- The SQL suite requires the disposable database-name prefix and wraps its fixtures and helpers in a transaction that ends with `ROLLBACK`. Expected constraint failures are checked by SQLSTATE and, where applicable, constraint/column name. No application triggers are disabled.
- Cleanup runs in `finally` and targets only containers and networks with this invocation's exact random label. Cleanup is attempted on failures; a cleanup failure cannot be reported as success. A forced process termination or unavailable Docker daemon can prevent cleanup, so inspect the run-specific resources after an interruption. Never use a global prune, volume deletion, `make db-reset` or project rollback to clean up these tests.

Run destructive migration tests only through this disposable runner. A database-name prefix alone is not sufficient isolation for a manually configured connection; do not point the SQL file at the active SwiftDrop database.

### Migration lifecycle and recorded results

The local November 1, 2026 verification completed this sequence:

| Stage                                            | Result                                                                                                                                                                                                    |
| ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Bootstrap and `up 2` for parcel and notification | PASS — both schemas at clean version 2; all seven baseline tables present.                                                                                                                                |
| SQL suite before rollback                        | PASS — **95/95 assertions**.                                                                                                                                                                              |
| `down 1` for notification, then parcel           | PASS — version `000002` rolled back; both schemas remain at clean version 1 and all seven baseline tables are absent.                                                                                     |
| `up 1` for parcel, then notification             | PASS — version `000002` reapplied; both schemas at clean version 2 and all seven tables restored.                                                                                                         |
| SQL suite after reapplication                    | PASS — **95/95 assertions** again.                                                                                                                                                                        |
| Migration-state checks                           | PASS — **33 checks**: 11 at each of initial version 2, rolled-back version 1 and reapplied version 2. Each snapshot checks two schemas, two clean migration versions and seven table-presence conditions. |

The seven tables are `parcel.parcels`, `parcel.parcel_status_history`, `parcel.parcel_assignments`, `parcel.delivery_proofs`, `parcel.outbox_events`, `notification.notification_logs` and `notification.processed_events`. Version `000001` is never rolled back by this runner. The M7 runner intentionally stops at baseline version `000002`; the separate PostgreSQL dispatch suite below covers M6's `000003_dispatch_foundations` migration. The 95 assertions are the same suite run twice, not 190 distinct assertions; the 33 migration-state checks are separate.

Normal cleanup and a deliberate SQL-failure probe were verified: the failure propagated a nonzero exit code and disposable resources were removed. Before/after observations showed the existing 12 SwiftDrop containers remained healthy with unchanged identities/start/restart state and the existing PostgreSQL/RabbitMQ volumes remained unchanged. The active database was not accessed by the runner. These are recorded local results, not a claim that CI or a new run passed when this document was updated.

## PostgreSQL tests

Dispatch database tests live in `tests/integration/database` behind the `integration` build tag. They verify the parcel assignment and outbox migrations, constraints, indexes, triggers, upgrade paths, and rollback/reapply behavior.

The tests create a uniquely named database on the server named by `TEST_DATABASE_URL`, run the migrations there, and drop it during cleanup. The URL must allow `CREATE DATABASE`. Never point it at a shared application database; the tests do not read or modify application data outside their isolated databases.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  go test -race -count=1 -tags=integration -v ./tests/integration/database/...
```

Or:

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  make test-db
```

Tests skip when `TEST_DATABASE_URL` is unset. CI starts a PostgreSQL 16 service container, sets the variable, and fails the job if any database test is skipped.

## M4 pricing and public tracking

`tests/integration/database/pricing_migrations_test.go` applies `pricing/000001` and `pricing/000002` to a uniquely named disposable database, then verifies the seed, `(min, max]` boundaries, foreign keys, unique/check/exclusion constraints, repeated execution, and down/reapply behavior.

`apps/services/pricing/internal/http/pricing_integration_test.go` uses the same isolated-database convention and exercises the real HTTP router and PostgreSQL-backed repository: the Colombo-to-Kandy 2 kg SMALL 550.00 LKR quote, every seeded weight boundary, above-maximum and gap `WEIGHT_NOT_PRICED`, unpriced-route precedence, unknown zones, invalid input, active/inactive/empty zones, and a real closed-connection server error.

`tests/integration/pricing_tracking_test.go` (tags `integration,backendintegration`) is a manual gateway suite. It requires a running disposable Compose stack with `JWT_ACCESS_SECRET`; it uses shared-auth admin tokens and UUID-scoped fixtures. Its cleanup disables trigger firing only for its own cleanup session because `parcel.parcel_status_history` is append-only.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  go test -race -count=1 -tags=integration \
  ./tests/integration/database/... \
  ./apps/services/pricing/internal/http/...

TEST_BASE_URL=http://localhost:8080 \
  go test -tags=integration,backendintegration -run TestPricingTrackingBackend ./tests/integration/...
```

The CI database job runs the migration, pricing HTTP, processed-event cleanup, notification-log repository and demo-seed integration tests with the PostgreSQL service and fails on skips. The manual gateway suite is not part of CI.

## Notification admin log

`apps/services/notification-worker/internal/notificationlog/postgres_integration_test.go` (tag `integration`) creates a uniquely named disposable database, applies `infra/docker/postgres/init/01-schemas.sql` plus notification `000001` and `000002`, and exercises the real repository with the actual schema. It verifies newest-first ordering, status filtering, pagination/offset, nullable fields and empty-table results. Cleanup drops only the database created by that test run.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  go test -race -count=1 -tags=integration -v \
  ./apps/services/notification-worker/internal/notificationlog/...
```

## Processed-event retention

`apps/services/notification-worker/internal/idempotency/processed_events_integration_test.go` (tag `integration`) creates a uniquely named disposable database, applies `infra/docker/postgres/init/01-schemas.sql` plus notification `000001` and `000002`, and verifies strict cutoff deletion, unchanged records at and after the cutoff, notification-log preservation, empty-table and repeated cleanup, stable database errors, context cancellation, concurrent cleanup, and retained-event idempotency. Cleanup drops only the database created by that test run.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  go test -race -count=1 -tags=integration -v \
  ./apps/services/notification-worker/internal/idempotency/...
```

## Demo-user seed

`apps/services/identity/internal/seed/postgres_integration_test.go` (tag `integration`) creates a uniquely named disposable database, applies the identity and pricing migrations, and runs the real seed orchestration. It verifies all five demo users and roles, the two courier profiles and their `CMB`/`KDY` zones, bcrypt password hashes, rerun idempotency, unchanged existing credentials, roles and profiles even when a rerun supplies different values, different-role skips, zone failures without partial users, forced-profile rollback, and concurrent executions. Cleanup drops only the database created by that test run.

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/postgres?sslmode=disable' \
  go test -race -count=1 -tags=integration -v \
  ./apps/services/identity/internal/seed/...
```

## Optional Go proof-photo validator verification

The service-owned [validator](../../apps/services/parcel/internal/proof/photo.go) and [unit tests](../../apps/services/parcel/internal/proof/photo_test.go) are separate from M7.1 database verification. They use only the Go standard library and do not implement the HTTP upload endpoint.

`ValidatePhoto(data, declaredMIME)` returns the canonical MIME type and actual byte count on success, or an identifiable validation error with zero metadata on failure. A supplied photo must be nonempty and at most 5,242,880 bytes. Missing/generic MIME declarations are rejected; valid MIME parameters and case/whitespace normalization are accepted. The declared JPEG/PNG type must match detected content, and dimensions are inspected before full decoding. Callers must bound upload reads before allocating the byte slice, persist the same validated bytes, and skip the validator when the optional photo is omitted.

For a proof-package-only run from the repository root, avoiding the memory-heavy parcel-wide build:

```powershell
$env:GOMAXPROCS = '2'
$env:GOMEMLIMIT = '256MiB'
go test -p 1 -count=1 -v -cover ./apps/services/parcel/internal/proof
```

Recorded local verification:

- **59 unit cases passed**, covering JPEG/PNG acceptance, empty/oversized input, declared MIME parsing and matching, unsupported formats, forged signatures, malformed/truncated content, byte boundaries and dimensions. Proof-package statement coverage was **100%**; scoped `go vet` and `go build` also passed.
- **19 additional manual cases passed** using 18 actual files on E:. Windows' image encoder produced genuine JPEG/PNG fixtures; legal JPEG comment segments produced exact sizes of 5,242,880 and 5,242,881 bytes. Both padded JPEGs passed full decoding; the validator accepted the former and rejected the latter. Other cases rejected an actual PDF renamed `.jpg`, a corrupted JPEG retaining a valid header, and a JPEG declared as PNG.
- The manual checks used fully encoded JPEG and PNG images: 8,192 pixels on either side were accepted and 8,193 rejected; 4,000×4,000 pixels were accepted and 4,000×4,001 rejected. All valid fixtures passed independent full decoding, including those rejected for byte or dimension limits. The inclusive 8,192-per-side and 16,000,000-total-pixel limits are implementation safeguards, not API v1.1 requirements.
- The combined run passed **78 cases** with 100% statement coverage. The temporary `photo_manual_test.go`, all fixtures and Go scratch files were removed, and checksums confirmed the retained validator files were unchanged. The command above reruns the retained 59 unit cases; reproducing the 19 manual cases requires recreating the temporary harness and fixtures.

Broader verification remains blocked: the full parcel test suite failed during compilation of existing dependencies, including `github.com/ugorji/go/codec`, because Windows exhausted paging memory (`errno=1455`). The Nx workflow was unavailable because pnpm/Node tool bootstrap exhausted memory. Neither the full parcel suite nor the Nx suites are recorded as passing. No shared infrastructure or system memory settings were changed to work around these limitations.

## RabbitMQ foundation tests

Broker-backed tests are guarded by the `integration` build tag, so `go test ./...` does not require a broker. The scenarios below exercise shared messaging mechanics, not complete API v1.1 business-event integration.

### API contract gaps and follow-up ownership

The API v1.1 section 11 audit found mismatches and missing implementations. **RabbitMQ integration compliance is not established by the M7 database or validator results, or by the generic broker scenarios below.** Follow-up ownership is recorded from the team task sheet; this documentation does not create tickets or imply the owners have completed the work.

| Follow-up                                                                                                                                                                                                           | Responsible owners                                                                            |
| ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------- |
| Reconcile the shared envelope/version conventions and topology: API `swiftdrop.dlx` differs from the implemented `swiftdrop.events.dlx`; API queue/routing names differ from current versioned worker contracts.    | M1 for shared infrastructure/contracts, coordinating with the producing and consuming owners. |
| Align notification queue/bindings and implement delivery/idempotent processing. API `notification.queue` consumes parcel events; the foundation uses `notification.requested.v1` and currently validates/logs only. | M1 notification-worker ownership, coordinating notification requirements with M7.             |
| Implement parcel event producers and outbox relay; the outbox table exists, but runtime publication is absent.                                                                                                      | M5 for parcel-created publication, M7 for parcel-status publication, M6 for outbox relay.     |
| Align `webhook.queue` versus `webhook.delivery.requested.v1`, implement webhook delivery and the documented retry schedule. The current shared default is three retries at a fixed 30-second delay.                 | M9 for webhook behavior; M1 for shared retry/topology support.                                |
| Implement the documented `realtime.queue` and `merchant.sync.queue` consumers.                                                                                                                                      | M10 for realtime and M8 for merchant integration, coordinating topology with M1.              |

API v1.1 section 10.3 requires six webhook attempts with the schedule exactly: **now, 1 minute, 5 minutes, 30 minutes, 2 hours, 6 hours**. The later delays are after the previous failure. After the sixth failure, dead-lettering and delivery-log recording are required. The fixed-delay foundation does not implement that schedule. See the [current repository event contracts](../../contracts/events/README.md) for the implemented conventions; that document is not evidence of v1.1 alignment.

### Prerequisites

Start RabbitMQ and wait for it to become healthy:

```sh
docker compose up -d rabbitmq
docker compose ps rabbitmq
```

Use the connection details from `.env.example` unless you changed them. From the host, the default URL is:

```text
amqp://swiftdrop:swiftdrop@localhost:5672/
```

### Run the tests

```sh
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -race -count=1 -tags=integration -v ./tests/integration
```

Tests call `t.Skip` when `RABBITMQ_URL` is unset. A skip is expected for local runs that do not need a broker, but CI treats any skip as a failure. Run from the repository root so the root Go module resolves.

### Messaging scenarios

`messaging_test.go` currently exercises eight scenarios:

| Scenario                   | Expected behavior                                                                                                                          |
| -------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ |
| Successful acknowledgement | A handler succeeds, the message is acknowledged, and the main queue drains.                                                                |
| Retry then success         | A retryable handler failure is republished through the retry queue; the next attempt succeeds and no dead letter is produced.              |
| Permanent failure          | A `messaging.Permanent` failure goes directly to the dead-letter queue with the failure reason and original message metadata preserved.    |
| Retry exhaustion           | A handler that keeps failing is attempted up to the configured retry limit, then dead-lettered with `retry_exhausted` and the retry count. |
| Panic recovery             | A handler panic is recovered, treated as retryable, retried, and can succeed without producing a dead letter.                              |
| Shutdown requeue           | A message in flight when the consumer stops is requeued and processed by the next consumer.                                                |
| Oversized message          | A body above `messaging.MaxMessageBytes` is dead-lettered as permanent without invoking the handler.                                       |
| Unsupported content type   | A non-JSON content type is dead-lettered as permanent without invoking the handler.                                                        |

Reconnect behavior is not exercised here. Forcing broker disconnects against shared infrastructure would be disruptive; bounded backoff, jitter, backoff reset, context-cancelled waits and dials, and AMQP URL redaction are covered by deterministic unit tests in `packages/go-common/messaging`.

### Isolated topology and cleanup

Each test builds a unique prefix from `swiftdrop.test.<UnixNano>` and declares:

- a main topic exchange, queue, and routing key
- a retry exchange and retry queue
- a dead-letter exchange and dead-letter queue

Cleanup runs through `t.Cleanup`: it deletes the queues, then the exchanges, and closes the channel and connection. Tests do not reuse the production `swiftdrop.events` exchange or worker queues.

### CI behavior

The `integration` job in [`.github/workflows/ci.yml`](../../.github/workflows/ci.yml) starts a RabbitMQ service container, sets `RABBITMQ_URL`, waits for port 5672, and runs:

```sh
go test -race -count=1 -tags=integration -v ./tests/integration
```

The `database` job starts a PostgreSQL 16 service container, sets `TEST_DATABASE_URL`, and runs the dispatch, pricing, processed-event cleanup, notification-log and demo-seed database tests. Both jobs fail if their dependency is unreachable or if any integration test is skipped. This documentation does not assert that CI passed for a given revision; check the Actions run.

### Guidelines for new tests

- Keep infrastructure-backed tests behind `//go:build integration`.
- Read connection details from environment variables such as `RABBITMQ_URL`. Do not hard-code credentials.
- Use unique names per run and remove created queues, exchanges, and other resources during cleanup.
- Do not make unit tests depend on this directory or on a live broker.
- Do not require a real external notification or webhook provider.
- Document any additional container or setup requirement here and in the test file.
