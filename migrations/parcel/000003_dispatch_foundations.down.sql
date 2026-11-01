-- Roll back only the additions made by 000003_dispatch_foundations.
-- The baseline parcel_assignments and outbox_events tables and their data
-- remain in place.

DROP TRIGGER IF EXISTS set_updated_at ON parcel.parcel_assignments;

DROP INDEX IF EXISTS parcel.outbox_aggregate_idx;

ALTER TABLE parcel.outbox_events
  DROP CONSTRAINT IF EXISTS outbox_aggregate_pair,
  DROP COLUMN IF EXISTS aggregate_id,
  DROP COLUMN IF EXISTS aggregate_type;

ALTER TABLE parcel.parcel_assignments
  DROP COLUMN IF EXISTS updated_at,
  DROP COLUMN IF EXISTS previous_courier_id;
