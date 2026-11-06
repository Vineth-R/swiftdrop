-- Roll back migration 000005: restore last_error to VARCHAR(255), drop the
-- added indexes, and reassert the baseline event-type/routing-key constraints.
--
-- Rollback precondition: 000005 widens last_error to TEXT so diagnostic values
-- are never truncated. Reverting to VARCHAR(255) fails, and the whole
-- transaction rolls back, when any stored value is longer than 255 characters.
-- This migration never truncates diagnostic data; shorten or archive the long
-- values first, then rerun the rollback.

BEGIN;

DO $$
BEGIN
  IF EXISTS (
    SELECT 1 FROM parcel.outbox_events WHERE length(last_error) > 255
  ) THEN
    RAISE EXCEPTION
      'cannot roll back 000005_outbox_event_types: outbox_events.last_error contains values longer than 255 characters; archive or shorten them first'
      USING ERRCODE = '22001';
  END IF;
END
$$;

DROP INDEX IF EXISTS parcel.idx_parcel_assignments_courier_workload;
DROP INDEX IF EXISTS parcel.idx_outbox_events_published_cleanup;

ALTER TABLE parcel.outbox_events
  DROP CONSTRAINT IF EXISTS outbox_events_event_type_check,
  DROP CONSTRAINT IF EXISTS outbox_events_routing_key_check,
  DROP CONSTRAINT IF EXISTS outbox_type_key_pair;

ALTER TABLE parcel.outbox_events
  ALTER COLUMN last_error TYPE VARCHAR(255);

ALTER TABLE parcel.outbox_events
  ADD CONSTRAINT outbox_events_event_type_check CHECK (
    event_type IN ('parcel.created', 'parcel.assigned', 'parcel.status_changed')
  ),
  ADD CONSTRAINT outbox_events_routing_key_check CHECK (
    routing_key IN ('parcel.created', 'parcel.assigned', 'parcel.status.changed')
  ),
  ADD CONSTRAINT outbox_type_key_pair CHECK (
    (event_type = 'parcel.created'        AND routing_key = 'parcel.created') OR
    (event_type = 'parcel.assigned'       AND routing_key = 'parcel.assigned') OR
    (event_type = 'parcel.status_changed' AND routing_key = 'parcel.status.changed')
  );

COMMIT;
