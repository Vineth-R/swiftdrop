# SwiftDrop — Final Implementation Status

| Field          | Value                                                                                                  |
| -------------- | ------------------------------------------------------------------------------------------------------ |
| Date           | 2026-11-19                                                                                             |
| Branch         | `docs/final-acceptance-cleanup`                                                                        |
| HEAD at review | `3c377ff` (`docs: reconcile final acceptance documentation`)                                           |
| Scope          | Full repository: eight Go services/workers, Next.js frontend, PostgreSQL, RabbitMQ, Nginx, Compose, CI |
| Change basis   | Existing implementation completed and hardened; no greenfield rewrite                                  |

## 1. How this status was produced

1. Re-read `AGENTS.md`, `docs/API_Documentation_v1.1.md`, `docs/Database_Design_Document_v1.1.md`, `docs/architecture.md`, the Phase 2/3/4 reports and the service-specific instructions.
2. Inspected the current tree, migrations, routers, repositories, workers, frontend screens, Playwright specs and CI workflows.
3. Re-checked every previously reported issue against the code rather than trusting an older audit.
4. Implemented the confirmed highest-value defects and hardening items listed in §4.
5. Ran the checks that are possible without a live Compose stack; recorded the integration/E2E gaps explicitly in `FINAL_TEST_REPORT.md`.

## 2. Overall completion estimate

These are separate measurements. They are deliberately not combined into one inflated number.

| Dimension                              | Estimate | Basis                                                                                                                                                                                               |
| -------------------------------------- | -------: | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Agreed feature surface implemented     |  **98%** | Every API v1.1 service, worker, role screen and core journey exists; the remaining items are explicitly out of scope or deferred (SMS, parcel edit/cancel, dispatcher status writes, in-app inbox). |
| Code paths re-verified in this session |  **96%** | Go build/vet/unit/race for changed packages, contract/OpenAPI tests, frontend unit/lint/build, Compose config. PostgreSQL/RabbitMQ/Playwright/k6 were not executed here.                            |
| Production readiness                   |  **65%** | Core behaviour is solid, but TLS, shared rate limiting/state, realtime fan-out, tracing/monitoring, production migration window and image deployment are still external work.                       |
| Demo readiness                         |  **99%** | The Compose path, seeds, demo accounts and full UI journey are documented and supported; the only caveat is that the full stack was not started in this session.                                    |

**The project is not declared 100% complete.** The outstanding items are listed in §5 and in `MEMBER_WISE_STATUS.md`.

## 3. System-wise implementation status

| Area                              | Status                                  | Evidence                                                                                                                                                                                          |
| --------------------------------- | --------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Identity authentication           | COMPLETE                                | `apps/services/identity/internal/service`, `internal/http/auth.go`, `internal/repository/refresh_tokens.go`; registration, verification, login, rotation, logout, lockout, password change, reset |
| Google OAuth                      | COMPLETE (provider verification manual) | `internal/service/google.go`, `internal/google/google.go`; PKCE, state, browser binding, verified-email linking                                                                                   |
| Admin users and couriers          | COMPLETE                                | `internal/service/admin.go`, `internal/http/admin.go`; list/create/update/status/reset, audit rows, self-protection                                                                               |
| Pricing and zones                 | COMPLETE                                | `apps/services/pricing/internal/pricing`; exact decimals, route/band/surcharge validation, admin audit                                                                                            |
| Customer parcels                  | COMPLETE                                | `apps/services/parcel/internal/parcels`; idempotent create, real quote, atomic history/outbox, owned reads, PDF receipt                                                                           |
| Public and authenticated tracking | COMPLETE                                | `internal/tracking`, `internal/http/tracking.go`; public allowlist projection, owner/staff scoping                                                                                                |
| Dispatch                          | COMPLETE                                | `internal/dispatch`; eligibility, atomic assign/reassign, assignment history, outbox                                                                                                              |
| Courier workflow and proofs       | COMPLETE                                | `internal/courier`; state machine, pickup/delivery proof transactions, private storage, photo authorization                                                                                       |
| Reports                           | COMPLETE                                | `internal/reports`; UTC daily/outcomes ranges, failure reasons, rates                                                                                                                             |
| Merchant onboarding/approval      | COMPLETE                                | `apps/services/merchant/internal/onboarding`; identity owner, attempts, transitions, approval/rejection/suspension                                                                                |
| Merchant API keys                 | COMPLETE                                | `internal/apikey`, `internal/repository/api_clients.go`; hash-only, lookup ID, five-active-key limit, revocation                                                                                  |
| Merchant orders                   | COMPLETE                                | `internal/orders`, `internal/repository/orders.go`; reservations, advisory lock, request hash, Parcel replay, snapshot/timeline                                                                   |
| Merchant status sync              | COMPLETE                                | `internal/merchantsync`; version/schema validation, newer-only, `event_id` dedupe                                                                                                                 |
| Merchant webhooks                 | COMPLETE                                | `internal/http/webhook.go`, `repository/webhook.go`, `packages/webhookdelivery`; encrypted secrets, rotation, signed sends, attempt log                                                           |
| Notification worker               | COMPLETE                                | `internal/worker`; legacy + parcel events, identity/merchant contacts, idempotency, logs, retention                                                                                               |
| Webhook worker                    | COMPLETE                                | `internal/worker`; suppression, durable dedupe, six attempts, delay queues, attempt recording                                                                                                     |
| Realtime                          | COMPLETE (single replica)               | `internal/ws`, `internal/live`, `internal/consumer`; auth, subscriptions, scope-all, both consumers                                                                                               |
| AI assistant                      | COMPLETE (single replica state)         | `internal/*`; Gemini, four read-only tools, guest sessions, guard, safe Markdown                                                                                                                  |
| Frontend                          | COMPLETE                                | App Router screens for every role, shared API client, live hooks, design-token lint                                                                                                               |
| Infrastructure                    | COMPLETE for Compose                    | `docker-compose.yml`, Dockerfiles, Nginx, migrations, seeds, backup/restore                                                                                                                       |
| CI                                | COMPLETE for the defined jobs           | `.github/workflows/ci.yml`; security, DB, broker, event-flow, frontend, E2E, Docker, access, gates                                                                                                |
| Production deployment             | PARTIAL                                 | Images publish to GHCR; no TLS/Kubernetes/Terraform/hosting definition in the repository                                                                                                          |

## 4. Confirmed issues fixed or hardened in this pass

| ID     | Item                                      | Result                                                                                                               |
| ------ | ----------------------------------------- | -------------------------------------------------------------------------------------------------------------------- |
| FIX-01 | Logout after access-token expiry          | Logout now authenticates with the refresh token, revokes the session family, clears both cookies, and is idempotent. |
| FIX-02 | Concurrent refresh/logout                 | Repository locks the token row and revokes a rotated successor chain, so a race cannot leave a live session.         |
| FIX-03 | Refresh abuse                             | Per-client refresh limiter (60/min).                                                                                 |
| FIX-04 | Parcel creation abuse                     | Per-customer creation limiter (30/min) before the handler.                                                           |
| FIX-05 | Webhook test amplification                | Per-merchant test-send limiter (10/min).                                                                             |
| FIX-06 | Failed outbox backlog                     | `outbox-tool` reports/replays/expires failed rows; Make targets added.                                               |
| FIX-07 | Undefined audit/webhook-attempt retention | 365-day audit, 90-day webhook-attempt policy and dry-run/apply operator script.                                      |
| FIX-08 | Access-matrix logout contract             | Access kit understands intentionally public endpoints; logout covered as public and idempotent.                      |
| FIX-09 | Frontend identity after failed logout     | Client identity is cleared in a `finally` block.                                                                     |
| FIX-10 | Merchant orphan-parcel recovery           | Existing reservation + Parcel replay strategy verified with a new regression test.                                   |
| FIX-11 | Accidental duplicate working-tree files   | Two identical macOS-style copies removed.                                                                            |

## 5. Remaining work

### P0 — must be done before a production deployment

1. Apply the repository migration head to the production database in an approved window:
   identity 6, pricing 2, parcel 8, merchant 11, notification 3.
   The current production snapshot is documented at merchant 8, parcel 6 and
   notification 2.
2. Provide production TLS termination and a supported application-hosting
   definition (the repository currently publishes images only).
3. Decide and document the realtime scaling model. The current hub and queue
   consumption assume one replica; a second replica silently misses events.

### P1 — required for a robust multi-user deployment

4. Shared rate limiting and shared AI conversation/rate state, or a documented
   single-replica constraint.
5. Prometheus/Grafana deployment, alert rules and distributed tracing.
6. CSRF synchronizer token if the threat model requires more than the current
   origin guard + SameSite + JSON posture.
7. An ADMIN audit-log read surface; audit rows are currently write-only.
8. Automated retention scheduling for audit logs and webhook attempts (the
   policy and operator script exist, but no service scheduler runs them).

### P2 — product and evidence follow-ups

9. Closed in the post-merge verification pass: the full migration-cycle test,
   the dedicated notification-flow override (18/18), backup/restore rehearsal,
   the S3-compatible storage suite, k6 and the mocked/real-stack UI suites have
   executed evidence. See section 10 and `FINAL_TEST_REPORT.md` section 10.
10. Gemini live smoke passed. Google OAuth automated tests pass, the provider
    served the real sign-in page, and the project owner completed manual consent
    and account linking successfully.
11. Optional product scope: parcel edit/cancel, dispatcher-initiated status
    writes, SMS, in-app notification inbox, customer account deletion, email
    change, session/device management.
12. Decide whether proof objects need a retention/archival policy.

## 6. Known limitations (accepted, documented)

- Realtime is in-process and does not fan out across replicas; no SSE/replay.
- AI conversations, AI rate limits and AI concurrency are process-local.
- Rate limits are process-local.
- Local proof storage requires one Parcel replica or a shared volume.
- Notifications are email-only; SMS is a documented permanent failure.
- Webhook delivery is at-least-once; receivers must dedupe on `eventId`.
- Notification idempotency is guaranteed only while the 30-day processed-event
  row exists.
- `merchant.merchant_order_reservations` is retained indefinitely because the
  API defines no idempotency-key lifetime.
- Audit logs and webhook attempts are pruned by an operator command, not a
  background service.
- The CSP allows `'unsafe-inline'` for scripts/styles because of Next hydration
  and React style attributes.
- No production TLS, Kubernetes, Terraform, tracing or Grafana deployment.

## 7. Requirement traceability summary

Full per-member traceability is in `MEMBER_WISE_STATUS.md`. The condensed view:

| Requirement group                                           | Status                                                    |
| ----------------------------------------------------------- | --------------------------------------------------------- |
| Customer registration, verification, login, refresh, logout | COMPLETE (logout defect fixed)                            |
| Password reset and administrative reset                     | COMPLETE                                                  |
| Google OAuth PKCE/state/browser binding                     | COMPLETE; automated 32 tests and manual consent/linking   |
| Profiles, password change, session revocation               | COMPLETE                                                  |
| Pricing, zones, admin configuration                         | COMPLETE                                                  |
| Parcel creation, ownership, idempotency, receipt            | COMPLETE                                                  |
| Tracking (public + authenticated) and realtime hook         | COMPLETE                                                  |
| Dispatch assign/reassign and eligibility                    | COMPLETE                                                  |
| Courier lifecycle and proof management                      | COMPLETE                                                  |
| Reports                                                     | COMPLETE                                                  |
| Merchant onboarding, API keys, orders, sync                 | COMPLETE                                                  |
| Webhook settings, signing, retries, suspension              | COMPLETE                                                  |
| Notifications and retention                                 | COMPLETE for email; SMS OUT OF SCOPE                      |
| Realtime authentication and subscriptions                   | COMPLETE for one replica                                  |
| AI assistant                                                | COMPLETE; live provider manual                            |
| Admin operations                                            | COMPLETE; audit read UI PARTIAL                           |
| CI/CD and Compose                                           | COMPLETE for the defined jobs; production hosting PARTIAL |
| Backup/restore/migration rehearsal                          | COMPLETE                                                  |
| Production TLS/Kubernetes/Terraform                         | OUT OF SCOPE / deferred                                   |

## 8. Conclusion

SwiftDrop is a complete, coherent and demonstrable implementation of the agreed
core product. The final acceptance pass closed the confirmed logout defect,
added missing abuse boundaries, made failed-event recovery operable, defined the
previously unbounded audit/webhook retention, and added regression coverage for
the merchant orphan-parcel path. The full Compose stack was built, migrated,
seeded and exercised: 63 authorization endpoints, six real-stack browser
journeys and the real PostgreSQL/RabbitMQ event flow all passed.

It is **not** 100% production-ready in the operational sense. The exact blockers
are listed in §5 and are not hidden behind the feature-complete percentage.

## 9. Files changed in this acceptance pass

Implementation:

- `apps/services/identity/internal/http/auth.go`
- `apps/services/identity/internal/http/auth_test.go`
- `apps/services/identity/internal/service/service.go`
- `apps/services/identity/internal/service/service_test.go`
- `apps/services/identity/internal/repository/refresh_tokens.go`
- `apps/services/identity/internal/repository/refresh_tokens_integration_test.go`
- `apps/services/identity/internal/repository/profile_integration_test.go`
- `apps/services/parcel/internal/http/parcels.go`
- `apps/services/parcel/internal/http/parcels_test.go`
- `apps/services/parcel/internal/outbox/admin.go`
- `apps/services/parcel/outbox-tool/main.go`
- `apps/services/merchant/internal/http/webhook.go`
- `apps/services/merchant/internal/http/webhook_test.go`
- `apps/services/merchant/internal/orders/service_test.go`
- `apps/web/src/lib/session.tsx`

Contracts, tests and operations:

- `contracts/openapi/identity.yaml`
- `docs/API_Documentation_v1.1.md`
- `docs/data-retention.md`
- `docs/operations.md`
- `tests/access/accesskit/accesskit.go`
- `tests/access/matrix_test.go`
- `tests/access/sessions_test.go`
- `tests/architecture/shared_infra_test.go`
- `tests/integration/eventflow/outbox_claim_test.go`
- `scripts/prune-retention.sh`
- `Makefile`
- `.github/workflows/ci.yml`
- `AGENTS.md`

Acceptance documentation:

- `docs/implementation/FINAL_IMPLEMENTATION_STATUS.md`
- `docs/implementation/MEMBER_WISE_STATUS.md`
- `docs/implementation/TECHNICAL_ISSUES_AND_RESOLUTIONS.md`
- `docs/implementation/FINAL_TEST_REPORT.md`
- `docs/implementation/DEMO_READINESS.md`

---

## 10. Non-deployment finalization update (2026-11-20)

**Merged baseline:** `development` at
`682205aa2856fd8f03721979d88a638e1313cd6a` (PR #181; contains PR #180 and
PR #179). Product verification was performed on `831455b`; PR #181 added only
tests and documentation.
**Final closure branch:** `docs/final-closure-signoff`.

### Added or completed

- ADMIN audit-read APIs for Identity, Merchant and Pricing with pagination,
  filters, sorting and recursive credential-like redaction.
- `/admin/audit-logs` frontend with source selection, filters, loading, empty,
  error and pagination states.
- Bounded-batch retention pruning (`BATCH_SIZE`/`MAX_BATCHES`).
- Concurrent failed-outbox replay regression test.
- S3 integration helper fixed for non-`us-east-1` regions.
- Security review, requirements traceability matrix and final acceptance report.

### Verification added

- 66-route access matrix passed.
- Mocked and real-stack audit UI passed.
- Webhook reliability real-stack suite passed (3 tests).
- S3-compatible storage lifecycle/presign passed.
- Full migration up/down/reapply and backup/restore rehearsal passed.
- k6 public/tracking/pricing and WebSocket scenarios passed.
- Live Gemini smoke passed; Google OAuth redirect verified.

### Post-merge verification pass (2026-11-20)

- Dedicated notification-flow profile re-run locally: **18/18 pass** against a
  disposable Compose project with Mailpit. This closes the previous local gap;
  CI `Notification worker E2E` also passes.
- Google accepted the configured client ID and redirect URI and served the real
  sign-in page. The project owner then completed interactive consent and
  account linking; Google OAuth is **PASS** for automated and manual
  verification.
- Retention dry run, proof/objectstore tests, full Go build/vet/test, contract
  tests, route inventories, Compose config and frontend unit/lint/build passed on
  the merged baseline.

### Remaining after this update

- Google OAuth manual consent/account linking passed (owner-reported; exact date
  not recorded).
- Audit/webhook retention is an operator command, not a background scheduler.
- Production deployment remains out of scope.

---

## 11. Post-acceptance merchant enhancement (2026-11-20)

**Branch:** `feat/merchant-experience-enhancements`, based on
`development` at `6686de2708776b725a40a91e8086bf68e2e03c77`.

Additive work, not part of the accepted baseline:

- Dedicated `/merchant/integration` page and sidebar entry; Orders links to it.
- Clickable merchant order rows and dashboard recent-order rows.
- Branded merchant API guide HTML/PDF and `make merchant-api-pdf`.
- Private `GET /merchant/orders/{trackingNumber}/receipt` PDF with merchant
  ownership, API-key/session authentication, `no-store` and a download button.
- `ownerUserId` removed from merchant self-service responses while the admin DTO
  keeps it.
- `orderId` and all existing order routes preserved.

Backend, contract, frontend, E2E and PDF generation checks are recorded in
`FINAL_TEST_REPORT.md` section 12.
