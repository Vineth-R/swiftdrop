# SwiftDrop AI Delivery Assistant — Final Technical Audit

Date: 2026-11-08
Auditor: Lead audit (Agent 1), with independent reviews by Gemini, backend security, frontend and infrastructure agents.
Repository: `Vineth-R/swiftdrop`
Branch audited: `fix/booking-fee-post-merge-hardening` (working tree, uncommitted)
Status: Audit complete. No commit, push, merge or branch change was performed.

## 1. Executive summary

The AI Delivery Assistant is functionally integrated and materially matches its published contract. The backend, frontend, Compose service and Nginx route build and run. Real Gemini calls succeeded, including a full function-calling round trip and final text synthesis. The live audit found one genuine integration blocker that the previous sessions did not catch: Gemini 3.x requires the opaque `thought_signature` from the function-call part to be echoed back. The provider had dropped it via `FunctionCalls()`, so every tool round would fail with `400 INVALID_ARGUMENT`. This is fixed and covered by tests.

No Critical security defects remain. The audit also fixed a timestamp-tie eviction flake, token-usage counters being redacted, missing-amount responses being rendered as zero, a gateway availability coupling on a missing Gemini key, Nginx exposing internal AI probes, missing `no-store`, case-sensitive content-type handling, model-supplied unknown tool names in the public response, and two frontend layout blockers (courier tab overlap and landscape panel clipping).

Real Gemini testing was partially successful and partially blocked by provider-side availability:

- The configured model, `gemini-3.8-flash`, returned successful text and function calls in early runs, including a 580-byte thought signature and a successful two-step tool round. Later in the audit the same model returned repeated `504 DEADLINE_EXCEEDED` after roughly 58 seconds. A direct SDK probe reproduced this, so it is a provider-side availability/latency issue, not a service regression. The service correctly returned `503 AI_UNAVAILABLE`.
- The verified stable fallback `gemini-3.5-flash` completed the service-level basic and pricing flows with the configured timeouts. The code and example defaults were corrected to this model; the local `.env` still explicitly selects `gemini-3.8-flash`.

Live authenticated private-tool testing was not performed because no safe test account/parcel was provided. It remains covered by unit, integration and mocked E2E tests and is marked as not performed live.

Development merge: **READY AFTER MINOR RELEASE-PROCESS FIXES** (commit on a dedicated branch cut from `development`; the code and tests are green).
Main merge: **NOT READY** as a normal merge. `origin/main` is deliberately AI-free and 382 commits behind `origin/development`; adopting AI on main requires an explicit product/legal decision and a curated release, not a fast-forward.

## 2. Final architecture and implementation verification

```text
Next.js widget
  -> Nginx /api/v1/ai/chat (prefix stripped)
  -> ai-assistant:8087
     -> optional JWT identity + 15/min user-or-IP rate limit
     -> conversation ownership + TTL store
     -> deterministic scope guard
     -> keyword knowledge retrieval (embedded Markdown)
     -> Gemini GenerateContent with four function declarations
     -> strict allowlist + argument validation + authorization
     -> Pricing / Parcel clients with caller-only credentials
     -> sanitized function result
     -> Gemini final text
  -> API v1.1 envelope
```

Verified against `apps/services/ai-assistant/IMPLEMENTATION_CONTRACT.md`:

- `POST /api/v1/ai/chat` maps to Gin `POST /ai/chat` on internal port 8087.
- Request/response envelope, `conversationId`, `toolCalls` and error codes match the contract.
- Exactly four tools are registered.
- No database, Redis, MCP, vector store, write tool or arbitrary HTTP tool was added.
- The Gemini key is read only by the assistant and never reaches the web bundle.

## 3. Session 1 and Session 2 deliverable verification

### Session 1 (backend) — verified

- Go service, config, middleware, conversation store, knowledge base, orchestration, HTTP handlers, OpenAPI and docs exist.
- Gemini and mock providers exist; provider tests inject the SDK call.
- Pricing and Parcel clients exist; tools sanitize results.
- Security tests cover auth, private-tool denial, argument validation, ownership, limits, sanitization and scope.

### Session 2 (frontend/infrastructure) — verified

- Six chat components, API client, hook, unit tests and Playwright spec exist.
- Root layout mounts the widget for public, authenticated and app routes.
- Compose has an `ai-assistant` service on internal 8087 with a `/ready` healthcheck.
- Nginx has an `ai-assistant` upstream and an AI route.
- CI has AI path filters; no Gemini key is required by CI.
- `docs/ai-assistant.md` and the root README describe the integration.

Discrepancies found and reconciled:

- The previous contract said Session 2 had not yet integrated Compose/Nginx; the tree already had. The contract was updated.
- The code default model was `gemini-2.5-flash`, which is not available to the configured key (`404 NOT_FOUND`). It was changed to the verified `gemini-3.5-flash`.
- The previous statement that the service refuses to start outside development without a key was changed to graceful degradation (`503 AI_UNAVAILABLE`) so a missing key cannot take down the gateway.

## 4. Gemini live API test results

Environment checks (values never printed):

- `GEMINI_API_KEY`: present and non-empty.
- `AI_PROVIDER=gemini`, `AI_ENABLED=true`.
- `AI_MODEL=gemini-3.8-flash` in the local `.env`.
- `AI_REQUEST_TIMEOUT=15s`, `AI_TOTAL_TIMEOUT=25s`.

| Test                         | Expected                               | Actual                                                                                                 | Result             | Evidence                                                                  |
| ---------------------------- | -------------------------------------- | ------------------------------------------------------------------------------------------------------ | ------------------ | ------------------------------------------------------------------------- |
| Model availability           | Configured model is usable             | `models/gemini-3.8-flash`, display name “Gemini 3.8 Flash”, input limit 1,048,576, output limit 65,536 | PASS               | SDK `Models.Get` returned the model; 531 ms                               |
| Basic SwiftDrop conversation | 200, text reply, no tools              | HTTP 200, reply, `toolCalls=[]`                                                                        | PASS               | Service run; 3.7–3.8 s provider latency                                   |
| Function call                | Model selects `calculate_delivery_fee` | `tool_calls=1`, arguments present                                                                      | PASS               | `origin=Colombo`, `destination=Kandy`, `weightKg=2`, `sizeCategory=SMALL` |
| Thought signature            | Signature preserved and echoed         | 369–580-byte opaque signature observed and replayed                                                    | PASS               | Live two-step round succeeded after fix                                   |
| Two-step tool round          | FunctionResponse produces final text   | HTTP 200, final text includes the quote                                                                | PASS               | Service run with controlled fake Pricing upstream                         |
| Usage metadata               | Prompt/output/total captured           | Numeric usage logged                                                                                   | PASS               | e.g. `usage_prompt=1344 usage_output=58 usage_total=1553`                 |
| Out-of-scope question        | Polite refusal, no tools               | Deterministic refusal, no provider call                                                                | PASS               | “Explain quantum computing.”                                              |
| Invalid input                | 422                                    | `422 VALIDATION_ERROR`                                                                                 | PASS               | Empty/whitespace message                                                  |
| Public tracking              | Tool selected, public fields only      | `get_public_parcel_status` status `OK`; no private sentinel fields in reply                            | PASS               | Controlled fake Parcel upstream with private fields present               |
| Pricing                      | Tool selected, real quote used         | `calculate_delivery_fee` status `OK`; reply contained upstream total 550                               | PASS               | Controlled fake Pricing upstream; no independent calculation              |
| Private tools live           | Authenticated owner only               | Not performed live (no safe test account/parcel)                                                       | SKIPPED            | Covered by unit/integration tests                                         |
| Later 3.8 availability       | Provider responds                      | Repeated `504 DEADLINE_EXCEEDED` after ~58 s                                                           | BLOCKED (provider) | Raw SDK probe reproduced; service returned 503 safely                     |
| Stable fallback              | Basic and pricing work                 | `gemini-3.5-flash`: both HTTP 200                                                                      | PASS               | Provider latency 1.3–2.0 s; usage totals 1,553 / 1,638 / 1,817            |
| Missing key, production mode | Service starts, chat 503               | `/ready` 200 with `provider=unconfigured`; chat `503 AI_UNAVAILABLE`                                   | PASS               | No key supplied, `APP_ENV=production`                                     |

The live function-calling tests above are real Gemini tests. The provider package tests that inject `generate` are labelled as injected SDK tests; the mock provider is never counted as live proof.

### Gemini defect found and fixed

`internal/provider/gemini/gemini.go` previously used `response.FunctionCalls()`, which returns `*genai.FunctionCall` and discards the function-call Part's `ThoughtSignature`. Gemini 3.x returned:

`400 INVALID_ARGUMENT: Function call is missing a thought_signature in functionCall parts...`

Fix: the provider now parses candidate parts directly, stores the signature in `contract.FunctionCall.ThoughtSignature`, and re-emits it on the function-call part in the next request. Regression tests cover request construction and response parsing. The live two-step round then succeeded.

## 5. Tool-by-tool verification

| Tool                       | Verification                                                                                                     | Evidence                                                                                              |
| -------------------------- | ---------------------------------------------------------------------------------------------------------------- | ----------------------------------------------------------------------------------------------------- |
| `calculate_delivery_fee`   | Live Gemini selected it; resolved zones through `/zones`; called the estimate endpoint; returned upstream amount | `tools=[{"name":"calculate_delivery_fee","status":"OK"}]`; reply contained `550`; no fabricated price |
| `get_public_parcel_status` | Live Gemini selected it; public tracking called; only allowlisted fields sent to the model                       | status `OK`; private sentinel fields were absent from the reply                                       |
| `get_my_parcel_history`    | Unit/integration only                                                                                            | Requires auth before HTTP; UUID validation; token forwarded; cross-customer 404 safe                  |
| `get_my_active_deliveries` | Unit/integration only                                                                                            | Requires auth; status enum; 20-item cap; allowlist excludes addresses/internal data                   |

All four are the only registered tools. Unknown names are never executed and are no longer published in the public `toolCalls` array.

## 6. Backend security review

Verified controls:

- HS256 verification, issuer, expiry, issued-at, subject and token ID; invalid supplied credentials return 401 and are never downgraded to anonymous.
- Identity/roles only from verified claims; no model-supplied identity.
- Anonymous private-tool calls return `DENIED` before any upstream request.
- The caller's Bearer token is forwarded only to Parcel; it is never placed in the system instruction, model contents or logs.
- Strict tool arguments: required keys, types, enums, UUID/tracking formats, weight precision, unknown keys rejected.
- Public tracking and delivery/history results use explicit allowlists; private upstream fields cannot leak.
- Prompt injection is addressed in the system instruction and independently by tool validation/authorization.
- Rate limiter is bounded, per-user/per-IP, fail-closed when full, and returns `Retry-After`.
- Conversation store enforces ownership, TTL, max messages and bounded eviction.
- Tool loop enforces max rounds, max calls, duplicate-call protection, provider/tool/total timeouts and safe fallbacks.
- Provider errors are logged only as categories; usage counters are now logged as numeric values.
- Responses now use `Cache-Control: no-store`.
- Internal HTTP clients do not use `HTTP_PROXY`/`HTTPS_PROXY`, so private Bearer tokens cannot traverse an environment proxy.
- Malformed upstream `200` responses with missing amounts are rejected as `UNAVAILABLE` instead of becoming `0`.

The previously reported `TestEvictsOldestWhenFull` flake was real: eviction depended on `UpdatedAt`, and equal timestamps made map iteration order decide the victim. The store now assigns a monotonic `seq` under the mutex; a frozen-clock regression test passes 500× sequentially and 100× with `-race`.

Remaining backend limitations are documented in section 12.

## 7. Frontend review

Fixed:

- **Courier mobile overlap (High):** the launcher now uses `bottom-20`, clearing the 56 px courier tab bar. A new E2E test signs in as COURIER and asserts no rectangle overlap with the Profile tab.
- **Landscape/short viewport clipping (High):** the floating panel now uses `lg:top-24 lg:bottom-40` instead of a fixed `h-128`; landscape phones keep the full-screen mobile inset layout and the Close button reachable. New landscape E2E test passes.
- **Lost input on failure (Medium):** the textarea no longer clears until a send succeeds; a 422 keeps the typed message for correction. E2E asserts the value remains.
- **36 px Close target (Medium):** changed to the 44 px `md` icon-button size.
- **Launcher label (Medium):** now changes between “Open…” and “Close…” and uses `aria-controls`.
- **Stale 429 alert (Low):** the alert clears when the retry window elapses.
- **Conversation ID validation and error reference (Low):** the client now requires `^cv[0-9a-f]{32}$` and only shows a reference for server/network faults.
- **Duplicate live-region announcement (Low):** the typing indicator no longer nests a second `role="status"`.

Verified:

- Widget mounts globally; open/close, focus return to launcher, Escape, Enter/Shift+Enter and empty-message prevention work.
- 19/19 AI Playwright tests pass.
- Web unit tests 65/65, lint, typecheck and build pass.
- Node library tests and ESLint-rule tests pass (`fail 0`).
- No Gemini key or `NEXT_PUBLIC_GEMINI` reference in the frontend; no key in the built static bundle.

Not fixed / remaining:

- `aria-modal="false"` with a manual focus trap is semantically inconsistent (Medium accessibility).
- Closing the panel discards the in-memory conversation ID; reopening starts a new conversation (Low).
- The character counter uses UTF-16 code units while the backend counts runes (Low; under-allows emoji).
- No explicit Stop control for an in-flight request (enhancement).
- iOS Safari visual-viewport behavior was not verified on a real device.

Full-suite Playwright: Agent 4 observed 557 passed / 6 failed on `next dev`. Four failures were dev-only (Strict Mode duplication and Next Dev Tools); the same subset passed under the production server mode used by CI. The failures are not AI-related. The full suite was not rerun after the AI-only fixes; the dedicated AI spec was rerun and passed 19/19.

## 8. Docker, Nginx and CI review

Verified:

- `docker compose config --quiet` passes.
- `docker compose build ai-assistant` succeeds; image `swiftdrop-ai-assistant` builds from `SERVICE=ai-assistant`.
- The assistant has no published host port; Compose healthcheck targets `127.0.0.1:8087/ready`.
- The Gemini key is injected only into `ai-assistant`, not into web, not a build arg, not `NEXT_PUBLIC_`, and not tracked.
- `.env` is ignored; `.env.example` contains an empty key placeholder.
- Nginx strips `/api/v1` and reaches `/ai/chat`.
- **Fixed:** the Nginx AI route was narrowed from a prefix proxy to exact `location = /api/v1/ai/chat`; other `/api/v1/ai/*` probes now return 404. A static Nginx test asserts this.
- **Fixed:** `stop_grace_period: 40s` was added to the assistant so its bounded graceful shutdown can drain in-flight turns.
- **Fixed:** a missing key no longer prevents process startup outside development; it returns `503 AI_UNAVAILABLE` and does not block Nginx via its health dependency.
- CI has AI path filters and does not require a Gemini key.
- `govulncheck ./...` reports 0 vulnerabilities; gRPC was upgraded to v1.83.1 during Session 1 to clear known advisories.

Remaining infrastructure/release risks:

- PR CI does not exercise the AI gateway route live. The static Nginx test and the route-check mock improve this, but the access job still runs only the existing `tests/access` suite.
- The release smoke test uses `.env.example` in development mode, so it does not validate the production no-key/startup path.
- `origin/main` is AI-free; see section 16.
- `.gitignore:117` contains pre-existing merge-conflict residue (`>>>>>>> origin/development`); not introduced by this work.

## 9. Full test execution results

| Check                                                                           | Result                                                          |
| ------------------------------------------------------------------------------- | --------------------------------------------------------------- |
| `go build ./...`                                                                | PASS                                                            |
| `go vet ./...`                                                                  | PASS                                                            |
| `go test -count=1 ./...`                                                        | PASS, all packages                                              |
| `go test -race -count=1 ./apps/services/ai-assistant/...`                       | PASS, all AI packages                                           |
| `go test -count=50` conversation; `-race -count=50`                             | PASS                                                            |
| `pnpm exec nx run-many -t build test lint -p ai-assistant --parallel=1`         | PASS                                                            |
| `pnpm exec nx test @org/web --skip-nx-cache`                                    | PASS, 65 tests                                                  |
| `pnpm exec nx lint @org/web --skip-nx-cache`                                    | PASS                                                            |
| `pnpm exec nx typecheck @org/web --skip-nx-cache`                               | PASS                                                            |
| `pnpm exec nx build @org/web --skip-nx-cache`                                   | PASS                                                            |
| `pnpm exec nx typecheck @org/web-e2e --skip-nx-cache`                           | PASS                                                            |
| `pnpm exec nx e2e @org/web-e2e --args="--project=chromium src/ai-chat.spec.ts"` | PASS, 19/19                                                     |
| `node --test "apps/web/src/lib/*.test.mjs"`                                     | PASS                                                            |
| `node --test "apps/web/eslint-rules/*.test.mjs"`                                | PASS                                                            |
| `docker compose config --quiet`                                                 | PASS                                                            |
| `docker compose build ai-assistant`                                             | PASS                                                            |
| `go test ./tests/nginxconfig/... ./tests/scripts/...`                           | PASS                                                            |
| `pnpm exec prettier --check` on changed docs/YAML/JSON                          | PASS                                                            |
| `git diff --check`                                                              | PASS                                                            |
| `govulncheck ./...`                                                             | 0 vulnerabilities                                               |
| Live Gemini early runs (3.8)                                                    | PASS                                                            |
| Live Gemini later runs (3.8)                                                    | BLOCKED by provider 504; service returned safe 503              |
| Live Gemini stable fallback (3.5)                                               | PASS                                                            |
| Live private tools                                                              | SKIPPED (no safe account/parcel)                                |
| Full Compose stack / live `make check-routes`                                   | SKIPPED                                                         |
| `nginx -t`                                                                      | SKIPPED (binary unavailable); static `tests/nginxconfig` passed |
| Gitleaks / Trivy / Syft locally                                                 | SKIPPED (not installed); CI owns them                           |

## 10. Defects identified

### High

1. **Gemini 3.x thought signature dropped** — every tool round returned `400 INVALID_ARGUMENT`. Fixed.
2. **Missing Gemini key could take down the gateway outside development** — startup refusal plus Nginx `depends_on: service_healthy`. Fixed with graceful `503` startup and Compose stop grace.
3. **Courier launcher overlapped the Profile tab** on mobile. Fixed.
4. **Panel clipped off-screen on landscape/short viewports**, hiding Close. Fixed.

### Medium

5. Token-usage counters were redacted by the shared logger. Fixed with non-sensitive key names.
6. `TestEvictsOldestWhenFull` could evict a non-oldest conversation when timestamps tied. Fixed with monotonic sequence.
7. Missing upstream amounts decoded to empty `json.Number` and marshalled as `0`, potentially fabricating a price. Fixed with response validation.
8. Nginx exposed AI `/health` and `/ready` under the public prefix. Fixed with exact route and 404 catch-all.
9. Chat responses lacked `Cache-Control: no-store`. Fixed.
10. Case-sensitive content-type check rejected `Application/JSON`. Fixed.
11. Unknown model-supplied tool names appeared in the public `toolCalls` enum. Fixed.
12. Failed sends cleared the input, making 422/429/401 recovery lose text. Fixed.
13. Close target was 36 px. Fixed to 44 px.
14. Launcher had a stale “Open” label while open. Fixed.
15. Rate-limit alert stayed after the countdown. Fixed.
16. Shutdown grace (10 s) was shorter than the 25 s turn budget. Fixed to `AI_TOTAL_TIMEOUT + 10 s` with Compose 40 s grace.
17. `AI_TEMPERATURE=NaN` passed validation. Fixed.
18. Internal clients could use environment proxies for Bearer-token calls. Fixed with `Proxy: nil`.
19. Code/example default model was unavailable (`gemini-2.5-flash` 404). Fixed to verified `gemini-3.5-flash`.

### Low / remaining

20. `aria-modal=false` conflicts with the focus trap.
21. Closing the panel loses conversation continuity.
22. Character counter uses UTF-16 units.
23. No in-flight cancel/Stop control.
24. iOS visual-viewport behavior unverified.
25. PR CI does not exercise the AI route live.
26. Local full-suite E2E has dev-only failures unrelated to AI.
27. `.gitignore` conflict residue is pre-existing.
28. In-process conversation/rate-limit state is not durable or multi-replica.

### Informational

- All provider API errors map to `503 AI_UNAVAILABLE` by contract; this is safe but hides 429/401/403 distinctions.
- `AI_MAX_OUTPUT_TOKENS=1024` is intentionally below the model limit.
- Anonymous conversations are capability IDs (128-bit `crypto/rand`), by contract.
- Shared `HTTP_TRUSTED_PROXIES=0.0.0.0/0` would weaken anonymous IP limiting; it is operator configuration.

## 11. Fixes applied

Backend:

- `contract.FunctionCall.ThoughtSignature`; provider preserves and echoes Gemini 3.x signatures.
- Provider parses candidate parts directly and ignores thought text.
- Provider optional `http.Client` timeout.
- Token usage logged as `usage_prompt` / `usage_output` / `usage_total`.
- Conversation eviction is FIFO by monotonic sequence; frozen-clock test added.
- `CanonicalToolKey` is JSON-canonical and delimiter-safe; contract tests added.
- Numeric response validation for Pricing estimates and Parcel list totals.
- JSON error is unavailable rather than fabricated zero.
- Internal HTTP clients disable environment proxies.
- `Cache-Control: no-store` on chat.
- Case-insensitive JSON content type.
- Unknown tool names are not published as executed calls.
- Missing key no longer refuses startup; production mode returns 503 and `/ready` reports `provider=unconfigured`.
- Non-finite temperature rejected.
- Shutdown timeout derived from `AI_TOTAL_TIMEOUT`.
- Nginx exact AI chat route; other AI probes return 404; static test added.
- Compose stop grace 40 s.
- Route-check script asserts AI probes are not public.
- Default model changed to verified `gemini-3.5-flash` in code, `.env.example`, Compose and docs.
- Concurrency tests added for the conversation store and limiter.
- Removed unused `ErrProviderRejection`, `UpstreamCode`, and unsafe key helpers.

Frontend:

- Launcher offset and dynamic accessible name/`aria-controls`.
- Responsive panel geometry; landscape E2E and courier-overlap E2E.
- Failed input retention; 422 E2E assertion.
- 44 px close target.
- Rate-limit alert auto-dismiss.
- Safer conversation ID validation and 4xx reference handling.
- Removed duplicate typing live region.

Documentation:

- `IMPLEMENTATION_CONTRACT.md`, `README.md`, `docs/ai-assistant.md`, `AGENTS.md`, `docs/architecture.md` and OpenAPI were updated for the new behavior, error code, model default, startup semantics and Nginx exact route.

## 12. Remaining limitations

- Conversation storage and rate limiting are process-local, TTL/bounded, and not shared across replicas. A multi-replica deployment needs shared state or edge coordination.
- No streaming; the reply arrives after the tool loop.
- Knowledge retrieval is keyword/IDF, not semantic.
- Real Gemini latency is variable. `gemini-3.8-flash` returned provider-side 504s during the audit; the service correctly fails safe, but production availability depends on model/provider health.
- Private-tool live verification requires a safe authenticated customer and parcel; only automated coverage exists.
- `aria-modal` and focus-trap semantics need a deliberate accessibility decision.
- Conversation continuity ends when the panel closes.
- Full deployment observability (metrics, tracing, alerting) is not implemented.
- Production TLS and high-availability deployment remain out of scope.

## 13. Known risks

- **Provider availability:** a slow/overloaded model can exceed the 15 s request / 25 s turn budget and return 503. This is handled safely but is a UX risk.
- **Cost/quota:** Gemini calls are billable and rate-limited; no cost dashboard or quota alarm exists.
- **Anonymous denial-of-service:** process-local IP limiting is bounded but not distributed; IP spoofing is possible only with a misconfigured trusted-proxy allowlist.
- **Memory:** bounded conversation storage can still consume significant memory at the maximum configuration.
- **Main-line policy:** merging `development` into the AI-free `main` would silently re-introduce a large AI tooling surface and bypass main's absent AI CI filters.
- **Secret handling:** the key is correctly isolated, but production must use a secret manager rather than a plain `.env`.

## 14. Environment and deployment requirements

Required for a real deployment:

- `APP_ENV` explicitly `development`, `staging` or `production`.
- `AI_ENABLED=true`, `AI_PROVIDER=gemini`.
- `GEMINI_API_KEY` injected as a secret only into the assistant container.
- `AI_MODEL` set to a model verified for the deployment key. The audited fallback is `gemini-3.5-flash`; `gemini-3.8-flash` is configured locally but showed provider-side 504s.
- `JWT_ACCESS_SECRET` and `COOKIE_NAME` matching Identity/Parcel, otherwise private tools and credential-bearing requests fail closed.
- `PRICING_BASE_URL`, `PARCEL_BASE_URL`, and their timeouts.
- `HTTP_TRUSTED_PROXIES` limited to the gateway network.
- Nginx `proxy_read_timeout` greater than the configured total turn budget; the current stack uses 25 s total and 30 s proxy read.
- Gemini key never in a build arg, web image, `NEXT_PUBLIC_` variable, source file or log.

## 15. Merge-readiness assessment for `development`

**READY AFTER MINOR RELEASE-PROCESS FIXES.**

Technical state is green:

- All Go, Nx AI, web unit, lint, typecheck, build, AI E2E, Nginx and Compose checks pass.
- The live Gemini and tool flows were verified, with the provider outage documented.
- No Critical/High security defect remains after the fixes.

Release-process blockers:

1. The AI work is uncommitted on `fix/booking-fee-post-merge-hardening`, which is 3 booking-fee commits ahead of `origin/development`. It must be moved to a dedicated feature branch cut from `development`.
2. `AGENTS.md` and `README.md` are touched by both the booking-fee commits and the AI work; those files need a careful merge/rebase.
3. No PR, CI run or review exists for the AI change set yet.
4. `main` is AI-free; do not merge the booking-fee branch into main as a shortcut.

Recommended sequence (do not run without team approval):

```sh
# Preserve the AI work, then create a clean feature branch from development
git stash push -u -m "ai-delivery-assistant"
git switch -c feature/ai-delivery-assistant origin/development
git stash pop
# resolve AGENTS.md/README.md overlap with the booking-fee work
pnpm exec prettier --write AGENTS.md README.md
go test -count=1 ./...
pnpm exec nx run-many -t build test lint -p ai-assistant
pnpm exec nx test @org/web && pnpm exec nx typecheck @org/web && pnpm exec nx build @org/web
pnpm exec nx e2e @org/web-e2e --args="--project=chromium src/ai-chat.spec.ts"
git add <reviewed files>
git commit -m "feat(ai): add SwiftDrop Delivery Assistant"
```

Open a PR into `development`, let CI run, and do not merge until the AI-free `main` policy is decided.

## 16. Merge-readiness assessment for `main`

**NOT READY for a normal merge.**

- `origin/main` is an ancestor of `origin/development` and is 382 commits behind.
- `main` deliberately excludes AI tooling (`31e341d chore(release): exclude AI tooling from main`).
- `main` has no `ai-assistant` service, no AI Compose/Nginx entries, and no AI path filters in CI.
- A plain `development -> main` merge would re-add hundreds of AI/tooling files and silently reverse the release policy.
- Main's CI would not build/scan the AI image because its path filters lack `ai-assistant`.

To adopt AI on main, the team must make an explicit product/legal decision and perform a curated release that includes the assistant service, web widget, contracts, Compose, Nginx, CI filters and documentation together. Otherwise, keep `main` AI-free and release AI only on `development`/feature environments.

Production readiness is separate: the current in-process conversation/rate-limit design is acceptable for a feature environment but not for a multi-replica production deployment without shared state and additional observability.

## 17. Exact remaining actions

1. Commit the AI work on a dedicated branch from `development`; preserve the booking-fee work; resolve `AGENTS.md`/`README.md` overlap.
2. Open the PR and run CI. Consider adding a live `/api/v1/ai/chat` check to the access job or `check-routes.sh`.
3. Decide the `main` policy: AI-free curated release vs. intentional AI adoption with a full merge of the AI CI/Compose/Nginx changes.
4. For deployment, set a verified model. `gemini-3.5-flash` succeeded in this audit; `gemini-3.8-flash` showed provider-side 504s. Monitor Gemini latency/error rate.
5. Decide whether to fix the remaining Medium/Low UX items: `aria-modal`/focus trap, conversation persistence across panel close, cancel control, and UTF-16 counter.
6. Run `gitleaks`, Trivy/Syft and the full CI pipeline on the committed branch (not available locally during this audit).
7. Keep `GEMINI_API_KEY` only in the deployment secret store and the untracked local `.env`.

## 18. Final recommendation

The SwiftDrop AI Delivery Assistant is a sound, well-tested feature that is technically ready for integration into `development` after the release-process steps in section 15. It is **not** ready for a naive merge into the AI-free `main` line, and it is not yet a production-grade multi-replica service without shared conversation/rate-limit state and operational monitoring.

The most important engineering outcome of this audit is the live-discovered Gemini 3.x thought-signature defect: the feature would have failed every real tool round without it. That is now fixed, tested and verified live. The remaining risks are release policy, provider availability and the documented MVP limitations—not unresolved authorization, sanitization or secret-exposure defects.

## 19. CI follow-up — 2026-11-08

This section is appended after the original audit and does not rewrite prior findings.

### PR and run state

- PR: [#106](https://github.com/Vineth-R/swiftdrop/pull/106)
- Head audited: `3b0b63cf4e301914ebeac0444b3ad64c7dcb2d7e`
- Workflow run: `42464200860`
- PR state: open, targeting `development`, mergeable with no conflicts
- Check summary at the end of the initial run: **18 passed, 10 failed, 0 pending** (29 checks)
- Access tests (Compose stack): **passed** in 14m55s

Passing checks included Go checks, frontend checks, Playwright E2E, Go vulnerability scan, PostgreSQL, RabbitMQ, identity, parcel and notification E2E, SAST/CodeQL, access tests, and the `pricing`/`webhook-worker` Docker builds.

### Gitleaks / Security gate

- Gitleaks: **failed** (job `128679815989`).
- Finding: `generic-api-key` in `apps/services/realtime/internal/ws/handler_test.go` at commit `c3d22b3886c851ae83d062b22cf933b97819e25d`.
- That commit is on `origin/feature/m10-realtime-handler`; it is not reachable from `development`, `main`, or PR #106.
- The file is a realtime WebSocket test; the flagged value is a hard-coded test JWT signing secret with explicit placeholder-style words. It is used only with `auth.NewTokenManager` in tests. It is not an AI credential and there is no evidence it is used in production; it should still be treated as a test credential and not reused.
- The branch has no open PR. The commit author is `WathsalaKJ`.
- Root cause of the PR impact: the workflow checks out full history with `fetch-depth: 0` and runs `gitleaks git .`, which scans every fetched ref, including unmerged feature branches. This is a repository-wide security-gate scope issue, not an AI feature defect.
- The finding was **not** suppressed. No broad Gitleaks exclusions were added. The Security gate and Required CI gate failed as downstream consequences.
- Recommended remediation (owner/security decision): replace the hard-coded test secret in the realtime branch, or add a narrow, owner-approved allowlist; alternatively, scope the PR Gitleaks scan to the PR range while retaining full-history scans on `development` pushes. None of these changes were made in this PR.

### Syft/SBOM Docker failures

- Seven Docker jobs failed: `ai-assistant`, `identity`, `merchant`, `notification-worker`, `parcel`, `realtime`, `web`.
- In every case the Docker build completed with no build error; the failure was the Syft installer download:
  `HTTP status=504` for `syft_1.56.0_linux_amd64.tar.gz`, followed by `The Syft installer failed to install v1.56.0`.
- `pricing` and `webhook-worker` Docker jobs passed.
- A failed-jobs rerun reproduced the same Syft HTTP 504 in the completed reruns. This is external GitHub release/CDN availability, not an AI build defect.
- SBOM/Trivy generation was not disabled or bypassed.

### Access tests

- Passed. The Compose stack built, started, migrated, passed the database baseline check, and completed the access test step.

### Fixes applied

- No AI code changes were required by this CI follow-up.
- Only this documentation follow-up was added.
- The unrelated realtime branch, booking-fee work, `main`, and PR #106 merge state were not modified.

### Outstanding blockers and recommendation

1. Gitleaks/Security gate: blocked by the unrelated realtime branch fixture; the owner must remediate it or the team must approve a scoped scanner-policy change.
2. Docker/SBOM jobs: blocked by repeated Syft download HTTP 504; rerun after the external artifact is available.
3. Required CI gate: fails while the above are red.

AI-specific checks are green. PR #106 is **not ready to merge** until the security gate and Docker/SBOM checks pass. Do not merge without separate approval.

## 20. CI policy follow-up — 2026-11-08

This section records the approved Gitleaks scope correction and remains an append-only continuation of the audit.

### Final Gitleaks scanning policy

- **Pull requests:** scan only `${PR_BASE_SHA}..${PR_HEAD_SHA}` with `--log-opts` and `-m`, using the base and head SHAs from the pull-request event. Unrelated remote branches are no longer scanned.
- **Protected branch and tag pushes:** retain the full-history scan only for pushes to `refs/heads/development`, `refs/heads/main`, and tag pushes.
- **Feature-branch pushes:** when the workflow is configured to run for them, scan the pushed range using `github.event.before..github.event.after`; for a new branch, resolve the base with `git merge-base origin/development <after>`.
- **Fail closed:** missing or unresolvable base/head/after SHAs stop the job before Gitleaks runs.
- **Merge commits:** `-m` is required. A disposable-repository test confirmed that a secret introduced only in a merge-conflict resolution is missed without `-m` and detected with `-m`.
- `.gitleaks.toml`, all existing rules, and all existing verified allowlists are unchanged. No broad exclusions, no disabled rules, and no security-check bypass were introduced.

### Validation

- Pinned Gitleaks 8.30.1 was downloaded and its `--log-opts` support verified.
- Current PR range `origin/development..HEAD -m`: **no leaks found**.
- Full-history scan still finds the unrelated realtime-branch fixture, proving the protected-branch/full-history path remains effective.
- New workflow test `tests/scripts/ci_secret_scan_test.go` checks the three scopes, the PR/feature range arguments, fail-closed SHA checks, the removal of `github.event_name != 'pull_request'`, and the absence of a realtime fixture allowlist.
- Dynamic test `TestGitleaksMergeOnlySecretDetected` confirms `-m` catches a merge-only secret.
- `go test ./tests/scripts/...` passes; the dynamic Gitleaks tests are skipped unless `GITLEAKS_BIN` is supplied.

### Syft/SBOM status

- The external Syft v1.56.0 download continued to return HTTP 504 during this follow-up.
- All eight Docker/SBOM jobs in run `42465547430` failed at the Syft installer; Docker builds themselves completed.
- SBOM, Trivy and Syft checks were not changed or bypassed. Rerun the Docker/SBOM jobs when the external asset is available.

### Access tests

- Access tests passed in the previous completed run (`14m55s`).
- The new run after this policy change is the authoritative result and must be observed before merging.

### Remaining blockers and recommendation

- Gitleaks/Security gate: expected to pass for PR #106 after this change because the PR range excludes the unrelated realtime branch.
- Docker/SBOM: still blocked by the external Syft HTTP 504.
- Required CI gate remains red while any required check is red.

PR #106 remains open, mergeable and **not ready to merge** until the Docker/SBOM checks and the resulting required gate pass. No merge was performed.
