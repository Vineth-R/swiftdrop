# SwiftDrop AI Delivery Assistant — Implementation Contract

Status: Session 1 backend contract. Session 2 (frontend, Compose, Nginx, E2E) must treat this file as the integration source of truth.

Owner: `apps/services/ai-assistant` (Go backend). Session 2 owns `apps/web`, `docker-compose.yml`, `infra/nginx/default.conf`, and end-to-end tests. Do not silently change this contract; propose a change and update this file first.

## 1. Scope

The assistant answers SwiftDrop domain questions and, when needed, calls exactly four approved read-only business tools that call existing SwiftDrop APIs with the caller's own verified credentials. It is public: guests receive an HttpOnly server-generated session, while signed-in callers keep their verified identity and role.

- Provider: Google Gemini only, through the official `google.golang.org/genai` Go SDK.
- No database access from the model.
- No write tools.
- No arbitrary HTTP, no MCP, no embeddings, no vector database, no Redis.
- The service is stateless apart from an in-process conversation store.

## 2. Service, port and routing

| Item           | Value                                                          |
| -------------- | -------------------------------------------------------------- |
| Service name   | `ai-assistant`                                                 |
| Default port   | `8087`                                                         |
| Public route   | `POST /api/v1/ai/chat`                                         |
| Gin route      | `POST /ai/chat`                                                |
| Health         | `GET /health` (also `/healthz`), `GET /ready` (also `/readyz`) |
| Nginx upstream | `ai_assistant_upstream { server ai-assistant:8087; }`          |

Session 2 owns the Nginx and Compose integration. The current stack proxies `/api/v1/ai/chat` to `ai-assistant:8087` and injects the environment variables in section 11. Session 1 owns the Go service; infrastructure changes are coordinated with Session 2.

The Go Docker build uses the existing generic `infra/docker/go.Dockerfile` with `SERVICE=ai-assistant`. The knowledge base is compiled into the binary with `go:embed`, so no extra runtime files are required.

## 3. Public API

### 3.1 `POST /api/v1/ai/chat`

Access: public, rate limited. Authentication is optional. Anonymous callers receive an HttpOnly, always-Secure `sd_ai_guest` session cookie (`gs` + 32 lowercase hex) that is bound to their conversations. A conversation ID alone never grants access. Local development must use localhost, 127.0.0.1 or HTTPS, which browsers treat as secure contexts for Secure cookies.

- If a valid `Authorization: Bearer <token>` or `sd_access` cookie is present, the caller is authenticated; private tools run only when the verified role is CUSTOMER, DISPATCHER or ADMIN.
- If no credentials are present, the caller is anonymous and only the public pricing tool may be used. All parcel tools, including tracking-number lookup, are denied before any upstream call.
- If credentials are present but `JWT_ACCESS_SECRET` is not configured, the request fails closed with `503 SERVICE_UNAVAILABLE`; it is never downgraded to anonymous.
- If credentials are present but invalid or expired, the request is rejected with `401 UNAUTHENTICATED` / `401 TOKEN_EXPIRED`. It is never silently downgraded to anonymous.

Request:

```json
{
  "message": "Where is my parcel?",
  "conversationId": null
}
```

Rules:

- `message` is required after trimming.
- Maximum 500 Unicode characters. A 500-character message is accepted; 501 is rejected.
- `conversationId` is optional. When provided it must match `^cv[0-9a-f]{32}$`.
- Unknown JSON fields are rejected.
- The request body is limited to 1 MiB.
- The request content type must be `application/json`.

Success `200`:

```json
{
  "data": {
    "conversationId": "cv81a2c4e6f8091b2c3d4e5f60718293a4",
    "reply": "Your parcel is currently in transit.",
    "toolCalls": [
      {
        "name": "get_public_parcel_status",
        "status": "OK"
      }
    ]
  },
  "meta": {
    "requestId": "req_40",
    "timestamp": "2026-11-08T10:00:00Z"
  }
}
```

`toolCalls` is always present and is `[]` when no tool ran. `status` is one of `OK`, `INVALID`, `DENIED`, `UNAVAILABLE`.

Errors use the existing API v1.1 envelope:

```json
{
  "error": {
    "code": "VALIDATION_ERROR",
    "message": "one or more fields are invalid",
    "details": [{ "field": "message", "issue": "..." }]
  },
  "meta": { "requestId": "req_40" }
}
```

| Status | Code                     | When                                                                               |
| ------ | ------------------------ | ---------------------------------------------------------------------------------- |
| 400    | `BAD_REQUEST`            | Malformed JSON, wrong content type, unknown fields, multiple objects               |
| 401    | `UNAUTHENTICATED`        | Credentials were supplied but are invalid                                          |
| 401    | `TOKEN_EXPIRED`          | Credentials were supplied but the access token expired                             |
| 404    | `CONVERSATION_NOT_FOUND` | Conversation ID is unknown, expired, or owned by someone else                      |
| 413    | `PAYLOAD_TOO_LARGE`      | Body exceeds 1 MiB                                                                 |
| 422    | `VALIDATION_ERROR`       | Empty/whitespace message, >500 characters, or malformed conversation ID            |
| 403    | `FORBIDDEN`              | The request Origin does not match `FRONTEND_BASE_URL`                              |
| 429    | `RATE_LIMITED`           | More than 15 requests per rolling minute for this caller, or the concurrency cap   |
| 503    | `AI_UNAVAILABLE`         | Assistant disabled/unconfigured, provider failure/timeout, or empty model response |
| 503    | `SERVICE_UNAVAILABLE`    | Credentials were supplied but `JWT_ACCESS_SECRET` is not configured                |

When rate limited, the response includes `Retry-After` and `X-RateLimit-Remaining: 0`. Successful responses include `X-RateLimit-Remaining`.

`503 AI_UNAVAILABLE` uses the safe message `The assistant is temporarily unavailable. Please try again later.` It never includes provider internals.

Dependent-service failures (Pricing/Parcel) are returned to the model as tool results with status `UNAVAILABLE`; the assistant explains the failure in natural language. If the model then cannot answer, the endpoint returns `503 AI_UNAVAILABLE`. The endpoint never fabricates a price, tracking status or history.

## 4. Rate limiting

- 15 requests per rolling minute per authenticated user ID.
- 15 requests per rolling minute per client IP for anonymous callers. Rotating the guest cookie does not reset the IP budget.
- Each caller may have at most `AI_MAX_CONCURRENT` (default 3) turns in flight. Excess concurrent turns return `429 RATE_LIMITED` with `Retry-After`.
- Invalid, oversized and out-of-scope requests never reach the provider.
- Process-local and bounded (4096 keys), matching the existing shared limiter's fail-closed behaviour.
- Multi-replica deployments must add coordinated edge enforcement; this is documented as a limitation.

## 5. Authentication and identity

- Identity comes only from verified JWT access-token claims (`sub`, `role`) using `packages/go-common/auth`.
- The Go service never accepts `userId`, `customerId`, `ownerId`, `role`, or `permissions` from the model.
- The caller's raw access token is forwarded to Parcel only for private tools and is never sent to Gemini or logged.
- Private tools fail closed with `DENIED` before any HTTP call for guests, MERCHANT and COURIER.
- CUSTOMER private reads use `GET /parcels` and `GET /parcels/{parcelId}/history`; ownership is enforced by Parcel.
- DISPATCHER and ADMIN use `GET /admin/parcels` for operational lists and `GET /parcels/{parcelId}/history` for history. ADMIN inherits DISPATCHER permissions; the caller's verified token is forwarded so Parcel remains authoritative.
- Parcel remains authoritative for ownership. Customer A receives the same `404` for Customer B's parcel; the assistant reports that the parcel could not be found.

## 6. Provider interface

Defined in `internal/contract`:

```go
type Provider interface {
    Generate(ctx context.Context, request GenerateRequest) (GenerateResponse, error)
}

type GenerateRequest struct {
    SystemInstruction string
    Contents          []Content
    Tools             []ToolDeclaration
    Temperature       float32
    MaxOutputTokens   int32
}

type Content struct {
    Role  string // "user" | "model"
    Parts []Part
}

type Part struct {
    Text             string
    FunctionCall     *FunctionCall
    FunctionResponse *FunctionResponse
}

type FunctionCall struct {
    ID        string
    Name      string
    Arguments map[string]any
}

type FunctionResponse struct {
    ID       string
    Name     string
    Response map[string]any
}

type GenerateResponse struct {
    Text      string
    ToolCalls []FunctionCall
    Usage     TokenUsage
}
```

Provider errors are classified as `Unavailable` or `Timeout`; details are logged server-side only.

Provider packages: `internal/provider/gemini` (real) and `internal/provider/mock` (tests and local development). Gemini-specific types must not leave `internal/provider/gemini`.

Verified SDK and model:

- SDK: `google.golang.org/genai` v1.77.0 (official Google Gen AI Go SDK).
- API: `client.Models.GenerateContent(ctx, model, contents, config)` with `GenerateContentConfig.Tools`.
- Default model: `gemini-3.5-flash`. `AI_MODEL` overrides it.
- Function-call results are submitted back as `FunctionCall` (role `model`) and `FunctionResponse` (role `user`) content parts.
- Gemini 3.x function calls carry an opaque `thought_signature` on the part; the provider preserves it in `contract.FunctionCall.ThoughtSignature` and echoes it on the next request. Dropping it makes the API reject the tool round with `400 INVALID_ARGUMENT`.
- No automatic/SDK-side function execution is used; the orchestration loop executes tools explicitly.

## 7. Tool interface

Defined in `internal/contract` and `internal/tools`:

```go
type ToolDefinition struct {
    Name        string
    Description string
    Parameters  Parameters
}

type Tool interface {
    Definition() ToolDefinition
    Execute(ctx context.Context, args map[string]any, caller CallerContext) (ToolResult, error)
}

type ToolRegistry interface {
    Get(name string) (Tool, bool)
    List() []ToolDefinition
    Execute(ctx context.Context, name string, args map[string]any, caller CallerContext) (ToolResult, error)
}
```

`CallerContext` is populated only by trusted middleware:

```go
type CallerContext struct {
    UserID        string
    Role          string
    Authenticated bool
    AccessToken   string
    // SessionID is the server-issued guest session for anonymous callers.
    // It is set only by the guest-session middleware and is empty when the
    // caller is authenticated.
    SessionID     string
}
```

Argument validation is strict: required keys, exact types, enum membership, UUID/format checks, no unknown keys. The model cannot add identity fields; any unknown argument rejects the call.

## 8. The four approved tools

| Tool                       | Upstream call                                                                    | Access                                     |
| -------------------------- | -------------------------------------------------------------------------------- | ------------------------------------------ |
| `calculate_delivery_fee`   | `POST {PRICING}/pricing/estimate`                                                | Public                                     |
| `get_public_parcel_status` | `GET {PARCEL}/parcels/tracking/{trackingNumber}`                                 | CUSTOMER own; DISPATCHER/ADMIN operational |
| `get_my_parcel_history`    | `GET {PARCEL}/parcels/{parcelId}/history`                                        | CUSTOMER, DISPATCHER, ADMIN                |
| `get_my_active_deliveries` | CUSTOMER: `GET {PARCEL}/parcels`; DISPATCHER/ADMIN: `GET {PARCEL}/admin/parcels` | CUSTOMER, DISPATCHER, ADMIN                |

### 8.1 `calculate_delivery_fee`

Arguments:

- `origin` (string, required): zone name or district, for example `Colombo`.
- `destination` (string, required): zone name or district, for example `Kandy`.
- `weightKg` (number, required): positive, at most two decimal places.
- `sizeCategory` (string enum, required): `SMALL`, `MEDIUM`, `LARGE`, `OVERSIZED`.

The client resolves `origin`/`destination` against the public `GET {PRICING}/zones` directory (case-insensitive match on zone name, code, or district) and then calls the real estimate endpoint. The tool never calculates a price. Result returns only: `currency`, `baseFee`, `weightCharge`, `sizeSurcharge`, `total`, `origin`, `destination`, `weightKg`, `sizeCategory`. Unknown/unserviceable routes and unpriced weights return a safe `INVALID` result explaining that no quote is available.

### 8.2 `get_public_parcel_status`

Arguments: `trackingNumber` (string, required, `^SD[KN][A-Z0-9]{8}$`).

Allowed roles: CUSTOMER (own parcels only), DISPATCHER and ADMIN (operational access). GUEST, MERCHANT and COURIER are denied before any HTTP call. The client forwards the caller's access token to `GET {PARCEL}/parcels/tracking/{trackingNumber}`; Parcel matches `customer_user_id` for customers and allows staff operationally. The assistant never calls the anonymous public tracking projection, and a tracking number is never treated as an identity credential. Another customer's tracking number and a nonexistent one return the same safe `INVALID` not-found result.

Result allowlist: `trackingNumber`, `status`, `pickupCity`, `dropoffCity`, `lastUpdatedAt`, `timeline` (`status`, `at`). No names, addresses, phones, notes, proof, or internal IDs.

### 8.3 `get_my_parcel_history`

Arguments: `parcelId` (string, required, UUID).

Allowed roles: CUSTOMER (own parcels only), DISPATCHER and ADMIN (operational access). GUEST, MERCHANT and COURIER are denied before any HTTP call. The model cannot supply an owner/user ID. The client forwards the caller's access token and Parcel enforces ownership/staff authorization. Result allowlist: `previousStatus`, `status`, `changedAt`, `note`. `failureReason` and `actorRole` are excluded.

### 8.4 `get_my_active_deliveries`

Arguments: `status` (string enum, optional): one of `CREATED`, `PICKED_UP`, `IN_TRANSIT`, `OUT_FOR_DELIVERY`, `DELIVERED`, `FAILED`.

CUSTOMER calls `GET {PARCEL}/parcels?page=1&pageSize=20` with the optional status filter and the caller's token. DISPATCHER and ADMIN call `GET {PARCEL}/admin/parcels?page=1&pageSize=20` with the same filter and their own token. Result scope is `customer` or `operations`.

- Customer allowlist: `id`, `trackingNumber`, `status`, `dropoffCity`, `recipientName`, `total`, `currency`, `createdAt`, `active`.
- Operational allowlist: `id`, `trackingNumber`, `status`, `pickupCity`, `dropoffCity`, `courierId`, `createdAt`, `active`.
- Both include `count`, `activeCount` and `totalItems`. Addresses, phone numbers, customer IDs, merchant IDs and internal metadata are excluded.

There are no other tools. Unknown tool names are never executed.

## 9. Orchestration

1. Validate request and load/create the conversation.
2. Enforce business scope (SwiftDrop only) and the deterministic extraction/permission guard.
3. Retrieve a bounded set of approved knowledge sections.
4. Build the system instruction (persona, scope, security, knowledge, tool rules).
5. Call Gemini; execute approved tools in a bounded loop.
6. Return the final natural-language reply and the executed tool summaries.

Limits (all configurable):

- `AI_MAX_TOOL_ROUNDS` (default 3): maximum rounds of tool execution.
- `AI_MAX_TOOL_CALLS` (default 4): maximum unique tool executions per request.
- `AI_REQUEST_TIMEOUT` (default 15s): each provider call.
- `AI_TOOL_TIMEOUT` (default 5s): each tool execution.
- `AI_TOTAL_TIMEOUT` (default 25s): the whole turn, so the tool loop cannot outlive the gateway read timeout.
- `AI_MAX_HISTORY_MESSAGES` (default 12): text turns sent to the provider.
- Duplicate `(tool, canonical-arguments)` calls are not re-executed; the previous result is returned to the model.
- Cancellation from the HTTP request propagates to provider and tool calls.

## 10. Conversation model

- New conversation ID format: `cv` + 32 lowercase hex characters.
- Anonymous conversations are bound to a server-generated guest session ID (`gs` + 32 lowercase hex) delivered in the HttpOnly `sd_ai_guest` cookie. Authenticated conversations are bound to the verified user ID.
- A guest must present both the conversation ID and the same guest session cookie. A conversation ID alone returns `404 CONVERSATION_NOT_FOUND`.
- Anonymous callers cannot continue authenticated conversations, authenticated users cannot continue anonymous conversations, and guests cannot continue another guest's conversation (all return `404`).
- Authenticated requests clear the guest cookie. Guest history is never merged into an account and does not reappear after logout.
- Storage is an in-process, bounded, TTL store (default TTL 30 minutes, 10 000 conversations, 40 stored messages). It is not durable across restarts and not shared across replicas. This is an accepted MVP limitation; Session 2 must not assume persistence.
- Provider context keeps the most recent `AI_MAX_HISTORY_MESSAGES` turns; older turns are truncated.

## 11. Configuration

| Variable                       | Default                 | Notes                                                              |
| ------------------------------ | ----------------------- | ------------------------------------------------------------------ |
| `APP_ENV`                      | required                | `development`, `staging`, `production`                             |
| `LOG_LEVEL`                    | `info`                  |                                                                    |
| `PORT`                         | `8087`                  |                                                                    |
| `HTTP_TRUSTED_PROXIES`         | empty                   | CIDR allowlist for forwarded client IPs                            |
| `AI_ENABLED`                   | `true`                  | `false` returns `503 AI_UNAVAILABLE`                               |
| `AI_PROVIDER`                  | `gemini`                | `gemini`; `mock` is development-only                               |
| `GEMINI_API_KEY`               | empty                   | Missing key leaves chat at 503; it does not fail startup           |
| `AI_MODEL`                     | `gemini-3.5-flash`      | Verified default                                                   |
| `AI_TEMPERATURE`               | `0.2`                   | 0–2                                                                |
| `AI_MAX_OUTPUT_TOKENS`         | `1024`                  |                                                                    |
| `AI_REQUEST_TIMEOUT`           | `15s`                   |                                                                    |
| `AI_TOOL_TIMEOUT`              | `5s`                    |                                                                    |
| `AI_TOTAL_TIMEOUT`             | `25s`                   | Whole turn; must be at least `AI_REQUEST_TIMEOUT`                  |
| `AI_MAX_TOOL_ROUNDS`           | `3`                     |                                                                    |
| `AI_MAX_TOOL_CALLS`            | `4`                     |                                                                    |
| `AI_MAX_HISTORY_MESSAGES`      | `12`                    |                                                                    |
| `AI_CONVERSATION_TTL`          | `30m`                   |                                                                    |
| `AI_CONVERSATION_MAX`          | `10000`                 |                                                                    |
| `AI_CONVERSATION_MAX_MESSAGES` | `40`                    |                                                                    |
| `AI_RATE_LIMIT`                | `15`                    | requests per window                                                |
| `AI_RATE_WINDOW`               | `1m`                    |                                                                    |
| `AI_MAX_CONCURRENT`            | `3`                     | simultaneous turns per user/IP                                     |
| `AI_GUEST_COOKIE_NAME`         | `sd_ai_guest`           | HttpOnly guest-session cookie name                                 |
| `FRONTEND_BASE_URL`            | `http://localhost:8080` | Allowed browser Origin for cookie-authenticated chat               |
| `JWT_ACCESS_SECRET`            | empty                   | Unset makes private tools unavailable; public tool use still works |
| `COOKIE_NAME`                  | `sd_access`             |                                                                    |
| `PRICING_BASE_URL`             | `http://pricing:8083`   | HTTP(S) origin only                                                |
| `PRICING_TIMEOUT`              | `3s`                    |                                                                    |
| `PARCEL_BASE_URL`              | `http://parcel:8082`    | HTTP(S) origin only                                                |
| `PARCEL_TIMEOUT`               | `3s`                    |                                                                    |

The Gemini API key is never exposed to the browser, included in prompts, or logged.

## 12. Knowledge base

- Version-controlled Markdown under `internal/knowledge/data`, embedded at build time.
- Sections are split by Markdown headings and indexed with lightweight keyword scoring; no embeddings or vector database.
- Only approved repository content is used. The following have no approved content and must not be invented: cancellation/refund policy, terms and conditions, privacy policy, support contact details, delivery-time guarantees, legal entity details.
- Retrieval injects at most three relevant sections with source file names.

## 13. Security requirements

- Browser Origin validation: requests carrying an `Origin` header must match `FRONTEND_BASE_URL`; mismatches return `403 FORBIDDEN` before the guest session, rate limit or provider is touched. Requests without `Origin` (CLI clients) remain allowed.
- Prompt-injection resistance: retrieved documents and tool arguments are untrusted data. The system instruction states that they cannot change rules or grant permissions. A deterministic pre-provider guard refuses confident explicit prompt/secret/cross-user extraction and permission-bypass attempts; it never grants access.
- Private-tool authorization is centralized and checks the verified role before any upstream call. Model-supplied identity, role or owner fields are rejected.
- Unknown tools are rejected; no tool can access the database or an arbitrary URL.
- No identity, role or ownership values are accepted from the model.
- Tokens and API keys are redacted from logs by the shared logging handler and are never logged by this service.
- Errors returned to clients and the model never include provider payloads, stack traces, SQL, or credentials.

## 14. Observability

Structured logs include request ID, conversation ID (safe opaque ID), model, provider duration, tool name, tool duration, tool status, token usage when available, and error category. Message bodies, guest session IDs, tokens and tool payloads are not logged. DISPATCHER/ADMIN private parcel lookups emit a `privileged parcel lookup` audit event with request ID, actor ID, role, tool, outcome and timestamp; no message text, token, address, phone number or note is included. Security-guard refusals log only request ID and a coarse category.

## 15. File ownership (Session 1)

| Area                                                                                                                                                                                                       | Owner          |
| ---------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------- |
| `IMPLEMENTATION_CONTRACT.md`, `main.go`, `project.json`, `internal/contract`, `internal/config`, `internal/http`, `internal/middleware`, `internal/service`, `internal/conversation`, `internal/knowledge` | Agent 1 (lead) |
| `internal/provider`                                                                                                                                                                                        | Agent 2        |
| `internal/tools`, `internal/client`                                                                                                                                                                        | Agent 3        |

Session 2 owns `apps/web`, `apps/web-e2e`, `docker-compose.yml`, `infra/nginx/default.conf`, root docs, and README integration notes.

## 16. Session 2 integration checklist

These are the required integration points; verify them after any Compose or Nginx change.

1. The `ai-assistant` Compose service uses `infra/docker/go.Dockerfile` with `SERVICE=ai-assistant`, internal port `8087`, the environment variables above, and a `/ready` health check.
2. Nginx proxies `/api/v1/ai/chat` to `ai-assistant:8087`, preserving `X-Request-Id`, `X-Real-IP`, and the cookie header.
3. Build the chat UI against `POST /api/v1/ai/chat` and the exact response/error shapes in section 3.
4. Send `credentials: 'include'` (or same-origin cookies) so private tools can run for signed-in users.
5. Treat `404 CONVERSATION_NOT_FOUND` as "start a new conversation" in the UI.
6. Show `toolCalls` only as non-sensitive progress labels; never render raw tool data.
7. Add E2E coverage with a mocked chat endpoint, plus an optional live-stack check when `GEMINI_API_KEY` is configured.
8. Render assistant Markdown with `react-markdown` only (no raw HTML) and keep user messages as plain text.
9. Reset the chat panel when the signed-in identity changes.
