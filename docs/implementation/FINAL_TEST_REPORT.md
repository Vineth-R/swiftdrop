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

| Suite                                                    | Reason                                                                                                                                                      | How to run                                                                                                |
| -------------------------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------- |
| Full migration-cycle test (`tests/integration/database`) | The test container does not include the `golang-migrate` CLI; CI and `make db-rehearse-restore` cover it                                                    | `TEST_DATABASE_URL=... go test -tags=integration ./tests/integration/database/...` with `migrate` on PATH |
| Dedicated notification-flow integration                  | Its dedicated Compose override was not started in this earlier session; it was later run successfully (18/18) in section 10, and CI runs `notificationflow` | `go test -tags=notificationflow ./tests/integration/notificationflow/...`                                 |
| Full mocked Playwright suite                             | The real-stack suite was prioritised; mocked suites are unchanged                                                                                           | `pnpm exec nx e2e @org/web-e2e`                                                                           |
| k6 public/auth/websocket/delivery-flow                   | Not part of this acceptance run                                                                                                                             | `k6 run tests/perf/...`                                                                                   |
| Backup/restore rehearsal                                 | Needs disposable source/target URLs                                                                                                                         | `REHEARSAL_DATABASE_URL=... make db-rehearse-restore`                                                     |
| S3-compatible storage tests                              | VersityGW profile not started                                                                                                                               | `make storage-up && make test-storage`                                                                    |
| Live Gemini smoke                                        | No provider call was made                                                                                                                                   | `docs/ai-assistant.md` procedure                                                                          |
| Live Google OAuth                                        | No real consent flow was performed in this earlier session; the project owner completed it later (section 11)                                               | Manual provider verification                                                                              |
| Retention prune `APPLY=1`                                | Destructive; only the dry-run script was syntax-checked                                                                                                     | `make prune-retention` first                                                                              |
| GitHub Actions execution                                 | Workflows cannot run locally                                                                                                                                | Next pushed PR/tag                                                                                        |

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
- The dedicated notification-flow Compose override in this earlier session (closed in section 10: 18/18 pass).
- k6 performance/SLO behaviour.
- Backup/restore and storage-provider integration.
- Live Google or Gemini provider behaviour in this earlier session (both later passed; section 11).
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

---

## 9. Non-deployment finalization results (2026-11-20)

| Check                              | Command                                                    | Result                                                |
| ---------------------------------- | ---------------------------------------------------------- | ----------------------------------------------------- |
| Go format/build/vet/unit           | `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...` | PASS                                                  |
| Race tests                         | identity, merchant HTTP, pricing HTTP, objectstore         | PASS                                                  |
| Contract/OpenAPI/route tests       | `node --test tests/contracts/*.test.mjs`                   | PASS — 32                                             |
| Frontend unit/lint/build/typecheck | Nx targets                                                 | PASS                                                  |
| Audit mocked UI                    | `admin-audit-logs.spec.ts`                                 | PASS — 2                                              |
| Audit real-stack UI                | `admin-audit-logs.real.spec.ts`                            | PASS                                                  |
| Authorization matrix               | `make test-access`                                         | PASS — 66 endpoints                                   |
| Audit API live smoke               | ADMIN vs CUSTOMER vs anonymous via gateway                 | PASS — 200/403/401                                    |
| Webhook reliability                | `webhook-reliability.real.spec.ts`                         | PASS — 3 (retry, exhaustion, rotation)                |
| S3-compatible storage              | `make test-storage` with VersityGW `us-east-2`             | PASS                                                  |
| Migration cycle                    | `TestAllServiceMigrationCycles` against PostgreSQL 16      | PASS                                                  |
| Backup/restore rehearsal           | `scripts/rehearse-restore.sh` against PostgreSQL 16        | PASS                                                  |
| Retention dry run                  | `make prune-retention`                                     | PASS                                                  |
| Failed-outbox concurrency          | integration test, 20 rows / 4 workers                      | PASS                                                  |
| k6 public/tracking/pricing         | 5 VU / 10s                                                 | PASS — 0 failed, p95 ≈ 11 ms                          |
| k6 WebSocket                       | 3 VU / 8s                                                  | PASS                                                  |
| Live Gemini                        | `AI_LIVE_SMOKE=1 node scripts/ai-live-smoke.mjs`           | PASS — HTTP 200                                       |
| Google OAuth redirect              | `GET /auth/google/login`                                   | PASS — 302 with PKCE/state; manual consent later PASS |
| Compose config                     | `docker compose config`                                    | PASS                                                  |
| Prettier/diff check                | changed docs and `git diff --check`                        | PASS                                                  |

**Not executed in this session:** full real-stack suite after the audit changes
(audit real-stack spec passed; CI runs the full suite on the PR). The dedicated
notification-flow override was later run successfully in section 10 (18/18), and
the project owner later completed live Google consent/account linking (section
11).

---

## 10. Post-merge remediation verification (2026-11-20)

**Verification baseline:** `development` at
`831455b952e32cbb63623799750720635ddd185b` (PR #180).
**Branch:** `docs/final-remediation-verification` (verification/documentation,
merged as PR #181 at `682205a`).

| Check                            | Command                                                                                                         | Result                                                                              |
| -------------------------------- | --------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- |
| Go format/build/vet/unit         | `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`                                                      | PASS                                                                                |
| Contract/OpenAPI/route tests     | `node --test "tests/contracts/*.test.mjs"`                                                                      | PASS — 32                                                                           |
| Route inventories                | `go test -run TestRouteInventory ./apps/services/...`                                                           | PASS                                                                                |
| Proof and object storage         | `go test ./apps/services/parcel/internal/courier/... ./packages/go-common/objectstore/...`                      | PASS                                                                                |
| Retention dry run                | `make prune-retention`                                                                                          | PASS — read-only report, no deletion                                                |
| Frontend unit/lint/build         | Nx `@org/web` targets                                                                                           | PASS                                                                                |
| E2E lint/typecheck               | Nx `@org/web-e2e` targets                                                                                       | PASS                                                                                |
| Compose configuration            | `docker compose config`                                                                                         | PASS                                                                                |
| Live audit authorization         | gateway ADMIN/CUSTOMER/anonymous for all three sources                                                          | PASS — 200/403/401                                                                  |
| Dedicated notification flow      | `go test -race -tags=notificationflow` against disposable Compose + Mailpit                                     | PASS — 18/18                                                                        |
| Notification local prerequisites | `SMTP_HOST=mailpit`, empty SMTP user/password, worker-level `RABBITMQ_RETRY_DELAY=5s`, `RABBITMQ_MAX_RETRIES=2` | Documented in `tests/integration/notificationflow/README.md`                        |
| Google provider reachability     | Loaded the generated `/auth/google/login` URL in a real browser                                                 | PASS — Google served its sign-in page for the configured client ID and redirect URI |
| Google account consent/linking   | Completed by the project owner with an authorized Google account; exact date not recorded                       | **PASS** — manual consent and account linking verified                              |
| Post-merge CI (PR #180)          | GitHub Actions CI + Identity database                                                                           | PASS — 22m37s and 3m04s                                                             |

### Notification-flow detail

The earlier local `TestParcelDomainEventIdentityUnavailableIsDeadLettered`
timeout was a harness prerequisite: the retry-delay and retry-count values were
passed only to `go test`, while the worker container kept its 30s/3-retry
defaults, so the 60s dead-letter wait expired. Exporting them before
`docker compose up` made all retry/dead-letter tests pass. CI already sets them
at job level; no product code changed.

### Not executed in this pass

- The full real-stack Playwright suite was not re-run on this documentation
  branch; it passed on PR #180 and its post-merge CI is green.
- The 66-route access matrix was not re-run locally in this pass; it passed on
  PR #180 and its post-merge CI is green. A live ADMIN/CUSTOMER/anonymous smoke
  of the audit APIs was re-run and passed.
- Live Google account consent was completed manually by the project owner; see
  section 11.

---

## 11. Final closure verification (2026-11-20)

**Final baseline:** `development` at
`682205aa2856fd8f03721979d88a638e1313cd6a` (PR #181 merge; contains PR #180 and
PR #179).

| Check                                         | Result                                                                              |
| --------------------------------------------- | ----------------------------------------------------------------------------------- |
| PR #181 required checks                       | PASS                                                                                |
| Post-merge CI (PR #181)                       | PASS — CI 5m01s; Identity database 2m39s                                            |
| Final `development` working tree              | Clean                                                                               |
| Retention safety regression (`tests/scripts`) | PASS — dry run read-only, four owned tables, bounded batches, unsafe limits refused |
| Contract tests on final tree                  | PASS — 32                                                                           |
| Dedicated notification flow                   | PASS — 18/18                                                                        |
| Google OAuth automated tests                  | PASS — 32                                                                           |
| Google OAuth manual consent/account linking   | PASS — completed by the project owner; exact date not recorded                      |
| Live audit authorization                      | PASS — ADMIN 200, CUSTOMER 403, anonymous 401                                       |

### Final verification statement

All mandatory approved non-deployment requirements have implementation and
executed evidence. The final repository state contains no unresolved mandatory
implementation defect. Google OAuth is verified by both automated tests and the
project owner's manual consent/account-linking run.

Production deployment, TLS/hosting, monitoring rollout and multi-replica state
remain out of scope and are not claimed.

---

## 12. Post-acceptance merchant enhancement results (2026-11-20)

**Branch:** `feat/merchant-experience-enhancements`, based on
`development` at `6686de2708776b725a40a91e8086bf68e2e03c77`.
This is an additive enhancement, not a change to the accepted baseline.

| Check                          | Command                                                                                                             | Actual result                                                         |
| ------------------------------ | ------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------- |
| Go format / build / vet / unit | `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`                                                          | PASS                                                                  |
| Receipt renderer               | `go test ./apps/services/merchant/internal/receipt/...`                                                             | PASS — 3 tests, PDF signature, content and Unicode name               |
| Receipt ownership integration  | `go test -tags=integration -run TestMerchantOrderReceiptPDFAndIsolation ./apps/services/merchant/internal/http/...` | PASS — own API key/session 200 PDF; other merchant 404; anonymous 401 |
| Contract/OpenAPI/route tests   | `make check-openapi`; `node --test "tests/contracts/*.test.mjs"`                                                    | PASS — 32                                                             |
| Route inventories              | `UPDATE_ROUTES=1 go test -run TestRouteInventory ./apps/services/merchant/...`; normal rerun                        | PASS                                                                  |
| Web typecheck/lint/build       | `nx typecheck`, `nx lint`, `nx build` `@org/web`                                                                    | PASS                                                                  |
| E2E typecheck/lint             | `nx typecheck`, `nx lint` `@org/web-e2e`                                                                            | PASS                                                                  |
| Merchant orders E2E            | `merchant-orders.spec.ts`                                                                                           | PASS — 9 tests                                                        |
| App shell E2E                  | `app-shell.spec.ts`                                                                                                 | PASS — 25 tests                                                       |
| Layouts E2E                    | `layouts.spec.ts`                                                                                                   | PASS — 40 tests, including `/merchant/integration`                    |
| Dashboards E2E                 | `dashboards.spec.ts`                                                                                                | PASS                                                                  |
| Merchant API guide PDF         | `make merchant-api-pdf`                                                                                             | PASS — 18-page A4 PDF, `%PDF-1.4`, cover inspected                    |
| PDF availability               | `apps/web/public/merchant-api-guide.pdf`                                                                            | PASS — generated by the same target                                   |

### Not run

- Full real-stack Compose/Playwright suite: the affected mocked/UI, customer and
  merchant integration tests and the two affected real-stack journeys were run;
  the complete suite is left to CI.
- CI: the PR checks are the authority for the final full matrix.

The combined `dashboards.spec.ts layouts.spec.ts` run produced two timeout
flakes; each spec passed when rerun alone (`layouts` 40/40, `dashboards` PASS).
No product assertion failed.
