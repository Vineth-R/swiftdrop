# Phase 4 Solo Implementation Plan

| Field                                  | Value                                                                                |
| -------------------------------------- | ------------------------------------------------------------------------------------ |
| Status                                 | Planning only — no implementation started                                            |
| Audit source                           | [`docs/phase-4-gap-audit.md`](phase-4-gap-audit.md)                                  |
| Application-code audit baseline        | `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`                                           |
| Current `development` at planning time | `c7665faaa598c0a656e49161d5321014d2eed594`                                           |
| Audit report merge                     | PR #157; documentation only                                                          |
| Delivery model                         | One developer, sequential milestones                                                 |
| Effort estimate                        | **21–30 developer working days**, including tests and CI iteration; not a commitment |

## 1. Objectives and scope

Phase 4 completes the business workflows that already have backend foundations
but lack browser-level, real-stack evidence. It is an integration and acceptance
phase, not a platform-expansion phase.

The phase is complete when one developer can demonstrate, repeatably:

1. a customer creates a parcel through the real UI;
2. the customer sees it in the owned list and detail/history view;
3. a dispatcher assigns a courier through the real UI;
4. the courier progresses the delivery through the supported state machine;
5. the courier completes it with recipient details and proof, or records a
   failure with a reason;
6. customer tracking, dispatcher state, notifications and merchant order state
   reflect the outcome;
7. the same evidence is produced by the repository's automated tests and CI.

The plan preserves service ownership, API v1.1 contracts, the existing RabbitMQ
topology and the current Docker Compose architecture. It does not add new
business features.

## 2. Audit baseline and repository verification

The audit report is anchored to application commit `9b719c8`. At planning time,
`development` is `c7665fa`; the only change between them is the addition of
`docs/phase-4-gap-audit.md` in PR #157.

Verification performed for this plan:

- `git diff --stat 9b719c8..c7665fa` changes one documentation file only.
- No application code, test harness, migration, contract or CI workflow changed
  after the audit baseline.
- The audit's 15 findings therefore remain current as implementation gaps.
- One audit detail was refined against the live route matrix: P4-07's prose
  names a few routes that are already present in `tests/access/matrix_test.go`.
  The verified omission list used below is the authoritative scope for that
  task.

Audit baseline for implementation:

```text
Branch: development
Application-code commit: 9b719c8daad32dc1a541e02da8d0ad4b768d65a5
Planning HEAD: c7665faaa598c0a656e49161d5321014d2eed594
PR: #156
```

## 3. Workstreams

| Workstream                               | Purpose                                                                | Findings                          |
| ---------------------------------------- | ---------------------------------------------------------------------- | --------------------------------- |
| A — Core delivery workflow               | Prove the customer, dispatcher and courier journey through the real UI | P4-01, P4-02, P4-03, P4-04        |
| B — Merchant and administration          | Close webhook retry/rotation and admin mutation evidence               | P4-05, P4-06                      |
| C — Quality and contract consistency     | Make access, documentation and route contracts truthful                | P4-07, P4-08, P4-11, P4-12        |
| D — Reliability and operational evidence | Add recovery, performance, restore and optional provider evidence      | P4-09, P4-10, P4-13, P4-14, P4-15 |

## 4. Finding-by-finding implementation details

### P4-01 — Cross-role end-to-end acceptance

- **Original priority:** P1
- **Current status after verification:** Open. No application change since the
  audit baseline.
- **What to implement:** One real-stack Playwright acceptance spec that performs
  the customer creation, dispatcher assignment and courier completion through
  the browser, then verifies tracking, dispatcher state, notification records
  and (where applicable) merchant order state.
- **Likely files:**
  - new `apps/web-e2e/src/customer-delivery-journey.real.spec.ts`
  - `apps/web-e2e/src/support/real-stack.ts` (prefer adding small read-only
    verification helpers, not new business logic)
  - existing customer, dispatch, courier and notification screens only if a
    real defect is exposed
- **Dependencies:** P4-02, P4-03 and P4-04 must be complete. The merchant-order
  observation depends on the existing event-flow/order APIs; API 8.16 remains
  API-key-only.
- **Steps:**
  1. Sign in as the seeded customer through the login form.
  2. Complete `/new` through the real zones/quote flow and submit the parcel.
  3. Assert the parcel appears in `/parcels`, open `/parcels/{id}`, and verify
     its history and receipt.
  4. Sign in as the seeded dispatcher and assign the parcel through the actual
     assignment modal, not the API.
  5. Sign in as the seeded courier, open Today/detail, progress the parcel
     through the real UI and complete it with a recipient/proof (or use the
     failure variant from P4-02).
  6. Verify the customer tracking page, dispatcher All parcels row, courier
     history, ADMIN notification log and merchant order state.
  7. Keep setup limited to read-only API checks and unique test data; do not
     replace a UI interaction under test with an API call.
- **Required tests:** New real-stack spec; full real-stack project; mocked
  Chromium suite for regressions; relevant Go/API tests only if application code
  changes.
- **Definition of done:** One CI real-stack run passes the complete journey and
  records the parcel, assignment, terminal status, proof/failure evidence and
  downstream state. The spec is deterministic and serial-safe with the shared
  login limiter.
- **Blocks next milestone:** Yes. This is the Phase 4 acceptance gate.

### P4-02 — Courier completion and proof of delivery

- **Original priority:** P1
- **Current status after verification:** Open. Backend transitions and proof
  endpoints exist; real-stack UI coverage stops at `PICKED_UP`.
- **What to implement:** Real-stack browser coverage for the terminal paths:
  `OUT_FOR_DELIVERY → DELIVERED` with recipient name and photo, and
  `OUT_FOR_DELIVERY → FAILED` with a reason. Verify proof metadata/photo
  retrieval and the resulting history.
- **Likely files:**
  - new `apps/web-e2e/src/courier-completion.real.spec.ts`
  - `apps/web-e2e/src/support/real-stack.ts`
  - only if a real defect appears: `apps/web/src/components/courier/*`,
    `apps/services/parcel/internal/http/courier_*`,
    `apps/services/parcel/internal/courier/*`
- **Dependencies:** P4-04 assignment path; existing proof storage; seeded
  courier account; P4-01 consumes this evidence.
- **Steps:**
  1. Create a customer parcel and assign it to the seeded courier (API setup is
     acceptable here because the courier UI is the system under test).
  2. Move the parcel through `PICKED_UP`, `IN_TRANSIT` and
     `OUT_FOR_DELIVERY` through the courier UI.
  3. Upload proof with a small valid JPEG/PNG and recipient name, then verify
     the metadata and open the short-lived photo.
  4. Complete a second parcel with `FAILED` and a valid reason; verify the
     failure appears in history and is not treated as delivered.
  5. Verify customer tracking/detail and the dispatcher row reflect both
     outcomes.
- **Required tests:** New real-stack terminal-path spec; existing mocked proof
  tests; parcel Go status/proof tests; full real-stack run.
- **Definition of done:** Both terminal paths pass in CI; a proof photo is
  retrievable; the failed reason and delivered recipient are persisted and
  visible; no mock API is used for the courier UI.
- **Blocks next milestone:** Yes; P4-01 requires it.

### P4-03 — Customer UI journey

- **Original priority:** P1
- **Current status after verification:** Open. The real-stack customer spec
  creates through `POST /api/v1/parcels` and checks only public tracking; the
  wizard/list/detail/receipt flows are mocked.
- **What to implement:** One real-stack browser journey through `/new`,
  `/parcels`, `/parcels/{id}` history and a real PDF receipt download.
- **Likely files:**
  - new `apps/web-e2e/src/customer-parcel-journey.real.spec.ts`
  - `apps/web-e2e/src/support/real-stack.ts`
  - only if a real defect appears: `apps/web/src/components/parcels/*`,
    `apps/web/src/lib/parcels-api.ts`
- **Dependencies:** real zones/pricing seed; seeded customer; Milestone 1 test helpers
  for unique data.
- **Steps:**
  1. Sign in as the customer through the form.
  2. Open `/new`, select a real priced route, choose a size/weight, wait for the
     real estimate, complete sender/recipient fields and submit.
  3. Assert the success state and navigate to `/parcels`; verify the new parcel
     is listed and can be opened.
  4. Verify detail fields, history timeline and the real receipt download.
  5. Capture the download and assert a PDF response/filename; do not stub the
     receipt route.
- **Required tests:** New real-stack spec; existing mocked wizard/receipt
  tests; frontend lint/typecheck/build; full real-stack run.
- **Definition of done:** The customer journey passes against the real stack
  through the actual screens and downloads a valid receipt.
- **Blocks next milestone:** Yes; P4-01 requires it.

### P4-04 — Dispatcher UI assignment and failure handling

- **Original priority:** P2
- **Current status after verification:** Open. The real-stack test assigns via
  the API and observes the row; the modal and failure states are mocked.
- **What to implement:** Dispatcher real-stack UI coverage for assignment,
  reassignment and one documented failure path.
- **Likely files:**
  - new `apps/web-e2e/src/dispatcher-assignment.real.spec.ts` or an extension
    of `assignment-live.real.spec.ts`
  - `apps/web-e2e/src/support/real-stack.ts`
  - only if a real defect appears: `apps/web/src/components/dispatch/*`,
    `apps/services/parcel/internal/http/dispatch.go`
- **Dependencies:** P4-03 (a parcel to assign, or an API-created fixture);
  seeded dispatcher and couriers.
- **Steps:**
  1. Create a parcel fixture with unique data.
  2. Sign in as the dispatcher, open All parcels, find the parcel and open the
     assignment modal.
  3. Select an eligible courier through the UI and submit; verify success and
     the updated row.
  4. Reassign the same parcel and verify the new courier appears.
  5. Exercise one documented refusal (for example an ineligible/closed parcel)
     through the UI and verify the safe error state and retry.
- **Required tests:** New/extended real-stack spec; existing dispatch mocked
  tests; parcel dispatch Go tests if code changes; full real-stack run.
- **Definition of done:** Assignment/reassignment and one failure path pass
  without a direct API call for the dispatcher UI action.
- **Blocks next milestone:** Yes; P4-01 requires the assignment step.

### P4-05 — Webhook retries, dead-letter and rotation semantics

- **Original priority:** P2
- **Current status after verification:** Open. Save/rotate/test/log/isolation
  and attempt 1 are real-stack tested; the six-attempt sequence and old-secret
  rejection are unit-tested only.
- **What to implement:** A controllable delivery target strategy and evidence
  for at least one real retry, the recorded attempt sequence, final
  dead-letter/suppression behavior and rotation semantics.
- **Likely files:**
  - `apps/web-e2e/src/merchant-webhook-retries.real.spec.ts` (new)
  - `apps/web-e2e/src/support/real-stack.ts`
  - `apps/services/webhook-worker/internal/worker/*` only if a test-only
    scheduling hook is approved
  - `apps/services/merchant/internal/http/webhook*` only if a real defect is
    found
  - `.github/workflows/ci.yml` if a protected/manual retry job is added
- **Dependencies:** P4-06 admin/merchant setup is not required, but the existing
  seeded/admin approval flow is; a decision on the controllable receiver.
- **Steps:**
  1. Register and approve a merchant; configure a webhook through the existing
     UI.
  2. Point it at a controlled failing target and trigger a real status event.
  3. Wait for the first retry and verify recorded attempts and next-retry
     metadata through the merchant delivery API/UI.
  4. Prove final dead-letter/suppression behavior with an accelerated test-only
     schedule or the existing worker integration tests; do not wait six real
     hours in CI.
  5. Rotate the secret, verify the new secret is returned once, and verify the
     worker uses it; prove the old secret is no longer accepted by a controlled
     receiver or the keyring/unit boundary.
- **Required tests:** New real-stack retry/rotation spec; webhook worker unit
  tests for six attempts/dead-letter; merchant webhook integration tests;
  event-flow regression.
- **Definition of done:** The chosen receiver strategy is documented; attempt
  ordering, one real retry, scheduled retry metadata, final failure behavior and
  rotation are evidenced; no external placeholder is presented as proof.
- **Blocks next milestone:** No. It is independent of the core acceptance flow;
  complete it after P4-01 if the receiver decision is unresolved.

### P4-06 — Admin mutations and reports

- **Original priority:** P2
- **Current status after verification:** Open. Real-stack coverage reads users,
  pricing and reports; create/edit/status, merchant decisions through the UI,
  pricing/zone edits and non-empty reports remain mocked or empty.
- **What to implement:** Real-stack browser journeys for admin user creation,
  edit/status, merchant approve/reject/suspend through the UI, pricing/zone
  edits and a reports range containing known parcel data.
- **Likely files:**
  - `apps/web-e2e/src/admin-mutations.real.spec.ts` (new)
  - `apps/web-e2e/src/support/real-stack.ts`
  - `apps/web-e2e/src/admin-quality.real.spec.ts` (reports dataset extension)
  - screens under `apps/web/src/components/admin/*` and
    `apps/web/src/components/pricing/*` only if defects are exposed
- **Dependencies:** Seeded ADMIN account; a customer-created parcel for reports;
  unique user/merchant data; no direct SQL writes.
- **Steps:**
  1. Sign in as ADMIN and create a user through `/admin/users/new`.
  2. Edit the user and change status through the actual screens; verify the
     list/detail reflects it.
  3. Register a merchant fixture and approve/reject/suspend it through the admin
     UI; verify the status and owner behaviour.
  4. Create a pricing rule/zone edit through the UI and verify it affects a new
     estimate.
  5. Create a known parcel through the normal customer API/UI, then open
     `/reports` and assert non-empty daily/outcome data for the range.
- **Required tests:** New real-stack admin spec; existing mocked admin specs;
  identity/pricing/merchant Go integration tests if code changes; full
  real-stack run.
- **Definition of done:** Admin mutations and a non-empty reports dataset pass
  in CI while preserving seeded-data isolation.
- **Blocks next milestone:** No. It can run after P4-01.

### P4-07 — Access-control matrix expansion

- **Original priority:** P2
- **Current status after verification:** Open, with a corrected scope. Some
  routes named in the audit prose are already present in the 40-case matrix.
  Verified omissions include:
  - `POST /merchants/me/webhook/test`
  - `GET /merchants/me/webhook/deliveries`
  - `POST /courier/deliveries/{id}/proof`
  - `GET /parcels/{id}/proof/photo/content`
  - `POST /merchant/orders` (API-key-only; record as an explicit matrix
    exception or add a dedicated API-key access test)
  - `GET /admin/users/{id}`
  - pricing writes: `POST/PATCH /admin/pricing/rules`,
    `GET/POST/PATCH /admin/pricing/bands`,
    `GET/PUT /admin/pricing/surcharges`
  - `PATCH /admin/zones/{id}`
  - authenticated `GET /parcels/tracking/{trackingNumber}` and logout, where
    the access suite's fixtures can support them
- **What to implement:** Add cases for every protected public route, mark
  intentional exceptions, and document the exception policy.
- **Likely files:** `tests/access/matrix_test.go`, `tests/access/README.md`,
  `tests/access/accesskit/*` only if the helper needs a new case shape.
- **Dependencies:** Milestone 1 route inventory (P4-11) so omissions are measured against
  a complete list.
- **Steps:**
  1. Freeze the current route inventory from Gin registrations and Nginx.
  2. Add matrix cases with safe bodies that the handlers reject.
  3. Add explicit `Missing`/exception entries only for API-key-only or
     intentionally non-session routes.
  4. Run the matrix against Compose and confirm every route is 401/403/allow as
     documented.
- **Required tests:** `make test-access`, `go test ./tests/access/accesskit/`,
  and CI access job.
- **Definition of done:** Matrix covers every public protected route or records
  an approved exception; a deliberately broken guard still fails the suite.
- **Blocks next milestone:** No, but complete before accepting new routes.

### P4-08 — Documentation truth

- **Original priority:** P2
- **Current status after verification:** Open. The audit report is merged, but
  the operational contradictions remain.
- **What to implement:** Correct `docs/operations.md`, `README.md` and
  `.env.example` against the merged implementation and actual test coverage.
- **Likely files:** `docs/operations.md`, `README.md`, `.env.example`,
  `docs/architecture.md` if the same claims appear there.
- **Dependencies:** None.
- **Steps:**
  1. Replace the stale readiness bullets with the actual matrix: identity and
     pricing require PostgreSQL; parcel includes the relay; merchant includes
     sync; realtime includes both consumers; notification includes all three;
     webhook-worker exposes `/ready` on 8088.
  2. Correct migration versions to identity 6, pricing 2, parcel 7, merchant 11,
     notification 2.
  3. Remove the false live-stack proof claim and the stale
     "admin/pricing/reports are mocked" statement; describe the real reads and
     remaining mutations accurately.
  4. Remove the stale email-verification comment in `.env.example`.
  5. Check `docs/architecture.md` for the same readiness/coverage claims.
- **Required tests:** Prettier check; manual source-to-doc review; no code test
  required.
- **Definition of done:** No reviewed statement contradicts source or current
  test evidence.
- **Blocks next milestone:** No, but do it first so later milestone PRs do not
  inherit false claims.

### P4-09 — Performance evidence

- **Original priority:** P2
- **Current status after verification:** Open. Public/auth/WebSocket k6 scripts
  exist; write-path and asynchronous drain evidence is absent.
- **What to implement:** A small authenticated k6 scenario for parcel creation
  and assignment, plus an observable async-drain/status scenario, with a
  recorded baseline and explicit local-only caveats.
- **Likely files:** `tests/perf/parcels.js`, `tests/perf/flow.js` (new),
  `tests/perf/README.md`, `docs/phase-4-performance-baseline.md`, possibly the
  Makefile.
- **Dependencies:** Seeded customer/dispatcher/courier accounts; running
  Compose; P4-03/P4-04 for realistic data if needed.
- **Steps:**
  1. Add a low-VU authenticated creation scenario with unique idempotency keys.
  2. Add assignment/status setup and poll for the resulting tracking/order state
     with a bounded timeout.
  3. Record latency/error thresholds and the local environment.
  4. Keep scripts local-only; do not point them at shared infrastructure.
- **Required tests:** Run k6 locally, record results; no required CI performance
  gate unless the project owner approves one.
- **Definition of done:** Scripts run against Compose and a baseline document
  records the command, environment, results and limitations.
- **Blocks next milestone:** No.

### P4-10 — Broker and identity outage recovery

- **Original priority:** P2
- **Current status after verification:** Open. Readiness is implemented, but
  recovery is manual evidence only.
- **What to implement:** A CI-able recovery check that stops RabbitMQ (and
  optionally Identity), asserts affected `/ready` endpoints go down, restarts
  the dependency, waits for recovery, and verifies a processed event.
- **Likely files:** `tests/recovery/*` or `tests/scripts/check-outage-recovery.sh`,
  `.github/workflows/ci.yml`, `docker-compose.yml` only if a test-only helper is
  necessary.
- **Dependencies:** Existing readiness endpoints; event-flow fixtures; a
  dedicated Compose project to avoid interfering with parallel jobs.
- **Steps:**
  1. Start a disposable Compose stack.
  2. Assert baseline readiness.
  3. Stop RabbitMQ; assert parcel/merchant/realtime/notification/webhook
     readiness fails and identity/pricing remain database-ready.
  4. Restart RabbitMQ; poll readiness to success.
  5. Publish or trigger one status event and assert a consumer processes it.
- **Required tests:** New CI job/script; run locally against a disposable stack;
  no disruption to the main real-stack job.
- **Definition of done:** CI fails if readiness lies or recovery does not
  complete; logs identify the dependency checks.
- **Blocks next milestone:** No, but complete before declaring operational
  readiness.

### P4-11 — OpenAPI and route-drift validation

- **Original priority:** P2
- **Current status after verification:** Open. Contract tests parse/ref-check
  and cover a critical-path allowlist, but do not compare the full route set.
- **What to implement:** A route inventory plus automated comparison across
  service routers, OpenAPI specs and the Nginx public map, with a documented
  exception list.
- **Likely files:** `tests/contracts/openapi.test.mjs` or a new
  `tests/contracts/route-inventory.test.mjs`;
  `contracts/openapi/route-inventory.json`; service `router_test.go` files;
  `infra/nginx/default.conf`.
- **Dependencies:** None beyond the existing route registrations. Define this
  inventory first; P4-07 consumes it. Any later service route change updates the
  inventory in the same PR.
- **Steps:**
  1. Define a canonical public route inventory with method, public path,
     service and contract file.
  2. Add a test in each service's HTTP package that compares
     `gin.Engine.Routes()` with its manifest subset.
  3. Add a Node test that checks each inventory path exists in the relevant
     OpenAPI spec and Nginx mapping.
  4. Record intentional exceptions (health aliases, probes, API-key-only
     routes).
- **Required tests:** `node --test tests/contracts/*.test.mjs`; affected Go
  router tests; `nginx -t`; CI.
- **Definition of done:** A new or changed public route without a contract or
  gateway entry fails a test; exceptions are explicit.
- **Blocks next milestone:** No, but complete before adding any new routes.

### P4-12 — Auth browser journeys

- **Original priority:** P3
- **Current status after verification:** Open. Password reset and Google login
  are mock-tested; no real-stack reset or live provider smoke.
- **What to implement:** A Mailpit-backed password-reset journey and an optional
  protected Google smoke. This is non-blocking for the core delivery workflow.
- **Likely files:** `apps/web-e2e/src/auth-reset.real.spec.ts`,
  `apps/web-e2e/src/auth-google.real.spec.ts` (optional),
  `apps/web-e2e/src/support/real-stack.ts`.
- **Dependencies:** Mailpit; seeded/customer fixture; Google test credentials
  only for the optional path.
- **Steps:**
  1. Request a reset through `/forgot-password`.
  2. Read the message from Mailpit and extract the token.
  3. Complete `/reset-password`, sign in with the new password, and verify the
     old password fails.
  4. Add a protected/manual Google smoke only if credentials are available.
- **Required tests:** New real-stack reset spec; mocked auth suite unchanged;
  protected Google job if implemented.
- **Definition of done:** Reset is proven from email to login; Google remains
  optional and clearly documented.
- **Blocks next milestone:** No. Optional/stretch.

### P4-13 — Restore and rollback rehearsal

- **Original priority:** P3
- **Current status after verification:** Open. Scripts exist; automated restore
  and full rollback coverage remain incomplete.
- **What to implement:** A disposable-database restore rehearsal and broaden
  rollback tests to the remaining services.
- **Likely files:** `tests/integration/database/*`, `scripts/*`,
  `Makefile`, `.github/workflows/ci.yml`, `docs/operations.md`.
- **Dependencies:** P4-15 retention policy is separate; use a disposable
  database only.
- **Steps:**
  1. Extend rollback tests to pricing, merchant and remaining migration heads.
  2. Add a restore script/test that creates a backup, restores into a uniquely
     named database, and verifies migration status/schema.
  3. Run in a dedicated CI job against a disposable target.
- **Required tests:** Restore rehearsal and expanded rollback tests in CI.
- **Definition of done:** Restore and rollback pass on disposable targets and
  the runbook commands are accurate.
- **Blocks next milestone:** No. Optional/stretch.

### P4-14 — Optional live AI provider smoke

- **Original priority:** P3
- **Current status after verification:** Open/optional. Deterministic tests and
  an opt-in script exist; CI stays offline.
- **What to implement:** A protected/manual job or documented manual procedure
  for the existing `scripts/ai-live-smoke.mjs`; do not make Gemini a required CI
  dependency.
- **Likely files:** `scripts/ai-live-smoke.mjs`, `.github/workflows/*` (manual
  workflow only), `docs/ai-assistant.md`.
- **Dependencies:** A valid Gemini key supplied outside source control.
- **Steps:** Document the command/environment, run it manually, record the
  result without exposing the key, and keep deterministic tests as the merge
  gate.
- **Required tests:** Manual smoke evidence; no required CI call.
- **Definition of done:** The smoke procedure is repeatable and protected;
  normal CI remains deterministic and offline.
- **Blocks next milestone:** No. Optional.

### P4-15 — Retention policy decisions

- **Original priority:** P3
- **Current status after verification:** Open/decision pending. Identity token
  cleanup exists; merchant order reservations and notification delivery logs
  have no deletion policy.
- **What to implement:** A written, approved policy before any cleanup code.
  This planning task does not choose a duration.
- **Likely files:** a new policy document under `docs/`; later implementation
  would touch `migrations/merchant`, `migrations/notification`, worker/repository
  jobs and tests, but only after approval.
- **Dependencies:** Product/owner decision on idempotency windows and audit
  retention.
- **Steps:**
  1. Decide retention for terminal merchant order reservations and
     notification logs.
  2. Record whether rows are deleted, anonymized or archived.
  3. Define the cleanup interval, batch size and safety checks.
  4. Implement only after the policy is approved; do not delete audit-sensitive
     rows by guesswork.
- **Required tests:** Policy review; later implementation needs integration
  tests for retention boundaries and idempotency.
- **Definition of done:** Approved policy exists; cleanup remains unimplemented
  until approval.
- **Blocks next milestone:** No, but unresolved policy is a Phase 4 decision
  item.

## 5. Solo implementation sequence

The sequence is deliberately sequential. Each milestone starts from a merged
`development` after the previous PR is green.

| Order | Milestone                        | Why here                                                                                          | Estimate |
| ----- | -------------------------------- | ------------------------------------------------------------------------------------------------- | -------- |
| 1     | Foundation and quality           | Fixes shared documentation, access and contract infrastructure before workflow specs depend on it | 2–3 days |
| 2     | Customer workflow                | Establishes the parcel fixture and proves the customer UI before dispatcher/courier work          | 2–3 days |
| 3     | Dispatcher and courier workflow  | Builds the assignment and terminal-path evidence used by the cross-role test                      | 4–6 days |
| 4     | Cross-role integration           | Combines the completed pieces into the primary acceptance journey                                 | 2–3 days |
| 5     | Merchant and administration      | Adds the remaining operator journeys once the core delivery story is stable                       | 5–7 days |
| 6     | Reliability and final validation | Adds recovery/performance/restore evidence and optional decisions last                            | 6–8 days |

**Total estimate: 21–30 developer working days**, including test authoring,
debugging, CI iteration and review time. If only P1/P2 findings are delivered,
the expected range is approximately **18–26 days**; P3 items add the remainder.

Sequencing rules:

- Do not start P4-01 until P4-02, P4-03 and P4-04 are merged. P4-01 is an
  acceptance test, not a substitute for the three underlying tests.
- Do not start P4-05 until the controllable receiver strategy is decided.
- Keep P4-06 after P4-01 if the admin report needs a known delivered parcel; it
  can use a simple created parcel earlier, but the cross-role test is the
  authoritative demo.
- Run the mocked Chromium suite after every frontend PR; run the full real-stack
  suite at milestones 2, 3, 4, 5 and after any readiness/worker change.
- Treat P4-12, P4-13 and P4-14 as stretch/optional after all P1/P2 findings are
  complete.

## 6. Milestones and dependencies

### Milestone 1 — Foundation and quality

Tasks:

- P4-08 documentation truth (0.5–1 day)
- P4-07 access matrix expansion (1–1.5 days)
- P4-11 route inventory/drift validation (1–1.5 days)

Exit: docs match source; access matrix covers the verified route set; contract
tests fail on drift; existing CI remains green.

### Milestone 2 — Customer workflow

Tasks:

- P4-03 customer UI journey (2–3 days)

Exit: `/new` → `/parcels` → detail/history → real receipt passes on the real
stack.

### Milestone 3 — Dispatcher and courier workflow

Tasks:

- P4-04 dispatcher assignment/failure (1–1.5 days)
- P4-02 courier completion/proof/failure (2.5–4.5 days)

Exit: dispatcher assigns through the UI; courier completes and fails through the
UI; proof is retrievable.

### Milestone 4 — Cross-role integration

Tasks:

- P4-01 full acceptance journey (2–3 days)

Exit: the primary journey passes end to end, including tracking, notification
records and merchant order state where applicable.

### Milestone 5 — Merchant and administration

Tasks:

- P4-05 webhook retries/dead-letter/rotation (2.5–3.5 days)
- P4-06 admin mutations and non-empty reports (2.5–3.5 days)

Exit: real-stack merchant/admin operation journeys pass; retry and rotation
semantics are evidenced.

Dependency graph:

```text
P4-08 ─┬─> P4-07 ─> P4-11 ─> later route changes
       │
       └─> documentation review

P4-03 ───────────────┐
P4-04 ───────────────┼─> P4-01 ─> P4-06 (reports dataset)
P4-02 ───────────────┘

P4-05 ─────────────────────────────> P4-10 (consumer recovery evidence)
P4-09 ─────────────────────────────> final validation
P4-12/P4-13/P4-14/P4-15 ───────────> optional/stretch
```

### Milestone 6 — Reliability and final validation

Tasks:

- P4-09 write-path/async k6 evidence (1.5–2 days)
- P4-10 broker/identity outage CI (1.5–2 days)
- P4-13 restore/rollback rehearsal (1–2 days, optional/stretch)
- P4-12 auth reset/Google journey (1–1.5 days, optional/stretch)
- P4-14 protected AI smoke (0.5 day, optional)
- P4-15 retention policy decision (0.5 day; no cleanup implementation)

Exit: required CI is green; recovery and performance evidence is recorded;
retention policy is decided or explicitly deferred.

## 7. Git and PR strategy

- Never commit directly to `development`.
- Start every branch from the current merged `development`.
- Use one short-lived branch per logical batch; merge it before starting the next
  batch when it touches overlapping files.
- Use merge commits (the repository's established merge method) and delete no
  history; do not force-push shared branches.
- Rebase a feature branch only before review if it has no external reviewers;
  otherwise merge `development` into it. Resolve conflicts and re-run checks.
- Avoid overlapping PRs that edit the same contracts, CI file, migration tree or
  Playwright config. The sequence above intentionally serializes those edits.
- Leave the planning PR open for review; do not merge planning docs as part of a
  feature PR.

Suggested branches and PR titles:

| Milestone | Branch                                    | PR title                                                    |
| --------- | ----------------------------------------- | ----------------------------------------------------------- |
| Planning  | `docs/phase-4-solo-plan`                  | `docs: add Phase 4 solo implementation plan`                |
| 1         | `docs/phase-4-doc-truth`                  | `docs: correct Phase 4 readiness and coverage claims`       |
| 1         | `test/phase-4-access-and-route-contracts` | `test: expand access matrix and validate route contracts`   |
| 2         | `test/phase-4-customer-journey`           | `test: cover the customer parcel journey on the real stack` |
| 3         | `test/phase-4-dispatcher-assignment`      | `test: cover dispatcher assignment through the real UI`     |
| 3         | `test/phase-4-courier-completion`         | `test: cover courier completion, failure and proof`         |
| 4         | `test/phase-4-cross-role-acceptance`      | `test: add the cross-role delivery acceptance journey`      |
| 5         | `test/phase-4-webhook-retries`            | `test: prove webhook retry and rotation semantics`          |
| 5         | `test/phase-4-admin-mutations`            | `test: cover admin mutations and non-empty reports`         |
| 6         | `perf/phase-4-write-path`                 | `perf: add parcel write-path and async baseline`            |
| 6         | `test/phase-4-outage-recovery`            | `test: verify broker and identity outage recovery`          |
| 6         | `test/phase-4-restore-rollback`           | `test: rehearse restore and migration rollback`             |
| 6         | `test/phase-4-auth-browser`               | `test: cover password reset on the real stack`              |
| 6         | `chore/phase-4-ai-smoke`                  | `chore: add protected AI live smoke`                        |
| 6         | `docs/phase-4-retention-policy`           | `docs: record retention policy decision`                    |

## 8. Testing strategy

### Per-milestone gates

| Milestone | Required validation                                                                                                                                          |
| --------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| 1         | `pnpm exec prettier --check` on docs; `node --test "tests/contracts/*.test.mjs"`; `make test-access`; `go test ./tests/access/accesskit/...`; `nginx -t`; CI |
| 2         | `pnpm exec nx lint @org/web-e2e`; `pnpm exec nx typecheck @org/web-e2e`; new real-stack customer spec; full real-stack project; mocked Chromium suite        |
| 3         | New dispatcher/courier real-stack specs; full real-stack project; affected parcel Go unit/integration tests; mocked courier/dispatch suites                  |
| 4         | Cross-role real-stack spec; full real-stack project; event-flow integration; notification-log check; affected Go tests                                       |
| 5         | Webhook real-stack retry/rotation spec; merchant webhook integration/unit tests; admin real-stack mutations; full real-stack project; reports with data      |
| 6         | k6 scripts and recorded baseline; outage recovery job; restore/rollback tests; optional auth/AI evidence; final required CI                                  |

### Real-stack execution

```sh
make up-build
make migrate
make seed
make seed-merchant
make seed-parcels

REAL_STACK=1 \
REAL_STACK_BASE_URL=http://localhost:8080 \
REAL_STACK_ADMIN_EMAIL=... REAL_STACK_ADMIN_PASSWORD=... \
REAL_STACK_DISPATCHER_EMAIL=... REAL_STACK_DISPATCHER_PASSWORD=... \
REAL_STACK_COURIER_1_EMAIL=... REAL_STACK_COURIER_1_PASSWORD=... \
REAL_STACK_CUSTOMER_EMAIL=... REAL_STACK_CUSTOMER_PASSWORD=... \
pnpm exec nx run @org/web-e2e:e2e --args="--project=real-stack"
```

The real-stack project runs one worker because the login limiter is shared and
one test stops Identity. Keep new tests serial and unique-keyed. Use the existing
`signInThroughForm`/`apiLogin` helpers and wait out `429 Retry-After` rather than
sleeping arbitrarily.

### Primary acceptance journey

The cross-role spec must perform the following through the browser:

1. Customer signs in.
2. Customer creates a parcel through `/new`.
3. Customer sees it in `/parcels`.
4. Customer opens details/history and downloads the real receipt.
5. Dispatcher assigns a courier through the actual modal.
6. Courier opens the assigned delivery.
7. Courier moves it through supported transitions.
8. Courier completes with recipient/proof or records a failure reason.
9. Customer tracking shows the final state.
10. Dispatcher state shows the final courier/status.
11. ADMIN notification logs show the expected delivery record.
12. Merchant order state is verified where the parcel originated from a merchant
    API order (API-key setup is a precondition, not a UI shortcut).

Use API calls only for read-only verification or setup data that is not the UI
interaction under test. Do not assign, complete, upload proof, or mutate admin
resources through the API in the acceptance journey.

### Failure-path coverage

At minimum:

- dispatcher assignment refusal (one documented conflict/eligibility path);
- courier invalid transition or invalid recipient/failure input;
- proof validation/limit/rate-limit state already covered by mocked tests plus
  one real successful upload;
- webhook first retry and final-failure/suppression behavior;
- admin rejected/suspended merchant state;
- broker/identity outage readiness and recovery.

## 9. Risks and mitigation

| Risk                                              | Mitigation                                                                                                                      |
| ------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------- |
| Real-stack suite is slow and login-rate-limited   | One worker, unique data, shared login helper, one acceptance spec; avoid redundant logins                                       |
| Webhook six-hour retry cannot be waited for in CI | Use a test-only accelerated schedule or prove scheduling/attempt rows plus existing worker integration; document the choice     |
| No controllable public HTTPS webhook receiver     | Decide before P4-05: protected manual receiver, test-only tunnel, or attempt-row/unit evidence; never disable SSRF protection   |
| PDF download differs in headless Chromium         | Assert the `download` event, filename and PDF magic bytes; keep the real receipt route unstubbed                                |
| Proof storage depends on local volume             | Keep one Parcel replica or shared volume in test; do not claim multi-replica proof support                                      |
| Admin tests mutate shared seed data               | Use unique users/merchants/parcels and clean up or isolate with a disposable stack                                              |
| CI cost/runtime grows                             | Keep mocked suite as the fast gate; run real-stack and recovery in dedicated jobs; do not make optional provider tests required |
| Solo context switching                            | One milestone at a time; do not open overlapping contract/CI PRs                                                                |
| Retention deletion harms idempotency/audit        | P4-15 is decision-only until an approved policy exists                                                                          |
| External Google/Gemini credentials                | Optional/protected manual jobs; deterministic tests remain the merge gate                                                       |

## 10. Scope exclusions

Phase 4 does not include:

- Kubernetes, GKE, Terraform, production TLS or cloud deployment changes;
- Prometheus/Grafana deployment, alerting or distributed tracing;
- multi-replica realtime fan-out, SSE or event replay;
- SMS provider integration;
- replacing RabbitMQ, the outbox or the retry topology;
- parcel edit/cancel APIs (not in API v1.1);
- new frontend frameworks, state managers or UI libraries;
- automatic retention cleanup before an approved policy;
- required external Google/Gemini calls in normal CI;
- broad refactors of healthy services without a failing acceptance test.

## 11. Definition of done

Phase 4 is done when:

1. All P1 and P2 findings are implemented and verified, or explicitly accepted
   with a written reason.
2. The cross-role acceptance journey passes on a clean Compose stack.
3. Customer creation/list/detail/history/receipt, dispatcher assignment and
   courier completion/failure/proof all have real-stack browser evidence.
4. Webhook retry/rotation and admin mutation journeys have real-stack evidence.
5. Access matrix and route-drift validation cover the public route set or record
   approved exceptions.
6. Operations, README and environment documentation match the implementation.
7. Broker/identity recovery and write-path performance evidence are recorded.
8. P3 items are completed or clearly deferred; retention policy is decided or
   explicitly blocked on approval.
9. Required CI is green on the final `development` merge commit.
10. No production code, contract or migration change is made outside a focused
    PR with its own tests and review.

## 12. Approval decisions before implementation

1. **Webhook retry evidence:** approve a test-only accelerated retry schedule,
   a protected public receiver/tunnel, or acceptance of attempt-row/unit evidence
   for the six-attempt schedule. This decides how P4-05 is proven.
2. **Retention policy:** approve durations and disposition for merchant order
   reservations and notification delivery logs; no cleanup implementation starts
   before this decision.
3. **Optional provider tests:** confirm whether P4-12 (Google), P4-14 (Gemini)
   and P4-13 (restore/rollback) are required Phase 4 deliverables or stretch
   evidence.
4. **Reports data:** confirm whether the reports demo must use a seeded
   historical dataset or a newly created parcel in the test run.

## 13. Planning review checklist

| Check                                       | Result                                                                                                     |
| ------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| All 15 audit findings covered               | Yes — P4-01 through P4-15 each has a finding section and tracker row                                       |
| No finding silently dropped or reclassified | Yes — original P0/P1/P2/P3 classifications are preserved                                                   |
| P1 work prioritized                         | Yes — P4-01, P4-02 and P4-03 are the first workflow deliverables and gate the acceptance milestone         |
| Dependencies accurate                       | Yes — route inventory precedes access expansion; customer/dispatcher/courier precede cross-role acceptance |
| Sequence realistic for one developer        | Yes — six sequential milestones, one active branch batch at a time                                         |
| Testing effort included                     | Yes — estimates include test authoring, debugging and CI iteration                                         |
| Shared changes grouped                      | Yes — docs, access and route-contract changes are isolated in Milestone 1                                  |
| Optional work distinguished                 | Yes — P4-12, P4-13 and P4-14 are optional/stretch; P4-15 is a decision gate                                |
| Implementation started                      | No — this is planning only; no feature branch, production change or feature PR exists                      |
| Branch and PR strategy defined              | Yes — short-lived branches, merge commits, no direct `development` commits                                 |
