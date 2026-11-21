# Realtime smoke test

Follows one synthetic outbox row through the realtime path on a real stack:

`parcel.outbox_events` → parcel relay → RabbitMQ → realtime consumer → WebSocket.

For the full API v1.1 section 9 surface (authorization, subscription and
message limits, message validation, the browser origin check and the
public-field projection), use [`realtime-verify.md`](realtime-verify.md). That
script runs this smoke check as its privacy step.

It verifies:

- synthetic outbox insertion;
- outbox relay publication;
- RabbitMQ delivery;
- the realtime consumer;
- WebSocket subscription and the public-field projection.

It injects the outbox row directly, so it does **not** exercise the courier status-update transaction, the parcel history write, or the production courier UI.

## Prerequisites

- A Docker Compose stack with PostgreSQL, RabbitMQ, parcel and realtime running, and migrations applied.
- Node.js 22 or newer (global `WebSocket`).
- Run the commands from the repository root. The script reads `POSTGRES_USER` and `POSTGRES_DB` from the running `postgres` container, so it honours the Compose `.env` even when those variables are not exported in your shell.

## Run it

Compose does not publish the realtime port (8085), so use a throwaway override outside the repository.

macOS / Linux:

```bash
override=$(mktemp)
printf 'services:\n  realtime:\n    ports:\n      - "8085:8085"\n' > "$override"
docker compose -f docker-compose.yml -f "$override" up -d postgres rabbitmq parcel realtime
node scripts/realtime-smoke.mjs
```

Windows PowerShell:

```powershell
$override = Join-Path $env:TEMP 'ws-ports.yml'
Set-Content $override "services:`n  realtime:`n    ports:`n      - '8085:8085'"
docker compose -f docker-compose.yml -f $override up -d postgres rabbitmq parcel realtime
node scripts/realtime-smoke.mjs
```

The script prints the tracking number it generated, waits for `subscribed` (there is no replay), inserts one outbox row, and expects a `parcel.status.updated` message.

## Settings

| Variable     | Default                  | Purpose                                                       |
| ------------ | ------------------------ | ------------------------------------------------------------- |
| `WS_URL`     | `ws://localhost:8085/ws` | Realtime endpoint                                             |
| `TRACKING`   | random `SDK…`            | Tracking number to use; must match `^SD[KN][A-Z0-9]{8}$`      |
| `TIMEOUT_MS` | `15000`                  | Positive integer timeout in milliseconds (maximum 2147483647) |

Examples:

```bash
TRACKING=SDK7M2Q9XP4 node scripts/realtime-smoke.mjs
TIMEOUT_MS=30000 node scripts/realtime-smoke.mjs
```

A supplied `TRACKING` is validated and never rewritten. A generated number is random and must not collide with a real parcel.

## Passes only if

- the payload has exactly `previousStatus`, `status`, `trackingNumber` and `updatedAt`;
- `trackingNumber`, `previousStatus` and `status` match the synthetic event;
- `updatedAt` parses and matches the outbox row's `created_at` within 2 s;
- the private `note` in the outbox payload never reaches the socket (the exact-key check also rejects any extra field);
- the update arrives within `TIMEOUT_MS`;
- the outbox row reaches `PUBLISHED` (the script retries briefly before deleting it).

A successful run prints `PASS: update received with only the four public fields; outbox row PUBLISHED (tracking …)` and exits 0. The script waits for `PUBLISHED`, then deletes its own outbox row by ID. On failure it leaves the row in place for diagnosis; the relay's normal published-event cleanup removes it later if it was published.

## Side effects on a full stack

The synthetic event is contract-valid: it has a random `parcel_id`, a random tracking number, and no owner fields. It carries no recipient details, so no email or SMS is sent.

The notification worker treats a status change with neither `customer_user_id` nor `merchant_id` as a permanent failure (`parcel.status_changed.v1 has no notification owner`) and dead-letters it. If the worker is running it dead-letters immediately; if it is stopped, the durable `notification.parcel.status_changed.v1` queue can hold the event until the worker restarts, and then dead-letter it. One run therefore leaves at most one message in `notification.parcel.status_changed.v1.dlq`; purge that queue if you need a clean run. The webhook worker consumes the same event and only logs it, so it does not dead-letter. The script does not stop or reconfigure workers, and production notification logic is not changed for the test.

## Clean up

Remove the temporary override and restore realtime to the tracked configuration:

```bash
rm -f "$override"
docker compose up -d --no-deps realtime
```

PowerShell:

```powershell
Remove-Item $override
docker compose up -d --no-deps realtime
```

Do not run `docker compose down -v`; it would delete local data.

## If it fails

| Symptom                                | Likely cause                                                                                                                  |
| -------------------------------------- | ----------------------------------------------------------------------------------------------------------------------------- |
| `cannot connect`                       | port 8085 not published, or realtime not running (`docker compose logs realtime`)                                             |
| `server closed the connection`         | origin or limit rejected; check `docker compose logs realtime`                                                                |
| `INVALID_MESSAGE`                      | subscribe must be flat: `{type, trackingNumber}`                                                                              |
| timeout, no update                     | relay not running, or the row is stuck `PENDING` (`SELECT status FROM parcel.outbox_events ORDER BY created_at DESC LIMIT 1`) |
| `could not read POSTGRES_USER` / `_DB` | postgres not running, or the command was not run from the repository root                                                     |
| `could not insert`                     | postgres not up, or migrations not applied                                                                                    |
