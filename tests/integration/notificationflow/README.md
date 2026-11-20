# Notification-flow integration suite

This suite runs the real notification worker against a disposable Docker Compose
stack and verifies delivery through Mailpit, the `notification` schema and the
dead-letter queue. It publishes only contract-valid or contract-invalid
messages; it does not implement a producer, outbox or relay.

The suite is behind the `notificationflow` build tag and runs in CI as
`Notification worker E2E (Compose)`.

## Why a separate Compose project

The developer stack's `.env` may point `SMTP_HOST` at a real provider. The suite
needs Mailpit and its own ports, so it uses:

- `COMPOSE_PROJECT_NAME=swiftdrop-notificationflow`
- `COMPOSE_FILE=docker-compose.yml:tests/integration/notificationflow/docker-compose.identity.yml`
- alternate host ports for RabbitMQ, Mailpit and Nginx

Never point this suite at a shared or production database. It creates and
dead-letters real rows and messages.

## Required environment

Export all of these before `docker compose up`:

| Variable                     | Value     | Why                                          |
| ---------------------------- | --------- | -------------------------------------------- |
| `SMTP_HOST`                  | `mailpit` | The worker must send to the test mailbox.    |
| `SMTP_PORT`                  | `1025`    | Mailpit SMTP port inside the network.        |
| `SMTP_USER`, `SMTP_PASSWORD` | empty     | Mailpit needs no authentication.             |
| `SMTP_SECURE`                | `auto`    | Plain SMTP inside the network.               |
| `RABBITMQ_RETRY_DELAY`       | `5s`      | Speeds retries for the dead-letter tests.    |
| `RABBITMQ_MAX_RETRIES`       | `2`       | Reaches exhaustion inside the test deadline. |

`RABBITMQ_RETRY_DELAY` and `RABBITMQ_MAX_RETRIES` are read by the **worker
container**, not only by `go test`. Exporting them after `docker compose up` has
no effect. With the default 30s delay and three retries, the
identity-unavailable dead-letter test's 60s wait expires.

## Run

```sh
export COMPOSE_PROJECT_NAME=swiftdrop-notificationflow
export COMPOSE_FILE='docker-compose.yml:tests/integration/notificationflow/docker-compose.identity.yml'
export RABBITMQ_PORT=25672 RABBITMQ_MANAGEMENT_PORT=25673
export MAILPIT_SMTP_PORT=21025 MAILPIT_UI_PORT=28025 NGINX_PORT=18080
export SMTP_HOST=mailpit SMTP_PORT=1025 SMTP_USER= SMTP_PASSWORD= SMTP_SECURE=auto
export RABBITMQ_RETRY_DELAY=5s RABBITMQ_MAX_RETRIES=2

docker compose down -v
docker compose up -d --wait --wait-timeout 300 \
  postgres rabbitmq mailpit identity merchant notification-worker
make migrate

NOTIFICATIONFLOW_IDENTITY_URL=http://127.0.0.1:18081 \
NOTIFICATIONFLOW_MERCHANT_URL=http://127.0.0.1:18084 \
NOTIFICATIONFLOW_RABBITMQ_URL='amqp://swiftdrop:swiftdrop@127.0.0.1:25672/' \
NOTIFICATIONFLOW_RABBITMQ_MANAGEMENT_URL='http://127.0.0.1:25673' \
NOTIFICATIONFLOW_MAILPIT_URL='http://127.0.0.1:28025' \
RABBITMQ_RETRY_DELAY=5s RABBITMQ_MAX_RETRIES=2 NOTIFICATIONFLOW_RETRY_WAIT=90s \
go test -race -tags=notificationflow -count=1 -v ./tests/integration/notificationflow/...

docker compose down -v
```

The target ports must be free. Override `NOTIFICATIONFLOW_IDENTITY_PORT`
(default `18081`) and `NOTIFICATIONFLOW_MERCHANT_PORT` (default `18084`) in
`tests/integration/notificationflow/docker-compose.identity.yml` if they clash.

## Coverage

- customer parcel-created and status-changed email;
- duplicate-event suppression;
- merchant-owned approved and suspended contacts;
- invalid and foreign-producer dead-lettering;
- identity-unavailable retry exhaustion and dead-lettering;
- SMTP failure recovery;
- legacy delivery and deduplication;
- contract-invalid envelopes and unroutable publishes.

## Result on the merged baseline

`831455b`: **18/18 tests passed** with `go test -race -tags=notificationflow`
against a disposable local Compose project and Mailpit.
