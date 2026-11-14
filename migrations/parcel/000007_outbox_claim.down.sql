-- Reverse 000007_outbox_claim. The claim columns only exist to coordinate the
-- relay; dropping them restores the pre-000007 outbox shape. Any in-flight
-- claim is discarded, which is safe because the relay is stopped before a
-- migration is applied.

BEGIN;

ALTER TABLE parcel.outbox_events
  DROP COLUMN IF EXISTS claim_token,
  DROP COLUMN IF EXISTS claimed_at;

COMMIT;
