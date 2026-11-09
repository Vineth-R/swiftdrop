# Realtime smoke test

Proves the whole chain once, on a real stack: `parcel.outbox_events` → parcel relay → RabbitMQ → realtime consumer → WebSocket.

## Run it

Start the stack with the realtime port published. Compose does not publish 8085 by default, so use an override outside the repo, for example `C:\temp\ws-ports.yml`:

```yaml
services:
  realtime:
    ports:
      - '8085:8085'
```

```powershell
docker compose -f docker-compose.yml -f C:\temp\ws-ports.yml up -d postgres rabbitmq parcel realtime
node scripts/realtime-smoke.mjs
```

It subscribes, waits for `subscribed` (there is no replay), inserts one outbox row, and expects a `parcel.status.updated` message.

## Passes only if

- the message carries exactly `previousStatus`, `status`, `trackingNumber`, `updatedAt`;
- the extra `recipient_phone` in the outbox payload never reaches the socket;
- the update arrives within 15 s (`TIMEOUT_MS`).

## If it fails

| Symptom            | Likely cause                                                                                                                  |
| ------------------ | ----------------------------------------------------------------------------------------------------------------------------- |
| `cannot connect`   | port 8085 not published, or realtime not running (`docker compose logs realtime`)                                             |
| `INVALID_MESSAGE`  | subscribe must be flat: `{type, trackingNumber}`                                                                              |
| timeout, no update | relay not running, or outbox row stuck `PENDING` (`SELECT status FROM parcel.outbox_events ORDER BY created_at DESC LIMIT 1`) |
| `could not insert` | postgres service not up, or wrong credentials                                                                                 |
