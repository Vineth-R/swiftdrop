# Phase 2 — Security, Reliability and Scalability Hardening

Status: implementation complete on `feat/phase-2-security-reliability`; pull
request open for owner review. Phase 1 (PR #151) was merged into `development`
at merge commit `19ef83c226ddec1aa4c828d0f9c2e6f80d9efb76`; Phase 2 started from
that commit.

## 1. Repository audit

The audit inspected the merged `development` branch, not an earlier assessment.
The table records what already existed, the real gap, and what this phase did.

| Workstream                           | Existing implementation                                                                                                      | Actual gap                                                                          | Proposed solution                                                                                                                        | Dependencies                        | Status                                  |
| ------------------------------------ | ---------------------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------- | ---------------------------------------------------------------------------------------------------------------------------------------- | ----------------------------------- | --------------------------------------- |
| 1. Realtime readiness                | `/health`, `/ready` checked PostgreSQL only; both broker consumers reconnected with backoff                                  | `/ready` reported ready while a consumer was disconnected                           | Consumer lifecycle state in `packages/go-common/messaging`; realtime `/ready` requires both consumers; liveness unchanged                | none                                | Implemented                             |
| 2. Shared rate limiting and AI state | Process-local `httpx.RateLimiter`, identity limiter, AI limiter and conversation store                                       | Limits/conversations differ per replica                                             | Evaluated Redis and PostgreSQL; no shared dependency adopted for the single-replica academic deployment                                  | Redis or a new shared schema        | Deferred (documented)                   |
| 3. Session revocation                | Identity and merchant checked `sv`; parcel, pricing and realtime did not                                                     | A suspended/disabled user kept access until the 15m token expiry                    | Shared `packages/go-common/sessioncheck` with bounded TTL cache and fail-closed policy; wired into parcel, pricing and realtime          | existing identity internal endpoint | Implemented                             |
| 4. Webhook secrets                   | AES-256-GCM `secretbox.Box`, one immutable key, no AAD                                                                       | Rotation would make every stored secret unreadable; ciphertext not bound to its row | Versioned keyring (`v2:<keyId>:...`), previous decryption keys, legacy `v1` fallback, merchant AAD binding                               | none (same column)                  | Implemented                             |
| 5. Proof storage                     | Provider-neutral `objectstore` with local, S3/minio, Neon adapters; presign or authorised stream; validation and quota tests | Local provider requires one replica or a shared volume                              | Verified existing abstraction and tests; documented the multi-replica requirement and S3 option                                          | operator choice                     | Verified / documented                   |
| 6. Application security              | SameSite=Lax HttpOnly cookies, JSON-only mutations, strict CORS posture, internal token, SSRF checks, WS origin checks       | No CSP; no explicit cross-site request guard at the API                             | CSP + security headers at Nginx; `Sec-Fetch-Site`/forwarded-Origin guard in the shared router                                            | none                                | Implemented                             |
| 7. Observability                     | Structured logs and request IDs                                                                                              | No metrics or tracing foundation                                                    | Dependency-free Prometheus registry, HTTP metrics, consumer metrics, realtime gauges, outbox gauges and `/metrics` on every HTTP service | none                                | Implemented (metrics); tracing deferred |

## 2. Session revocation guarantee

Identity remains authoritative for `is_active`, role and `session_version`.
`packages/go-common/sessioncheck` calls the existing
`GET /internal/users/{userId}/session-state` endpoint with `X-Internal-Token`
and a request ID.

- A successful lookup is cached for **30 seconds** (`DefaultCacheTTL`).
- A revoked, deactivated, role-changed or stale-version session is rejected
  with `401 SESSION_REVOKED` on parcel, pricing and the notification-worker
  admin read API, and the WebSocket is closed with `4401`.
- New WebSocket connections verify the session before accepting subscriptions.
- Open WebSockets revalidate every `REALTIME_SESSION_CHECK_INTERVAL`
  (default `1m`, must be positive). The access token's own expiry (15m) remains
  the final bound.
- On identity failure the checker fails closed (`503 SERVICE_UNAVAILABLE`).
  A cached value authorizes requests only while it is younger than `CacheTTL`
  (30s); there is no stale-authorization fallback. An open WebSocket is closed
  with `1013 Try Again Later` when identity cannot confirm the session after the
  cache expires.
- Merchant routes keep their existing per-request identity check.
- The notification-worker ADMIN notification-log read API also checks the
  token's session state; an unavailable identity service makes that endpoint
  fail closed with `503`.
- Logout revokes the refresh token only; an already-issued access token remains
  valid until its normal expiry (at most 15 minutes). Immediate logout across
  every service would require bumping `session_version` on logout, which also
  signs the user out of every other device and is a product decision, not a
  silent change.

This removes direct database access from the other services and keeps identity
as the only owner of session state.

## 3. Realtime readiness and shutdown

- Consumer state is `connecting`, `ready`, `reconnecting` or `stopped`.
- `/health` remains liveness: the process is alive.
- `/ready` is readiness: PostgreSQL plus both `parcel.status_changed.v1` and
  `parcel.assigned.v1` consumers must be attached.
- A reconnect keeps the process alive and flips readiness to `503` until both
  consumers reattach; readiness recovers automatically.
- Shutdown cancels the consumers, requeues in-flight messages through the
  existing nack path, and closes WebSocket clients with "going away".

## 4. Webhook secret rotation

Stored format:

- `v2:<keyId>:<base64url(nonce || ciphertext)>` for all new writes.
- `v1:<base64url(...)>` remains readable for existing rows.

Configuration:

- `WEBHOOK_SECRET_KEY` — active encryption key (64 hex characters).
- `WEBHOOK_SECRET_KEY_ID` — active key id, default `k1`.
- `WEBHOOK_SECRET_KEYS` — comma-separated `id=hex64` decryption-only keys.

Rotation procedure:

1. Generate a new key.
2. Set the new key as `WEBHOOK_SECRET_KEY` and give it a new
   `WEBHOOK_SECRET_KEY_ID`.
3. Move the old key to `WEBHOOK_SECRET_KEYS` (`oldId=<old hex>`).
4. Restart the merchant service. Existing `v2:<oldId>` values and legacy `v1`
   values still decrypt.
5. Legacy `v1` values are upgraded automatically on the next successful
   decrypt (test send or webhook-worker delivery): the same plaintext secret is
   re-encrypted as an AAD-bound `v2` value. The settings API also re-encrypts on
   an explicit rotation.
6. Remove the old entry only after no stored row references it. Until a legacy
   row has been read or re-saved, it remains a `v1` value with no AAD; it is
   still decryptable but does not have cross-merchant AAD binding. Treat the
   first read after an upgrade as the migration point.

Ciphertext is bound to `merchant.webhook:<merchantId>` as additional
authenticated data, so moving a ciphertext to another merchant's row fails
authentication. The plaintext secret is still returned only once at creation
and through the internal worker endpoint.

## 5. Security posture

### Cross-site requests

Every service router now rejects a state-changing request (anything except
`GET`, `HEAD`, `OPTIONS`) when the browser marks it `Sec-Fetch-Site:
cross-site`. When the request came through the gateway (signalled by the
`X-Real-IP` header Nginx always sets), an `Origin` header must match the
forwarded host and scheme. Two loopback hosts are accepted even when their
ports differ, which keeps the Next.js development proxy (`:3000` to the gateway
on `:8080`) working without weakening a public site. Non-browser clients send
neither header and are unaffected. The existing SameSite=Lax HttpOnly session
cookies and JSON-only mutation bodies remain the primary controls. A per-session
CSRF token is not used because the current request model (first-party cookies,
same-origin API, JSON content type) is covered by these checks; adding a token
without a real cross-site form flow would not add protection.

### Content Security Policy

Nginx adds:

```
default-src 'self';
script-src 'self' 'unsafe-inline';
style-src 'self' 'unsafe-inline';
img-src 'self' data: blob:;
font-src 'self';
connect-src 'self' ws: wss:;
media-src 'self';
worker-src 'self' blob:;
object-src 'none';
base-uri 'self';
form-action 'self';
frame-ancestors 'self'
```

`connect-src` includes the WebSocket schemes because CSP source matching does
not treat `ws://` as the same scheme as the page's `http://`; scripts remain
restricted to `'self'`, so this does not allow cross-origin code. `script-src 'unsafe-inline'` is required because Next injects its hydration
bootstrap inline and the theme initializer sets `data-theme` before paint;
`style-src 'unsafe-inline'` is required for React style attributes. The policy
still blocks external scripts, frames, object embeds, base-tag hijacking and
form posts to other origins. A nonce-based policy is future work because it
would make every App Router page dynamic.

Also added: `Cross-Origin-Opener-Policy: same-origin`,
`X-Permitted-Cross-Domain-Policies: none`, and a restrictive
`Permissions-Policy`.

The webhook SSRF validation (`https` only, public addresses, DNS re-check at
dial time) is already implemented in `packages/go-common/webhookurl` and is
used by `packages/webhookdelivery`.

## 6. Metrics

Every HTTP service exposes `GET /metrics` on its internal service port. Nginx
does not route `/metrics`.

| Metric                                           | Type      | Labels                |
| ------------------------------------------------ | --------- | --------------------- |
| `swiftdrop_http_requests_total`                  | counter   | method, route, status |
| `swiftdrop_http_errors_total`                    | counter   | method, route, status |
| `swiftdrop_http_request_duration_seconds`        | histogram | method, route         |
| `swiftdrop_consumer_messages_processed_total`    | counter   | queue                 |
| `swiftdrop_consumer_messages_failed_total`       | counter   | queue, class          |
| `swiftdrop_consumer_messages_retried_total`      | counter   | queue                 |
| `swiftdrop_consumer_processing_duration_seconds` | histogram | queue                 |
| `swiftdrop_realtime_connections`                 | gauge     | —                     |
| `swiftdrop_realtime_consumer_ready`              | gauge     | consumer              |
| `swiftdrop_outbox_pending_events`                | gauge     | —                     |
| `swiftdrop_outbox_failed_events`                 | gauge     | —                     |
| `swiftdrop_outbox_oldest_pending_seconds`        | gauge     | —                     |
| `swiftdrop_outbox_publishes_total`               | counter   | result                |

Routes are normalised (`/parcels/:id/history`, `/tracking/:trackingNumber`)
and probes are skipped, so cardinality stays bounded. No user ID, parcel ID, token, secret or error text is
used as a label. The consumer observer is currently wired into the realtime
consumers; other workers can opt in with the same `messaging.Observer` without
a code change.

Suggested alerts:

- `swiftdrop_outbox_pending_events > 100` for 10 minutes.
- `swiftdrop_outbox_oldest_pending_seconds > 300`.
- `swiftdrop_outbox_failed_events > 0`.
- `rate(swiftdrop_http_errors_total{status=~"5.."}[5m]) > 1`.
- `swiftdrop_realtime_consumer_ready == 0` for 2 minutes.
- `swiftdrop_consumer_messages_failed_total` increasing.

## 7. Deferred work and limitations

### Shared rate limiting and AI conversation state

Not adopted in this phase. The deployment runs one replica of each service;
introducing Redis (or a new shared PostgreSQL schema) would add an operational
dependency and a second consistency model for a student deployment that does
not need it yet. Consequences while more than one replica runs:

- Rate limits are enforced per process, so the effective limit multiplies by
  the replica count.
- AI conversations, guest sessions and concurrency limits are per process, so a
  follow-up request routed to a different replica may not see the conversation.

Before horizontal scaling, choose one of:

1. Redis with atomic sliding-window increments and a TTL-backed conversation
   store, or
2. A PostgreSQL `rate_limit` table owned by each service schema plus a shared
   conversation table, accepting the extra write load.

Both require an explicit architecture decision and a failure policy. The
current fail-closed table-full behaviour in each limiter is documented in code.

### Proof storage across replicas

The local filesystem provider requires one replica or a shared volume. For
multiple hosts, set `STORAGE_PROVIDER=s3` (or `minio`/`neon`) with the existing
bucket configuration. VersityGW remains the opt-in local S3-compatible
profile. The existing tests cover type validation, size limits, authorization,
key validation and cleanup.

### Distributed tracing and a monitoring stack

Metrics are available now. OpenTelemetry tracing, Prometheus, Grafana and
Alertmanager are not deployed because the Compose deployment has no monitoring
service budget yet. The metrics above are the agreed foundation; a later phase
can scrape them without changing service contracts. RabbitMQ queue depth is not
exported: it requires an additional management API call per service and is not
needed for the current alerts.

### Realtime fan-out

Each replicated realtime service has its own queue and in-process hub, so a
message reaches one replica. Multi-replica fan-out needs a shared pub/sub layer
or a per-replica queue design and is unchanged from Phase 1.

## 8. Final-review remediation

The independent review tightened several controls before merge:

- Removed the stale-cache authorization fallback. A cache entry older than
  `CacheTTL` is never used to authorize a request; if identity cannot refresh
  it, the request fails closed with `503`.
- An open WebSocket now closes with `1013 Try Again Later` when identity cannot
  confirm the session after the cache expires, instead of continuing to receive
  updates.
- The notification-worker ADMIN notification-log read API now performs the same
  session-version check.
- Legacy `v1` webhook ciphertext is read-repaired into AAD-bound `v2` on the
  next successful decrypt, using a compare-and-swap update so a concurrent
  rotation cannot be overwritten.
- HTTP metric route labels now normalise tracking numbers, not only UUIDs, so
  the public tracking endpoint cannot create unbounded label cardinality.
- The cross-site guard runs after request logging so rejected requests are
  observable.

## 9. Verification

Run on the Phase 2 branch:

```sh
gofmt -l apps packages tests
go vet ./...
go test ./...
go test -race ./packages/go-common/... ./apps/services/realtime/... \
  ./apps/services/parcel/... ./apps/services/pricing/... ./apps/services/merchant/...
pnpm exec nx lint @org/web
pnpm exec nx typecheck @org/web
pnpm exec nx test @org/web
pnpm exec nx build @org/web
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e
docker compose config
docker run --rm --add-host identity:127.0.0.1 ... nginx:1.27-alpine nginx -t
```

Integration suites that need PostgreSQL or RabbitMQ are covered by CI. The
local S3 storage profile (`make test-storage`) was not run because VersityGW
was not started; treat that as a gap.

### Runtime evidence from the local Compose stack

- All images rebuilt and `docker compose up -d --no-build --wait` reported
  every service healthy.
- Realtime `/ready` returned
  `{"status":"ready","checks":{"consumer.assigned":"ok","consumer.status_changed":"ok","database":"ok"}}`.
- With RabbitMQ stopped, realtime `/health` stayed `200` and `/ready` returned
  `503`; `swiftdrop_realtime_consumer_ready` was `0` for both consumers. After
  RabbitMQ restarted, readiness and both gauges recovered to `ok`/`1`.
- `GET /metrics` returned Prometheus text on identity, parcel, pricing,
  merchant, realtime, notification-worker and ai-assistant; the parcel outbox
  gauges and the realtime connection/consumer gauges were present.
- The gateway returned the CSP, `Permissions-Policy`, `COOP` and existing
  security headers. A `POST` with `Sec-Fetch-Site: cross-site` returned
  `403 CSRF_ORIGIN_MISMATCH`; the same request with a same-origin `Origin` and
  `Sec-Fetch-Site` reached the handler.
- Chromium loaded `/`, `/track` and `/design` through the gateway with the CSP
  active and no console errors or CSP violations, and opened a WebSocket to
  `/ws`.
- Session revocation was exercised against Parcel and the notification-worker
  admin read API with signed tokens: after bumping
  `identity.users.session_version`, the old token returned `401
SESSION_REVOKED`; a token with the new version returned `200`. The original
  versions were restored afterwards.
- Two legacy `v1` webhook rows were read through the Merchant internal API;
  both were stored as `v2:` afterwards, confirming the AAD-bound read-repair.
- A request to `/tracking/SDK7M2Q9XP4` produced the bounded metric label
  `route="/tracking/:trackingNumber"`.
- The full mocked Playwright Chromium suite is timing-sensitive on this host:
  one run passed 724/733 with 9 failures, and a second run passed 727/733 with a
  different set of 6 failures. Individually re-run cases in the untouched
  frontend code were mixed pass/fail (three deterministic local failures:
  `admin-merchants-actions` retry-suspension, `dashboards` customer-failed-load,
  `dashboards` one-card-failing, plus `merchant-register-form` pickup zones).
  There are no frontend changes in this phase and CI's Playwright job passes;
  these are classified as pre-existing/environment-dependent frontend test
  issues, not Phase 2 regressions. The real-stack Playwright specs were not run
  because they require seeded credentials and mutate shared state; the direct
  API/session/WS checks above cover the changed backend paths.
