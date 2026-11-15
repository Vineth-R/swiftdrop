# Outage recovery evidence

`outage-recovery.sh` runs against a disposable real-stack Compose project. It:

1. verifies baseline readiness for the affected and unaffected services;
2. stops RabbitMQ and waits until parcel, merchant, realtime, notification and
   webhook readiness report down;
3. checks identity and pricing remain ready;
4. restarts RabbitMQ and waits for every readiness check to recover;
5. runs `delivery-flow.mjs` to create, assign and move a parcel, then waits for
   public tracking to reflect the new status.

The CI real-stack job runs this after Playwright with its seeded accounts.

```sh
COMPOSE_PROJECT_NAME=swiftdrop-realstack \
REAL_STACK_BASE_URL=http://localhost:8080 \
REAL_STACK_CUSTOMER_EMAIL=... REAL_STACK_CUSTOMER_PASSWORD=... \
REAL_STACK_DISPATCHER_EMAIL=... REAL_STACK_DISPATCHER_PASSWORD=... \
REAL_STACK_COURIER_1_EMAIL=... REAL_STACK_COURIER_1_PASSWORD=... \
bash tests/recovery/outage-recovery.sh
```
