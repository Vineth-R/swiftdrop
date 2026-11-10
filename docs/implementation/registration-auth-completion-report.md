# Registration, Email Verification and Merchant Approval — Completion Report

## 1. Initial state

The audit is in `docs/implementation/registration-auth-audit.md`. Identity had
email/password auth, refresh rotation, lockout, password reset, Google login and
an unused `email_verified` column. Merchant had status transitions, audit logs,
API keys and webhooks, but no registration, no admin list, no session auth on
merchant business routes and no lifecycle/session invalidation. The frontend had
a customer register form but no account-type selection, verification flow,
public merchant onboarding or admin merchant screen. The mocked Playwright suite
existed; no real-stack suite existed.

## 2. Architecture decisions

- Customer registration creates an unverified CUSTOMER, sends a single-use
  verification email, and never creates a session.
- Merchant registration creates an **inactive** Identity MERCHANT account and a
  PENDING merchant application. No session is issued.
- Merchant approval applies an Identity lifecycle transition, then commits
  APPROVED. Login is blocked until activation succeeds.
- Rejection leaves the account inactive; the reason is optional and nullable.
- Suspension commits SUSPENDED immediately, then deactivates Identity and
  revokes refresh tokens. Merchant business routes also reject non-approved
  merchants.
- Access tokens carry `sv`. Every newly applied lifecycle transition increments
  Identity `session_version`, even when `is_active` already matches the target.
  Retries reuse the transition ID and sequence and do not increment it again.
- Identity checks merchant approval during MERCHANT login/refresh; Merchant
  business requests check Identity session state. Both fail closed with 503.
- Customer verification and merchant admin approval are separate mechanisms.
- Account emails use the existing shared SMTP mailer/templates in the
  background. No new RabbitMQ event contracts were introduced.

## 3. Database migrations

Identity:

- `000004_email_verification` — hashed single-use verification tokens with one
  unused token per user.
- `000005_session_version` — `identity.users.session_version`.
- `000006_merchant_lifecycle_transitions` — applied transition records with
  transition ID, user, sequence, operation and applied time.

Merchant:

- `000007_registration_onboarding` — business address, required non-blank BR
  number for new rows, unique normalized BR index, lifecycle sequence,
  optional/widened rejection reason, registration attempts and merchant status
  transitions.

All migrations applied successfully with `make migrate` against the local
PostgreSQL 16 Compose service.

## 4. Identity implementation

- Customer registration stores user + verification token transactionally and
  sends the verification email in the background.
- `POST /auth/verify-email` and `POST /auth/resend-verification` implement
  hashed, expiring, single-use, one-active-token verification with per-IP and
  per-email rate limits and no account enumeration.
- Unverified CUSTOMER login returns 403 `EMAIL_VERIFICATION_REQUIRED`.
- Inactive or non-approved MERCHANT login returns 403
  `MERCHANT_ACCOUNT_INACTIVE`. Identity checks the merchant status internally;
  an unreachable Merchant service fails closed with 503.
- JWT issuance includes `sv`. Identity profile routes compare it with the
  current user row and return 401 `SESSION_REVOKED` on a mismatch.
- Internal APIs create/resume an inactive merchant owner, apply idempotent
  lifecycle transitions, expose session state, and compensate failed
  registration.
- Lifecycle transitions validate transition ID payload consistency and reject
  stale sequences.

## 5. Merchant implementation

- `POST /merchants/register` orchestrates an idempotent merchant-owned saga:
  registration attempt, pricing zone validation, inactive Identity owner,
  PENDING merchant row, completion, confirmation email.
- Compensation deletes only an eligible newly created inactive owner.
- Merchant business routes require a valid JWT, current Identity session state,
  an APPROVED merchant and ownership resolved from `owner_user_id`.
- Admin listing supports status filter, search (business name, BR, contact
  email), sort and pagination.
- Approve/reject/suspend/reapprove use persisted transition IDs and sequences.
  Approval stays fail-closed until Identity applies the transition.
- Rejection accepts missing/null/empty/whitespace reasons, stores NULL, and
  limits the reason to 1,000 characters.
- Suspension requires a reason, commits SUSPENDED immediately, then deactivates
  Identity and revokes refresh tokens. A failed sync returns
  `identitySync: "PENDING"` for safe retry.
- Merchant emails are background SMTP with bounded retries and structured,
  secret-free logging.

## 6. Lifecycle-bound session invalidation

- Transition ID + per-merchant sequence are persisted by Merchant and applied
  idempotently by Identity.
- Identity increments `session_version` for every new transition; retries with
  the same ID do not.
- A reused ID with a different payload returns 409 `TRANSITION_ID_CONFLICT`.
- A lower sequence returns 409 `STALE_TRANSITION`.
- Merchant business requests compare the JWT `sv` with Identity's current
  session version and reject stale sessions with 401 `SESSION_REVOKED`.
- Old access tokens therefore remain invalid after reapproval even if the
  suspension deactivation call previously failed.

## 7. Frontend

- `/register` account-type selection.
- `/register/customer` customer registration with a check-email result and no
  auto-login.
- `/register/merchant` full merchant application with BR, business address,
  pickup zone and confirmation password; no auto-login.
- `/verify-email` token consumption, invalid/expired states, resend, token
  removal from the URL and redirect to login.
- `/login` verification-required/resend and merchant-inactive states; existing
  login, lockout, OAuth and reset behaviour preserved.
- Public `/merchant/pending` informational page with no authenticated calls or
  merchant data.
- `/admin/merchants` ADMIN review with optional/required reasons, confirmation,
  pagination and identity-sync warnings.
- `/api-keys` and `/merchant/profile` for approved merchants.
- Merchant navigation no longer advertises unbuilt order/webhook screens.

## 8. Contracts and documentation

Updated OpenAPI identity, identity-internal, merchant and merchant-internal
contracts; API Documentation v1.1; Database Design Document v1.1 addendum;
email documentation; merchant integration notes; AGENTS, README, `.env.example`,
Docker Compose and Nginx routing.

## 9. Security and authorization

- No plaintext verification or refresh tokens; only SHA-256 hashes are stored.
- Verification tokens are single-use, expiring and limited to one active token
  per user; raw tokens and links are never logged.
- Merchant lifecycle transitions are idempotent and payload-consistent.
- Merchant ownership comes only from the verified token subject.
- ADMIN-only merchant lifecycle routes are checked on the backend.
- Internal APIs require `X-Internal-Token` and a valid `X-Request-Id`; Nginx
  does not route `/internal`.
- Suspension revokes refresh tokens and bumps the session version.
- Merchant business routes reject stale `sv`, inactive Identity users and
  non-approved merchants.
- Password hashing, refresh rotation, lockout and existing session rules are
  unchanged.

## 10. Tests executed and results

Passed:

- `go test -race -count=1 ./...`
- `go vet ./...`
- `go build ./...`
- `gofmt -l apps/services packages/go-common tests` (no output)
- PostgreSQL integration tests against a disposable database created with a
  temporary Compose port override:
  - identity repository (including lifecycle/version idempotency)
  - identity seed
  - merchant HTTP/webhook/API-key integration
  - merchant rejection-reason persistence/audit
  - `tests/integration/database`
  - pricing HTTP integration
  - notification idempotency and notification-log repositories
- `docker compose config --quiet`
- Sequential Docker image builds for every service; the first fully parallel
  build failed with an out-of-memory Go compiler kill, and the sequential build
  succeeded.
- Full Compose stack reached healthy state and `make migrate` applied all
  migrations.
- `make test-access` passed, including the role matrix and session/ownership
  suites.
- Real-stack Playwright against Compose + Mailpit: **4 passed**
  - customer register → verify email → login → dashboard
  - merchant pending → admin approval → approval email → merchant login → API keys
  - rejection with and without a reason → correct email → login refused
  - approve → login → stop Identity → suspend (`identitySync: PENDING`) →
    restart Identity → reapprove → old token rejected → fresh login → API keys 200
- `pnpm exec nx build @org/web`
- `pnpm exec nx test @org/web` (87 tests)
- `pnpm exec nx run-many -t lint typecheck --projects=@org/web,@org/web-e2e`
- Mocked Playwright: a full run reached 601/602; the only failure was the
  pre-existing `reports.spec.ts` pagination test under parallel load, which
  passed when rerun alone. A later full rerun did not complete within the shell
  timeout; no source change was made to that suite after the passing run.

Not run or incomplete:

- `govulncheck` and `pnpm audit --prod --audit-level=high` were not run in this
  session.
- The CI real-stack job is defined but was not executed by GitHub Actions here.
- The parallel image build failure is a local resource limit, not a code
  failure.

## 11. CI

Added a path-filtered `real-stack-e2e` job that builds images, starts a
disposable Compose stack, applies migrations, creates the ADMIN fixture, runs the
real-stack Playwright project, dumps logs on failure and tears down with
`down -v`. The required gate treats the job as optional/skipped-or-success.

## 12. Git

- Branch: `feature/registration-email-verification-merchant-approval`
- Implementation commits:
  - `2f906f9` — identity verification, session version and lifecycle foundations
  - `621d733` — registration, merchant approval, frontend, contracts, tests,
    infrastructure and docs
- This report is committed separately on the same branch.
- Nothing was pushed, and no PR was opened or merged.

## 13. Remaining limitations

- Account emails are background SMTP, not a durable outbox; failed emails are
  logged and may be resent manually. Decision emails may duplicate on a manual
  retry.
- Identity MERCHANT login/refresh depends on Merchant Service availability and
  fails closed with 503. Merchant business requests depend on Identity session
  state and also fail closed.
- Merchant admin routes trust the short-lived JWT role and do not call Identity
  session state, which keeps admin actions available during an Identity outage.
- Suspension immediately blocks business operations, but a previously issued
  access token can still reach non-merchant endpoints until its normal short
  expiry; merchant resources reject it by status/session version.
- In-process rate limiting remains per replica.
- Merchant order and webhook UI screens remain future work because their order
  APIs are not implemented.
- Verification tokens are not garbage-collected.
- Pre-existing seeded databases may need re-verification or reset for demo
  CUSTOMER logins.
- The Docker parallel build memory requirement is high; CI/local builds may
  need `COMPOSE_PARALLEL_LIMIT=1`.
