-- Dispatch Milestone 1: extend the baseline assignment and outbox tables.
--
-- parcel_assignments and outbox_events are created by 000002_tables, so this
-- migration must not recreate them. It only adds the metadata the dispatch
-- workflow needs:
--   * previous_courier_id for reassignment history returned by API 5.2
--   * updated_at so closing an assignment is auditable
--   * aggregate_type/aggregate_id so a future outbox relay can group events
--
-- courier_id, assigned_by and previous_courier_id intentionally have no
-- foreign key: users are owned by the identity service, and service schemas
-- must not depend on each other directly.

ALTER TABLE parcel.parcel_assignments
  ADD COLUMN IF NOT EXISTS previous_courier_id UUID,
  ADD COLUMN IF NOT EXISTS updated_at TIMESTAMPTZ NOT NULL DEFAULT now();

DROP TRIGGER IF EXISTS set_updated_at ON parcel.parcel_assignments;
CREATE TRIGGER set_updated_at
  BEFORE UPDATE ON parcel.parcel_assignments
  FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();

ALTER TABLE parcel.outbox_events
  ADD COLUMN IF NOT EXISTS aggregate_type VARCHAR(50),
  ADD COLUMN IF NOT EXISTS aggregate_id UUID;

ALTER TABLE parcel.outbox_events
  DROP CONSTRAINT IF EXISTS outbox_aggregate_pair;
ALTER TABLE parcel.outbox_events
  ADD CONSTRAINT outbox_aggregate_pair CHECK (
    (aggregate_type IS NULL) = (aggregate_id IS NULL));

CREATE INDEX IF NOT EXISTS outbox_aggregate_idx
  ON parcel.outbox_events (aggregate_type, aggregate_id);
