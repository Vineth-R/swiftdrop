# Merchant database integration follow-up

**Owner:** merchant service owner
**Status:** review and verification; the original `database/sql` concern is no longer present on this branch.

## Current implementation

- `apps/services/merchant/internal/repository/repository.go` defines a small `DB` interface (`Exec`, `QueryRow`, `Query`, `Begin`) using `pgx`/`pgconn` types.
- `apps/services/merchant/main.go` opens the shared `packages/go-common/postgres` pool and passes it to `merchantrepository.New(database)`. There is no second pool and no `database/sql` usage in `apps/services/merchant`.
- `apps/services/merchant/internal/http/router.go` registers merchant routes when a repository is supplied.
- The repository's transaction calls use the pgx API (`Begin(ctx)`, `Rollback(ctx)`, `Exec(ctx, …)`, `Commit(ctx)`).

## Expected integration

- The merchant service uses the single shared `postgres.Pool` created in `main.go`.
- No service creates an independent connection pool or bypasses `packages/go-common/postgres`.
- Health/readiness continue to use the shared pool and `/ready` reports database connectivity.
- Schema access stays inside the `merchant` schema and the shared migration chain (`migrations/merchant/000006_merchant_constraints`).

## Files likely affected

- `apps/services/merchant/main.go`
- `apps/services/merchant/internal/http/` (`router.go`, `merchant.go`, `api_key_middleware.go`)
- `apps/services/merchant/internal/repository/` (`repository.go`, `merchants.go`, `api_clients.go`, tests)
- `migrations/merchant/`

## Required tests

- Repository tests against a real PostgreSQL instance (build tag `integration`) covering merchant creation, profile update, approve/reject/suspend, API-key limit, and lookup.
- Handler tests for authentication/authorization boundaries and error mappings.
- Readiness test proving `/ready` reflects shared-pool connectivity.
- Regression test that no `database/sql` import or second pool is introduced.

## Acceptance criteria

- Only `postgres.Pool` is used for merchant database access; `grep -r "database/sql" apps/services/merchant` returns nothing.
- `go test ./apps/services/merchant/...` and the PostgreSQL integration tests pass.
- Merchant handlers and repositories are covered for the failure and authorization paths above.
- Migrations apply cleanly on fresh and version-2 databases and match the merchant service's schema expectations.
- The merchant owner has reviewed and approved the repository/HTTP integration.

## Notes

The pgx transaction fix and formatting change landed as part of the M1 startup branch to unblock CI after `development` advanced. Business logic and ownership remain with the merchant service owner.
