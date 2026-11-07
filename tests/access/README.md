# Access tests

These tests prove that nobody can do what their role does not allow, through the same gateway the browser uses. They cover the task sheet's M2.5 rules: roles are refused on other roles' endpoints, nobody touches another account's records, revoked and suspended merchant keys are refused, and the sign-in, lockout, refresh, logout and expiry rules hold end to end.

| Part                                                                      | Needs                                       | Runs in                         |
| ------------------------------------------------------------------------- | ------------------------------------------- | ------------------------------- |
| `accesskit/` self-test (a server with rules broken on purpose)            | nothing                                     | `make test`, CI `go` job        |
| `matrix_test.go`, `sessions_test.go`, `ownership_test.go` (`access`)      | the local Compose stack on `localhost:8080` | `make test-access`, CI `access` |
| `apps/services/merchant/internal/http/api_key_access_integration_test.go` | `TEST_DATABASE_URL` (a throwaway database)  | `make test-db`, CI `database`   |

## Run

```sh
make up-build
```

```sh
make test-access
```

The suite takes about two minutes. Login and registration share a limit of 10 requests a minute per address, so it waits for `Retry-After` when it reaches it. It refuses to run against a gateway that is not on `localhost`, because it creates accounts. It signs in as five fixture accounts (one per role, made through the real register endpoint and given a role with SQL) and removes them when it ends.

If a test fails with `502` for a service, your image is older than `development`: `docker compose build <service>`, then `docker compose up -d <service>` and `docker compose restart nginx`.

## What the role matrix checks

`matrix_test.go` lists every endpoint with an access rule. For each one the checker calls it as an anonymous caller and as each of the five roles:

- anonymous gets **401**;
- a role that may use it gets an answer from the handler, never 401 or 403;
- every other role gets **403**.

ADMIN satisfies every DISPATCHER rule, so list `DISPATCHER` and ADMIN is expected to get in.

## Add your endpoints

Add one `accesskit.Case` to `matrix` in `matrix_test.go`:

```go
{Name: "GET /parcels", Method: http.MethodGet, Path: "/api/v1/parcels/", Allowed: []auth.Role{auth.RoleCustomer}},
```

- Use the path as the browser sees it (with `/api/v1`), and the API v1.1 endpoint table for `Allowed`.
- For a write, send a body the handler rejects, such as `{}`, so the suite never changes data.
- Endpoint not built yet: set `Missing: "why"`. The case is listed as skipped. Delete the field when the endpoint exists and the case starts running.
- Service that does not verify access tokens yet: set `Pending: "why"`. Only the refusals are checked, and nobody who must be refused may get data.

## Ownership cases

`TestCustomerParcelOwnership` creates a real parcel using two authenticated customers and the live pricing quote. It verifies owned list/detail/history/PDF reads and cross-customer 404 responses without parcel data. Its tagged parcel/history/outbox fixture is retained because history is append-only; use a disposable local stack. Courier and merchant ownership cases remain skipped with their exact request and expected response until those endpoints exist.

## Break a rule on purpose

The suite must fail when a rule is broken, or it proves nothing. Two checks do this:

1. `accesskit_test.go` runs the same checker against a server where a role check is forgotten, a route is open to anyone, and a pending case leaks. It asserts the exact list of violations. It runs on every `make test`.
2. On the real stack: remove the role guard of one route (for example the `couriers` group in `apps/services/identity/internal/http/admin.go`), rebuild that service, and run `make test-access`. `TestRoleMatrix` fails and names the route and each role that got in. Put the guard back and rebuild.

## Merchant API keys

`apps/services/merchant/internal/http/api_key_access_integration_test.go` runs the real `MerchantAPIKey` middleware against the real merchant schema. An ACTIVE key of an APPROVED merchant works and resolves to its own merchant only. A revoked key gets 401 `INVALID_API_KEY`. A key of a suspended, pending or rejected merchant gets 403 `MERCHANT_NOT_APPROVED`, including the moment after suspension. A wrong secret, an unknown key, a malformed key and a missing key get 401. No gateway route uses this middleware yet, which is why the test mounts it on a probe route.

## Known gaps these tests report

- The merchant service does not verify access tokens on `/merchants/*` and `/admin/merchants/*`. Every caller is refused, so the routes fail closed, but a merchant cannot use them and an admin cannot approve anyone. The matrix marks them `Pending`.
- Customer parcel routes now participate in the matrix and real ownership test. Dispatch and courier routes remain `Missing`.
