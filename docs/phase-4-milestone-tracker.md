# Phase 4 Milestone Tracker

Planning baseline: application code `9b719c8daad32dc1a541e02da8d0ad4b768d65a5`; current documentation HEAD `c7665faaa598c0a656e49161d5321014d2eed594`.

All implementation tasks are **Not Started**. This tracker is planning-only; no feature branch or implementation PR exists yet.

## Status summary

| Milestone                            |  Tasks | Status          |
| ------------------------------------ | -----: | --------------- |
| 1 — Foundation and quality           |      3 | Not Started     |
| 2 — Customer workflow                |      1 | Not Started     |
| 3 — Dispatcher and courier workflow  |      2 | Not Started     |
| 4 — Cross-role integration           |      1 | Not Started     |
| 5 — Merchant and administration      |      2 | Not Started     |
| 6 — Reliability and final validation |      6 | Not Started     |
| **Total**                            | **15** | **Not Started** |

## Milestone 1 — Foundation and quality

| Milestone | Task ID | Audit finding | Priority | Dependencies            | Status      | Branch                                    | PR  | Acceptance criteria                                                                                                     | Verification evidence |
| --------- | ------- | ------------- | -------- | ----------------------- | ----------- | ----------------------------------------- | --- | ----------------------------------------------------------------------------------------------------------------------- | --------------------- |
| 1         | MS1-T01 | P4-08         | P2       | None                    | Not Started | `docs/phase-4-doc-truth`                  | —   | Readiness, migration versions, live-stack proof coverage and `.env.example` statements match source and actual tests.   | Pending               |
| 1         | MS1-T02 | P4-07         | P2       | MS1-T03 route inventory | Not Started | `test/phase-4-access-and-route-contracts` | —   | Every protected public route is in the access matrix or has an explicit approved exception; a broken guard still fails. | Pending               |
| 1         | MS1-T03 | P4-11         | P2       | None                    | Not Started | `test/phase-4-access-and-route-contracts` | —   | A new/changed public route without an OpenAPI entry or Nginx mapping fails a contract test; exceptions are documented.  | Pending               |

## Milestone 2 — Customer workflow

| Milestone | Task ID | Audit finding | Priority | Dependencies                                                   | Status      | Branch                          | PR  | Acceptance criteria                                                                             | Verification evidence |
| --------- | ------- | ------------- | -------- | -------------------------------------------------------------- | ----------- | ------------------------------- | --- | ----------------------------------------------------------------------------------------------- | --------------------- |
| 2         | MS2-T01 | P4-03         | P1       | Milestone 1 quality baseline; seeded customer and pricing data | Not Started | `test/phase-4-customer-journey` | —   | `/new` → `/parcels` → detail/history passes on the real stack and downloads a real PDF receipt. | Pending               |

## Milestone 3 — Dispatcher and courier workflow

| Milestone | Task ID | Audit finding | Priority | Dependencies                                                                          | Status      | Branch                               | PR  | Acceptance criteria                                                                                                     | Verification evidence |
| --------- | ------- | ------------- | -------- | ------------------------------------------------------------------------------------- | ----------- | ------------------------------------ | --- | ----------------------------------------------------------------------------------------------------------------------- | --------------------- |
| 3         | MS3-T01 | P4-04         | P2       | Milestone 2 customer fixture or unique API-created parcel; seeded dispatcher/couriers | Not Started | `test/phase-4-dispatcher-assignment` | —   | Assign and reassign pass through the actual modal; one documented refusal shows the safe error state.                   | Pending               |
| 3         | MS3-T02 | P4-02         | P1       | MS3-T01 assignment; proof storage; seeded courier                                     | Not Started | `test/phase-4-courier-completion`    | —   | `DELIVERED` with recipient and proof, and `FAILED` with reason, pass through the real courier UI; proof is retrievable. | Pending               |

## Milestone 4 — Cross-role integration

| Milestone | Task ID | Audit finding | Priority | Dependencies               | Status      | Branch                               | PR  | Acceptance criteria                                                                                                                                                               | Verification evidence |
| --------- | ------- | ------------- | -------- | -------------------------- | ----------- | ------------------------------------ | --- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| 4         | MS4-T01 | P4-01         | P1       | P4-02, P4-03, P4-04 merged | Not Started | `test/phase-4-cross-role-acceptance` | —   | One real-stack browser journey covers customer creation, dispatcher assignment, courier completion/failure, tracking, notification log and merchant order state where applicable. | Pending               |

## Milestone 5 — Merchant and administration

| Milestone | Task ID | Audit finding | Priority | Dependencies                                                        | Status      | Branch                         | PR  | Acceptance criteria                                                                                                                                    | Verification evidence |
| --------- | ------- | ------------- | -------- | ------------------------------------------------------------------- | ----------- | ------------------------------ | --- | ------------------------------------------------------------------------------------------------------------------------------------------------------ | --------------------- |
| 5         | MS5-T01 | P4-05         | P2       | Receiver/accelerated-retry decision; existing merchant webhook flow | Not Started | `test/phase-4-webhook-retries` | —   | Attempt 1 failure, recorded retry progression, final dead-letter/suppression and rotated-secret behavior are evidenced without waiting six real hours. | Pending               |
| 5         | MS5-T02 | P4-06         | P2       | MS4-T01 or a known parcel fixture; seeded ADMIN                     | Not Started | `test/phase-4-admin-mutations` | —   | Admin user create/edit/status, merchant decisions, pricing/zone edits and a non-empty reports dataset pass on the real stack.                          | Pending               |

## Milestone 6 — Reliability and final validation

| Milestone | Task ID | Audit finding | Priority              | Dependencies                                          | Status      | Branch                          | PR  | Acceptance criteria                                                                                                                         | Verification evidence |
| --------- | ------- | ------------- | --------------------- | ----------------------------------------------------- | ----------- | ------------------------------- | --- | ------------------------------------------------------------------------------------------------------------------------------------------- | --------------------- |
| 6         | MS6-T01 | P4-09         | P2                    | Seeded customer/dispatcher; running Compose           | Not Started | `perf/phase-4-write-path`       | —   | Authenticated parcel creation/assignment and async-drain scenarios run against Compose; baseline is recorded with limits.                   | Pending               |
| 6         | MS6-T02 | P4-10         | P2                    | Readiness endpoints; disposable Compose project       | Not Started | `test/phase-4-outage-recovery`  | —   | Stopping/restarting RabbitMQ flips affected readiness to down and back to ready; a processed event proves recovery; CI fails on regression. | Pending               |
| 6         | MS6-T03 | P4-13         | P3 (optional/stretch) | Disposable database; existing migration scripts       | Not Started | `test/phase-4-restore-rollback` | —   | Restore rehearsal and broader rollback tests pass on disposable targets and the runbook is accurate.                                        | Pending               |
| 6         | MS6-T04 | P4-12         | P3 (optional/stretch) | Mailpit; seeded customer; optional Google credentials | Not Started | `test/phase-4-auth-browser`     | —   | Password reset is proven from email link to new-password login; Google smoke is protected/manual if implemented.                            | Pending               |
| 6         | MS6-T05 | P4-14         | P3 (optional)         | Gemini key supplied outside source control            | Not Started | `chore/phase-4-ai-smoke`        | —   | Protected/manual AI smoke is repeatable; normal CI stays deterministic and offline.                                                         | Pending               |
| 6         | MS6-T06 | P4-15         | P3 (decision)         | Owner approval of retention durations/disposition     | Not Started | `docs/phase-4-retention-policy` | —   | Approved policy exists; no cleanup implementation starts before approval.                                                                   | Pending               |

## Dependency map

```text
MS1-T03 (route inventory) ──> MS1-T02 (access matrix)
MS1-T02 + MS1-T01 ────────────> quality baseline

MS2-T01 (customer journey)
        │
MS3-T01 (dispatcher UI) ──> MS3-T02 (courier completion/proof)
        │                         │
        └──────────┬──────────────┘
                   v
             MS4-T01 (cross-role acceptance)
                   │
                   ├──> MS5-T02 (reports with known parcel)
                   └──> MS6-T01 (performance fixture)

MS5-T01 (webhook retries) ──> MS6-T02 (recovery evidence)
MS6-T03, MS6-T04, MS6-T05, MS6-T06 ──> optional/stretch or decision
```

## Verification commands by task

| Task    | Minimum local verification                                                                                                 |
| ------- | -------------------------------------------------------------------------------------------------------------------------- |
| MS1-T01 | `pnpm exec prettier --check docs README.md`; manual `.env.example` review                                                  |
| MS1-T02 | `make test-access`; `go test ./tests/access/accesskit/...`                                                                 |
| MS1-T03 | `node --test "tests/contracts/*.test.mjs"`; affected service router tests; `nginx -t`                                      |
| MS2-T01 | `pnpm exec nx lint @org/web-e2e`; `pnpm exec nx typecheck @org/web-e2e`; real-stack customer spec; full real-stack project |
| MS3-T01 | New real-stack dispatcher spec; existing dispatch mocked suite                                                             |
| MS3-T02 | New real-stack courier completion spec; parcel Go status/proof tests; full real-stack project                              |
| MS4-T01 | Cross-role real-stack spec; event-flow integration; full real-stack project                                                |
| MS5-T01 | Webhook real-stack retry/rotation spec; webhook worker unit/integration tests; event-flow regression                       |
| MS5-T02 | Admin real-stack mutations; non-empty reports; affected Go integration tests                                               |
| MS6-T01 | `k6 run` write-path/async scenarios; recorded baseline                                                                     |
| MS6-T02 | CI recovery job / disposable stack script                                                                                  |
| MS6-T03 | Restore rehearsal and rollback tests on a disposable database                                                              |
| MS6-T04 | Mailpit-backed reset spec; optional Google job                                                                             |
| MS6-T05 | Manual/protected AI smoke; deterministic AI tests remain green                                                             |
| MS6-T06 | Policy review/approval; no cleanup code until approved                                                                     |

## Tracker rules

- Mark a task complete only after its acceptance criteria are observed, its tests
  pass, and the PR is merged into `development`.
- Update the PR link only after the PR exists.
- Do not start a dependent task while its blocker is unmerged unless the plan
  explicitly marks the dependency as fixture-only.
- Keep optional/stretch tasks clearly marked; they do not block Phase 4 exit.
- Do not change this tracker to claim evidence that was not actually run.
