# Nginx local reverse proxy

`default.conf` is mounted into the `nginx:1.27-alpine` container by `docker-compose.yml`.

## Routing

| Public path           | Upstream        | Forwarded path |
| --------------------- | --------------- | -------------- |
| `/api/v1/identity/*`  | `identity:8081` | `/*`           |
| `/api/v1/parcels/*`   | `parcel:8082`   | `/*`           |
| `/api/v1/pricing/*`   | `pricing:8083`  | `/*`           |
| `/api/v1/merchants/*` | `merchant:8084` | `/*`           |
| `/api/v1/realtime/*`  | `realtime:8085` | `/*`           |
| `/`                   | `web:3000`      | unchanged      |
| `/nginx-health`       | Nginx itself    | `ok`           |

The `/api/v1/<service>/` prefix is **stripped** before forwarding. For example:

```text
GET http://localhost:8080/api/v1/identity/healthz
    -> http://identity:8081/healthz
```

Gin routes therefore use root paths such as `/healthz` and `/readyz`. When adding a new route, update the service router, the OpenAPI specification, and this proxy mapping together.

A service prefix without a trailing slash (for example `/api/v1/identity`) returns a `308` redirect to the trailing-slash path.

The Next.js application keeps `/api/hello` available because only the `/api/v1/` namespace is proxied to Go services.

The Compose Nginx configuration resolves every upstream at startup. Work on a single Go service directly through its exposed port (for example <http://localhost:8081/healthz>) instead of starting only Nginx and one backend container.

## Operational notes

- Request IDs are forwarded. Nginx generates one when the client does not supply `X-Request-ID`.
- Realtime routes enable WebSocket upgrade headers, disable proxy buffering, and use a long read timeout.
- Local development uses plain HTTP. Production TLS should terminate at a load balancer or a separately managed Nginx configuration with real certificates. No certificates are committed to this repository.
