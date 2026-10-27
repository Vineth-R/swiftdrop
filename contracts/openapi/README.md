# OpenAPI contracts

One specification per HTTP service:

| Service  | Specification   | Public base path    |
| -------- | --------------- | ------------------- |
| Identity | `identity.yaml` | `/api/v1/identity`  |
| Parcel   | `parcel.yaml`   | `/api/v1/parcels`   |
| Pricing  | `pricing.yaml`  | `/api/v1/pricing`   |
| Merchant | `merchant.yaml` | `/api/v1/merchants` |
| Realtime | `realtime.yaml` | `/api/v1/realtime`  |

The public base paths are the Nginx routes. Nginx strips that prefix before forwarding, so service routers expose `/healthz` and `/readyz` at their root. See `docs/architecture.md`.

Specifications are maintained by hand for now. When an endpoint is added or changed, update the specification, the Gin route, and the Nginx mapping together. No code-generation pipeline is used during initialization.

The current specifications intentionally describe only the operational health endpoints. Business endpoints will be added as they are implemented.
