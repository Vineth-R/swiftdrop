# SwiftDrop — Final Acceptance Report

**Date:** 2026-11-20
**Baseline:** `development` at `831455b952e32cbb63623799750720635ddd185b`
(the PR #180 merge, which already contains PR #179).
**Verification branch:** `docs/final-remediation-verification` (verification and
documentation only; no product-code changes in this pass).
**Scope:** approved non-deployment functionality, security, reliability,
integration and documentation.

## 1. Decision

| Dimension                           | Decision                                                               |
| ----------------------------------- | ---------------------------------------------------------------------- |
| Core application functionality      | **COMPLETE**                                                           |
| Authentication/authorization        | **COMPLETE**                                                           |
| Database integrity and transactions | **COMPLETE**                                                           |
| Event-driven processing             | **COMPLETE**                                                           |
| Notifications and webhooks          | **COMPLETE**                                                           |
| Realtime                            | **COMPLETE for the supported single-instance architecture**            |
| AI assistant                        | **COMPLETE**; live Gemini smoke passed; live Google consent blocked    |
| Admin audit visibility              | **COMPLETE** (new in this phase)                                       |
| Security review                     | **COMPLETE**; no open critical/high defect                             |
| Documentation/API consistency       | **COMPLETE**                                                           |
| Production deployment               | **OUT OF SCOPE** for this phase                                        |
| Overall non-deployment acceptance   | **COMPLETE except one blocked external-provider account-consent step** |

The project is **not claimed as 100% end-to-end complete** because live Google
OAuth consent/account-linking could not be performed in this environment and
production deployment is excluded. Everything else required by the approved
non-deployment scope has implementation and executed evidence.

## 2. Implemented and verified in this phase

### Admin audit visibility

- `GET /api/v1/admin/audit-logs/identity` — Identity audit rows.
- `GET /api/v1/admin/audit-logs/merchant` — Merchant lifecycle audit rows.
- `GET /api/v1/admin/audit-logs/pricing` — Pricing configuration audit rows.
- Pagination, actor/action/resource/date/sort filters.
- Recursive redaction of password/token/secret/credential/authorization/cookie/
  API-key detail keys.
- ADMIN-only at backend, access matrix and Nginx mapping.
- `/admin/audit-logs` frontend with source selector, filters, loading/empty/
  error/pagination and details view.
- Mocked and real-stack UI tests; 66-route authorization matrix.

### Failed outbox recovery

- Report, replay and expiry commands.
- Deterministic cutoff/limit behaviour.
- Concurrent replay test proving each FAILED row is replayed once.
- Existing claims/publisher-confirm/duplicate-prevention tests remain green.

### Retention

- 365-day audit-log and 90-day webhook-attempt policy.
- Dry-run/apply operator command.
- Bounded batching (`BATCH_SIZE`, `MAX_BATCHES`) so a run is resumable and does
  not hold one long transaction.
- Local dry run executed against the Compose database.
- New regression tests prove the dry run is read-only, apply deletes only the
  four owned tables, batching bounds reach the SQL, and windows below 30 days or
  non-positive/non-numeric limits are refused.

### Proof lifecycle

- Private storage, authorized reads, short-lived links and missing-object
  handling are implemented and tested.
- `proof-transfer -reconcile` reports orphan/missing objects.
- Retention/deletion policy documented as a business decision; no automatic
  deletion of committed delivery evidence.

### Security improvements/fixes

- Logout/session family fix from PR #179 remains verified.
- Audit detail redaction.
- S3 integration helper now creates buckets correctly outside `us-east-1`.
- No security check was weakened to pass a test.

### Notification flow

- The dedicated `notificationflow` Compose profile was re-run locally against a
  disposable project (`swiftdrop-notificationflow`) with Mailpit as SMTP.
- All 18 tests/subtests passed, including customer parcel-created and
  status-changed email, duplicate suppression, merchant-owned approved/suspended
  contacts, invalid/foreign-producer dead-lettering, identity-unavailable
  dead-lettering, SMTP failure recovery, legacy delivery and deduplication.
- Correct local runs require `SMTP_HOST=mailpit`, an empty `SMTP_USER`/
  `SMTP_PASSWORD`, and `RABBITMQ_RETRY_DELAY`/`RABBITMQ_MAX_RETRIES` exported
  before `docker compose up` so the worker container receives them. This is a
  harness prerequisite, not a product defect; CI already sets them.
- The exact reproduction command is in `docs/implementation/DEMO_READINESS.md`
  and `tests/integration/notificationflow/README.md`.

### Google OAuth

- 32 automated tests pass: 25 service-level tests (state, PKCE, browser binding,
  single-use state, callback, account creation/linking, disabled accounts,
  merchant approval, store failures) plus 7 provider-helper tests.
- `GET /api/v1/auth/google/login` returns a `302` with PKCE `S256`, a random
  state and the configured callback; loading that URL in a real browser reached
  Google's sign-in page for the configured client ID and redirect URI.
- Completing consent and account linking needs an authorized Google test
  account. It is not claimed as passed and is recorded as blocked below.

### Reliability/verification

- Full migration up/down/reapply cycle passed against a disposable PostgreSQL 16.
- Full backup/restore rehearsal passed against a disposable PostgreSQL 16,
  including 31 tables, five schemas, versions, dirty flags and representative
  data.
- Webhook reliability real-stack suite passed: transient retry, six-attempt
  exhaustion, rotation.
- S3-compatible storage lifecycle/presign tests passed.
- k6 public/tracking/pricing and WebSocket baselines passed.
- Live Gemini smoke passed.
- Google OAuth redirect with PKCE/state/browser binding verified; consent flow
  not completed.

## 3. Implemented but not fully verified

| Item                                           | Why                                                                                                                                                                       |
| ---------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Google OAuth full consent/account-link journey | Google accepted the configured client ID/redirect URI and served the real sign-in page; completing consent needs an interactive Google account, which is unavailable here |
| Multi-replica realtime/AI/rate-limit behaviour | Not implemented; documented single-instance constraint                                                                                                                    |
| Automatic audit/webhook retention scheduling   | Operator command implemented; no background service scheduler                                                                                                             |

## 4. Not implemented (optional/future)

- Parcel editing and cancellation.
- Dispatcher-initiated status writes.
- SMS notifications.
- In-app notification inbox.
- Courier self-service availability.
- Account deletion, email change, device/session management.
- Payments.
- Merchant live parcel views.
- Bulk operations/exports.
- Support ticketing.

These are not required by the approved non-deployment scope and are not counted
as complete.

## 5. Blocked

| Item                                | Dependency                              | Manual procedure                                                                                                                                                                                                                                                                                                                                                     |
| ----------------------------------- | --------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Live Google consent/account linking | Interactive Google account credentials  | `GET /api/v1/auth/google/login` returns a 302 to Google with PKCE/state; Google accepts the client ID and redirect URI and serves its sign-in page. Sign in with an authorized test account, approve consent, and verify the callback creates/links the account and issues a session. Automated tests cover state, PKCE, binding, callback, linking and error paths. |
| Production deployment verification  | Production hosting/TLS/migration window | Separate deployment phase                                                                                                                                                                                                                                                                                                                                            |

## 6. Out of scope

- Kubernetes, Terraform, production hosting, TLS termination, domains.
- Production CI/CD deployment and production migration execution.
- Prometheus/Grafana/tracing infrastructure.
- Multi-replica shared state (Redis or equivalent) unless separately approved.

## 7. Evidence summary

| Evidence                                              | Result                         |
| ----------------------------------------------------- | ------------------------------ |
| `gofmt`, `go build`, `go vet`, `go test ./...`        | PASS                           |
| Race tests (identity, merchant, pricing, objectstore) | PASS                           |
| Contract/OpenAPI/route tests (32)                     | PASS                           |
| Frontend unit/lint/build/e2e typecheck                | PASS                           |
| Compose config                                        | PASS                           |
| 66-route access matrix                                | PASS                           |
| Mocked audit UI                                       | PASS                           |
| Real-stack audit UI                                   | PASS                           |
| Real-stack core journeys (previous pass)              | PASS                           |
| Dedicated notification-flow suite (18 tests)          | PASS — local Compose + Mailpit |
| Webhook reliability (3 real-stack tests)              | PASS                           |
| Event-flow PostgreSQL + RabbitMQ                      | PASS                           |
| Identity repository integration                       | PASS                           |
| Merchant order/API-key integration                    | PASS                           |
| Migration up/down/reapply                             | PASS                           |
| Backup/restore rehearsal                              | PASS                           |
| S3-compatible storage lifecycle/presign               | PASS                           |
| k6 public/tracking/pricing (5 VU, 10s)                | PASS — 0 failures, p95 ~11 ms  |
| k6 WebSocket (3 VU, 8s)                               | PASS                           |
| Live Gemini smoke                                     | PASS — HTTP 200                |
| Google PKCE redirect + provider sign-in page          | PASS; account consent BLOCKED  |

## 8. Risk statement

No unresolved critical or high-severity application defect was found in the
approved non-deployment scope. Residual risks are documented in
`SECURITY_REVIEW.md`: shared internal token, no CSRF synchronizer token,
CSP inline allowance, process-local rate/AI/realtime state, at-least-once
webhooks, and operator-driven audit/webhook retention.

## 9. Reproduction

From a clean checkout:

```sh
pnpm install --frozen-lockfile
make up-build
make seed-demo
make test-access
pnpm exec nx run @org/web-e2e:e2e --args="--project=real-stack"
```

Provider and storage checks are documented in `DEMO_READINESS.md`.

## 10. Final conclusion

SwiftDrop meets the approved **non-deployment functional, security and
verification scope**. It is **not** declared production-ready: deployment,
TLS/hosting, the production migration window and multi-replica shared state
remain outside this phase, and live Google consent remains a manual provider
check.
