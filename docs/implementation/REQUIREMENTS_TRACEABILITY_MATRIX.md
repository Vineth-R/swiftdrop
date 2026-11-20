# SwiftDrop — Requirements Traceability Matrix

**Final baseline:** `development` at
`682205aa2856fd8f03721979d88a638e1313cd6a` (the PR #181 merge; contains PR #180
and PR #179). Product verification was performed on `831455b`; PR #181 added only
tests and documentation.
**Basis:** `AGENTS.md`, API Documentation v1.1, Database Design v1.1, architecture, member task allocation and the merged acceptance reports.

Status terms:

- **COMPLETE** — implemented and independently verified in the repository or the live local stack.
- **PARTIAL** — implemented, but one acceptance criterion is not fully verified here.
- **NOT IMPLEMENTED** — required functionality is absent.
- **BLOCKED** — cannot be verified because an external dependency/credential is unavailable.
- **OUT OF SCOPE** — explicitly excluded from this phase.

## Identity and access

| ID    | Requirement                             | Component                     | Implementation                                                | Test evidence                                                                                                         | Remaining gap | Action | Member | Result   |
| ----- | --------------------------------------- | ----------------------------- | ------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------- | ------------- | ------ | ------ | -------- |
| ID-01 | Customer registration                   | Identity                      | `service.Register`, `POST /auth/register`                     | Unit/handler tests; real-stack registration                                                                           | None          | —      | M2     | COMPLETE |
| ID-02 | Email verification                      | Identity                      | Token issue/consume, `verify-email`, `resend-verification`    | Unit/handler + real-stack Mailpit                                                                                     | None          | —      | M2     | COMPLETE |
| ID-03 | Login/lockout                           | Identity                      | `service.Login`, lockout policy                               | Unit; access matrix lockout test                                                                                      | None          | —      | M2     | COMPLETE |
| ID-04 | Refresh rotation/reuse detection        | Identity                      | `RotateRefreshToken`                                          | Unit + repository integration                                                                                         | None          | —      | M2     | COMPLETE |
| ID-05 | Logout (including expired access token) | Identity/web                  | `RevokeRefreshSession`, refresh-token logout                  | Unit + repository integration + access logout test                                                                    | None          | —      | M2     | COMPLETE |
| ID-06 | Google OAuth PKCE/state/browser binding | Identity                      | `service/google.go`, `internal/google`                        | 32 automated tests plus manual consent/account-linking completed by the project owner; provider sign-in page verified | None          | —      | M3     | COMPLETE |
| ID-07 | Password reset                          | Identity                      | Hashed single-use reset token, revoke sessions                | Unit/handler + real-stack Mailpit reset                                                                               | None          | —      | M3     | COMPLETE |
| ID-08 | Profile/password change                 | Identity                      | `GET/PATCH /users/me`, `PUT /users/me/password`               | Unit/handler + access matrix                                                                                          | None          | —      | M2     | COMPLETE |
| ID-09 | Admin user management                   | Identity                      | List/create/get/update/status/reset + audit                   | Unit + integration + access matrix                                                                                    | None          | —      | M3     | COMPLETE |
| ID-10 | Courier list                            | Identity                      | `GET /couriers`                                               | Unit + access matrix                                                                                                  | None          | —      | M3     | COMPLETE |
| ID-11 | Internal identity lookups               | Identity                      | Contact, eligibility, session state, merchant owner lifecycle | Unit/integration                                                                                                      | None          | —      | M1/M3  | COMPLETE |
| ID-12 | ADMIN audit visibility                  | Identity/Merchant/Pricing/Web | `GET /admin/audit-logs/{source}` + `/admin/audit-logs` screen | Handler tests, repository integration, mocked + real-stack UI, 66-route access matrix                                 | None          | —      | M3/M8  | COMPLETE |

## Pricing

| ID    | Requirement                         | Component      | Implementation                                 | Test evidence                            | Remaining gap | Action | Member | Result   |
| ----- | ----------------------------------- | -------------- | ---------------------------------------------- | ---------------------------------------- | ------------- | ------ | ------ | -------- |
| PR-01 | Public estimate                     | Pricing        | `POST /pricing/estimate`                       | Unit/handler/HTTP integration + k6       | None          | —      | M4     | COMPLETE |
| PR-02 | Zones                               | Pricing        | `GET /zones`, internal zone read               | Unit/handler + k6                        | None          | —      | M4     | COMPLETE |
| PR-03 | Exact decimals                      | Pricing/Parcel | `Decimal` hundredths, no float64               | Unit tests                               | None          | —      | M4     | COMPLETE |
| PR-04 | Band/route/surcharge validation     | Pricing        | Repository quote + admin validation            | Unit/handler                             | None          | —      | M4     | COMPLETE |
| PR-05 | ADMIN pricing configuration + audit | Pricing        | Admin rules/bands/zones/surcharges, audit rows | Handler/HTTP integration + access matrix | None          | —      | M4     | COMPLETE |

## Parcel, tracking and dispatch

| ID    | Requirement                | Component    | Implementation                                      | Test evidence                          | Remaining gap         | Action                     | Member | Result                   |
| ----- | -------------------------- | ------------ | --------------------------------------------------- | -------------------------------------- | --------------------- | -------------------------- | ------ | ------------------------ |
| PA-01 | Customer parcel creation   | Parcel       | `POST /parcels`, real quote, atomic history/outbox  | Unit/integration + real-stack wizard   | None                  | —                          | M5     | COMPLETE                 |
| PA-02 | Idempotent creation        | Parcel       | Advisory lock, key replay, conflict                 | Unit/integration                       | None                  | —                          | M5     | COMPLETE                 |
| PA-03 | Owned list/detail/history  | Parcel       | `ParcelByViewer`, filters, pagination               | Integration + access ownership         | None                  | —                          | M5     | COMPLETE                 |
| PA-04 | PDF receipt                | Parcel       | `parcels.Receipt`                                   | Unit/PDF tests + real-stack download   | None                  | —                          | M5     | COMPLETE                 |
| PA-05 | Public tracking            | Parcel       | Allowlisted public projection                       | Unit/integration + real-stack `/track` | None                  | —                          | M4     | COMPLETE                 |
| PA-06 | Authenticated tracking     | Parcel       | Ownership/staff-scoped lookup                       | Unit/integration + access matrix       | None                  | —                          | M4     | COMPLETE                 |
| PA-07 | Dispatch queue/assignment  | Parcel       | Eligibility, transaction, assignment history/outbox | Integration + real-stack assignment    | None                  | —                          | M6     | COMPLETE                 |
| PA-08 | Reports                    | Parcel       | Daily/outcomes                                      | Unit/integration + real-stack reports  | None                  | —                          | M4/M10 | COMPLETE                 |
| PA-09 | Realtime tracking/dispatch | Realtime/Web | `/ws`, consumers, hooks, reconnect/refetch          | Unit + real-stack live tests           | Multi-replica fan-out | Document as single-replica | M10    | PARTIAL (single replica) |

## Courier and proof

| ID    | Requirement                   | Component          | Implementation                         | Test evidence                 | Remaining gap                    | Action             | Member | Result             |
| ----- | ----------------------------- | ------------------ | -------------------------------------- | ----------------------------- | -------------------------------- | ------------------ | ------ | ------------------ |
| CO-01 | Courier list/detail           | Parcel             | Assignment-scoped reads                | Integration + real-stack      | None                             | —                  | M7     | COMPLETE           |
| CO-02 | Status state machine          | Parcel             | `statemachine` + transactional updates | Unit/integration/access       | None                             | —                  | M7     | COMPLETE           |
| CO-03 | Pickup proof workflow         | Parcel             | Proof + PICKED_UP/IN_TRANSIT + outbox  | Integration + real-stack      | None                             | —                  | M7     | COMPLETE           |
| CO-04 | Delivery proof workflow       | Parcel             | Proof + recipient + DELIVERED + outbox | Integration + real-stack      | None                             | —                  | M7     | COMPLETE           |
| CO-05 | Failure reason                | Parcel             | FAILED transition validation           | Unit/integration + real-stack | None                             | —                  | M7     | COMPLETE           |
| CO-06 | Private proof storage/access  | Parcel/objectstore | Local/S3, ownership, presign/stream    | Unit/integration + S3 suite   | Retention policy only documented | —                  | M7     | COMPLETE           |
| CO-07 | Orphan/missing reconciliation | Parcel             | `proof-transfer -reconcile`            | Tool + storage tests          | Scheduled job absent             | Operator procedure | M7     | PARTIAL (operator) |

## Merchant and webhooks

| ID    | Requirement                     | Component          | Implementation                                         | Test evidence                                   | Remaining gap | Action | Member | Result   |
| ----- | ------------------------------- | ------------------ | ------------------------------------------------------ | ----------------------------------------------- | ------------- | ------ | ------ | -------- |
| ME-01 | Registration/approval lifecycle | Merchant/Identity  | Inactive owner + PENDING merchant + transitions        | Integration + real-stack onboarding             | None          | —      | M8     | COMPLETE |
| ME-02 | Profile/API keys                | Merchant           | Owner-scoped profile; hash-only keys; 5-key limit      | Unit/integration + real-stack                   | None          | —      | M8     | COMPLETE |
| ME-03 | Merchant orders/idempotency     | Merchant/Parcel    | Reservation, advisory lock, hash, Parcel replay        | Unit/HTTP integration + real-stack              | None          | —      | M8     | COMPLETE |
| ME-04 | Order status synchronization    | Merchant           | `merchant.sync.queue`, newer-only, event dedupe        | Integration + real-stack event flow             | None          | —      | M8     | COMPLETE |
| ME-05 | Webhook settings/rotation/test  | Merchant           | Encrypted keyring, AAD, one-time secret                | Unit/integration + real-stack webhooks          | None          | —      | M8/M9  | COMPLETE |
| ME-06 | Webhook signed delivery/retries | Webhook worker     | HMAC, six attempts, delay queues, suppression          | Unit + webhook reliability real-stack (3 tests) | None          | —      | M9     | COMPLETE |
| ME-07 | Webhook SSRF/HTTPS/replay       | Webhook URL/sender | HTTPS, DNS/dial checks, no redirects, timestamp window | Unit tests                                      | None          | —      | M9     | COMPLETE |
| ME-08 | ADMIN merchant actions/email    | Merchant           | Approve/reject/suspend/email + audit                   | Integration + real-stack admin                  | None          | —      | M8     | COMPLETE |

## Notifications, events and AI

| ID    | Requirement                      | Component           | Implementation                                               | Test evidence                                                                  | Remaining gap                        | Action                  | Member | Result                    |
| ----- | -------------------------------- | ------------------- | ------------------------------------------------------------ | ------------------------------------------------------------------------------ | ------------------------------------ | ----------------------- | ------ | ------------------------- |
| EV-01 | Transactional outbox             | Parcel              | Atomic outbox, claim/publish/confirm                         | Integration + event flow                                                       | None                                 | —                       | M1/M6  | COMPLETE                  |
| EV-02 | Consumer idempotency/retries/DLQ | All workers         | Shared consumer, event IDs, retry/DLQ                        | Unit/integration + event flow                                                  | None                                 | —                       | M1/M10 | COMPLETE                  |
| EV-03 | Failed outbox recovery           | Parcel ops          | `outbox-tool` report/replay/expire                           | Unit build + integration (concurrent replay)                                   | No scheduler; operator               | Make targets            | M1     | COMPLETE (operator)       |
| NO-01 | Customer/merchant email          | Notification worker | Identity/merchant contact resolution, SMTP                   | Unit/integration + event flow + real-stack + dedicated notification flow 18/18 | SMS out of scope                     | —                       | M1     | COMPLETE                  |
| NO-02 | Notification logs/idempotency    | Notification worker | Processed events + masked logs                               | Unit/integration + real-stack + dedicated duplicate/dead-letter tests          | None                                 | —                       | M1     | COMPLETE                  |
| NO-03 | Retention                        | Notification/ops    | 180-day logs, 30-day events; audit 365d/webhook 90d operator | Unit + dry run                                                                 | No automatic audit/webhook scheduler | Documented operator run | M1     | PARTIAL (operator)        |
| AI-01 | Gemini assistant/tools           | AI assistant        | Four read-only tools, role gate, guest sessions              | Unit/orchestration + live Gemini smoke                                         | Live provider manual in CI           | Manual smoke            | M10    | COMPLETE                  |
| AI-02 | Prompt/extraction defense        | AI assistant        | Deterministic guard + backend authorization                  | Unit tests                                                                     | Heuristic only                       | Documented              | M10    | COMPLETE                  |
| AI-03 | Conversation isolation           | AI assistant        | Guest/user ownership checks                                  | Unit tests                                                                     | Process-local state                  | Documented              | M10    | PARTIAL (single instance) |

## Platform, security and operations

| ID    | Requirement                      | Component                     | Implementation                                 | Test evidence                                | Remaining gap                 | Action                | Member | Result              |
| ----- | -------------------------------- | ----------------------------- | ---------------------------------------------- | -------------------------------------------- | ----------------------------- | --------------------- | ------ | ------------------- |
| PL-01 | Gateway/routing/security headers | Nginx                         | Prefix strip, CSP, headers, internal block     | Contract tests + real-stack security headers | CSP `unsafe-inline`           | Documented            | M1     | COMPLETE            |
| PL-02 | CSRF posture                     | Shared/web                    | Origin guard, SameSite, JSON                   | Unit + real-stack                            | No synchronizer token         | Threat-model decision | M2     | COMPLETE (accepted) |
| PL-03 | Internal service auth            | All services                  | Shared token + request ID                      | Unit/integration                             | Broad shared trust            | Documented residual   | M1     | PARTIAL (accepted)  |
| PL-04 | Audit read/redaction             | Identity/Merchant/Pricing/Web | New endpoints/page                             | Unit/integration/real-stack                  | None                          | —                     | M3/M8  | COMPLETE            |
| PL-05 | Migrations/backup/restore        | PostgreSQL/scripts            | versioned migrations, backup/restore/rehearsal | Migration cycle + full rehearsal             | None                          | —                     | M1/M10 | COMPLETE            |
| PL-06 | CI quality/security gates        | GitHub Actions                | Required gate, security, access, real-stack    | All PR checks passed                         | None                          | —                     | M10    | COMPLETE            |
| PL-07 | Production deployment            | infra                         | Out of scope this phase                        | —                                            | TLS/hosting/migration window  | Separate plan         | —      | OUT OF SCOPE        |
| PL-08 | Optional product features        | product                       | Not required by approved scope                 | —                                            | Edit/cancel, SMS, inbox, etc. | Future                | —      | OUT OF SCOPE        |

## Final closure statement

Final `development`: `682205aa2856fd8f03721979d88a638e1313cd6a` (PR #181 merge).
No mandatory non-deployment requirement is NOT IMPLEMENTED or BLOCKED. Rows
marked PARTIAL are accepted, documented limitations (single-instance process
state, operator-driven retention and the shared internal-token trust boundary),
not missing implementation. Google OAuth (ID-06) is COMPLETE with 32 automated
tests and the project owner's manual consent/account-linking verification.

## Post-acceptance merchant enhancement (2026-11-20)

These rows are additive to the accepted baseline. They do not change the
baseline status of any requirement above.

| ID    | Requirement                                          | Component        | Implementation                                                                                    | Test evidence                                                                         | Result   |
| ----- | ---------------------------------------------------- | ---------------- | ------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------- | -------- |
| ME-09 | Preserve `orderId` in API v1.1                       | Merchant/API     | Create/list/detail DTOs and `GET /merchant/orders/{orderId}` unchanged                            | Existing merchant order integration tests plus the new receipt test                   | COMPLETE |
| ME-10 | Do not expose `ownerUserId` in merchant self-service | Merchant/API     | `merchantSelfResponse` omits it; admin DTO keeps it                                               | `TestMerchantSelfResponseOmitsOwnerUserID`                                            | COMPLETE |
| ME-11 | Clickable merchant order rows                        | Web              | Full-row Link on Orders and dashboard recent orders                                               | `merchant-orders.spec.ts`, `dashboards.spec.ts`                                       | COMPLETE |
| ME-12 | Dedicated API integration page                       | Web              | `/merchant/integration`, sidebar entry, Orders link, onboarding panel moved                       | `merchant-orders.spec.ts`, `app-shell.spec.ts`, `layouts.spec.ts`                     | COMPLETE |
| ME-13 | Merchant API guide HTML + PDF                        | Docs/Web         | `docs/merchant-api-guide.html`, generated PDF, `make merchant-api-pdf`, download link             | PDF generated and inspected; Playwright download-link test                            | COMPLETE |
| ME-14 | Branded order receipt PDF                            | Merchant/API/Web | `GET /merchant/orders/{trackingNumber}/receipt`, `internal/receipt`, order-detail download button | Receipt renderer tests, merchant ownership integration test, Playwright download test | COMPLETE |
| ME-15 | Receipt authorization and privacy                    | Merchant/API     | Merchant-scoped repository query, session/API-key auth, `no-store`, 404 for another merchant      | `TestMerchantOrderReceiptPDFAndIsolation`                                             | COMPLETE |

**Post-acceptance conclusion:** the enhancement is implemented and verified
without changing the accepted baseline contract for `orderId` or existing
routes.

| ME-16 | Customer parcel receipt PDF | Parcel/Web | `GET /parcels/{reference}/receipt` accepts UUID or tracking; shared receipt design; timeline and support footer | `TestCustomerReceiptWithHistoryAndSupport`, `TestReceiptDataByTrackingChecksOwnership`, parcel HTTP workflow, `parcels.spec.ts`, real customer journey | COMPLETE |
| ME-17 | Customer receipt ownership and privacy | Parcel/API | Merchant-scoped query by `customer_user_id`; another customer 404; no internal IDs in the PDF; `no-store` | `TestReceiptRowsDoNotLeakInternalIdentifiers`, `TestReceiptDataByTrackingChecksOwnership`, access matrix | COMPLETE |
| ME-18 | Shared receipt design system | go-common/Parcel/Merchant | `receiptdesign` palette, geometry, status/date/money/contact formatting used by both receipts | `receiptdesign` tests, customer and merchant receipt tests | COMPLETE |
