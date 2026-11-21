# SwiftDrop — Technical Issues and Resolutions

This is the defect register for the final acceptance pass. It records only issues
that were confirmed against the current code. Items already fixed before this
pass are marked as such and are not presented as new work.

Severity: **Critical**, **High**, **Medium**, **Low**, **Informational**.

## Summary

| ID      | Severity | Issue                                                                           | Status                        |
| ------- | -------- | ------------------------------------------------------------------------------- | ----------------------------- |
| DEF-001 | High     | Logout failed after the access token expired and left the refresh session alive | Fixed                         |
| DEF-002 | Medium   | Concurrent refresh could leave a live successor after logout                    | Fixed                         |
| DEF-003 | Medium   | No refresh rate limit                                                           | Fixed                         |
| DEF-004 | Medium   | No per-customer parcel creation rate limit                                      | Fixed                         |
| DEF-005 | Medium   | No per-merchant webhook test-send rate limit                                    | Fixed                         |
| DEF-006 | Medium   | Failed outbox events had no retention or replay tool                            | Fixed (operator tooling)      |
| DEF-007 | Medium   | Audit-log and webhook-attempt retention was undefined                           | Fixed (policy + script)       |
| DEF-008 | Medium   | Access matrix treated logout as requiring an access token                       | Fixed                         |
| DEF-009 | Low      | Frontend kept the old identity in memory after a failed logout                  | Fixed                         |
| DEF-010 | Low      | Merchant order recovery after a post-Parcel persistence failure was untested    | Fixed (regression test)       |
| DEF-011 | Low      | Two accidental duplicate files were untracked in the working tree               | Fixed                         |
| DEF-012 | Low      | API 8.17/8.18 documentation says JWT only; code also accepts an API key         | Open (documentation decision) |
| DEF-013 | Low      | Audit rows are written but no read API/UI exists                                | Open (scope)                  |

---

## DEF-001 — Logout after access-token expiry

| Field             | Value                  |
| ----------------- | ---------------------- |
| Severity          | High                   |
| Affected services | Identity, web frontend |
| Members           | M2                     |
| Status            | Fixed                  |

### Symptom

After the 15-minute access token expired, `POST /api/v1/auth/logout` required a
valid access token. The browser client does not auto-refresh `/auth/*` paths. The
request returned `401 TOKEN_EXPIRED`; the handler never ran, so the refresh token
was not revoked and both cookies were not cleared. The UI still navigated to
`/signed-out`, but the next app page loaded `getMe()`, which triggered
`/auth/refresh` with the surviving refresh cookie and silently restored the
session.

### Root cause

Logout authenticated with the wrong credential. It used the short-lived access
token (`auth.Authenticate`) instead of the refresh token, which is the actual
session credential.

### Resolution

- `POST /auth/logout` no longer uses `auth.Authenticate`.
- The handler reads the refresh token from the cookie or `X-Token-Mode: body`.
- `Service.Logout(ctx, refreshToken)` hashes the token and calls the new
  `Repository.RevokeRefreshSession`.
- Cookies are cleared before revocation is attempted, including on a service
  error.
- Missing, malformed, unknown and already revoked tokens return 204.
- Other protected endpoints still require a valid access token; only logout
  changed.

### Regression tests

- `TestAuthHandlers_logout/revokes_the_refresh_session_without_an_access_token`
- `TestAuthHandlers_logout/works_with_an_expired_access_token_and_a_valid_refresh_token`
- `TestAuthHandlers_logout/is_idempotent_without_tokens_and_when_repeated`
- `TestAuthHandlers_logout/a_service_failure_is_a_generic_500_but_still_clears_the_cookies`
- `TestService_Logout/revokes_the_refresh_session`
- `TestService_Logout/is_idempotent_and_succeeds_without_a_refresh_token`
- `TestRepository_RevokeRefreshSession` (integration)
- `tests/access/sessions_test.go::TestSessions_logoutEndsTheRefreshToken`

---

## DEF-002 — Concurrent refresh/logout race

| Field             | Value    |
| ----------------- | -------- |
| Severity          | Medium   |
| Affected services | Identity |
| Members           | M2       |
| Status            | Fixed    |

### Symptom

If a refresh completed between the client reading its cookie and sending
logout, the browser could present the old token while the server had already
issued a successor. A naive logout that only revoked the presented hash would
leave the successor valid.

### Root cause

Logout had no notion of a token family and did not lock the token row.

### Resolution

`RevokeRefreshSession`:

1. begins a transaction;
2. locks the token row `FOR UPDATE`;
3. if the token is already revoked **and** has a successor, calls `revokeChain`
   to revoke the whole descendant chain;
4. otherwise revokes the presented row;
5. commits.

A concurrent refresh either wins (logout then sees the successor and revokes it)
or loses (it sees the revoked row and fails). No ordering leaves a live session.

### Regression tests

- `TestRepository_RevokeRefreshSession/a_rotated_token_revokes_its_successor`
- `TestRepository_RevokeRefreshSessionConcurrentWithRefresh` (20 iterations)
- `TestService_Logout/revokes_the_successor_when_a_concurrent_refresh_already_rotated_the_token`

---

## DEF-003 — Refresh endpoint had no rate limit

| Field             | Value    |
| ----------------- | -------- |
| Severity          | Medium   |
| Affected services | Identity |
| Members           | M2       |
| Status            | Fixed    |

### Root cause

Only login, registration, verification and reset had limiters. Refresh is a
credential exchange that touches the database.

### Resolution

Added a per-client fixed-window limiter of **60 refresh attempts per minute**
before the handler. The token remains high-entropy; this is an abuse bound, not
a brute-force control.

### Regression tests

Router/handler tests continue to exercise refresh; the limiter is the shared
`servicehttp.rateLimiter`, already unit-tested in the identity package.

---

## DEF-004 — No per-customer parcel creation limit

| Field             | Value           |
| ----------------- | --------------- |
| Severity          | Medium          |
| Affected services | Parcel, Pricing |
| Members           | M5              |
| Status            | Fixed           |

### Root cause

`POST /parcels` had authentication and role checks but no rate limit. Each
request performs an internal pricing call and writes three rows.

### Resolution

Added a route-level limiter of **30 creations per minute per verified customer**
(`MiddlewareKeyed` on the token subject, falling back to client IP only if the
claim is somehow absent).

### Regression tests

- `TestParcelCreateIsRateLimitedPerCustomer` — 30 admitted requests, 31st is
  `429 RATE_LIMITED` with `Retry-After`.

---

## DEF-005 — Webhook test-send amplification

| Field             | Value    |
| ----------------- | -------- |
| Severity          | Medium   |
| Affected services | Merchant |
| Members           | M8/M9    |
| Status            | Fixed    |

### Root cause

`POST /merchants/me/webhook/test` performs an outbound HTTP request to the
merchant's endpoint. It had no rate limit, so one merchant could use it as a
repeated request amplifier.

### Resolution

Added **10 test sends per minute per merchant** before the handler.

### Regression tests

- `TestTestSendIsRateLimitedPerMerchant`

---

## DEF-006 — Failed outbox events had no retention or replay tool

| Field             | Value                    |
| ----------------- | ------------------------ |
| Severity          | Medium                   |
| Affected services | Parcel                   |
| Members           | M1/M6/M9                 |
| Status            | Fixed (operator tooling) |

### Root cause

`CleanupPublished` deleted only `PUBLISHED` rows older than 14 days. Rows that
exhausted five publish attempts became `FAILED` and remained forever. There was
no operator command to inspect, replay or expire them.

### Resolution

Added `apps/services/parcel/outbox-tool` and three Make targets:

```sh
make outbox-failed
make outbox-replay OUTBOX_ARGS="-older-than 720h"
make outbox-expire OUTBOX_ARGS="-older-than 720h"
```

- `FailedSummary` is read-only and reports count + oldest.
- `ReplayFailed` returns FAILED rows to PENDING, resets attempts and clears the
  claim, scoped by cutoff and limit.
- `DeleteFailed` deletes only FAILED rows older than an explicit cutoff.
- Replay/expire are dry runs unless `-apply` is passed; the Make targets pass it.
- The tool never touches PENDING or PUBLISHED rows.

### Regression tests

- `tests/integration/eventflow/outbox_claim_test.go::TestFailedOutboxReplayAndExpiry`
  (integration tag; requires `TEST_DATABASE_URL`).

---

## DEF-007 — Undefined audit-log and webhook-attempt retention

| Field             | Value                            |
| ----------------- | -------------------------------- |
| Severity          | Medium                           |
| Affected services | Identity, Merchant, Pricing      |
| Members           | M1/M8/M9                         |
| Status            | Fixed (policy + operator script) |

### Root cause

Audit logs and webhook delivery attempts grow with every administrative action
and delivery retry. Only notification logs and processed events had a policy.

### Resolution

Defined and documented:

| Data                                 | Window   |
| ------------------------------------ | -------- |
| `identity.audit_logs`                | 365 days |
| `merchant.audit_logs`                | 365 days |
| `pricing.audit_logs`                 | 365 days |
| `merchant.webhook_delivery_attempts` | 90 days  |

Added `scripts/prune-retention.sh`, `make prune-retention` (dry run) and
`make prune-retention-apply`. Windows must be at least 30 days. The script never
touches notification logs, processed events, merchant orders, reservations,
parcel rows or proof objects.

### Verification

- Shell syntax check (`sh -n`) passes.
- The script prints eligible counts before deleting.
- The deletion path is not run in this session because no disposable database was
  available; this is recorded in `FINAL_TEST_REPORT.md`.

---

## DEF-008 — Access matrix assumed logout needed an access token

| Field          | Value        |
| -------------- | ------------ |
| Severity       | Medium       |
| Affected areas | Access tests |
| Members        | M2/M10       |
| Status         | Fixed        |

### Root cause

The access kit assumed every non-public case returns 401 for anonymous callers.
Logout is now intentionally callable without an access token.

### Resolution

- Added `Public bool` to `accesskit.Case`.
- Anonymous and every role must get a handler answer (not 401/403) for a public
  case.
- Marked logout `Public: true`.
- Updated the session test to log out with the refresh token only and to assert
  repeated logout is idempotent.

### Regression tests

- `tests/access/accesskit` self-tests pass.
- The Compose-backed access matrix must be re-run with a live gateway.

---

## DEF-009 — Frontend identity survived a failed logout

| Field          | Value                |
| -------------- | -------------------- |
| Severity       | Low                  |
| Affected areas | Web session provider |
| Members        | M2                   |
| Status         | Fixed                |

### Root cause

`signOut()` called `emitSessionIdentity(null)` only after `logout()` resolved.
A network failure skipped it, leaving live screens and the local session event
bus believing the old identity was still current.

### Resolution

`signOut()` now clears the emitted identity in a `finally` block. The caller
already catches the error and navigates to `/signed-out`.

---

## DEF-010 — Merchant orphan-parcel recovery was untested

| Field             | Value                             |
| ----------------- | --------------------------------- |
| Severity          | Low                               |
| Affected services | Merchant, Parcel                  |
| Members           | M8                                |
| Status            | Verified; no code change required |

### Analysis

The existing orchestration already handles this path:

1. Merchant serializes per `(merchantId, idempotencyKey)` with an advisory lock.
2. It writes a durable reservation with the normalized request hash before
   calling Parcel.
3. Parcel replays by `(merchantId, externalOrderId)` and returns the existing
   parcel if the first attempt created one.
4. If Merchant persistence fails after Parcel returns, the reservation is marked
   `FAILED`; a retry calls Parcel again, receives the same parcel, and persists
   the order.

The missing piece was a regression test for step 4.

### Resolution

Added
`TestCreateRecoversAfterMerchantPersistenceFailsFollowingParcelCreation`:

- first call: Parcel succeeds, Merchant `CreateOrder` fails;
- retry with the same key: Parcel is called again (idempotent replay) and the
  order is persisted once;
- exactly one merchant order exists.

Concurrent duplicate, same-key/different-payload, many-payload and API-key-limit
tests already existed and remain in place.

---

## DEF-011 — Accidental duplicate files

| Field          | Value         |
| -------------- | ------------- |
| Severity       | Informational |
| Affected areas | Working tree  |
| Status         | Fixed         |

Two identical macOS-style copies were untracked:

- `scripts/rehearse-restore 2.sh`
- `tests/integration/database/migration_cycle_test 2.go`

They were byte-identical to the canonical files and were removed. They were
never committed.

---

## DEF-012 — API 8.17/8.18 documentation drift

| Field             | Value                         |
| ----------------- | ----------------------------- |
| Severity          | Low                           |
| Affected services | Merchant                      |
| Members           | M8                            |
| Status            | Open (documentation decision) |

API v1.1 section 15 describes `GET /merchant/orders` and
`GET /merchant/orders/{id}` as `MERCHANT (JWT)`. The implementation also accepts
a valid merchant API key on those reads. This is additive and not a security
weakening (the key already acts for the approved merchant owner), but the
contract and code should agree. Recommendation: document the API-key read
capability or restrict the read routes to sessions if the stricter contract is
intended.

---

## DEF-013 — Audit data has no read surface

| Field             | Value                       |
| ----------------- | --------------------------- |
| Severity          | Low                         |
| Affected services | Identity, Merchant, Pricing |
| Members           | M3/M8                       |
| Status            | Open (scope)                |

Audit rows are written by identity user changes, merchant lifecycle actions and
pricing configuration. There is no ADMIN endpoint or screen to read them, so
operators must query the database directly. This is an observability gap, not a
correctness defect.

Recommendation: add an ADMIN audit-log read API with pagination and actor/action
filters, then a read-only screen.

---

## DEF-014 — Required CI gate accepted skipped path-filtered suites

| Field          | Value  |
| -------------- | ------ |
| Severity       | Medium |
| Affected areas | CI     |
| Members        | M10    |
| Status         | Fixed  |

### Root cause

`Required CI gate` treated `access`, `notificationflow` and `real-stack-e2e` as
optional (`success` or `skipped`). A change that should have exercised one of
those suites could still pass the gate if the job was skipped.

### Resolution

The gate now reads the `changes` job outputs:

- when `access` is in scope, the access job is **required**;
- when `notificationflow` is in scope, that job is **required**;
- when `realstack` is in scope, the real-stack job is **required**.

Unrelated changes may still skip them. A failure or an unexpected skip in scope
fails the gate.

### Verification

The workflow YAML was edited and inspected. GitHub Actions cannot be executed in
this environment; the next PR run is the verification.

## DEF-015 — Seeded-role Playwright logins were rate-limit fragile

| Field          | Value     |
| -------------- | --------- |
| Severity       | Low       |
| Affected areas | E2E tests |
| Members        | M10       |
| Status         | Fixed     |

`seeded-roles.real.spec.ts` performed manual form logins for five roles in one
worker after other real-stack journeys had already used the shared 10/min login
limit. Three role cases stayed on `/login`; a retry within seconds could not
recover. The product rate limit is correct.

The spec now uses `signInThroughForm`, which detects the rate-limit message and
waits for the window to reset. The rerun passed 6/6 (one CI-only test skipped
locally).

## Residual risks (not defects)

| Risk                           | Mitigation / status                                       |
| ------------------------------ | --------------------------------------------------------- |
| Process-local rate limits      | Documented; shared limiting is deferred                   |
| In-process realtime hub        | Single replica required; fan-out is future work           |
| In-process AI state            | Single replica required; durable state is future work     |
| CSRF token absent              | Origin guard + SameSite + JSON; documented residual       |
| CSP `unsafe-inline`            | Required by Next hydration/inline styles                  |
| Shared `INTERNAL_TOKEN`        | Trust boundary documented; least privilege is future work |
| Webhook at-least-once          | Merchant receivers must dedupe on `eventId`               |
| Proof storage single volume    | S3-compatible/Neon storage for multi-replica              |
| No TLS/Kubernetes/Terraform    | Production deployment is outside this repository          |
| Production DB behind repo head | Explicit migration window required                        |
| Live Google/Gemini not in CI   | Manual verification procedure only                        |

## Verification commands used for this register

```sh
go test ./apps/services/identity/...
go test ./apps/services/merchant/...
go test ./apps/services/parcel/...
go test -race ./apps/services/identity/... ./apps/services/merchant/internal/orders/... ./apps/services/parcel/internal/outbox/...
go vet -tags=integration ./apps/services/identity/... ./tests/integration/eventflow/...
node --test "tests/contracts/*.test.mjs"
pnpm exec nx test @org/web
pnpm exec nx lint @org/web
pnpm exec nx build @org/web
```

Integration tests were executed against real PostgreSQL 16 and RabbitMQ 4.1 in
the Compose network: the identity repository suite, the event-flow suite
(including outbox replay/expiry), and the merchant order/API-key suite all
passed. These checks were later executed in the post-merge verification pass:
the full migration-cycle, dedicated notification-flow (18/18), k6,
backup/restore rehearsal, S3-compatible storage and live Gemini. Only live
Google OAuth automated tests pass, the provider served the real sign-in page,
and the project owner completed manual consent and account linking. See
`FINAL_TEST_REPORT.md` section 11.

---

## Non-deployment finalization defects (2026-11-20)

| ID      | Severity | Issue                                                     | Resolution                                                                                                 |
| ------- | -------- | --------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------- |
| DEF-016 | Medium   | ADMIN audit rows had no read API/UI                       | Added Identity/Merchant/Pricing audit endpoints, redaction, `/admin/audit-logs`, mocked + real-stack tests |
| DEF-017 | Medium   | Retention apply used one unbounded delete per table       | Bounded batches with `BATCH_SIZE`/`MAX_BATCHES`, resumable and idempotent                                  |
| DEF-018 | Low      | S3 integration helper failed outside `us-east-1`          | CreateBucket now sends the configured LocationConstraint; S3 suite passes on `us-east-2`                   |
| DEF-019 | Low      | Audit routes could drift from Nginx/OpenAPI/access matrix | Route inventories, OpenAPI, Nginx and 66-route matrix updated together; contract tests pass                |

No critical or high-severity application defect remains open in the
non-deployment scope.
