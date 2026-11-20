# SwiftDrop — Member-wise Status (M1–M10)

This file maps the agreed member responsibilities to the current implementation.
Status terms:

- **COMPLETE** — implemented and covered by automated tests or a directly
  inspected, deterministic code path.
- **PARTIAL** — implemented but one acceptance criterion is not verified in this
  environment.
- **BLOCKED** — cannot be verified here because an external dependency
  (Compose/PostgreSQL/RabbitMQ/browser/provider credentials) is unavailable.
- **OUT OF SCOPE** — explicitly excluded from the agreed scope.

## Summary

| Member | Area                          | Status                                                       | Main evidence                                                                               |
| ------ | ----------------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------------------- |
| M1     | Notifications and platform    | COMPLETE                                                     | `notification-worker`, `packages/go-common/emails`, `mailer`, internal contacts, demo seeds |
| M2     | Authentication and frontend   | COMPLETE                                                     | Identity service, auth screens, session gate, role nav, API client                          |
| M3     | OAuth, reset and admin        | COMPLETE (live provider manual)                              | Google PKCE/state/binding, reset flow, admin user/merchant screens                          |
| M4     | Pricing and tracking          | COMPLETE                                                     | Pricing service, exact decimals, public/auth tracking, live tracking hook                   |
| M5     | Parcel operations             | COMPLETE                                                     | Parcel service, atomic create, idempotency, receipts, internal merchant create              |
| M6     | Dispatch                      | COMPLETE                                                     | Dispatch service/repository, eligibility, assignment transactions, outbox                   |
| M7     | Courier and proofs            | COMPLETE                                                     | Courier state machine, pickup/delivery transactions, proof storage/auth                     |
| M8     | Merchant                      | COMPLETE                                                     | Onboarding, API keys, orders, sync, webhook UI, internal APIs                               |
| M9     | Webhooks                      | COMPLETE                                                     | Signed delivery, retries, attempts, suppression, dead letters                               |
| M10    | Realtime, reports and testing | COMPLETE for one replica; integration execution BLOCKED here | WebSocket, consumers, reports, Playwright/CI suites                                         |

---

## M1 — Notifications and Platform

| Item                                     | Status   | Evidence                                                                                      |
| ---------------------------------------- | -------- | --------------------------------------------------------------------------------------------- |
| Merchant recipient contact resolution    | COMPLETE | `GET /internal/merchants/{merchantId}/contact`; `notification-worker/internal/merchantclient` |
| Internal identity contact API            | COMPLETE | `GET /internal/users/{userId}/contact`; `identity/internal/http/internal.go`                  |
| Customer notification processing         | COMPLETE | `notification-worker/internal/worker`; `parcel.created.v1`, `parcel.status_changed.v1`        |
| Merchant notification processing         | COMPLETE | `handleParcelStatusChanged`; approved merchant contact resolution                             |
| Notification logs                        | COMPLETE | `notification.notification_logs`; ADMIN read API on 8086                                      |
| Processed-event idempotency              | COMPLETE | `notification.processed_events`; 30-day retention                                             |
| Retention and cleanup                    | COMPLETE | 180-day notification logs; 30-day processed events; audit/webhook policy added in this pass   |
| Deterministic demo seeds                 | COMPLETE | `make seed`, `make seed-merchant`, `make seed-parcels`, `make seed-demo`                      |
| Shared error handling and redacting logs | COMPLETE | `httpx.Error`, `logging.RedactText`, path-only gateway logs                                   |
| Operational documentation                | COMPLETE | `docs/operations.md`, `docs/email.md`, `docs/data-retention.md`                               |

**Remaining:** Email-only channel; SMS is a documented permanent failure and is
outside API v1.1. No in-app notification inbox.

**Acceptance criteria:** A customer and an approved merchant each receive one
email for the applicable parcel event; a duplicate event does not send twice;
a failed delivery is recorded. The event-flow real-stack test passed with a
real merchant email through Mailpit, and the access/notification log checks
passed.

---

## M2 — Authentication and Frontend

| Item                                           | Status               | Evidence                                                                                          |
| ---------------------------------------------- | -------------------- | ------------------------------------------------------------------------------------------------- |
| Customer registration                          | COMPLETE             | `POST /auth/register`; validation, bcrypt, verification token                                     |
| Email verification                             | COMPLETE             | `POST /auth/verify-email`, `resend-verification`; single-use hashed token                         |
| Login                                          | COMPLETE             | Lockout, bcrypt, merchant standing, email-verification gate                                       |
| Refresh                                        | COMPLETE             | Rotation, reuse detection, fixed limiter added                                                    |
| Logout                                         | **COMPLETE (fixed)** | Refresh-token credential; no access token required; revokes successor; clears cookies; idempotent |
| Profile editing                                | COMPLETE             | `GET/PATCH /users/me`; full name/phone only                                                       |
| Password change                                | COMPLETE             | Current-password check, policy, revokes refresh tokens                                            |
| Session invalidation                           | COMPLETE             | `sv` claim + `sessioncheck`; 30s cache; fail closed                                               |
| Role-based route protection                    | COMPLETE             | `RequireRole`, nav, backend role middleware                                                       |
| Consistent API errors/loading states           | COMPLETE             | `ApiError`, `useLoad`, screen states                                                              |
| Frontend session identity after logout failure | **COMPLETE (fixed)** | `session.tsx` clears identity in `finally`                                                        |

**Remaining:** No session/device list, no email change, no account deletion.

**Acceptance criteria:** A customer can register, verify, log in, refresh, edit
the profile, change the password and log out — including after the access token
expires. All roles are refused on screens and APIs they do not own.

**Verification:** Identity unit/handler and repository integration tests pass;
`TestAuthHandlers_logout` covers the expired-token case; the 63-endpoint access
matrix and the logout session test passed against the Compose gateway.

---

## M3 — OAuth, Reset and Admin

| Item                                       | Status   | Evidence                                                            |
| ------------------------------------------ | -------- | ------------------------------------------------------------------- |
| Google OAuth PKCE                          | COMPLETE | `internal/google`, `service/google.go`                              |
| Browser-bound state                        | COMPLETE | `sd_oauth_binding_<state prefix>` cookie, single-use state          |
| Verified-email linking                     | COMPLETE | `linkExisting`, password cleared and sessions revoked when needed   |
| Password reset email/browser flow          | COMPLETE | `forgot-password`, `reset-password`, Mailpit real-stack spec        |
| Reset token hashing/single use             | COMPLETE | `password_reset_tokens`; `used_at`, expiry, revocation              |
| Reset token log safety                     | COMPLETE | Nginx `swiftdrop_no_query` on `/api/v1/auth/`; app logs omit tokens |
| Admin user management                      | COMPLETE | list/create/get/update/status/password-reset; audit rows            |
| Merchant approval UI                       | COMPLETE | `/admin/merchants`; backend transitions                             |
| Backend authorization for admin operations | COMPLETE | `auth.RequireRoles(ADMIN)`, `requireCurrentAccess`, self-protection |

**Remaining / dependency:** Live Google verification requires real Google
credentials and is not a CI gate. Reset UI is covered by
`auth-reset.real.spec.ts` when Compose is running.

**Acceptance criteria:** Reset works from email to browser; a used/expired token
fails; an ADMIN cannot deactivate or demote themselves into a lockout; every
admin route refuses non-ADMIN callers.

---

## M4 — Pricing and Tracking

| Item                                                | Status   | Evidence                                                             |
| --------------------------------------------------- | -------- | -------------------------------------------------------------------- |
| Exact decimal calculations                          | COMPLETE | `pricing.Decimal`, `parcels.Decimal`; no float64                     |
| Pricing boundaries/invalid inputs                   | COMPLETE | Unit/handler tests for routes, bands, gaps, weights, size            |
| Existing parcel prices unchanged after rule updates | COMPLETE | Parcel stores `price_amount` + `price_breakdown` snapshot            |
| Public tracking privacy                             | COMPLETE | Allowlist projection: tracking number, status, cities, timeline only |
| Realtime hook in public tracking                    | COMPLETE | `tracking-view.tsx`, `useLiveUpdates`                                |
| Reconnect + REST refetch                            | COMPLETE | `live-connection.ts`, `onSync` after reconnect                       |
| Authenticated tracking ownership                    | COMPLETE | `LookupAuthorized`; staff operational access                         |

**Remaining:** No live provider dependency. Tracking is not multi-replica
fan-out (M10).

**Acceptance criteria:** A quote is exact and stable; an old parcel keeps its
price; public tracking never exposes names, phones, addresses, prices or IDs;
a live update arrives and a reconnect refetches authoritative state.

---

## M5 — Parcel Operations

| Item                            | Status               | Evidence                                                             |
| ------------------------------- | -------------------- | -------------------------------------------------------------------- |
| Parcel creation                 | COMPLETE             | `parcels.Service.CreateCustomer`; validation, tracking number        |
| Idempotency                     | COMPLETE             | advisory lock, replay by customer+key, conflict on different payload |
| Pricing integration             | COMPLETE             | `PricingClient`; verifies LKR/components/expected fee                |
| Atomic initial history + outbox | COMPLETE             | `PostgresStore.Create`; one transaction                              |
| Customer ownership checks       | COMPLETE             | `authorized`, `ParcelByViewer`; cross-customer 404                   |
| Receipts                        | COMPLETE             | `parcels.Receipt`; Unicode/PDF tests                                 |
| Merchant internal creation      | COMPLETE             | `POST /internal/parcels`; trusted caller boundary documented         |
| Database failure/concurrency    | PARTIAL here         | Unit + existing integration tests; not re-run without PostgreSQL     |
| Creation abuse boundary         | **COMPLETE (fixed)** | 30/min per verified customer                                         |

**Remaining:** Parcel edit/cancel is not in API v1.1 and is out of scope.

**Acceptance criteria:** A create either commits parcel + history + outbox
together or commits nothing; a retry with the same key/payload returns the
original; a different payload conflicts; a customer cannot read another
customer's parcel.

---

## M6 — Dispatch

| Item                                | Status   | Evidence                                                                                 |
| ----------------------------------- | -------- | ---------------------------------------------------------------------------------------- |
| Assignment/reassignment transaction | COMPLETE | `dispatch.Repository.AssignCourier`; parcel lock, close previous, insert, update, outbox |
| One active assignment per parcel    | COMPLETE | `is_current` update + service/repository tests                                           |
| Courier eligibility                 | COMPLETE | Identity internal check: active, available, COURIER                                      |
| Terminal parcels rejected           | COMPLETE | `statemachine.IsTerminal` → `PARCEL_CLOSED`                                              |
| Outbox reliability                  | COMPLETE | PENDING `parcel.assigned`; relay claim/publish                                           |
| Dispatcher realtime updates         | COMPLETE | `/parcels` and `/unassigned` live hooks; `parcel.assigned.updated`                       |

**Remaining:** No unassign endpoint; reassignment is the supported operation.

**Acceptance criteria:** Assigning a parcel creates exactly one active
assignment and one outbox event; ineligible/terminal targets are refused;
reassignment preserves history and notifies dispatch clients.

---

## M7 — Courier and Proofs

| Item                                | Status   | Evidence                                                                    |
| ----------------------------------- | -------- | --------------------------------------------------------------------------- |
| Assignment ownership                | COMPLETE | `statemachine.VerifyCourierOwnership`, current assignment check             |
| Valid status transitions            | COMPLETE | `statemachine` matrix; generic endpoint blocks proof-backed transitions     |
| Mandatory recipient for DELIVERED   | COMPLETE | `ValidateTransition`; delivery endpoint                                     |
| Mandatory failure reason for FAILED | COMPLETE | 5–255 characters                                                            |
| Pickup/delivery proof uploads       | COMPLETE | transactional workflow; mandatory photo                                     |
| File content/size validation        | COMPLETE | JPEG/PNG sniff + decode + 5 MiB + dimension/pixel caps                      |
| Private access                      | COMPLETE | ownership checks; presigned URL or authorized stream                        |
| Proof retrieval/storage consistency | COMPLETE | transaction after upload; delete on DB failure; `proof-transfer -reconcile` |

**Remaining:** No proof retention/archival policy; local storage needs one
replica or a shared volume.

**Acceptance criteria:** Only the assigned courier can transition; pickup and
delivery each commit proof + history + outbox atomically; an unassigned courier
gets 403; a customer sees only their own proof.

---

## M8 — Merchant

| Item                               | Status                  | Evidence                                                                    |
| ---------------------------------- | ----------------------- | --------------------------------------------------------------------------- |
| Onboarding and approval            | COMPLETE                | `onboarding`; inactive identity owner + PENDING merchant; transitions       |
| API key generation/hash/revocation | COMPLETE                | `apikey`, `merchant_api_clients`; hash-only, lookup ID                      |
| Active key limit                   | COMPLETE                | DB trigger + service error; concurrency test                                |
| Merchant order idempotency         | COMPLETE                | reservation, advisory lock, hash, Parcel replay                             |
| Orphan-parcel recovery             | **COMPLETE (verified)** | New regression test: retry after merchant persistence failure reuses Parcel |
| Webhook management frontend        | COMPLETE                | `/webhook`; save/rotate/test/deliveries                                     |
| Order status synchronization       | COMPLETE                | `merchant.sync.queue`; newer-only, `event_id` dedupe                        |

**Remaining:** API 8.17/8.18 accept an API key as well as a session while the
documentation says JWT; this is additive but should be reconciled.

**Acceptance criteria:** Same key/payload replays; same key/different payload
conflicts; concurrent requests create one parcel; a failed merchant write can be
retried without a duplicate parcel; a status event updates the order once.

---

## M9 — Webhooks

| Item                              | Status               | Evidence                                                     |
| --------------------------------- | -------------------- | ------------------------------------------------------------ |
| HMAC-SHA256 signing               | COMPLETE             | `webhookdelivery.Sign`; raw body + timestamp                 |
| Timestamp/replay validation       | COMPLETE             | 5-minute window, canonical timestamp, constant-time compare  |
| Stable event IDs                  | COMPLETE             | Envelope event ID reused on every attempt                    |
| HTTPS-only                        | COMPLETE             | DB constraint + `webhookurl.Validate`                        |
| Private/local/link-local blocking | COMPLETE             | IP prefix checks, host checks, DNS resolution                |
| DNS/redirect protection           | COMPLETE             | `ValidateResolved`, `DialControl`, no redirects, no proxy    |
| Retry/dead-letter                 | COMPLETE             | 1m/5m/30m/2h/6h; delay queues; max 6                         |
| Delivery attempt recording        | COMPLETE             | idempotent `webhook_delivery_attempts`                       |
| Merchant suspension policy        | COMPLETE             | suppress + dead-lettered attempt; no replay on reinstatement |
| Test-send abuse boundary          | **COMPLETE (fixed)** | 10/min per merchant                                          |

**Remaining:** A real merchant endpoint must dedupe on `eventId`; this is
documented, not enforceable by SwiftDrop.

**Acceptance criteria:** A signed event verifies within the window; a stale or
tampered body fails; a suspended merchant is suppressed; attempts are recorded;
retry exhaustion dead-letters.

---

## M10 — Realtime, Reports and Testing

| Item                                      | Status           | Evidence                                                                                  |
| ----------------------------------------- | ---------------- | ----------------------------------------------------------------------------------------- |
| Realtime hooks in tracking/dispatch       | COMPLETE         | `useLiveUpdates` consumers                                                                |
| Reconnect with backoff                    | COMPLETE         | `live-connection.ts`; state machine                                                       |
| REST refetch after reconnect              | COMPLETE         | `onSync` callback                                                                         |
| WebSocket auth/subscription authorization | COMPLETE         | cookie/`?token=`, session revalidation, `scope: all` role gate                            |
| Reports integration                       | COMPLETE         | `/reports`; daily/outcomes; empty and non-empty states                                    |
| Full-stack integration testing            | **BLOCKED here** | Suites exist; Compose/PostgreSQL/RabbitMQ/browser not started in this session             |
| Consolidated coverage evidence            | PARTIAL          | Go/contract/frontend evidence recorded; integration/E2E evidence pending a full-stack run |

**Remaining:** Multi-replica fan-out, replay, SSE, tracing, shared rate limits.

**Acceptance criteria:** A dispatcher sees live status/assignment changes; an
anonymous visitor can follow one parcel but not `scope: all`; a disconnected
client reconnects and refetches; reports reflect real parcel rows. The
`event-flow.real.spec.ts` and `customer-delivery-journey.real.spec.ts` runs
verified the live socket and the authoritative REST state after completion.

---

## Cross-member dependencies

| Dependency                        | Owner     | Impact                                                                       |
| --------------------------------- | --------- | ---------------------------------------------------------------------------- |
| Identity session state            | M2        | Parcel, pricing, realtime, notification admin API fail closed if unavailable |
| Merchant approval state           | M3/M8     | Merchant login and business routes                                           |
| Pricing quote                     | M4        | Parcel creation                                                              |
| Parcel internal create/summary    | M5        | Merchant orders and webhook worker                                           |
| Merchant internal contact/webhook | M8        | Notification and webhook workers                                             |
| RabbitMQ + PostgreSQL             | M10/infra | All asynchronous and persistence evidence                                    |
| Compose stack + seeds             | M10       | Demo and Playwright/access evidence                                          |

---

## Non-deployment finalization updates (2026-11-20)

| Member | Added/final state                                                           |
| ------ | --------------------------------------------------------------------------- |
| M1     | Bounded-batch audit/webhook retention; failed-outbox concurrent replay test |
| M2     | Logout/session fix remains verified by the 66-route matrix                  |
| M3     | Identity ADMIN audit-log API and page; audit redaction                      |
| M4     | Pricing ADMIN audit-log API                                                 |
| M5     | Outbox replay/expiry verification; no new migration                         |
| M6     | Assignment/outbox evidence unchanged and green                              |
| M7     | Proof lifecycle policy documented; storage reconciliation verified          |
| M8     | Merchant ADMIN audit-log API; webhook reliability still green               |
| M9     | Webhook reliability real-stack suite passed (retry, exhaustion, rotation)   |
| M10    | Audit UI tests; k6, migration cycle, backup/restore, S3, Gemini smoke       |

**Member conclusion:** all M1–M10 mandatory non-deployment responsibilities have
implementation and executed evidence. The only external-provider gap is live
Google consent, which is recorded as blocked/manual.

### Post-merge verification pass (2026-11-20)

Baseline: `development` at `831455b`. Branch: `docs/final-remediation-verification`.

| Member | Verified state on the merged baseline                                                                                                                        |
| ------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| M1     | Dedicated notification flow re-run locally: 18/18 tests pass (customer and merchant email, dedupe, dead-lettering, retry recovery); retention dry run passes |
| M2     | Session/logout and authorization evidence unchanged; E2E lint/typecheck pass                                                                                 |
| M3     | Google OAuth initiation reachability confirmed; account consent remains blocked/manual                                                                       |
| M4     | Pricing audit read API live smoke returns 200 for ADMIN and 403 for CUSTOMER                                                                                 |
| M5     | Full Go build/vet/test pass; no regression in parcel/outbox code                                                                                             |
| M6     | Route inventories and contracts pass                                                                                                                         |
| M7     | Proof and object-storage tests pass; lifecycle documentation confirmed                                                                                       |
| M8     | Merchant audit read API live smoke passes                                                                                                                    |
| M9     | Webhook reliability evidence unchanged (post-merge CI green)                                                                                                 |
| M10    | Notification-flow, contract, frontend and Compose checks pass; documentation reconciled to `831455b`                                                         |

**Member conclusion unchanged:** all M1–M10 mandatory non-deployment
responsibilities have implementation and executed evidence. Live Google account
consent remains the only blocked external-provider check.
