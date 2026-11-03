# Contributing to SwiftDrop

SwiftDrop is a foundation-stage monorepo. Keep changes focused, preserve existing service boundaries, and do not describe planned business behavior as implemented.

## Before you start

1. Read [`AGENTS.md`](AGENTS.md) for repository rules and [`docs/architecture.md`](docs/architecture.md) for service boundaries.
2. Install the prerequisites from the [README](README.md#prerequisites).
3. Set up the workspace:

   ```sh
   cp .env.example .env
   pnpm install --frozen-lockfile
   ```

4. Run the relevant checks for the area you are changing before opening a pull request.

## Branch workflow

The current remote default branch is `chore/initial-workspace`. It is a shared coordination branch, not a permanent product mainline.

- Create feature branches from `chore/initial-workspace`.
- Do not push directly to the shared branch.
- Use a short descriptive branch name, for example `feature/parcel-create-endpoint`, `fix/worker-shutdown`, or `docs/onboarding`.
- When the team establishes a permanent mainline, rename or replace the shared branch deliberately, update the remote default, and protect the new mainline. Until then, do not assume a protected-branch setting or required-review rule is enforced.
- Rebase or merge from the shared branch only when needed to resolve conflicts. Do not rewrite other contributors' history.

## Commits

- Use a concise imperative subject line, optionally prefixed with the affected area, for example `messaging: reset backoff after reconnect` or `docs: clarify migration prerequisites`.
- Keep each commit coherent and reviewable. Do not mix unrelated refactors with behavior changes.
- Do not commit generated build output, local environment files, credentials, or logs.

## Pull requests

Open a pull request against `chore/initial-workspace` using [`.github/pull_request_template.md`](.github/pull_request_template.md). Include:

- A summary of the change and the problem it solves.
- A linked issue when one exists.
- The type of change and the affected services, packages, contracts, migrations, or infrastructure.
- Evidence that the required checks ran, including commands and outcomes.
- Rollout and rollback notes when the change affects deployment, messaging, data, or configuration.

Keep pull requests small enough to review. Split independent work. Update documentation and contracts in the same pull request as the implementation.

## Review and merge expectations

No enforced branch protection or required-review configuration is documented in this repository yet. The recommended team default is:

- At least one reviewer approves before merge.
- All configured CI checks pass for the pull request head.
- Review comments are resolved or explicitly deferred.
- The author has run the local checks listed below.
- Direct pushes to the shared branch are avoided.

A reviewer should verify behavior, tests, error handling, service ownership, secret safety, and documentation accuracy, not only style.

## Required local checks

Run the checks that match your change.

### All Go changes

```sh
gofmt -w apps/services packages/go-common tests/integration
go vet ./...
make test
go build ./...
```

`make test` runs `go test -race -count=1 ./...`, which is what CI runs. Database tests need `TEST_DATABASE_URL`; see `make test-db` and [`tests/integration/README.md`](tests/integration/README.md).

For affected Nx projects:

```sh
pnpm exec nx run-many -t lint test build \
  -p identity,parcel,pricing,merchant,notification-worker,webhook-worker,realtime,go-common
```

### Messaging changes

Unit tests must not require RabbitMQ. Run the broker-backed integration tests when changing the consumer, publisher, topology, retry, dead-letter, envelope, or framing behavior:

```sh
docker compose up -d rabbitmq
RABBITMQ_URL='amqp://swiftdrop:swiftdrop@localhost:5672/' \
  go test -tags=integration ./tests/integration/...
```

See [`tests/integration/README.md`](tests/integration/README.md). CI sets `RABBITMQ_URL` and fails if integration tests are skipped.

### Frontend changes

Read [`DESIGN.md`](DESIGN.md) before you build or change a screen, and follow it. It defines the design language, the allowed tokens, the component kit and the checklist to finish with. `nx lint @org/web` enforces the checkable parts and fails on a raw colour, an off-token class, an off-scale spacing value, an inline style and similar mistakes (`DESIGN.md` section 16).

```sh
node --test "apps/web/eslint-rules/*.test.mjs"   # only when you change the lint rules
pnpm exec nx lint @org/web
pnpm exec nx build @org/web
```

### E2E changes

```sh
pnpm exec nx lint @org/web-e2e
pnpm exec nx typecheck @org/web-e2e
pnpm exec playwright install chromium
pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"
```

CI runs Chromium only in this phase.

### Infrastructure changes

```sh
docker compose config --quiet
```

Run `nginx -t` when you change Nginx configuration directly. Start the full stack when the change affects routing or service health checks.

### Documentation changes

Run Prettier on the files you changed:

```sh
pnpm exec prettier --write path/to/changed-file.md
pnpm exec prettier --check path/to/changed-file.md
```

## Test results and coverage

Every service owner reports reproducible test results for their service. One command runs the Go tests and prints coverage per service and in total:

```sh
make coverage
```

It runs `go test -race` over the whole module, prints a table, writes it to `coverage-summary.md` and the raw profile to `coverage.out` (both ignored by git). Without a database the PostgreSQL tests are skipped. To include them, point `TEST_DATABASE_URL` at a throwaway server that may create databases (never the Compose database):

```sh
docker run -d --name sd-test-pg -p 127.0.0.1:5434:5432 -e POSTGRES_USER=swiftdrop -e POSTGRES_PASSWORD=swiftdrop -e POSTGRES_DB=postgres postgres:16-alpine
```

```sh
TEST_DATABASE_URL='postgres://swiftdrop:swiftdrop@127.0.0.1:5434/postgres?sslmode=disable' make coverage
```

CI runs the same script, shows the table on the run's summary page, uploads both files as the `go-coverage` artifact and fails when a service falls below its minimum. The minimums live in `COVERAGE_MIN` (default `identity=60`, the task sheet target). Pass your own gates for a local run, for example `COVERAGE_MIN="identity=60 parcel=70" make coverage`.

Paste this into the pull request description of any change that adds or changes tests, filling in the real numbers from your run:

```markdown
### Test results

- Service: <name>
- Commit: <short sha>
- Command: `make coverage` (with or without `TEST_DATABASE_URL`)
- Result: <all pass | N failing: names>
- Coverage: <service %> unit only, <service %> with PostgreSQL tests; total <%>
- Skipped tests and why: <none | list>
- Behaviour not covered yet: <list what you know is untested>
```

Only report what you ran. A skipped test is not a pass.

## Migrations

- Put migrations under the owning service's `migrations/<service>/` directory.
- Use `NNNNNN_short_description.up.sql` and `.down.sql`.
- Never edit an applied migration; add a new one.
- Start PostgreSQL before running migrations locally:

  ```sh
  docker compose up -d postgres
  ```

- No service connects to PostgreSQL yet. Do not add application database access or speculative business tables as part of an unrelated change.
- Down migrations may drop schemas. Confirm the target database before running them.
- Follow [`migrations/README.md`](migrations/README.md) for tooling and per-service migration version tables.

## Contracts and documentation

- When a public HTTP route changes, update the Gin route, `contracts/openapi/<service>.yaml`, and `infra/nginx/default.conf` together.
- When an event changes, update the schema under `contracts/events/` and respect versioning rules.
- When configuration changes, update `.env.example` and the relevant documentation.
- Mark planned functionality as planned.
- Keep the README, architecture document, contracts, migration guide, and agent guide consistent.

## Secrets policy

- Never commit real credentials, tokens, private keys, connection strings with passwords, or production data.
- `.env` is untracked. `.env.example` contains safe local placeholders only.
- Do not log secrets, connection strings, tokens, or personal data.
- Do not bake secrets into Docker images or CI configuration.
- Do not rewrite Git history to remove a secret without explicit team approval; rotate the secret first and follow the repository's incident process.

## Troubleshooting

- General local setup, ports, Compose, Nginx, workers, Playwright, and migration issues: [README troubleshooting](README.md#troubleshooting).
- Broker-backed test setup and skip behavior: [`tests/integration/README.md`](tests/integration/README.md).
- Migration commands and rollback warnings: [`migrations/README.md`](migrations/README.md).
- Event and envelope rules: [`contracts/events/README.md`](contracts/events/README.md).
- HTTP contracts and shared error behavior: [`contracts/openapi/README.md`](contracts/openapi/README.md).
