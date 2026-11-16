# Phase 4 Completion Report

Status: implementation complete for the Phase 4 workflow scope. The approved
P4-13 backup/restore/rollback verification and P4-15 retention are implemented;
P4-14 remains an optional operational experiment.

## 1. Baseline and merged delivery

| Item                              | Value                                             |
| --------------------------------- | ------------------------------------------------- |
| Implementation base               | `development`                                     |
| Phase 3 audit baseline            | `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`        |
| Phase 4 completion head at report | `d9783730769c68b1a83ae3e1ab794c2e21131932`        |
| Merge strategy                    | Merge commits through PRs targeting `development` |

Merged implementation PRs:

| PR   | Merge commit                               | Scope                                                                         |
| ---- | ------------------------------------------ | ----------------------------------------------------------------------------- |
| #160 | `981006c53ba304e4c9fc906bd210bcb15b3a4298` | Access matrix, route coverage, documentation truth, planning-artifact cleanup |
| #161 | `85aa8fca39caa595931add26053f3b5c779729fa` | Real-stack customer booking/list/detail/history/receipt                       |
| #162 | `ced8a602059d3fe89d16d4c762a87adb856d27bf` | Real-stack courier delivery, failure and proof                                |
| #163 | `004722f1c76d70bbf39ee16a1a407afdca02f716` | Real-stack dispatcher assignment/reassignment and validation                  |
| #164 | `d50f97e2debe189d88f84864404e225d6a5a7c30` | Cross-role customer → dispatcher → courier acceptance                         |
| #165 | `56839e0b9055b0994ae98e03f53de7751215ab56` | ADMIN user/merchant/pricing mutations and non-empty reports                   |
| #167 | `bbc789f871aeab1c6ec523005256b44b5d224013` | Webhook retry, exhaustion, attempt history and rotation                       |
| #168 | `c3431aaad0adf2d2c890a811add8f2fd29d376da` | Rapid-transition tracking reconciliation                                      |
| #169 | `baa4979c6b33062991ea3b99476565eb67af142e` | Authenticated write-path k6 baseline                                          |
| #170 | `6c8441998a3ffa19e6f3529425b953a53b185b58` | RabbitMQ outage and post-recovery event flow                                  |
| #171 | `d9783730769c68b1a83ae3e1ab794c2e21131932` | Real-stack password recovery through Mailpit                                  |

PR #166 was closed and replaced by #167 before merge after its first branch
history briefly contained a test-only key fixture. The final branch and merge
history contain no private key; the receiver generates an ephemeral certificate
at startup.

## 2. Findings status

| Finding                                 | Priority | Status                            | Evidence                                                                                                                                       |
| --------------------------------------- | -------- | --------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------------- |
| P4-01 Cross-role acceptance             | P1       | Complete                          | `customer-delivery-journey.real.spec.ts`                                                                                                       |
| P4-02 Courier completion/proof          | P1       | Complete                          | `courier-completion.real.spec.ts`                                                                                                              |
| P4-03 Customer UI journey               | P1       | Complete                          | `customer-parcel-journey.real.spec.ts`                                                                                                         |
| P4-04 Dispatcher UI assignment          | P2       | Complete                          | `dispatcher-assignment.real.spec.ts`                                                                                                           |
| P4-05 Webhook retry/exhaustion/rotation | P2       | Complete                          | `webhook-reliability.real.spec.ts`, worker/confix tests                                                                                        |
| P4-06 Admin mutations/reports           | P2       | Complete                          | `admin-mutations.real.spec.ts`, `admin-pricing-reports.real.spec.ts`                                                                           |
| P4-07 Access matrix                     | P2       | Complete                          | 56-route matrix in `tests/access/matrix_test.go`                                                                                               |
| P4-08 Documentation truth               | P2       | Complete                          | Readiness/migration/test docs corrected in #160                                                                                                |
| P4-09 Performance evidence              | P2       | Complete                          | `tests/perf/delivery-flow.js`, `docs/phase-4-performance-baseline.md`                                                                          |
| P4-10 Outage recovery                   | P2       | Complete                          | `tests/recovery/outage-recovery.sh`, real-stack CI job                                                                                         |
| P4-11 OpenAPI/route coverage            | P2       | Complete                          | Runtime Gin inventories in `contracts/routes`, OpenAPI cross-check and Nginx upstream validation in `tests/contracts/route-inventory.test.mjs` |
| P4-12 Auth browser journeys             | P3       | Complete (reset); Google optional | `auth-reset.real.spec.ts`; no live Google dependency added                                                                                     |
| P4-13 Restore/rollback                  | P3       | Complete                          | Disposable restore rehearsal, automatic post-restore validation and all-service golang-migrate up/down/reapply cycle in CI                     |
| P4-14 AI live provider                  | P3       | Deferred/optional                 | Deterministic AI tests remain the merge gate; no live provider dependency added                                                                |
| P4-15 Retention policy                  | P3       | Complete                          | Approved: notification logs 180 days, merchant reservations indefinite, processed events unchanged at 30 days; see `docs/data-retention.md`    |

## 3. Delivered workflows

### Customer

- Registration, email verification, login, refresh, logout and profile remain
  implemented.
- Real-stack booking through `/new` with real zones, quote and contact fields.
- Owned list, detail, history and a real PDF receipt download.
- Public tracking, including rapid-transition reconciliation.

### Dispatcher and courier

- Real-stack assignment and reassignment through the actual modal; empty-courier
  validation is covered.
- Courier Today/detail, supported transitions, successful delivery with
  recipient/photo proof, proof retrieval, and failed delivery with a reason.
- Cross-role acceptance verifies customer tracking, dispatcher state and ADMIN
  notification records.

### Merchant and administration

- ADMIN user create/edit/deactivate/reactivate with persisted state and denied
  login while inactive.
- Merchant approve/suspend/reject through the real UI, including suspended login
  denial and persisted status after reload.
- Zone, pricing rule, weight band and size surcharge mutation through the real
  UI, with a real quote and parcel created from the changed pricing.
- Non-empty daily/outcome report verified against the created parcel.
- CUSTOMER access to an ADMIN mutation route is refused.

### Event-driven and realtime

- Webhook configuration uses the real merchant flow.
- A transient failure retries and succeeds; a persistent failure records all
  six attempts and ends dead-lettered.
- Attempt ordering, retry metadata, persisted delivery history and signatures
  are verified.
- Secret rotation is verified: subsequent deliveries use the new secret and the
  old secret no longer verifies.
- Rapid PICKED_UP → IN_TRANSIT → OUT_FOR_DELIVERY → DELIVERED transitions keep
  tracking's final headline and complete five-item history.
- RabbitMQ outage makes dependent readiness fail, leaves identity/pricing ready,
  and recovers; a post-recovery parcel status reaches tracking.

## 4. Verification evidence

Observed across the implementation PRs:

- `go test ./...` — pass.
- `go vet ./...` — pass.
- `gofmt` — clean.
- `make check-openapi` — 19 contract tests pass.
- `make test-access` — 56 endpoints pass across anonymous and five roles.
- Customer, dispatcher, courier and cross-role real-stack specs — pass locally.
- Admin real-stack mutations and non-empty reports — 3 scenarios pass locally.
- Webhook reliability — 3 scenarios pass locally against Compose.
- Tracking rapid-transition regression — pass locally.
- Clean-worktree `go test ./...` and Compose config validation for the webhook
  delivery — pass.
- k6 delivery flow, 1 VU / 10 s — 10 iterations, 55 requests, 0 failures,
  55/55 checks, HTTP p95 232 ms, tracking propagation 100%. This is local
  evidence, not a production SLO.
- Playwright e2e lint/typecheck pass for changed specs.
- Each merged PR passed the required CI checks before merge. The post-merge CI
  run for `d978373` is recorded separately in the final handover.

## 5. Known limitations

- P4-11 now snapshots `gin.Engine.Routes()` per service and cross-checks every
  route with OpenAPI and the gateway upstream. Operational probes registered by
  service entrypoints (`/health`, `/ready`, `/metrics`) remain explicit
  exemptions because they are mounted outside the business routers.
- P4-13 is complete: `make db-rehearse-restore` creates isolated source/target
  databases, uses the real backup and restore scripts, and validates schemas,
  data, migration versions, dirty state and the notification retention index.
  `TestAllServiceMigrationCycles` covers fresh up, down-one, reapply and clean
  version tracking for every service.
- P4-14 live Gemini smoke remains an opt-in/manual procedure; normal CI never
  calls the provider.
- P4-15 is complete. Notification delivery logs are deleted after 180 days by
  default through an independently configurable worker job; merchant order
  reservations are retained indefinitely and processed-event cleanup remains at
  30 days. See `docs/data-retention.md`.
- Shared rate limiting, AI conversation state, realtime replay/multi-replica
  fan-out, production tracing/monitoring and TLS/Kubernetes/Terraform remain
  outside the agreed Phase 4 scope.
- The webhook accelerated retry schedule and test origin are rejected outside
  `APP_ENV=development`; production uses the 1m/5m/30m/2h/6h schedule and normal
  HTTPS/SSRF validation.

## 6. Exit state

The core Phase 4 workflow and integration evidence is delivered. The only open
Phase 4 item is the optional P4-14 live Gemini experiment. P4-13 and P4-15 are
complete: recovery is rehearsed against disposable databases, and only
notification delivery logs are deleted after the approved 180-day window.
