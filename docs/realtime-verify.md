# Realtime verification

One command that checks the WebSocket service against API v1.1 section 9 on a
running stack: who may follow what, the subscription and message limits, message
validation, the browser origin check, and the public-field projection of a
status update. It prints a PASS/FAIL line per check and a Markdown table.

It is operator tooling, not a CI job. A successful run is evidence for the
realtime milestone; it is not a substitute for the Go tests.

## What it checks

There are 14 checks. The script prints the same list at the end of a run.

| Area          | Check                                                               | Expected                           |
| ------------- | ------------------------------------------------------------------- | ---------------------------------- |
| Authorization | anonymous visitor follows one parcel                                | `subscribed`                       |
| Authorization | anonymous visitor cannot follow everything                          | close `4401`                       |
| Authorization | an invalid access token is refused                                  | close `4401`                       |
| Authorization | a CUSTOMER cannot follow everything                                 | `FORBIDDEN` error, close `4403`    |
| Authorization | a DISPATCHER follows everything                                     | `subscribed` with `scope: all`     |
| Limits        | 20 subscriptions are allowed                                        | 20 `subscribed` messages           |
| Limits        | the 21st subscription is refused                                    | close `4429`                       |
| Limits        | unsubscribing frees a slot                                          | a new subscription is accepted     |
| Limits        | more than 60 messages in 10 seconds are refused                     | close `4429`                       |
| Limits        | a message over 1024 bytes is refused                                | close `1009`                       |
| Messages      | malformed JSON and an invalid tracking number get `INVALID_MESSAGE` | 2 errors, then a working subscribe |
| Origin        | a disallowed browser origin is refused                              | HTTP `403`                         |
| Origin        | the allowed browser origin upgrades                                 | HTTP `101`                         |
| Privacy       | a status update exposes only public fields                          | `realtime-smoke.mjs` passes        |

The idle timeout, token expiry and shutdown close take 60 seconds or more, so
this script does not wait for them. They are covered by
`apps/services/realtime/internal/ws/handler_test.go`:

- `TestSilentClientIsDroppedByTheIdleTimeout`
- `TestTokenEndsTheConnection`
- `TestShutdownClosesConnections`

## Prerequisites

- A running Docker Compose stack with PostgreSQL, RabbitMQ, Identity, Parcel and
  Realtime, plus the gateway (Nginx). The default `make up-build` stack works.
- Migrations applied and the five demo identity accounts seeded (`make seed`).
- Node.js 22 or newer (the script uses the global `WebSocket`).
- Run from the repository root.
- Docker access for the privacy check, which runs
  `scripts/realtime-smoke.mjs`. That script reads `POSTGRES_USER` and
  `POSTGRES_DB` from the running `postgres` container and inserts one synthetic
  outbox row.
- The demo dispatcher and customer credentials in the terminal (below). The
  script signs in through the real `/api/v1/auth/login` endpoint, exactly like
  the Playwright helpers.

The gateway publishes `8080` by default. Realtime itself is internal to the
Compose network; use the gateway URL, not the unpublished service port.

## Required services

| Service           | Why                                                     |
| ----------------- | ------------------------------------------------------- |
| `postgres`        | privacy check (`realtime-smoke.mjs`) and service state  |
| `rabbitmq`        | the privacy event's relay/consumer path                 |
| `identity`        | the two login requests                                  |
| `parcel`          | the outbox relay and parcel state for the privacy event |
| `realtime`        | every WebSocket check                                   |
| `nginx` (gateway) | `WS_URL` and `API_URL` by default                       |

The web container is not required by the script, but the default stack starts
it behind the gateway. If the notification worker is running, an ownerless
synthetic status event is dead-lettered; the webhook worker only logs it. See
[Synthetic event side effects](#synthetic-event-side-effects-and-dlq).

## Environment variables

| Variable                         | Default                  | Purpose                                                       |
| -------------------------------- | ------------------------ | ------------------------------------------------------------- |
| `REAL_STACK_DISPATCHER_EMAIL`    | required                 | dispatcher login                                              |
| `REAL_STACK_DISPATCHER_PASSWORD` | required                 | dispatcher login; never stored                                |
| `REAL_STACK_CUSTOMER_EMAIL`      | required                 | customer login                                                |
| `REAL_STACK_CUSTOMER_PASSWORD`   | required                 | customer login; never stored                                  |
| `WS_URL`                         | `ws://localhost:8080/ws` | WebSocket endpoint; `ws://` or `wss://`                       |
| `API_URL`                        | `http://localhost:8080`  | where `/api/v1/auth/login` is                                 |
| `GOOD_ORIGIN`                    | `http://localhost:8080`  | origin the service must allow                                 |
| `BAD_ORIGIN`                     | `http://evil.example`    | origin the service must refuse                                |
| `COMPOSE_PROJECT_NAME`           | Compose default          | project used by the privacy check's `docker compose` commands |
| `SKIP_PRIVACY`                   | unset                    | set to `1` to skip the privacy check and its synthetic event  |

With the default `APP_ENV=development`, `REALTIME_ALLOWED_ORIGINS` may stay
empty: the realtime service falls back to the development origins, which
include `localhost:8080` and `127.0.0.1:8080`. With any other `APP_ENV`, set
`REALTIME_ALLOWED_ORIGINS` to include `GOOD_ORIGIN` (host or `host:port`) or the
origin check fails with HTTP 403.

`COMPOSE_PROJECT_NAME` must match the running Compose environment when it is
not the default `swiftdrop`. The privacy check runs `docker compose exec
postgres` in the repository; the project name selects the right container. It
does not read demo passwords or the host `DATABASE_URL`.

## Seed accounts

The script uses the same account variable names as the Playwright real-stack
helpers. Seed the deterministic demo users first:

```sh
set -a
. ./.env
set +a
# Host-side seed values; Compose PostgreSQL is not published to the host.
export DATABASE_URL='postgres://swiftdrop:swiftdrop@<host-reachable-db>:5432/swiftdrop?sslmode=disable'
export SEED_DISPATCHER_EMAIL='...'
export SEED_DISPATCHER_PASSWORD='...'
export SEED_CUSTOMER_EMAIL='...'
export SEED_CUSTOMER_PASSWORD='...'
# The other six SEED_* variables are required by the seed program.
make seed
```

Then export the same values as `REAL_STACK_DISPATCHER_*` and
`REAL_STACK_CUSTOMER_*` for the verification run. Passwords are read only from
the environment and are never printed.

## Run it

Default Compose project, gateway on `8080`:

```sh
REAL_STACK_DISPATCHER_EMAIL='...' \
REAL_STACK_DISPATCHER_PASSWORD='...' \
REAL_STACK_CUSTOMER_EMAIL='...' \
REAL_STACK_CUSTOMER_PASSWORD='...' \
node scripts/realtime-verify.mjs
```

A non-default Compose project:

```sh
COMPOSE_PROJECT_NAME='my-stack' \
REAL_STACK_DISPATCHER_EMAIL='...' \
REAL_STACK_DISPATCHER_PASSWORD='...' \
REAL_STACK_CUSTOMER_EMAIL='...' \
REAL_STACK_CUSTOMER_PASSWORD='...' \
node scripts/realtime-verify.mjs
```

A `wss://` endpoint (the origin check uses HTTPS and the default CA store):

```sh
WS_URL='wss://staging.example.test/ws' \
API_URL='https://staging.example.test' \
GOOD_ORIGIN='https://staging.example.test' \
REAL_STACK_DISPATCHER_EMAIL='...' \
REAL_STACK_DISPATCHER_PASSWORD='...' \
REAL_STACK_CUSTOMER_EMAIL='...' \
REAL_STACK_CUSTOMER_PASSWORD='...' \
node scripts/realtime-verify.mjs
```

Skip the Docker-based privacy check:

```sh
SKIP_PRIVACY=1 node scripts/realtime-verify.mjs
```

The script prints `Realtime verification against <origin><path>` at startup. It
deliberately omits any query string and any user information, so a token placed
in `WS_URL` is not echoed.

## The 14 checks

### Authorization

- **Anonymous single parcel.** An anonymous connection subscribes to one
  well-formed tracking number and receives `{"type":"subscribed",...}`.
  Single-parcel subscriptions are public by design, like the public tracking
  page.
- **Anonymous `scope: all`.** The service closes with `4401` (token invalid or
  missing). A browser is expected to renew its session and retry.
- **Invalid token.** A syntactically bad `?token=` is refused at the handshake
  with `4401`.
- **Customer `scope: all`.** A valid CUSTOMER token cannot follow everything:
  the service sends an error with code `FORBIDDEN` and closes with `4403`.
- **Dispatcher `scope: all`.** A DISPATCHER token receives
  `{"type":"subscribed","scope":"all"}`.

### Limits

- **20 subscriptions.** One connection holds the API maximum of 20.
- **21st subscription.** The next subscribe closes with `4429`.
- **Unsubscribe.** After 20 subscriptions, unsubscribing from one frees a slot
  and a new subscribe is accepted.
- **Message rate.** More than 60 client messages in a 10-second window closes
  with `4429`.
- **Message size.** A text message over 1024 bytes closes with `1009` (the
  WebSocket "message too big" status).

### Messages

- **Recoverable errors.** A non-JSON frame and a subscribe with an invalid
  tracking number each produce an `INVALID_MESSAGE` error. The connection stays
  open and a valid subscribe then succeeds.

### Origin

- **Refused origin.** A raw upgrade request with `Origin: BAD_ORIGIN` receives
  HTTP 403.
- **Allowed origin.** A raw upgrade request with `Origin: GOOD_ORIGIN` receives
  HTTP 101 and the socket is closed immediately by the script.

The origin checks use `http.request` for `ws://` and `https.request` for
`wss://`, with Node's default certificate verification. A TLS error (for
example `DEPTH_ZERO_SELF_SIGNED_CERT`) is reported as the actual result rather
than being mistaken for a rejected origin. Certificate verification is never
disabled.

### Privacy

The script runs `scripts/realtime-smoke.mjs`, which injects one synthetic
outbox row and follows it through the parcel relay, RabbitMQ, the realtime
consumer and a WebSocket client. It passes only if the update carries exactly
`previousStatus`, `status`, `trackingNumber` and `updatedAt`; the private note
in the synthetic event must not appear.

## Expected output

A passing run ends with the table and:

```text
14 of 14 checks passed.
```

Each check prints `PASS  <Area>: <check> (<actual>)`. The privacy section also
prints the synthetic tracking number so you can find its artifacts.

A failing run prints `FAIL  <Area>: <check> (<actual>)`, still prints the table,
and ends with `<n> of 14 checks passed.` The process exits non-zero.

## Exit codes

| Code | Meaning                                                    |
| ---- | ---------------------------------------------------------- |
| `0`  | every check passed                                         |
| `1`  | at least one check failed, or configuration/startup failed |

Configuration failures print `FAIL: ...` before any check, for example an
unsupported `WS_URL` protocol. The script never exits zero when a required
check is missing or failed.

## Troubleshooting

| Symptom                                                | Likely cause                                                                                                  |
| ------------------------------------------------------ | ------------------------------------------------------------------------------------------------------------- |
| `login for ... returned 401`                           | wrong demo account or password; reseed or re-export `REAL_STACK_*`                                            |
| `login for ... stayed rate limited`                    | more than 10 login attempts per minute; wait a minute and retry                                               |
| `FAIL: WS_URL must use ws:// or wss://`                | `WS_URL` uses `http://`/`https://`; use the WebSocket scheme                                                  |
| `FAIL: WS_URL is not a valid URL`                      | malformed `WS_URL`                                                                                            |
| `Origin: ... is refused (HTTP 403)` but you expect 101 | `GOOD_ORIGIN` is not in `REALTIME_ALLOWED_ORIGINS` for a non-development `APP_ENV`                            |
| `Origin: ... (error DEPTH_ZERO_SELF_SIGNED_CERT)`      | the TLS certificate is not trusted; trust the CA or use the development `ws://` stack                         |
| `Privacy: failed: ... docker`                          | Docker is not running, the stack does not use that project name, or the command is not run from the repo root |
| `Privacy: failed: ... no update`                       | the relay/consumer is stopped, or the outbox row is stuck `PENDING`; check `docker compose logs`              |
| `INVALID_MESSAGE` in a check                           | the subscribe shape is wrong; the script already uses the contract shape                                      |
| `502`/`101` origin check differs from expectation      | a proxy in front of the gateway is rewriting or rejecting the `Upgrade` request                               |

## Security considerations

- Use disposable demo accounts. Do not run this against production: it signs
  in with real credentials and the privacy check injects a synthetic outbox
  row.
- Credentials come from the environment only. They are never written to the
  repository, printed, or passed to the smoke child process. The child receives
  an explicit allowlist (`PATH`, Docker/Compose settings, `WS_URL`, `TRACKING`)
  and none of the demo passwords or database secrets.
- Access tokens are sent only in the WebSocket `?token=` query, as API v1.1
  permits for non-browser clients. The startup line redacts user information
  and the query string, so a token in `WS_URL` is not echoed.
- The script verifies server authorization and limits; it does not change them.
- The origin check never disables TLS verification.

## Known limitations

- Idle timeout, token expiry and shutdown close are not exercised (they take 60
  seconds or more); the named Go tests cover them.
- The origin check supports `ws://` (HTTP) and `wss://` (HTTPS with the default
  CA store). It does not accept self-signed certificates unless Node trusts
  them.
- Single-parcel subscriptions are public by design. The script does not verify
  that a parcel exists; the realtime service accepts any well-formed tracking
  number.
- There is no replay, persistence or multi-replica fan-out in the service; the
  script does not test those.
- The script is manual operator tooling. It is not wired into CI; run it
  against a real stack when you need the evidence.

## Synthetic event side effects and DLQ

The privacy check runs `realtime-smoke.mjs`, which inserts one contract-valid
outbox row with a random `parcel_id` and tracking number and no owner fields.
The notification worker treats an ownerless status event as a permanent
failure, so at most one message can end up in
`notification.parcel.status_changed.v1.dlq`. The webhook worker consumes the
same event and only logs it. The run does not send customer email.

The script prints:

```text
Privacy synthetic tracking number: SDKXXXXXXXX (see docs/realtime-verify.md)
```

Use that tracking number to find the artifact:

- On success, `realtime-smoke.mjs` deletes its own outbox row after the relay
  marks it `PUBLISHED`.
- On failure, the row may remain for diagnosis; the relay's normal
  published-event cleanup removes it later if it was published.

To clean the dead-letter queue safely:

1. Open the RabbitMQ management UI at <http://localhost:15672/>.
2. Open **Queues** → `notification.parcel.status_changed.v1.dlq` and inspect the
   message count and payload.
3. Delete only the message whose `tracking_number` is the one printed above.

Do not purge the whole queue unless you are certain it contains no unrelated
messages. If you do need to purge it, the whole-queue command is:

```sh
docker compose exec rabbitmq rabbitmqadmin purge queue \
  name=notification.parcel.status_changed.v1.dlq
```

That deletes every message in the queue, so prefer deleting the single message
in the UI.

## Safe cleanup

- The script creates no containers, volumes or files.
- `SKIP_PRIVACY=1` avoids the synthetic event entirely.
- On failure, inspect `parcel.outbox_events` for the printed tracking number
  and remove only that row if needed.
- Remove only the matching DLQ message as described above.
- Never run `docker compose down -v` to clean up; it deletes local data. See
  [`realtime-smoke.md`](realtime-smoke.md) for the temporary port-override
  cleanup it uses.

## See also

- [`docs/realtime-smoke.md`](realtime-smoke.md): the synthetic-outbox privacy
  path in detail.
- [`docs/architecture.md`](architecture.md): realtime boundaries and readiness.
- [`docs/API_Documentation_v1.1.md`](API_Documentation_v1.1.md): section 9, the
  WebSocket contract.
