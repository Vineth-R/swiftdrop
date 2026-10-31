# M4: requirements and plan for 1 November 2026

Prepared on 31 October 2026 from all seven PDFs in `C:/Users/Thisal K/Downloads/Swiftdrop`.
This is a document-based requirements analysis, not an audit of completed repository work. Unchecked items mean completion has not been verified.

## 1. Resolve the scope conflict without losing the common requirements

The documents disagree:

| Source | What it says about M4 and 1 November |
| --- | --- |
| M4.pdf, pp. 1–3 | Explicitly requires pricing AND actual public tracking by 1 November, with tests, integration evidence, documentation, PR and demo materials. Frontend is conditional on assigned scope. |
| Nov 1 Submission Guide, p. 5 | Requires pricing migrations/seed, estimate and zones endpoints, Sections 6.1 and 8, Figures 8 and 9. A sample-data mobile tracking layout is optional. |
| Milestone 2 Submission Plan, p. 3 | Same M4 Must/Should split as the guide. Its title calls this Milestone 2, but p. 1 says the earlier plan calls it Milestone 1 and full implementation is due 15 November. |
| Team Task Sheet, pp. 10–12 | Seed and estimate due 1 November; initial pricing work continues to 4 November. Admin pricing is 4–9 November; actual tracking is 6–10 November; consolidated test report is 9–14 November. |

No document establishes that it supersedes the others. Ask the team lead whether M4.pdf is the final scope. Until confirmed, complete the shared minimum first, then the actual tracking requirements in M4.pdf; do not silently drop that explicit deadline. This question does not block migrations, pricing, report writing or tests.

Team submission: one combined design report PDF and one repository link. M10 assembles/submits the report; M1 owns repository readiness, clean-clone test and tag `milestone-1`. Your responsibility is to deliver your reviewed, integrated contribution and supporting evidence to them.

## 2. Shared minimum checklist: required by the team guides

### A. Database and seed

- [ ] Connect the pricing service to PostgreSQL through M1's shared helper; verify readiness.
- [ ] Verify the existing migrations before adding anything. Ensure `pricing.zones`, `pricing.pricing_rules`, `pricing.weight_bands`, `pricing.size_surcharges` and `pricing.audit_logs` exist with the DB v1.1 structure.
- [ ] Provide missing changes as versioned up/down migrations. Repository guidance reserves 000002 for baseline tables; use new versions from 000003, and do not rewrite applied migrations or duplicate existing tables.
- [ ] Enforce unique zone codes, nonnegative monetary values and one active rule per origin/destination pair.
- [ ] Enforce active-band non-overlap in application validation and the database, using the documented `(min, max]` interval semantics. Adjacent bands may touch.
- [ ] Provide reproducible seed data: active Colombo/CMB and Kandy/KDY zones, their active route rule, appropriate weight bands, and size surcharges.
- [ ] Demonstrate 2 kg SMALL Colombo → Kandy = LKR 550.00: base fee 450.00 + weight charge 100.00 + size surcharge 0.00.
- [ ] Test migrations on a fresh database and an existing development database; test rollback only on a confirmed disposable target.
- [ ] Give M1 migration/seed instructions and M5 confirmation of your five tables for the ER diagram.

Source: Guide p. 5; Submission Plan p. 3; Task Sheet pp. 10–11; DB document §§4.1–4.5, pp. 9–11.

### B. Pricing backend and contract handoff

- [ ] Implement public `POST /api/v1/pricing/estimate`; Gin registers `/pricing/estimate` because Nginx strips `/api/v1`.
- [ ] Accept `originZoneId`, `destinationZoneId`, `weightKg` and `sizeCategory` with contract validation.
- [ ] Calculate `total = baseFee + weightCharge + sizeSurcharge` using real pricing data, with separate calculators as assigned in the Task Sheet.
- [ ] Return the API v1.1 `data`/`meta` response with zone IDs, weight, size, LKR currency, all three components and total. Use exact decimal money handling.
- [ ] Implement public `GET /api/v1/zones`, returning active zones with `id`, `code`, `name`, `districts`.
- [ ] Handle unknown zones (`404 ZONE_NOT_FOUND`), invalid input (`422 VALIDATION_ERROR`), an unpriced route (`422 ROUTE_NOT_SERVICEABLE`) and a valid weight in no active band (`422 WEIGHT_NOT_PRICED`).
- [ ] Handle database failures safely through shared error/logging helpers; do not expose SQL or credentials.
- [ ] Implement public-estimate rate limiting as required by the API; agree any unspecified limit rather than inventing a contract value.
- [ ] Share `POST /internal/pricing/quote` with M5 today: same input/output as estimate, error codes, connection details, timeout/failure behavior and required service authentication. Keep `/internal` inaccessible through the public gateway.
- [ ] Prefer implementing the internal quote alongside estimate with shared logic. The Task Sheet includes it in M4.1, although only seed/estimate are explicitly labelled Nov 1 and the shorter guides list only estimate/zones.
- [ ] Record `GET /internal/zones/{zoneId}` as another API-defined Parcel→Pricing dependency; agree delivery timing with M5 rather than assume the Nov 1 guides explicitly require it.
- [ ] Verify public routes through Nginx and run the affected services in Docker Compose.

Source: API §§3.1–3.2, pp. 14–15 and internal APIs p. 44; Task Sheet M4.1.

### C. Report sections and figures

- [ ] Write/update Section 6.1, "Create a delivery (customer)".
- [ ] Redraw Figure 8, the creation sequence diagram, with editable draw.io source and an exported figure.
- [ ] Redraw Figure 9, the creation activity diagram, with editable source and an exported figure.
- [ ] Show zones → estimate → confirmation with `expectedFee` and `Idempotency-Key` → internal quote → price comparison → transactional parcel/history/outbox save → response and eventual publication.
- [ ] Include the alternative branches: `422 ROUTE_NOT_SERVICEABLE`, `422 WEIGHT_NOT_PRICED`, duplicate idempotency key returning the original parcel, and `409 PRICE_CHANGED` with `currentFee` and reconfirmation.
- [ ] Ensure the sequence diagram shows publication after the transaction; do not imply the HTTP response waits for broker publication.
- [ ] Use SDK customer tracking numbers and SDN merchant tracking numbers; match implemented contracts for event names and envelope fields. Flag PDF/repository mismatches rather than copying stale examples.
- [ ] Write/update Section 8, "Design Patterns and Principles", with concrete code file references for every implemented claim and a short explanation of why each choice fits.
- [ ] Cover the existing Section 8 entries: layering, repository, strategy, state, publish/subscribe, adapter, outbox, gateway, snapshot read model, dependency injection, single responsibility, open/closed, interface segregation/dependency inversion, DRY, idempotency and fail-fast configuration.
- [ ] Ask relevant owners for code references outside your slice. Mark absent patterns planned/unimplemented; do not claim planned business behavior exists or introduce empty abstractions just to match report prose.
- [ ] Commit section drafts in `docs/report` and diagram sources in `docs/diagrams`, with captions/figure numbers for M10.
- [ ] Have M3 review your sections; fix their comments before the report freeze.
- [ ] Review M5's Section 3 and Figure 6: 26 tables in five schema groups, dashed cross-service references, correct pricing tables and constraints, DB document title v1.1. Return corrections to M5.
- [ ] Give M9 accurate status, code locations, tests and demo proof for your traceability rows.

Source: Task Sheet M4.2 and M5 report task; Guide pp. 5–6; Design Report §6.1, pp. 14–16 and §8, pp. 26–27.

### D. Tests, evidence and integration

- [ ] Pricing unit tests: formula, all relevant size categories, positive/invalid weights, exact band boundaries and gaps, missing route/zone and inactive data.
- [ ] Explicitly demonstrate the 550.00 seed example, ROUTE_NOT_SERVICEABLE and WEIGHT_NOT_PRICED.
- [ ] Database integration tests: repositories, migration application and constraints, particularly active-band overlap. Keep live infrastructure out of ordinary unit tests.
- [ ] Handler/contract tests for success and failure responses, and public zones filtering.
- [ ] Quote parity check: estimate and internal quote produce the same fee from the same inputs/data.
- [ ] Save actual test commands/output, gateway requests/responses, and a short demo or screenshots. Clearly record failed or unexecuted checks.
- [ ] Run configured Nx build/test/lint targets for affected projects through pnpm after inspecting the actual targets; do not guess commands or targets.
- [ ] Check Compose integration and obtain a green real CI run for the submitted changes.

Clarification to settle: API §3.1 calls a weight above the maximum band VALIDATION_ERROR, while the team acceptance text says a weight outside every band is WEIGHT_NOT_PRICED. Use a gap within the supported range to demonstrate WEIGHT_NOT_PRICED; agree and test the above-maximum rule separately.

## 3. Additional explicit requirements in M4.pdf

These are not optional if M4.pdf is the final instruction. They conflict with the later dates in the team sheet.

### E. Actual public tracking

- [ ] Implement `GET /api/v1/tracking/{trackingNumber}` backed by actual parcel and status-history records, without customer authentication.
- [ ] Keep implementation in the Parcel service, which owns the tables and gateway route. Coordinate edits with M5/M6/M7. Do not query the parcel schema from the Pricing service.
- [ ] Retrieve real current status and ordered history; support CREATED, PICKED_UP, IN_TRANSIT, OUT_FOR_DELIVERY, DELIVERED and FAILED. Coordinate lifecycle behavior with M6/M7 rather than duplicate their transition engine.
- [ ] Return only public fields: `trackingNumber`, `status`, `pickupCity`, `dropoffCity`, `lastUpdatedAt`, and timeline entries containing `status`/`at`.
- [ ] Exclude names, addresses, phone numbers, notes, proof files and other private fields from responses, UI and logs.
- [ ] Return `404 TRACKING_NOT_FOUND` for an unknown number and enforce the Task Sheet's 30 requests/minute/IP limit with 429 behavior.
- [ ] Coordinate any missing tracking lookup index/migration with the Parcel owner; do not duplicate an existing unique tracking-number index.
- [ ] Test successful anonymous lookup, every lifecycle status, actual ordered history, unknown numbers, privacy and rate limiting; save gateway integration evidence.
- [ ] Provide the public `/track` UI if the full assigned frontend scope applies: 360 px layout, loading/error/empty/not-found/success states, accessible timeline and real API calls through the gateway.
- [ ] Live updates via M10's hook are explicitly scheduled later in the Task Sheet. Record them separately from a working REST lookup if the hook is not ready; a sample-data page does not satisfy actual tracking.

Source: M4.pdf pp. 1–2; API §4.6, pp. 22–23; Task Sheet M4.4.

### F. Pricing management and frontend scope

M4.pdf requires pricing management "where required by the specification" and frontend "if ... included in your assigned scope". The full Task Sheet assigns both, but schedules admin work for 4–9 November. Confirm whether those later dates still apply.

- [ ] If accelerated into Nov 1: implement API §§3.3–3.12, including zone create/update; rule list/create/update; band list/create/update; surcharge list/update.
- [ ] Enforce ADMIN access and audit changes; map `ZONE_CODE_EXISTS`, `RULE_EXISTS`, `BAND_OVERLAP` and validation errors correctly.
- [ ] Provide four admin screens: zones, rules, bands and surcharges; verify that pricing changes affect new estimates while existing stored parcel fees remain unchanged.
- [ ] Test authorization, validation, concurrent constraint enforcement and the relevant browser flows.

### G. M4 delivery package

- [ ] Source code and any required migrations/seed files.
- [ ] Updated OpenAPI specifications, pricing rules, tracking usage/response documentation and migration instructions where applicable.
- [ ] Test instructions plus actual automated test results and integration evidence.
- [ ] Short implementation report: completed features, architectural decisions, dependencies, limitations and incomplete work.
- [ ] Screenshots or a short pricing/tracking demo; distinguish live data from sample layouts.
- [ ] Changes committed and pushed to the designated feature branch; PR targeting `development`; PR URL and completion summary handed over. Do not commit directly to `development` or `main`.
- [ ] M5 PR review, passing CI, and integration/merge coordinated before M1 selects the tested submission commit. A branch alone is not integrated work under the Task Sheet's definition of done.

Source: M4.pdf pp. 1–3 and Task Sheet pp. 1–2.

## 4. Execution order and completion gates

Suggested effort blocks below are planning estimates, not promises. Remaining time depends on current implementation progress, which has not been audited.

| Order | Work block | Suggested effort | Completion gate / handoff |
| --- | --- | --- | --- |
| 1 | Resolve scope and contracts; inspect existing code/migrations | 20–30 min | Scope conflict recorded; M1 DB details and M5 quote contract agreed; no duplicate migration planned |
| 2 | Draft Sections 6.1/8 and Figures 8/9 immediately | 60–90 min | Drafts/source figures committed; M3 can review and M10 can assemble without waiting for code |
| 3 | Verify/fill pricing schema and seed | 45–90 min | Fresh DB applies; seed yields the documented components; constraints tested; M1/M5 receive handoff |
| 4 | Implement pricing logic, estimate, zones and internal quote | 2–3 h | Shared calculation logic, contract responses, validation and repository tests pass |
| 5 | Gateway/Compose/CI and evidence for core pricing | 45–60 min | 550 example and negative cases proven through the gateway; output saved; PR review requested |
| 6 | Real tracking API if M4.pdf applies, or while awaiting scope resolution after core work | 1–2 h, dependent on Parcel readiness | Real public projection and history, privacy/unknown-number/rate-limit tests; owner-reviewed Parcel changes |
| 7 | Tracking UI; admin management only if confirmed accelerated | UI 45–90 min; admin work materially longer | Real API integration and required UI tests; no claim that full scope fits one evening |
| 8 | Fix report review comments, review M5, package and hand over | 45–60 min | Final sections/figures, actual code references, evidence and limitations sent to M10; PR/CI links sent to M1/M5 |

Do report drafts early because their freeze can precede implementation completion. Finish technical code references after the implementation is known. If a dependency is unavailable, work on the report, pure calculation tests or contract fakes, and record the dependency; fakes are not final integration evidence.

## 5. Team checkpoints: Sri Lanka time

The documents contain two schedules. The Task Sheet/Submission Plan specify report drafts at 3 pm and reviews by 7 pm. The evening Nov 1 Guide shifts drafts to 8 pm and reviews to 8:30 pm. Both retain the 9 pm report freeze. Treat the earlier checkpoints as missed if already elapsed and use the evening schedule for recovery, subject to team confirmation.

| Deadline | Your action |
| --- | --- |
| Oct 31, immediately | Agree scope; send M5 the quote contract; start report drafts and migration verification |
| Oct 31, 8 pm recovery checkpoint | Migration SQL and seed ready; section/figure drafts committed and sent to M10 |
| Oct 31, 8:30 pm | M3 reviewing your report; your review of M5 returned |
| Oct 31, 9 pm | All report review comments fixed; report frozen |
| Oct 31, 9–11 pm | Support M10's assembly and M1's clean-clone test; deliver verified code/evidence for the tested commit |
| Nov 1, 9 am | Check your sections/figures in the final PDF; confirm submitted repo includes your intended contribution |
| Nov 1, by noon, or earlier if the portal requires | M10/M1 submit PDF + repository URL; obtain confirmation and retain links/evidence |

Noon is the team's target, not a verified university portal cutoff. Confirm the actual portal time with the team.

## 6. Later work under the team schedule

Do not confuse these with the shared Nov 1 minimum; the M4.pdf conflict remains relevant.

| Dates | M4 responsibility |
| --- | --- |
| Through Nov 4 | Finish remaining M4.1 pricing/internal integration work |
| Nov 5 checkpoint | Estimate → customer parcel creation works in the browser with saved data, jointly with M5 |
| Nov 4–9 | Admin pricing endpoints and four screens |
| Nov 6–10 | Full public tracking endpoint/page, mobile privacy and realtime integration with M10 |
| Nov 9–14 | Own pricing/tracking tests; pricing coverage at least 70%; collect team test/coverage/event-flow results into a reproducible test report PDF linked from README |
| By Nov 12 | Collect owners' results; feature freeze, then bug fixes |
| By Nov 13 | Send as-built report updates to M10; prepare viva notes and contribution details |
| Nov 14 | Final test report and two timed demo rehearsals |
| Nov 15 | Final submission/demo; team contribution sheet, coverage summary and full feature demonstration |

## 7. Source map

All paths are under `C:/Users/Thisal K/Downloads/Swiftdrop/`:

1. `M4.pdf`: all three pages; dedicated deadline, functional and evidence requirements.
2. `SwiftDrop Nov 1 Submission Guide.pdf`: pp. 1–3 team scope/dependencies, p. 5 M4, p. 6 M5 review context, pp. 9–10 timing/final checklist.
3. `SwiftDrop Milestone 2 Submission Plan.pdf`: p. 1 submission scope, p. 3 M4, pp. 5–6 schedule/final checklist.
4. `SwiftDrop-Team Task Sheet.pdf`: pp. 1–4 rules/dates, pp. 10–12 full M4 card, pp. 20–21 final tasks.
5. `SwiftDrop API Documentation (v1.1, consolidated).pdf`: §§3.1–3.12, 4.1, 4.6 and internal APIs; exact routes, fee response, errors and public tracking projection.
6. `SwiftDrop Database Design Document (v1.1, aligned with API v1.1).pdf`: §§3.6, 4.1–4.5 and Parcel tables; pricing constraints, audit structure and data ownership.
7. `SwiftDrop_Design_Report_v1.1.pdf`: §6.1/Figures 8–9 and §8; the report content to revise and validate against the actual implementation.
