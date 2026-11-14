# Phase 3 — Testing, Documentation and QA Improvements

Status: implementation complete on `feat/phase-3-testing-documentation-qa`; pull
request open for review. Base: `development` at
`686a03dc25538f6353839fbb16dd83c0c56457e5`.

## 1. Scope

Phase 3 addresses the findings in
[`docs/phase-3-initial-audit.md`](phase-3-initial-audit.md). The goal is a
stable CI baseline, accurate contracts and documentation, honest readiness
semantics, stronger cross-service and authorization coverage, safe data
lifecycle controls, and measurable local performance evidence. It does not add
new business capabilities or production infrastructure.

## 2. Findings addressed

| Finding                               | Status                        | Evidence                                                                                                                                |
| ------------------------------------- | ----------------------------- | --------------------------------------------------------------------------------------------------------------------------------------- |
| F-01 flaky merchant fixture           | Fixed                         | All UUID-derived test registration numbers now include a guaranteed digit; regression test loops 2,000 UUIDs                            |
| F-02 Compose webhook rotation env     | Fixed                         | `WEBHOOK_SECRET_KEY_ID` and `WEBHOOK_SECRET_KEYS` are passed to Merchant; Compose config test added                                     |
| F-03 realtime OpenAPI                 | Fixed                         | `/ws` auth, lifecycle, messages, close codes and consumer readiness documented                                                          |
| F-04 README stale claims              | Fixed                         | AI, profiles, realtime readiness, live updates, pricing UI and live-stack statements corrected                                          |
| F-05 architecture stale claims        | Fixed                         | Identity profiles, pricing and parcel lifecycle boundaries corrected                                                                    |
| F-06 CI real-stack path filter        | Fixed                         | Parcel/realtime/notification/webhook/AI service and migration paths added; path-filter tests updated                                    |
| F-07 performance evidence             | Partially addressed           | k6 public/auth/WebSocket scenarios and a local baseline added; merchant/parcel/outbox load still open                                   |
| F-08 worker/outbox readiness          | Fixed                         | Notification, merchant sync, parcel relay and webhook worker readiness now report consumer/relay health                                 |
| F-09 OpenAPI validation               | Fixed                         | OpenAPI parse/ref/operation/critical-route test added to CI; two stale merchant `$ref` defects were found and fixed                     |
| F-10 runtime event schema validation  | Fixed                         | Embedded schema package validates consumer bodies; invalid payloads are permanent failures                                              |
| F-11 access matrix                    | Fixed                         | Expanded from 26 to 40 protected-route cases                                                                                            |
| F-12 admin real-stack UI              | Fixed                         | Real-stack read journeys added for users, pricing and reports                                                                           |
| F-13 AI provider testing              | Partially addressed           | Deterministic tests remain; opt-in `scripts/ai-live-smoke.mjs` added; normal CI stays offline                                           |
| F-14 Playwright flakiness             | Fixed                         | Local E2E now runs the production build; the nine previously failing tests pass together                                                |
| F-15 identity token retention         | Fixed                         | Bounded cleanup for refresh/reset/verification tokens; integration test added                                                           |
| F-16 merchant reservation retention   | Deferred with approval needed | No safe idempotency-window policy exists; deleting rows could allow duplicate orders                                                    |
| F-17 notification recipient retention | Documented                    | Raw recipient remains for delivery/audit; access is ADMIN-only and masked at the API; retention policy needs owner approval             |
| F-18 readiness semantics              | Fixed/documented              | AI readiness is 503 when unavailable; Compose uses liveness for the optional AI capability                                              |
| F-19 historical docs                  | Fixed                         | Historical banners added to old audits/implementation/report files                                                                      |
| F-20 pricing tracking note            | Fixed                         | Superseded M5 note corrected                                                                                                            |
| F-21 parcel OpenAPI proof text        | Fixed                         | Proof upload/read is no longer described as planned                                                                                     |
| F-22 Phase 1 record                   | Fixed                         | `docs/phase-1-completion-record.md` added                                                                                               |
| F-23 Compose production limits        | Documented                    | Compose remains local-first; no resource limits or production TLS claim                                                                 |
| F-24 Google nonce/azp                 | Accepted limitation           | PKCE, signature, issuer, audience and expiry are validated; no nonce is sent                                                            |
| F-25 CSP unsafe-inline                | Accepted limitation           | Required by Next hydration/theme and React styles; documented                                                                           |
| F-26..F-30, F-33, F-34                | Accepted/deferred             | Shared limiter/AI state, realtime fan-out, local proof storage, tracing/monitoring, client codegen and scrape config remain future work |
| F-31 migration rollback coverage      | Still open                    | Parcel/notification rollback is tested; other services remain partial                                                                   |
| F-32 backup/restore verification      | Still open                    | Scripts exist; no automated restore rehearsal                                                                                           |

## 3. Implementation summary

### CI and tests

- Merchant integration fixtures now use `fixtureBusinessRegNo`, which always
  includes a digit.
- The real-stack path filter includes all service and migration paths that its
  journeys exercise.
- OpenAPI validation runs in CI and locally.
- Playwright local runs now use the production build, matching CI and avoiding
  React Strict Mode's intentional double-effect request sequence.
- New real-stack specs cover administrative reads and gateway security.

### Readiness and reliability

- Added `httpx.ReadinessHandler`, `DatabaseDependency` and
  `BooleanDependency`.
- Notification worker `/ready` requires PostgreSQL and all three consumers.
- Merchant `/ready` includes the merchant-sync consumer when sync is enabled.
- Parcel `/ready` includes the outbox relay when a publisher is configured.
- Webhook worker now serves internal `/health` and `/ready` on port `8088`;
  readiness requires its consumer.
- Realtime continues to require PostgreSQL and both consumers.
- AI `/ready` is `503` when the provider is disabled or unconfigured. Compose
  uses AI `/health` for the container healthcheck because the capability is
  optional; the gateway still returns `503 AI_UNAVAILABLE` from chat.

Readiness contract:

| Service             | Liveness         | Readiness dependencies                                              |
| ------------------- | ---------------- | ------------------------------------------------------------------- |
| Identity            | `/health`        | PostgreSQL                                                          |
| Parcel              | `/health`        | PostgreSQL; outbox relay when publishing is configured              |
| Pricing             | `/health`        | PostgreSQL                                                          |
| Merchant            | `/health`        | PostgreSQL; merchant-sync consumer when enabled                     |
| Realtime            | `/health`        | PostgreSQL; status and assignment consumers                         |
| Notification worker | `/health`        | PostgreSQL; legacy, parcel-created and parcel-status consumers      |
| Webhook worker      | `/health` (8088) | webhook consumer                                                    |
| AI assistant        | `/health`        | provider capability (`ok`); `/ready` 503 when disabled/unconfigured |

Identity session state is not a readiness dependency for parcel/pricing/
realtime; authenticated requests fail closed at request time with `503` when
Identity cannot answer. This keeps public capabilities available during an
Identity outage.

### Event validation

- Added `packages/go-common/eventschema` with embedded, contract-identical
  schemas and `santhosh-tekuri/jsonschema/v6`.
- Notification, webhook, merchant-sync and realtime consumers validate the
  complete envelope/payload before handling. Schema failures are permanent and
  dead-letter; infrastructure failures stay retryable.
- A test compares the embedded copies byte-for-byte with `contracts/events`.

### Data lifecycle

- Identity deletes expired refresh tokens and expired/used reset and verification
  tokens after `IDENTITY_TOKEN_RETENTION` (default `720h`), at startup and every
  `IDENTITY_TOKEN_CLEANUP_INTERVAL` (default `24h`). Deletion is batched
  (1,000 rows per table per statement, 100 iterations per run) and never deletes
  unexpired, unused tokens.
- Notification processed-event retention remains 30 days.
- Outbox published-event retention remains 14 days.
- Merchant order reservations and notification delivery logs have no automatic
  deletion. Both require an owner-approved retention/idempotency policy; deleting
  them without one would weaken duplicate-order protection or auditability.

### Contracts and documentation

- Realtime OpenAPI now describes the implemented WebSocket.
- Parcel proof text and merchant response references are corrected.
- Merchant OpenAPI gains the missing `BadRequest` and `RateLimited` responses
  found by validation.
- README, architecture and OpenAPI README reflect current implementation.
- Historical audits, implementation reports and milestone report sources carry
  a historical banner and link to current documentation.
- Phase 1 completion record added.

### Performance

`tests/perf` contains k6 scenarios for public API latency, authentication and
WebSocket connections. Results are in
[`docs/phase-3-performance-baseline.md`](phase-3-performance-baseline.md). They
are local baselines, not production SLOs.

## 4. How to run the important suites

```sh
# Go
gofmt -l apps packages tests
go vet ./...
go build ./...
go test ./...
go test -race ./packages/go-common/... ./apps/services/realtime/... \
  ./apps/services/parcel/... ./apps/services/pricing/... \
  ./apps/services/merchant/... ./apps/services/notification-worker/...

# Contracts
node --test "tests/contracts/*.test.mjs"
pnpm exec node --test "tests/contracts/openapi.test.mjs"

# Frontend
pnpm exec nx lint @org/web
pnpm exec nx typecheck @org/web
pnpm exec nx test @org/web
pnpm exec nx build @org/web
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e

# Mocked E2E (now uses the production build)
pnpm exec nx e2e @org/web-e2e --args="--project=chromium"

# Real stack
make up
REAL_STACK=1 REAL_STACK_ADMIN_EMAIL=... REAL_STACK_ADMIN_PASSWORD=... \
  pnpm exec nx e2e @org/web-e2e --args="--project=real-stack"

# Access matrix
make test-access

# Performance smoke
k6 run tests/perf/public.js
k6 run tests/perf/websocket.js

# Deployment
docker compose config
docker run --rm ... nginx:1.27-alpine nginx -t
```

Integration tests that create disposable databases require
`TEST_DATABASE_URL`. The full Compose stack exposes PostgreSQL only on the
Compose network; run those suites from a container on that network, as CI does.

## 5. Verification evidence

Observed during this implementation:

- `go vet ./...`, `go build ./...` — pass.
- `go test ./...` — pass.
- `node --test "tests/contracts/*.test.mjs"` — 16 pass (event + OpenAPI).
- `go test ./tests/scripts/` — pass (including path-filter and Compose rotation mapping).
- `make test-access` — 40 endpoints checked across anonymous and five roles; pass.
- Merchant PostgreSQL integration suite — pass (including the previously flaky test).
- Identity token cleanup PostgreSQL integration test — pass.
- Real-stack administrative specs — 3 pass (users, pricing, reports).
- Real-stack security spec — 3 pass (headers/CSP, CSRF, WebSocket).
- Previously flaky Playwright subset — 9 pass together with production build.
- Full mocked Chromium suite — **733/733 pass** locally with the production build
  and two workers. Before the configuration change, full parallel runs showed
  different one/two-test timing flakes; the tests now use a stable local worker
  count and the production build, while CI keeps its own worker count.
- Real-stack readiness after broker loss: parcel/merchant/realtime/
  notification-worker/webhook-worker `/ready` returned `503`; identity and
  pricing remained ready (their required database was healthy). All five
  recovered after RabbitMQ restarted.
- Compose webhook rotation: Merchant was restarted with active key `k2` and
  `k1` retained in `WEBHOOK_SECRET_KEYS`; an existing `v2:k1` webhook decrypted
  successfully through the internal API. Merchant was then recreated with the
  original configuration and returned to ready.
- k6 public/auth/WebSocket baseline — recorded in the performance document.
- `docker compose config` and `nginx -t` — pass.

## 6. Remaining limitations

- Merchant order reservation retention needs an approved policy; no deletion is
  performed.
- Notification delivery-log retention needs an approved policy; recipients remain
  stored for delivery/audit and are masked by the admin API.
- Performance coverage is public/auth/WebSocket only; parcel creation, merchant
  orders, outbox drain, consumer throughput and retry recovery under sustained
  load are not yet measured.
- Migration rollback tests cover parcel/notification baseline, not every service.
- Backup/restore scripts exist but are not exercised by CI.
- Compose is not a production deployment: no TLS, resource limits, Kubernetes,
  Terraform or monitoring stack.
- Shared rate limiting/AI conversation state remain process-local.
- Realtime multi-replica fan-out/replay/SSE remain unimplemented.
- Local proof storage still requires a shared volume or S3-compatible provider
  for independent hosts.
- CSP still uses `'unsafe-inline'` for Next hydration and React styles.
- Google OIDC does not validate a nonce/`azp`; the current flow uses PKCE and
  verifies signature, issuer, audience and expiry.
