# SwiftDrop — Final Test Report

**Date:** 2026-11-19
**Branch:** `docs/final-acceptance-cleanup`
**Environment:** macOS host, Go 1.27.4 toolchain, Node/pnpm/Nx, Docker 29.9.3
with Compose v2. A disposable local Compose stack was built and started for the
integration pass.

> A check is reported as PASS only when it was executed. Integration-tagged tests
> were executed against real PostgreSQL 16 and RabbitMQ 4.1 inside the Compose
> network. Existing suites that were not executed are listed as NOT RUN with the
> reason.

## 1. Executed checks

|   # | Check                                    | Command                                                                                                                                                  | Result                                          |
| --: | ---------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------- |
|   1 | Go formatting                            | `gofmt -l apps packages tests`                                                                                                                           | PASS — no files listed                          |
|   2 | Go build                                 | `go build ./...`                                                                                                                                         | PASS                                            |
|   3 | Go vet                                   | `go vet ./...`                                                                                                                                           | PASS                                            |
|   4 | Go unit tests                            | `go test ./...`                                                                                                                                          | PASS                                            |
|   5 | Race detector, changed concurrency paths | `go test -race ./apps/services/identity/... ./apps/services/merchant/internal/orders/... ./apps/services/parcel/internal/outbox/... ./tests/scripts/...` | PASS                                            |
|   6 | Contract + OpenAPI + route inventory     | `node --test "tests/contracts/*.test.mjs"`                                                                                                               | PASS — 32 tests                                 |
|   7 | OpenAPI gate                             | `make check-openapi`                                                                                                                                     | PASS — 32 tests                                 |
|   8 | Frontend unit tests                      | `pnpm exec nx test @org/web`                                                                                                                             | PASS                                            |
|   9 | Frontend lint                            | `pnpm exec nx lint @org/web`                                                                                                                             | PASS                                            |
|  10 | Frontend production build                | `pnpm exec nx build @org/web`                                                                                                                            | PASS                                            |
|  11 | Compose configuration                    | `docker compose config`                                                                                                                                  | PASS                                            |
|  12 | Compose build + start + migrations       | `make up-build`                                                                                                                                          | PASS — all services healthy, migrations applied |
|  13 | Gateway route inventory                  | `make check-routes` (run by `make up-build`)                                                                                                             | PASS — every route and redirect                 |
|  14 | Deterministic demo seed                  | `make seed-demo`                                                                                                                                         | PASS — 5 users, merchant, 6 parcels             |
|  15 | Authorization matrix                     | `make test-access`                                                                                                                                       | PASS — 63 endpoints × anonymous + five roles    |
|  16 | Real-stack browser acceptance            | 6 Playwright `real-stack` specs                                                                                                                          | PASS — 6/6 in 2m19s                             |
|  17 | Event-flow integration                   | `go test -tags=integration ./tests/integration/eventflow/...` in Compose network                                                                         | PASS — 7 tests                                  |
|  18 | Identity repository integration          | `go test -tags=integration ./apps/services/identity/internal/repository/...`                                                                             | PASS                                            |
|  19 | Merchant order/API-key integration       | `go test -tags=integration -run 'TestMerchantOrder                                                                                                       | TestMerchantAPIKey                              | TestMerchantOrderConcurrent' ./apps/services/merchant/internal/http/...` | PASS |
|  20 | Retention script syntax                  | `sh -n scripts/prune-retention.sh`                                                                                                                       | PASS                                            |
|  21 | CI gate policy tests                     | `go test ./tests/scripts/...`                                                                                                                            | PASS                                            |
|  22 | Whitespace/conflict check                | `git diff --check`                                                                                                                                       | PASS — no output                                |
|  23 | Documentation formatting                 | `pnpm exec prettier --check ...`                                                                                                                         | PASS                                            |

## 2. Regression tests added or updated

### Authentication / logout

| Test                                        | File                                                                                                  |
| ------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| Logout with refresh token only              | `apps/services/identity/internal/http/auth_test.go`                                                   |
| Logout with an expired access token         | `apps/services/identity/internal/http/auth_test.go`                                                   |
| Idempotent repeated logout                  | `apps/services/identity/internal/http/auth_test.go`                                                   |
| Service failure still clears cookies        | `apps/services/identity/internal/http/auth_test.go`                                                   |
| Service revokes the presented session only  | `apps/services/identity/internal/service/service_test.go`                                             |
| Service revokes a rotated successor         | `apps/services/identity/internal/service/service_test.go`                                             |
| Repository revokes a token family           | `apps/services/identity/internal/repository/refresh_tokens_integration_test.go`                       |
| Concurrent refresh + logout (20 iterations) | `apps/services/identity/internal/repository/refresh_tokens_integration_test.go`                       |
| Access-matrix logout is public/idempotent   | `tests/access/matrix_test.go`, `tests/access/accesskit/accesskit.go`, `tests/access/sessions_test.go` |

The repository integration suite was executed against real PostgreSQL and
passed, including `TestRepository_RevokeRefreshSessionConcurrentWithRefresh`.

### Rate limits

| Test                                  | File                                                   |
| ------------------------------------- | ------------------------------------------------------ |
| Parcel creation 30/min per customer   | `apps/services/parcel/internal/http/parcels_test.go`   |
| Webhook test send 10/min per merchant | `apps/services/merchant/internal/http/webhook_test.go` |

### Merchant order recovery

| Test                                                                        | File                                                                               |
| --------------------------------------------------------------------------- | ---------------------------------------------------------------------------------- |
| Retry after Parcel success + Merchant persistence failure reuses the parcel | `apps/services/merchant/internal/orders/service_test.go`                           |
| Concurrent duplicate/same-key/many-payload creation                         | `apps/services/merchant/internal/http/orders_integration_test.go` — executed, PASS |

### Outbox recovery

| Test                                  | File                                                                |
| ------------------------------------- | ------------------------------------------------------------------- |
| Failed summary, replay and expiry     | `tests/integration/eventflow/outbox_claim_test.go` — executed, PASS |
| Claim once, stale reclaim, lost claim | `tests/integration/eventflow/outbox_claim_test.go` — executed, PASS |

### CI gate

| Test                                                        | File                                   |
| ----------------------------------------------------------- | -------------------------------------- |
| In-scope access/notification/real-stack skips fail the gate | `tests/scripts/ci_path_filter_test.go` |
| Out-of-scope skips remain allowed                           | `tests/scripts/ci_path_filter_test.go` |

## 3. Full-stack evidence

### 3.1 Compose startup and migrations

`make up-build` completed:

- all nine service/web images built;
- PostgreSQL, RabbitMQ and Mailpit started healthy;
- every service migration applied;
- every application service became healthy;
- Nginx started after its upstreams;
- `make check-routes` passed every gateway route, internal-API block,
  no-slash redirect and WebSocket route check.

### 3.2 Authorization matrix

`make test-access`:

- **63 endpoints** checked as anonymous and as CUSTOMER, MERCHANT, COURIER,
  DISPATCHER and ADMIN;
- customer, courier and merchant ownership cases passed;
- login error/ lockout / refresh rotation / reuse / logout / password change /
  expired and forged token cases passed.

### 3.3 Real-stack browser acceptance

Six Playwright `real-stack` specs passed against the live stack:

1. `courier-completion.real.spec.ts` — delivery with proof and a failed delivery.
2. `customer-delivery-journey.real.spec.ts` — customer → dispatcher → courier →
   delivered, customer tracking/detail, dispatcher state, notification log.
3. `customer-parcel-journey.real.spec.ts` — customer wizard, list and detail.
4. `event-flow.real.spec.ts` — one status change reaches realtime socket,
   merchant order snapshot, signed webhook and notification email; duplicate
   delivery is idempotent.
5. `merchant-orders.real.spec.ts` — merchant order creation, status sync and
   cross-merchant isolation.
6. `merchant-webhooks.real.spec.ts` — save, rotate, test and scope isolation.

### 3.4 Event and repository integration

- `tests/integration/eventflow` passed all 7 tests:
  status change → notification/webhook/realtime, outbox claim once, stale claim
  reclaim, fresh-claim protection, lost-claim protection, failed replay/expiry,
  creation + assignment versioned events.
- Identity repository integration passed, including logout/refresh concurrency.
- Merchant order and API-key HTTP integration passed, including concurrent
  duplicate creation.

### 3.5 Test robustness issue found during the full-suite batch

`seeded-roles.real.spec.ts` signed in with manual form calls. When it ran after
the other real-stack journeys, the shared per-address login limiter (10/min)
rejected the later logins and three role cases stayed on `/login`. The product
behaviour is correct; the test was fragile. It now uses the shared
`signInThroughForm` helper, which waits out a 429. The suite was rerun and passed
6/6 (the CI-only account-presence test is skipped locally).

### 3.6 Configuration issue found during the run

The local `.env` contained `SMTP_USER`/`SMTP_PASSWORD` while `SMTP_HOST=mailpit`.
The mailer correctly refused to send credentials to a server that does not offer
STARTTLS, so some notification retries exhausted. The notification worker was
recreated with empty SMTP credentials for Mailpit, after which the event-flow
email test passed. This is a demo configuration mismatch, not a code defect.
The corrected guidance is in `DEMO_READINESS.md`.

## 4. Not executed in this session

| Suite                                                    | Reason                                                                                                             | How to run                                                                                                |
| -------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------- |
| Full migration-cycle test (`tests/integration/database`) | The test container does not include the `golang-migrate` CLI; CI and `make db-rehearse-restore` cover it           | `TEST_DATABASE_URL=... go test -tags=integration ./tests/integration/database/...` with `migrate` on PATH |
| Dedicated notification-flow integration                  | Its dedicated Compose override was not started; email delivery is covered by the event flow and real-stack journey | CI job `notificationflow`                                                                                 |
| Full mocked Playwright suite                             | The real-stack suite was prioritised; mocked suites are unchanged                                                  | `pnpm exec nx e2e @org/web-e2e`                                                                           |
| k6 public/auth/websocket/delivery-flow                   | Not part of this acceptance run                                                                                    | `k6 run tests/perf/...`                                                                                   |
| Backup/restore rehearsal                                 | Needs disposable source/target URLs                                                                                | `REHEARSAL_DATABASE_URL=... make db-rehearse-restore`                                                     |
| S3-compatible storage tests                              | VersityGW profile not started                                                                                      | `make storage-up && make test-storage`                                                                    |
| Live Gemini smoke                                        | No provider call was made                                                                                          | `docs/ai-assistant.md` procedure                                                                          |
| Live Google OAuth                                        | No real Google consent flow was performed                                                                          | Manual provider verification                                                                              |
| Retention prune `APPLY=1`                                | Destructive; only the dry-run script was syntax-checked                                                            | `make prune-retention` first                                                                              |
| GitHub Actions execution                                 | Workflows cannot run locally                                                                                       | Next pushed PR/tag                                                                                        |

## 5. Coverage

Coverage was not re-measured in this pass. The repository has `make coverage`
and CI publishes a Go coverage artifact; `make coverage-merchant` enforces 60%
for the merchant service with PostgreSQL tests. The new tests are unit/handler
tests plus integration tests for refresh/logout concurrency and outbox recovery.

## 6. What the executed evidence establishes

- The complete Go tree builds, vets and passes unit tests.
- The changed concurrency paths pass the race detector.
- OpenAPI, route inventory, event schema and gateway mappings are consistent.
- The full Compose stack builds, migrates, becomes healthy and routes correctly.
- All 63 access-matrix endpoints enforce the intended roles.
- A customer, dispatcher and courier complete a real delivery through the real
  UI, and the customer/tracking/dispatcher/notification state agrees.
- The event chain works with real PostgreSQL and RabbitMQ: realtime, merchant
  order, webhook and email consumers each receive one status change.
- Duplicate event delivery does not duplicate business side effects.
- The new failed-outbox replay/expiry operations work against real PostgreSQL.
- Merchant order idempotency and API-key limits work against real PostgreSQL.

## 7. What the executed evidence does not establish

- The full migration down/up cycle in this environment (tooling gap only).
- The dedicated notification-flow Compose override.
- k6 performance/SLO behaviour.
- Backup/restore and storage-provider integration.
- Live Google or Gemini provider behaviour.
- Production deployment, TLS, multi-replica realtime or shared rate limiting.
- The edited GitHub Actions workflow on a GitHub runner.

## 8. Final assessment

**Unit, contract, frontend, Compose, authorization, real-stack browser and
event-flow integration evidence: PASS.**

The core customer-to-delivery workflow is demonstrably working across identity,
pricing, parcel, dispatch, courier proof, notification, webhook, merchant sync,
realtime and the frontend. The remaining gaps are operational (production
deployment, multi-replica state, live providers, full migration rehearsal) and
are listed explicitly rather than counted as verified.
