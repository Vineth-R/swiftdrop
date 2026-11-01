CREATE TABLE IF NOT EXISTS parcel.parcel_assignments (
  id                  UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  parcel_id           UUID         NOT NULL REFERENCES parcel.parcels(id) ON DELETE CASCADE,
  courier_id          UUID         NOT NULL,
  previous_courier_id UUID,
  assigned_by         UUID         NOT NULL,
  reason              VARCHAR(255),
  is_current          BOOLEAN      NOT NULL DEFAULT TRUE,
  assigned_at         TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
  unassigned_at       TIMESTAMPTZ,
  CONSTRAINT assignments_closed_has_time CHECK (is_current OR unassigned_at IS NOT NULL)
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_parcel_assignments_one_current
  ON parcel.parcel_assignments (parcel_id)
  WHERE is_current = TRUE;

CREATE INDEX IF NOT EXISTS idx_parcel_assignments_courier_workload
  ON parcel.parcel_assignments (courier_id)
  WHERE is_current = TRUE;

CREATE INDEX IF NOT EXISTS idx_parcel_assignments_parcel_idx
  ON parcel.parcel_assignments (parcel_id);

CREATE TABLE IF NOT EXISTS parcel.outbox_events (
  id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id     UUID         NOT NULL UNIQUE,
  event_type   VARCHAR(100) NOT NULL,
  routing_key  VARCHAR(100) NOT NULL,
  aggregate_id UUID         NOT NULL,
  payload      JSONB        NOT NULL,
  attempts     INT          NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error   TEXT,
  published_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_outbox_events_unpublished
  ON parcel.outbox_events (created_at ASC)
  WHERE published_at IS NULL;

CREATE INDEX IF NOT EXISTS idx_outbox_events_published_at
  ON parcel.outbox_events (published_at);

CREATE INDEX IF NOT EXISTS idx_outbox_events_aggregate
  ON parcel.outbox_events (aggregate_id);

