-- Align the parcel outbox and assignment indexes with the approved API v1.1
-- section 11.4 and Database Design Document v1.1 section 5.5 contracts.
--
-- The outbox stores unversioned storage labels. The relay owns the mapping to
-- the versioned bus events (`parcel.created.v1`, `parcel.assigned.v1`,
-- `parcel.status_changed.v1`); it must not copy the stored routing key.
-- This migration reasserts the exact storage-label contract and does not add
-- `.v1` labels to the table.

BEGIN;

-- 1. Reassert the outbox event-type and routing-key constraints with their
--    canonical names. The definitions are the approved API v1.1 pairs:
--      parcel.created        -> parcel.created
--      parcel.assigned       -> parcel.assigned
--      parcel.status_changed -> parcel.status.changed
--    The underscore in the event type and the dots in the status routing key
--    are intentional and must not be normalised.
ALTER TABLE parcel.outbox_events
  DROP CONSTRAINT IF EXISTS outbox_events_event_type_check,
  DROP CONSTRAINT IF EXISTS outbox_events_routing_key_check,
  DROP CONSTRAINT IF EXISTS outbox_type_key_pair;

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

-- 2. Allow last_error to hold full diagnostic messages without truncation.
ALTER TABLE parcel.outbox_events
  ALTER COLUMN last_error TYPE TEXT;

-- 3. Index for the 14-day retention cleanup of published events
--    (Database Design Document v1.1 section 10).
CREATE INDEX IF NOT EXISTS idx_outbox_events_published_cleanup
  ON parcel.outbox_events (published_at)
  WHERE status = 'PUBLISHED';

-- 4. Partial index for the current-assignment courier workload query
--    (API v1.1 section 5.3). The existing assignments_courier_idx covers every
--    historical assignment; this partial index holds only current rows.
CREATE INDEX IF NOT EXISTS idx_parcel_assignments_courier_workload
  ON parcel.parcel_assignments (courier_id)
  WHERE is_current = TRUE;

-- The pending FIFO relay poll is already served by outbox_status_created
-- (status, created_at). No duplicate partial index is added.

COMMIT;
