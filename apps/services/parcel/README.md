# Parcel service

Parcel implements customer creation, owned list/detail/history and PDF receipts,
internal merchant creation and summaries, public tracking and delivery reports.
M6 dispatch APIs, screens and the outbox relay are integrated. M7 courier reads,
status updates and proof uploads are implemented here; the remaining courier
workflow UI and end-to-end scenarios remain separate work. See
[M5 verification](../../../docs/m5-complete-parcel-flow.md) for executed evidence
and the older [M5.1 record](../../../docs/m5-parcel-database-verification.md) for
the already merged schema/repository foundation.

## Routes and security

Gin registers these paths. Public calls add `/api/v1` through Nginx; internal
routes require `X-Internal-Token` and are never forwarded by the public gateway.

| Method | Service path                            | Access                            |
| ------ | --------------------------------------- | --------------------------------- |
| POST   | `/parcels`                              | CUSTOMER                          |
| GET    | `/parcels`                              | CUSTOMER, own parcels             |
| GET    | `/parcels/{parcelId}`                   | CUSTOMER owner, DISPATCHER, ADMIN |
| GET    | `/parcels/{parcelId}/history`           | CUSTOMER owner, DISPATCHER, ADMIN |
| GET    | `/parcels/{parcelId}/receipt`           | CUSTOMER owner, ADMIN             |
| POST   | `/internal/parcels`                     | Trusted internal token            |
| GET    | `/internal/parcels/{parcelId}/summary`  | Trusted internal token            |
| GET    | `/courier/deliveries`                   | COURIER                           |
| GET    | `/courier/deliveries/{parcelId}`        | COURIER, current assignment       |
| POST   | `/courier/deliveries/{parcelId}/status` | COURIER, current assignment       |
| POST   | `/courier/deliveries/{parcelId}/proof`  | COURIER, current assignment       |

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
M5.1 repository and migrations `000001`–`000005` are reused unchanged; the
additive `000006_customer_parcel_list_index` adds the composite
`(customer_user_id, created_at, id)` index used by the ordered customer list and
does not rewrite applied history.

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

The internal merchant route is a **trusted-caller boundary**, not an
authentication broker. `POST /internal/parcels` verifies the shared internal
token, the request shape, and stable replay ownership, but it cannot prove that
`merchantId` belongs to `createdByUserId` or that the merchant is APPROVED:
the Merchant service exposes no trusted internal merchant lookup yet. M8's order
caller must authenticate the API key, enforce `MerchantStatusApproved`
(including suspension), and supply the verified owner. Until that integration is
reviewed, do not claim merchant-ownership security is fully enforced by Parcel,
and never route `/internal` through the gateway.

## Couriers, receipts and request safety

When an assigned courier no longer exists, `GET /parcels/{parcelId}` succeeds
with `courier: null`; history and receipts are unaffected. Genuine Identity
outages still return `503`. Customers receive the assigned courier's name and
phone (documented in API v1.1), while only staff receive `courier.id`.

Session cookies are `HttpOnly`, `SameSite=Lax` and optionally `Secure`. All
state-changing parcel routes require `Content-Type: application/json`, which a
cross-site form cannot send without a preflight, so cookie-authenticated writes
are protected without a separate CSRF token. Adding a token is an M2/shared-auth
change and must not be duplicated here.

Receipt font faces and the compressed Latin font are parsed once per process and
shared read-only; each receipt gets its own shaping buffer. A font
initialization failure becomes a generic 500 and is logged server-side.

Outbox data contains identifiers/status only. The merged M6 relay builds the canonical envelope
and publishes `parcel.created.v1` using the outbox UUID as `event_id`; the stored
label/key remain `parcel.created`. A successful create does not claim email
delivery. See the [event contract](../../../contracts/events/README.md).

## Courier delivery proofs

`POST /courier/deliveries/{parcelId}/proof` accepts
`multipart/form-data` with optional `recipientName` (150 characters), `note`
(500 characters) and `photo` (JPEG/PNG, exactly 5 MiB maximum). At least one
field is required. The flow is:

1. The courier group authenticates the access token and requires the COURIER role.
2. The handler bounds the multipart body, rejects duplicate/unknown fields, and
   reads the photo into memory up to the 5 MiB limit.
3. The service validates image content, declared MIME type, dimensions and pixel
   count; client file names are ignored.
4. The service locks the parcel row, verifies `current_courier_id` and the
   current assignment row, and counts existing proofs.
5. The photo is written under a generated name with 0600 permissions in the
   private `PROOF_STORAGE_DIR` (0700), then proof metadata is inserted and
   committed in the same transaction. A post-commit rollback error cannot turn
   a committed proof into a failure.
6. The response returns metadata only; storage keys and filesystem paths never
   appear in responses or the courier read APIs.

Uploads are limited to 30 per minute per verified courier and to
`PROOF_MAX_PER_PARCEL` proofs per parcel (default 20, range 1-100), enforced
under the parcel row lock. Reaching the quota returns `409 PROOF_LIMIT_REACHED`;
rate limiting returns `429 RATE_LIMITED`. The gateway allows 6 MiB so multipart
overhead does not reject a 5 MiB photo, while the application still enforces the
5 MiB photo-content limit and returns `413 PAYLOAD_TOO_LARGE` with the shared
JSON error shape.

Proof files are local to the Parcel service filesystem or its `parcel-proofs`
volume. The implementation assumes one replica or shared persistent storage:
multiple replicas with independent local volumes are unsafe because a proof row
may reference a file that exists only on the replica that accepted the upload.
Horizontal scaling requires shared object storage or an equivalent shared
volume. Back up the volume together with PostgreSQL; a restore needs both the
rows and the files. The service creates the directory 0700 and files 0600 as its
non-root user, and startup fails if the configured directory cannot be opened.

## Configuration and screens

| Variable               | Default                | Purpose                                                                              |
| ---------------------- | ---------------------- | ------------------------------------------------------------------------------------ |
| `INTERNAL_TOKEN`       | unset, fail closed     | Internal and downstream authentication                                               |
| `PRICING_BASE_URL`     | `http://pricing:8083`  | Real Pricing service origin                                                          |
| `PRICING_TIMEOUT`      | `3s`                   | Pricing HTTP deadline                                                                |
| `IDENTITY_BASE_URL`    | `http://identity:8081` | Assigned courier contact                                                             |
| `IDENTITY_TIMEOUT`     | `3s`                   | Identity HTTP deadline                                                               |
| `RABBITMQ_URL`         | unset, relay disabled  | M6 publisher connection; shared safe URL validation                                  |
| `RABBITMQ_EXCHANGE`    | `swiftdrop.events`     | Versioned event exchange used by the relay                                           |
| `PROOF_STORAGE_DIR`    | unset, uploads `503`   | Private delivery-proof directory; Compose uses `/home/app/proofs` on `parcel-proofs` |
| `PROOF_MAX_PER_PARCEL` | `20`                   | Maximum proofs per parcel; allowed range 1-100                                       |

Origins accept HTTP(S), without credentials, paths, queries or fragments. Clients
reject redirects and propagate request IDs/cancellation. History and receipts use
authorized stored data independently of courier-contact availability. Existing
shared database/auth/startup validation continues to apply.

Customer screens `/new`, `/parcels`, `/parcels/{id}` reuse the authenticated shell.
The shared `/parcels` route selects the customer list or M6 dispatcher workspace
by role. The wizard preserves keys on retry and data after errors,
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
