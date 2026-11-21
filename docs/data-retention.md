# Data retention policy

This is the approved project retention policy. It is an operational and
privacy decision for this repository, not a claim of statutory or regulatory
compliance.

| Data                                       | Retention                     | Cleanup owner                |
| ------------------------------------------ | ----------------------------- | ---------------------------- |
| `notification.notification_logs`           | **180 days** (default)        | notification-worker          |
| `merchant.merchant_order_reservations`     | **Indefinitely**              | none                         |
| `notification.processed_events`            | **30 days** (existing policy) | notification-worker          |
| Identity verification/reset/refresh tokens | Existing cleanup policy       | identity                     |
| `identity.audit_logs`                      | **365 days** (operator)       | `scripts/prune-retention.sh` |
| `merchant.audit_logs`                      | **365 days** (operator)       | `scripts/prune-retention.sh` |
| `pricing.audit_logs`                       | **365 days** (operator)       | `scripts/prune-retention.sh` |
| `merchant.webhook_delivery_attempts`       | **90 days** (operator)        | `scripts/prune-retention.sh` |
| `parcel.outbox_events` rows in `FAILED`    | **30 days** recommended       | `outbox-tool` + operator     |

## Notification delivery logs

`notification.notification_logs` records delivery outcomes and the recipient
used for the delivery attempt. The approved retention period is **180 days**
(4,320 hours). It is long enough to investigate recent delivery failures and
short enough to avoid retaining personal data indefinitely.

Configuration:

```sh
NOTIFICATION_LOG_RETENTION=4320h
NOTIFICATION_LOG_CLEANUP_INTERVAL=24h
```

- Both values use Go duration syntax and must be strictly positive.
- Cleanup runs once at worker startup and then every configured interval.
- The cutoff is `time.Now().UTC() - NOTIFICATION_LOG_RETENTION`.
- The delete removes only rows where `created_at < cutoff`; a row exactly on the
  cutoff is retained.
- Cleanup runs serially in one goroutine, so runs cannot overlap within a worker.
- Multiple workers may run the same delete; the operation is idempotent and needs
  no distributed lock.
- The worker logs the deleted row count, duration, and a stable error message.
- A cleanup failure does not stop notification consumption and is not part of
  `/ready`. The next interval retries the cleanup.

The cleanup is a single database-side `DELETE` scoped to
`notification.notification_logs`. The `notification_logs_created_at_idx` index
keeps the cutoff lookup bounded. The scheduled cadence keeps steady-state runs
small; the first run on a large legacy table may delete many rows, so run it
during a maintenance window if the existing table is large. No application-side
row loading or per-row loop is used. `notification.processed_events` is not
deleted by this job.

## Merchant order reservations

`merchant.merchant_order_reservations` is retained **indefinitely** in this
milestone. The table coordinates API v1.1 merchant-order idempotency: a
reservation stores the idempotency key, normalized request hash and completion
state. Deleting a reservation would allow a later request with the same key and
a different payload to bypass duplicate detection and potentially create a
second order.

API v1.1 does not define an idempotency-key lifetime, so there is no safe
retention window yet. No reservation cleanup method, scheduler, migration or
delete operation is added by this policy.

The decision can be revisited if the API contract formally defines:

- how long an idempotency key remains reserved,
- what clients should do after that window, and
- how a replayed request is rejected or accepted after reservation expiry.

Until then, reservation growth is an accepted storage cost of correct
duplicate-order protection.

## Processed events

`notification.processed_events` keeps its existing **30-day** policy and
`NOTIFICATION_PROCESSED_EVENTS_RETENTION` /
`NOTIFICATION_PROCESSED_EVENTS_CLEANUP_INTERVAL` configuration. The
notification-log job does not modify those values, SQL or scheduler behavior.

Idempotency is guaranteed only while a processed-event row is retained; a
redelivery after retention can be processed again. This limitation is unchanged.

## Identity tokens

Identity refresh, password-reset and email-verification token cleanup keeps its
existing policy and `IDENTITY_TOKEN_RETENTION` /
`IDENTITY_TOKEN_CLEANUP_INTERVAL` configuration. This document does not change
it.

## Audit logs and webhook delivery attempts

The three audit-log tables and `merchant.webhook_delivery_attempts` are not
deleted by a service scheduler. They are pruned by an explicit operator
command so an audit or incident review cannot lose rows silently:

```sh
make prune-retention          # dry run, prints eligible row counts
make prune-retention-apply    # deletes rows past the windows below
```

| Table                                | Default window | Environment override             |
| ------------------------------------ | -------------: | -------------------------------- |
| `identity.audit_logs`                |       365 days | `AUDIT_RETENTION_DAYS`           |
| `merchant.audit_logs`                |       365 days | `AUDIT_RETENTION_DAYS`           |
| `pricing.audit_logs`                 |       365 days | `AUDIT_RETENTION_DAYS`           |
| `merchant.webhook_delivery_attempts` |        90 days | `WEBHOOK_ATTEMPT_RETENTION_DAYS` |

Both windows must be at least 30 days. `APPLY=1` (which the
`prune-retention-apply` target sets) is required to delete anything; the default
is a read-only dry run. Deletion runs in bounded batches (`BATCH_SIZE`, default 5000) for a bounded number of rounds (`MAX_BATCHES`, default 100), so one run
does not hold a long lock or build one huge transaction. Re-running the command
is safe and continues from the remaining rows. The script never touches
notification logs, processed events, merchant orders, reservations, parcel rows
or proof objects.

## Failed outbox events

`parcel.outbox_events` rows move to `FAILED` after five publish attempts. They
are retained for investigation; the relay's automatic cleanup deletes only
`PUBLISHED` rows older than 14 days. Operators have two explicit tools:

```sh
make outbox-failed                                   # read-only backlog report
make outbox-replay OUTBOX_ARGS="-older-than 720h"    # FAILED -> PENDING
make outbox-expire OUTBOX_ARGS="-older-than 720h"    # delete old FAILED rows
```

`outbox-replay` and `outbox-expire` are dry runs unless `-apply` is passed; the
Make targets pass it. Replaying resets `attempts` and clears the claim so the
normal relay owns the retry. Expiry should only run after the backlog has been
reviewed or replayed. The tool never changes `PENDING` or `PUBLISHED` rows.

## Migration and production notes

The retention index migration is
`migrations/notification/000003_notification_log_created_at_index`.

`CREATE INDEX` is transactional in golang-migrate. If the production table is
already large, create the same index concurrently in a maintenance window before
applying the migration:

```sql
CREATE INDEX CONCURRENTLY IF NOT EXISTS notification_logs_created_at_idx
  ON notification.notification_logs (created_at);
```

The migration then becomes a no-op because it uses `IF NOT EXISTS`. Do not run
`CREATE INDEX CONCURRENTLY` inside a transaction.
