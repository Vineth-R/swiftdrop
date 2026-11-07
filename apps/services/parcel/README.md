# Parcel service

Parcel implements customer creation, owned list/detail/history and PDF receipts,
internal merchant creation and summaries, public tracking and delivery reports.
Courier/dispatch writes and the outbox relay remain separate work. See
[M5 verification](../../../docs/m5-complete-parcel-flow.md) for executed evidence
and the older [M5.1 record](../../../docs/m5-parcel-database-verification.md) for
the already merged schema/repository foundation.

## Routes and security

Gin registers these paths. Public calls add `/api/v1` through Nginx; internal
routes require `X-Internal-Token` and are never forwarded by the public gateway.

| Method | Service path                           | Access                            |
| ------ | -------------------------------------- | --------------------------------- |
| POST   | `/parcels`                             | CUSTOMER                          |
| GET    | `/parcels`                             | CUSTOMER, own parcels             |
| GET    | `/parcels/{parcelId}`                  | CUSTOMER owner, DISPATCHER, ADMIN |
| GET    | `/parcels/{parcelId}/history`          | CUSTOMER owner, DISPATCHER, ADMIN |
| GET    | `/parcels/{parcelId}/receipt`          | CUSTOMER owner, ADMIN             |
| POST   | `/internal/parcels`                    | Trusted internal token            |
| GET    | `/internal/parcels/{parcelId}/summary` | Trusted internal token            |

Exact contracts live in [OpenAPI](../../../contracts/openapi/parcel.yaml) and API
v1.1 sections 4/13. List filters: `status`, inclusive UTC `from`/`to`, `page`
(1–100000), `pageSize` (1–100, default 20), `sort` (`createdAt:asc`/`createdAt:desc`).
Another customer's detail/history/receipt returns the same `404 NOT_FOUND` as a
missing record. Ownership is part of the database lookup; authenticated responses
use `Cache-Control: no-store`.

## Transactions, pricing and replay

Creation calls real `/internal/pricing/quote`; there is no production mock.
Customer consent is checked against `expectedFee`. A stale fee returns 409
`PRICE_CHANGED` with numeric `error.currentFee` before writing. Exact decimal LKR
components and total are stored; receipts never reprice using current rules.

Parcel insertion, initial append-only CREATED history and PENDING `parcel.created`
outbox data commit in one pgx transaction. Failure rolls back all three. Existing
M5.1 repository and migrations `000001`–`000005` are reused unchanged.

A customer UUID `Idempotency-Key` is scoped to the authenticated owner. PostgreSQL
transaction advisory locks serialize replay, quote and commit across instances.
Unique constraints remain the final guard. Tracking collisions roll back a
savepoint and retry at most five times. Tracking numbers contain SDK/SDN plus
eight unbiased cryptographic characters from `ABCDEFGHJKLMNPQRSTUVWXYZ23456789`.

Identical replay returns 201 with the original parcel/fee without Pricing.
Different delivery fields under the same key return `IDEMPOTENCY_CONFLICT`.
`expectedFee` is consent metadata excluded from successful-replay comparison.
UUIDs are normalized. `PRICE_CHANGED` writes nothing, so reconfirmation reuses
the same intended request key.

Merchant replay uses `(merchantId, externalOrderId)`. The trusted M8 caller must
send `createdByUserId`, the verified merchant owner's identity user UUID, for
DB v1.1's required audit field. Merchant ID is never used as a user ID. This
completes the previously planned internal contract; no order caller existed.
M8 remains responsible for approval and verifying the supplied owner identity.

Outbox data contains identifiers/status only. M6 must build the canonical envelope
and publish `parcel.created.v1` using the outbox UUID as `event_id`; the stored
label/key remain `parcel.created`. A successful create does not claim email
delivery. See the [event contract](../../../contracts/events/README.md).

## Configuration and screens

| Variable            | Default                | Purpose                                |
| ------------------- | ---------------------- | -------------------------------------- |
| `INTERNAL_TOKEN`    | unset, fail closed     | Internal and downstream authentication |
| `PRICING_BASE_URL`  | `http://pricing:8083`  | Real Pricing service origin            |
| `PRICING_TIMEOUT`   | `3s`                   | Pricing HTTP deadline                  |
| `IDENTITY_BASE_URL` | `http://identity:8081` | Assigned courier contact               |
| `IDENTITY_TIMEOUT`  | `3s`                   | Identity HTTP deadline                 |

Origins accept HTTP(S), without credentials, paths, queries or fragments. Clients
reject redirects and propagate request IDs/cancellation. History and receipts use
authorized stored data independently of courier-contact availability. Existing
shared database/auth/startup validation continues to apply.

Customer screens `/new`, `/parcels`, `/parcels/{id}` reuse the authenticated shell.
The shared `/parcels` route selects by role; the dispatcher workspace remains
explicitly planned. The wizard preserves keys on retry and data after errors,
requires fee reconfirmation, and prevents edits to an uncertain submission from
creating duplicates.

## Local verification

Use `.env.example`, then the documented `make up-build`/`make up` workflow. The
latter starts images, migrates and verifies gateway routes. Never reset volumes
for M5 checks. Compose service/database ports are internal by default.

```sh
pnpm exec nx test parcel
pnpm exec nx run-many -t lint test typecheck --projects=@org/web,@org/web-e2e
pnpm exec nx build @org/web
```

For PostgreSQL 16 tests set `PARCEL_TEST_DATABASE_URL` and `TEST_DATABASE_URL` to a
disposable maintenance server with CREATEDB and the pinned migrate CLI available:

```sh
go test -race -count=1 -tags=integration,postgres -covermode=atomic -coverpkg=./apps/services/parcel/... -coverprofile=test-output/m5-parcel-coverage.out ./apps/services/parcel/...
node scripts/check-m5-coverage.mjs test-output/m5-parcel-coverage.out
```

Fixtures create/migrate/drop only generated databases. Quote dotted Go flags in
PowerShell. The M5 CI workflow runs all tagged workflows and enforces 60% statement
coverage for the documented M5 slice. See
[real journeys](../../../tests/integration/parcelflow/README.md) for the opt-in
API/browser checks; mocked screen tests are reported separately.
