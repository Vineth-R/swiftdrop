# M5.1 acceptance verification — November 1, 2026

**Result: M5.1 complete against the acceptance criteria supplied by the user.**
Branch: `feature/m5-parcel-creation`. Baseline migrations and other members'
implementation are preserved. This report records local verification before the
user-authorized commit, push and pull request.

No authoritative numbered member task sheet is present in the repository. The
scope below uses the user's M5.1 list and the actual DB v1.1 design, rather than
assuming assignments from the API specification. M5.2 and M5.3–M5.5 are outside
this verification.

## Environment and evidence

- Actual database: **PostgreSQL 16.15**, official EDB Windows portable binaries,
  in a fresh, private loopback cluster under ignored `test-output/`. The existing
  PostgreSQL 18 Windows service was not used for acceptance or changed.
- Go **1.27.3**, Windows/amd64, `CGO_ENABLED=1`, existing MSYS2 UCRT64 GCC **16.1.0**
  selected through `CC` and process-local `PATH`. Race detection executed on this
  supported Windows configuration; it was not replaced with a non-race run.
- Existing golang-migrate CLI **v4.18.3**; real bootstrap SQL and migration files.
  Every database fixture checks the server major version and refuses anything
  other than PostgreSQL 16 before bootstrapping its generated database.
- Audit root: `test-output/m5-final-verification-88cb1b1969c3461a8ad37c162ea35ccf/`.
  It contains JSON test events, commands/exit codes, toolchain settings, readiness,
  red regression output, download provenance, and cleanup evidence. These are
  local ignored artifacts, not files intended for source control.

Final unit race run: **28 top-level tests, 73 passing test/subtest events**.
Final tagged race run: **16 top-level tests, 101 passing test/subtest events**,
including **13 database-backed top-level tests**. Both runs have **zero failed
tests, zero skipped tests, and zero race warnings**. The Parcel main package has
no unit test files; this is not a skipped test. The remaining tagged tests are
the ordinary integration-package wrapper and database-URL isolation unit tests.

## Acceptance checklist

| Criterion                                   | Status | Executed evidence                                                                                                                                                                                                                                  |
| ------------------------------------------- | ------ | -------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| PostgreSQL 16 connectivity                  | PASS   | Built Parcel binary: `/ready` HTTP 200, `checks.database=ok`, server 16.15, clean Parcel migration version 4. Startup uses `apps/services/parcel/main.go` and the existing shared pgx pool.                                                        |
| `parcels` migration                         | PASS   | Existing `migrations/parcel/000002_tables.up.sql`; actual CLI up, no-change, rollback, and reapply through version 4 in `TestParcelMigrations`. No duplicate table migration.                                                                      |
| `parcel_status_history` migration           | PASS   | Same baseline, same lifecycle test; all five Parcel tables checked.                                                                                                                                                                                |
| Exactly one owner                           | PASS   | `TestParcelOwnerAndMetadataConstraints`: customer-only and merchant-only accepted; neither/both rejected by `parcels_one_owner`; owner/tracking prefix checked.                                                                                    |
| Per-customer idempotency                    | PASS   | `TestParcelIdempotencyConstraints` and repository duplicate/concurrency tests: duplicate pair rejected; different customers, different keys, and nullable keys supported. Concurrent requests produce one winner and one duplicate.                |
| Delivery/failure metadata                   | PASS   | Required `delivered_at`, `failed_at`, `delivered_to_name`, `failure_reason`; invalid combinations rejected and valid values round-trip through the repository.                                                                                     |
| Reporting indexes                           | PASS   | `TestParcelReportingIndexes` checks all ten documented Parcel/history indexes and column order against PostgreSQL catalogs.                                                                                                                        |
| History relationship and integrity          | PASS   | `TestParcelHistoryConstraints`: orphan/history deletion/updates rejected, failed history needs a reason; history FK and status-transition trigger verified.                                                                                        |
| Repository layer                            | PASS   | `apps/services/parcel/internal/repository`: real pgx pool and caller-owned transaction, parameterized writes, exact decimals, nullable fields, scoped lookups, history ordering, duplicate error translation.                                      |
| Atomic repository transactions              | PASS   | `TestParcelRepositorySharesCallerTransaction`: Parcel + initial history + baseline outbox fixture are invisible before commit; commit retains all, rollback retains none.                                                                          |
| DB v1.1 compatibility                       | PASS   | Baseline table definitions match `docs/db.sql` and DB v1.1 sections 5.1/5.2. Forward migration `000004` fixes the documented price-total CHECK's JSON-null bypass; its regression cases pass.                                                      |
| M6 assignment/outbox database compatibility | PASS   | Assignment FK/current-assignment/reassignment tests pass. Shared transaction test uses existing outbox storage labels; no outbox contracts or columns changed.                                                                                     |
| Complete migration-chain compatibility      | PASS   | `TestParcelServiceMigrationCompatibility` starts before the Parcel schema exists, applies the real five-service order twice, checks 26 tables and clean versions: identity 2, pricing 2, parcel 4, merchant 6, notification 2.                     |
| M7 integration boundary                     | PASS   | Existing startup/readiness integrates PostgreSQL. Repository accepts the shared pool and `pgx.Tx`. Business handler, auth, pricing and publication wiring are not requirements in the supplied M5.1 list; those future tasks were not implemented. |
| Race detection                              | PASS   | Both complete focused scopes ran with `-race`, CGO and GCC enabled, including concurrent idempotency and live PostgreSQL tests.                                                                                                                    |
| Focused build, vet and formatting           | PASS   | Executed commands below; no M5 compilation, vet, formatting or test failures remain.                                                                                                                                                               |

## Defect fixed without rewriting history

DB v1.1 section 5.1 explicitly requires `price_amount` to equal the sum of
`base_fee`, `weight_charge`, and `size_surcharge` (design lines 1234–1244).
The baseline CHECK accepted JSON-null components because SQL comparisons then
evaluate to NULL and PostgreSQL CHECK accepts unknown.

The regression test was first run against the baseline and failed for each of
the three JSON-null components, with expected SQLSTATE `23514` but no error.
`000004_price_breakdown_total.up.sql` wraps the existing expression with
`IS TRUE`, validates existing rows, and preserves the constraint name and all
columns. Its transaction rolls back if validation fails; it does not repair or
delete data. The down migration restores the baseline CHECK. The complete final
race suite then passed, including matching, missing, mismatched and null
components and the migration round trip.

Owners must agree on remediation if an existing deployment contains invalid
price rows. No shared deployment was inspected or migrated here. Coordinate new
Parcel migration versions from the next unused version, currently `000005` after combining M6's changes with
this branch. Applied `000001`/`000002` and `docs/db.sql` remain unchanged.

## Executed commands

Run from the repository root. `$audit` below names the audit root above;
`$state` is its recorded private cluster configuration. `-p 1` limits build
parallelism; `-count=1` prevents cached test results. No test assertions or race
checks were disabled.

```powershell
$env:CGO_ENABLED = '1'
$env:CC = 'C:/msys64/ucrt64/bin/gcc.exe'
$env:PATH = 'C:/msys64/ucrt64/bin;C:/Users/supes/go/bin;' + $env:PATH
$env:GOMEMLIMIT = '1500MiB'
$env:GOGC = '50'
$env:PARCEL_TEST_DATABASE_URL = 'postgres://m5_test_admin@127.0.0.1:' + $state.Port + '/postgres?sslmode=disable'
$env:TEST_DATABASE_URL = $env:PARCEL_TEST_DATABASE_URL
go env -json GOVERSION GOOS GOARCH CGO_ENABLED CC
go test -p 1 -race -count=1 -json ./apps/services/parcel/... ./packages/go-common/postgres ./tests/integration/parceltest
go test -p 1 -race -count=1 -json -timeout=5m '-tags=integration,postgres' -run '^TestParcel' ./tests/integration/... ./apps/services/parcel/internal/repository
go vet '-tags=integration,postgres' ./apps/services/parcel/... ./tests/integration/...
go build -p 1 -o (Join-Path $audit 'parcel.exe') ./apps/services/parcel
```

All commands above exited 0. JSON events are saved in `unit-race.jsonl` and
`postgresql16-race.jsonl`; `checks.json` records exit codes and `test-summary.json`
records counts. `run-verification.ps1` contains the exact execution script.
The following checks also executed:

```powershell
# Regression before the forward migration: FAIL, as expected (three null cases).
go test -p 1 -count=1 -v '-tags=integration,postgres' -run '^TestParcelPriceBreakdownConstraints$' ./tests/integration
# Existing envelope behavior: PASS with race detection.
go test -p 1 -race -count=1 -run '^TestEnvelopeValidateEventTypeVersion$' ./packages/go-common/messaging
# Match the configured Nx build's trimpath option: PASS.
go build -p 1 -trimpath -o (Join-Path $audit 'parcel.exe') ./apps/services/parcel
# Parse actual CI YAML and assert PG16, CGO, both complete race scopes: PASS.
go run -p 1 (Join-Path $audit 'check-workflow.go')
# Create/migrate a generated PG16 probe DB, run Parcel, request /ready, clean up: PASS.
& (Join-Path $audit 'probe-readiness.ps1')
# Raw Windows gofmt lists 11 unchanged tracked files due solely to CRLF (see below).
gofmt -l apps/services/parcel packages/go-common/postgres tests/integration/parceltest tests/integration/parcel_database_test.go
git diff --check
python (Join-Path $audit 'review-source.py')
```

The pnpm shim cannot launch its cached pnpm executable, and workspace
`node_modules/nx` is absent. Inspected `apps/services/parcel/project.json` targets
and ran their underlying Go test/build/format scopes directly, extending them
for database/race acceptance. No JavaScript workspace dependencies were installed.
The existing pnpm 12.15.0 entry point was used for Prettier 3.8.1:

```powershell
node 'C:/Users/supes/AppData/Local/node/corepack/v1/pnpm/12.15.0/bin/pnpm.mjs' dlx prettier@3.8.1 --check .github/workflows/m5-parcel-database.yml apps/services/parcel/README.md docs/m5-parcel-database-verification.md
```

The raw broad `gofmt -l` is not empty: this checkout uses
`core.autocrlf=true`, and 11 unchanged tracked Go files have CRLF endings. The
source-review script compares formatter output after normalizing line endings
and confirms no code-style differences. All 11 new M5 Go files pass raw gofmt;
tracked files were preserved. Whitespace checks also include every untracked
source addition, because ordinary `git diff --check` alone does not inspect them.

## Remaining integration and environment issues

| Issue / owner                                                                                                | Status                                                                                                                                                                          | Blocks M5.1?                                                               |
| ------------------------------------------------------------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------- |
| Docker Desktop / local environment                                                                           | BLOCKED: daemon API returns HTTP 500; guest-service sockets refuse connections and WSL guest execution times out. Native isolated PG16 supplied actual target-version coverage. | No                                                                         |
| Outbox publication contract / M6 with M1 messaging and consumer owners, per the user's ownership description | Pending coordination; details below.                                                                                                                                            | No, under supplied M5.1 criteria; resolve before publishing Parcel events. |
| Future business integration / assigned application owners including M7                                       | Not implemented here; database/repository integration is verified.                                                                                                              | No                                                                         |
| Prepared GitHub workflow / branch maintainer                                                                 | PostgreSQL 16, CGO and both race commands reviewed and YAML checked. No GitHub run was executed or claimed.                                                                     | No; actual local checks passed.                                            |
| Windows worker portability / notification and webhook worker owners                                          | Existing `syscall.Kill` compilation errors, separate from Parcel.                                                                                                               | No                                                                         |
| Official member task sheet / team                                                                            | Missing; explicit user acceptance list used.                                                                                                                                    | No for that list; reassess if an additional signed criterion is supplied.  |

### Outbox contract trace and proposed coordination

`migrations/parcel/000002_tables.up.sql` stores `parcel.created`,
`parcel.assigned`, `parcel.status_changed` and maps the latter to routing key
`parcel.status.changed`. DB v1.1 section 5.5 and API v1.1 section 11 document these
legacy unversioned names; the API also illustrates a flat camelCase envelope.

For the **current implemented bus**, `contracts/events/README.md`,
`packages/go-common/messaging/envelope.go` and the supplied agent guide require
canonical versioned names and matching versions, snake_case metadata, producer
and nested object data. A rollback-only PostgreSQL 16 probe confirms that
`parcel.created.v1` currently violates the outbox's CHECK (SQLSTATE `23514`), while
the shared envelope tests confirm unversioned names are invalid for the bus.

No Parcel event producer or outbox dispatcher exists. The shared publisher
implements retry/dead-letter routing. Actual worker handlers accept only
`notification.requested.v1` and `webhook.delivery.requested.v1`; they are not Parcel
event consumers. Appending `.v1` to SQL alone cannot settle the envelope,
status-name spelling, conversion into worker requests, and queue bindings.

Proposed owner decision: preserve storage labels and add an explicit dispatcher
mapping into agreed versioned bus contracts, or coordinate a new forward
migration plus a pending-row migration policy if storage itself must change.
Agree on the canonical status event spelling and payload/consumer contracts in
either case. Do not rewrite the applied baseline. This change is pending M6/
platform/consumer coordination, outside M5's two-table repository acceptance.

### Safe Docker diagnosis and unrelated worker failure

Executed safe checks: `docker info --format
'ServerVersion={{.ServerVersion}} OperatingSystem={{.OperatingSystem}}'`,
`wsl --version`, `wsl --list --verbose --all`, WSL guest execution, and recent
`com.docker.backend.exe.log` entries. WSL lists `docker-desktop` as Running v2,
but guest execution times out (`Wsl/Service/0x8007274c`); Docker reports 500 at
`/v1.56/info`, with refused `docker.proxy.sock`/`stats.sock` and EOF ping in host
logs. This does not prove an API-version mismatch. No reset, service restart,
distribution unregister, Docker volume deletion or global configuration change
was attempted during this follow-up.

`go test -p 1 -run '^$' ./apps/services/notification-worker
./apps/services/webhook-worker` fails on Windows at each worker's `main.go:62`
(`undefined: syscall.Kill`). The focused Parcel build and vet pass. Those existing
worker files were not modified, and a whole-repository Windows build is not
claimed to pass.

## Follow-up changes and cleanup

Modified existing prepared M5 files:

- `.github/workflows/m5-parcel-database.yml`: explicit CGO and complete unit race scope.
- `apps/services/parcel/README.md`: scope, migration correction and integration handoff.
- `tests/integration/parcel_database_test.go`: complete service-chain, price regression and migration version coverage.
- `tests/integration/parceltest/database.go`: PostgreSQL 16 guard and full-chain fixture support.

Added:

- `migrations/parcel/000004_price_breakdown_total.up.sql`
- `migrations/parcel/000004_price_breakdown_total.down.sql`
- `docs/m5-parcel-database-verification.md`

The previously prepared repository implementation/tests and URL-isolation
helpers were preserved. The tracked working tree and baseline migrations remain
unchanged during verification. Final review checks both
tracked changes and untracked M5 source files. Generated binaries, portable
PostgreSQL, test databases and logs remain excluded by `test-output/` ignore rules.

All generated test/probe databases were removed from the dedicated cluster and
that cluster was stopped. Existing PostgreSQL 18 remains running. No existing
database volumes or shared data were deleted. Local ignored evidence is retained
for inspection.

## Pre-submission compatibility refresh

Before publication, fetched `origin/development` at
`7761bc365783e3f73b3b694dd884482b8676bb3e`. It now includes M6's applied
`000003_dispatch_foundations`, adding assignment reassignment/audit metadata and
outbox aggregate metadata. M5's correction was still unpublished and had been
used only in disposable test databases; it was moved to `000004` without changing
M6's migration or rewriting any applied team migration. Subsequent migration
numbers must start at the next unused version, currently `000005`.

Migration rollback tests inspect actual applied versions so both the feature
branch and the combined chain `000001` → `000002` → M6 `000003` → M5 `000004`
are covered. The M5 workflow also supplies M6's `TEST_DATABASE_URL` alongside
`PARCEL_TEST_DATABASE_URL` so recursive database coverage does not silently skip
M6 tests. An ignored export of the exact development commit with only M5's
source additions overlaid is used for combined verification; the working branch
is not switched, merged, or rebased.

Refreshed feature results: 28 top-level unit tests and 16 top-level tagged tests
passed with race detection, plus focused vet, build and live `/ready` on
PostgreSQL 16.15 at migration version 4. Combined-development results: **41
top-level unit tests (152 passing test/subtest events)** and **22 top-level tagged
tests (143 passing events)** passed, including M6's matching dispatch database
tests. Both combined scopes had zero failures, skipped tests or race warnings;
focused combined vet and the trimmed Parcel build also passed. The five-service
chain remained at 26 business tables with clean Parcel version 4. The exact
runner is `verify-combined.ps1` and results are in `combined-checks.json`,
`combined-unit-race.jsonl`, `combined-postgresql16-race.jsonl` and
`pre-submission-summary.json` under the ignored audit root above.
