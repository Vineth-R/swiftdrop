# How to adopt error handling and safe logging

1. Build the router with `httpx.NewRouter(logger)`. This adds request IDs, recovery, and the shared error envelope.
2. Return failures with `httpx.Error(c, status, code, message)`. Use API v1.1 codes, for example `VALIDATION_ERROR` or `NOT_FOUND`.
   For field errors use `httpx.ErrorWithDetails(c, 422, "VALIDATION_ERROR", "One or more fields are invalid.", details)`.
3. Never write stack traces, SQL, tokens, passwords, or personal data into `message`. Log safe details server-side with `slog`.
4. Create loggers with `logging.New(service, env, level)`; the redacting handler masks passwords, tokens, secrets, API keys, Authorization/Cookie values, phone numbers, addresses, and bodies.
5. Do not log request bodies. If a body must be referenced, pass it through `logging.BodyForLog(path, body)`, which always returns `[REDACTED]`; use `logging.IsAuthPath(path)` to skip body handling entirely on auth routes.
6. Health and readiness live at `/health` and `/ready` (with `/healthz` and `/readyz` as deprecated aliases); `/ready` is the database-backed check.
