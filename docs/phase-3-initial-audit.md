# Phase 3 Initial Repository Audit & Quality Assessment

| Field                       | Value                                                                                                                                  |
| --------------------------- | -------------------------------------------------------------------------------------------------------------------------------------- |
| Assessment date             | 2026-11-14                                                                                                                             |
| Repository                  | `github.com/Vineth-R/swiftdrop`                                                                                                        |
| Assessment baseline         | `development` @ `686a03dc25538f6353839fbb16dd83c0c56457e5`                                                                             |
| Phase 1 merge               | `19ef83c226ddec1aa4c828d0f9c2e6f80d9efb76` (PR #151)                                                                                   |
| Phase 2 merge               | `e94eee7a11ba6d9ef217f39d1d93c6a4353991b0` (PR #154)                                                                                   |
| Phase 2 documentation merge | `686a03dc25538f6353839fbb16dd83c0c56457e5` (PR #155)                                                                                   |
| Assessment type             | Read-only architecture, implementation, contract, database, messaging, security, test, performance, deployment and documentation audit |
| Code changes made           | None (this report only)                                                                                                                |

## A. Executive summary

SwiftDrop is a substantial, working microservice platform rather than a scaffold. The latest
`development` branch contains eight Go services, a Next.js application, five owned PostgreSQL
schemas, a transactional outbox, an isolated retry/dead-letter RabbitMQ topology, signed merchant
webhooks, email delivery, realtime status and assignment updates, and a Gemini-backed assistant.
The Phase 2 security/reliability controls are present and covered by tests.

The system is **not yet in a clean Phase 3 starting state**. The latest `development` HEAD has a
failing required CI gate caused by a flaky merchant PostgreSQL integration test. Documentation is
also materially out of date in the root README, `docs/architecture.md`, the realtime OpenAPI
contract, and several historical audit/report files. Webhook key rotation cannot be configured
through Compose even though it is documented in `.env.example`. Worker readiness does not reflect
broker/consumer health outside realtime. There are no load/performance tests and no performance
baseline.

### Major strengths

- Clear service/schema ownership; no cross-service `internal/` imports or cross-schema SQL in
  service code (seed tools are documented exceptions).
- Real end-to-end business paths exist for customer parcel creation, public tracking, dispatcher
  assignment, courier lifecycle, merchant orders, webhooks, notifications and realtime updates.
- Strong CI breadth: Go race tests, PostgreSQL, RabbitMQ, event flow, migrations, access matrix,
  real-stack Playwright, S3-compatible storage, pooled PostgreSQL, security scanning and image
  scans.
- Phase 2 session revocation is fail-closed, webhook ciphertext is AAD-bound with legacy
  read-repair, CSRF/CSP controls are present, and metrics are available on every HTTP service.
- Docker Compose starts the complete system and all containers reached `healthy` during this audit.

### Most important remaining gaps

1. The required CI gate on `development` is currently red because of a test-data flake in the
   merchant integration suite.
2. Compose does not pass `WEBHOOK_SECRET_KEY_ID` or `WEBHOOK_SECRET_KEYS` to Merchant, so the
   documented rotation procedure is not usable in the default stack.
3. Root README, architecture documentation and realtime OpenAPI contain false or stale statements
   that will mislead implementation and assessment work.
4. CI path filters can skip access and real-stack E2E suites for service changes that affect them.
5. There is no load/performance test suite or measured throughput/latency baseline.
6. Worker and outbox readiness does not represent broker/consumer health; a broker outage can leave
   workers looking healthy while queues back up.
7. OpenAPI contracts are not validated in CI, and event payloads are not validated against their
   JSON Schemas at runtime.
8. Identity refresh/reset/verification token tables have no retention cleanup.
9. Several important admin/pricing/reports UI workflows are covered only by mocked Playwright tests.
10. Historical audits and milestone reports contradict the current implementation and are not
    marked historical.

## B. Repository baseline

### Applications and services

| Component             | Responsibility                                                                                       | Port            | Persistence           |
| --------------------- | ---------------------------------------------------------------------------------------------------- | --------------- | --------------------- |
| `identity`            | Auth, sessions, admin users, couriers, profiles, internal identity lookups                           | 8081            | `identity` schema     |
| `parcel`              | Parcel lifecycle, pricing calls, tracking, reports, dispatch, courier delivery, proofs, outbox relay | 8082            | `parcel` schema       |
| `pricing`             | Quotes, zones, pricing rules/bands/surcharges, admin configuration                                   | 8083            | `pricing` schema      |
| `merchant`            | Merchant onboarding/status, API keys, orders, webhooks, order status sync                            | 8084            | `merchant` schema     |
| `realtime`            | WebSocket `/ws`, status and assignment consumers, in-process hub                                     | 8085            | none                  |
| `notification-worker` | Customer/merchant email, idempotency, delivery logs, ADMIN read API                                  | 8086 (internal) | `notification` schema |
| `ai-assistant`        | Public Gemini-backed assistant, guest sessions, private read-only tools                              | 8087            | none                  |
| `webhook-worker`      | Signed merchant webhook delivery, attempts, six business retries                                     | no HTTP         | none                  |
| `web`                 | Next.js App Router frontend                                                                          | 3000            | none                  |
| `nginx`               | Public edge, prefix stripping, security headers, WebSocket proxy                                     | 8080            | none                  |

### Shared Go packages

`packages/go-common` contains auth, config, logging, HTTP helpers, request IDs, lifecycle,
rate limiting, internal-token middleware, cross-origin protection, messaging envelope and consumer,
retry/dead-letter topology, PostgreSQL pool/readiness, metrics, object storage, request validation,
secretbox keyring, session checking, webhook URL SSRF validation, email rendering and SMTP delivery.
`packages/webhookdelivery` owns signed webhook requests and the business retry schedule.

### Database schemas and migrations

| Schema         | Applied version | Migration files | Notes                                                                                               |
| -------------- | --------------: | --------------: | --------------------------------------------------------------------------------------------------- |
| `identity`     |               6 | 6 up/down pairs | Includes email verification, OAuth browser binding, session version, merchant lifecycle transitions |
| `pricing`      |               2 | 2 up/down pairs | Baseline tables and seeds                                                                           |
| `parcel`       |               7 | 7 up/down pairs | Dispatch foundations, price breakdown, outbox types/claim, customer list index                      |
| `merchant`     |              11 | 8 up/down pairs | Versions 3-5 were removed before merge; gaps are documented                                         |
| `notification` |               2 | 2 up/down pairs | Delivery logs and processed events                                                                  |

Read-only `make migrate-status` on the running stack reported exactly those versions. All migration
pairs have a matching down file. CI's fresh-database migration job passed on the audited tree.

The baseline is 31 business tables: 26 baseline tables plus five additive tables. No cross-schema
foreign keys exist. Cross-schema data is exchanged through HTTP or events. Operator seed tools are
the documented exception: identity seed reads `pricing.zones`; parcel seed reads identity users,
courier profiles and pricing zones; merchant seed reads pricing zones.

### API and event contracts

Nine OpenAPI 3.0.3 files cover identity, identity-internal, parcel, pricing, merchant,
merchant-internal, notification, AI assistant and realtime. Four event JSON Schemas cover
`parcel.created.v1`, `parcel.assigned.v1`, `parcel.status_changed.v1` and the deprecated
`notification.requested.v1`.

Event contracts are validated in CI by `node --test "tests/contracts/*.test.mjs"`. OpenAPI files are
maintained by hand and are not parsed or validated in CI.

### Tests and CI

- 267 Go `_test.go` files.
- 69 files carrying the `integration` build tag (including helpers).
- 50 Playwright spec files, 9 of them real-stack specs.
- 11 Vitest specs under `apps/web/src`.
- Unit coverage snapshot: 65.3% overall; merchant CI gate requires at least 60% with PostgreSQL
  integration tests.
- CI jobs include Go checks, race coverage, vulnerability scanning, Gitleaks, CodeQL, RabbitMQ,
  PostgreSQL, event flow, migrations, storage, pooled PostgreSQL, frontend checks, Chromium E2E,
  access matrix, real-stack onboarding, notification workflow, Docker builds/SBOM/Trivy and a
  required aggregate gate.

### Current baseline health

- Working tree clean at `686a03d`; `origin/development` matches HEAD.
- Phase 1, Phase 2 and the Phase 2 documentation update are merged.
- Docker Compose configuration validates.
- All 13 running application/infrastructure containers reported `healthy` during the audit.
- **Latest `development` CI is red**: required gate failed because the PostgreSQL integration job
  failed `TestWebhookRepositoryUpdatePreservesSecretUnlessRotated` in
  `apps/services/merchant/internal/http/webhook_repository_integration_test.go:192`.
  The test data helper uses `"WEBHOOK-" + merchantID[:8]`; a UUID prefix can contain no decimal
  digit (for example `abeabcdc`), while `businessreg.Valid` requires at least one digit. This is a
  nondeterministic test-data defect, not a production-code failure. It blocked the required gate on
  the audited merge commit.

## C. Functional completeness matrix

Status terms:

- **Complete/verified** — implementation and meaningful execution evidence exist.
- **Complete/test gap** — implementation exists but a material integration/E2E path is missing.
- **Partial** — some layers or acceptance paths exist.
- **Missing** — not implemented.
- **Accepted** — intentionally deferred and documented.

| Feature                                     | Implementation status | Integration status                              | Test coverage                                            | Findings                                           |
| ------------------------------------------- | --------------------- | ----------------------------------------------- | -------------------------------------------------------- | -------------------------------------------------- |
| Customer registration/login/refresh/logout  | Complete              | Identity + gateway + web                        | Unit, integration, mocked + real-stack onboarding        | No material gap                                    |
| Email verification/resend                   | Complete              | Identity + Mailpit                              | Unit/integration + real-stack                            | No material gap                                    |
| Google login                                | Complete              | Identity + Google OIDC                          | Unit/integration; live provider evidence in docs, not CI | Provider path needs a controlled live smoke test   |
| Password reset                              | Complete              | Identity + Mailpit                              | Unit/integration; mocked UI                              | No real-stack reset UI journey                     |
| User profiles/password change               | Complete              | Identity + web                                  | Unit + mocked UI                                         | No material gap                                    |
| Admin users                                 | Complete              | Identity + web                                  | Backend integration; UI mocked                           | No real-stack admin-user journey                   |
| Session revocation                          | Complete              | Identity → parcel/pricing/realtime/notification | Unit, route tests, prior runtime evidence                | Fail-closed; 30s bounded cache                     |
| Pricing estimate/zones/internal quote       | Complete              | Pricing + parcel/AI                             | Unit, HTTP integration, mocked UI                        | No real-stack pricing-admin journey                |
| Pricing configuration                       | Complete              | Pricing + web                                   | Unit/integration; UI mocked                              | Admin UI lacks real-stack coverage                 |
| Public tracking                             | Complete              | Parcel + web                                    | Integration + real-stack                                 | No material gap                                    |
| Customer parcel creation                    | Complete              | Parcel + pricing + outbox                       | Unit/integration + real-stack                            | No material gap                                    |
| Customer parcel list/detail/history/receipt | Complete              | Parcel + web                                    | Unit/HTTP + mocked UI                                    | No real-stack UI journey                           |
| Parcel edit/cancel                          | Not in API v1.1       | N/A                                             | N/A                                                      | Do not invent; out of current contract             |
| Dispatcher assignment/reassignment          | Complete              | Parcel + identity eligibility                   | Integration + real-stack                                 | No material gap                                    |
| Courier delivery list/detail                | Complete              | Parcel + identity contact                       | Integration + real-stack                                 | No material gap                                    |
| Courier status lifecycle                    | Complete              | Parcel state machine + outbox                   | Integration + real-stack                                 | No material gap                                    |
| Courier proof upload/view                   | Complete              | Parcel + objectstore                            | Unit/integration + mocked UI                             | Local storage multi-host limitation                |
| Reports (daily/outcomes)                    | Complete              | Parcel + web                                    | DB integration; UI mocked                                | No real-stack reports UI journey                   |
| Merchant registration/onboarding            | Complete              | Merchant + identity + admin                     | Integration + real-stack                                 | No material gap                                    |
| Merchant profile/API keys                   | Complete              | Merchant + web                                  | Integration + mocked/real-stack merchant journeys        | No material gap                                    |
| Merchant orders/status snapshots            | Complete              | Merchant → parcel → sync consumer               | Integration + real-stack                                 | No material gap                                    |
| Webhook configuration/delivery/retries      | Complete              | Merchant + webhook worker                       | Unit/integration + real-stack/event flow                 | Compose rotation env missing                       |
| Notification email                          | Complete              | Notification worker + identity/merchant         | Integration + notification/real-stack                    | No material gap                                    |
| SMS notification                            | Missing               | N/A                                             | N/A                                                      | Accepted; no provider                              |
| AI assistant                                | Complete              | AI + pricing/parcel                             | Unit/orchestration + mocked E2E                          | Live Gemini path not in CI; state is process-local |
| Realtime status/assignment updates          | Complete              | Realtime consumers + web                        | Unit + real-stack                                        | Multi-replica fan-out missing                      |
| Frontend pages/forms/states                 | Complete              | N/A                                             | Vitest + Playwright                                      | Admin pricing/users/reports lack real-stack        |
| Metrics                                     | Complete              | All HTTP services                               | Unit + runtime                                           | No scrape/alert deployment                         |
| Distributed tracing                         | Missing               | N/A                                             | N/A                                                      | Accepted/deferred                                  |
| Load/performance validation                 | Missing               | N/A                                             | None                                                     | No k6 or benchmark baseline                        |

## D. Business workflow matrix

| Workflow                            | Services involved                                        | Current status  | Missing integration                        | Required tests                                    |
| ----------------------------------- | -------------------------------------------------------- | --------------- | ------------------------------------------ | ------------------------------------------------- |
| 1. Registration and auth            | Web, identity, Mailpit                                   | Complete        | None material                              | Keep real-stack onboarding                        |
| 2. Merchant registration/onboarding | Web, merchant, identity, Mailpit, admin                  | Complete        | None material                              | Keep real-stack onboarding                        |
| 3. Merchant order creation          | Merchant API client, merchant, parcel, pricing           | Complete        | None material                              | Keep real-stack merchant order journey            |
| 4. Order-to-parcel integration      | Merchant → parcel internal API                           | Complete        | No direct Parcel→Merchant lookup by design | Add failure-path tests for Parcel outage          |
| 5. Parcel creation and pricing      | Web, parcel, pricing, outbox                             | Complete        | None material                              | Add real-stack quote/timeout/price-change journey |
| 6. Assignment and dispatch          | Web, parcel, identity eligibility, realtime              | Complete        | None material                              | Keep assignment-live real-stack                   |
| 7. Courier lifecycle                | Web, parcel, identity, outbox, realtime                  | Complete        | None material                              | Keep courier real-stack                           |
| 8. Public tracking                  | Web, parcel                                              | Complete        | None material                              | Keep customer tracking real-stack                 |
| 9. Realtime status/assignment       | Parcel, RabbitMQ, realtime, web                          | Complete        | Multi-replica fan-out/replay               | Add broker outage/recovery E2E                    |
| 10. Notification delivery           | Parcel, RabbitMQ, notification worker, identity/merchant | Complete        | SMS absent                                 | Keep notificationflow/real-stack                  |
| 11. Webhook delivery/retry          | Parcel, RabbitMQ, webhook worker, merchant               | Complete        | Compose rotation env                       | Add rotation-in-Compose test                      |
| 12. Administrative workflows        | Web, identity, merchant, pricing, parcel, notification   | Mostly complete | Real-stack admin users/pricing/reports     | Add real-stack admin journeys                     |

## E. API, event and database findings

### Contract drift

1. **Realtime OpenAPI is materially stale.** `contracts/openapi/realtime.yaml` still says live
   tracking is future, lists only `/health` and `/ready`, and does not document `/ws`, subscriptions,
   ping/pong, token/cookie auth, status messages, assignment messages or consumer-aware readiness.
2. **Parcel OpenAPI description is stale.** It says proof uploads and remaining lifecycle writes
   remain planned, although proof upload/read/view is implemented.
3. **OpenAPI is not validated in CI.** There is no parser, linter or route-drift test. Hand-maintained
   specs can drift without a failing check.
4. **Event payloads are not validated against JSON Schema at runtime.** Envelope and message framing
   are validated; handlers validate the fields they consume. Schema validation is test-only.
5. The API documentation and OpenAPI files otherwise match implemented route families; no
   unauthorized extra public route family was found.

### Database findings

- No cross-schema foreign keys or cross-schema service SQL were found in product code.
- Migrations are additive and documented; a fresh database chain passes CI.
- Merchant versions 3-5 are gaps by history, not missing files; the chain is current at 11.
- Parcel/notification rollback is tested, but rollback coverage is not complete for every service.
- **Identity token retention is missing** for `refresh_tokens`, `password_reset_tokens` and
  `email_verification_tokens`; only OAuth states are deleted. Tables can grow indefinitely.
- **Merchant order reservations have no retention policy.** Completed/failed rows are needed for
  idempotency, but old rows accumulate.
- Notification logs store the raw recipient for delivery and audit; the API masks it, but retention
  and access handling are not documented as a privacy control.
- Outbox cleanup and notification processed-event cleanup exist.

### Messaging findings

- Producers map unversioned outbox labels to versioned routing keys correctly.
- Retry queues dead-letter to their owning queue; queue isolation is implemented and tested.
- Claimed outbox rows use `FOR UPDATE SKIP LOCKED`, claim tokens, stale reclaim and scoped
  transitions.
- Notification idempotency uses `processed_events`; delivery is at-least-once and documented.
- Webhook delivery is at-least-once with successful-attempt deduplication; receivers must dedupe.
- Realtime has no persistence/replay and each replicated consumer receives one event.
- No event payload JSON Schema validation at the consumer boundary.
- No automated chaos/failure E2E for identity or broker outage in CI; realtime failure recovery was
  verified manually in Phase 2 review but is not a CI regression test.

## F. Testing gap analysis

### Strong coverage

- Go unit and race suites across all services and shared packages.
- Real PostgreSQL integration tests for identity, merchant, parcel, pricing, notification logs and
  event origins.
- Real RabbitMQ integration tests for retry, dead-letter, framing and event flow.
- Migration validation from a fresh database and merchant coverage gate.
- Access/ownership matrix against the Compose stack for a representative route set.
- Nine real-stack Playwright specs covering registration, merchant orders, webhooks, courier
  workflow, assignment live updates, customer tracking, event flow, seeded roles and notification
  logs.
- Contract schema tests for events and merchant registration.
- Security scanning, dependency audit, SBOM and image scan jobs.

### Gaps by risk

| Gap                         | Impact                                                                                                                                                        | Evidence                                                               |
| --------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- |
| CI path filters             | Access tests trigger only on Compose/infra/migrations/tests, not service code. Real-stack filter omits parcel/realtime/notification/webhook/AI service paths. | `.github/workflows/ci.yml` `changes` filters                           |
| Access matrix scope         | Only 26 cases; missing profile PATCH, webhook settings, merchant orders/keys, proof photo, report reads and several admin routes.                             | `tests/access/matrix_test.go`                                          |
| Admin UI E2E                | Admin users, pricing admin and reports UI are mocked only.                                                                                                    | `apps/web-e2e/src/*.spec.ts` vs real specs                             |
| AI provider                 | No live Gemini smoke in CI.                                                                                                                                   | AI docs and CI                                                         |
| Performance                 | No k6 scripts or benchmarks.                                                                                                                                  | Repository search                                                      |
| OpenAPI                     | No parser/drift validation.                                                                                                                                   | `contracts/openapi/README.md`, CI                                      |
| Worker failure modes        | No CI E2E for broker loss/identity outage on workers/outbox.                                                                                                  | CI jobs                                                                |
| CSP/CSRF browser regression | Unit/CSRF tests exist; no automated browser header/CSP regression check.                                                                                      | CI jobs                                                                |
| Baseline test flake         | Merchant integration test can fail on a UUID with no decimal digit.                                                                                           | CI run `43612384956`, `webhook_repository_integration_test.go:111/192` |
| Playwright local flakiness  | Different failure sets across full local runs.                                                                                                                | `docs/phase-2-security-reliability.md` verification section            |

## G. Security and reliability assessment

### Verified Phase 2 protections

- **Session revocation:** 30-second cache; no stale authorization; identity failure → 503; revoked
  sessions → 401; realtime closes unconfirmed sockets with 1013; notification-worker admin API
  checks session state.
- **Webhook secrets:** active key + decryption-only previous keys; `v2:<keyId>:...`; legacy `v1`
  readable and read-repaired with AAD; compare-and-swap avoids overwriting a concurrent rotation;
  wrong key/tag/AAD/cross-merchant use fails.
- **CSRF/CORS:** cross-site `Sec-Fetch-Site` rejected; Origin checked behind the gateway; loopback
  dev proxy allowed narrowly; no wildcard credential CORS.
- **CSP/headers:** CSP, COOP, Permissions-Policy and standard headers configured at Nginx; Chromium
  runtime evidence exists in the Phase 2 record.
- **Internal APIs:** shared internal token and gateway `/internal` block; request IDs required.
- **SSRF:** webhook URL validation and dial-time IP blocking.
- **Upload security:** decoded image validation, 5 MiB limit, assignment/quota checks, object key
  validation, private storage.
- **Secrets/PII:** no browser token storage; no secrets in tracked files; logs redact sensitive
  values; admin notification recipients masked.
- **SQL injection:** queries are parameterized; dynamic sort/filter fragments are resolved from
  fixed values or placeholders.
- **Metrics cardinality:** UUID, hex and tracking-number segments are normalized.

No confirmed exploitable critical or high-severity application vulnerability was found in this
review. The high-severity findings below are operational/quality gaps, not demonstrated exploits.

### Reliability findings

- **Worker readiness gap:** notification-worker `/ready` checks PostgreSQL only; webhook-worker
  health is process-based; merchant sync and parcel outbox relay are not represented in `/ready`.
  A broker outage can leave workers "healthy" while work accumulates.
- **Realtime identity dependency:** `/ready` checks database and consumers, not identity session
  state; authenticated sockets fail closed while readiness can remain ready.
- **AI readiness:** `/ready` returns 200 with a provider status even when the provider is
  unconfigured and chat will return 503.
- **Token retention:** identity token tables grow without cleanup.
- **Order reservations:** retention is unbounded.
- **Baseline CI flake:** current required gate cannot be treated as reliably green.

### Accepted limitations

- Shared rate limiting and shared AI conversation state are process-local.
- Realtime multi-replica fan-out, replay and SSE are absent.
- Local proof storage requires one replica or a shared volume.
- Distributed tracing, Prometheus/Grafana and alerting are not deployed.
- SMS delivery has no provider.
- CSP requires `'unsafe-inline'` for Next hydration and React styles.
- Google OIDC does not validate a nonce/`azp` (no nonce is sent; provider signature/issuer/audience/
  expiry are validated).
- Production TLS, Kubernetes and Terraform are deferred.

## H. Performance and deployment readiness

### Performance evidence

- **No k6, load-test, benchmark or throughput evidence exists.**
- The only query performance evidence is an outbox/workload `EXPLAIN (ANALYZE, BUFFERS)` note in
  `migrations/README.md`.
- AI provider latency measurements exist in `docs/AI_ASSISTANT_FINAL_AUDIT.md`; they show variable
  provider latency and provider-side 504s, not platform throughput.
- No measured limits for auth, parcel creation, tracking, pricing, merchant orders, realtime
  connections, consumer throughput or outbox drain rate.

### Deployment readiness

- Compose validates and the full stack reached healthy.
- Dockerfiles are multi-stage, non-root, static Go / Next standalone.
- Healthchecks exist for 14 services; workers have stop grace periods.
- Migrations are explicit operator steps, never service startup.
- Nginx waits for healthy upstreams and supports WebSocket upgrades.
- Published host ports are limited to Nginx, RabbitMQ and Mailpit in local Compose.
- Production workflow builds/pushes GHCR images and applies Neon migrations.
- No production TLS, Kubernetes, Terraform or runtime deployment manifests exist.
- No Compose resource limits/restart policy for production-like operation.
- No monitoring scrape/alert deployment despite metrics endpoints.

## I. Documentation assessment

| Document                                                              | Issue                                                                                                           | Recommended change                                         |
| --------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------- |
| `README.md:493`                                                       | Says `/api/v1/ai` is a TODO returning 404; it is implemented and routed.                                        | Replace with the implemented AI route note.                |
| `README.md:501`                                                       | Says profiles are still planned; `GET/PATCH /users/me` and password change are implemented.                     | Update auth scope.                                         |
| `README.md:718`                                                       | Says `/ready` does not check broker consumers and no business screen consumes `useLiveUpdates`; both are false. | Update realtime/readiness and consumer note.               |
| `README.md:722`                                                       | Lists the pricing administration UI and live-stack delivery scenarios as not implemented; both exist.           | Reconcile the not-implemented list.                        |
| `README.md:5`                                                         | Ambiguous "dispatch status writes" planned; courier status writes exist.                                        | Clarify dispatcher vs courier writes.                      |
| `README.md` config paragraph                                          | Claims `.env.example` documents every variable, but rotation variables are not wired into Compose.              | Document Compose mapping (or add it in Phase 3).           |
| `docs/architecture.md:40`                                             | Says identity profile endpoints are planned; implemented.                                                       | Update service boundary.                                   |
| `docs/architecture.md:42`                                             | Says pricing is a future phase with no formulas; fully implemented.                                             | Replace with current pricing ownership.                    |
| `docs/architecture.md:41`                                             | Says remaining parcel lifecycle writes are planned; proof is implemented. Verify which writes remain.           | Narrow the statement.                                      |
| `docs/architecture.md:75`                                             | Correctly admits no runtime JSON Schema validation; keep but track as Phase 3 contract gap.                     | Add remediation task.                                      |
| `contracts/openapi/realtime.yaml`                                     | Future-only health/readiness spec; no `/ws` or message contract.                                                | Rewrite from implemented realtime behaviour.               |
| `contracts/openapi/parcel.yaml:13`                                    | Says proof uploads are planned; implemented.                                                                    | Correct description.                                       |
| `contracts/openapi/README.md`                                         | Says other business endpoints will be added; most are already present.                                          | Update status/validation instructions.                     |
| `docs/audits/SWIFTDROP_*.md`                                          | Dated 2026-11-05 at old commit `bf69119`; report many now-implemented features as not implemented.              | Mark historical/archive and remove from current guidance.  |
| `docs/implementation/registration-auth-completion-report.md:302`      | Says merchant order/webhook APIs are not implemented; implemented.                                              | Mark historical or update.                                 |
| `docs/m3-consistency-review.md`                                       | Says email verification is not implemented; it is.                                                              | Mark historical/superseded.                                |
| `docs/pricing-and-tracking.md:61`                                     | Says M5 parcel-creation consumer is not implemented; it is.                                                     | Update or mark historical.                                 |
| `docs/report/section_2_4`, `section_2_5`, `M6_DISPATCH_DESIGN_REPORT` | Say courier/dispatch endpoints are planned; implemented.                                                        | Keep as historical milestone sources with a status banner. |
| Phase 1 completion record                                             | No dedicated Phase 1 completion/verification document exists; Phase 2 has one.                                  | Add a concise Phase 1 record or link the merge/evidence.   |
| `docs/phase-2-security-reliability.md`                                | Current and accurate; keep as the Phase 2 source of truth.                                                      | Maintain with changes.                                     |
| `docs/operations.md`                                                  | Metrics/security runbook exists; retention and worker readiness gaps not covered.                               | Add in Phase 3.                                            |

## J. Prioritized findings register

Severity: Critical, High, Medium, Low. Classification: confirmed defect, missing test coverage,
documentation gap, reliability risk, performance concern, accepted architectural limitation, future
enhancement.

| ID   | Component                       | Description                                                                                                                                                        | Evidence                                                                                                  | Severity | Business impact                                                                               | Recommended action                                                                  | Dependencies              | Suggested owner   | Acceptance criteria                                                           | Classification                           |
| ---- | ------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------------------------------------------------------------------------------------------- | -------- | --------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | ------------------------- | ----------------- | ----------------------------------------------------------------------------- | ---------------------------------------- |
| F-01 | Merchant integration tests / CI | Required CI gate on `development` is red from a nondeterministic business-registration test value                                                                  | CI run `43612384956`; test fails when `merchantID[:8]` has no digit; `businessreg.Valid` requires a digit | High     | Every PR baseline is unreliable; required gate can fail without a product regression          | Make test data always contain a digit; rerun the database job                       | None                      | M8 + M1           | Required gate consistently green across reruns                                | Confirmed defect                         |
| F-02 | Compose / Merchant              | `WEBHOOK_SECRET_KEY_ID` and `WEBHOOK_SECRET_KEYS` are documented but not passed to the Merchant container                                                          | `docker-compose.yml:303-318`; `.env.example:394-401`                                                      | High     | Rotation cannot be configured in the default stack; old/new key operation is incomplete       | Add the variables to the Merchant service environment and test rotation in Compose  | F-01                      | M8 + M1           | Rotation works through Compose with old ciphertext readable                   | Confirmed defect                         |
| F-03 | Realtime contract               | OpenAPI describes realtime as future and omits `/ws`                                                                                                               | `contracts/openapi/realtime.yaml:1-12`; actual `/ws` implemented                                          | High     | Contract consumers and reviewers cannot implement/test realtime correctly                     | Author the implemented WebSocket/message contract; add validation                   | F-09                      | M10 + M1          | Spec covers auth, subscribe, ping/pong, status/assignment messages, readiness | Documentation gap / contract drift       |
| F-04 | Root README                     | Multiple false/stale current-scope statements                                                                                                                      | `README.md:5,493,501,718,722`                                                                             | High     | Onboarding, assessment, and Phase 3 scoping can start from false assumptions                  | Reconcile README with AGENTS/implementation                                         | None                      | M1 + owners       | README statements match current code                                          | Documentation gap                        |
| F-05 | Architecture docs               | Service boundaries claim identity profiles are planned and pricing is unimplemented                                                                                | `docs/architecture.md:40-42`                                                                              | High     | Misleads architecture and security review                                                     | Update service boundaries and add historical banners to old reports                 | None                      | M1 + M2/M4        | Architecture matches implemented services                                     | Documentation gap                        |
| F-06 | CI                              | The real-stack E2E filter omits parcel, realtime, notification-worker, webhook-worker and AI service paths; access tests already run for any Docker service change | `.github/workflows/ci.yml` `changes` filters                                                              | Medium   | Workflow regressions in those services may merge without the real-stack journey               | Expand the realstack filter; add a path-filter regression test                      | F-01                      | M1 + M10          | Relevant service changes always trigger real-stack checks                     | Missing test coverage                    |
| F-07 | Performance                     | No load tests or throughput/latency baseline                                                                                                                       | No k6/benchmark files; only one EXPLAIN note                                                              | High     | No evidence for login, tracking, parcel creation, pricing, realtime or outbox capacity        | Add k6 scenarios and a documented local baseline                                    | Stable stack, seeded data | M1 + M4/M5/M6/M10 | Documented p95/error/throughput baselines for agreed paths                    | Performance concern                      |
| F-08 | Workers / outbox                | Readiness does not represent broker/consumer health outside realtime                                                                                               | notification `/ready` DB-only; webhook health process-only; merchant/parcel readiness DB-only             | High     | Broker outages can leave workers "healthy" while queues backlog or delivery stops             | Add consumer health to worker readiness and expose outbox/relay readiness or gauges | `messaging.Health`        | M1 + M7/M8/M9     | `/ready` fails when a required consumer is detached; alerts/metrics available | Reliability risk                         |
| F-09 | Contracts / CI                  | No OpenAPI parsing, linting or route-drift test                                                                                                                    | `contracts/openapi/README.md`; CI jobs                                                                    | Medium   | Contract drift (already visible in realtime/parcel) can merge silently                        | Add OpenAPI validation and route coverage test                                      | Contract ownership        | M1 + M10          | CI fails on invalid/drifted OpenAPI                                           | Missing test coverage                    |
| F-10 | Messaging                       | Event payloads are not validated against JSON Schema at consumer boundaries                                                                                        | `docs/architecture.md:75`                                                                                 | Medium   | Invalid producer payloads can reach handlers; failure classification may be late/inconsistent | Add optional schema validation at consumer boundary with permanent-failure mapping  | F-09                      | M1 + M6/M7/M8     | Invalid schemas dead-letter without retries; valid events pass                | Reliability risk                         |
| F-11 | Access tests                    | Access matrix covers 26 representative routes, not all protected routes                                                                                            | `tests/access/matrix_test.go`                                                                             | Medium   | Some authorization regressions may not be caught end-to-end                                   | Expand matrix for profile, webhook, orders/keys, proof photo, reports, admin routes | Compose                   | M2/M8/M10         | Every protected route has a role case                                         | Missing test coverage                    |
| F-12 | Frontend E2E                    | Admin users, pricing admin and reports UI are mocked only                                                                                                          | `apps/web-e2e/src/*.spec.ts`; no matching real specs                                                      | Medium   | UI/API integration regressions can escape                                                     | Add real-stack journeys for the three screens                                       | Real-stack seed           | M2/M4/M10         | Real-stack CRUD/read journeys pass                                            | Missing test coverage                    |
| F-13 | AI assistant                    | Live Gemini path is not exercised in CI                                                                                                                            | AI docs and CI                                                                                            | Medium   | Provider/model changes can break real chat/tool rounds                                        | Add a gated live smoke job or contract test                                         | Provider key              | M9/M1             | Opt-in live smoke passes; CI remains deterministic without key                | Missing test coverage                    |
| F-14 | Playwright                      | Full local suite is timing-sensitive; failure sets differ                                                                                                          | `docs/phase-2-security-reliability.md` verification                                                       | Medium   | Local verification results are unreliable                                                     | Stabilize selectors/waits or isolate resource-heavy tests                           | None                      | M10               | Repeated local runs produce the same result                                   | Reliability risk                         |
| F-15 | Identity data                   | Refresh/reset/email-verification tokens have no retention cleanup                                                                                                  | Repository SQL inspection                                                                                 | Medium   | Unbounded table growth; old rows retain metadata                                              | Add bounded cleanup jobs and tests                                                  | None                      | M2                | Cleanup deletes expired rows on schedule; tests pass                          | Reliability risk                         |
| F-16 | Merchant data                   | Order reservation rows have no retention policy                                                                                                                    | Migration/repository inspection                                                                           | Medium   | Unbounded growth                                                                              | Add retention for completed/failed rows while preserving idempotency window         | None                      | M8                | Cleanup tested; recent retries still idempotent                               | Reliability risk                         |
| F-17 | Notification data               | Raw recipient stored in delivery logs; retention/access policy not documented                                                                                      | `docs/db.sql` notification logs; admin API masks                                                          | Medium   | Privacy/data-retention exposure                                                               | Document retention/access and add cleanup policy if required                        | F-15                      | M7/M9             | Documented retention and access; implementation matches                       | Reliability/privacy risk                 |
| F-18 | Readiness semantics             | AI readiness 200 when unconfigured; realtime/pricing readiness omit identity dependency                                                                            | `ai-assistant/internal/http/router.go`; realtime/pricing readiness                                        | Medium   | Orchestrators can route to a service that cannot serve authenticated/AI work                  | Document readiness contract or add dependency checks where appropriate              | F-08                      | M1 + M9/M10       | Readiness reflects agreed required capabilities                               | Reliability risk                         |
| F-19 | Historical docs                 | Old audit/report files contradict current implementation                                                                                                           | `docs/audits/*`; `docs/implementation/*`                                                                  | Medium   | New agents/readers may repeat completed work or misdiagnose gaps                              | Add historical banners and archive out of current guidance                          | F-04/F-05                 | M1                | Historical docs clearly marked; current docs authoritative                    | Documentation gap                        |
| F-20 | Pricing docs                    | Says M5 parcel-creation consumer is not implemented                                                                                                                | `docs/pricing-and-tracking.md:61`                                                                         | Medium   | Misleads integration planning                                                                 | Update or mark historical                                                           | None                      | M4                | Document matches current flow                                                 | Documentation gap                        |
| F-21 | Parcel contract                 | OpenAPI says proof uploads are planned                                                                                                                             | `contracts/openapi/parcel.yaml:13`                                                                        | Medium   | Contract readers underestimate implemented capability                                         | Correct the description                                                             | F-09                      | M5/M10            | Spec matches implementation                                                   | Documentation gap                        |
| F-22 | Phase records                   | No dedicated Phase 1 completion record                                                                                                                             | `docs/` inventory                                                                                         | Medium   | Audit trail incomplete                                                                        | Add concise Phase 1 record/links                                                    | None                      | M1                | Phase 1 evidence is discoverable                                              | Documentation gap                        |
| F-23 | Compose                         | No resource limits/restart policy for production-like operation                                                                                                    | `docker-compose.yml`                                                                                      | Medium   | Not production-hardened; acceptable for local only                                            | Document as local-only; add limits if Compose becomes deployment target             | None                      | M1                | Limitation explicit or limits added                                           | Deployment risk                          |
| F-24 | Google OIDC                     | No nonce/`azp` validation                                                                                                                                          | `internal/google/google.go`; report note                                                                  | Low      | Defense-in-depth gap; current flow uses PKCE and verifies signature/issuer/audience/expiry    | Document decision or add nonce/azp validation                                       | Provider config           | M3                | Explicit documented decision                                                  | Accepted limitation / future enhancement |
| F-25 | CSP                             | `script-src`/`style-src` use `'unsafe-inline'`                                                                                                                     | `infra/nginx/default.conf`                                                                                | Low      | Weakens XSS mitigation                                                                        | Move to nonce-based script CSP when App Router rendering allows                     | Frontend work             | M10               | CSP without unsafe-inline for scripts; app works                              | Accepted limitation                      |
| F-26 | Scaling                         | Rate limiting and AI conversation state are process-local                                                                                                          | `docs/phase-2-security-reliability.md`                                                                    | Low      | Limits/state diverge across replicas                                                          | Architecture decision for Redis or PostgreSQL state                                 | Approval                  | M1/M2/M9          | Documented single-replica deployment or shared state implemented              | Accepted architectural limitation        |
| F-27 | Realtime                        | No multi-replica fan-out/replay/SSE                                                                                                                                | Architecture docs                                                                                         | Low      | Each event reaches one replica                                                                | Shared pub/sub or per-replica queue design                                          | Approval                  | M10               | Documented limitation or implemented fan-out                                  | Accepted architectural limitation        |
| F-28 | Proof storage                   | Local provider unsafe for independent multi-host replicas                                                                                                          | Architecture docs                                                                                         | Low      | Missing proofs across hosts                                                                   | Use shared volume or S3-compatible provider                                         | Ops                       | M7/M1             | Multi-host deployment uses shared storage                                     | Accepted architectural limitation        |
| F-29 | Observability                   | No tracing, Prometheus/Grafana, scrape config or alerts                                                                                                            | Phase 2 docs                                                                                              | Low      | Outage diagnosis is log-only                                                                  | Add monitoring stack when budget permits                                            | Approval                  | M1                | Metrics scraped and alerts configured                                         | Accepted/future enhancement              |
| F-30 | Notifications                   | SMS not implemented                                                                                                                                                | Docs/worker behaviour                                                                                     | Low      | Only email channel available                                                                  | Add provider when in scope                                                          | Approval                  | M7/M9             | SMS delivered or clearly out of scope                                         | Accepted limitation                      |
| F-31 | Migrations                      | Rollback coverage is partial (parcel/notification baseline)                                                                                                        | `migrations/README.md`                                                                                    | Low      | Rollback risk in other services                                                               | Add isolated rollback tests per service                                             | Test DB                   | M5 owners         | Every service rollback path tested                                            | Missing test coverage                    |
| F-32 | Operations                      | No automated backup/restore verification                                                                                                                           | Scripts exist; no test                                                                                    | Low      | Recovery confidence lower than backups suggest                                                | Add disposable restore test                                                         | Ops                       | M1                | Restore rehearsal documented/tested                                           | Reliability risk                         |
| F-33 | API client                      | No generated client from OpenAPI                                                                                                                                   | `contracts/openapi/README.md`; frontend manual client                                                     | Low      | Drift risk                                                                                    | Add codegen or contract tests                                                       | F-09                      | M10               | Client/schema drift detected                                                  | Future enhancement                       |
| F-34 | Monitoring                      | Metrics exported but not scraped                                                                                                                                   | Phase 2 docs                                                                                              | Low      | Metrics provide little operational value                                                      | Add scrape/alert config when deployment allows                                      | F-29                      | M1                | Metrics visible and alerting tested                                           | Future enhancement                       |

## K. Phase 3 execution roadmap

The roadmap separates **essential** work from optional enhancements. Do not start optional
infrastructure work before the essential baseline is green.

### Workstream 1 — Functional gap remediation (essential)

| Priority | Component           | Problem                             | Proposed solution                                                                 | Dependencies | Deliverable                    | Test requirements                  | Acceptance criteria                             | Owner       |
| -------- | ------------------- | ----------------------------------- | --------------------------------------------------------------------------------- | ------------ | ------------------------------ | ---------------------------------- | ----------------------------------------------- | ----------- |
| P0       | Merchant tests / CI | Flaky business-registration fixture | Add a guaranteed digit to generated registration numbers; audit all helpers       | None         | Stable test data               | Repeated database job runs         | Gate green across reruns                        | M8 + M1     |
| P0       | Compose / Merchant  | Rotation env not wired              | Pass `WEBHOOK_SECRET_KEY_ID` and `WEBHOOK_SECRET_KEYS`; add Compose rotation test | F-01         | Rotation-capable Compose stack | Encrypt/decrypt/rotate integration | Old ciphertext readable; new writes use new key | M8 + M1     |
| P1       | Documentation       | False current-scope statements      | Reconcile README/architecture with code                                           | None         | Correct docs                   | Doc review                         | No contradictory current statements             | M1 + owners |

### Workstream 2 — End-to-end business workflow testing (essential)

| Priority | Component      | Problem                                    | Proposed solution                                           | Dependencies | Deliverable             | Test requirements     | Acceptance criteria           | Owner             |
| -------- | -------------- | ------------------------------------------ | ----------------------------------------------------------- | ------------ | ----------------------- | --------------------- | ----------------------------- | ----------------- |
| P1       | Real-stack E2E | Admin users/pricing/reports UI mocked only | Add real-stack journeys                                     | Seed data    | 3 new specs             | CI real-stack         | Journeys pass                 | M10 + M2/M4       |
| P1       | Access matrix  | Incomplete protected-route coverage        | Expand role/ownership cases                                 | Compose      | Extended matrix         | Access CI             | Every protected route checked | M2/M8/M10         |
| P2       | Load tests     | No capacity evidence                       | Add k6 scenarios for auth/tracking/estimate/parcel/consumer | Stable stack | Load scripts + baseline | Local/CI optional job | Documented baselines          | M1 + M4/M5/M6/M10 |

### Workstream 3 — API and event contract validation (essential)

| Priority | Component         | Problem                       | Proposed solution                                          | Dependencies       | Deliverable             | Test requirements | Acceptance criteria          | Owner         |
| -------- | ----------------- | ----------------------------- | ---------------------------------------------------------- | ------------------ | ----------------------- | ----------------- | ---------------------------- | ------------- |
| P0       | Realtime contract | `/ws` undocumented            | Write implemented spec                                     | None               | Updated `realtime.yaml` | Schema/lint       | Contract matches code        | M10 + M1      |
| P1       | OpenAPI CI        | No validation/drift detection | Add parser/linter and route coverage test                  | Contract ownership | CI job                  | CI                | Invalid/drifted spec fails   | M1 + M10      |
| P1       | Events            | No runtime schema validation  | Add opt-in schema validation and permanent-failure mapping | F-09               | Consumer validation     | Integration tests | Invalid payloads dead-letter | M1 + M6/M7/M8 |
| P2       | Parcel contract   | Stale proof description       | Correct OpenAPI                                            | F-09               | Updated spec            | Lint              | Matches code                 | M5/M10        |

### Workstream 4 — Database integrity and migration QA (essential)

| Priority | Component    | Problem                          | Proposed solution                                                 | Dependencies | Deliverable             | Test requirements | Acceptance criteria                    | Owner         |
| -------- | ------------ | -------------------------------- | ----------------------------------------------------------------- | ------------ | ----------------------- | ----------------- | -------------------------------------- | ------------- |
| P1       | Identity     | No token retention               | Add bounded cleanup for expired refresh/reset/verification tokens | None         | Cleanup job + tests     | Integration       | Old rows removed; active rows retained | M2            |
| P2       | Merchant     | Reservation retention unbounded  | Define retention for completed/failed reservations                | None         | Cleanup job + tests     | Integration       | Growth bounded; retry window preserved | M8            |
| P2       | Notification | Recipient retention undocumented | Define retention/access; add cleanup if required                  | F-15         | Policy + implementation | Integration       | Documented and enforced                | M7/M9         |
| P2       | All schemas  | Partial rollback coverage        | Add isolated rollback tests per service                           | Test DB      | Tests                   | CI                | Each service rollback verified         | Schema owners |

### Workstream 5 — Messaging and cross-service integration testing (essential)

| Priority | Component      | Problem                          | Proposed solution                                                                   | Dependencies | Deliverable               | Test requirements            | Acceptance criteria              | Owner         |
| -------- | -------------- | -------------------------------- | ----------------------------------------------------------------------------------- | ------------ | ------------------------- | ---------------------------- | -------------------------------- | ------------- |
| P0       | Workers/outbox | Broker/consumer health invisible | Add `messaging.Health` to worker readiness; expose relay/outbox readiness or gauges | F-08         | Readiness/metrics updates | Unit + Compose failure tests | `/ready` reflects broker session | M1 + M7/M8/M9 |
| P1       | Event flow     | No CI broker-loss recovery test  | Add Compose failure/recovery scenario                                               | F-08         | CI job                    | E2E                          | Readiness/gauges recover         | M10 + M1      |
| P1       | Event payloads | Runtime schema gap               | See Workstream 3                                                                    | F-09         | Validation                | Integration                  | Invalid payloads dead-letter     | M1 + owners   |

### Workstream 6 — Frontend integration and quality (essential)

| Priority | Component            | Problem               | Proposed solution                                | Dependencies | Deliverable   | Test requirements | Acceptance criteria        | Owner       |
| -------- | -------------------- | --------------------- | ------------------------------------------------ | ------------ | ------------- | ----------------- | -------------------------- | ----------- |
| P1       | Admin UI             | Mocked-only journeys  | Real-stack specs for admin users/pricing/reports | Seed         | Specs         | CI                | Pass                       | M10 + M2/M4 |
| P2       | Playwright stability | Local flakiness       | Stabilize waits/selectors; isolate heavy tests   | None         | Updated specs | Repeated runs     | Stable local runs          | M10         |
| P2       | CSP testing          | No browser regression | Add header/CSP check to E2E                      | None         | E2E assertion | CI                | Headers and page load pass | M10         |

### Workstream 7 — Security regression testing (essential)

| Priority | Component              | Problem                                       | Proposed solution           | Dependencies | Deliverable   | Test requirements | Acceptance criteria        | Owner  |
| -------- | ---------------------- | --------------------------------------------- | --------------------------- | ------------ | ------------- | ----------------- | -------------------------- | ------ |
| P1       | CI real-stack triggers | Real-stack filter skips several service paths | Expand the realstack filter | F-06         | CI update     | Path-filter tests | Relevant changes trigger   | M1     |
| P2       | Webhook rotation       | Compose gap                                   | See Workstream 1            | F-01         | Compose test  | Integration       | Rotation verified          | M8     |
| P2       | CSP/CSRF               | Unit-only browser coverage                    | Add browser checks          | None         | E2E           | CI                | Security headers/flow pass | M10/M2 |
| P3       | Google OIDC            | no nonce/azp                                  | Decide and document         | Provider     | Decision/test | Provider smoke    | Documented control         | M3     |

### Workstream 8 — Performance and reliability validation (essential for Phase 3 acceptance)

| Priority | Component       | Problem                     | Proposed solution                          | Dependencies   | Deliverable      | Test requirements    | Acceptance criteria           | Owner          |
| -------- | --------------- | --------------------------- | ------------------------------------------ | -------------- | ---------------- | -------------------- | ----------------------------- | -------------- |
| P1       | API load        | No baseline                 | k6 auth/tracking/estimate/parcel scenarios | Stable stack   | Scripts + report | Local documented run | Baselines and limits recorded | M1 + M4/M5/M10 |
| P1       | Consumer/outbox | No throughput evidence      | Measure relay and consumer processing      | Seed/load data | Report           | Integration/load     | Bottlenecks documented        | M1 + M6/M7     |
| P2       | Realtime        | No connection-load evidence | WebSocket connection/message test          | Stable stack   | Script/report    | Local                | Connection limits documented  | M10            |

### Workstream 9 — Deployment readiness (essential)

| Priority | Component    | Problem                              | Proposed solution                            | Dependencies | Deliverable         | Test requirements | Acceptance criteria       | Owner |
| -------- | ------------ | ------------------------------------ | -------------------------------------------- | ------------ | ------------------- | ----------------- | ------------------------- | ----- |
| P0       | CI baseline  | Required gate red                    | Fix F-01 and confirm green on development    | None         | Green required gate | CI                | Gate success              | M1/M8 |
| P1       | Compose docs | Rotation/readiness gaps              | Document local-only limits and required envs | F-02/F-08    | Updated docs        | Doc review        | Accurate                  | M1    |
| P1       | Operations   | Retention/readiness runbooks missing | Add runbook sections                         | F-15/F-08    | Operations doc      | Review            | On-call usable            | M1    |
| P3       | Production   | No TLS/K8s/Terraform                 | Keep deferred; document explicit limitation  | Approval     | Status doc          | Docs              | No false production claim | M1    |

### Workstream 10 — Documentation completion (essential)

| Priority | Component           | Problem                    | Proposed solution                          | Dependencies | Deliverable   | Test requirements | Acceptance criteria        | Owner       |
| -------- | ------------------- | -------------------------- | ------------------------------------------ | ------------ | ------------- | ----------------- | -------------------------- | ----------- |
| P0       | README/architecture | Stale current-scope claims | Reconcile with implemented state           | None         | Updated docs  | Review            | No false statements        | M1 + owners |
| P0       | Historical docs     | Contradict current state   | Add historical banners/archive old reports | None         | Marked docs   | Review            | Current docs authoritative | M1          |
| P1       | Phase records       | No Phase 1 record          | Add completion/evidence record             | None         | Phase 1 doc   | Review            | Traceable                  | M1          |
| P1       | Contract docs       | Realtime/parcel drift      | Update contracts and README                | F-03/F-21    | Updated specs | Lint/tests        | Match code                 | M10/M1      |

### Essential vs optional

**Essential before Phase 3 feature work:**

- F-01 CI flake and green required gate.
- F-02 Compose rotation wiring.
- F-03/F-04/F-05/F-19/F-20/F-21/F-22 documentation and contract reconciliation.
- F-06 CI path-filter correction.
- F-08 worker/outbox readiness and failure tests.
- F-09/F-10 contract validation.
- F-11/F-12 access and real-stack coverage.
- F-15/F-16/F-17 data retention.
- F-07 load/performance baseline.
- F-18 readiness semantics decision.

**Optional/future:**

- Shared rate limiting and AI conversation state.
- Realtime multi-replica fan-out/replay/SSE.
- Prometheus/Grafana/Alertmanager and distributed tracing.
- Nonce-based CSP.
- SMS provider.
- Google nonce/azp.
- Kubernetes/Terraform/production TLS.
- OpenAPI code generation and automated backup/restore.

## L. Recommended M1–M10 allocation

Assignments are recommendations and require team-lead confirmation. Do not overlap shared package,
contract, migration or CI edits without coordination.

| Member | Workstream                              | Tasks                                                                                                                                                                   | Dependencies           | Deliverables                                      | Acceptance criteria                                                              |
| ------ | --------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- | ------------------------------------------------- | -------------------------------------------------------------------------------- |
| M1     | Platform, CI, observability, deployment | Fix CI path filters; OpenAPI validation; worker readiness shared package; load-test harness; README/architecture reconciliation; historical-doc banners; Phase 1 record | F-01, F-06, F-08, F-09 | CI updates, readiness/metrics, docs, load scripts | Gate green; filters tested; worker readiness fails on broker loss; docs accurate |
| M2     | Identity/auth/security                  | Token retention cleanup; access-matrix expansion for profile/admin; real-stack admin users; session/CSRF regression                                                     | F-11, F-12, F-15       | Cleanup job/tests, specs, docs                    | Cleanup tested; admin journey passes; auth matrix complete                       |
| M3     | Google/OAuth/security                   | Decide nonce/azp; live provider smoke (gated); auth docs                                                                                                                | Provider config        | Decision/test/docs                                | Explicit control; optional smoke passes when enabled                             |
| M4     | Pricing/public tracking                 | Real-stack pricing admin; pricing load/contract tests; pricing docs                                                                                                     | Seed, F-07, F-12       | Specs, load scenarios, docs                       | Admin journey and baseline pass                                                  |
| M5     | Parcel/data                             | Parcel contract correction; load scenarios; rollback tests; customer workflow coverage                                                                                  | F-07, F-11             | Contract/load/tests                               | Specs match; baselines recorded                                                  |
| M6     | Dispatch/state machine/outbox           | Outbox relay readiness/metrics; broker failure tests; event payload validation                                                                                          | F-08, F-09             | Readiness, CI test, schema guard                  | Relay/consumer failures observable                                               |
| M7     | Courier/proofs/notification             | Proof storage multi-host guidance; notification retention; worker readiness; SMS decision                                                                               | F-08, F-17             | Docs, cleanup, readiness                          | Storage/retention documented; worker readiness correct                           |
| M8     | Merchant/orders/webhooks                | Fix flaky merchant fixture; Compose rotation env; reservation retention; webhook rotation test                                                                          | F-01, F-02, F-16       | Green integration suite, Compose rotation test    | Gate stable; rotation works                                                      |
| M9     | Notifications/AI                        | Notification E2E breadth; AI live smoke (gated); AI readiness docs; historical doc updates                                                                              | F-13, F-18             | Specs/docs                                        | Deterministic CI + opt-in live smoke                                             |
| M10    | Realtime/reports/frontend E2E           | Realtime OpenAPI; real-stack reports/admin UI; Playwright stability; CSP browser check                                                                                  | F-03, F-12, F-14       | Specs, contracts, stable suite                    | Contract/E2E pass consistently                                                   |

## M. Audit completion criteria

| Criterion                            | Status                                                                 |
| ------------------------------------ | ---------------------------------------------------------------------- |
| Latest development baseline verified | Pass — `686a03d`, Phase 1/2 merged, `development` not advanced         |
| Architecture understood              | Pass — service/package/schema/contract/messaging inventory             |
| Functional completeness assessed     | Pass — matrix above                                                    |
| Critical workflows traced            | Pass — 12 workflow matrix                                              |
| API/event contracts reviewed         | Pass with findings (realtime/parcel drift, no OpenAPI CI)              |
| Database integrity assessed          | Pass with retention/rollback findings                                  |
| Messaging flows examined             | Pass with readiness/schema-validation findings                         |
| Tests inventoried                    | Pass — 267 Go, 50 Playwright, 11 Vitest, 69 integration-tagged files   |
| Security regressions assessed        | Pass — Phase 2 controls verified; no exploitable critical/high finding |
| Performance evidence reviewed        | Pass — no load tests; query/AI latency only                            |
| Deployment readiness assessed        | Pass — Compose healthy; production limitations documented              |
| Documentation accuracy evaluated     | Pass — findings table                                                  |
| Findings prioritized                 | Pass — F-01 to F-34                                                    |
| Phase 3 roadmap prepared             | Pass — workstreams 1-10                                                |
| Team allocation recommended          | Pass — M1-M10 matrix                                                   |

## N. Audit evidence and limitations

Commands executed during this audit (non-destructive):

- `git status`, `git fetch --all --prune --tags`, `git log`, merge verification.
- `gh pr view 154/155`, `gh run list`, `gh api .../check-runs`.
- Source, migration, contract, CI, Compose, Nginx and documentation inspection.
- `go test ./...` — pass.
- `node --test "tests/contracts/*.test.mjs"` — 11 pass.
- `pnpm exec nx test @org/web`, `typecheck`, `lint` — pass.
- `docker compose config` — pass.
- `make migrate-status` — identity 6, pricing 2, parcel 7, merchant 11, notification 2.
- `docker compose ps` — all services healthy.

Not executed in this audit:

- Full PostgreSQL/RabbitMQ integration suites (CI evidence inspected instead).
- Playwright suites (CI and Phase 2 evidence inspected; local flakiness documented).
- Load/performance tests (none exist).
- Destructive migration rollback on the shared database.

The primary current-state blocker is not a missing feature; it is the red required CI baseline
caused by a nondeterministic test. Phase 3 should start by making the baseline reliably green,
then reconcile documentation/contracts, close the Compose rotation gap, and add the missing
worker readiness, contract validation, retention and performance evidence.
