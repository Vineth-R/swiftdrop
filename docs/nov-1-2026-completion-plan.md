# SwiftDrop completion plan for 1 November 2026

Planning only. Prepared 31 October 2026. No product implementation, branch creation, commit, push, migration execution, build or test run was performed for this review.

## 1. Decision and delivery target

Treat this as two related commitments:

1. **Team submission:** one combined design report PDF, repository URL, reproducible local setup, working registration/email login/refresh/logout and Google login in the browser, reviewed migrations, and the tested `milestone-1` tag.
2. **M4 submission:** pricing estimates and zones, report Sections 6.1 and 8 with Figures 8 and 9, plus actual public tracking under the explicit requirements in `M4.pdf`. Include source, tests/results, documentation, integration evidence, implementation report and PR/demo links.

The whole-team analysis is included because the request does not identify a member number. M4 has the detailed execution breakdown because the folder includes a dedicated M4 assignment. Ownership remains with the assigned members; this plan does not transfer everyone else's work to M4.

The available code is not submission-ready. Most required business flows are absent, and the latest remote changes contain apparent build blockers. A full team needs to work concurrently during implementation. There is no evidence supporting a promise that one developer can finish the full scope by the deadline.

The source clock read during analysis was **31 October, 23:44 Asia/Colombo**. The documents' 21:00 report freeze and 23:00 assembly checkpoints had already elapsed at that point. Use the recovery schedule in Section 9, not the original afternoon schedule. **1 November at noon is a team target, not a verified portal cutoff.** M1/M10 must confirm the actual deadline immediately and bring all gates forward if needed.

## 2. Evidence and source precedence

All seven PDFs under `C:/Users/Thisal K/Downloads/Swiftdrop` were text-extracted and reviewed for scope, deadlines, ownership, contracts and acceptance criteria. The review covers 141 source pages in total. Figures were assessed through their captions, extracted labels and accompanying requirements; this is not a visual-quality certification of the existing PDFs.

| ID  | Source                                                                                                                               | Pages | Planning use                                                                                                     |
| --- | ------------------------------------------------------------------------------------------------------------------------------------ | ----: | ---------------------------------------------------------------------------------------------------------------- |
| S1  | [M4 requirements](<C:/Users/Thisal K/Downloads/Swiftdrop/M4.pdf>)                                                                    |     3 | Explicit 1 November pricing and actual public tracking requirements, evidence and handover package               |
| S2  | [Nov 1 Submission Guide](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop Nov 1 Submission Guide.pdf>)                               |    10 | Team submission, member cards, evening checkpoints, final checklist                                              |
| S3  | [Milestone 2 Submission Plan](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop Milestone 2 Submission Plan.pdf>)                     |     6 | Must/Should scope for 1 November; clarifies full implementation is due 15 November                               |
| S4  | [Team Task Sheet](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop-Team Task Sheet.pdf>)                                             |    21 | Full ownership, acceptance criteria, dependency handoffs and later milestones                                    |
| S5  | [API Documentation v1.1](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop API Documentation (v1.1, consolidated).pdf>)               |    48 | Endpoint methods, paths, responses, errors, tokens, tracking projection, internal APIs and integration contracts |
| S6  | [Database Design v1.1](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop Database Design Document (v1.1, aligned with API v1.1).pdf>) |    26 | Table ownership, constraints, decimal money, seed, privacy and state rules                                       |
| S7  | [Design Report v1.1](<C:/Users/Thisal K/Downloads/Swiftdrop/SwiftDrop_Design_Report_v1.1.pdf>)                                       |    27 | Report structure, all 17 figures, design claims and corrections needed before assembly                           |

Repository evidence: root [AGENTS.md](../AGENTS.md), `origin/development` tree/history, routers/startup, repository and shared PostgreSQL code, migrations, web pages, worker handlers, event/OpenAPI contracts, Makefile, Nginx, CI, tests and existing documentation. Tracked files unchanged between the local and remote commits were inspected in the working tree; the eight changed files were examined using `git show origin/development:<path>`.

Follow the user's instruction and `AGENTS.md`: **development is always the base and PR target**. Repository implementation is the evidence for what exists. PDF requirements remain requirements; where they differ from implementation, document a gap or decision rather than silently choosing one and claiming compliance.

No standalone `skills.md`/`SKILLS.md` was found in the supplied folder or repository search. The repository uses individual `.agents/skills/*/SKILL.md` files. This review applied `nx-workspace`, the PDF reading skill and `verification-before-completion`; future implementation must load the relevant skills listed in Section 11. Do not create a competing root instructions file.

### Conflicts and decisions to record before implementation

| ID  | Conflict or ambiguity                                                                                                                                                                                                    | Planning decision / required resolution                                                                                                                                                                                                                                                                        |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| D1  | S1 requires actual public tracking by 1 November; S2/S3 make a sample tracking layout optional; S4 schedules tracking for 6–10 November                                                                                  | Keep real REST tracking in the M4 deadline scope unless the team explicitly changes S1. Deliver the shared minimum first, then real tracking. Record any agreed reduction as an incomplete requirement, never imply a sample layout satisfies S1.                                                              |
| D2  | S1 includes pricing management where required and UI if assigned; S4 assigns both but schedules admin endpoints/screens for 4–9 November                                                                                 | Plan the full admin work as a conditional deadline track. M4/M1 must settle whether it is accelerated. Until resolved, list it as outstanding under a strict reading of S1. Do not silently defer or invent an approval.                                                                                       |
| D3  | S3's title says Milestone 2, but its body calls 1 November the report/login submission and 15 November the full implementation                                                                                           | Label this plan by date. Keep the documented `milestone-1` tag unless the team changes its convention.                                                                                                                                                                                                         |
| D4  | S3/S4 use afternoon checkpoints; S2 uses 20:00 drafts and 20:30 reviews; all retain 21:00 freeze                                                                                                                         | Those checkpoints are elapsed. Use an explicit recovery freeze and owner acknowledgements.                                                                                                                                                                                                                     |
| D5  | S5 §3.1 says above-maximum weight is `VALIDATION_ERROR`; team text broadly says weights outside all bands are `WEIGHT_NOT_PRICED`                                                                                        | Decision (M4): nonpositive/malformed weight -> 422 `VALIDATION_ERROR`; any weight outside every active band, including above-maximum and gaps, -> 422 `WEIGHT_NOT_PRICED`; an unpriced route -> 422 `ROUTE_NOT_SERVICEABLE` first. This deliberately diverges from S5 §3.1 wording and is recorded for M5/M10. |
| D6  | S5/S7 use camelCase, unversioned parcel events and queues such as `notification.queue`; repository messaging uses snake_case versioned envelopes and `notification.requested.v1`/`webhook.delivery.requested.v1` workers | M1/M6/M7/M8/M9/M10 must agree a contract reconciliation before publishers/consumers are built. Existing outbox SQL also constrains unversioned names. Update contracts, migrations if required and report together. Do not rewrite the current broker topology as an incidental M4 change.                     |
| D7  | S5/S7 describe six webhook attempts: now, 1 min, 5 min, 30 min, 2 h, 6 h; shared consumer currently retries three times at a fixed delay                                                                                 | Describe implemented transport retry separately from planned webhook business delivery. No claim of six-attempt delivery until implemented and tested; later design must prevent retry multiplication.                                                                                                         |
| D8  | S7 says eight Go units including the optional AI service and implies every unit has HTTP probes; repository has seven Go projects, two of them non-HTTP workers                                                          | Show current seven-unit deployment and label AI as future bonus. Describe HTTP probes and worker lifecycle accurately. No new worker HTTP APIs or AI/Kubernetes work for this deadline.                                                                                                                        |
| D9  | S5 calls public estimates rate-limited but does not define a useful anonymous-user limit in its general per-user default                                                                                                 | Agree the anonymous estimate policy, trusted proxy/IP handling and 429 headers before implementing it. Tracking and login limits are explicitly specified.                                                                                                                                                     |

## 3. Exact development baseline

| Item                     | Observed result                                                                                                                                                                                                                           |
| ------------------------ | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Local checked-out branch | `development`                                                                                                                                                                                                                             |
| Local HEAD               | `0217b4a4fb1fda5808012e750c5eef88288e6e97`                                                                                                                                                                                                |
| Remote development       | `49fa6a1e1f8ae5ca551e013dee9d3093d8e957e8`, verified with `git ls-remote origin refs/heads/development`                                                                                                                                   |
| Difference               | Local is 0 commits ahead, 2 behind; remote adds merchant HTTP/API-key/database integration across eight files                                                                                                                             |
| Existing user work       | Untracked `docs/m4-nov-1-submission-plan.md`; preserved unchanged and not treated as implementation evidence                                                                                                                              |
| This review's output     | This new planning document only; PDF extraction dependencies/intermediates are under ignored `tmp/`                                                                                                                                       |
| Dynamic verification     | No builds, tests, migration runs, Compose startup or CI-status claims. Nx MCP tools were not exposed; local `node_modules` and usable pnpm were unavailable during discovery. No package install or lockfile changes for the application. |

Refresh the remote SHA before development begins. Never build the feature from the older local HEAD just because the checkout is named `development`.

### Current functionality versus required outcome

`Present` below means source exists, not that it passed this audit's runtime tests.

| Area         | Current remote-development evidence                                                                                                                             | Gap for 1 November                                                                                                                                                                                  |
| ------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Platform     | Shared pgx pool/config, readiness, request IDs, error helpers, logging, Compose, gateway and CI are present                                                     | Repair integration blockers, prove fresh migration/startup and actual required routes; get a green CI run on the selected commit                                                                    |
| Identity     | Router registers health aliases; startup opens DB and adds `/health` and `/ready`; baseline roles/users/token/OAuth/reset/audit tables and five-role seed exist | Registration, email login, refresh rotation, logout, JWT/auth middleware, lockout and meaningful auth tests are absent                                                                              |
| Google login | `oauth_states` table and environment placeholders exist                                                                                                         | Authorization/callback endpoints, provider verification, state consumption and Google button are absent                                                                                             |
| Web          | Nx starter, example API/title test, `/merchant` sample portal                                                                                                   | No real login/register flow, shared authenticated API behavior, role redirects or public track page. Merchant controls/data are a mockup                                                            |
| Pricing      | DB connection startup; five baseline tables, overlap exclusion, active route uniqueness and seed already exist in `000002_tables`                               | No estimate/zones/internal quote business routes, pricing repositories/calculators or pricing behavior tests                                                                                        |
| Parcel       | DB connection startup; five baseline tables including history, assignments, proofs and outbox                                                                   | No business repositories, public tracking route or state-machine package. Full creation/dispatch APIs are later team work                                                                           |
| Merchant     | Partial repository/state/API-key work, remote profile/key/admin routes, report Markdown and three editable merchant figures                                     | Migration collisions; integration/type issues; authenticated identity source is not wired; required global report assembly is absent. Registration/order flows are not established by existing docs |
| Notification | Baseline notification/processed-event tables; worker validates framing/envelope/type and logs                                                                   | No actual email sending, event producers or business deduplication. SMTP configuration is not proof of email delivery                                                                               |
| Webhook      | Baseline webhook/attempt tables; envelope-validating worker                                                                                                     | No settings/delivery/signing workflow. Later implementation except database/report requirements                                                                                                     |
| Realtime     | Health-only router and gateway WebSocket settings                                                                                                               | No `/ws` handler or live subscription hook; later under S4                                                                                                                                          |
| Reports      | API/DB references, merchant sections/diagrams and M9 review note                                                                                                | No committed `docs/report` assembly or final combined report. Supplied PDF is a source, not proof that this milestone's reviews/corrections are done                                                |
| Tests        | Infrastructure/config/HTTP helper/messaging tests and example E2E                                                                                               | Required auth, pricing, tracking and database-constraint evidence is missing; nothing was executed in this planning review                                                                          |

## 4. Blockers to clear before feature work

| ID  | Evidence                                                                                                                                                                                   | Required action and acceptance gate                                                                                                                                                                                                                                                                                                                                                                                                                            | Owner                  |
| --- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------------------- |
| B1  | Remote `packages/go-common/postgres/pool.go`, `Pool.Query`: `return p.pool.Query(ctx, sql, args...), nil`                                                                                  | Inspect/fix the apparent multi-value return error, maintain the declared `(pgx.Rows, error)` contract and verify go-common plus all dependent services. Static finding; no compiler output was produced in this review.                                                                                                                                                                                                                                        | M1                     |
| B2  | Remote `merchant/internal/http/merchant.go`: `c.Set(identityKey{}, ...)` and `c.Get(identityKey{})`                                                                                        | Check compatibility with the pinned Gin API and fix any context-key type error. Then wire identity from verified authentication. Routes currently have no production authentication middleware populating `Identity`. Inspect API-key identity too: it omits `UserID`, while `identityFrom` requires one. Do not solve this with caller-controlled identity headers.                                                                                           | M8 + M2                |
| B3  | `migrations/merchant` contains both `000002_tables` and `000002_merchants`; later 000003–000005 also recreate baseline tables without `IF NOT EXISTS`                                      | Inventory applied versions/checksums and dirty state on each relevant database. Establish canonical migration history and a fresh-install/upgrade repair plan with M1/M8/M9. A new 000006 alone cannot bypass duplicate version 000002. Applied migrations remain immutable; if correction of the migration source history is necessary, obtain an explicit architectural exception and preserve an upgrade path. Never reset a shared DB to hide the problem. | M1 + M8/M9             |
| B4  | Nginx exact `/api/v1/zones` redirects to `/api/v1/zones/`; proposed Gin route is `/zones`                                                                                                  | Test the exact documented public URL. Align slash handling explicitly; avoid a gateway-to-Gin redirect cycle. Inspect other exact collection paths while fixing the same issue. Pass means exact documented routes reach business handlers without looping.                                                                                                                                                                                                    | M1 + M4                |
| B5  | `AGENTS.md` still describes no business UI, `migrations/README.md` says no DB connection/business tables/seed, route script only checks probe 404s, report overstates implemented patterns | Update stale sections as part of the relevant delivery PRs. Retain route plumbing checks but add actual business-response smoke checks. Evidence must show login/pricing work, not merely an upstream 404.                                                                                                                                                                                                                                                     | M1 + owners            |
| B6  | Report source says 26 tables but gives incorrect explanatory arithmetic; baseline includes 26 distinct schema-qualified tables and extra merchant files duplicate several                  | After migration repair verify 7 identity + 5 pricing + 5 parcel + 7 merchant + 2 notification = 26 business tables, excluding migration bookkeeping. Correct prose to 23 non-audit tables + 3 audit tables. Update ER diagram against canonical schema, not duplicate CREATE statements.                                                                                                                                                                       | M5 + all schema owners |

B1/B3 are immediate platform gates. Keep drafting reports and pure pricing/state tests while their owners resolve them. Fakes support development but cannot be presented as final integration proof.

## 5. Whole-team 1 November work and acceptance

Source: S2 member guides pp. 4–9, S3 pp. 2–5, S4 tasks explicitly marked Nov 1. Existing baseline tables replace duplicate migration-writing work with inspection, reconciliation and executed constraint tests.

| Owner | Required work from the actual baseline                                                                                                                                                                                                                                  | Done when                                                                                                                                                                                                                                                |
| ----- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| M1    | Resolve B1/B3/B4 with owners; prove DB helper/readiness and all five schemas; clean README/Makefile/environment instructions; preserve common errors/logging; verify CI and clean clone; Sections 4.1/4.4 + Figure 7                                                    | Fresh stack migrates with one documented command; DB outage makes readiness 503; gateway blocks `/internal`; actual auth/browser paths work; selected commit has observed green checks, accessible repo and tested tag                                   |
| M2    | Reuse identity tables/roles; implement register/login/refresh/logout, safe password hashing, lockout, auth middleware and cookies; browser login/register/logout, shared API/refresh/error/role behavior and one-page UI guide; Sections 2.6/7 + Figure 5 + threat list | Valid registration/login persists across restart; unknown-email/wrong-password share 401; disabled account 403; repeated failures 423; refresh reuse 401; logout clears/revokes; required 10/min/IP limit and failure tests; pages usable at 360/1280 px |
| M3    | Reuse OAuth table; configure real Google client/test users privately; implement login/callback on M2's session path; state expiry/one-time use, ID-token verification and safe redirect; Google button/error; Sections 6.4/6.5 + Figures 14–17                          | New Google user becomes CUSTOMER; invalid/reused/expired state rejected; external and protocol-relative redirects refused; browser callback works through gateway with same cookies; real provider evidence, not only a fake                             |
| M4    | Pricing and tracking tasks in Section 6; Sections 6.1/8 + Figures 8/9; review M5's schema/report                                                                                                                                                                        | 550.00 example and negative cases verified; exact gateway paths work; strict-S1 tracking uses real records and public projection; report claims cite actual code or are labelled planned                                                                 |
| M5    | Reuse/verify parcels/history baseline, exact owner constraint, tracking uniqueness, idempotency, delivered/failed fields and report indexes; add actual repository seam; Section 3 + Figure 6 and DB title/count correction                                             | Fresh/upgrade migration checks; two owners refused; repeated customer idempotency refused; all 26 tables confirmed by owners and represented; DB readiness works                                                                                         |
| M6    | Reuse assignment/outbox tables; implement shared parcel state machine with table-driven tests; Sections 2.4/2.5/6.6 + Figures 3/4                                                                                                                                       | One current assignment enforced; all six-by-six status pairs covered as allowed/refused, including terminal states; only assigned couriers change status in report; dispatcher/admin assignment role accurately stated                                   |
| M7    | Verify proofs and notification tables already in baseline, delivered/failed checks, notification uniqueness; Sections 5/6.3 + Figures 12/13, topology and event catalogue                                                                                               | Relevant constraints tested; event differences D6/D7 explicitly resolved or recorded; report distinguishes current transport behavior from planned delivery behavior                                                                                     |
| M8    | Resolve merchant migration and wiring issues with M1; verify one merchant/owner, key lookup uniqueness, order idempotency/hash/snapshot/timeline/audit structure; turn existing Markdown/figures into reviewed report Sections 2.3/6.2 + Figures 2/10/11                | Canonical migration chain works; UC-M1–M8 name correct endpoints; figures and prose agree; absent business flows are labelled planned rather than described as working                                                                                   |
| M9    | Verify webhook tables/attempt uniqueness in baseline; Sections 1/2.1/2.2/9 + Figure 1; login/wizard/parcel-detail/mobile-tracking wireframes and complete requirements tracker                                                                                          | All five roles, UC-C1–C9 endpoint mapping, real section references; each compulsory requirement has owner, complete/partial/missing/unverified state, code/test/demo evidence                                                                            |
| M10   | Sections 4.2/4.3, check report indexes with M5, assemble corrected source sections/figures/wireframes into one PDF, final checklist and submission                                                                                                                      | All 17 numbered figures plus topology present, captions/references correct, PDF opens/prints legibly, repo URL accessible, submission posted with second-person confirmation                                                                             |

For M2: access TTL 15 min, refresh TTL 7 days, hashed refresh storage and atomic rotation; `sd_access`/`sd_refresh` are HttpOnly/SameSite cookies with local HTTP-compatible settings. Support documented `X-Token-Mode: body` behavior without exposing tokens in normal browser responses/logs. Set lockout threshold/duration explicitly with the team if not already agreed; do not invent a PDF requirement. Reject invalid JWT signature/algorithm/expiry and client-supplied roles. For Google, match by verified `google_sub` then email under an explicit safe account-linking policy; existing privileged roles must not be escalated by a client or provider payload.

The full UI kit list in S4 M2.2 is Button, Input, Table, Modal, Toast, StatusBadge and Timeline. Keep it within existing CSS modules/global CSS; build purposeful components without a new library. Optional dashboards do not outrank working authentication.

### Report ownership and review inventory

| Owner | Required sections                  | Figures                                                       |
| ----- | ---------------------------------- | ------------------------------------------------------------- |
| M1    | 4.1, 4.4                           | 7 architecture                                                |
| M2    | 2.6, 7, threat list                | 5 account use cases                                           |
| M3    | 6.4, 6.5                           | 14/15 Google sequence/activity; 16/17 reset sequence/activity |
| M4    | 6.1, 8                             | 8/9 customer creation sequence/activity                       |
| M5    | 3, corrected DB document           | 6 ER diagram                                                  |
| M6    | 2.4, 2.5, 6.6                      | 3 courier; 4 dispatcher/admin use cases                       |
| M7    | 5, 6.3, event catalogue            | 12/13 status-update sequence/activity; additional topology    |
| M8    | 2.3, 6.2                           | 2 merchant use cases; 10/11 merchant sequence/activity        |
| M9    | 1, 2.1, 2.2, 9, wireframe appendix | 1 customer use cases; four wireframes                         |
| M10   | 4.2, 4.3, assembly                 | Complete figure numbering/captions and cross-references       |

Report ring: M1 reviews M2, M2 reviews M3, M3 reviews M4, M4 reviews M5, M5 reviews M6, M6 reviews M7, M7 reviews M8, M8 reviews M9, M9 reviews M10, M10 reviews M1. PR buddies: M1/M10, M2/M3, M4/M5, M6/M7, M8/M9. Reviews are actions for future execution; none were sent by this planning review.

## 6. Detailed M4 implementation work packages

Efforts below are rough engineering effort, excluding long external waits and major baseline repair. They are estimates, not delivery promises. Each package includes code, tests, contract/docs changes and review corrections.

| ID  | Package                                                                                                             | Dependencies                                             | Estimate          | Exit gate                                                                                             |
| --- | ------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------- | ----------------- | ----------------------------------------------------------------------------------------------------- |
| P0  | Agree D1/D2/D5/D9, freeze quote/public contracts, review current seed and constraints with M5                       | M1/M5 availability                                       | 0.5–1 h           | Scope and error decisions written; no duplicate migration planned                                     |
| P1  | Service-owned pricing repository and exact-money calculation functions                                              | B1 fixed; existing DB helper                             | 2–3 h             | Repository cancellation/error handling and pure pricing/boundary tests pass                           |
| P2  | Public estimate/zones and authenticated internal quote/zone lookup; DB injection, gateway and OpenAPI alignment     | P1; B4 fixed                                             | 2–3 h             | Same-input quote parity, required responses/errors, active-zone filtering and exact public path tests |
| P3  | Real anonymous public tracking repository/handler in Parcel, privacy and IP rate limiting                           | M5 parcel ownership/repository agreement; working schema | 2–4 h             | Actual stored status/history, correct ordering, 404/429, no private fields; gateway evidence          |
| P4  | Real `/track` UI with lookup/timeline and states; estimate UI only as agreed with M5                                | P2/P3; M2 UI conventions                                 | 1.5–3 h           | 360 px and desktop flow, accessible errors/timeline, genuine API integration and browser evidence     |
| P5  | Sections 6.1/8, editable Figures 8/9, source references, review M5                                                  | Can start immediately                                    | 2–3 h             | M3 review fixed; M10 receives reviewed content; patterns honestly marked                              |
| P6  | Fresh/upgrade DB checks, integration tests, affected Nx checks, screenshots/demo, results and implementation report | P2/P3/P4; stable platform                                | 2–3 h             | Evidence ties to exact SHA; tested PR and handoff package                                             |
| P7  | Conditional: all admin pricing endpoints + four screens + audit/authorization/concurrency tests                     | D2 resolved, M2 auth, M3 admin layout, P1/P2             | Additional 8–14 h | All API §§3.3–3.12 implemented and protected; changes affect new estimates only                       |

Core M4 including real tracking/UI is approximately **12–20 engineering hours** before contingency. Full accelerated admin scope makes that approximately **20–34 hours**. This exceeds the overnight window for one person. Split independent report/frontend/test ownership with team agreement, or explicitly renegotiate scope/deadline; do not remove tests or conceal incomplete work. Cross-service contributors must coordinate Parcel edits with M5/M6/M7.

### P1/P2: pricing behavior and database reuse

- Reuse `migrations/pricing/000002_tables.up.sql`: `zones`, `pricing_rules`, `weight_bands`, `size_surcharges`, `audit_logs`, seed, active-pair uniqueness and GiST overlap exclusion are already authored. Verify execution; do not recreate them.
- Verify CMB -> KDY base 450.00, `(1,5]` band 100.00 and SMALL surcharge 0.00 give **550.00 LKR** for 2 kg. Resolve zone IDs by seed codes instead of hardcoding example UUIDs.
- Implement separate base-fee, weight and size calculation functions composed by a pricing operation. Use exact decimal/fixed-point representation with deliberate weight precision and JSON number encoding; no binary-floating rounding for money. No speculative dependency/interface framework.
- Select active zones, active route rule and exactly one active band with `min < weight <= max`. Reject malformed IDs, unknown/inactive zones, invalid weight/precision and invalid size according to the agreed contract.
- Register Gin paths `/pricing/estimate`, `/zones`, `/internal/pricing/quote`, `/internal/zones/:zoneId`. Public gateway adds `/api/v1`; never duplicate that prefix in Gin. Protect internal paths with `X-Internal-Token`, retain edge 404 blocking.
- Public/internal quote use the same calculation path. Return zone IDs, weight/size, LKR currency, baseFee/weightCharge/sizeSurcharge/total and `meta.requestId`/timestamp as in S5. Zones expose `id`, `code`, `name`, `districts` only for active entries.
- Errors: malformed JSON 400; missing/invalid semantic data 422 `VALIDATION_ERROR`; 404 `ZONE_NOT_FOUND`; 422 `ROUTE_NOT_SERVICEABLE`; 422 `WEIGHT_NOT_PRICED`; 500 safe internal failure. Confirm inactive-zone behavior and above-maximum policy in P0. Keep SQL, identifiers containing secrets and database details out of client errors.
- Share quote shapes, timeout/503 mapping, token setup and Compose service addressing with M5. M5 remains responsible for parcel creation, expectedFee comparison, stored fee and idempotency.
- Add a new pricing migration only for a proven missing requirement, from the next unused version at least 000003. Do not edit an applied 000002. Current seed services every seeded pair and covers the whole positive range to 25 kg: use transaction-scoped fixtures for unpriced-route/gap tests.

### P3/P4: tracking behavior and boundaries

- Implement within `apps/services/parcel/internal/`, because Parcel owns `parcel.parcels` and `parcel.parcel_status_history`; M4 contributes by agreement. Pricing must never query the Parcel schema.
- Query explicit public columns, never `SELECT *` followed by removal. Return `trackingNumber`, current `status`, `pickupCity`, `dropoffCity`, `lastUpdatedAt`, and timeline entries containing only `status`/`at`. Map persisted history `new_status` to public `status`; order deterministically by occurrence time with a tie-breaker.
- Permit anonymous `GET /api/v1/tracking/{trackingNumber}`. Unknown number -> 404 `TRACKING_NOT_FOUND`. Enforce 30 requests/min/IP with 429, `Retry-After` and the specified rate-limit metadata; configure trusted proxy handling so forwarded headers cannot bypass the limit.
- Support display/query of CREATED, PICKED_UP, IN_TRANSIT, OUT_FOR_DELIVERY, DELIVERED and FAILED. No new transition engine in M4: M6 owns rules and M7 owns courier changes.
- Reuse the existing unique tracking-number index. Verify its suitability with query evidence before adding an index. New schema work belongs to the Parcel owner and must use the next unused migration number.
- Prove results from actual DB records and ordered history. Owner-approved disposable DB fixtures can test statuses before lifecycle-write APIs exist; label this limitation. If no live status-write path exists, do not call a seeded lookup an end-to-end courier lifecycle demonstration.
- Exclude customer/recipient names, street addresses, phones, internal notes, failure details, proof photos and private identifiers from public JSON/UI/logs. The coarse cities and status/timestamps are the allowlist.
- `/track` needs input validation, keyboard operation, loading, initial/empty, success, not-found, retryable network/server-error and rate-limited states. Build an accessible timeline at 360 px; avoid leaking private data in screenshots.
- M10's live hook is later in the team schedule. Working REST tracking satisfies the real lookup portion of S1; it does not satisfy later live-update acceptance. Track realtime as a separate dependency and incomplete later item.
- Public estimate is a backend must. The full customer delivery wizard belongs to M5 and is later under the team schedule; agree any Nov 1 pricing UI rather than duplicating the wizard. Under strict S1's assigned-UI condition, identify the exact UI being delivered.

### P7: full conditional admin scope

If D2 accelerates pricing management, deliver all of the following rather than an unprotected partial form:

| Resource   | API methods/paths relative to `/api/v1`                                         | Screen and behavior                                                     |
| ---------- | ------------------------------------------------------------------------------- | ----------------------------------------------------------------------- |
| Zones      | POST `/admin/zones`; PATCH `/admin/zones/{zoneId}`                              | Zone editor; unique code, name/district/active-state validation         |
| Rules      | GET/POST `/admin/pricing/rules`; PATCH `/admin/pricing/rules/{ruleId}`          | Rules list/editor with zone filters and pagination; one active pair     |
| Bands      | GET/POST `/admin/pricing/bands`; PATCH `/admin/pricing/bands/{bandId}`          | Band list/editor; `(min,max]`, overlap and concurrent-conflict handling |
| Surcharges | GET `/admin/pricing/surcharges`; PUT `/admin/pricing/surcharges/{sizeCategory}` | Four-category surcharge editor; nonnegative exact amounts               |

Require verified ADMIN access, audit rows in the same local transaction, 409 `ZONE_CODE_EXISTS`/`RULE_EXISTS`/`BAND_OVERLAP`, standard validation/errors and ownership of pricing data. Test anonymous and wrong-role refusal, concurrent conflicts and the browser flow. Existing stored parcel prices must remain unchanged. If authentication is unavailable, this package is blocked, not permission to expose management publicly.

## 7. Report correction plan

1. Put editable section sources under `docs/report`, editable figures under `docs/diagrams`, and exports where M10's assembly process expects. Existing merchant draw.io sources can be reused after content review. No report artifacts are generated by this planning task.
2. Keep all 17 figures, the additional topology and four wireframes. Cut decorative polish before required content. Each figure needs a caption, source, section reference and legible export.
3. Correct the ER arithmetic and database title to v1.1. Use five schema groups and dashed logical cross-service references. Do not add cross-service foreign keys.
4. Correct deployment descriptions: current seven Go units, workers without public HTTP ports, actual developer dashboard exposure, DB-backed HTTP readiness, no AI/Kubernetes implementation. S7 references an ORM; actual repositories use pgx/parameterized SQL, so correct that wording.
5. Figures 8/9 and Section 6.1 must show zones -> estimate -> confirmation with expectedFee/Idempotency-Key -> internal quote -> price check -> one transaction for parcel/history/outbox -> HTTP response; relay publication occurs after commit independently. Include duplicate replay, 422 pricing failures and 409 `PRICE_CHANGED` with `currentFee`. Existing Figure 8 puts publish before response; remove any implication that broker success gates HTTP completion.
6. Use SDK customer and SDN merchant tracking numbers, all five roles and the six canonical statuses. Only assigned couriers change parcel status; dispatcher/admin assignment privileges are separate.
7. Section 8 needs an evidence row for layering, repository, strategy, state, pub/sub, adapter, outbox, gateway, snapshot model, DI, SRP, open/closed, interface segregation/dependency inversion, DRY, idempotency and fail-fast. Add implemented/partial/planned, exact source file, rationale and test/demo reference. A table or skeleton is not proof a behavioral pattern is implemented.
8. Safe current examples include `infra/nginx/default.conf` for gateway, `packages/go-common/httpx/router.go` and `requestid` for shared middleware, config loaders for fail-fast, messaging consumer/confirmed routing for transport behavior, and merchant constructors/state helpers for their narrow implemented responsibilities. Do not label absent pricing strategy, parcel state engine, outbox relay, SMTP delivery, authenticated middleware or snapshot sync as implemented until the submitted code provides them.
9. Section 9 tracks every compulsory brief requirement, including later ones, with honest current status. References must point to the final assembled section numbers and actual code/tests. Password-reset diagrams are required report content now even though the endpoint implementation is later under the team schedule.
10. M3 reviews M4's sections; M4 reviews M5's section/ER and confirms the five Pricing tables. M10 exports and visually checks the final PDF; a second member checks it and submission receipt.

## 8. Verification and evidence plan for future execution

Do not execute these steps as part of this planning task. First fix baseline issues, use the latest approved development snapshot and install the pinned project tools normally. No application tests were run here.

### Commands and environments

Inspect resolved Nx targets first, using the pinned pnpm package manager:

```text
pnpm exec nx show projects --json
pnpm exec nx show project go-common --json
pnpm exec nx show project pricing --json
pnpm exec nx show project parcel --json
pnpm exec nx show project identity --json
pnpm exec nx show project merchant --json
pnpm exec nx show project @org/web --json
pnpm exec nx show project @org/web-e2e --json
```

Then use only confirmed configured targets. Expected examples from AGENTS.md:

```text
pnpm exec nx run-many -t lint test build --projects=go-common,pricing,parcel,identity,merchant
pnpm exec nx run @org/web:lint
pnpm exec nx run @org/web:build
pnpm exec nx run @org/web-e2e:lint
pnpm exec nx run @org/web-e2e:typecheck
pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"
```

Do not assume a web `test`/`typecheck` target or E2E `build` target. Next build performs web type checking. Use existing Nx orchestration; check `--help`/Nx docs before unfamiliar flags. CI's authoritative Go checks additionally include formatting, vet, race tests with coverage and build from the root module. Concurrency-sensitive auth rotation, limiting and state work must receive race coverage; add appropriate configured verification without replacing Nx.

Integration uses an explicitly disposable PostgreSQL/Compose environment and `//go:build integration`; ordinary Go tests must not require live services. Existing broker-backed tests run with the broker configured and must not silently skip. Use a Linux/WSL or documented make-capable environment for the shell-based Makefile; document Windows prerequisites. Confirm the database target before executing destructive rollback/reset; no automatic startup migrations or shared-volume deletion.

M1 runs Compose config validation, fresh migration application in order identity/pricing/parcel/merchant/notification, upgrade-path verification from the known existing migration state, service readiness, real gateway smoke checks and a clean-clone rehearsal. Tests must cover failure paths, not merely startup. A fresh-install demonstration cannot replace the existing-database requirement in S1.

### Minimum behavioral evidence

| Area                 | Required evidence                                                                                                                                                                                                                 |
| -------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Pricing unit/handler | Exact 550.00; all four sizes; zero/negative/invalid precision; exact 1/5/10/25 boundaries; band gap; no route; missing/inactive zone/rule; repository failure; envelope and quote parity                                          |
| Pricing database     | Fresh/upgraded schema; nonnegative amounts; active pair uniqueness; adjacent bands accepted; overlap and concurrent overlap rejected; seed values confirmed                                                                       |
| Tracking             | Anonymous lookup for actual records across all statuses; stable chronological history; unknown number 404; public allowlist in success/errors/logs; valid/invalid number; per-IP limit and forwarded-header handling; 429 headers |
| Frontend             | Real estimate/tracking requests through gateway; track input, loading, success, not-found/server-error/429 and keyboard/mobile flow; no sample data presented as backend evidence                                                 |
| Identity             | Register/login/logout; normal cookie mode versus body token mode; wrong password/unknown email parity; lockout/disabled account; refresh replay and concurrent rotation; token expiry/invalid token; persistence and rate limits  |
| OAuth                | Real browser sign-in; verified provider claims; new CUSTOMER; existing account handling; state expiry/reuse/wrong state; safe redirects; friendly provider failure                                                                |
| Integration          | Exact public paths, internal token refusal/acceptance inside network, `/internal` edge 404, DB down/readiness 503, restart persistence, fresh clone from README and real CI result                                                |
| Documentation        | 17 figures + topology + wireframes; correct code references/status; source-to-requirement checklist; no undocumented contract differences                                                                                         |

For every result record command, date/time, base/submitted SHA, tool/environment version, fixture identity without personal data, pass/fail/skip and output/evidence path. Use `docs/evidence/nov-1/` during implementation for concise reproducible evidence, excluding secrets and large generated outputs. Failed, blocked and unexecuted checks stay visible. CI configuration alone is not a green run.

The documented `milestone-1` tag does **not** match the current workflow's `v*` release trigger. M1 must deliberately run an equivalent reviewed release smoke path or adjust the workflow in a reviewed change, then verify it; do not claim pushing `milestone-1` automatically runs the release job. Branch protection/required review settings cannot be established from repository files and must be verified in GitHub.

## 9. Recovery schedule to 1 November

All times Asia/Colombo. This is a proposed triage schedule after elapsed checkpoints, assuming the real portal cutoff is no earlier than noon. It is not a claim that the estimated work fits. Recalculate immediately against the actual deadline and the team's available people.

| Time / gate                         | Parallel owner work                                                                                                                                                                                   | Outcome / stop condition                                                                                                                                      |
| ----------------------------------- | ----------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| Immediately, first 20–30 min        | M1/M10 confirm deadline; M4/M1 settle D1/D2; M1/M8 handle B1–B3; M2/M3 agree session seam; M10 collects all report drafts                                                                             | Written scope, current remote SHA, owners and blockers. If build/migration repair is unresolved, name it and escalate rather than start duplicate foundations |
| By 01:00 Nov 1                      | Platform repair and migration verification; M2 auth core; M3 OAuth provider setup against shared session seam; M4 pricing pure logic/repository; M5/M6 database/state tests; all owners report drafts | **Platform gate:** usable common DB/code baseline; contracts frozen. Reports and diagrams continue independently                                              |
| By 03:00                            | M2/M3 integrate login + screens; M4 estimate/zones/internal quote and gateway; M5 supports tracking repository; report ring reviews in parallel                                                       | **Feature checkpoint:** demonstrate auth and 550.00 through gateway. Any missing flow becomes a named critical blocker; do not report ready                   |
| By 05:00                            | Auth/OAuth failures and refresh tests; real tracking API/UI if S1 applies; owner DB tests; reviewed report fixes                                                                                      | **Scope checkpoint:** real status and evidence for each must. Decide delivery escalation if still absent; optional work remains paused                        |
| 05:00–07:00                         | Freeze only working reviewed changes; rerun relevant checks; M1/M10 integrate evidence and report                                                                                                     | **Code/report freeze:** recorded commit, review status, unresolved list. Late fixes must repeat affected checks                                               |
| 07:00–09:00                         | M1 clean-clone stack/migration/login rehearsal; M4/M5 real pricing/tracking smoke; M10 PDF visual/caption/reference QA                                                                                | **Submission candidate:** observed passing required checks and tested PDF. If failing, it remains a candidate, not a completed milestone                      |
| 09:00–10:00                         | Fix only blocking defects, verify again; final PDF/repo access check by second member                                                                                                                 | **Go/no-go:** every required evidence row attached, exact SHA selected, tag only after checks. Record any agreed incomplete items explicitly                  |
| By 11:00 proposed internal deadline | M10/M1 submit PDF + repository URL; second member confirms; retain receipt                                                                                                                            | One hour buffer before the documents' noon team target; actual portal time overrides this                                                                     |

The original documents put login ahead of later features and forbid cutting it, migrations or report sections. Stop optional portal layouts, styling, later business features, AI and Kubernetes first. A scope change must be explicit. If the required auth or strict-S1 functionality cannot be delivered, communicate the incomplete status to the team before submission rather than substituting fake evidence.

Dependency paths:

```text
Baseline build/migration repair -> working DB/gateway
  -> M2 sessions + login screens -> M3 Google callback -> clean-clone auth proof
  -> M4 pricing -> estimate/quote proof -> M5 creation integration later
  -> M5 Parcel data seam -> M4 real public lookup -> track UI/evidence

All report owners -> review ring -> fixes -> M10 assembly/QA
Both submission paths + observed CI -> tested SHA/tag -> submission/receipt
```

## 10. Branch and integration plan

**Recommended M4 implementation branch: `feature/m4-pricing-public-tracking`.** Base: the latest reviewed `origin/development`, initially `49fa6a1e1f8ae5ca551e013dee9d3093d8e957e8` at this audit. PR target: `development`.

For the planning document alone, use **`docs/nov-1-2026-completion-plan`** when a branch is requested. This review has not created either branch.

Use small independent branches for other owners, all based on development:

| Scope                                         | Suggested branch                  |
| --------------------------------------------- | --------------------------------- |
| Shared compile/migration/gateway repair       | `fix/nov-1-platform-integration`  |
| M2 identity/email session flow                | `feature/m2-email-authentication` |
| M3 Google flow                                | `feature/m3-google-login`         |
| M5/M6 missing Parcel repositories/state rules | `feature/m5-m6-parcel-foundation` |
| Combined report corrections/assembly          | `docs/nov-1-design-report`        |
| Conditional full admin pricing                | `feature/m4-admin-pricing`        |

Future sequence: preserve the untracked existing M4 plan; refresh remote refs; check/record working-tree changes; update development without discarding work; branch from that agreed baseline. If repair PRs land first, update feature branches using the team's normal non-destructive workflow and rerun affected verification. Do not base work on main or another member's feature branch, and do not rewrite shared history. Coordinate file ownership for identity startup/router, Parcel routes and report assembly to avoid competing edits.

Each PR contains problem/result, exact delivered scope, changed contracts/migrations, dependencies, actual validation and remaining limits. Review buddy checks it; CI and Compose acceptance run; merge to development through the team's process. Feature branch existence or an open PR does not satisfy S4's merged/integrated definition of done. Commit, push, PR creation, merge and tag operations remain future work, not actions taken by this plan.

## 11. Development instructions and skills to apply later

Read `AGENTS.md` and applicable nested instructions again at implementation start. Load relevant individual SKILL.md files before their work:

| Work                                           | Required/relevant skills                                                                                               |
| ---------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------- |
| Workspace/targets                              | `nx-workspace`; `nx-run-tasks` before task execution; `nx-generate` first only if real scaffolding is required         |
| Go/Gin pricing/auth/tracking                   | `golang-code-style`, `golang-error-handling`, `golang-testing`, `golang-database`, `golang-security`, `golang-gin-api` |
| Any SQL/migration change or DB troubleshooting | `supabase-postgres-best-practices`, plus `migrations/README.md`; no Supabase hosting/RLS assumption                    |
| Next/React/UI                                  | `vercel-react-best-practices`, `accessibility`; existing CSS, strict TypeScript and Server Components by default       |
| E2E                                            | `playwright-best-practices`; existing Nx Playwright setup, no new test framework                                       |
| Changed infrastructure                         | `docker-compose-patterns`, `docker-build-strategies` for Dockerfile work, `reverse-proxy` for gateway work             |
| Messaging reconciliation                       | `rabbitmq-development`; current ownership/topology rules in AGENTS.md govern                                           |
| Actual email work                              | `email-best-practices`; SMTP configuration alone remains distinct from a sender                                        |
| Library/API usage                              | `context7-mcp` where applicable; verify current official APIs rather than guessing                                     |
| Before any completion/PR claim                 | `verification-before-completion`; fresh commands/output and final diff review                                          |

Keep the single root Go module, service-owned `internal` packages and schemas, shared infrastructure-only go-common, HTTP/REST and RabbitMQ. No gRPC/Kafka/new frontend libraries or speculative abstraction layers. Keep local HTTP. Do not change dependency versions/lockfiles unless the task requires a justified dependency. Never edit installed skills as part of the feature.

## 12. Handover package and final completion checklist

- [ ] All agreed must items implemented, reviewed, integrated into development and tied to the selected commit.
- [ ] Scope D1/D2 acknowledged; strict-S1 tracking/admin/UI outcomes stated explicitly, including anything incomplete.
- [ ] Canonical migrations apply fresh and upgrade the known existing database state; constraints/seed verified; no duplicate versions/tables.
- [ ] Registration, email login, Google login, refresh and logout work through the browser/gateway and survive required restart checks.
- [ ] M4 estimate/zones/quote, exact 550.00 and required negative cases proven; actual tracking/history/privacy/unknown-number/429 proven when S1 applies.
- [ ] OpenAPI, response/error names, pricing rules, public tracking docs, migration instructions, environment example and testing instructions reflect the delivered code.
- [ ] Automated results plus gateway/Compose/browser evidence exist; failed/skipped/unrun checks are disclosed, without credentials or personal data.
- [ ] Short M4 implementation report includes completed features, design choices, service-owner dependencies, limitations and outstanding work.
- [ ] Report sources and editable figures reviewed; final PDF has Sections 1–9, all 17 figures, topology, wireframes and correct references; every pattern claim has evidence or planned status.
- [ ] PR/branch URLs, merge SHA, CI link and screenshots/short pricing/tracking demo are included in the handoff to M1/M10/M5.
- [ ] M1's clean-clone rehearsal succeeds from README; repository access verified; `milestone-1` points to that tested commit; release smoke trigger mismatch handled explicitly.
- [ ] M10/M1 submit final PDF and repository URL before the verified portal cutoff; second member confirms; receipt retained.

### Work kept distinct from the shared Nov 1 minimum

Under S4: remaining pricing integration to Nov 4; estimate-to-create browser flow by Nov 5; admin pricing Nov 4–9; tracking/live hook Nov 6–10; dispatch/courier flow by Nov 8; merchant/webhook/live tracking by Nov 11; feature freeze Nov 12; as-built report/viva updates Nov 13; consolidated test report/coverage and two rehearsals Nov 14; final submission/demo and contribution sheet Nov 15. Pricing coverage target is 70%; several other slices target 60%. These later targets do not supersede S1 unless explicitly agreed.

The consolidated test report PDF and full event-flow demo are later team deliverables; actual tests/results for the code submitted now remain mandatory. AI and Kubernetes are excluded from this plan's deadline scope.

## 13. Verification of this planning deliverable

Verified this planning file exists, all seven PDF source links resolve, extraction reports seven PDFs/141 pages, and the recorded local/remote commit identifiers match Git. `git diff --check` and the new-file `git diff --no-index --check` completed without whitespace errors. Git status shows only the pre-existing untracked M4 plan and this new untracked plan; tracked product files are unchanged. The existing M4 plan was not edited.

Prettier was not run because application dependencies/pnpm were unavailable; no application build or test was run, as requested. Application behavior, CI, branch protection, the portal deadline and work on teammates' unmerged branches remain unverified. Re-audit development after new merges before estimating or implementing against it.
