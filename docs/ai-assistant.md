# SwiftDrop AI Delivery Assistant

The assistant answers SwiftDrop delivery questions and, when a question needs
live data, calls one of exactly four approved read-only business tools. Gemini
is the only model provider. The browser never calls Gemini and the API key never
leaves the `ai-assistant` container.

The integration source of truth is
[`apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md`](../apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md).
The Go service lives in
[`apps/services/ai-assistant`](../apps/services/ai-assistant) (see its README);
this document covers the frontend, gateway, Compose and verification work.

## Flow

```text
Guest/user -> chat widget (apps/web) -> /api/v1/ai/chat -> Nginx -> ai-assistant:8087
     -> optional JWT + guest session cookie
     -> scope + deterministic extraction guard
     -> Gemini + approved knowledge + approved tools -> reply -> widget
```

- Public route: `POST /api/v1/ai/chat` (Nginx strips `/api/v1`, so Gin serves
  `POST /ai/chat`).
- Health: `GET /health` and `GET /healthz`; readiness: `GET /ready` and
  `GET /readyz`, on internal port `8087`.
- The browser sends its own `sd_access` cookie automatically. Guests also receive
  an HttpOnly `sd_ai_guest` session cookie from the assistant; anonymous
  conversations are bound to it.
- Guests can use FAQs and pricing, but must not retrieve parcel status with a
  tracking number through the assistant: a tracking number is not an
  authentication credential. CUSTOMER can use the private parcel tools for
  their own parcels (including their own tracking numbers); DISPATCHER and ADMIN
  can use them for operational parcel access. MERCHANT and COURIER are denied
  private tools. The backend enforces this; the frontend never decides it. The
  separate anonymous public tracking page at `/track` remains available and
  returns only its existing allowlisted fields.

## Request and response

```json
{ "message": "What is SwiftDrop?", "conversationId": null }
```

```json
{
  "data": {
    "conversationId": "cv81a2c4e6f8091b2c3d4e5f60718293a4",
    "reply": "SwiftDrop is a parcel delivery platform...",
    "toolCalls": [{ "name": "get_public_parcel_status", "status": "OK" }]
  },
  "meta": { "requestId": "req_40", "timestamp": "2026-11-08T10:00:00Z" }
}
```

`toolCalls` is always present and `[]` when no tool ran. The widget renders only
a short, non-sensitive status label for a failed tool; it never renders tool
data. Errors use the API v1.1 envelope; the frontend maps them to its own copy
and never shows `error.message`.

## The four approved tools

| Tool                       | What it does                                                              | Access                            |
| -------------------------- | ------------------------------------------------------------------------- | --------------------------------- |
| `calculate_delivery_fee`   | Real pricing estimate from the Pricing API                                | Public                            |
| `get_public_parcel_status` | Ownership-scoped tracking by tracking number                              | CUSTOMER (own), DISPATCHER, ADMIN |
| `get_my_parcel_history`    | Parcel status history by parcel ID                                        | CUSTOMER (own), DISPATCHER, ADMIN |
| `get_my_active_deliveries` | CUSTOMER own list; DISPATCHER/ADMIN operational search (`/admin/parcels`) | CUSTOMER, DISPATCHER, ADMIN       |

No other tool can run. The model cannot choose a user ID, role or owner.
GUEST, MERCHANT and COURIER are denied all three parcel tools before any
upstream call. CUSTOMER ownership, including tracking-number lookup, is enforced
by Parcel; DISPATCHER/ADMIN access uses their verified token and Parcel's
existing staff authorization. For a customer, another customer's tracking number
returns the same not-found result as a nonexistent one, so numbers cannot be
enumerated.

## Chat panel scrolling

The panel's internal conversation container starts at the top on first open, so
the header, welcome copy and suggested questions are fully visible. Automatic
scrolling only happens after a conversation update and only while the reader is
near the bottom (or has just sent a message); scrolling up to read older messages
is never interrupted. Closing and reopening starts a fresh conversation at the
top, and signing in or out discards the previous identity's panel state.

## Local setup

1. Copy the template and set the shared secrets:

   ```sh
   cp .env.example .env
   ```

   `APP_ENV`, `JWT_ACCESS_SECRET` and (for the rest of the stack) the PostgreSQL
   settings are required by the existing services.

2. Choose a provider:

   - **Real Gemini:** put a Gemini API key in `.env`:

     ```env
     AI_PROVIDER=gemini
     GEMINI_API_KEY=your-key
     AI_MODEL=gemini-3.5-flash
     ```

   - **No paid key (deterministic mock):**

     ```env
     AI_PROVIDER=mock
     GEMINI_API_KEY=
     ```

   The mock provider is for local development, tests and demos. It is not a
   production provider and does not prove Gemini integration.

   Never use `NEXT_PUBLIC_` for `GEMINI_API_KEY`. It is read only by the
   `ai-assistant` service, never baked into the web image and never sent to the
   browser.

3. Build and start the stack:

   ```sh
   make up-build
   ```

   To rebuild only the assistant after a backend change:

   ```sh
   docker compose build ai-assistant
   docker compose up -d --no-deps ai-assistant nginx
   ```

## Verify health and routing

```sh
docker compose ps ai-assistant
docker compose exec -T nginx wget -qO- http://ai-assistant:8087/ready
curl -s http://localhost:8080/api/v1/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"What is SwiftDrop?","conversationId":null}'
```

A working stack returns `200` with `data.reply`. If the provider is disabled or
misconfigured, the service returns `503 AI_UNAVAILABLE` with the safe sentence
`The assistant is temporarily unavailable. Please try again later.` Startup
configuration problems are logged server-side and must not be exposed by the
gateway.

## Open the chatbot

The launcher is on every page (public pages included) at the bottom right. Open
it and try:

- `What is SwiftDrop?` — approved knowledge, no tool.
- `How does delivery pricing work?` — asks for a missing route, weight or size.
- `How much is 2 kg from Colombo to Kandy?` — calls the real pricing estimate.
- `Track parcel SDK7M2Q9XP4` — signed-in customer or authorized staff only; guests are asked to sign in.
- `Show my active deliveries.` — needs a signed-in CUSTOMER; DISPATCHER and ADMIN use operational parcel search.

## Environment variables

See the AI section of [`.env.example`](../.env.example) for the full list and
defaults. The important ones:

| Variable               | Default                 | Notes                                               |
| ---------------------- | ----------------------- | --------------------------------------------------- |
| `AI_ENABLED`           | `true`                  | `false` makes every chat call return 503            |
| `AI_PROVIDER`          | `gemini`                | `gemini` or `mock`                                  |
| `GEMINI_API_KEY`       | empty                   | Missing key leaves chat at 503, not startup failure |
| `AI_MODEL`             | `gemini-3.5-flash`      |                                                     |
| `AI_REQUEST_TIMEOUT`   | `15s`                   | Per provider call                                   |
| `AI_TOOL_TIMEOUT`      | `5s`                    | Per tool call                                       |
| `AI_MAX_TOOL_ROUNDS`   | `3`                     |                                                     |
| `AI_MAX_TOOL_CALLS`    | `4`                     |                                                     |
| `AI_RATE_LIMIT`        | `15`                    | Requests per rolling window per caller              |
| `AI_RATE_WINDOW`       | `1m`                    |                                                     |
| `AI_MAX_CONCURRENT`    | `3`                     | Simultaneous turns per caller                       |
| `AI_GUEST_COOKIE_NAME` | `sd_ai_guest`           | HttpOnly guest-session cookie                       |
| `FRONTEND_BASE_URL`    | `http://localhost:8080` | Allowed browser Origin for chat requests            |
| `JWT_ACCESS_SECRET`    | empty                   | Enables private tools; unset keeps them unavailable |
| `COOKIE_NAME`          | `sd_access`             | The session cookie the assistant reads              |
| `PRICING_BASE_URL`     | `http://pricing:8083`   | Compose sets this internally                        |
| `PARCEL_BASE_URL`      | `http://parcel:8082`    | Compose sets this internally                        |

For host-side Go development use `http://127.0.0.1:8083` and
`http://127.0.0.1:8082`.

## Tests

```sh
# Frontend unit tests (chat copy, error mapping, tool labels)
pnpm exec nx test @org/web

# Frontend lint, typecheck and build
pnpm exec nx lint @org/web
pnpm exec nx typecheck @org/web
pnpm exec nx build @org/web

# Assistant widget E2E, mocked API (Chromium)
pnpm exec nx e2e @org/web-e2e --args="--project=chromium"

# E2E types
pnpm exec nx typecheck @org/web-e2e

# Compose configuration
docker compose config --quiet
```

The E2E suite mocks `POST /api/v1/ai/chat`, so it never needs a Gemini key. It
covers opening/closing, the welcome and suggestions, sending, immediate composer
clearing, duplicate prevention, Unicode counting, the typing state, conversation
continuity, 422/429/503, network failure, retry without duplication, anonymous
and signed-in journeys, fresh conversation after identity change, Markdown
formatting, XSS/unsafe links, conversation expiry, malformed responses, the
phone/landscape layouts and keyboard focus.

A live-stack smoke test is optional and requires a configured provider:

```sh
curl -s -o /dev/null -w '%{http_code}\n' http://localhost:8080/api/v1/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"What is SwiftDrop?","conversationId":null}'
```

## Security notes

- The Gemini key is injected only into the `ai-assistant` container through the
  environment. It is not in the web bundle, the frontend source or Git.
- All four tools are read-only and are called with the caller's own credentials.
- Ownership is enforced by the Parcel service. Customer A receives the same
  `404` for Customer B's parcel as for a missing one.
- Guests, MERCHANT and COURIER cannot execute private tools; the backend fails
  closed before any upstream call. DISPATCHER and ADMIN operational lookups are
  audited with actor, role, tool and outcome (no message or token).
- A deterministic pre-provider guard refuses confident explicit system-prompt,
  credential, cross-user extraction and permission-bypass attempts. It never
  grants access; authorization remains in the tools and Parcel.
- Guest conversations are bound to an HttpOnly, always-Secure `sd_ai_guest`
  session; a conversation ID alone returns `404`. Authenticated requests clear the guest
  cookie, so guest history is never merged into an account.
- Browser requests with an `Origin` header must match `FRONTEND_BASE_URL`;
  mismatches return `403 FORBIDDEN` before any session or provider work.
- Assistant replies render as safe Markdown (`react-markdown`, no raw HTML,
  safe URL schemes). User messages stay plain text.
- Rate limiting (15 requests per rolling minute) and the per-caller concurrency
  cap (3) are process-local. Multi-replica deployments need coordinated edge
  enforcement; this is a known MVP limitation.
- Conversation IDs are opaque, unguessable and bound to the guest session or
  signed-in user. They are held in an in-process store and do not survive a
  restart.

## Troubleshooting

| Symptom                                   | Check                                                                 |
| ----------------------------------------- | --------------------------------------------------------------------- |
| `503 AI_UNAVAILABLE` on every message     | `AI_ENABLED`, `AI_PROVIDER`, `GEMINI_API_KEY`; service logs           |
| `401 UNAUTHENTICATED` after signing in    | The session cookie expired; sign in again. The widget does not retry  |
| `403 FORBIDDEN`                           | The browser Origin differs from `FRONTEND_BASE_URL`                   |
| Private questions say sign-in is required | Expected for anonymous callers; sign in as a customer                 |
| Tracking or pricing says unavailable      | That upstream service is down; the assistant does not invent data     |
| Nginx returns 404 for `/api/v1/ai/chat`   | Rebuild/reload Nginx after editing `infra/nginx/default.conf`         |
| Follow-up loses context                   | The in-process conversation expired or the service restarted          |
| `429 RATE_LIMITED`                        | Wait for `Retry-After`; the panel disables Send for that many seconds |

## Known limitations

- The conversation store is in-process, bounded and TTL-based (30 minutes by
  default). It is not durable and not shared across replicas.
- Rate limiting and the concurrency cap are process-local.
- The extraction guard is a supplementary heuristic, not an authorization
  control.
- There is no streaming; a reply arrives when the provider and tool loop finish.
- The mock provider is deterministic and intended for development and tests.
- No write tools, database access, MCP, vector database or arbitrary HTTP.
- The frontend, gateway, Compose service and tests target the request, response
  and error shapes in `IMPLEMENTATION_CONTRACT.md`.
