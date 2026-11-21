# Notification migrations

The notification worker owns the `notification` schema.

- `000001_initial_schema` creates the schema.
- `000002_tables` is reserved by the platform foundation for the baseline tables from `docs/db.sql` (`notification_logs`, `processed_events`).
- `000003_notification_log_created_at_index` adds the `created_at` index used by the approved 180-day notification-log retention cleanup.
- Add new migrations from `000004` using `NNNNNN_short_description.up.sql` and `.down.sql`. Applied migrations are immutable.

Run migrations with `make migrate` from the repository root; do not apply them at service startup.
