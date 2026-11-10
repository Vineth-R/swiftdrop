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
- Lifecycle transitions validate transition ID, user ID, operation and sequence
  payload consistency and reject stale sequences.

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
- Identity ADMIN/courier current-access checks compare the JWT `sv` claim.
- Password hashing, refresh rotation, lockout and existing session rules are
  unchanged.

## 10. Final integration and fixes

`origin/development` advanced from `6652a44` to `f03d31e` (the M7 courier
proof-upload merge) while this branch was in review. The branch was integrated
with merge commit `99dfd9b`; AGENTS and README conflicts were resolved by
keeping the latest development proof-upload/configuration text and adding the
registration/merchant/session-version implementation notes. The original M7
worktree was not modified.

Fixes applied during final QA:

- Playwright pagination flake root cause: `getByRole('button', { name: 'Next' })`
  also matched Next.js Dev Tools' "Open Next.js Dev Tools" button. The reports
  test now uses `exact: true`; `reports.spec.ts:170` passed 20/20 repeated runs
  with four workers. No timeout increase, test skip or assertion weakening.
- The mocked suite was run against the production Next server (`CI=1`) to avoid
  dev-server restarts under parallel load.
- Real-stack tests now retry only on the documented login/registration 429 with
  `Retry-After`, cache the admin token for the suite, and exercise lifecycle
  approval/suspension through the API after the browser journeys.
- Identity lifecycle idempotency now compares the stored transition's `user_id`
  as well as operation and sequence. Reusing a transition ID for another user
  returns `TRANSITION_ID_CONFLICT`; an integration test covers it.
- Identity ADMIN and courier current-access checks now compare the JWT `sv` claim
  with the current user session version and reject stale sessions.

## 11. Security and dependency audits

`govulncheck ./...`:

- The code is affected by **0** vulnerabilities.
- 0 vulnerabilities in imported packages; 1 vulnerability exists in a required
  module but is not reachable from this code.

`pnpm audit` before correction: 19 vulnerabilities, all in development-only
tooling (3 low, 10 moderate, 6 high); `pnpm audit --prod` reported **0**.
Undici was pinned through Nx, and minimatch through ESLint tooling.

Corrections in `pnpm-workspace.yaml`:

- `undici: 7.30.1` (patched minor; used only by Nx).
- `minimatch: 11.0.1` (patched major; lint, unit tests and build re-run
  successfully).

Final `pnpm audit`: **0 vulnerabilities**. `pnpm audit --prod`: **0**.

## 12. Tests executed and results

Passed:

- `gofmt -l apps/services packages/go-common tests` (no output)
- `go vet ./...`
- `go test -race -count=1 ./...`
- `go build ./...`
- PostgreSQL integration tests against disposable databases:
  - identity repository/seed
  - merchant HTTP/webhook/API-key/rejection-audit
  - `tests/integration/database`
  - pricing HTTP
  - notification idempotency and notification-log repositories
- `docker compose config --quiet`
- Sequential Docker image builds; the initial fully parallel build hit a local
  out-of-memory compiler kill, and sequential builds of identity, parcel, web
  and the remaining services succeeded.
- Full Compose stack reached healthy state; `make migrate` reported no pending
  changes after the initial migration run.
- `make test-access`: passed, including role matrix, ownership and session
  suites.
- Real-stack Playwright against Compose + Mailpit: **4 passed**:
  - customer register → verify email → login → dashboard
  - merchant pending → admin approval → approval email → merchant login → API keys
  - rejection with and without reason → correct email → login refused
  - approve → login → stop Identity → suspend (`identitySync: PENDING`) →
    restart Identity → reapprove → old token rejected → fresh login → API keys 200
- `pnpm exec nx run-many -t lint test typecheck --projects=@org/web,@org/web-e2e`
- `pnpm exec nx build @org/web`
- Mocked Playwright (`CI=1`, Chromium): **602 passed, 0 failed, 0 skipped** in
  2.7 minutes. The previously failing pagination test also passed 20/20 repeated
  runs under four workers.
- `govulncheck ./...`: no reachable vulnerabilities.
- `pnpm audit`: 0 vulnerabilities.
- `git diff --check`.

Not run:

- GitHub Actions CI was not executed from this environment; the workflow and
  path filters were reviewed and `tests/scripts` passed.
- Mailpit delivery was verified through the real-stack tests rather than a
  separate manual inbox check.

## 13. CI

The path-filtered `real-stack-e2e` job builds images, starts a disposable
Compose stack, applies migrations, creates the ADMIN fixture, runs the real-stack
Playwright project, dumps logs on failure and tears down with `down -v`. The
required gate treats the job as optional/skipped-or-success. Dependency
overrides live in `pnpm-workspace.yaml` for the pnpm 12 configuration format.

## 14. Git

- Branch: `feature/registration-email-verification-merchant-approval`
- Base: `origin/development`
- Commits:
  - `2f906f9` identity verification, session version and lifecycle foundations
  - `621d733` registration, merchant approval, frontend, contracts, tests,
    infrastructure and docs
  - `58a5ea0` initial completion report
  - `99dfd9b` merge of latest development (`f03d31e`)
  - `3b85446` lifecycle idempotency hardening and Playwright/security fixes
  - This report is committed separately on the same branch.
- Nothing was merged. The branch is pushed and a PR is opened into `development`
  for review; the PR is not merged.

## 15. Remaining limitations

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
- Docker builds may need `COMPOSE_PARALLEL_LIMIT=1` on memory-constrained
  machines.
