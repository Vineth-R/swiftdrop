-- Roll back migration 000005: restore baseline constraints and drop added indexes.

BEGIN;

DROP INDEX IF EXISTS parcel.idx_parcel_assignments_courier_workload;
DROP INDEX IF EXISTS parcel.idx_outbox_events_published_cleanup;
DROP INDEX IF EXISTS parcel.idx_outbox_events_pending_relay;

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

