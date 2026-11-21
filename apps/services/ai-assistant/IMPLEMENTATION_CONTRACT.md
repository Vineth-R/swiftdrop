# SwiftDrop AI Delivery Assistant — Implementation Contract

Status: Session 1 backend contract. Session 2 (frontend, Compose, Nginx, E2E) must treat this file as the integration source of truth.

Owner: `apps/services/ai-assistant` (Go backend). Session 2 owns `apps/web`, `docker-compose.yml`, `infra/nginx/default.conf`, and end-to-end tests. Do not silently change this contract; propose a change and update this file first.

## 1. Scope

The assistant answers SwiftDrop domain questions and, when needed, calls one of exactly twelve approved read-only business tools that call existing SwiftDrop APIs with the caller's own verified credentials. Each tool is exposed only to the roles in the matrix in section 8, and every execution re-checks the verified role. It is public: guests receive an HttpOnly server-generated session, while signed-in callers keep their verified identity and role.

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

- If a valid `Authorization: Bearer <token>` or `sd_access` cookie is present, the caller is authenticated and keeps the tools for the verified role: CUSTOMER, COURIER, MERCHANT, DISPATCHER or ADMIN.
- If no credentials are present, the caller is anonymous and only the public pricing estimate and the anonymous public tracking projection are available. Every private tool is denied before any upstream call.
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
- The caller's raw access token is forwarded to Parcel and Merchant only for private tools and is never sent to Gemini or logged.
- Guest tools are limited to the pricing estimate and the anonymous public tracking projection. Every private tool fails closed with `DENIED` before any HTTP call for guests and for roles not in its matrix.
- CUSTOMER reads use `GET /parcels`, `GET /parcels/{parcelId}`, `GET /parcels/{parcelId}/history` and `GET /parcels/tracking/{trackingNumber}`; ownership is enforced by Parcel.
- DISPATCHER and ADMIN use `GET /admin/parcels`, `GET /dispatch/unassigned`, `GET /dispatch/workload` and the parcel reads. ADMIN inherits DISPATCHER permissions; the caller's verified token is forwarded so Parcel remains authoritative.
- COURIER reads use `GET /courier/deliveries` and `GET /courier/deliveries/{parcelId}`; Parcel derives the courier from the token and enforces the current assignment.
- MERCHANT reads use `GET /merchant/orders` and `GET /merchants/me/webhook/deliveries`; Merchant resolves the merchant from the verified session and enforces merchant scoping.
- Parcel and Merchant remain authoritative for ownership. Customer A receives the same `404` for Customer B's parcel; another merchant's order is indistinguishable from a missing one.

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

## 8. The twelve approved tools

The registry contains exactly these tools. `List(caller)` returns only the tools
available to the caller; `Execute` independently re-checks the same role before
any HTTP call, so a model that names a hidden tool receives `DENIED` with zero
upstream traffic. ADMIN inherits DISPATCHER permissions through the shared
`isStaffRole` check.

| #   | Tool                        | Upstream call                                                                                                |   GUEST   | CUSTOMER | COURIER | MERCHANT | DISPATCHER / ADMIN |
| --- | --------------------------- | ------------------------------------------------------------------------------------------------------------ | :-------: | :------: | :-----: | :------: | :----------------: |
| 1   | `calculate_delivery_fee`    | `POST {PRICING}/pricing/estimate` after `GET {PRICING}/zones`                                                |    ✅     |    ✅    |   ✅    |    ✅    |         ✅         |
| 2   | `get_public_parcel_status`  | GUEST: `GET {PARCEL}/tracking/{trackingNumber}`; signed-in: `GET {PARCEL}/parcels/tracking/{trackingNumber}` | ✅ public |  ✅ own  |    –    |    –     |         ✅         |
| 3   | `get_parcel_history`        | `GET {PARCEL}/parcels/{parcelId}/history`                                                                    |     –     |    ✅    |    –    |    –     |         ✅         |
| 4   | `get_active_parcels`        | CUSTOMER: `GET {PARCEL}/parcels`; staff: `GET {PARCEL}/admin/parcels`                                        |     –     |    ✅    |    –    |    –     |         ✅         |
| 5   | `get_parcel_details`        | `GET {PARCEL}/parcels/{parcelId}`                                                                            |     –     |  ✅ own  |    –    |    –     |         ✅         |
| 6   | `search_parcels`            | `GET {PARCEL}/admin/parcels`                                                                                 |     –     |    –     |    –    |    –     |         ✅         |
| 7   | `list_unassigned_parcels`   | `GET {PARCEL}/dispatch/unassigned`                                                                           |     –     |    –     |    –    |    –     |         ✅         |
| 8   | `get_courier_workload`      | `GET {PARCEL}/dispatch/workload`                                                                             |     –     |    –     |    –    |    –     |         ✅         |
| 9   | `get_my_courier_deliveries` | `GET {PARCEL}/courier/deliveries`                                                                            |     –     |    –     |   ✅    |    –     |         –          |
| 10  | `get_my_courier_delivery`   | `GET {PARCEL}/courier/deliveries/{parcelId}`                                                                 |     –     |    –     |   ✅    |    –     |         –          |
| 11  | `get_my_merchant_orders`    | `GET {MERCHANT}/merchant/orders`                                                                             |     –     |    –     |    –    |    ✅    |         –          |
| 12  | `get_my_webhook_deliveries` | `GET {MERCHANT}/merchants/me/webhook/deliveries`                                                             |     –     |    –     |    –    |    ✅    |         –          |

There are no other tools. Unknown tool names are never executed. Argument
validation is strict (required keys, exact types, enum membership, date format,
UUID/tracking format, no unknown keys); the model cannot add identity fields.

### 8.1 `calculate_delivery_fee`

Arguments: `origin`, `destination` (zone/code/district), `weightKg` (positive, at
most two decimals), `sizeCategory` (`SMALL|MEDIUM|LARGE|OVERSIZED`). The client
resolves zones against the public directory and calls the real estimate. Result
allowlist: `currency`, `baseFee`, `weightCharge`, `sizeSurcharge`, `total`,
`origin`, `destination`, `weightKg`, `sizeCategory`. No price is ever calculated
locally.

### 8.2 `get_public_parcel_status`

Argument: `trackingNumber` (`^SD[KN][A-Z0-9]{8}$`). Guests use the anonymous
`/tracking/{trackingNumber}` projection with no credentials; their result has
`scope: "public"`. CUSTOMER, DISPATCHER and ADMIN use the ownership-scoped
`/parcels/tracking/{trackingNumber}` endpoint with their own token and
`scope: "account"`. MERCHANT and COURIER are denied before any HTTP call. A
tracking number is never an authentication credential. Result allowlist:
`trackingNumber`, `status`, `pickupCity`, `dropoffCity`, `lastUpdatedAt`,
`timeline` (`status`, `at`), `scope`.

### 8.3 `get_parcel_history`

Argument: `parcelId` (UUID). CUSTOMER (own), DISPATCHER and ADMIN. The caller's
token is forwarded and Parcel enforces ownership. Result allowlist:
`previousStatus`, `status`, `changedAt`, `note`.

### 8.4 `get_active_parcels`

Optional `status` enum. CUSTOMER calls `/parcels`; staff call `/admin/parcels`.
Result scope is `customer` or `operations`; customer rows include
`id`, `trackingNumber`, `status`, `dropoffCity`, `recipientName`, `total`,
`currency`, `createdAt`, `active`; operational rows include `id`,
`trackingNumber`, `status`, `pickupCity`, `dropoffCity`, `courierId`,
`createdAt`, `active`. Addresses, phone numbers and customer/merchant IDs are
excluded.

### 8.5 `get_parcel_details`

Argument: `parcelId` (UUID). CUSTOMER (own), DISPATCHER and ADMIN via
`GET /parcels/{parcelId}`. Result allowlist: `id`, `trackingNumber`, `status`,
`pickupCity`, `dropoffCity`, `weightKg`, `sizeCategory`, `total`, `currency`,
`courierName` (no phone), `failureReason`, `createdAt`, `updatedAt`,
`deliveredAt`. Sender/recipient contacts, addresses, phones, proof data and
internal metadata are excluded.

### 8.6 `search_parcels`

DISPATCHER/ADMIN. Optional `status` enum, `trackingNumber` (prefix, letters and
digits), `from` and `to` (`YYYY-MM-DD`, UTC creation bounds). The client converts
them to the RFC3339 bounds the Parcel API expects. Returns at most the first 20
matches plus `totalItems` and `hasMore`; the result includes `periodNote` so the
model states the date scope. Addresses and customer/merchant IDs are excluded.

### 8.7 `list_unassigned_parcels`

DISPATCHER/ADMIN. No arguments. Returns the first page (at most 20) of parcels
with no current courier, with `id`, `trackingNumber`, `status`, cities, weight,
size and creation time. Street addresses and zone IDs are excluded. Read-only.

### 8.8 `get_courier_workload`

DISPATCHER/ADMIN. No arguments. Returns up to 50 active couriers sorted by
`activeDeliveries` descending, then name. Allowlist: `courierId`, `fullName`,
`isAvailable`, `activeDeliveries`, `byStatus`. Phone numbers are excluded.

### 8.9 `get_my_courier_deliveries`

COURIER only. Optional `status` enum and `date` (`YYYY-MM-DD`, default today in
Asia/Colombo). The Parcel API derives the courier from the forwarded token, so
the model cannot select another courier. The upstream set is currently assigned
active parcels plus deliveries completed or failed on the selected day. Returns
at most 20 rows with `id`, `trackingNumber`, `status`, `dropoffCity`,
`recipientName`, `weightKg`, `assignedAt`, `active`, plus `date` and `hasMore`.
Addresses and phone numbers are excluded.

### 8.10 `get_my_courier_delivery`

COURIER only. Argument: `parcelId` (UUID) from the courier's own list. The
Parcel service enforces the current assignment. Allowlist: `id`,
`trackingNumber`, `status`, `dropoffCity`, `recipientName`, `weightKg`,
`sizeCategory`, `assignedAt`, `history` (`status`, `previousStatus`, `note`,
`failureReason`, `changedAt`). Sender/recipient phones, addresses and proof data
are excluded.

### 8.11 `get_my_merchant_orders`

MERCHANT only. Optional `status` enum, `externalOrderId` (exact, max 100) and
`trackingNumber` (exact, max 20). The Merchant service resolves the merchant
from the verified session; `merchantId` is never accepted. Without a tracking
number the tool returns the recent page (at most 20). With a tracking number it
checks only the merchant's **most recent 100 orders** and requires an exact
match; a miss returns an `INVALID` result that states the scan window and asks
for an order ID or external order ID. No fuzzy matching and no claim of a full
historical search. Allowlist: `orderId`, `externalOrderId`, `trackingNumber`,
`status`, `recipientName`, `dropoffCity`, `total`, `currency`, `createdAt`,
`updatedAt`, `active`, plus `searchWindow` and `matchedBy` on a tracking match.

### 8.12 `get_my_webhook_deliveries`

MERCHANT only. Optional `success` boolean. Returns at most 20 attempts with
`eventId`, `trackingNumber`, `attemptNumber`, `httpStatus`, `success`,
`nextRetryAt`, `deadLettered`, `createdAt`. The endpoint URL, payload and signing
secret are never modelled. A merchant without a webhook receives an `INVALID`
result explaining that no webhook is configured.

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
| `MERCHANT_BASE_URL`            | `http://merchant:8084`  | HTTP(S) origin only                                                |
| `MERCHANT_TIMEOUT`             | `3s`                    |                                                                    |

The Gemini API key is never exposed to the browser, included in prompts, or logged.

## 12. Knowledge base

- Version-controlled Markdown under `internal/knowledge/data`, embedded at build time.
- Sections are split by Markdown headings and indexed with lightweight keyword scoring; no embeddings or vector database.
- Only approved repository content is used. The following have no approved content and must not be invented: cancellation/refund policy, terms and conditions, privacy policy, support contact details, delivery-time guarantees, legal entity details.
- Retrieval injects at most three relevant sections with source file names.

## 13. Security requirements

- Browser Origin validation: requests carrying an `Origin` header must match `FRONTEND_BASE_URL`; mismatches return `403 FORBIDDEN` before the guest session, rate limit or provider is touched. Requests without `Origin` (CLI clients) remain allowed.
- Prompt-injection resistance: retrieved documents and tool arguments are untrusted data. The system instruction states that they cannot change rules or grant permissions. A deterministic pre-provider guard refuses confident explicit prompt/secret/cross-user extraction and permission-bypass attempts; it never grants access.
- Tool authorization is centralized (`requireParcelAccess`, `requireStaffAccess`, `requireCourierAccess`, `requireMerchantAccess`) and checks the verified role before any upstream call. Registry visibility is a usability filter only. Model-supplied identity, role or owner fields are rejected by strict argument validation.
- Unknown tools are rejected; no tool can access the database or an arbitrary URL.
- No identity, role or ownership values are accepted from the model.
- Tokens and API keys are redacted from logs by the shared logging handler and are never logged by this service.
- Errors returned to clients and the model never include provider payloads, stack traces, SQL, or credentials.

## 14. Observability

Structured logs include request ID, conversation ID (safe opaque ID), model, provider duration, tool name, tool duration, tool status, token usage when available, and error category. Message bodies, guest session IDs, tokens and tool payloads are not logged. DISPATCHER/ADMIN operational lookups (`get_public_parcel_status`, `get_parcel_history`, `get_active_parcels`, `get_parcel_details`, `search_parcels`, `list_unassigned_parcels`, `get_courier_workload`) emit a `privileged parcel lookup` audit event with request ID, actor ID, role, tool, outcome and timestamp; no message text, token, address, phone number or note is included. Security-guard refusals log only request ID and a coarse category.

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
