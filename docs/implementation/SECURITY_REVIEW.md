# SwiftDrop — Security Review

**Scope:** non-deployment application security. Production TLS, hosting and cloud
provisioning are excluded from this phase.

## 1. Authentication and sessions

| Control                    | Status | Evidence                                                                               |
| -------------------------- | ------ | -------------------------------------------------------------------------------------- |
| Password hashing           | PASS   | bcrypt cost 12 with a decoy hash for unknown accounts                                  |
| Access tokens              | PASS   | HS256, issuer, expiry, `jti`, role, `sv`; verification rejects unknown roles           |
| Refresh tokens             | PASS   | 32-byte random, SHA-256 at rest, rotation, reuse detection with 10s grace              |
| Logout after access expiry | PASS   | Refresh token is the credential; session family revoked; cookies cleared; idempotent   |
| Concurrent refresh/logout  | PASS   | Row lock + successor-chain revocation; 20-iteration integration test                   |
| Session revocation         | PASS   | `sessioncheck` cache 30s, fail closed; applied to parcel/pricing/realtime/notification |
| Lockout                    | PASS   | 5 failures / 15 minutes; timing equalized for unknown accounts                         |
| Cookie posture             | PASS   | HttpOnly, SameSite=Lax, configurable Secure, narrow refresh path                       |
| Token logging              | PASS   | Auth/WS gateway logs path only; tokens/passwords never logged                          |

Residual: an issued access token remains valid until expiry after logout (≤15
minutes); immediate global logout would require a product decision and a
session-version bump.

## 2. Authorization

| Area                       | Status | Evidence                                                              |
| -------------------------- | ------ | --------------------------------------------------------------------- |
| Customer ownership         | PASS   | Parcel detail/history/receipt/proof reads scope by `customer_user_id` |
| Courier ownership          | PASS   | Current assignment + `is_current` verified under lock                 |
| Dispatcher/Admin hierarchy | PASS   | `Role.Satisfies`; ADMIN satisfies DISPATCHER                          |
| Merchant isolation         | PASS   | Merchant resolved from token subject; order reads scoped by merchant  |
| ADMIN audit reads          | PASS   | ADMIN-only; each service reads only its own schema; details redacted  |
| Access matrix              | PASS   | 66 endpoints × anonymous + five roles passed against Compose          |
| Frontend role gates        | PASS   | `RequireRole` plus backend checks; hiding a link is not authorization |

## 3. CSRF and browser security

| Control                     | Status             | Decision                                                                                                                                                                                                                                                                        |
| --------------------------- | ------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `Sec-Fetch-Site` rejection  | PASS               | Cross-site state-changing requests refused                                                                                                                                                                                                                                      |
| Forwarded-Origin comparison | PASS               | Origin must match gateway host/scheme                                                                                                                                                                                                                                           |
| SameSite cookies            | PASS               | Lax session cookies                                                                                                                                                                                                                                                             |
| JSON-only mutations         | PASS               | Cross-site HTML forms cannot set the required content type                                                                                                                                                                                                                      |
| CSRF synchronizer token     | ACCEPTED GAP       | Not added. The current combination is proportionate for the same-origin SPA/API deployment. A token would be required if the API is exposed cross-origin or cookie policy is relaxed.                                                                                           |
| CSP                         | PASS with residual | Default-src self, object/base/form restrictions, frame ancestors. `'unsafe-inline'` remains for Next hydration and React style attributes; a nonce requires a Next middleware/hydration change and was not introduced without evidence it can be done without breaking the app. |
| Clickjacking/MIME/referrer  | PASS               | X-Frame-Options, nosniff, referrer policy, COOP, permissions policy                                                                                                                                                                                                             |

## 4. Internal service trust

`INTERNAL_TOKEN` is a shared secret accepted by identity, pricing, parcel and
merchant internal routes. It is checked with constant-time comparison, internal
routes are never proxied by Nginx, and unset tokens fail closed.

**Residual risk (accepted):** any service holding the token can call any internal
route, including merchant contact lookups, parcel creation for a supplied
merchant/owner and webhook secret reads. Per-service identities or signed
requests would reduce blast radius but require a cross-service credential
redesign. The acceptance documentation and AGENTS.md record the boundary; no
new privilege was added in this phase.

## 5. Merchant ownership

- Merchant order creation authenticates an API key or MERCHANT session and
  resolves the merchant from the authenticated owner.
- Merchant reads are scoped by the resolved merchant ID.
- Parcel's internal create endpoint is a trusted-caller boundary: it validates
  shape and replay ownership but cannot independently prove that a caller-supplied
  merchant/owner pair is valid. Merchant is the authoritative caller and checks
  API key, approval and owner before calling.
- Cross-merchant order reads return 404; ownership tests pass.

**Residual risk (accepted):** a compromised internal service with the shared token
could forge a merchant/owner pair. Fixing this requires a signed internal
identity or a Merchant verification endpoint; it is documented rather than
silently ignored.

## 6. AI assistant

| Control                       | Status                                                                               |
| ----------------------------- | ------------------------------------------------------------------------------------ |
| Tool allowlist                | PASS — exactly four read-only tools                                                  |
| Role gate                     | PASS — CUSTOMER/DISPATCHER/ADMIN only; guest/MERCHANT/COURIER denied before upstream |
| Caller identity from model    | REJECTED — identity comes only from verified middleware                              |
| Ownership-scoped parcel reads | PASS — employee/customer Parcel APIs, never anonymous tracking                       |
| Guest sessions                | PASS — HttpOnly Secure cookie bound to conversation                                  |
| Prompt/extraction guard       | PASS as supplementary heuristic; not an authorization control                        |
| Output sanitization           | PASS — allowlisted safe fields; frontend renders a safe Markdown subset              |
| Rate/concurrency              | PASS for one instance; process-local                                                 |
| Cross-user tests              | PASS — guard and tool tests cover another customer's data                            |

Live Gemini smoke passed (HTTP 200, deterministic reply). Live Google consent
remains a manual provider check.

## 7. Webhooks

- HTTPS-only destinations; private/loopback/link-local ranges blocked at save and
  at dial time; no redirects; no proxy.
- HMAC-SHA256 over the raw body and timestamp; five-minute replay window.
- Six business attempts (1m/5m/30m/2h/6h) through delay queues; attempts recorded;
  suppression for non-APPROVED merchants/inactive webhooks; no replay on
  reinstatement.
- Secret rotation uses a versioned AES-256-GCM keyring with AAD binding.
- Real-stack reliability suite passed: transient retry, six-attempt exhaustion,
  rotation.

Residual: delivery is at-least-once; receivers must dedupe on `eventId`.

## 8. Proof storage

- Private local/S3-compatible storage; server-generated keys; MIME/content/
  dimension limits; assignment and quota checks under lock.
- Authorized customer/courier/staff reads; presigned URLs are short-lived; local
  streams are `no-store`.
- Missing objects return 404; reconciliation detects orphans/missing rows.
- Retention is a business decision; no automatic deletion.

## 9. Audit and sensitive data

- New ADMIN audit APIs are read-only, paginated and scoped to the owning schema.
- Credential-like detail keys are redacted recursively before rendering.
- Audit tables cannot be modified through the read API.
- Notification logs mask recipients.
- CI secret scanning (Gitleaks) and CodeQL passed.

## 10. Findings summary

| Severity | Finding                                    | Status                                   |
| -------- | ------------------------------------------ | ---------------------------------------- |
| Critical | None                                       | —                                        |
| High     | Logout after access expiry (previous pass) | FIXED and verified                       |
| High     | None open                                  | —                                        |
| Medium   | Shared internal-token blast radius         | Accepted/documented                      |
| Medium   | CSRF synchronizer token absent             | Accepted for same-origin threat model    |
| Medium   | CSP `unsafe-inline`                        | Accepted with documented nonce follow-up |
| Medium   | Single-instance rate/AI/realtime state     | Accepted, documented                     |
| Low      | Audit read API absent (previous pass)      | FIXED and verified                       |
| Low      | S3 test helper failed outside us-east-1    | FIXED                                    |

**Conclusion:** no unresolved critical or high-severity application defect was
found in the approved non-deployment scope. Residual risks are explicit,
bounded and recorded; they are not presented as solved.

## 11. Post-merge re-verification (2026-11-20)

Re-checked on the merged baseline `831455b` without changing any control:

- ADMIN audit reads returned `200` for ADMIN, `403` for CUSTOMER and `401` for
  anonymous on Identity, Merchant and Pricing.
- The notification flow re-ran all 18 tests, including suppressed merchant
  deliveries, invalid/foreign events, identity-unavailable dead-lettering, and
  SMTP-failure recovery; no secret or recipient value appeared in logs.
- Authentication, authorization, contract, proof/object-storage, frontend and
  Compose checks passed; no test or security control was weakened.

Findings and accepted residual risks in sections 1–10 are unchanged.
