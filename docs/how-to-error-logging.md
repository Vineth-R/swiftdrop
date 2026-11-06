# How to adopt error handling and safe logging

1. Build the router with `httpx.NewRouter(logger)`. This adds request IDs, recovery, and the shared error envelope.
2. Return failures with `httpx.Error(c, status, code, message)`. Use API v1.1 codes, for example `VALIDATION_ERROR` or `NOT_FOUND`.
   For field errors use `httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "One or more fields are invalid.", details)`.
3. Never write stack traces, SQL, tokens, passwords, or personal data into `message`. Log safe details server-side with `slog`.
4. Create loggers with `logging.New(service, env, level)`; the redacting handler masks sensitive keys at supported nesting levels and phone-like string values. See [Redaction coverage and limits](#redaction-coverage-and-limits).
5. Do not log request bodies. If a body must be referenced, pass it through `logging.BodyForLog(path, body)`, which always returns `[REDACTED]`; use `logging.IsAuthPath(path)` to skip body handling entirely on auth routes.
6. Health and readiness live at `/health` and `/ready` (with `/healthz` and `/readyz` as deprecated aliases); `/ready` is the database-backed check.

## Redaction coverage and limits

The redacting handler recursively inspects:

- `slog.Group` attributes, including nested groups.
- String-keyed maps such as `map[string]any` and `map[string]string`.
- Slices and arrays of supported values.
- Nested combinations of maps, slices and groups.
- Primitive values, `time.Time`, and non-nil `*time.Time` (nil pointers become null).

Sensitive keys are matched case-insensitively at every supported nesting level: passwords, tokens, secrets, API keys, authorization/cookie values, credentials, DSNs and connection strings, SMTP usernames, any key containing `address`, `phone` or `msisdn`, and the exact keys `body`, `request_body`, `requestbody` or `payload`. Sensitive map keys and group keys are replaced with `[REDACTED]`. String values are also checked for phone-like patterns.

The handler deliberately does **not** claim universal coverage:

- Arbitrary structs and error values are replaced with `[REDACTED]`; their exported fields and `Stringer`/`Error` output are not inspected.
- `[]byte`, `json.RawMessage` and other byte slices are replaced with `[REDACTED]`.
- Nested values implementing `slog.LogValuer`, `json.Marshaler` or `encoding.TextMarshaler` are replaced with `[REDACTED]` without invoking their custom methods.
- Maps with non-string keys are replaced with `[REDACTED]`.
- Strings under non-sensitive keys are not content-scanned except for phone-like values. Put secrets under a sensitive key.
- Cyclic containers are redacted at the cycle, and recursion is bounded to 32 levels.

Log structured data with `slog.Group` or string-keyed maps. Do not log arbitrary structs, error values, byte slices, or request bodies. If a value cannot be represented safely, log an explicit safe identifier instead.

## Shared validation helper

Use `packages/go-common/validation` for reusable field checks. It has no business rules and does not know HTTP status codes; callers keep their existing messages, field names and response status.

```go
issues := validation.New()
issues.Required("email", request.Email)
issues.Email("email", strings.TrimSpace(request.Email))
issues.UUID("zoneId", request.ZoneID)
issues.Enum("status", request.Status, "must be one of PENDING, SENT, FAILED", "PENDING", "SENT", "FAILED")
issues.Length("fullName", strings.TrimSpace(request.FullName), 2, 150, "must be 2 to 150 characters")
issues.Custom("redirectTo", safeRelativePath(request.RedirectTo), "must be a relative path")

if !issues.Empty() {
    httpx.ErrorWithDetails(c, http.StatusUnprocessableEntity, "VALIDATION_ERROR",
        "One or more fields are invalid.", httpx.ErrorItems(issues.Issues()))
    return
}
```

Rules:

- Add checks in the same order the endpoint already reports them. The collector preserves insertion order, so results are deterministic.
- Keep the existing API v1.1 `code`, top-level `message`, field names and HTTP status. Do not invent new validation messages during adoption.
- Use `RequiredExact` for values where whitespace is significant, such as passwords. `Required` treats whitespace-only input as missing.
- Use `Custom` for service-specific rules such as password policy, redirect safety or pricing decimals. Do not move business rules into `go-common`.
- `httpx.ErrorItems` converts shared issues to the API v1.1 `details` array. Do not build that array by hand.
- Services may keep a single-error type, such as pricing's `Invalid`, when collecting multiple issues would change existing behavior. Use the shared predicates (`validation.UUID`, `validation.OneOf`, `validation.RuneLength`) in that case.

## Requirements for new HTTP services

- Build the router with `httpx.NewRouter(logger)` so request IDs, panic recovery, JSON 404/405 and request logging are present.
- Construct the logger with `logging.New(service, env, level)` and pass it to the router. Do not use `slog.New` directly in production service code.
- Return failures with `httpx.Error` or `httpx.ErrorWithDetails`. Do not write ad-hoc JSON error shapes.
- Keep health and readiness at `/health` and `/ready`; register the deprecated aliases through the shared helpers where applicable.
- Validate request bodies and query parameters before calling business code. Use the shared validation helper for generic checks.
- Never put stack traces, SQL, credentials, tokens or personal data in client-facing messages.

## Requirements for new workers

- Construct the logger with `logging.New`; worker logs must identify events and operations without recipients, message bodies, templates, tokens or credentials.
- Validate message framing, envelopes and payload fields before side effects. Permanent failures stay permanent; unclassified handler errors remain retryable.
- Do not add an HTTP server to a worker unless an explicit internal API is part of the design.

## Automated repository checks

`tests/architecture/shared_infra_test.go` parses the Go AST for every direct service under `apps/services`:

- Every service or worker `main.go` must assign `logging.New` to a non-blank identifier.
- `main.go` must not call `slog.New` directly.
- An HTTP service is identified by an `internal/http` directory or by `httpx.NewRouter` in `main.go`.
- `httpx.NewRouter` must be initialized in `main`/`run`, or in an `internal/http.New` function that `main`/`run` calls.
- A `httpx.NewRouter` call in an unused helper function does not satisfy the check.

The check resolves aliased imports and ignores comments and strings. It runs as part of `go test ./...` and has a dedicated CI step:

```sh
go test ./tests/architecture -run TestRepositorySharedInfrastructure -v
```

This is a deterministic convention and wiring check, not a complete data-flow proof. It does not prove that every logger value is used at every call site, and it does not inspect arbitrary files outside `main.go` and `internal/http`. New services should follow the conventional `main.go`/`internal/http` layout. A violation fails CI with the exact file and missing requirement.
