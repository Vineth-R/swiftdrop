# Nginx local reverse proxy

`default.conf` is mounted into the `nginx:1.27-alpine` container by `docker-compose.yml`. The gateway is the only public application port (`http://localhost:8080`).

## Routing (API v1.1)

| Public path group                                                                                                                    | Upstream            | Forwarded path       |
| ------------------------------------------------------------------------------------------------------------------------------------ | ------------------- | -------------------- |
| `/api/v1/auth/*`, `/api/v1/users/*`, `/api/v1/admin/users/*`, `/api/v1/couriers/*`                                                   | `identity:8081`     | prefix stripped      |
| `/api/v1/pricing/*`, `/api/v1/zones/*`, `/api/v1/admin/pricing/*`, `/api/v1/admin/zones/*`                                           | `pricing:8083`      | prefix stripped      |
| `/api/v1/parcels/*`, `/api/v1/tracking/*`, `/api/v1/dispatch/*`, `/api/v1/admin/parcels/*`, `/api/v1/courier/*`, `/api/v1/reports/*` | `parcel:8082`       | prefix stripped      |
| `/api/v1/merchants/*`, `/api/v1/merchant/*`, `/api/v1/admin/merchants/*`                                                             | `merchant:8084`     | prefix stripped      |
| `/ws`                                                                                                                                | `realtime:8085`     | unchanged            |
| `/`                                                                                                                                  | `web:3000`          | unchanged            |
| `/healthz`                                                                                                                           | Nginx itself        | `{"status":"ok"}`    |
| `/nginx-health`                                                                                                                      | Nginx itself        | `ok`                 |
| `/internal/*`                                                                                                                        | never routed        | JSON `404 NOT_FOUND` |
| `/api/v1/ai/*`                                                                                                                       | not implemented yet | JSON `404 NOT_FOUND` |

The `/api/v1` prefix is stripped before forwarding. For example:

```text
GET http://localhost:8080/api/v1/auth/health
    -> http://identity:8081/auth/health
GET http://localhost:8080/api/v1/parcels/123
    -> http://parcel:8082/parcels/123
```

Gin routes therefore register root paths such as `/auth/...`, `/parcels/...`, `/health`, and `/ready`. When adding a new route, update the service router, the OpenAPI specification, and this proxy mapping together.

A path group without a trailing slash generally returns a `308` redirect. `/api/v1/zones` and `/api/v1/admin/zones` are exact resource routes and proxy directly, preserving their methods and avoiding a trailing-slash redirect loop with Gin.

Health endpoints are not public except `/healthz` at the gateway. `/health` and `/ready` are reachable only on the internal service network (Docker healthchecks).

## Operational notes

- `X-Request-Id` is forwarded. Nginx generates one when the client does not supply it.
- `X-Real-IP` and `X-Forwarded-For` are set from the observed client address. Pricing and Parcel accept `X-Real-IP` only from configured trusted proxy CIDRs (`HTTP_TRUSTED_PROXIES`), so caller-supplied forwarding headers cannot rotate the public rate-limit identity through this edge.
- `/api/v1/auth/` uses the `swiftdrop_no_query` access-log format: it records the method, URI path, status and timing but never the query string. This keeps OAuth authorization codes and state values out of the gateway logs. Other routes keep the default combined log.
- `/internal` is explicitly blocked so internal service-to-service APIs never reach browsers.
- `/api/v1/courier/deliveries/{parcelId}/proof` has a route-scoped 6 MiB `client_max_body_size` so a 5 MiB photo plus multipart overhead reaches Parcel; the application still enforces its own 5 MiB photo limit. Oversized gateway requests return the shared JSON `PAYLOAD_TOO_LARGE` error instead of Nginx's HTML page.
- `/ws` enables WebSocket upgrade headers, disables proxy buffering, and uses a long read timeout.
- Local development uses plain HTTP. Production TLS should terminate at a load balancer or a separately managed Nginx configuration with real certificates. No certificates are committed to this repository.
- Nginx resolves every upstream at startup. Start the full Compose stack; starting Nginx alone with missing upstreams is not supported.

Registration routes `/api/v1/auth/verify-email` and
`/api/v1/auth/resend-verification` use the existing `/api/v1/auth/` proxy.
Merchant registration uses `/api/v1/merchants/`; admin listing uses the exact
`/api/v1/admin/merchants` and prefix routes.

`/api/v1/admin/users` (the documented list path, no trailing slash) has its own
exact rule that proxies to Identity, so it is not sent on to the web app.

## Redirects

`absolute_redirect off;` makes every redirect Nginx itself issues relative, such as the `308` that adds the
trailing slash to a collection path (`/api/v1/couriers` to `/api/v1/couriers/`) and the `301` Nginx adds
for a prefix location. Without it, Nginx builds the address from its own port (80), so a caller on
`http://localhost:8080` is sent to `http://localhost/...`, a closed port. The explicit `308` rules also
carry the query string over (`$is_args$args`), so `GET /api/v1/couriers?isAvailable=true` keeps its filter.

`scripts/check-routes.sh` checks both (`make check-routes`). A new collection path that needs a
no-slash redirect should follow the same `return 308 /path/$is_args$args;` shape.
