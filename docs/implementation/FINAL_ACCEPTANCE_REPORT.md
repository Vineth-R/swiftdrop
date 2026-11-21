# SwiftDrop — Final Acceptance Report

**Date:** 2026-11-20
**Final repository baseline:** `development` at
`682205aa2856fd8f03721979d88a638e1313cd6a` (the PR #181 merge, which contains
PR #180 and PR #179).
**Product verification baseline:** `831455b` (PR #180). PR #181 added regression
tests and documentation only; it changed no product code, API, event, migration
or infrastructure.
**Scope:** approved non-deployment functionality, security, reliability,
integration and documentation.

## 1. Project overview

SwiftDrop is a parcel delivery and courier management platform built as an Nx
monorepo:

- Go HTTP services and RabbitMQ workers.
- Next.js App Router frontend with shadcn/ui and the SwiftDrop design tokens.
- PostgreSQL 16 with one schema per owning service.
- RabbitMQ 4.1 with a durable topic exchange, retry and dead-letter topology.
- Nginx gateway, Docker Compose, and private local/S3-compatible object storage.
- A WebSocket realtime service and a Gemini-backed delivery assistant.

The approved basis is API Documentation v1.1, Database Design Document v1.1, the
Team Implementation Guide and the team task allocation. Production deployment,
TLS, hosting and multi-replica shared state are explicitly outside this scope.

## 2. Final architecture and implemented scope

| Component             | Implemented responsibility                                                                                                                                                                                                                                                           |
| --------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `identity`            | Registration, email verification, login/lockout, refresh rotation and reuse detection, logout, Google OAuth, password reset, profile/password change, ADMIN user management and courier list, internal lookups, ADMIN identity audit read                                            |
| `pricing`             | Public estimate and zones, internal quote/zone lookups, ADMIN configuration and audit, ADMIN pricing audit read                                                                                                                                                                      |
| `parcel`              | Customer creation, owned reads/history/PDF receipt, public and authenticated tracking, DISPATCHER/ADMIN reports, DISPATCHER assignment, COURIER reads and status workflow, pickup/delivery proof, private proof reads, internal merchant creation/summary, outbox relay and recovery |
| `merchant`            | Registration/approval lifecycle, profile and hash-only API keys, orders (8.16–8.18) with idempotency, trusted Parcel caller, status-sync consumer, webhook settings/rotation/test, internal contact and webhook APIs, ADMIN merchant actions/email, ADMIN merchant audit read        |
| `notification-worker` | Customer and merchant parcel email, processed-event idempotency, notification logs, ADMIN read API, processed-event and log retention                                                                                                                                                |
| `webhook-worker`      | Signed public projection, attempt recording, six business retries, suppression for non-approved merchants or inactive webhooks                                                                                                                                                       |
| `realtime`            | `/ws` cookie/token auth, per-parcel and dispatch-all subscriptions, status and assignment consumers, graceful shutdown                                                                                                                                                               |
| `ai-assistant`        | Guest/authenticated chat, exactly four read-only tools, role gate, deterministic extraction guard, safe Markdown, rate/concurrency limits                                                                                                                                            |
| `web`                 | Customer, courier, dispatcher, merchant and ADMIN journeys; public tracking; AI widget; audit and notification-log screens                                                                                                                                                           |

Infrastructure: Compose Postgres, RabbitMQ, Mailpit, services, workers, web and
Nginx; versioned `golang-migrate` migrations; OpenAPI specs, event schemas and
route inventories; Prometheus metrics; backup/restore and rollback rehearsal.

## 3. Completed mandatory requirements

| Area                                                     | Status                                                  | Evidence                                                                                                     |
| -------------------------------------------------------- | ------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------ |
| Customer registration, verification, login/logout, reset | COMPLETE                                                | Unit/handler, repository integration, real-stack Mailpit journeys                                            |
| Google OAuth                                             | COMPLETE                                                | 32 automated tests; provider sign-in page; manual consent and account linking completed by the project owner |
| Profile/password and ADMIN user management               | COMPLETE                                                | Unit/handler, access matrix, real-stack                                                                      |
| Pricing quotes, zones and ADMIN configuration            | COMPLETE                                                | Unit/handler/HTTP integration, k6, access matrix                                                             |
| Customer parcel creation and owned reads                 | COMPLETE                                                | Transactional integration tests, real-stack wizard/list/detail                                               |
| Public and authenticated tracking                        | COMPLETE                                                | Integration, real-stack `/track`, live updates, k6                                                           |
| Dispatcher queue, assignment and reports                 | COMPLETE                                                | Integration, real-stack assignment, reports screen                                                           |
| Courier reads, pickup, delivery and proof                | COMPLETE                                                | Transactional integration, real-stack proof flows, object-storage tests                                      |
| Merchant lifecycle, API keys, orders and sync            | COMPLETE                                                | Unit/HTTP/PostgreSQL integration, real-stack merchant journey                                                |
| Signed webhooks, retries and rotation                    | COMPLETE                                                | Unit plus 3-test real-stack reliability suite                                                                |
| Notifications and idempotency                            | COMPLETE                                                | Dedicated notification flow 18/18, event flow, Mailpit                                                       |
| Realtime                                                 | COMPLETE for the supported single-instance architecture | WebSocket/consumer tests, real-stack live updates                                                            |
| AI assistant                                             | COMPLETE                                                | Unit/orchestration/client/tool tests, live Gemini smoke                                                      |
| ADMIN audit visibility                                   | COMPLETE                                                | Three read APIs, `/admin/audit-logs`, redaction, mocked and real-stack tests                                 |
| Outbox recovery and retention                            | COMPLETE with operator workflow                         | Concurrent replay test, dry-run/apply, retention safety tests                                                |
| Backup/restore and migration cycle                       | COMPLETE                                                | Disposable PostgreSQL 16 rehearsal and up/down/reapply cycle                                                 |

## 4. Core application workflow verification

| Journey       | Verified behaviour                                                                                                                       |
| ------------- | ---------------------------------------------------------------------------------------------------------------------------------------- |
| Customer      | Register → verify email → login → quote → create parcel → track → receive notifications → view delivery proof when authorized            |
| Dispatcher    | Login → unassigned queue → eligible couriers → assign → watch live status/reassignment                                                   |
| Courier       | Login → today/history → pickup with proof → out for delivery → delivery with proof, recipient and timestamps                             |
| Merchant      | Register → ADMIN approve → create/rotate API key → create order idempotently → track/sync → configure and test webhook → read deliveries |
| Administrator | Users and roles → merchant approval/suspension → reports → notification logs → audit logs                                                |
| AI assistant  | Guest and signed-in chat → role-aware tools → denial for unauthorized roles → safe Markdown and fallbacks                                |

These are covered by Go unit/integration tests, the 66-route access matrix, the
real-stack Playwright suite and the dedicated event-flow and notification-flow
suites. The only external-provider step that needs a human is Google account
consent, now recorded as completed.

## 5. Integration history

| PR   | Merge commit                               | Merged at            | Content                                                                                                                         |
| ---- | ------------------------------------------ | -------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| #179 | `eec403842e1f444a0f13a582d6176b63366e22bb` | 2026-11-19T19:08:46Z | Final acceptance fixes: refresh-token logout, rate limits, outbox operations, retention, audit hardening                        |
| #180 | `831455b952e32cbb63623799750720635ddd185b` | 2026-11-19T22:22:53Z | ADMIN audit APIs/UI, bounded retention, concurrent replay test, S3 helper fix, security review, acceptance docs                 |
| #181 | `682205aa2856fd8f03721979d88a638e1313cd6a` | 2026-11-20T04:24:55Z | Retention safety regression tests, notification-flow local prerequisites, post-merge verification evidence, final documentation |

All three PRs passed their required checks and were merged with merge commits
into `development`. No direct commits were made to `development`, and no check
or branch protection was bypassed.

## 6. Backend and frontend verification

| Check                                                           | Result                               |
| --------------------------------------------------------------- | ------------------------------------ |
| `gofmt`, `go build ./...`, `go vet ./...`, `go test ./...`      | PASS                                 |
| Race tests (identity, merchant HTTP, pricing HTTP, objectstore) | PASS                                 |
| Coverage minima                                                 | PASS — identity 60.3%, pricing 70.1% |
| Contract/OpenAPI/route tests                                    | PASS — 32                            |
| Route inventories (`TestRouteInventory`)                        | PASS                                 |
| Frontend unit tests, lint, production build                     | PASS                                 |
| E2E lint and typecheck                                          | PASS                                 |
| Playwright Chromium E2E                                         | PASS (CI)                            |
| 66-route authorization matrix                                   | PASS (PR #180 and post-merge CI)     |
| Real-stack onboarding/core journeys (39 tests)                  | PASS (PR #180 CI)                    |

## 7. Integration and notification verification

| Check                                                                            | Result                                        |
| -------------------------------------------------------------------------------- | --------------------------------------------- |
| Dedicated notification flow (disposable Compose + Mailpit)                       | PASS — 18/18                                  |
| Legacy and domain notification events, duplicates, dead-lettering, SMTP recovery | PASS                                          |
| Event flow with PostgreSQL + RabbitMQ                                            | PASS                                          |
| Webhook reliability (transient retry, six-attempt exhaustion, rotation)          | PASS — 3 real-stack tests                     |
| S3-compatible object storage (upload, access, presign expiry, missing object)    | PASS                                          |
| Migration up/down/reapply                                                        | PASS                                          |
| Backup/restore rehearsal                                                         | PASS                                          |
| k6 public/tracking/pricing                                                       | PASS — 0 failed, p95 ≈ 11 ms                  |
| k6 WebSocket                                                                     | PASS                                          |
| Live Gemini smoke                                                                | PASS — HTTP 200                               |
| Live audit authorization                                                         | PASS — ADMIN 200, CUSTOMER 403, anonymous 401 |

Local notification runs require `SMTP_HOST=mailpit`, empty SMTP credentials and
worker-level `RABBITMQ_RETRY_DELAY`/`RABBITMQ_MAX_RETRIES` before
`docker compose up`. The exact procedure is in
`tests/integration/notificationflow/README.md` and `DEMO_READINESS.md`; CI sets
the same values at job level.

## 8. Google OAuth automated and manual verification

- 32 automated tests pass: 25 service-level tests (state, PKCE S256, browser
  binding, single-use state, callback, account creation and linking, disabled
  accounts, merchant approval, store failures) and 7 provider-helper tests.
- `GET /api/v1/auth/google/login` returns a `302` with PKCE, a random state and
  the configured callback. The generated URL served Google's real sign-in page
  for the configured client ID and redirect URI.
- **Manual verification: PASS.** The project owner completed interactive Google
  consent and verified account linking/callback behaviour. The exact manual
  verification date was not captured in the repository, so none is asserted
  here.
- No authorization code, access token, refresh token or provider secret is
  logged or stored in plaintext.

## 9. Retention and proof lifecycle verification

- Audit logs: 365-day policy. Webhook delivery attempts: 90-day policy.
- Pruning is an operator command, dry-run by default, and requires `APPLY=1`
  (or the Make target) to delete.
- Apply deletes only `identity.audit_logs`, `merchant.audit_logs`,
  `pricing.audit_logs` and `merchant.webhook_delivery_attempts`, in bounded
  batches (`BATCH_SIZE`, `MAX_BATCHES`), and is resumable/idempotent.
- Regression tests prove the dry run emits no `DELETE`, the apply path has
  exactly four `DELETE` statements, batching bounds reach the SQL, and
  sub-30-day windows or non-positive/non-numeric limits are refused.
- Proof objects are private; reads are limited to the owning customer, assigned
  courier, DISPATCHER and ADMIN. Presigned URLs are short-lived. Missing objects
  return 404; `proof-transfer -reconcile` reports orphans and missing objects.
- Committed delivery evidence is never deleted automatically. A future deletion
  policy must be an explicit business/legal decision.
- Automatic audit/webhook retention scheduling remains an accepted limitation;
  the notification worker keeps its own scheduled retention.

## 10. Security review and residual risks

Verified controls include bcrypt password hashing, signed access tokens with
issuer/expiry/role/session version, hashed rotating refresh tokens with reuse
detection, session-family revocation, HttpOnly/SameSite cookies, cross-site
origin rejection, CSP and security headers, per-user/route rate limits,
constant-time internal-token checks, HMAC webhook signing with rotation and
SSRF guards, ownership-scoped parcel/proof reads, parameterized SQL, masked
notification recipients and recursively redacted audit details.

ADMIN audit reads were re-smoked on the final baseline: ADMIN `200`, CUSTOMER
`403`, anonymous `401` for all three sources. No open critical or high-severity
application defect was found in the approved non-deployment scope.

Accepted residual risks (documented, not hidden):

- the shared `INTERNAL_TOKEN` gives any internal service broad internal access;
- no separate CSRF synchronizer token; the same-origin JSON/Origin/SameSite
  posture is the accepted threat-model decision;
- CSP retains `'unsafe-inline'` for Next.js hydration and React style attributes;
- rate-limit, AI conversation/concurrency and realtime hub state are
  process-local, so the supported deployment is a single instance;
- webhook delivery is at-least-once and receivers must deduplicate on `eventId`;
- audit/webhook retention is operator-driven.

## 11. Member-wise completion (M1–M10)

| Member | Responsibility                     | Status                                                                         |
| ------ | ---------------------------------- | ------------------------------------------------------------------------------ |
| M1     | Notifications, events and platform | COMPLETE — notification flow 18/18, event flow, outbox, retention              |
| M2     | Authentication and frontend        | COMPLETE — identity flows, session/logout, frontend screens and tests          |
| M3     | OAuth, password reset and admin    | COMPLETE — Google automated + manual PASS, reset, admin users, identity audit  |
| M4     | Pricing and tracking               | COMPLETE — quotes/zones/admin, public/auth tracking, reports                   |
| M5     | Parcel operations                  | COMPLETE — creation, owned reads, receipts, idempotency, outbox                |
| M6     | Dispatch                           | COMPLETE — eligibility, assignment, history, live notices                      |
| M7     | Courier and delivery proofs        | COMPLETE — pickup/delivery, proof storage/access, reconciliation               |
| M8     | Merchant orders                    | COMPLETE — lifecycle, keys, orders, sync, merchant audit                       |
| M9     | Webhooks                           | COMPLETE — settings, rotation, signed delivery, retries, suppression           |
| M10    | Realtime, reports and testing      | COMPLETE — realtime, reports, access/real-stack/integration/performance suites |

## 12. Requirements traceability

Every mandatory requirement in `REQUIREMENTS_TRACEABILITY_MATRIX.md` is
implemented. The matrix records the implementation, test evidence and
responsible member per requirement. Google OAuth (ID-06) is COMPLETE with both
automated and manual evidence.

No mandatory non-deployment requirement is NOT IMPLEMENTED or BLOCKED. Rows
marked PARTIAL are accepted, documented limitations (single-instance process
state, operator-driven retention and the shared internal-token trust boundary),
not missing implementation.

## 13. Known limitations

- Single-instance operating constraint for process-local realtime, AI and
  rate-limit state.
- At-least-once webhook delivery; receivers must deduplicate on `eventId`.
- Operator-driven audit/webhook retention; no background service scheduler.
- Local filesystem proof storage requires one Parcel replica or a shared volume;
  S3-compatible storage is required for independent replicas.
- No SSE, realtime replay/persistence or multi-replica fan-out.

These are accepted for the approved scope and are not presented as complete
production capabilities.

## 14. Out of scope

- Kubernetes, Terraform, production hosting, TLS termination and DNS.
- Production CI/CD deployment, production migration windows and monitoring or
  distributed-tracing rollout.
- Multi-replica shared state (Redis or equivalent).
- Optional product features: parcel edit/cancel, dispatcher-initiated status
  writes, SMS, in-app inbox, courier self-service availability, account deletion,
  email change, device/session management, payments, merchant live parcel views,
  bulk operations/exports and support ticketing.

## 15. Final academic/demo readiness decision

| Dimension                                  | Decision                                          |
| ------------------------------------------ | ------------------------------------------------- |
| Approved-scope implementation completeness | **COMPLETE**                                      |
| Functional verification completeness       | **COMPLETE**                                      |
| Security review completeness               | **COMPLETE**; no open critical/high defect        |
| Academic submission readiness              | **READY**                                         |
| Demonstration readiness                    | **READY**                                         |
| Production readiness                       | **NOT CLAIMED** — deployment work is out of scope |

SwiftDrop is accepted as functionally complete for the approved non-deployment
academic/demo scope. It is not declared production-ready.

## 16. Actual final Git state

| Item                    | Value                                                       |
| ----------------------- | ----------------------------------------------------------- |
| Final branch            | `development`                                               |
| Final commit            | `682205aa2856fd8f03721979d88a638e1313cd6a`                  |
| Final commit content    | PR #181 merge (tests and documentation on top of `831455b`) |
| Working tree            | clean                                                       |
| PR #179                 | MERGED at `eec4038`                                         |
| PR #180                 | MERGED at `831455b`                                         |
| PR #181                 | MERGED at `682205a`                                         |
| PR #181 required checks | PASS                                                        |
| Post-merge CI (PR #181) | PASS — CI 5m01s; Identity database 2m39s                    |

Reproduction from a clean checkout:

```sh
pnpm install --frozen-lockfile
make up-build
make seed-demo
make test-access
pnpm exec nx run @org/web-e2e:e2e --args="--project=real-stack"
```

Provider, notification-flow, retention and storage procedures are documented in
`DEMO_READINESS.md` and `tests/integration/notificationflow/README.md`.

## 17. Post-acceptance merchant and receipt enhancement (2026-11-20)

This section records work completed after the accepted baseline
(`682205a`/`6686de2`). It is **additive** and does not change the acceptance
decision or historical results in sections 1–16. It is not part of the accepted
baseline and does not imply the baseline was defective.

### Compatibility decision

- `orderId` remains in every merchant order response and
  `GET /merchant/orders/{orderId}` remains the detail route. No identifier
  semantics, database key or migration changed.
- `ownerUserId` is no longer included in the merchant's own
  `GET`/`PATCH /merchants/me` response. Administrator responses still include
  it for the merchant review screen.
- No existing API v1.1 route, field or status code was removed or renamed.

### Merchant experience additions

- A dedicated `/merchant/integration` page (sidebar **API integration**) holds
  the server-to-server onboarding panel; the Orders page links to it instead of
  embedding it.
- Merchant order rows, and the dashboard's recent-order rows, are one
  keyboard-accessible navigation target with hover and focus-visible states.
- `GET /api/v1/merchant/orders/{trackingNumber}/receipt` returns a private,
  branded application/pdf receipt for the authenticated merchant's own order.
  It accepts a MERCHANT session or API key, is scoped by merchant, sets
  `Content-Disposition` and `Cache-Control: no-store`, and returns the existing
  safe 404 for another merchant's order.
- The merchant API guide is available as
  [`docs/merchant-api-guide.html`](../merchant-api-guide.html) and
  [`docs/merchant-api-guide.pdf`](../merchant-api-guide.pdf), and as a download
  from the integration page.
- The customer parcel receipt at `GET /api/v1/parcels/{reference}/receipt`
  (parcel UUID or public tracking number) now uses the same SwiftDrop print
  identity: navy header, lime mark, status, sender/recipient cards, parcel
  details, fee table, status timeline, support footer and page numbers.
  Ownership is enforced in the same query; another customer receives 404.
- Both receipts use `packages/go-common/receiptdesign` for the palette, page
  geometry, status labels and money/date/contact formatting. The merchant
  receipt uses `fpdf`; the customer receipt retains the shaping-capable text
  engine because `fpdf` cannot shape Sinhala/Tamil/Arabic/Hebrew. This is a
  deliberate quality trade-off, not an incomplete migration.

### Verification

- Backend: Go format/build/vet/unit tests; receipt renderer tests; merchant
  ownership integration test for the receipt; race tests for the changed
  merchant packages.
- Contracts: OpenAPI validation and the 34 contract/route tests pass; the route
  inventory includes the receipt route.
- Frontend: unit/lint/build/typecheck pass; Playwright covers the dedicated
  integration page, clickable rows, the receipt download and the dashboard row.
- Security: cross-merchant receipt access returns 404, anonymous access returns
  401, receipts are `no-store`, and no internal owner ID appears in merchant
  self-service responses. The updated access matrix covers 68 endpoints.

The production deployment, multi-replica and operator-retention limitations in
sections 13–15 remain unchanged.
