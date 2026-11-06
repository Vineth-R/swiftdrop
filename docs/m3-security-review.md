# M3 remediation security review

Reviewed integrated branch: `integration/m3-remaining-fixes` at `6202b67f35bc08bc89f931a1e8e9967a24ac7597`.
Scope: OAuth browser binding, Google account linking, gateway log redaction, production cookie enforcement,
database migration/rollback, and the associated regression tests. Frontend work is out of scope.

## 1. Findings

| ID | Finding | Severity before | Status | Evidence | Residual risk |
| :-- | :------ | :-------------- | :----- | :------- | :------------ |
| F1 | OAuth login CSRF: server-side state alone let an attacker transfer their own callback URL to a victim. | High | **Fixed** | `internal/service/google.go` creates a separate random binding value per attempt and stores only its SHA-256 hash. `internal/repository/oauth_states.go` consumes `state + browser_binding_hash + expires_at` in one `DELETE ... RETURNING`. `internal/http/google.go` sets and clears the cookie. `internal/http/cookies.go` uses `HttpOnly`, `SameSite=Lax`, `Path=/api/v1/auth/google`, 600s, `Secure` from config. | A same-browser prefetch can still consume the attempt; the user may need to restart. The cookie is deliberately not `SameSite=Strict` because the callback is a cross-site top-level navigation. |
| F2 | Account takeover: a pre-registered email/password could keep working after the legitimate owner linked Google. | High | **Fixed** | `internal/repository/users_google.go` locks the user row, rejects a different `google_sub`, and for an unverified email links the account, clears `password_hash`, and revokes active refresh tokens in one transaction. `internal/service/google.go` re-reads and safely links on a concurrent-create race. | Admin-created staff accounts also have `email_verified=false`, so they lose password login until a reset. `auth_provider` remains `LOCAL` (it records the original registration method). Verified accounts keep their password. |
| F3 | OAuth `code`/`state` could appear in gateway access logs. | Medium | **Fixed in configuration** | `infra/nginx/default.conf` defines `swiftdrop_no_query` using `$uri` and applies it to `/api/v1/auth/`; `tests/nginxconfig/nginx_config_test.go` asserts the format and block. | `nginx -t` and live log inspection could not run because the Docker daemon is unavailable. The static test is not a substitute for a runtime gateway test. |
| F4 | Production could start with `COOKIE_SECURE=false`. | Medium | **Fixed** | `internal/config/auth.go` rejects production with insecure cookies; `internal/config/auth_test.go` covers rejection and the production-secure path. | Staging is not enforced (production only). The config loader has no logger, so no staging warning is emitted. Local development remains HTTP-compatible. |
| F5 | OIDC `nonce`/`azp`, encoded redirect variants, Google key-fetch failure, callback prefetch, multi-replica rate limiting, API error-code consistency. | Low | **Open / documented** | Report section 6.4.6 and `docs/m3-consistency-review.md` record them. | No new confirmed vulnerability in this review. |
| F6 | M4 report sections were to be reviewed by M3. | Process | **Outstanding** | No M4 report/diagram artifact exists in the repository, branches, or PR #16; no M3 review comment exists on PR #17. | The review cannot be performed until M4 publishes its sections. This is a dependency, not a completed review. |

## 2. Attack-scenario regression evidence

All rows below were executed locally on PostgreSQL 14.17 with `TEST_DATABASE_URL` set, using isolated
databases. The remediation commit has not been pushed, so CI for it has not run.

| # | Scenario | Test | Result |
| :- | :------- | :--- | :----- |
| 1 | Attacker sends their own callback URL to a victim | `TestGoogleFlow_bindingIsRequiredAndNotBurnedOnMismatch`; `TestGoogleAuth_CompleteGoogleLogin_missingBindingDoesNotBurnState` | Pass: no binding cookie → `OAUTH_STATE_INVALID`, no session. |
| 2 | Callback without the initiating cookie | Same tests | Pass. |
| 3 | Callback with a mismatched cookie | Same tests; `TestRepository_ConsumeOAuthState_mismatchedBindingDoesNotConsume` | Pass: rejected, and the legitimate state is not consumed. |
| 4 | Replay a consumed state | `TestGoogleAuth_CompleteGoogleLogin_stateIsSingleUse`; `TestGoogleFlow_replayRejected`; `TestRepository_ConsumeOAuthState_worksOnce` | Pass. |
| 5 | Concurrent callbacks consume one state | `TestRepository_ConsumeOAuthState_concurrentCallbacksOnlyOneWins` (8 callers, `-race`) | Pass: exactly one success. |
| 6 | Pre-registered email/password then Google login | `TestGoogleFlow_linksUnverifiedEmailAndClearsPassword`; `TestRepository_LinkGoogleAccount` | Pass: password cleared, refresh token revoked, old password rejected. |
| 7 | Password reset after linking | `TestGoogleFlow_passwordResetRestoresPasswordAfterLinking`; `TestRepository_LinkGoogleAccount_thenPasswordResetRestoresPassword` | Pass: new password logs in. |
| 8 | Google subject already linked to another user | `TestGoogleAuth_CompleteGoogleLogin_linkConflict`; `TestRepository_LinkGoogleAccount` | Pass: conflict, no reassignment. |
| 9 | Disabled account cannot be linked | `TestGoogleFlow_disabledAccountIsNotLinked`; `TestGoogleAuth_CompleteGoogleLogin_disabledAccount` | Pass. |
| 10 | Concurrent first login | `TestRepository_CreateGoogleUser_concurrentFirstLogin`; `TestGoogleFlow_concurrentFirstLogin` | Pass: one account, both requests predictable. |
| 11 | Production `COOKIE_SECURE=false` | `TestLoadAuthFrom_errors/production_requires_secure_cookies` | Pass: startup error. |
| 12 | Gateway logs omit OAuth query values | `TestNginxAuthAccessLogOmitsQueryString`; `TestNginxUnrelatedRoutesKeepDefaultLogging` | Pass as a static configuration test. |
| 13 | Migration upgrade/rollback/re-upgrade | `TestIdentityMigration_rollbackAndReapply` | Pass on local PostgreSQL 14.17. |
| 14 | Application logs do not expose secrets | `TestGoogleCallback_logsNeverContainCredentials` | Pass: code, state and binding are absent. |

## 3. Residual risks and limitations

- CI has not run for the remediation commit because the branch was not pushed. The earlier CI result at
  `cf60e8f` covers the pre-remediation code only.
- PostgreSQL integration ran on local PostgreSQL 14.17, not the CI PostgreSQL 16. The SQL uses standard
  features, but the CI version remains unverified for this commit.
- The Docker daemon is unavailable, so `nginx -t`, the Compose runtime, and live gateway log output were
  not exercised. `docker compose config --quiet` passed.
- No real Google account or Google credentials were used. The provider is a fake in tests; the real
  redirect URI, consent screen and token endpoint remain unverified.
- The frontend is out of scope: there is no login page, Google button or `/login?error=oauth_failed`
  handler in `apps/web`.
- `auth_provider` is not changed when an unverified local account is linked; it remains `LOCAL` even
  though the password is cleared. This is an informational field, not an authorization input.
- The rate limiter is in-process; multiple identity replicas multiply the effective limit.
- OIDC `nonce`/`azp` and encoded-redirect hardening remain outstanding as documented.

## 4. Sign-off

No blocking security finding remains in the reviewed changes. The two high-severity issues identified
in the M3 audit are fixed in code and covered by executable regression tests, and the migration,
production-cookie, and gateway-log controls are implemented with tests.

Local verification is satisfactory. Remote CI, a real-Google smoke test, a runtime `nginx -t`, and the
M4 report review remain outstanding, so this review recommends **READY FOR REVIEW**, not READY TO MERGE.
