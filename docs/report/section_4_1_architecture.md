# 4.1 Architecture

SwiftDrop uses a layered, domain-oriented microservice architecture. The public
entry point is a single Nginx gateway. Browser requests reach the Next.js web
application or one of six Go HTTP services; asynchronous work runs in two Go
workers connected through RabbitMQ. Each service owns one PostgreSQL schema and
is the only writer of its tables.

## Layers

| Layer          | Technology                                       | Responsibility                                                                                                                          |
| -------------- | ------------------------------------------------ | --------------------------------------------------------------------------------------------------------------------------------------- |
| Presentation   | Next.js App Router, React, TypeScript, shadcn/ui | Customer, merchant, courier, dispatcher and ADMIN screens, the public tracking page and the AI chat widget. It holds no business rules. |
| Business logic | Go and Gin HTTP services, Go workers             | Request validation, authorization, pricing, parcel lifecycle, notifications, webhooks, realtime fan-out and merchant synchronization.   |
| Integration    | Nginx, RabbitMQ, internal HTTP APIs              | A single public edge, durable topic exchange, retry and dead-letter queues, and authenticated service-to-service calls.                 |
| Data           | PostgreSQL 16, private object storage            | Five service-owned schemas plus private delivery-proof objects.                                                                         |

## Deployable units

| Unit                  | Port                 | Owns           | Responsibility                                                                                                                                                           |
| --------------------- | -------------------- | -------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `identity`            | 8081                 | `identity`     | Registration, email verification, login, refresh, logout, Google OAuth, password reset, profiles, ADMIN users, courier records and internal contact/eligibility lookups. |
| `parcel`              | 8082                 | `parcel`       | Parcel creation, pricing persistence, tracking, dispatch, courier status updates, proof uploads, PDF receipts, reports, outbox and event publication.                    |
| `pricing`             | 8083                 | `pricing`      | Zones, pricing rules, weight bands, size surcharges, public estimates and internal quotes.                                                                               |
| `merchant`            | 8084                 | `merchant`     | Merchant onboarding and approval, API keys, orders, order snapshots, status synchronization, webhooks and internal worker endpoints.                                     |
| `realtime`            | 8085                 | none           | WebSocket `/ws`, per-parcel subscriptions, DISPATCHER/ADMIN `scope: all`, and public-safe status fan-out.                                                                |
| `ai-assistant`        | 8087                 | none           | Gemini-backed delivery assistant with four read-only tools, guest sessions, rate limiting and privileged-lookup auditing.                                                |
| `notification-worker` | 8086 (internal HTTP) | `notification` | Consumes parcel events, resolves contacts through internal APIs, sends email, records delivery logs and exposes the ADMIN notification-log read API.                     |
| `webhook-worker`      | none                 | none           | Delivers signed merchant webhooks, records attempts through the Merchant internal API and schedules the six business retries.                                            |

The web application and Nginx are separate containers. PostgreSQL, RabbitMQ,
Mailpit and the optional S3-compatible storage service complete the local
platform.

## Service ownership

- Each service reads and writes only its own schema. Cross-service data is
  obtained through REST or events; there are no cross-service foreign keys.
- Parcel writes the parcel row, its status history and a PENDING outbox event in
  one transaction. A relay publishes committed events to the
  `swiftdrop.events` topic exchange.
- Four consumers act independently on parcel events: the notification worker,
  the webhook worker, the realtime service and the merchant sync consumer. A
  slow email or webhook never delays the courier.
- Internal endpoints are authenticated with a shared `X-Internal-Token` and are
  never routed by Nginx. The gateway returns `404` for `/internal`.
- Realtime and the webhook worker own no tables. Delivery attempts are stored by
  the Merchant service; realtime state is in-process.

## Runtime topology

Figure 7 shows the deployed topology:

- Browser and API clients → Nginx (`:8080`).
- Nginx → Next.js web (`:3000`) and the six HTTP upstreams.
- HTTP services → PostgreSQL schemas and, where needed, internal HTTP APIs.
- Parcel → RabbitMQ `swiftdrop.events`.
- RabbitMQ → notification, webhook, realtime and merchant sync queues.
- Notification worker → Identity and Merchant internal APIs and SMTP/Mailpit.
- Webhook worker → Merchant and Parcel internal APIs and the merchant's HTTPS
  receiver.
- AI assistant → Pricing and Parcel APIs and Gemini. It never accesses a
  database and never accepts a user ID from the model.
- Optional storage profile → VersityGW for private proof objects.

## Key architectural decisions

- **Single gateway**: one public origin keeps cookies, routing and security
  headers consistent and keeps internal ports private.
- **Transactional outbox**: parcel changes and events commit together; the relay
  publishes after commit.
- **At-least-once events**: consumers are idempotent. Notification and merchant
  sync deduplicate durably; webhook delivery records a durable attempt and skips
  an event that already succeeded; realtime uses a bounded in-process window.
- **Failure isolation**: notification, webhook and realtime failures do not roll
  back a courier status update.
- **Stateless edge services**: realtime and AI hold no durable state. Their
  in-process limits are documented single-replica constraints.

## Figure 7

The editable source is
[`docs/diagrams/figure-7-platform-architecture.drawio`](../diagrams/figure-7-platform-architecture.drawio).
It reflects the Compose services, ports, schemas, queues and external
dependencies described above.
