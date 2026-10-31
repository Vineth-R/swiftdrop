# Nginx local reverse proxy

`default.conf` is mounted into the `nginx:1.27-alpine` container by `docker-compose.yml`. The gateway is the only public application port (`http://localhost:8080`).

## Routing (API v1.1)

| Public path group | Upstream | Forwarded path |
| ----------------- | -------- | -------------- |
| `/api/v1/auth/*`, `/api/v1/users/*`, `/api/v1/admin/users/*`, `/api/v1/couriers/*` | `identity:8081` | prefix stripped |
| `/api/v1/pricing/*`, `/api/v1/zones/*`, `/api/v1/admin/pricing/*`, `/api/v1/admin/zones/*` | `pricing:8083` | prefix stripped |
| `/api/v1/parcels/*`, `/api/v1/tracking/*`, `/api/v1/dispatch/*`, `/api/v1/admin/parcels/*`, `/api/v1/courier/*`, `/api/v1/reports/*` | `parcel:8082` | prefix stripped |
| `/api/v1/merchants/*`, `/api/v1/merchant/*`, `/api/v1/admin/merchants/*` | `merchant:8084` | prefix stripped |
| `/ws` | `realtime:8085` | unchanged |
| `/` | `web:3000` | unchanged |
| `/healthz` | Nginx itself | `{"status":"ok"}` |
| `/nginx-health` | Nginx itself | `ok` |
| `/internal/*` | never routed | JSON `404 NOT_FOUND` |
| `/api/v1/ai/*` | not implemented yet | JSON `404 NOT_FOUND` |

The `/api/v1` prefix is stripped before forwarding. For example:

```text
GET http://localhost:8080/api/v1/auth/health
    -> http://identity:8081/auth/health
GET http://localhost:8080/api/v1/parcels/123
    -> http://parcel:8082/parcels/123
```

Gin routes therefore register root paths such as `/auth/...`, `/parcels/...`, `/health`, and `/ready`. When adding a new route, update the service router, the OpenAPI specification, and this proxy mapping together.

A path group without a trailing slash returns a `308` redirect to the trailing-slash path.

Health endpoints are not public except `/healthz` at the gateway. `/health` and `/ready` are reachable only on the internal service network (Docker healthchecks).

## Operational notes

- `X-Request-Id` is forwarded. Nginx generates one when the client does not supply it.
- `/internal` is explicitly blocked so internal service-to-service APIs never reach browsers.
- `/ws` enables WebSocket upgrade headers, disables proxy buffering, and uses a long read timeout.
- Local development uses plain HTTP. Production TLS should terminate at a load balancer or a separately managed Nginx configuration with real certificates. No certificates are committed to this repository.
- Nginx resolves every upstream at startup. Start the full Compose stack; starting Nginx alone with missing upstreams is not supported.
