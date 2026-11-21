# Route inventories

These files are the runtime Gin route inventory for every HTTP service. They are
generated from `gin.Engine.Routes()` and committed so a route change is visible
in review. They are not a second API contract: `contracts/openapi` and
`infra/nginx/default.conf` remain the source of truth for the public contract
and the gateway.

| File                       | Service             | Router snapshot                                                |
| -------------------------- | ------------------- | -------------------------------------------------------------- |
| `identity.json`            | identity            | `apps/services/identity/internal/http`                         |
| `parcel.json`              | parcel              | `apps/services/parcel/internal/http`                           |
| `pricing.json`             | pricing             | `apps/services/pricing/internal/http`                          |
| `merchant.json`            | merchant            | `apps/services/merchant` (router plus webhook helpers)         |
| `notification-worker.json` | notification-worker | `apps/services/notification-worker/internal/http`              |
| `ai-assistant.json`        | ai-assistant        | `apps/services/ai-assistant/internal/http`                     |
| `realtime.json`            | realtime            | `apps/services/realtime/internal/http` plus `/ws` registration |
| `webhook-worker.json`      | webhook-worker      | `apps/services/webhook-worker` operational probes              |

Each file has the shape:

```json
{
  "service": "parcel",
  "routes": [
    {
      "method": "GET",
      "path": "/parcels/{parcelId}",
      "classification": "public",
      "documented": true,
      "gateway": true
    }
  ]
}
```

`reason` is present on operational and probe exceptions.

## How the inventory is produced

Each service has a `TestRouteInventory` test beside its router. It constructs the
router with zero-value or fake dependencies; no database, broker, listener or
full configuration is required. The test calls `routecontract.Snapshot`, which:

1. reads the actual `gin.Engine.Routes()` registrations;
2. keeps the business methods `GET`, `POST`, `PUT`, `PATCH`, `DELETE`;
3. converts `:param` and `*param` to `{param}`;
4. collapses trailing-slash aliases to one canonical path, because Gin registers
   both forms only to avoid a redirect that drops the gateway's `/api/v1` prefix;
5. sorts routes by path and method and writes deterministic JSON.

The inventory is compared with the committed file in ordinary test runs. It is
never rewritten unless `UPDATE_ROUTES=1` is explicitly set.

## Classifications and exceptions

| Classification | Documented in OpenAPI                                                             | Gateway upstream         | Meaning                                                                                                                                                                                                                                       |
| -------------- | --------------------------------------------------------------------------------- | ------------------------ | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `public`       | Required                                                                          | Required, owning service | Browser/API business route.                                                                                                                                                                                                                   |
| `internal`     | Required in the service's OpenAPI specification (`*-internal.yaml` where present) | Forbidden                | Service-to-service route under `/internal` or `/internal/...`.                                                                                                                                                                                |
| `operational`  | Exempt                                                                            | Forbidden                | `/health`, `/ready`, `/metrics`, `/healthz`, `/readyz`. These are registered by service entrypoints rather than the business router, so most are not in the inventory. Health and readiness may still be documented in OpenAPI for operators. |
| `probe`        | Forbidden                                                                         | Allowed                  | `GET /dispatch/probe` and `GET /admin/parcels/probe` exist for the access matrix and are authenticated, but are not public API contract. The reason is recorded in the inventory.                                                             |

No route is ignored silently. A classification change or a new exception must
appear in the inventory diff and is reviewed like any other code change.

The gateway check follows a collection-level `308` redirect to its slash-suffixed
location, so `GET /api/v1/couriers` is valid because Nginx redirects to
`/api/v1/couriers/`, which proxies to the identity upstream. Exact, longest-prefix,
`^~`, and regex locations are resolved in Nginx order.

## Adding or changing an endpoint

1. Register the Gin route in the owning service.
2. Add or update the operation in `contracts/openapi/<service>.yaml` (or the
   internal specification). Update `docs/API_Documentation_v1.1.md` when the
   endpoint is part of API v1.1.
3. Add or update the public mapping in `infra/nginx/default.conf`; internal
   routes must never be proxied.
4. Regenerate the inventory explicitly:

   ```sh
   UPDATE_ROUTES=1 go test -run TestRouteInventory ./apps/services/...
   ```

   Or update one service, for example:

   ```sh
   UPDATE_ROUTES=1 go test -run TestRouteInventory ./apps/services/parcel/internal/http
   ```

5. Review the inventory diff. An intentional new `public` route must have the
   matching OpenAPI operation and Nginx upstream; a new `internal` route must
   have the internal specification and no gateway proxy.
6. Run the gates:

   ```sh
   make check-openapi
   go test ./...
   ```

`make check-openapi` runs the existing OpenAPI checks and the new
`tests/contracts/route-inventory.test.mjs`, which cross-checks:

- Gin inventory ↔ OpenAPI: every public/internal route is documented, every
  documented operation has a registered route, methods match, and path parameter
  names are normalised (`{id}` matches `{parcelId}`).
- Gin inventory ↔ Nginx: every public/probe route resolves to the owning
  service's upstream; every internal/operational route must not resolve to that
  upstream.
- OpenAPI ↔ Nginx: the existing public-path location check remains in place.

CI already runs `node --test "tests/contracts/*.test.mjs"` and `go test ./...`, so
no workflow change is needed. The failure messages list the missing, unexpected
or incorrectly classified routes instead of only failing the whole suite.

## Limitations

- Operational probes registered in service `main` functions (`/health`,
  `/ready`, `/metrics`) are explicitly exempt rather than runtime-enumerated.
  `/healthz` and `/readyz` are enumerated through the shared router.
- Probe routes are a deliberate, documented exception. Remove the probe
  registration and its inventory entry together if the access matrix no longer
  needs it.
- The service list in `tests/contracts/route-inventory.test.mjs` and
  `serviceSpecs` is explicit. A new HTTP service must add its inventory test,
  specification mapping and upstream before the gate accepts it.
- The gate checks route shape and ownership, not request or response schemas;
  `openapi.test.mjs` and the service tests cover those at their own layers.
