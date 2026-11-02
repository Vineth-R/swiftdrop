# Parcel database (M5.1)

The Parcel Service already opens the shared `postgres.Pool` when `DATABASE_URL`
is configured. `/ready` checks connectivity; it does not check migration versions.
No business endpoint is implemented by this database work.

## M5.1 acceptance boundary

The acceptance basis is the Member 5 task definition supplied by the team in this
session: PostgreSQL integration, the two Parcel tables, ownership and idempotency
constraints, delivery/failure metadata, reporting indexes, repository, and tests.
No authoritative member task sheet is checked into the repository. DB/API v1.1
specify system behavior, but do not assign that behavior to numbered member tasks.

M5.1 verifies the existing startup connection/readiness and a repository using the
shared pool or a caller-owned transaction. Business creation/status handlers,
authorization, pricing calls, and event dispatch belong to subsequent application
integration. Adding an unused repository to `main.go` would not complete those
workflows and is not necessary for this database deliverable.

## Existing database definition

The owning schema is `parcel`. Its existing migration chain is:

- `migrations/parcel/000001_initial_schema`: creates the schema.
- `migrations/parcel/000002_tables`: M1's baseline, including `parcels`,
  `parcel_status_history`, `parcel_assignments`, `delivery_proofs`, and
  `outbox_events`, their indexes, and their triggers.
- `migrations/parcel/000004_price_breakdown_total`: closes the baseline CHECK's
  JSON-null loophole so the DB v1.1 price sum must evaluate to true. Existing rows
  are validated, never silently repaired; rollback restores the baseline CHECK.

Do not recreate those tables or edit applied migrations. Both M5 table definitions
match `docs/db.sql` and DB v1.1 sections 5.1 and 5.2. The baseline already has:

- exactly one customer or merchant owner;
- partial customer/idempotency and merchant/external-order uniqueness;
- delivery/failure timestamps, accepting recipient name, and failure reason;
- the required reporting indexes;
- the history-to-parcel foreign key and append-only history trigger;
- parcel status-transition enforcement and automatic `updated_at` maintenance.

Migration `000004` adds no fields or tables. Its stricter CHECK implements the
documented total requirement; `docs/db.sql` remains the immutable baseline source.
Version `000003_dispatch_foundations` is owned by M6 and already exists on
`development`; the M5 correction uses `000004` to avoid a collision.
Coordinate subsequent Parcel migrations from the next unused version (`000005`
after combining these branches). If existing rows violate the total requirement, the
migration fails transactionally; owners must agree on remediation before retrying.

User, merchant, courier, and zone IDs are logical references. There are no
cross-service foreign keys. The shared `public.set_updated_at()` bootstrap must
exist before the table migration runs; follow `migrations/README.md`.

## Repository contract

`internal/repository` follows the existing merchant repository style: a concrete
repository, a small pgx-compatible database interface, explicit parameterized SQL,
context propagation, nullable pointer fields, and wrapped errors. PostgreSQL
`NUMERIC` values use the existing dependency's `pgtype.Numeric` so money and weight
do not pass through binary floating point. JSON price components keep their
documented snake_case names.

The repository provides:

- `CreateParcel`: inserts one parcel with the database's `CREATED` status and
  timestamps. The caller supplies its UUID, tracking number, creator, and the
  authoritative price. This method does not calculate fees.
- `ParcelByID`: loads the full internal record.
- `ParcelByCustomerIdempotencyKey`: scopes replay lookup to a customer.
- `ParcelByMerchantExternalOrderID`: scopes lookup to a merchant.
- `AppendStatusHistory`: inserts a history row.
- `StatusHistoryByParcel`: returns history ordered by `created_at`, then `id` for
  deterministic ties. An empty result is an initialized empty slice.

These are persistence operations, not public response models or authorization.
The application layer must authorize reads and writes and select safe fields for public tracking.
Expected duplicate/validation failures have stable sentinel errors; context and
unexpected database errors remain inspectable with `errors.Is`/`errors.As`.
Handlers must never return raw database errors to clients.

### Shared transactions with M6/M7

`New` accepts both the existing shared pool and `pgx.Tx`. Reads can use the pool.
The future parcel-creation application layer must:

1. Begin a transaction on the shared pool.
2. Construct `repository.New(tx)`.
3. Call `CreateParcel` and `AppendStatusHistory` for the initial `CREATED` row,
   with `PreviousStatus` left nil.
4. Have M6's outbox implementation insert its record through the **same** `tx`.
5. Commit only after all writes succeed; otherwise roll back.

Repository methods do not start or finish transactions. This also allows later
status updates, history, assignments, and outbox writes to share the service's
transaction. A unique-index failure aborts a PostgreSQL transaction: roll it back
before looking up the existing parcel through the pool or a fresh transaction.
Returning the existing parcel for a duplicate request is a service decision.

M6's outbox/publisher and subsequent business-handler wiring are not changed here.
The baseline outbox stores unversioned names, while `contracts/events/README.md`
and the implemented shared envelope validator require versioned names. The API
v1.1 document also describes a different flat camelCase envelope, whereas the
implemented envelope uses snake_case metadata, a producer, and nested object data.
The integration test uses the existing SQL names only to verify transaction
compatibility; its empty outbox payload is a test fixture, not a publishable event.

This is a pending platform/outbox/consumer integration decision, not a failed
M5.1 table or repository requirement. No Parcel producer or outbox dispatcher is
implemented. Current workers accept `notification.requested.v1` and
`webhook.delivery.requested.v1`, not Parcel event types. Merely appending `.v1` to
database CHECK constraints would not reconcile envelopes or consumer bindings.

The owners can preserve existing storage labels and explicitly map them to the
agreed bus envelope/routing at dispatch. If they instead choose versioned storage,
they must coordinate a forward migration at the next unused Parcel version and a
policy for existing pending rows. M5 must not rewrite M1's applied baseline or
unilaterally change M6's outbox/consumer contracts. Resolve this before publishing.
The creator identity for internal merchant creation and customer same-key /
different-payload behavior also need owner agreement; no speculative columns or
business rules have been added.

## Verification without local Docker

With the workspace's JavaScript tooling installed, ordinary Parcel tests use:

```sh
pnpm exec nx test parcel
```

They require neither PostgreSQL nor RabbitMQ. The equivalent underlying Go scope
is `go test ./apps/services/parcel/...`.

The separate `.github/workflows/m5-parcel-database.yml` workflow provisions
PostgreSQL 16, installs golang-migrate CLI v4.18.3, and runs the Parcel unit and
database tests with the race detector. It can run on a pushed M5 branch, a matching
pull request, or a manual workflow dispatch once the workflow is on GitHub.

For another test machine, supply `PARCEL_TEST_DATABASE_URL` as a PostgreSQL URL to
an administrator/maintenance database. The login needs `CREATEDB` permission and
the `migrate` v4.18.3 CLI must be on `PATH`. The fixture checks the connected
server's major version and requires PostgreSQL 16; another version cannot stand
in for target-version acceptance. Run from the repository root:

```sh
go test -race -count=1 -tags=integration,postgres -run '^TestParcel' ./tests/integration ./tests/integration/parceltest ./apps/services/parcel/internal/repository
go test -race -count=1 -tags=integration ./tests/integration/database/...
```

For the complete dispatch suite in the second command, also set
`TEST_DATABASE_URL` to the same isolated server URL. M6's existing PostgreSQL
tests use that variable; the M5 workflow supplies both variables so those tests
execute rather than skip. Run the dispatch suite without a test-name filter:
its migration inventory and outbox tests do not begin with `TestParcel`.

In PowerShell, quote the tag argument: `'-tags=integration,postgres'`. Race detection
also requires `CGO_ENABLED=1` and a supported C compiler. On Windows, an existing
MSYS2 UCRT64 compiler can be selected for the current process with `CC` and `PATH`;
the PostgreSQL 16 server itself can run from portable binaries on a private port.
Neither choice requires changing the installed PostgreSQL service or Docker data.

The fixture creates a randomly named `swiftdrop_parcel_test_*` database, applies the
real bootstrap and migrations, and drops only its generated database afterwards.
It does not migrate or clear the supplied maintenance database. Opt-in database
tests fail if configuration or the migration CLI is missing; they do not silently
skip. Both `integration` and `postgres` tags are required, so the existing
RabbitMQ-only job does not pick up PostgreSQL tests.

The fixture removes database-name URL query overrides and verifies the connected
database before bootstrapping it. The URL-isolation regression tests are ordinary
unit tests and require no database.

Constraint and migration tests live under `tests/integration`. Repository database
tests live beside the internal package to respect Go's `internal` import rules;
both use the shared fixture in `tests/integration/parceltest`.

Coverage includes migration up/no-change/down/up, the five-service migration chain
and its 26-table inventory, price-total consistency including JSON-null components,
ownership, customer and merchant
uniqueness, delivery/failure metadata, history integrity, reporting indexes,
repository round-trips, exact decimals, duplicate error translation, customer and
merchant lookup isolation, concurrent customer idempotency, and caller-owned
commit/rollback with an outbox record.
Assignment compatibility tests also verify parcel references, current-assignment
uniqueness, and reassignment with the existing courier-copy field.
Authoring these tests does not establish that the PostgreSQL suite has passed;
use actual local test results or an executed workflow result as verification.
