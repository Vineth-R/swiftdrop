# Pricing and public tracking backend

These APIs belong to the M4 backend slice. No tracking UI, pricing administration UI, customer delivery wizard or submission report is included in this change. Identity login, courier writes, outbox publication and realtime subscriptions remain separate owners' work.

## Local prerequisites

Use the root `AGENTS.md` and installed `.agents/skills/*/SKILL.md` guidance. JavaScript tasks use the pinned pnpm through Nx. Go uses the single root module. Start Compose, then run `bash scripts/migrate.sh` (or `make migrate` where GNU Make is installed). See [migration instructions](../migrations/README.md) for canonical/legacy Merchant recovery.

Only Nginx is the public API entry. Its default URL is `http://localhost:8080`; use the local `NGINX_PORT` value if another application occupies that port. No internal service ports need to be published.

Pricing reads `DATABASE_URL`, `INTERNAL_TOKEN`, `JWT_ACCESS_SECRET`, `COOKIE_NAME` and `HTTP_TRUSTED_PROXIES` in addition to the existing service configuration. Parcel reads `DATABASE_URL` and `HTTP_TRUSTED_PROXIES`. Services read process environment; Compose loads `.env`. Never commit `.env`, real tokens or database credentials.

`HTTP_TRUSTED_PROXIES` is an optional comma-separated CIDR allowlist. With no setting, forwarded headers are ignored. The local Compose configuration explicitly trusts its private bridge ranges; narrow this to the gateway network in other deployments. Nginx overwrites `X-Real-IP` and `X-Forwarded-For` with the observed client address. Pricing and Parcel trust `X-Real-IP` only from configured proxies. Do not expose app ports or put an untrusted proxy inside that allowlist.

## Public pricing

- `POST /api/v1/pricing/estimate`
- `GET /api/v1/zones`

Example request:

```json
{
  "originZoneId": "<active Colombo UUID>",
  "destinationZoneId": "<active Kandy UUID>",
  "weightKg": 2,
  "sizeCategory": "SMALL"
}
```

Resolve actual IDs from `GET /api/v1/zones`. The seed yields base fee 450.00 + weight charge 100.00 + surcharge 0.00 = **550.00 LKR**. Quotes return the input fields, currency, all three components and total under `data`; `meta` contains request ID and UTC timestamp. Zones return only active records and `id`, `code`, `name`, `districts`.

Amounts and weights use exact hundredths; quoted numeric strings, fractions beyond two effective decimal places, overflow, null and nonpositive weight are rejected. Scientific JSON numbers are accepted when exactly representable in hundredths. No binary floating-point arithmetic is used for money. The quote reads its configuration in one PostgreSQL statement/snapshot.

Weight bands use **lower exclusive, upper inclusive** bounds: `(0,1]`, `(1,5]`, `(5,10]`, `(10,25]` in the baseline. At 1 kg the weight charge is 0.00; at 5 kg it is 100.00; at 10 kg it is 250.00. The baseline size categories are SMALL, MEDIUM, LARGE and OVERSIZED.

Errors: malformed JSON is 400 `BAD_REQUEST`; invalid types/weight/size/IDs are 422 `VALIDATION_ERROR`; unknown/inactive zones are 404 `ZONE_NOT_FOUND`; no active route is 422 `ROUTE_NOT_SERVICEABLE`; a gap between active bands is 422 `WEIGHT_NOT_PRICED`. Above the greatest active upper bound is 422 `VALIDATION_ERROR`, matching API v1.1 §3.1. An unconfigured database returns 503; unexpected failures return a safe 500 with no SQL or secrets. Bodies are limited to 1 MiB (413).

Public estimates allow **300 requests per rolling minute per client IP**, the explicit anonymous policy used for this implementation. Tracking uses its separate required 30/minute limit. Limiters are bounded and process-local; a multi-replica deployment needs coordinated enforcement at the edge. Successful responses include `X-RateLimit-Remaining`; 429 `RATE_LIMITED` includes `Retry-After`. A full client table fails closed until expired entries can be reclaimed.

## Internal pricing handoff

Inside the Compose network:

- `POST http://pricing:8083/internal/pricing/quote`: same request and quote calculation as the public estimate.
- `GET http://pricing:8083/internal/zones/{zoneId}`: returns `id`, `code`, `name`, `isActive`, including inactive zones for authoritative validation.

Both require `X-Internal-Token` matching configured `INTERNAL_TOKEN`. An absent configuration never disables authentication. The public gateway returns 404 for `/internal`; it does not forward these calls. Parcel creation's internal client, timeout/503 handling, `expectedFee` comparison and stored fee are M5's responsibility. No Parcel creation endpoint is invented here.

## Pricing management

All `/api/v1/admin/...` pricing routes require an ADMIN access token through the `sd_access` cookie (configurable with `COOKIE_NAME`) or `Authorization: Bearer ...`. The validator uses `golang-jwt/jwt/v5`, restricts signatures to HS256, requires expiry, issued-at, subject and token ID, and checks the role. It does not issue tokens or implement login. M2 must issue tokens with the documented shared secret/claims. Use a strong environment-managed signing key. Missing authentication configuration fails closed with 503.

| Resource   | Methods and paths relative to `/api/v1`                                         |
| ---------- | ------------------------------------------------------------------------------- |
| Zones      | POST `/admin/zones`; PATCH `/admin/zones/{id}`                                  |
| Rules      | GET/POST `/admin/pricing/rules`; PATCH `/admin/pricing/rules/{id}`              |
| Bands      | GET/POST `/admin/pricing/bands`; PATCH `/admin/pricing/bands/{id}`              |
| Surcharges | GET `/admin/pricing/surcharges`; PUT `/admin/pricing/surcharges/{sizeCategory}` |

Lists accept `page` (1–10000), `pageSize` (1–100), and allowed `sort` values documented in OpenAPI. Rules also filter by origin/destination zone and active state. Updates reject empty bodies, unknown fields and null changes. Conflicts map to `ZONE_CODE_EXISTS`, `RULE_EXISTS` or `BAND_OVERLAP` (409), including database-enforced concurrent overlap/uniqueness failures.

Configuration changes and their audit entry commit in one transaction. Rules record the updating actor. Failed audit writes roll back the configuration change. Existing Parcel fee snapshots are never read or modified by Pricing.

## Public tracking

`GET /api/v1/tracking/{trackingNumber}` is anonymous and owned by the Parcel service. SDK/SDN plus eight uppercase alphanumeric characters are accepted. A missing valid number returns 404 `TRACKING_NOT_FOUND`; malformed numbers return 422. All six lifecycle statuses can be read; this API does not change status.

The one-query public projection contains only:

```json
{
  "trackingNumber": "SDK7M2Q9XP4",
  "status": "IN_TRANSIT",
  "pickupCity": "Colombo",
  "dropoffCity": "Kandy",
  "lastUpdatedAt": "2026-11-01T01:00:00Z",
  "timeline": [{ "status": "CREATED", "at": "2026-11-01T00:00:00Z" }]
}
```

The result uses actual `parcel.parcels` and `parcel.parcel_status_history` rows. History is ordered by timestamp then ID for deterministic ties. No names, street addresses, phones, internal notes, failure details or proof files are selected. The existing unique tracking-number index is reused. There is no sample-data fallback and no realtime push claim.

## Verification commands

Inspect resolved Nx targets before running them:

```text
pnpm exec nx show project pricing --json
pnpm exec nx show project parcel --json
pnpm exec nx show project go-common --json
pnpm exec nx run-many -t test --projects=pricing,parcel,go-common,merchant --parallel=1
pnpm exec nx run pricing:test --args="-coverprofile=test-output/pricing-coverage.out"
go tool cover -func=test-output/pricing-coverage.out
bash scripts/test-migrations.sh
BASE_URL=http://localhost:8080 bash scripts/check-routes.sh
TEST_BASE_URL=http://localhost:8080 go test -tags=integration,backendintegration -run TestPricingTrackingBackend ./tests/integration/...
```

The migration scenarios use uniquely named disposable databases and preserve existing legacy rows. They do not reset the application database. Unit tests do not require PostgreSQL or RabbitMQ. Backend HTTP integration tests use the explicit `integration,backendintegration` build tags and require a running gateway/Compose stack; commands and fixtures are in the test source. Record actual output and limitations. Command listings are not claims that checks passed.

The JWT dependency is necessary to validate admin tokens without implementing a custom JWT parser/signature verifier. It adds no frontend dependencies or styling changes. The relevant contracts are [Pricing OpenAPI](../contracts/openapi/pricing.yaml) and [Parcel OpenAPI](../contracts/openapi/parcel.yaml).

Dependency review: the pinned JWT v5.3.1 is newer than the v5.2.2 fix for the maintainer's [header-parsing allocation advisory](https://github.com/golang-jwt/jwt/security/advisories/GHSA-mh63-6h87-95cp). The middleware rejects every parse/signature/claims error and restricts the signing algorithm. This targeted advisory review is not a whole-application vulnerability scan.
