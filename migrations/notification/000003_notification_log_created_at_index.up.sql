-- Supports the 180-day notification-log retention cleanup. The cleanup deletes
-- by created_at, so the index keeps each scheduled run bounded instead of
-- scanning the whole delivery log.
--
-- golang-migrate runs this migration in a transaction, so CREATE INDEX
-- CONCURRENTLY cannot be used here. On a large existing production table,
-- pre-create the same index concurrently during a maintenance window; the
-- IF NOT EXISTS clause makes this migration a no-op afterwards.

CREATE INDEX IF NOT EXISTS notification_logs_created_at_idx
  ON notification.notification_logs (created_at);
