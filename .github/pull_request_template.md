## Summary

<!-- What changed, why, and which problem it solves. -->

## Linked issue

<!-- Closes #123, or "None". -->

## Type of change

- [ ] Bug fix
- [ ] New feature
- [ ] Refactor
- [ ] Documentation
- [ ] Test
- [ ] Infrastructure or CI
- [ ] Contract or migration
- [ ] Breaking change

## Test evidence

List the commands you ran and their outcomes. Do not check a box you did not run.

- [ ] Go unit tests: `go test ./...`
- [ ] Go race tests: `go test -race ./...`
- [ ] Go vet: `go vet ./...`
- [ ] Go build: `go build ./...`
- [ ] Nx project checks: `pnpm exec nx run-many -t lint test build -p <projects>`
- [ ] Frontend lint/build: `pnpm exec nx lint @org/web` and `pnpm exec nx build @org/web`
- [ ] E2E typecheck/lint: `pnpm exec nx typecheck @org/web-e2e` and `pnpm exec nx lint @org/web-e2e`
- [ ] Playwright E2E: `pnpm exec nx run @org/web-e2e:e2e --args="--project=chromium"`
- [ ] RabbitMQ integration tests: `RABBITMQ_URL=... go test -tags=integration ./tests/integration/...`
- [ ] Compose config: `docker compose config --quiet`
- [ ] Other (describe):

## Checklist

- [ ] `gofmt` is clean for changed Go files.
- [ ] `go test ./...` and `go test -race ./...` pass for the affected scope.
- [ ] `go vet ./...` and `go build ./...` pass.
- [ ] Relevant Nx `lint`, `typecheck`, and `build` targets pass.
- [ ] E2E tests were updated and run when the UI changed.
- [ ] Messaging integration tests were run when consumer, publisher, topology, retry, dead-letter, envelope, or framing behavior changed.
- [ ] `docker compose config --quiet` passes when Compose or infrastructure changed.
- [ ] API contracts, event schemas, migrations, `.env.example`, and documentation were updated together where needed.
- [ ] No secrets, credentials, tokens, private keys, or sensitive data are included.
- [ ] No generated build output or unrelated rewrites are included.
- [ ] `git diff --check` passes.

## Rollout and rollback

<!--
Deployment steps, configuration changes, migration order, feature flags, or "Not applicable".
Describe how to roll back safely, including data or message compatibility concerns.
Do not run destructive database operations without explicit approval.
-->
