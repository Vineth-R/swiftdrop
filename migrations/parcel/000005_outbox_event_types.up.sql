-- Forward migration to align parcel.outbox_events and parcel.parcel_assignments
-- with event contract versioning (API v1.1 Section 11, contracts/events/README.md)
-- and outbox relay performance requirements.

BEGIN;

-- 1. Relax outbox CHECK constraints to permit both canonical versioned (.v1)
-- and legacy unversioned storage labels.
ALTER TABLE parcel.outbox_events
  DROP CONSTRAINT IF EXISTS outbox_events_event_type_check,
  DROP CONSTRAINT IF EXISTS outbox_events_routing_key_check,
  DROP CONSTRAINT IF EXISTS outbox_type_key_pair;

ALTER TABLE parcel.outbox_events
  ADD CONSTRAINT outbox_events_event_type_check CHECK (
    event_type IN (
      'parcel.created',
      'parcel.assigned',
      'parcel.status_changed',
      'parcel.created.v1',
      'parcel.assigned.v1',
      'parcel.status_changed.v1'
    )
  ),
  ADD CONSTRAINT outbox_events_routing_key_check CHECK (
    routing_key IN (
      'parcel.created',
      'parcel.assigned',
      'parcel.status.changed',
      'parcel.status_changed',
      'parcel.created.v1',
      'parcel.assigned.v1',
      'parcel.status.changed.v1',
      'parcel.status_changed.v1'
    )
  ),
  ADD CONSTRAINT outbox_type_key_pair CHECK (
    (event_type IN ('parcel.created', 'parcel.created.v1') AND
     routing_key IN ('parcel.created', 'parcel.created.v1')) OR
    (event_type IN ('parcel.assigned', 'parcel.assigned.v1') AND
     routing_key IN ('parcel.assigned', 'parcel.assigned.v1')) OR
    (event_type IN ('parcel.status_changed', 'parcel.status_changed.v1') AND
     routing_key IN ('parcel.status.changed', 'parcel.status_changed', 'parcel.status.changed.v1', 'parcel.status_changed.v1'))
  );

-- 2. Allow last_error to hold full error messages without truncation.
ALTER TABLE parcel.outbox_events
  ALTER COLUMN last_error TYPE TEXT;

-- 3. Dedicated indexes for the Transactional Outbox Relay:
-- Polling pending events in FIFO order
CREATE INDEX IF NOT EXISTS idx_outbox_events_pending_relay
  ON parcel.outbox_events (created_at ASC)
  WHERE status = 'PENDING';

-- 14-day retention cleanup of published events (DB Design Doc v1.1 Section 10)
CREATE INDEX IF NOT EXISTS idx_outbox_events_published_cleanup
  ON parcel.outbox_events (published_at)
  WHERE status = 'PUBLISHED';

-- 4. Workload index on parcel_assignments for GET /dispatch/workload (API 5.3)
CREATE INDEX IF NOT EXISTS idx_parcel_assignments_courier_workload
  ON parcel.parcel_assignments (courier_id)
  WHERE is_current = TRUE;

COMMIT;

