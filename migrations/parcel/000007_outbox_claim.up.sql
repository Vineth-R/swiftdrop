-- Add an atomic claim seam to the parcel outbox relay.
--
-- The relay previously selected PENDING rows without locking them and only
-- compared `status = 'PENDING'` in the later UPDATE. Two parcel replicas
-- running the relay could therefore both select and publish the same row,
-- producing duplicate broker publications.
--
-- A relay now claims a batch in one statement:
--   UPDATE ... SET claim_token = gen_random_uuid(), claimed_at = now()
--   WHERE id IN (SELECT ... WHERE status='PENDING' AND
--                (claimed_at IS NULL OR claimed_at < stale_cutoff)
--                ORDER BY created_at LIMIT n FOR UPDATE SKIP LOCKED)
--
-- `FOR UPDATE SKIP LOCKED` makes concurrent claimants take disjoint rows, and
-- `claimed_at` lets a row whose claimant died mid-publish be reclaimed after a
-- bounded window. Claims are committed immediately, so the broker publish no
-- longer runs while database row locks are held.
--
-- The columns are additive and nullable, so existing rows and the baseline
-- outbox contract are unchanged.

BEGIN;

ALTER TABLE parcel.outbox_events
  ADD COLUMN IF NOT EXISTS claim_token UUID,
  ADD COLUMN IF NOT EXISTS claimed_at TIMESTAMPTZ;

-- A stale claim is reclaimable. The existing outbox_status_created
-- (status, created_at) index already serves the pending FIFO scan, so no
-- duplicate index is added here.

COMMIT;
