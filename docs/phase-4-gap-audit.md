# Phase 4 Gap Audit

| Field                     | Value                                                                    |
| ------------------------- | ------------------------------------------------------------------------ |
| Audit date                | 2026-11-15                                                               |
| Audit type                | Read-only implementation, integration, test and documentation assessment |
| Audit baseline branch     | `development`                                                            |
| Audit baseline commit     | `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`                               |
| Source pull request       | [#156](https://github.com/Vineth-R/swiftdrop/pull/156)                   |
| PR head commit            | `4eef1ba90bc8e4c3fa22a681a817b20a3e014191`                               |
| PR base commit            | `686a03dc25538f6353839fbb16dd83c0c56457e5`                               |
| Merge method              | Merge commit                                                             |
| Merge timestamp           | 2026-11-15T04:41:28Z                                                     |
| Merge result on CI        | Run `43677865852` — passing (see §2)                                     |
| Code changes in this task | Audit report only; no production code or migration changed               |

## 1. Purpose and scope

This audit answers one question: **what remains before SwiftDrop's core business
workflows are complete, integrated, demonstrable and ready for Phase 4
implementation?**

It is deliberately anchored to the merged `development` commit above, not to the
Phase 3 branch and not to the older Phase 3 audit baseline. Documentation was
checked against the implementation, and implementation was checked against the
API/event contracts and tests.

Scope covered:

- customer registration, authentication, profile, parcel creation, tracking,
  history and notifications;
- dispatcher queue, assignment/reassignment, monitoring and realtime updates;
- courier assigned work, status progression, proof of delivery and failure
  handling;
- merchant onboarding, API orders, idempotency, webhooks, secret rotation,
  delivery history and status synchronisation;
- admin users, pricing, reports and access control;
- event publication, consumption, retries, idempotency and failure handling;
- frontend/backend integration and real-stack versus mocked coverage;
- operational readiness, database migration safety and security posture.

Out of scope: implementing Phase 4, creating feature branches, changing product
behaviour, editing existing contracts or migrations, or declaring planned
features as implemented. Parcel edit/cancel is not part of API v1.1 and is not
reported as missing functionality.

## 2. Baseline and merge record

PR #156 was reviewed against its actual base (`development`), its full diff,
commit history, review threads, required checks and the active branch ruleset.
The PR head was still `4eef1ba`; the required checks were green; the ruleset
required the `Required CI gate` and `Security gate` with strict up-to-date
checks. No unresolved review threads existed (the Copilot review attempt did not
run because of quota). The PR was merged with a merge commit.

| Item                             | Verified value                                                                |
| -------------------------------- | ----------------------------------------------------------------------------- |
| PR                               | #156 — `feat: complete Phase 3 testing, documentation and QA`                 |
| Merge commit                     | `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`                                    |
| First parent (base)              | `686a03dc25538f6353839fbb16dd83c0c56457e5`                                    |
| Second parent (PR head)          | `4eef1ba90bc8e4c3fa22a681a817b20a3e014191`                                    |
| Remote `development` after fetch | `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`                                    |
| Merge commit message             | `Merge pull request #156 from Vineth-R/feat/phase-3-testing-documentation-qa` |
| Merge-author timestamp           | `Sun Nov 15 10:11:27 2026 +0530` (Git author time)                            |

The merge result is the audit commit. A green check on the PR head is not
treated as the merge-result check; the post-merge CI run is reported separately
in §4.4.

Phase 4 Audit Baseline:

```text
Branch: development
Commit: 9b719c8daad32dc1a541e02da8d0ad4b768d65a5
PR: #156
```

## 3. Repository and service overview

SwiftDrop remains an Nx monorepo with one Go module, eight Go services/workers, a
Next.js App Router frontend, five owned PostgreSQL schemas, RabbitMQ, Nginx and
Docker Compose.

| Component             | Implemented responsibility                                                                                                         | Persistence    |
| --------------------- | ---------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| `identity`            | Authentication, email verification, Google login, reset, profiles, admin users, couriers, internal identity lookups                | `identity`     |
| `parcel`              | Creation, owned reads/history/receipt, public/authenticated tracking, dispatch, courier reads/status/proofs, reports, outbox relay | `parcel`       |
| `pricing`             | Estimate/zones/internal quote, admin rules/bands/surcharges and audit                                                              | `pricing`      |
| `merchant`            | Registration/approval lifecycle, profile, API keys, orders, order snapshots/timeline, webhooks, merchant status sync               | `merchant`     |
| `notification-worker` | Customer/merchant email, idempotency, delivery logs, admin notification-log read API                                               | `notification` |
| `webhook-worker`      | Signed merchant webhook delivery, attempts, six business retries                                                                   | none           |
| `realtime`            | `/ws`, status and assignment consumers, per-parcel and scope-all delivery                                                          | none           |
| `ai-assistant`        | Gemini-backed delivery assistant with read-only tools and guest sessions                                                           | none           |
| `web`                 | Customer, courier, dispatcher, merchant and admin screens                                                                          | none           |
| `nginx`               | Public edge, prefix stripping, security headers and WebSocket proxy                                                                | none           |

Service boundaries are respected in product code: no service imports another
service's `internal/` package or reads another service's schema. Cross-service
work uses HTTP or RabbitMQ. Seed tools remain the documented exception.

## 4. Current implementation status

### 4.1 Business capability matrix

Status terms:

- **Verified** — implementation plus integration or real-stack evidence exists.
- **Implemented, test gap** — code is present but a material browser or
  end-to-end path is missing.
- **Partial** — some layers exist, but a required acceptance path is absent.
- **Missing** — not implemented.
- **Accepted** — deliberately deferred and documented as future work.

| Capability                                                             | Status                                     | Evidence and remaining gap                                                                |
| ---------------------------------------------------------------------- | ------------------------------------------ | ----------------------------------------------------------------------------------------- |
| Customer registration, email verification, login, refresh, logout      | Verified                                   | Real-stack onboarding/verification; identity tests                                        |
| Customer profile and password change                                   | Implemented, test gap                      | Backend tests plus mocked UI; no real-stack profile/password journey                      |
| Password reset                                                         | Implemented, test gap                      | Backend and mocked UI; no real-stack reset journey                                        |
| Google login                                                           | Implemented, test gap                      | Provider verification tests and documented live evidence; no controlled live smoke in CI  |
| Customer parcel creation                                               | Verified                                   | Pricing integration, atomic parcel/history/outbox, real-stack API creation, mocked wizard |
| Customer ownership, list and detail                                    | Verified backend; test gap frontend        | Integration/HTTP tests; customer list/detail UI mocked                                    |
| Customer history and PDF receipt                                       | Implemented, test gap                      | HTTP receipt tests; mocked UI download only                                               |
| Public tracking and live updates                                       | Verified                                   | Real-stack tracking and `/track` live update                                              |
| Customer notifications                                                 | Verified backend; partial UX               | Mailpit notification-log real-stack; email only, no in-app inbox (not in API v1.1)        |
| Dispatcher queue and workload                                          | Verified backend                           | Integration plus mocked UI                                                                |
| Dispatcher assignment/reassignment                                     | Verified backend; test gap UI action       | Real-stack assignment is API-driven; mocked modal covers UI                               |
| Dispatcher realtime status/assignment                                  | Verified                                   | Real-stack `/parcels` and assignment notices                                              |
| Dispatcher reports                                                     | Verified backend; partial UI               | Database integration; real-stack read only, including empty range                         |
| Courier list/detail                                                    | Verified                                   | Real-stack Today/detail plus original mocked states                                       |
| Courier status state machine                                           | Verified backend; partial real-stack       | Full transition matrix tested; real-stack covers `PICKED_UP`                              |
| Courier delivered with recipient                                       | Implemented, mocked UI only                | State machine/HTTP tests; no real-stack `DELIVERED`                                       |
| Courier failed with reason                                             | Implemented, mocked UI only                | State machine/HTTP tests; no real-stack `FAILED`                                          |
| Courier proof upload/view                                              | Verified backend; test gap frontend/object | Integration tests and mocked upload/view; no real-stack proof                             |
| Merchant onboarding and approval                                       | Verified                                   | Real-stack registration, approval, rejection, suspension and emails                       |
| Merchant API keys                                                      | Verified                                   | Real-stack creation/revocation and isolation                                              |
| Merchant orders and idempotency                                        | Verified                                   | Integration plus real-stack order journey                                                 |
| Merchant status synchronisation                                        | Verified                                   | Event-flow real-stack and order real-stack                                                |
| Merchant webhook configuration and rotation                            | Verified                                   | Real-stack save/rotate/test/log/isolation                                                 |
| Merchant webhook retries and dead-letter                               | Implemented, test gap                      | Unit tests for all six attempts; real-stack observes only attempt 1                       |
| Admin users                                                            | Implemented, test gap                      | Backend integration plus real-stack list; create/edit UI mocked                           |
| Admin merchant actions                                                 | Verified backend; test gap UI              | Real-stack API approve/reject/suspend; mocked UI actions                                  |
| Admin pricing/zones                                                    | Verified backend; partial UI               | Backend tests; real-stack pricing read only; edits mocked                                 |
| Admin notification logs                                                | Verified                                   | Real-stack masked log and denied dispatcher                                               |
| Realtime status/assignment                                             | Verified                                   | Real-stack status and assignment notices                                                  |
| AI assistant                                                           | Implemented, test gap                      | Deterministic/mocked tests and opt-in live smoke; no CI provider call                     |
| Metrics/readiness                                                      | Verified                                   | Internal `/metrics`, consumer/relay-aware readiness, broker-loss evidence                 |
| Distributed tracing, production monitoring, TLS, Kubernetes, Terraform | Missing / accepted                         | Documented future work; not required by current Compose scope                             |

### 4.2 Event and integration status

| Flow                        | Published                    | Consumed                                       | Idempotency                                                                         | Failure path                            |
| --------------------------- | ---------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------- | --------------------------------------- |
| `parcel.created.v1`         | Transactional outbox         | Notification                                   | Processed-event table                                                               | Retry then DLQ                          |
| `parcel.assigned.v1`        | Transactional outbox         | Realtime                                       | In-process only                                                                     | Retry then DLQ                          |
| `parcel.status_changed.v1`  | Courier transaction + outbox | Notification, realtime, merchant sync, webhook | Notification processed events; merchant exact-once snapshot; webhook attempt dedupe | Retry/DLQ; webhook six business retries |
| `notification.requested.v1` | Legacy path                  | Notification                                   | Processed-event table                                                               | Retry then DLQ; SMS permanently fails   |

The event-flow real-stack test proves one courier status change reaches realtime,
merchant order state, the webhook worker and the notification worker. It does not
prove the full delivery lifecycle, proof objects, retry exhaustion or a failed
delivery.

### 4.3 Frontend integration status

All reviewed production screens call real API paths through the shared client;
no hardcoded business-data screen was found. The distinction is in **evidence**:
several screens are connected in code but exercised only with mocked API
responses.

- Real-stack browser evidence: registration/onboarding, seeded role homes,
  merchant orders, merchant webhooks, customer tracking (record created by API),
  dispatcher live status/assignment observation, courier `PICKED_UP`, event
  flow, admin notification logs, admin reads, gateway security.
- Mocked-only browser evidence: customer wizard/list/detail/receipt, dispatcher
  assign/reassign actions, courier delivered/failed/proof upload, admin
  user/merchant/pricing mutations, password reset and Google error paths.
- No mock API is used by production screen code.

### 4.4 Post-merge CI

The CI workflow triggered on `9b719c8` is run `43677865852`. It completed with
**success**: 31 jobs, no failures, and both `Required CI gate` and `Security
gate` passed. This confirms the merged result, not only the PR head, satisfies
the required gates. The PR-head check set was already green on `4eef1ba`.

## 5. Findings

No **P0 – Critical** finding was identified. The merged commit is deployable to
the local Compose stack, required checks pass, and no confirmed exploitable
critical/high-severity application vulnerability was found.

### 5.1 P1 — High

| ID    | Area                         | Description                                                                                                                                                                                                         | Evidence                                                                                                                                                                                                                                                                                                                          | Impact                                                                                                                                      | Recommendation                                                                                                                                                                                                              | Owner | Dependencies                                 | Acceptance criteria                                                                                                |
| ----- | ---------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ----- | -------------------------------------------- | ------------------------------------------------------------------------------------------------------------------ |
| P4-01 | Cross-role demo              | The core customer→parcel→dispatcher→courier→completion workflow is not exercised as one browser-driven journey. Pieces are proven separately through API calls, mocked screens and one partial courier path.        | Real-stack customer creates by `POST /api/v1/parcels` (`apps/web-e2e/src/customer-parcel-tracking.real.spec.ts:73`); assignment is API-driven (`assignment-live.real.spec.ts:85`); courier stops at `PICKED_UP` (`courier-parcel-workflow.real.spec.ts:108`); proof/delivery UI is mocked (`courier-deliveries.spec.ts:679,824`). | The team can show isolated features, but cannot yet demonstrate the complete business story from the UI with one repeatable acceptance run. | Add one cross-role acceptance spec that signs in as customer, dispatcher and courier, completes the visible happy-path journey, then asserts tracking, dispatcher state, courier history, notification log and order state. | M10   | M5, M6, M7                                   | One real-stack spec passes in CI and records the created parcel, assignment, completion and downstream evidence.   |
| P4-02 | Courier completion and proof | `DELIVERED` with recipient name and `FAILED` with reason are tested only as mocked UI and backend transitions; proof upload/view has no real-stack browser coverage.                                                | Full transition matrix and proof HTTP tests exist; mocked proof tests at `courier-deliveries.spec.ts:679,824`; no real-stack spec contains `proof` (`rg` over `*.real.spec.ts`); real-stack only selects `PICKED_UP` (`courier-parcel-workflow.real.spec.ts:108`).                                                                | Proof of delivery is a core acceptance requirement and is not demonstrable end to end against real storage, ownership and history.          | Add real-stack specs for (a) OUT_FOR_DELIVERY→DELIVERED with recipient and photo, (b) →FAILED with reason, then view the proof and verify customer/dispatcher/tracking states.                                              | M7    | M6 assignment; proof storage; M10 acceptance | Both terminal paths pass in CI; proof metadata and photo are retrievable; history shows the transition and reason. |
| P4-03 | Customer UI journey          | The customer wizard, owned list, detail timeline and PDF receipt are connected to real APIs but only mocked in browser tests; the real-stack customer test creates through the API and checks only public tracking. | `customer-parcel-tracking.real.spec.ts:73` creates via request; receipt download is mocked at `parcels.spec.ts:481`; no real-stack `/new` or `/parcels/{id}` journey.                                                                                                                                                             | The main customer-facing workflow has not been proven through its actual screens, form validation or receipt download.                      | Add a real-stack journey: `/new` completes with real zones/quote, then `/parcels` and `/parcels/{id}` show the parcel/history and download a real PDF receipt.                                                              | M5    | M4 pricing; M10 acceptance                   | One real-stack spec covers wizard creation, owned list/detail/history and a valid PDF receipt.                     |

### 5.2 P2 — Medium

| ID    | Area                                   | Description                                                                                                                                                                                                                                                                                                                                           | Evidence                                                                                                                                                                                                                                                                                                   | Impact                                                                                                                                     | Recommendation                                                                                                                                                                         | Owner        | Dependencies                      | Acceptance criteria                                                                                         |
| ----- | -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------ | --------------------------------- | ----------------------------------------------------------------------------------------------------------- |
| P4-04 | Dispatcher UI action                   | The dispatcher can assign/reassign in the UI, but the real-stack test assigns through the API and only observes the row change.                                                                                                                                                                                                                       | `assignment-live.real.spec.ts:85` calls `assignParcel`; the modal and error states are mocked in `dispatch.spec.ts`.                                                                                                                                                                                       | A browser regression in the assignment modal or eligibility error path would not be caught against the real stack.                         | Extend the real-stack dispatcher journey to open the modal, select a courier, submit, and observe success plus one eligibility/conflict failure.                                       | M6           | M6 assignment API; M10 acceptance | Modal assignment and a real failure path pass in CI without direct API assignment.                          |
| P4-05 | Webhook retries and rotation semantics | Real-stack webhook testing proves save/rotate/test/log/isolation and attempt 1; it does not prove the 1m/5m/30m/2h/6h retry sequence, dead-letter outcome, or that the old secret stops signing after rotation.                                                                                                                                       | Real-stack webhook test saves/rotates and asserts the first attempt (`merchant-webhooks.real.spec.ts`); unit tests cover retry scheduling (`apps/services/webhook-worker/internal/worker/worker_test.go`); event-flow asserts attempt 1 only (`event-flow.real.spec.ts:52`).                               | The externally visible retry contract is implemented but only unit-tested; a working retry demonstration is not available.                 | Add a controllable webhook sink and an accelerated real-stack test for a failing target, attempt progression and suppressed/rotated secret behaviour.                                  | M8           | M1 webhook worker; M10 acceptance | Retry attempts are recorded in order, final failure is dead-lettered, and a rotated-out secret is rejected. |
| P4-06 | Admin mutations                        | Real-stack admin coverage is read-only. User create/edit/status, merchant approve/reject/suspend through the UI, and pricing/zone edits remain mocked.                                                                                                                                                                                                | `admin-quality.real.spec.ts` reads users/pricing/reports; `admin-user-create.spec.ts`, `admin-user-edit.spec.ts`, `admin-merchants-actions.spec.ts`, `admin-pricing.spec.ts` and `admin-zones.spec.ts` are mocked.                                                                                         | Administrative operations can regress in the UI without a real-stack failure, and the reports screen is only asserted with an empty range. | Add real-stack admin journeys for user create/edit, merchant decision actions, pricing/zone edits and a reports range with parcel data.                                                | M2/M4/M8/M10 | Seeded admin; M10 acceptance      | Admin mutation journeys pass in CI and reports render a known non-empty dataset.                            |
| P4-07 | Access control tests                   | The 40-case matrix still omits routes such as webhook test/deliveries, courier proof upload/photo read, receipt/download variants, admin merchant detail/actions, and merchant order creation.                                                                                                                                                        | `tests/access/matrix_test.go` lists the current 40 cases; omitted routes are registered in `apps/services/merchant/internal/http/webhook.go`, `apps/services/parcel/internal/http/courier_proof.go`, `apps/services/parcel/internal/http/parcels.go` and `apps/services/merchant/internal/http/orders.go`. | New protected routes can be added with a wrong role requirement without the matrix failing.                                                | Extend the matrix to every public route family and document intentional exclusions (internal-only, unsupported methods, API-key-only).                                                 | M9           | M1 CI                             | Matrix covers all public registered routes or records an explicit exception.                                |
| P4-08 | Documentation truth                    | Operations and README still contradict the merged behaviour: operations says webhook-worker has no HTTP readiness and realtime does not check consumers; it lists stale migration versions; README says courier proof is live-stack covered and admin/pricing/reports remain mocked; `.env.example` still says email verification is not implemented. | `docs/operations.md:161-182` and `:121-125`; `README.md:738`; `.env.example:203`.                                                                                                                                                                                                                          | Operators and assessors receive false readiness, migration and test-coverage claims, which undermines the Phase 3 documentation goal.      | Reconcile these files with the implemented readiness matrix, current migration versions (identity 6, pricing 2, parcel 7, merchant 11, notification 2) and actual real-stack coverage. | M1/M9        | None                              | Documentation statements are verified against source and a review check finds no contradiction.             |
| P4-09 | Performance evidence                   | The local k6 baseline covers only public API, auth and WebSocket; there is no parcel creation/assignment/status, merchant order, webhook, outbox drain or consumer-throughput scenario.                                                                                                                                                               | `docs/phase-3-performance-baseline.md`; `tests/perf/public.js`, `auth.js`, `websocket.js`.                                                                                                                                                                                                                 | The platform's critical write path and asynchronous drain have no repeatable performance evidence.                                         | Add a small authenticated scenario for parcel creation and assignment, plus an async latency/drain observation; record environment and limits.                                         | M1/M10       | Seeded users and stack            | Scripts run against Compose and results are recorded with thresholds and caveats.                           |
| P4-10 | Failure recovery regression            | Broker/identity outage recovery was verified manually in Phase 2/3, but no CI job asserts worker/outbox readiness, consumer reconnection or message drain after a broker restart.                                                                                                                                                                     | Readiness implementation and manual evidence in `docs/phase-3-testing-documentation-qa.md`; no corresponding CI test.                                                                                                                                                                                      | A regression in readiness or reconnect behaviour would not block a merge.                                                                  | Add a Compose-based integration test that stops/starts RabbitMQ and asserts `/ready`, consumer recovery and a processed event.                                                         | M1           | M7/M8 consumers                   | CI test fails if any required `/ready` remains green during the outage or does not recover.                 |
| P4-11 | OpenAPI route drift                    | OpenAPI tests validate parse/refs/operation IDs and a critical-path allowlist, but do not compare the full documented route set with the Gin routers or Nginx map.                                                                                                                                                                                    | `tests/contracts/openapi.test.mjs` critical-path checks; route registrations in service routers.                                                                                                                                                                                                           | A newly registered route or changed path can drift from its contract and gateway mapping while tests stay green.                           | Add a route inventory test (or generated route manifest) that compares public Gin paths and Nginx mappings with the OpenAPI files.                                                     | M4/M9        | M1 CI                             | Every public route has a contract entry and gateway mapping, with an explicit exception list.               |

### 5.3 P3 — Low

| ID    | Area                           | Description                                                                                                                                   | Evidence                                                                                                                                    | Impact                                                                                                                       | Recommendation                                                                                                       | Owner        | Dependencies                | Acceptance criteria                                                                       |
| ----- | ------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------- | ------------ | --------------------------- | ----------------------------------------------------------------------------------------- |
| P4-12 | Auth browser journeys          | Password reset and Google login have backend/mocked coverage but no real-stack browser journey; Google's live provider path is not exercised. | `apps/web-e2e/src/auth.spec.ts` mocks reset/Google UI; real-stack suite covers registration/login only; Google docs record non-CI evidence. | Lower risk than the core delivery flow, but the reset link and provider redirect remain unproven in the current environment. | Add one Mailpit-backed reset journey and a controlled opt-in Google smoke using test credentials.                    | M2/M3        | Mailpit; Google test client | Reset succeeds from email link; opt-in Google smoke passes with documented configuration. |
| P4-13 | Restore and rollback rehearsal | Backup/restore scripts exist, but no automated restore or full rollback rehearsal is in CI.                                                   | `docs/operations.md`; migration tests cover only selected services.                                                                         | Recovery procedures are documented but not continuously proven.                                                              | Add a disposable-database restore rehearsal and broaden rollback tests for the remaining services.                   | M1           | CI database job             | Restore and rollback jobs pass on a disposable database and are documented.               |
| P4-14 | AI live provider               | Deterministic AI tests and an opt-in live smoke exist; normal CI never calls Gemini.                                                          | `scripts/ai-live-smoke.mjs`; AI test docs; Phase 3 F-13 remains partial.                                                                    | Provider regressions are not detected automatically; accepted because the capability is optional and the key is external.    | Run the live smoke only in a protected/manual job with a configured key; keep deterministic tests as the merge gate. | M10/AI owner | Gemini key                  | Manual job records a successful live response without exposing the key.                   |
| P4-15 | Retention decisions            | Merchant order reservations and notification delivery logs have no automatic deletion policy; identity token cleanup is implemented.          | Phase 3 F-16/F-17; `docs/phase-3-testing-documentation-qa.md` remaining limitations.                                                        | Data grows and privacy/audit obligations remain undefined.                                                                   | Obtain an owner-approved retention policy; implement migrations/jobs only after the policy is agreed.                | M8/M1        | Team decision               | Policy is documented and an approved cleanup implementation has tests.                    |

## 6. Existing limitations and accepted technical debt

These are not Phase 4 blockers unless the team changes scope:

- **No SMS delivery.** EMAIL is the implemented notification channel; SMS has no
  provider and is a permanent failure.
- **No SSE, realtime replay or multi-replica fan-out.** Each queued event reaches
  one replica; a reconnect cannot recover missed events.
- **Proof storage requires one Parcel replica or a shared volume** in local
  filesystem mode. Neon Object Storage is the production provider.
- **Shared rate limiting and AI conversation state are process-local.** Multiple
  replicas do not share limits or conversations.
- **No distributed tracing, Prometheus/Grafana deployment or alerting.** Internal
  metrics endpoints exist only.
- **No production TLS, Kubernetes, Terraform or production deployment
  automation.** Compose is a local/demo environment.
- **CSP retains `'unsafe-inline'`** for Next hydration and React inline styles.
- **Google OIDC does not send/validate a nonce or `azp`.** Signature, issuer,
  audience and expiry are validated.
- **No parcel edit/cancel API.** This is outside API v1.1 and must not be
  invented.
- **Merchant order reservation and notification-log retention need an approved
  policy** (P4-15).
- **At-least-once delivery** is intentional. Consumers must remain idempotent;
  exactly-once is not claimed.

## 7. Recommended Phase 4 scope

Phase 4 should be **workflow completion and evidence**, not infrastructure
expansion. The recommended scope has four workstreams, in this order:

1. **Complete the core delivery story in the browser (P1).** One cross-role
   acceptance journey: customer creates through `/new`, dispatcher assigns
   through the UI, courier progresses to `DELIVERED` with proof (and `FAILED`
   with reason), then customer tracking/history, dispatcher state, notification
   log and merchant order state are checked. This is the highest-value work
   because it converts existing isolated features into a demonstrable product.
2. **Close the merchant/admin integration gaps (P2).** Real-stack webhook retry
   and rotation semantics, admin user/merchant/pricing mutations, and a
   reports dataset with parcels. These are the remaining operator journeys that
   currently depend on mocks or API calls.
3. **Make contracts and operations truthful (P2).** Fix the stale readiness,
   migration and coverage documentation; expand the access matrix; add OpenAPI
   route-drift validation. These are cheap, high-leverage changes that prevent
   the next phase from repeating Phase 3's documentation drift.
4. **Add targeted reliability evidence (P2/P3).** Broker/identity outage CI,
   write-path k6 scenarios, restore/rollback rehearsal and an opt-in AI live
   smoke. Do not add Kubernetes, Terraform, tracing, Grafana or multi-replica
   realtime unless the team explicitly expands scope.

Explicitly **not** recommended for Phase 4:

- parcel edit/cancel (not in the API contract);
- SMS provider work;
- a new UI library, state manager or API framework;
- replacing RabbitMQ or the outbox;
- Kubernetes/GKE, Terraform or production TLS;
- broad unit-test volume without a user-visible workflow.

## 8. Suggested M1–M10 allocation

| Owner | Phase 4 responsibility                                                                    | Primary acceptance                                                |
| ----- | ----------------------------------------------------------------------------------------- | ----------------------------------------------------------------- |
| M1    | CI reliability, documentation truth, route/readiness checks, restore/rollback rehearsal   | Green required gate; docs match source; recovery checks run in CI |
| M2    | Admin user create/edit/status real-stack journey; profile/password reset coverage         | Admin UI mutations and reset journey pass against the stack       |
| M3    | Controlled Google live smoke and auth failure-path evidence                               | Opt-in provider smoke and documented configuration                |
| M4    | Customer wizard and pricing/admin read-write real-stack coverage                          | `/new` uses real quote; pricing edits verified                    |
| M5    | Customer list/detail/history/receipt real-stack journey                                   | Owned parcel and PDF receipt proven in browser                    |
| M6    | Dispatcher UI assign/reassign and failure-path journey                                    | Modal actions pass without API shortcuts                          |
| M7    | Courier delivered/failed/proof real-stack journey; broker outage regression               | Both terminal paths and proof retrieval pass                      |
| M8    | Webhook retry/dead-letter/rotation real-stack journey; order idempotency browser evidence | Retry attempts and rotated secret behavior proven                 |
| M9    | Access-matrix expansion, requirements traceability, doc review                            | Every public route classified; no stale claims                    |
| M10   | Cross-role acceptance spec, reports dataset, final demo evidence                          | One repeatable end-to-end run with recorded artifacts             |

## 9. Dependencies and implementation order

```text
M1 documentation/CI truth + M9 access matrix
        |
        v
M6 dispatcher UI assignment ──> M7 courier completion + proof ──> M5 customer UI journey
        |                              |                                  |
        +──────────────> M10 cross-role acceptance <──────────────────────+
                               |
                               v
                  M4 pricing/admin, M8 webhooks, M2/M3 auth
                               |
                               v
                  M1 recovery/performance evidence
```

Hard dependencies:

- P4-01 depends on P4-02 and P4-03; do not report the cross-role journey complete
  until both are merged.
- P4-02 depends on a current assignment and proof storage; use the existing
  courier API and assignment path.
- P4-05 depends on a controllable webhook receiver; do not use an external
  placeholder for retry assertions.
- P4-06 depends on seeded admin accounts and real report data; create data through
  the normal API path, not direct SQL.
- P4-07/P4-11 depend on the route inventory being complete; add routes and
  contracts together.

## 10. Acceptance criteria and exit conditions

Phase 4 is complete when all of the following are true:

1. The cross-role acceptance spec passes against a clean Compose stack and
   records a created parcel, assigned courier, delivered/FAILED outcome, proof
   (where applicable), tracking state and downstream notification/order state.
2. Courier `DELIVERED` with recipient, `FAILED` with reason and proof photo
   upload/view are covered by real-stack tests, not mocks alone.
3. Customer `/new`, `/parcels`, `/parcels/{id}` history and PDF receipt are
   covered by one real-stack journey.
4. Dispatcher assign/reassign and at least one assignment failure are exercised
   through the UI against the real stack.
5. Admin user and merchant mutations, pricing/zone edits and a non-empty reports
   dataset pass real-stack browser tests.
6. Webhook retry progression and rotated-secret rejection are demonstrated
   against a controllable receiver.
7. Access matrix and OpenAPI route inventory cover every public route or record
   an approved exception.
8. `docs/operations.md`, README and `.env.example` match the merged readiness,
   migration and test-coverage facts.
9. Broker/identity outage recovery is a CI regression test, not only a manual
   observation.
10. Required CI is green on the final Phase 4 baseline; no new P0/P1 finding is
    open.

## 11. Risks and team decisions

| Risk / decision        | Why it matters                                                                                            | Suggested decision                                                                                                                   |
| ---------------------- | --------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------ |
| Cross-role E2E runtime | Real-stack tests are already slow and login-rate-limited; one long journey can become flaky.              | Keep one deterministic acceptance spec, reuse API setup for non-UI prerequisites, and preserve the existing rate-limit retry helper. |
| Webhook retry timing   | The production schedule reaches 6 hours; a real-stack test cannot wait for it.                            | Add a test-only accelerated schedule or assert recorded attempt scheduling/attempt rows rather than wall-clock delivery.             |
| Real report data       | The seeded database has no parcel history, so reports are empty.                                          | Create a known parcel through the normal API in the test, then assert daily/outcome values for the selected range.                   |
| Proof storage in CI    | Local filesystem proof storage is single-replica.                                                         | Keep one Parcel replica or a shared volume in the test stack; do not add multi-replica proof claims.                                 |
| Retention policy       | Merchant reservation and notification-log deletion cannot be guessed without affecting idempotency/audit. | Require an owner-approved policy before implementation (P4-15).                                                                      |
| Scope expansion        | Infrastructure work can consume the phase without improving the demo.                                     | Freeze the four workstreams above; require explicit approval for Kubernetes, Terraform, tracing or multi-replica realtime.           |
| Google/AI credentials  | External providers cannot be a normal CI dependency.                                                      | Keep deterministic tests in the required gate; run provider smokes in protected/manual jobs.                                         |

## 12. Evidence index

Primary evidence used in this audit:

- Merge and baseline: `git show --stat 9b719c8`, `git log -1 --format=fuller`,
  remote `development` fast-forward.
- Required checks: PR #156 check rollup and post-merge run `43677865852`.
- Routes and boundaries: service `internal/http` routers, `infra/nginx/default.conf`,
  `contracts/openapi/*.yaml`, `contracts/events/*.schema.json`.
- Customer: `apps/services/parcel/internal/http/parcels.go`,
  `apps/web/src/components/parcels/*`, `customer-parcel-tracking.real.spec.ts`,
  `parcels.spec.ts`.
- Dispatcher: `apps/services/parcel/internal/http/dispatch.go`,
  `dispatch.spec.ts`, `dispatch-live.spec.ts`, `assignment-live.real.spec.ts`.
- Courier: `courier.go`, `courier_status.go`, `courier_proof.go`,
  `statemachine.go`, `courier-deliveries.spec.ts`,
  `courier-parcel-workflow.real.spec.ts`.
- Merchant: `merchant/internal/http/orders.go`, `webhook.go`, `webhook_internal.go`,
  `merchant/internal/merchantsync`, `merchant-orders.real.spec.ts`,
  `merchant-webhooks.real.spec.ts`, `event-flow.real.spec.ts`.
- Admin: `admin-quality.real.spec.ts` and the mocked admin specs listed in P4-06.
- Events/readiness: `packages/go-common/messaging`,
  `packages/go-common/eventschema`, `packages/go-common/httpx/readiness.go`,
  `docs/phase-3-testing-documentation-qa.md`.
- Operations/docs drift: `docs/operations.md:121-182`, `README.md:726-738`,
  `.env.example:196-203`.
- CI: `.github/workflows/ci.yml`; access matrix `tests/access/matrix_test.go`;
  OpenAPI test `tests/contracts/openapi.test.mjs`; performance `tests/perf`.

This report is an assessment and planning artifact. It does not modify
production code, contracts, migrations or infrastructure.
