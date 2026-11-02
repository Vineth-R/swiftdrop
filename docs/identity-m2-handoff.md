# Identity endpoints for the web screens: handoff for M2

From M3. Everything here is browser-facing and goes through the gateway at `http://localhost:8080`.
Exact request and response shapes are in [`contracts/openapi/identity.yaml`](../contracts/openapi/identity.yaml); API v1.1 sections are
given in brackets. Errors always use the standard envelope `{"error": {"code", "message", "details?"}, "meta": {"requestId"}}`.

## 1. Google button (API 2.5, 2.6)

The button must be a **normal link or `window.location` navigation**, not a `fetch` call. The browser has to follow the redirects to
Google and back.

```html
<a href="/api/v1/auth/google/login?redirectTo=/dashboard">Sign in with Google</a>
```

- `redirectTo` is optional. It must be a **relative path**: it starts with `/`, not `//`, with no `\`, spaces or control characters,
  at most 255 characters. Anything else returns `422 VALIDATION_ERROR` as JSON instead of redirecting, so build it from a known list
  of pages and never from user input.
- On success the browser lands on `redirectTo` (or `/dashboard`) with the `sd_access` and `sd_refresh` cookies already set. Nothing to
  store in JavaScript; call `GET /api/v1/users/me` (M2) to learn who signed in.
- On **any** failure the browser lands on **`/login?error=oauth_failed`** with no cookies. Show one generic message, for example
  "Google sign-in did not complete. Please try again." The cause is deliberately not shown.
- The first call counts toward a limit of 10 per minute per address (`429 RATE_LIMITED` as JSON).
- The route exists only when `GOOGLE_CLIENT_ID`, `GOOGLE_CLIENT_SECRET` and `GOOGLE_REDIRECT_URI` are all set on the server;
  otherwise the link answers `404 NOT_FOUND`.
- `GOOGLE_REDIRECT_URI` must be `http://localhost:8080/api/v1/auth/google/callback` locally, and the same URI must be added to the
  OAuth client in Google Cloud.
- A Google user is always created as a `CUSTOMER`. A user who registered with the same email is signed in to that account.

## 2. Forgot password page (API 2.16)

`POST /api/v1/auth/forgot-password`, `Content-Type: application/json`, body `{"email": "..."}`.

| Result | What to show                                                                                                                 |
| :----- | :--------------------------------------------------------------------------------------------------------------------------- |
| `202`  | Always the same message, whether or not the address has an account: "If an account exists for this email, a password reset link will be sent." Do not say "email sent". |
| `422`  | The email field is empty or malformed (`details[0].field` is `email`).                                                       |
| `429`  | Too many requests. Read the `Retry-After` header (seconds). 5 per minute per address and per email.                          |

## 3. Reset password page (API 2.17)

The emailed link is `http://localhost:8080/reset-password?token=<token>` (built from `FRONTEND_BASE_URL`). Read `token` from the URL
and ask only for a new password. The user never types the token.

`POST /api/v1/auth/reset-password`, body `{"token": "<from the URL>", "newPassword": "..."}`.

| Result                                        | What to show / do                                                                                               |
| :-------------------------------------------- | :-------------------------------------------------------------------------------------------------------------- |
| `200`                                         | "Password changed." Send the user to `/login`. All their sessions, including this browser's cookies, are cleared. |
| `400` `INVALID_OR_EXPIRED_RESET_TOKEN`        | "This link is invalid or has expired." Link back to the forgot password page. The same message covers a missing, wrong, expired and already used token. |
| `422` `VALIDATION_ERROR`                      | The token is fine but the password is not. `details` has `field: "newPassword"` and the rule that failed. **The link still works**, so keep the form open. |
| `429`                                         | Too many attempts. Use `Retry-After`.                                                                           |

Password rules (same as registration): at least 8 characters, one upper case letter, one lower case letter, one digit, at most 72 bytes.
The token is checked before the password, so a request with a wrong token always gets `400`, whatever the password is.

## 4. Admin screens that call M3 (API 2.10 to 2.15)

Call the **slashed collection paths**, because the gateway has no rule for the unslashed form of `/api/v1/admin/users`:

| Screen action              | Request                                                                                          |
| :------------------------- | :----------------------------------------------------------------------------------------------- |
| User list with filters     | `GET /api/v1/admin/users/?role=&isActive=&q=&sort=createdAt:desc&page=1&pageSize=20`              |
| User detail                | `GET /api/v1/admin/users/{userId}`                                                                |
| Create staff               | `POST /api/v1/admin/users/` (role `COURIER`, `DISPATCHER` or `ADMIN`; `courierProfile` only for couriers) |
| Edit                       | `PATCH /api/v1/admin/users/{userId}` (any subset of `fullName`, `phone`, `role`, `courierProfile`) |
| Activate / deactivate      | `PATCH /api/v1/admin/users/{userId}/status` with `{"isActive": false}`                            |
| Courier picker (dispatch)  | `GET /api/v1/couriers/?isAvailable=true&zoneId=&page=1&pageSize=20`                               |

Behaviour the screens need to handle:

- **Only an ADMIN** may use `/admin/users`; `/couriers` also allows DISPATCHER. The server rechecks the account on every call, so a
  user who was deactivated or demoted after signing in gets `403 ACCOUNT_DISABLED` or `403 FORBIDDEN` straight away. Treat
  `ACCOUNT_DISABLED` as "sign out and go to login".
- `409 CANNOT_DEACTIVATE_SELF`: hide or disable the deactivate button for the signed-in admin's own row.
- `409 CONFLICT` on edit: an admin cannot change their own role. `409 EMAIL_ALREADY_EXISTS` on create.
- The `MERCHANT` role cannot be assigned or changed here (`422`), and a user cannot become a `COURIER` without a `courierProfile`.
- Lists return `meta.pagination` (`page`, `pageSize`, `totalItems`, `totalPages`). `pageSize` is at most 100.
- Deactivating a user revokes their refresh tokens: they are signed out within the access-token lifetime (15 minutes by default).
