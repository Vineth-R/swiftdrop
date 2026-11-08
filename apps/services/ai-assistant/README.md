# SwiftDrop AI Delivery Assistant — backend

The Go backend for the SwiftDrop Delivery Assistant. It answers SwiftDrop domain
questions and, when live data is required, calls exactly four approved read-only
business tools against the existing Pricing and Parcel APIs.

Gemini is the only model provider, accessed through the official
[`google.golang.org/genai`](https://pkg.go.dev/google.golang.org/genai) Go SDK
(v1.77.0). The API key never leaves this service, never reaches the browser and is
never logged.

- Integration contract: [`IMPLEMENTATION_CONTRACT.md`](IMPLEMENTATION_CONTRACT.md)
- Frontend/gateway/Compose notes: [`docs/ai-assistant.md`](../../../docs/ai-assistant.md)
- Public API: `POST /api/v1/ai/chat` (Gin serves `POST /ai/chat` on port `8087`)
- Health: `GET /health`; readiness: `GET /ready`

## Architecture

```text
Next.js widget
    -> Nginx /api/v1/ai/chat
    -> ai-assistant:8087
         -> request validation + optional JWT identity
         -> rate limit (15/min per user or anonymous IP)
         -> conversation load/create
         -> business-scope guard
         -> keyword knowledge retrieval (embedded Markdown)
         -> Gemini GenerateContent (function declarations)
         -> strict tool allowlist + argument validation
         -> Pricing client / Parcel client (caller credentials only)
         -> sanitized function result
         -> Gemini final response
    -> API v1.1 success/error envelope
```

No database. The conversation store is a bounded in-process TTL map. No Redis,
MCP, embeddings, vector database, arbitrary HTTP tool or write tool is used.

## Responsibilities

Gemini decides whether a tool is needed, selects a tool, supplies arguments and
writes the natural-language answer. Go validates everything, owns identity,
authorizes private access, executes tools, bounds the loop, sanitizes results and
maps failures to safe responses. Gemini never receives an access token, a user
ID, a role, a database connection or an arbitrary URL.

### Provider seam

`internal/contract` defines `Provider`, `Tool`, `ToolRegistry`, `CallerContext`,
`GenerateRequest`/`GenerateResponse` and the tool-result statuses. Gemini types
stay inside `internal/provider/gemini`.

- `internal/provider/gemini`: real Gemini `GenerateContent` adapter. Function
  declarations are generated from the same strict parameter schemas used by the
  validator. Function calls are returned to the orchestrator; the SDK never
  executes them automatically. Gemini 3.x opaque `thought_signature` values are
  preserved from the function-call part and echoed with the next turn.
- `internal/provider/mock`: deterministic scripted provider used by tests and by
  `AI_PROVIDER=mock`. It proves the orchestration, not the Gemini API.

### Orchestration

`internal/service` validates the 500-character message limit, resolves
conversation ownership, refuses clearly out-of-scope topics, retrieves up to
three approved knowledge sections, and runs a bounded tool loop:

- `AI_MAX_TOOL_ROUNDS` (default 3) and `AI_MAX_TOOL_CALLS` (default 4). A 25 s whole-turn timeout prevents the tool loop from outliving the gateway read timeout.
- Per-provider 15 s timeout and per-tool 5 s timeout by default.
- Duplicate `(tool, canonical arguments)` calls are not re-executed; two
  consecutive duplicate rounds return a safe fallback.
- Unknown tool names are never executed and are reported to the model as
  unavailable.
- Cancellation propagates from the HTTP request.
- Tool failures are returned to the model as safe statuses, never as upstream
  bodies. If the provider also fails, the endpoint returns `503 AI_UNAVAILABLE`.

## The four approved tools

| Tool                       | Upstream                          | Access             |
| -------------------------- | --------------------------------- | ------------------ |
| `calculate_delivery_fee`   | `POST /pricing/estimate`          | Public             |
| `get_public_parcel_status` | `GET /tracking/{trackingNumber}`  | Public             |
| `get_my_parcel_history`    | `GET /parcels/{parcelId}/history` | Signed-in owner    |
| `get_my_active_deliveries` | `GET /parcels?status=...`         | Signed-in customer |

`calculate_delivery_fee` resolves the customer's location names against the
public zone directory (`GET /zones`) and then calls the Pricing estimate. It
never computes a price. Pricing remains the source of truth.

The private tools refuse anonymous callers before any HTTP request. They forward
only the caller's verified access token; they never accept a user/owner/customer
ID from the model. Parcel remains authoritative for ownership, so another
customer's parcel produces the same safe not-found result as a missing parcel.

Argument validation is strict: required keys, exact types, enum membership,
UUID/tracking formats, weight precision, and no unknown keys. Tracking output and
delivery output are re-marshalled through allowlists, so private upstream fields
cannot leak even if an upstream returns them.

## Security model

- Authentication: optional verified HS256 access token from `Authorization:
Bearer` or the `sd_access` cookie. Invalid supplied credentials return `401`;
  they are never downgraded to anonymous.
- Identity/roles are read only from verified claims, never from the model.
- Prompt injection: the system instruction marks user messages, knowledge and
  tool results as untrusted data. Tool calls are independently validated and
  authorized; the model cannot grant permissions, reveal credentials or call an
  unregistered tool.
- Knowledge and tool results are untrusted reference material, not executable
  instructions.
- Logs contain request ID, conversation ID, model, durations, tool name/status,
  token usage and error category. They never contain message bodies, access
  tokens, API keys or private parcel data.
- Rate limiting is process-local and bounded; a multi-replica deployment needs
  coordinated edge enforcement.
- Errors never include provider payloads, SQL, stack traces or credentials.

## Knowledge base

Approved Markdown lives in `internal/knowledge/data` and is embedded with
`go:embed`. Sections are split by heading and scored with lightweight keyword
matching and inverse document frequency; no embeddings or vector database.

The knowledge base deliberately does **not** contain invented content for:

- cancellation/refund policy,
- terms and conditions,
- privacy policy,
- support contact details,
- delivery-time guarantees,
- legal entity details.

Those documents explicitly tell the assistant to say that confirmed information
is unavailable. To maintain the knowledge base, edit the Markdown, add only
approved repository facts, and run `go test ./apps/services/ai-assistant/...`.

## Configuration

| Variable                       | Default               | Notes                                               |
| ------------------------------ | --------------------- | --------------------------------------------------- |
| `APP_ENV`                      | required              | `development`, `staging`, `production`              |
| `LOG_LEVEL`                    | `info`                |                                                     |
| `PORT`                         | `8087`                |                                                     |
| `HTTP_TRUSTED_PROXIES`         | empty                 | CIDR allowlist for `X-Real-IP`                      |
| `AI_ENABLED`                   | `true`                | `false` returns `503`                               |
| `AI_PROVIDER`                  | `gemini`              | `gemini`; `mock` is development-only                |
| `GEMINI_API_KEY`               | empty                 | Missing key leaves chat at 503, not startup failure |
| `AI_MODEL`                     | `gemini-3.5-flash`    | Configurable                                        |
| `AI_TEMPERATURE`               | `0.2`                 | 0–2                                                 |
| `AI_MAX_OUTPUT_TOKENS`         | `1024`                | 1–8192                                              |
| `AI_REQUEST_TIMEOUT`           | `15s`                 | Per provider call                                   |
| `AI_TOOL_TIMEOUT`              | `5s`                  | Per tool call                                       |
| `AI_TOTAL_TIMEOUT`             | `25s`                 | Whole turn; must be at least `AI_REQUEST_TIMEOUT`   |
| `AI_MAX_TOOL_ROUNDS`           | `3`                   | 1–5                                                 |
| `AI_MAX_TOOL_CALLS`            | `4`                   | 1–8                                                 |
| `AI_MAX_HISTORY_MESSAGES`      | `12`                  |                                                     |
| `AI_CONVERSATION_TTL`          | `30m`                 |                                                     |
| `AI_CONVERSATION_MAX`          | `10000`               |                                                     |
| `AI_CONVERSATION_MAX_MESSAGES` | `40`                  | ≥ history limit                                     |
| `AI_RATE_LIMIT`                | `15`                  | Per rolling window                                  |
| `AI_RATE_WINDOW`               | `1m`                  |                                                     |
| `JWT_ACCESS_SECRET`            | empty                 | Enables private tools; unset keeps them unavailable |
| `COOKIE_NAME`                  | `sd_access`           |                                                     |
| `PRICING_BASE_URL`             | `http://pricing:8083` | HTTP(S) origin                                      |
| `PRICING_TIMEOUT`              | `3s`                  |                                                     |
| `PARCEL_BASE_URL`              | `http://parcel:8082`  | HTTP(S) origin                                      |
| `PARCEL_TIMEOUT`               | `3s`                  |                                                     |

A missing `GEMINI_API_KEY` does not stop the process or the gateway: the
service starts with an unconfigured provider, `/ready` reports `unconfigured`,
and chat returns `503 AI_UNAVAILABLE` until a key is set. `AI_PROVIDER=mock` is
development-only. The full
local template is the AI section of `.env.example`.

## API behavior

Success `200`:

```json
{
  "data": {
    "conversationId": "cv81a2c4e6f8091b2c3d4e5f60718293a4",
    "reply": "Your parcel is currently in transit.",
    "toolCalls": [{ "name": "get_public_parcel_status", "status": "OK" }]
  },
  "meta": { "requestId": "req_40", "timestamp": "2026-11-08T10:00:00Z" }
}
```

Errors: `400 BAD_REQUEST`, `401 UNAUTHENTICATED`/`TOKEN_EXPIRED`, `404
CONVERSATION_NOT_FOUND`, `413 PAYLOAD_TOO_LARGE`, `422 VALIDATION_ERROR`, `429
RATE_LIMITED` with `Retry-After`, `503 AI_UNAVAILABLE`, `500 INTERNAL_ERROR`.
Tool-call statuses are `OK`, `INVALID`, `DENIED` and `UNAVAILABLE`.

Conversation IDs are `cv` plus 32 lowercase hex characters. Anonymous
conversations and signed-in conversations are isolated; cross-owner IDs return
`404`. The store is not durable across restarts.

## Running locally

```sh
# Deterministic provider, no paid key:
APP_ENV=development AI_PROVIDER=mock PORT=8087 \
  PRICING_BASE_URL=http://127.0.0.1:8083 \
  PARCEL_BASE_URL=http://127.0.0.1:8082 \
  go run ./apps/services/ai-assistant

curl -s http://127.0.0.1:8087/ai/chat \
  -H 'Content-Type: application/json' \
  -d '{"message":"What is SwiftDrop?","conversationId":null}'
```

For the full stack, use the Compose service that Session 2 added
(`SERVICE=ai-assistant`, internal port `8087`, Nginx `/api/v1/ai/chat`).

## Tests

```sh
go test ./apps/services/ai-assistant/...
go test -race ./apps/services/ai-assistant/...
go vet ./apps/services/ai-assistant/...
pnpm exec nx test ai-assistant
pnpm exec nx lint ai-assistant
```

Coverage includes message validation, conversation ownership/expiry/truncation,
rate limiting, optional authentication, provider failure/timeout/empty response,
tool loop success, unknown tools, strict arguments, private-tool denial before
upstream, cross-customer safety, identity override rejection, public-tracking
sanitization, pricing non-fabrication, upstream unavailability, scope refusal,
knowledge retrieval and the final API envelope.

The Gemini adapter tests inject the SDK call, so CI needs no paid Gemini key and
no network. A live Gemini call is not part of automated CI; verify it manually
with a real `GEMINI_API_KEY` if required.

## Known limitations

- Conversation history and rate limits are in-process; they do not survive a
  restart and are not shared between replicas.
- No streaming; the reply arrives after the provider/tool loop completes.
- The knowledge base uses keyword retrieval, not semantic search.
- The mock provider is for development and tests and does not prove Gemini.
- No write tools, arbitrary HTTP, MCP, database access, embeddings or vector
  database.
- Live Gemini model names and quotas can change; `AI_MODEL` is configurable and
  the default is verified against the SDK at implementation time.
