# Registration, Email Verification and Merchant Onboarding — Repository Audit

## Scope and baseline

- Baseline: local `development` at `f313ec3`.
- Worktree: isolated `feature/registration-email-verification-merchant-approval`.
- The existing M7 proof-upload worktree is not modified by this milestone.

## Existing identity implementation

- Email/password registration, login, refresh rotation, logout and lockout already exist.
- `identity.users.email_verified` exists but is not enforced by login.
- Password reset already implements the pattern this milestone reuses: a 32-byte URL-safe token, SHA-256 storage, one active token per user, 30-minute expiry, transactional consume and uniform invalid-token errors.
- Google sign-in creates/links verified accounts.
- Internal identity APIs currently expose courier eligibility and user contact only.
- Access tokens are HS256 JWTs with `sub`, `role`, `jti`, `iat` and `exp`; no session version exists.
- Seed users insert `email_verified=false`; a customer login gate would otherwise break the local demo accounts.

## Existing merchant implementation

- Merchant records already support `PENDING`, `APPROVED`, `REJECTED` and `SUSPENDED`, with transitions, audit logs and nullable `rejection_reason` (currently required for rejected rows and limited to 255 characters).
- API-key and webhook settings exist.
- There is no registration endpoint and no admin merchant list.
- `/merchants/*` and `/admin/merchants/*` are registered without access-token middleware. The access matrix documents this as a known pending gap.
- Merchant emails for received, approved, rejected, suspended and API-key creation already exist as templates.
- The merchant service has no SMTP setup, identity client or pricing client.

## Existing frontend implementation

- `/register` is the customer registration form.
- Merchant registration and merchant profile/key components exist but their routes are not wired.
- Admin navigation points at `/admin/merchants`, but no page exists.
- There is no `/register/customer`, `/register/merchant`, `/verify-email`, `/merchant/pending`, `/api-keys` or `/merchant/profile` page.
- Login only handles invalid credentials, lockout and deactivation.
- The E2E suite mocks all API traffic; there is no real Compose + Mailpit browser suite.

## Gaps addressed by this milestone

1. Customer verification tokens, endpoints, resend policy and login enforcement.
2. Merchant registration saga, inactive Identity account, admin list and approval activation.
3. Optional, nullable rejection reason up to 1,000 characters.
4. Lifecycle-bound session invalidation using `sv` and transition IDs/sequences.
5. Merchant and admin session authorization middleware.
6. Frontend onboarding, verification, pending, admin and merchant screens.
7. Contracts, documentation, Compose, Nginx, CI and real-stack tests.

## Decisions

- Customer verification and merchant approval remain separate.
- Merchant accounts are inactive until admin approval.
- Merchant registration never creates a session.
- Merchant lifecycle transitions are idempotent and bound to a unique transition ID and per-merchant sequence.
- Identity increments `session_version` for every newly applied lifecycle transition, even when `is_active` already matches the target.
- Merchant registration uses an idempotent attempt saga with compensating Identity deletion.
- Account and merchant lifecycle emails are sent in the background over the existing SMTP mailer.
- Public `/merchant/pending` is informational only and leaks no merchant-specific data.
