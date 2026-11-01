# M3 consistency review: implementation against API v1.1 and Database Design v1.1

Scope: Google login, password reset, the shared mailer, the internal APIs, admin user
management and `GET /couriers` (Team Task Sheet M3.1 to M3.4; the frontend, M3.5, is out of scope).
Method: each endpoint was read from `docs/API_Documentation_v1.1.md` and checked against the Gin
routes, handlers, service rules, SQL and tests. `Matches` means verified by reading the code and by a
unit test; `SQL` items are covered only by integration tests that have **not been run** (see the last
section).

## 1. Endpoints

| API  | Route (service-side)                           | Access (documented → built)                    | Result                                                                                                                       |
| :--- | :--------------------------------------------- | :--------------------------------------------- | :--------------------------------------------------------------------------------------------------------------------------- |
| 2.5  | `GET /auth/google/login`                       | Public → public                                | Matches. `redirectTo` optional, relative only. Extra: `422` for a bad `redirectTo`, `429`, route absent (`404`) if Google is not configured. |
| 2.6  | `GET /auth/google/callback`                    | Public → public                                | Matches: state check, code exchange, ID token, `google_sub` then email then create CUSTOMER, cookies `sd_access`/`sd_refresh`, `302`, failure to `/login?error=oauth_failed`, JSON `400 OAUTH_STATE_INVALID` / `502 OAUTH_PROVIDER_ERROR`. Extra: `403`, `429`. |
| 2.10 | `GET /admin/users`                             | ADMIN → ADMIN                                  | Matches: filters `role`, `isActive`, `q`, `page`, `pageSize`, `sort`; fields `id,email,fullName,phone,role,isActive,createdAt`; `meta.pagination`. |
| 2.11 | `POST /admin/users`                            | ADMIN → ADMIN                                  | Matches: `201`, `courierProfile` required only for `COURIER`, `409 EMAIL_ALREADY_EXISTS`, `422`, audited.                    |
| 2.12 | `GET /admin/users/{userId}`                    | ADMIN → ADMIN                                  | Matches: adds `courierProfile`, `lastLoginAt`, `updatedAt`; `404`.                                                           |
| 2.13 | `PATCH /admin/users/{userId}`                  | ADMIN → ADMIN                                  | Matches: any subset of `fullName,phone,role,courierProfile{vehicleType,vehicleNumber,homeZoneId,isAvailable}`; `404`, `422`, audited. Extra rules in section 2. |
| 2.14 | `PATCH /admin/users/{userId}/status`           | ADMIN → ADMIN                                  | Matches: `{ "data": { "id", "isActive" } }`, `409 CANNOT_DEACTIVATE_SELF`, deactivation revokes all refresh tokens, audited. |
| 2.15 | `GET /couriers`                                | DISPATCHER, ADMIN → same                       | Matches: active couriers; `isAvailable`, `zoneId`, `page`, `pageSize`; fields `id,fullName,phone,vehicleType,homeZoneId,isAvailable`. ADMIN passes through the existing "ADMIN satisfies DISPATCHER" rule. |
| 2.16 | `POST /auth/forgot-password`                   | Public, rate limited → same                    | Matches: always `202` with the documented message; token hash only; 30 minutes; older links die; never in logs or responses. |
| 2.17 | `POST /auth/reset-password`                    | Public, valid token → same                     | Matches: token checked first, then the password; `400 INVALID_OR_EXPIRED_RESET_TOKEN` for missing, invalid, expired and used alike; `422` leaves the token unspent; one transaction; sessions revoked. |
| 13   | `GET /internal/couriers/{courierId}/eligibility` | `X-Internal-Token`, `X-Request-Id` → same     | Matches: `isActive`, `isAvailable`, `role`. Not routed by Nginx.                                                              |
| 13   | `GET /internal/users/{userId}/contact`         | `X-Internal-Token`, `X-Request-Id` → same      | Matches: `fullName`, `email`, `phone`. Not routed by Nginx.                                                                   |

`PUT /users/me/password` (2.9) belongs to M2 and is deliberately not registered by M3
(`TestRouter_everyOptionTogether` asserts this).

## 2. Where the implementation goes beyond, or differs from, the documents

None of these changes a documented request or response field. Each is a decision the documents leave open,
or a stricter reading, and each is explained in the report sections or the handoff.

| #  | Item                                                                                          | Document says                              | Implementation                                                                                                              | Why                                                                                            |
| -- | :-------------------------------------------------------------------------------------------- | :----------------------------------------- | :-------------------------------------------------------------------------------------------------------------------------- | :--------------------------------------------------------------------------------------------- |
| 1  | Unverified Google email                                                                       | 2.6: silent                                | Refused (`403 FORBIDDEN` when called directly)                                                                              | Linking by email is only safe when Google vouches for the address.                              |
| 2  | Google endpoint rate limit                                                                    | 1.7: no figure for these two endpoints     | 10 per minute per client address                                                                                            | Login writes a row per call; this prevents flooding. The login figure is reused.                |
| 3  | Own-role change by an admin                                                                   | 2.13: silent                               | `409 CONFLICT` (a documented generic code)                                                                                  | Stops an admin removing the last ADMIN by accident.                                              |
| 4  | `MERCHANT` role in `PATCH`                                                                    | 2.13: silent                               | Cannot be assigned, and a merchant's role cannot be changed (`422`)                                                         | A merchant account is tied to a merchant record owned by the Merchant service.                  |
| 5  | Admin create: roles                                                                           | 2.11: "courier, dispatcher or admin"       | Exactly `COURIER`, `DISPATCHER`, `ADMIN`; `phone` required in E.164                                                         | Same phone rule as registration.                                                                |
| 6  | Courier role needs a profile                                                                  | 2.11/2.13: profile "required only when role is COURIER" | A user cannot become a `COURIER` without a profile, and profile fields are refused for other roles (`422`) | Keeps `/internal/.../eligibility` and `GET /couriers` meaningful.                       |
| 7  | Internal responses in the standard envelope                                                   | 13: "Returns { ... }"                      | `{ "data": { ... }, "meta": { ... } }`                                                                                      | 1.5 says every success response uses the envelope. Callers read `data`.                          |
| 8  | Internal errors                                                                               | 13: none listed                            | `401 UNAUTHENTICATED` (token), `400 BAD_REQUEST` (request ID), `404 NOT_FOUND`                                              | Codes taken from the global table (1.9).                                                         |
| 9  | Missing phone in internal contact                                                             | 13: silent                                 | `"phone": ""`                                                                                                                | Callers need no special case for `null`.                                                         |
| 10 | Admin list default order                                                                      | 2.10: silent                               | `createdAt:desc`                                                                                                             | Newest first is the natural default for an admin list.                                           |
| 11 | `INTERNAL_TOKEN` shorter than 16 characters or unset                                          | 1.2: "shared secret"                       | All internal calls refused (`401`); startup warning                                                                          | An unset variable must never open the endpoints.                                                 |
| 12 | Admin and courier endpoints check the caller's account in the database                       | 1.2: role comes from the access token     | After the token check, the caller's current role and active flag are read from the database on every call (`403 ACCOUNT_DISABLED`, `403 FORBIDDEN`, or `401` if the account is gone) | An access token is a signed copy of the role at issue time. Without this, a deactivated or demoted administrator keeps administering until the token expires. |
| 13 | Google `sub` longer than 100 characters                                                       | 3.2: `google_sub` is `VARCHAR(100)`       | Refused as a provider error                                                                                                  | Fails cleanly instead of as a database error. |


## 3. Database design

The original M3 PR added no migration because `migrations/identity/000002_tables.up.sql` already created every table the features use. The security remediation adds identity `000003_oauth_states_browser_binding` for the nullable `browser_binding_hash` column; it does not recreate `oauth_states`.

| Table / rule (Database Design v1.1)                                             | Used how                                                                                                              | Result |
| :------------------------------------------------------------------------------ | :-------------------------------------------------------------------------------------------------------------------- | :----- |
| 3.2 `users.google_sub` unique, `auth_provider`, `email_verified`, `password_hash` NULL for Google | Google sign-up inserts `GOOGLE`, `NULL` password, `email_verified = true`; the `users_has_login_method` check is satisfied by `google_sub` | Matches (SQL) |
| 3.2 `email_verified`: "Set true for Google sign-ups"                             | Also set when an existing account is linked, because Google just verified the address                                | Consistent |
| 3.3 `courier_profiles` (`vehicle_type` 30, `vehicle_number` 20, `home_zone_id` no FK, `is_available`) | Created with a COURIER; partial updates use an upsert; zone is not checked against Pricing (no cross-service FK by design) | Matches (SQL); zone existence is **not** validated |
| 3.4 `refresh_tokens` stored as SHA-256 hashes, `revoked_at`                      | Google login reuses the M2 helpers; deactivation and reset revoke every active token of the user                      | Matches (SQL) |
| 3.5 `oauth_states` (`state` 64, `code_verifier`, relative `redirect_to`, `browser_binding_hash`, expiry about 10 minutes) | 43-character state and verifier; 10 minute expiry; the SHA-256 browser-binding hash, state and expiry are consumed by a single `DELETE ... RETURNING`; expired rows purged on each login start | Matches (SQL) |
| 3.6 `audit_logs` (actor, action, `entity_type`, `entity_id`, JSONB details; **no secrets or personal data**) | Actions `USER_CREATED`, `USER_UPDATED`, `USER_ACTIVATED`, `USER_DEACTIVATED`, entity type `user`, written in the same transaction as the change. Details hold changed field names and role/status before and after; never phone, name, password or token | Matches (SQL) |
| 3.7 `password_reset_tokens` (hash, 30 minutes, `used_at`, `requested_ip`; one unused token per user) | Older tokens marked used before the insert, with the user row locked; consume in one transaction with the password update and session revocation | Matches (SQL) |
| 11 retention clean-up (daily)                                                   | `oauth_states`: expired rows deleted on each login start. `refresh_tokens` and `password_reset_tokens` older than 7 days: **not implemented** | Partly. Recommendation only; harmless because validity is checked by `expires_at` |
| 11 "personal data ... never written to logs, audit_logs or error messages"       | Logs carry user IDs only; tests assert no address, link, token, code, state or contact data appears                   | Matches |

## 4. Gaps and inconsistencies found in the documents

1. API section 13 lists no error responses and does not say whether internal bodies use the envelope. Decisions 7 and 8 above
   fill this in; M1 and M6 should confirm they parse `data`.
2. The gateway has no exact-match rule for `/api/v1/admin/users` (without the trailing slash), unlike `users`, `couriers`
   and the other groups, and its `return 308` rules drop the query string. The frontend should use the slashed form.
   (M1's file; not changed by M3.)
3. `INTERNAL_TOKEN` is passed only to `identity` in `docker-compose.yml`; `parcel` and `notification-worker` need it too
   (exact lines in `docs/identity-internal-api.md`).
4. API 2.11 does not say whether `phone` is required; the registration rule (E.164, required) was applied.
5. M4's unmerged branch (`feature/pricing-public-tracking`) adds `httpx.InternalToken` to `packages/go-common/httpx`, a second
   internal-token check. It compares raw bytes (leaking the token length by timing), has no minimum length and does not require
   `X-Request-Id`, which section 13 says every internal call carries. Identity keeps its own stricter check. When that branch is
   merged the two should be consolidated by agreement between M3 and M4.
6. Resolved in this revision: the shared documents that still said Identity "does not issue tokens yet", "No service sends email
   yet", or listed Google login and password reset as planned (`docs/architecture.md`, `README.md`, `AGENTS.md`).

## 5. Known limits

- **Access tokens are not revocable.** A signed access token stays valid until it expires (15 minutes by default). Identity's admin
  and courier endpoints now check the caller's current account (section 2, item 12), so a deactivated or demoted administrator
  is refused there at once. Other services (`parcel`, `pricing`, `merchant`) and Identity's own `/users/me` endpoints still
  honour the token until it expires, and a password reset does not end an access token already issued. Closing this for the whole
  system needs a team decision (shorter `ACCESS_TOKEN_TTL`, or a shared revocation check); see the completion report.
- **Linking a Google account by email (fixed).** When the existing account's email was not verified, the linking transaction
  clears `password_hash` and revokes every active refresh token; the legitimate owner restores password login through the reset
  flow. A verified account keeps its password. See report sections 6.4.2 and 6.4.4.
- **OAuth login CSRF (fixed).** A per-attempt HttpOnly `SameSite=Lax` cookie carries a random binding value; only its SHA-256
  hash is stored. The callback must present the matching cookie, and a missing or mismatched binding is rejected without
  consuming the legitimate state. See report section 6.4.
- Request time for `forgot-password` differs by a few milliseconds between known and unknown emails (a database write);
  the large difference, email delivery, is hidden by sending in the background.

## 6. What has not been verified

The SQL integration tests, unit tests and race tests now run locally against PostgreSQL 14.17 with `TEST_DATABASE_URL` (isolated databases). CI for the remediation commit has not run because the branch was not pushed; the earlier CI result at `cf60e8f` predates these changes.

| Not verified                                                                                  | Needs                                           |
| :-------------------------------------------------------------------------------------------- | :---------------------------------------------- |
| PostgreSQL integration on the CI database version (PostgreSQL 16)                             | Push the branch and run CI                      |
| A real Google sign-in; the redirect URI registered in Google Cloud matching `GOOGLE_REDIRECT_URI` | Docker Compose, real credentials, a browser     |
| A reset email arriving in Mailpit; Gmail STARTTLS delivery                                    | Docker Compose with Mailpit                     |
| Internal endpoints from `parcel` / `notification-worker`; `/internal` returning `404` through the gateway | Docker Compose                                  |
| Login with a deactivated user rejected end to end                                             | Docker Compose                                  |
| `nginx -t` and live gateway access-log output                                                 | Docker daemon / a running gateway               |
| Figures 14 to 17 opened and rendered in draw.io; the OpenAPI files in a rendering tool        | draw.io / an OpenAPI viewer                     |

## 7. M4 report review status

No M4 report sections or diagrams exist in the repository, its branches, or PR #16 as of this review. M3 therefore could not
review them. This is an outstanding dependency, not a completed review. When M4 publishes its sections, M3 should review them
and record the outcome here.
