# Realtime smoke test

Follows one synthetic outbox row through the realtime path on a real stack:

`parcel.outbox_events` → parcel relay → RabbitMQ → realtime consumer → WebSocket.

It does not run the courier status-update transaction (parcel row, history and outbox together). It injects the outbox row directly, so it checks the transport and the projection, not the parcel service's business logic.

## Run it

Compose does not publish the realtime port (8085), so use a throwaway override file outside the repo.

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

The script subscribes, waits for `subscribed` (there is no replay), inserts one outbox row, and expects a `parcel.status.updated` message. It uses a random tracking number, so it never touches a real parcel. Set `TRACKING` (format `SDK` or `SDN` plus 8 letters or digits) to choose one.

## Passes only if

- the message has exactly `previousStatus`, `status`, `trackingNumber` and `updatedAt`;
- all four values are right, and `updatedAt` matches the outbox row's creation time (within 2 s);
- the extra `recipient_phone` in the outbox payload never reaches the socket;
- it arrives within 15 s (`TIMEOUT_MS`).

On success the script deletes its outbox row.

## Side effects on a full stack

The event is valid for the contract (it has a random `parcel_id`), but that parcel does not exist. If the notification or webhook worker is running, it cannot resolve the parcel and may dead-letter the message, which leaves one message in its `.dlq` queue. Run the test with those workers stopped (the command above starts only the four services it needs), or purge the queue afterwards.

## Settings

`WS_URL`, `TRACKING`, `TIMEOUT_MS`, and `POSTGRES_USER` / `POSTGRES_DB` (the same variables Compose uses, default `swiftdrop`).

## If it fails

| Symptom                        | Likely cause                                                                                                                  |
| ------------------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `cannot connect`               | port 8085 not published, or realtime not running (`docker compose logs realtime`)                                             |
| `server closed the connection` | origin or limit rejected; check `docker compose logs realtime`                                                                |
| `INVALID_MESSAGE`              | subscribe must be flat: `{type, trackingNumber}`                                                                              |
| timeout, no update             | relay not running, or the row is stuck `PENDING` (`SELECT status FROM parcel.outbox_events ORDER BY created_at DESC LIMIT 1`) |
| `could not insert`             | postgres not up, or wrong `POSTGRES_USER` / `POSTGRES_DB`                                                                     |
