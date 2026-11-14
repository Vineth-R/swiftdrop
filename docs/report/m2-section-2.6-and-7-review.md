# M2 review — report Sections 2.6 and 7

Reviewer: M1 platform owner. Source: `SwiftDrop_with_M5_Section3.docx` and the
current repository at `origin/development` (`0a05f7a`) plus the M1 completion
branch.

## Finding

Both headings are present in the DOCX, but neither has body content:

- `2.6 Account use cases (every signed-in role)` is immediately followed by
  `3. Data Model`.
- `7. Security Design` is immediately followed by
  `8. Design Patterns and Principles`.

The requirement traceability table still lists `2.6` and `7` as satisfying real
requirements, so the report currently overstates coverage. This is a content
gap, not a code gap.

## What Section 2.6 should contain

Every signed-in role shares a common account journey. The section should
describe, with the actual routes:

| Journey                 | Route                                        | Evidence                                                                              |
| ----------------------- | -------------------------------------------- | ------------------------------------------------------------------------------------- |
| Register a customer     | `POST /api/v1/auth/register`                 | `identity/internal/http/auth.go:159`; verification email is required before a session |
| Verify email            | `POST /api/v1/auth/verify-email`             | `identity/internal/http/auth.go:100,118`                                              |
| Log in                  | `POST /api/v1/auth/login`                    | `identity/internal/http/auth.go:179`; lockout and 10/min per client                   |
| Refresh                 | `POST /api/v1/auth/refresh`                  | `identity/internal/http/auth.go:210`; rotation and reuse rejection                    |
| Log out                 | `POST /api/v1/auth/logout`                   | `identity/internal/http/auth.go:240`; clears cookies                                  |
| Read/update own profile | `GET/PATCH /api/v1/users/me`                 | `identity/internal/http/profile.go`                                                   |
| Change password         | `PUT /api/v1/users/me/password`              | `identity/internal/http/profile.go`                                                   |
| Google login (opt-in)   | `GET /api/v1/auth/google/login` and callback | `identity/internal/http/google.go`; requires external credentials                     |

The section should also state:

- Roles: CUSTOMER, MERCHANT, COURIER, DISPATCHER, ADMIN.
- Access tokens are short-lived; refresh tokens rotate.
- Merchant accounts require approval before merchant business routes work.
- Unverified customers cannot obtain a session.
- Account use cases are shared; role-specific screens are described in 2.3–2.5.

## What Section 7 should contain

The security design should be grounded in the implemented controls:

| Control                | Implementation                                                                                     |
| ---------------------- | -------------------------------------------------------------------------------------------------- |
| Password hashing       | Identity password hasher with a current work factor; no plaintext storage                          |
| JWT validation         | Shared `packages/go-common/auth`; signature, algorithm, expiry and role checks                     |
| Session version (`sv`) | Access tokens carry `sv`; suspension and reapproval invalidate old tokens                          |
| Cookies                | HttpOnly access/refresh cookies; `COOKIE_SECURE` is mandatory in production; SameSite is validated |
| Authorization          | Backend role middleware on protected routes; client checks are never the control                   |
| Internal APIs          | `X-Internal-Token`; Nginx returns 404 for `/internal`; fail-closed when unset                      |
| Request IDs            | Accepted or generated at the edge, echoed and propagated to internal calls                         |
| Error handling         | Shared envelope, panic recovery, no stack traces or SQL details to clients                         |
| Logging                | Structured `slog` with a redacting handler; no request bodies; secrets/PII redacted                |
| Input limits           | Per-handler body limits; gateway proof limit; JSON 413 responses                                   |
| Secrets                | Environment only; `.env` untracked; Gitleaks in CI                                                 |
| Images                 | Non-root, minimal runtime; Trivy and SBOM in CI                                                    |
| Merchant webhooks      | HTTPS-only, encrypted signing secret, signed deliveries, durable attempt log                       |
| AI assistant           | Server-side Gemini key, verified caller context, no database access, read-only tools, rate limits  |
| Data protection        | Private proof storage, presigned short-lived URLs, masked notification recipients                  |

The section should explicitly state limitations:

- Webhook delivery is at-least-once across an HTTP boundary.
- Realtime deduplication is process-local and single-replica.
- Google OAuth requires external configuration and was not tested without
  credentials.
- No production TLS or Kubernetes deployment exists yet.

## Recommendation

Assign Section 2.6 to M2 and Section 7 to M2/M9 as previously agreed. Do not
mark either requirement as fully evidenced until the body content is inserted
and cross-checked against the routes above.
