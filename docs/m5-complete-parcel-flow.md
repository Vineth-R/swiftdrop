# Member 5 parcel flow: implementation and verification

Verification dates: 6?7 November 2026. Working branch:
`feature/m5-complete-parcel-flow`; target: `development`. This record supplements
the [merged M5.1 database record](m5-parcel-database-verification.md), rather than
replacing the team design report.

## Latest integration: 7 November

Development `b6b49ca` added M6 dispatch APIs, the transactional outbox relay and
dispatcher screens after the earlier verification. These have been merged into
this branch, preserving both customer and dispatcher behavior at `/parcels`.
The relay is now implemented, superseding the historical dependency status below.
The original 511-test result and 83.75% M5 coverage describe the 6 November source;
post-merge verification is recorded separately before publication.

## Sources and baseline

Both supplied PDFs were available in the local Downloads directory and read:
`SwiftDrop_Remaining_Tasks.pdf` and `SwiftDrop-UI-Foundation-Guide.pdf`. The former
audits 2 November's commit `2dbbbb8`; its statuses were treated as historical.
The API/DB v1.1 documents, `DESIGN.md`, `AGENTS.md`, contribution instructions,
OpenAPI, event schemas, migrations and current source/tests were also inspected.

The initial clean checkout was `5c47fa7` (merged M5.1). On resuming after the
usage interruption, a fresh fetch found 12 additional merged commits. A clean
fast-forward to `bc7d02d` preserved every uncommitted M5 file and brought in the
latest reports and webhook URL validation work. No teammate files conflicted.

Already implemented and reused:

- M5.1 parcel schema, repository, constraints and append-only history tests.
- M6 dispatch/outbox migrations and state machine, plus the newly merged dispatch APIs, relay and dispatcher screens; no migrations were rewritten.
- M4 real pricing estimate/quote, zones, admin rules and public tracking.
- M1/M2/M3 shared HTTP/errors/auth/internal-token middleware, Identity APIs,
  notification consumers, API refresh handling, authenticated shell and UI kit.
- M2/M10 reports and role navigation; M7 courier history UI. No dispatcher parcel workspace existed at the initial checkout; the new M6 workspace is preserved by the final merge.
- M8's schema/service foundation. There was no integrated merchant order caller.

The interrupted work had already added the main API/UI implementation, isolated
tests and native real-service journeys. Final coverage, production browser
verification, Docker application checks and all Git publishing steps were still
unfinished. The resume did not discard or regenerate the implementation.

## Requirement matrix

Implementation status is separate from verification status. COMPLETE means the
M5 implementation exists; dependent flows can still be BLOCKED below.

| Requirement                                           | Initial audit | Implementation and evidence                                                                                                                   |
| ----------------------------------------------------- | ------------- | --------------------------------------------------------------------------------------------------------------------------------------------- |
| M5.1 schema/repository and owner constraints          | COMPLETE      | Reused unchanged; migration/repository PostgreSQL tests.                                                                                      |
| Authenticated customer POST and full input validation | MISSING       | COMPLETE: authenticated owner, decimal/UUID/length/phone/size checks, strict JSON and body limit; NUL text rejected before I/O.               |
| Real M4 quote and numeric `PRICE_CHANGED`             | MISSING       | COMPLETE: internal HTTP client, stored exact breakdown, current-fee response; strict upstream response validation.                            |
| Customer-scoped replay and concurrent duplicates      | MISSING       | COMPLETE: transaction advisory lock, unique constraints, same-key payload comparison; expectedFee excluded from successful-replay comparison. |
| SDK/SDN tracking and collision handling               | PARTIAL       | COMPLETE: eight unbiased cryptographic characters, unique index and bounded savepoint retry.                                                  |
| Atomic parcel/history/outbox persistence              | PARTIAL       | COMPLETE: one transaction, rollback/restart/fee-stability tests, existing append-only constraints preserved.                                  |
| Customer list/filter/sort/pagination                  | MISSING       | COMPLETE: owned SQL queries, inclusive UTC dates, bounded pagination and stable ordering.                                                     |
| Detail/history/ownership-safe 404                     | MISSING       | COMPLETE: ownership in SQL, documented staff roles, chronological append-only history, common errors/request IDs.                             |
| Downloadable original-fee PDF                         | MISSING       | COMPLETE: stored breakdown, tracking/contact/delivery data and no repricing; Unicode shaping and independently inspected PDFs.                |
| Internal merchant create/summary/token guard          | MISSING       | COMPLETE: SDN, merchant/order replay, mandatory audit actor and fail-closed internal token; gateway excludes internal endpoints.              |
| `/new` wizard                                         | MISSING       | COMPLETE: shared form/kit, live estimate/review, stable key/retry, PRICE_CHANGED reconfirmation, safe loading/error/success states.           |
| `/parcels` and `/parcels/{id}`                        | MISSING       | COMPLETE: customer list/details, filters/pages, timeline, stored fee, PDF download; role-aware shared route.                                  |
| Responsive/accessibility checks                       | MISSING       | COMPLETE implementation; explicit 360px/1280px browser checks and screenshots.                                                                |
| M6 bus relay and notification side effect             | BLOCKED       | COMPLETE after merging M6: versioned relay and customer notification verification are recorded in the 7 November addendum.                    |
| M8 merchant order-to-parcel caller                    | BLOCKED       | Internal API tested; actual merchant order caller/approval integration remains M8 work.                                                       |
| M7 courier/status lifecycle                           | Outside M5    | Existing code preserved; no claim of courier lifecycle completion.                                                                            |

## Contracts and integration decisions

The external prefix is `/api/v1`; Gin registers root resource paths. Implemented
operations are POST/GET `/parcels`, GET `/parcels/{parcelId}`, its `/history` and
`/receipt`, POST `/internal/parcels`, and GET
`/internal/parcels/{parcelId}/summary`. Exact schemas/roles/envelopes are in
[`parcel.yaml`](../contracts/openapi/parcel.yaml).

Customer identity comes exclusively from authenticated claims. Another customer's
detail, history or receipt returns the same 404 as a missing record. Authenticated
responses are not cached. Internal calls require the shared token. Both
`/internal` and `/api/v1/internal` are denied by the gateway; internal service
ports are not browser endpoints.

Pricing is real `/internal/pricing/quote`, with bounded timeout, cancellation,
request ID propagation and redirects disabled. Fees are stored as exact decimal
LKR amounts, including each component. A rule change cannot reprice an existing
parcel or receipt. Invalid/missing upstream fields fail closed rather than
silently becoming zero. Test fakes are confined to isolated tests.

Customer replay is scoped by owner/key; a PostgreSQL transaction advisory lock
covers replay, quote and commit across instances. Different delivery fields return
409 `IDEMPOTENCY_CONFLICT`; expectedFee is consent metadata. PRICE_CHANGED writes
nothing, allowing the same intended request/key to be reconfirmed. The browser
preserves entered data and the key after recoverable failures; ambiguous network
outcomes keep delivery fields fixed until the intended submission is resolved.

Internal merchant creation requires `createdByUserId`: the verified merchant
owner's Identity UUID. DB v1.1 requires this audit field; a merchant UUID must not
be substituted for a user. The previously planned contract was completed in API
v1.1/OpenAPI. M8 must supply and verify that owner when its caller is implemented.

Outbox storage uses `parcel.created` for both label/key and the versioned schema's
data object, with exactly one owner and no phone/address/email. M6's relay must
wrap the data using the outbox UUID as event ID and publish `parcel.created.v1`.
It must not copy the stored legacy routing key. No relay or historical migration
was changed. The current M1 consumer supports customer-owned domain events;
merchant-owned notification events remain unsupported by that consumer.

Receipts bundle five OFL-licensed Noto fonts and use pure-Go shaping for Latin,
Sinhala, Tamil, Arabic and Hebrew, including combining marks and bidi text.
Searchable text retains original code points. Font hashes, source, license,
coverage limits and optional independent parser/rendering commands are recorded
in [receiptfonts/README.md](../apps/services/parcel/internal/parcels/receiptfonts/README.md).
Universal CJK/emoji rendering is not claimed.

## Reproduction and verification

Use the project's locked pnpm/Node versions and existing Nx targets:

```sh
pnpm install --frozen-lockfile
pnpm exec nx test parcel
pnpm exec nx run-many -t lint test typecheck --projects=@org/web,@org/web-e2e
pnpm exec nx build @org/web
node --test 'apps/web/src/lib/*.test.mjs' 'tests/contracts/*.test.mjs'
```

The Windows pnpm shim referenced a missing entry point. Verification used the
already installed Node 24.22.1 and Corepack-cached pnpm 12.15.0 through an ignored
local wrapper; no package or lockfile changes were needed. Nx daemon/plugin
isolation were disabled for this host. Playwright used installed Chrome with
`--disable-gpu` because bundled Chromium was unavailable and the host compositor
stalled. These are verification settings, not product changes.

For real PostgreSQL fixtures, set `PARCEL_TEST_DATABASE_URL` and
`TEST_DATABASE_URL` to a disposable maintenance server with CREATEDB, and install
the project's pinned migrate CLI. Fixtures create/drop generated databases only.
Windows race tests used GCC from MSYS2 and `CGO_ENABLED=1`.

```sh
go test -p 1 -race -count=1 -tags=integration,postgres \
  -covermode=atomic -coverpkg=./apps/services/parcel/... \
  -coverprofile=test-output/m5-complete/parcel-final-validated-coverage.out \
  ./apps/services/parcel/...
node scripts/check-m5-coverage.mjs test-output/m5-complete/parcel-final-validated-coverage.out
go test -p 1 -race -count=1 -tags=integration,postgres -run '^TestParcel' \
  ./tests/integration ./tests/integration/parceltest
go test -p 1 -race -count=1 -tags=integration ./tests/integration/database/...
```

The coverage gate counts each M5 statement block once even when `-coverpkg`
repeats blocks across binaries, using the maximum hit count. Its denominator
includes startup/configuration, M5.1 repository, the new parcel services and
customer/internal handlers; other members' tracking/reports/dispatch/proof code
is excluded. The CI workflow enforces at least 60%; whole-service coverage is a
separate figure. Partial profiles from failed runs are not accepted as evidence.
Frontend test counts are reported separately; no combined Go/TypeScript or frontend
coverage percentage is claimed.

See [real-service journey instructions](../tests/integration/parcelflow/README.md)
for reproducible opt-in API/browser commands. They register real customers, call
Identity/Pricing/Parcel/PostgreSQL, check cross-customer denial and download real
receipts. Optional admin credentials enable actual rule mutation/restoration.
Mocked Playwright screens are reported separately from these journeys.

### 6 November executed results (before the M6 merge)

PASS means executed successfully; FAIL preserves an observed failed attempt;
BLOCKED names an unavailable dependency; NOT TESTED makes no verification claim.

| Check                                                  | Result     | Evidence / limitation                                                                                                                                                                                                                                              |
| ------------------------------------------------------ | ---------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| Final Parcel unit + real PostgreSQL race suite         | PASS       | Nine packages completed, no failing packages. Includes concurrency, rollback, restart/pool persistence, fee stability, merchant creation and HTTP ownership/token/error cases.                                                                                     |
| Member 5 statement coverage                            | PASS       | 804/960 statements = 83.75%; minimum 60%. Final validated profile only. Whole Parcel service:85.5% using `go tool cover -func`.                                                                                                                                    |
| Final Parcel vet / build                               | PASS       | go vet -p 1 ./apps/services/parcel/... and go build -p 1 ./apps/services/parcel/... both exit0.                                                                                                                                                                    |
| NUL and raw column-limit regressions                   | PASS       | Observed red then green:32 HTTP cases (20 NUL, 12 raw-length), 24 raw-length service cases, NUL service cases, positive Unicode/newline/tab and exactmaximum inputs.                                                                                               |
| Frontend TypeScript                                    | PASS       | Fresh app, unit and E2E tsconfig checks each exit0 using pnpm exec tsc --noEmit -p <config>.                                                                                                                                                                       |
| Frontend Vitest                                        | PASS       | 53/53 tests in 5 files, including 11 M5 validation cases; 2.33s.                                                                                                                                                                                                   |
| Frontend lint                                          | PASS       | pnpm exec eslint . in each exact project cwd: web 0 issues; E2E 0 errors / 39 pre-existing warnings.                                                                                                                                                               |
| Production web Docker build                            | PASS       | Dockerfile runs pnpm exec nx build @org/web: compile, TypeScript and 22 static pages; image `81668a6ad2c3`. Includes `/new`, `/parcels`, `/parcels/[id]`.                                                                                                          |
| Final Parcel Docker build                              | PASS       | Linux build succeeded with final Unicode/pricing/NUL/rawlength changes; image `dd5456a73868`.                                                                                                                                                                      |
| Shared API and event contract Node tests               | PASS       | 89 passed, zero failed/skipped on resumed checkout.                                                                                                                                                                                                                |
| Broker-backed consumer tests                           | PASS       | 8 passed, zero failed/skipped, 12.766s; real isolated RabbitMQ. Does not prove the unimplemented relay.                                                                                                                                                            |
| Parcel migration/repository tests                      | PASS       | Tagged race suite completed against isolated PostgreSQL 16.                                                                                                                                                                                                        |
| Complete dispatch database suite                       | PASS       | Tagged race suite completed successfully, 27.089s.                                                                                                                                                                                                                 |
| Native real-service customer journey                   | PASS       | Real registration/estimate/create/list/detail/PDF and ownership denial; 360px/1280px artifacts. Before final receipt/validation refinements; final Docker results recorded separately.                                                                             |
| Native real pricing mutation journey                   | PASS       | Actual ADMIN API changed/restored rule; stale expectedFee rejected, original parcel/replay/PDF fee retained.                                                                                                                                                       |
| Independent Unicode PDF inspection                     | PASS       | 8 samples: strict parse, exact original names and 550.00 fee with two PDF parsers, rasterization and visual comparison with native font shaping.                                                                                                                   |
| Parcel vulnerability scan                              | PASS       | govulncheck ./apps/services/parcel/...: 0 reachable or imported-package vulnerabilities; 3 module advisories concern code not called by this scope.                                                                                                                |
| Secret scan of intended files                          | PASS       | Gitleaks 8.30.1, redacted report; no leaks. Runtime state/secrets excluded from commits.                                                                                                                                                                           |
| OpenAPI parse/references                               | PASS       | YAML parsed; all 114 local references resolved.                                                                                                                                                                                                                    |
| Scoped formatting and whitespace                       | PASS       | Changed supported files pass Prettier; git diff --check passes. Historical API PDF-to-Markdown document already fails baseline formatting and was not broadly rewritten.                                                                                           |
| Earlier interrupted coverage attempt                   | FAIL       | One fixture pool-open timeout during concurrent heavy builds; partial profile excluded. Final rerun above passed without skipping or weakening the test.                                                                                                           |
| Initial full browser run on dev server                 | FAIL       | 7 recorded navigation/StrictMode failures; interrupted. Final production rerun passed all 511 checks without weakening those tests.                                                                                                                                |
| Resumed native Nx build                                | NOT TESTED | Interrupted during graph inference under severe host memory pressure; no compile result. Final Docker build provides production Nx build evidence. Direct project tools used for final host lint/types/unit checks.                                                |
| Whole-repository Windows Go vet/build                  | FAIL       | Existing workers reference Linux-only syscall.Kill at notification main:129 and webhook main:61. No unrelated workaround added.                                                                                                                                    |
| Initial Docker migration wrapper                       | FAIL       | CRLF checkout shebang caused /usr/local/bin/migrate-all: no such file or directory. Resolved as below.                                                                                                                                                             |
| Same migrations, ephemeral LF normalization            | PASS       | All 5 schemas clean: identity 3, pricing 2, parcel 5, merchant 6, notification 2; 26 base tables. No applied migration edited.                                                                                                                                     |
| Documented migration command after checkout correction | PASS       | docker compose run --rm --entrypoint /usr/local/bin/migrate-all migrate up: 5 clean no-change results. Restored tracked LF bytes under existing .gitattributes; no source content change.                                                                          |
| M6 relay to notification delivery                      | BLOCKED    | Relay unavailable; no email-on-create or full downstream event-flow claim.                                                                                                                                                                                         |
| M8 actual order caller                                 | BLOCKED    | Caller/merchant approval flow unavailable; internal API has isolated coverage.                                                                                                                                                                                     |
| Container vulnerability/SBOM release checks            | NOT TESTED | Local dependency/secret checks do not substitute for existing remote image-security jobs.                                                                                                                                                                          |
| Remote CI / production deployment                      | NOT TESTED | Local results do not imply remote CI success; no production systems touched.                                                                                                                                                                                       |
| Real Docker customer API journey                       | PASS       | No mocks: eight concurrent creates produce one parcel; real ADMIN rule mutation/restoration, 409 currentFee, original replay/detail/PDF fee, ownership denial, filters/history and internal gateway guards. Updated redaction harness rerun also passed in 3.699s. |
| Real Docker internal merchant journey                  | PASS       | Direct isolated service-network calls: real M4 quote, PostgreSQL SDN creation/replay/summary and missing/wrong token rejection. No public internal-route exposure.                                                                                                 |
| Actual Parcel container restart                        | PASS       | Original detail and PDF byte-identical after process restart; stored total 550, one history row and one PENDING creation outbox row; persistent volumes retained.                                                                                                  |
| Real Docker customer browser journey                   | PASS       | Registration through estimate/create/list/detail/timeline/PDF, cross-customer 404, public tracking privacy and 360px/1280px; zero mocks, 11.22s. Updated diagnostic/screenshot harness rerun recorded before publishing.                                           |
| Real gateway access matrix / ownership                 | PASS       | Two top-level tests, 7.193s. 19 active endpoints checked anonymously and across five roles. Five planned M6/M7 child cases skipped; five M8 allowed-role checks remain pending, not counted as implemented passes.                                                 |
| Service log privacy/error inspection                   | PASS       | Seven service logs: no known fixture secrets/JWTs, recipient names, SQLSTATE, panic or Go ERROR entries; no Nginx errors. This does not erase the separate test-diagnostic incident below.                                                                         |
| Live-test error redaction                              | PASS       | Four standalone regression tests; Playwright call logs/credential headers and original diagnostic properties omitted, assertion failures retained. Added to M5 CI workflow.                                                                                        |
| Initial restart readiness probe                        | FAIL       | Transient ECONNRESET immediately after restart. A bounded readiness retry then passed; no service code or test assertion weakened.                                                                                                                                 |
| Full production Chromium regression                    | PASS       | 511/511 checks in 24 files, including all 16 M5 scenarios and 495 existing checks; zero failed/skipped/flaky; 386.068s. Direct Playwright against final Docker gateway, not the dev server.                                                                        |

### Docker notes

Docker Desktop was initially unavailable and later started for verification.
The isolated project `swiftdrop-m5-complete-verify` uses generated local-only
credentials, gateway port 18080, RabbitMQ port 15673, and its own persistent
volumes. Existing user `.env` and volumes are preserved. Make was unavailable,
so inspected Makefile-equivalent Compose commands were used. No database reset,
volume deletion or production operation was performed.

Infrastructure startup is `docker compose up -d --wait postgres rabbitmq mailpit`.
The first Windows migration attempt used a safe ephemeral LF-normalized copy.
The checkout was then corrected to the script's exact tracked LF bytes under its
existing `.gitattributes` rule; the documented wrapper passed without a source
patch or destructive reset.
Final application images, real journeys, restart persistence and access checks
are recorded separately above; infrastructure readiness alone was not counted
as an application pass.

Final host checks used the configured project tools directly after native Nx
inference stalled under memory pressure:

```sh
pnpm exec tsc --noEmit -p apps/web/tsconfig.json
pnpm exec tsc --noEmit -p apps/web/tsconfig.spec.json
pnpm exec tsc --noEmit -p apps/web-e2e/tsconfig.json
# Working directory apps/web:
pnpm exec vitest run
pnpm exec eslint .
# Working directory apps/web-e2e:
pnpm exec eslint .
# Workspace root, temporary config selecting existing production gateway:
pnpm exec playwright test --config tmp/m5/playwright-docker.config.mts --project chromium
node --test tests/integration/parcelflow/safe-errors.test.mjs
```

The ignored Playwright override imports the normal project configuration, disables
its dev `webServer`, sets `baseURL` to `http://127.0.0.1:18080`, keeps one worker,
and selects installed Chrome with `--disable-gpu`. It writes results under ignored
`test-output/m5-complete`. Firefox/WebKit were not executed. Production application
code has no browser/GPU override.

Authoritative final artifacts in the local ignored evidence directory include
`parcel-final-validated-{tests.log,coverage.out,measurement.log,functions.log}`,
`frontend-check-results.json`, `frontend-browser-summary.json`,
`frontend-browser-results.json`, `gitleaks-final.json`, and
`docker-live/{api-journey-result.json,restart-result.json,access-tests.jsonl,log-privacy-result.json}`.
Generated receipts/screenshots and fixture credentials are intentionally not Git
artifacts. CI recreates automated coverage/test evidence on the PR.

## Scope and report-owner notes

No shared app layout, authentication store, design tokens, applied migrations,
outbox relay, dispatcher writes, courier lifecycle or other-member business logic
was replaced. Shared API helpers gained stable request headers and an authenticated
PDF download helper with the existing refresh behavior. The app-shell browser
test was updated because `/parcels` is now a real screen; unknown routes still
exercise the existing not-found interface.

Existing as-built reports that describe M5 APIs/screens as future work are stale
after this change. Report owners should use this record and OpenAPI to update
their assigned report sections. Dispatcher/relay/courier/merchant-order claims
must retain their separate limitations.

## Exact change manifest

73 files relative to `bc7d02d`; A = created, M = modified.
No applied migrations, persistent volumes, shared app layout or generated outputs
are included. The LF checkout correction has no source diff.

```text
M .env.example
M .github/workflows/m5-parcel-database.yml
M AGENTS.md
M DESIGN.md
M README.md
M apps/services/parcel/README.md
M apps/services/parcel/internal/config/config.go
M apps/services/parcel/internal/config/config_test.go
A apps/services/parcel/internal/http/parcels.go
A apps/services/parcel/internal/http/parcels_test.go
A apps/services/parcel/internal/http/parcels_validation_test.go
A apps/services/parcel/internal/http/parcels_workflow_integration_test.go
M apps/services/parcel/internal/http/router.go
M apps/services/parcel/internal/http/tracking.go
A apps/services/parcel/internal/parcels/adapters_test.go
A apps/services/parcel/internal/parcels/courier.go
A apps/services/parcel/internal/parcels/model.go
A apps/services/parcel/internal/parcels/pricing.go
A apps/services/parcel/internal/parcels/pricing_contract_test.go
A apps/services/parcel/internal/parcels/receipt.go
A apps/services/parcel/internal/parcels/receipt_text.go
A apps/services/parcel/internal/parcels/receipt_unicode_test.go
A apps/services/parcel/internal/parcels/receiptfonts/NotoSans-Regular.ttf
A apps/services/parcel/internal/parcels/receiptfonts/NotoSansArabic-Regular.ttf
A apps/services/parcel/internal/parcels/receiptfonts/NotoSansHebrew-Regular.ttf
A apps/services/parcel/internal/parcels/receiptfonts/NotoSansSinhala-Regular.ttf
A apps/services/parcel/internal/parcels/receiptfonts/NotoSansTamil-Regular.ttf
A apps/services/parcel/internal/parcels/receiptfonts/OFL.txt
A apps/services/parcel/internal/parcels/receiptfonts/README.md
A apps/services/parcel/internal/parcels/service.go
A apps/services/parcel/internal/parcels/service_test.go
A apps/services/parcel/internal/parcels/store.go
A apps/services/parcel/internal/parcels/testdata/inspect_receipts.py
A apps/services/parcel/internal/parcels/validation_test.go
A apps/services/parcel/internal/parcels/workflow_integration_test.go
A apps/services/parcel/internal/repository/customer_reads.go
M apps/services/parcel/main.go
M apps/web-e2e/src/app-shell.spec.ts
A apps/web-e2e/src/parcels.spec.ts
A apps/web/src/app/(app)/new/page.tsx
A apps/web/src/app/(app)/parcels/[id]/page.tsx
A apps/web/src/app/(app)/parcels/page.tsx
A apps/web/src/components/parcels/contact-fields.tsx
A apps/web/src/components/parcels/delivery-form.spec.ts
A apps/web/src/components/parcels/delivery-form.ts
A apps/web/src/components/parcels/fee-breakdown.tsx
A apps/web/src/components/parcels/new-delivery-view.tsx
A apps/web/src/components/parcels/parcel-detail-view.tsx
A apps/web/src/components/parcels/parcel-problem.tsx
A apps/web/src/components/parcels/parcels-view.tsx
A apps/web/src/lib/api-download.test.mjs
M apps/web/src/lib/api-errors.ts
M apps/web/src/lib/api.ts
M apps/web/src/lib/http.ts
A apps/web/src/lib/parcels-api.ts
M contracts/events/README.md
M contracts/openapi/parcel.yaml
M docker-compose.yml
M docs/API_Documentation_v1.1.md
M docs/architecture.md
A docs/m5-complete-parcel-flow.md
M go.mod
M go.sum
M infra/nginx/default.conf
A scripts/check-m5-coverage.mjs
M tests/access/README.md
M tests/access/matrix_test.go
M tests/access/ownership_test.go
A tests/integration/parcelflow/README.md
A tests/integration/parcelflow/api-journey.mjs
A tests/integration/parcelflow/customer-journey.mjs
A tests/integration/parcelflow/safe-errors.mjs
A tests/integration/parcelflow/safe-errors.test.mjs
```
