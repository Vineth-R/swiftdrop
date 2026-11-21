CREATE TABLE IF NOT EXISTS parcel.parcels (
  id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  tracking_number    VARCHAR(20)   NOT NULL UNIQUE,      -- SDK + 8 (customer), SDN + 8 (merchant)
  customer_user_id   UUID,                               -- ref users
  merchant_id        UUID,                               -- ref merchants
  external_order_id  VARCHAR(100),
  idempotency_key    VARCHAR(100),
  sender_name        VARCHAR(150)  NOT NULL,
  sender_phone       VARCHAR(20)   NOT NULL,
  pickup_address     VARCHAR(255)  NOT NULL,
  pickup_city        VARCHAR(100)  NOT NULL,
  pickup_zone_id     UUID          NOT NULL,             -- ref zones
  recipient_name     VARCHAR(150)  NOT NULL,
  recipient_phone    VARCHAR(20)   NOT NULL,
  dropoff_address    VARCHAR(255)  NOT NULL,
  dropoff_city       VARCHAR(100)  NOT NULL,
  dropoff_zone_id    UUID          NOT NULL,             -- ref zones
  weight_kg          NUMERIC(6,2)  NOT NULL CHECK (weight_kg > 0),
  size_category      VARCHAR(12)   NOT NULL
                     CHECK (size_category IN ('SMALL','MEDIUM','LARGE','OVERSIZED')),
  description        VARCHAR(255),
  status             VARCHAR(20)   NOT NULL DEFAULT 'CREATED'
                     CHECK (status IN ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  failure_reason     VARCHAR(255),
  current_courier_id UUID,                               -- copy of the active assignment
  price_amount       NUMERIC(12,2) NOT NULL CHECK (price_amount >= 0),
  currency           CHAR(3)       NOT NULL DEFAULT 'LKR',
  price_breakdown    JSONB         NOT NULL,             -- base_fee, weight_charge, size_surcharge
  created_by_user_id UUID          NOT NULL,
  created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
  delivered_at       TIMESTAMPTZ,
  failed_at          TIMESTAMPTZ,
  delivered_to_name  VARCHAR(150),
  -- exactly one owner
  CONSTRAINT parcels_one_owner CHECK ((customer_user_id IS NOT NULL) <> (merchant_id IS NOT NULL)),
  -- ADDED: tracking prefix matches the owner
  CONSTRAINT parcels_tracking_prefix CHECK (
    (customer_user_id IS NOT NULL AND tracking_number LIKE 'SDK%') OR
    (merchant_id      IS NOT NULL AND tracking_number LIKE 'SDN%')),
  -- FAILED needs a reason (5 to 255 characters); ADDED: and failed_at
  CONSTRAINT parcels_failed_fields CHECK (
    status <> 'FAILED' OR (failure_reason IS NOT NULL AND char_length(failure_reason) >= 5 AND failed_at IS NOT NULL)),
  -- DELIVERED needs delivered_at; ADDED: and the recipient name (API 6.3)
  CONSTRAINT parcels_delivered_fields CHECK (
    status <> 'DELIVERED' OR (delivered_at IS NOT NULL AND delivered_to_name IS NOT NULL)),
  -- ADDED: stored total equals the breakdown
  CONSTRAINT parcels_price_breakdown CHECK (
    price_breakdown->'base_fee' IS NOT NULL AND
    price_breakdown->'weight_charge' IS NOT NULL AND
    price_breakdown->'size_surcharge' IS NOT NULL AND
    price_amount = (price_breakdown->>'base_fee')::numeric
                 + (price_breakdown->>'weight_charge')::numeric
                 + (price_breakdown->>'size_surcharge')::numeric)
);
CREATE UNIQUE INDEX IF NOT EXISTS parcels_merchant_external_uq
  ON parcel.parcels (merchant_id, external_order_id)
  WHERE merchant_id IS NOT NULL AND external_order_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS parcels_customer_idem_uq
  ON parcel.parcels (customer_user_id, idempotency_key)
  WHERE customer_user_id IS NOT NULL AND idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS parcels_customer_idx     ON parcel.parcels (customer_user_id);
CREATE INDEX IF NOT EXISTS parcels_merchant_idx     ON parcel.parcels (merchant_id);
CREATE INDEX IF NOT EXISTS parcels_courier_idx      ON parcel.parcels (current_courier_id);
CREATE INDEX IF NOT EXISTS parcels_status_created   ON parcel.parcels (status, created_at);
CREATE INDEX IF NOT EXISTS parcels_created_idx      ON parcel.parcels (created_at);
CREATE INDEX IF NOT EXISTS parcels_delivered_idx    ON parcel.parcels (delivered_at);
CREATE INDEX IF NOT EXISTS parcels_failed_idx       ON parcel.parcels (failed_at);
CREATE INDEX IF NOT EXISTS parcels_pickup_zone_idx  ON parcel.parcels (pickup_zone_id, created_at);


CREATE TABLE IF NOT EXISTS parcel.parcel_status_history (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  parcel_id       UUID         NOT NULL REFERENCES parcel.parcels(id),
  previous_status VARCHAR(20)
                  CHECK (previous_status IS NULL OR previous_status IN
                    ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  new_status      VARCHAR(20)  NOT NULL
                  CHECK (new_status IN
                    ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  actor_user_id   UUID,
  actor_role      VARCHAR(20),
  note            VARCHAR(500),
  failure_reason  VARCHAR(255),
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT history_failed_reason CHECK (new_status <> 'FAILED' OR failure_reason IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS history_parcel_idx     ON parcel.parcel_status_history (parcel_id, created_at);
CREATE INDEX IF NOT EXISTS history_status_created ON parcel.parcel_status_history (new_status, created_at);


CREATE TABLE IF NOT EXISTS parcel.parcel_assignments (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  parcel_id     UUID         NOT NULL REFERENCES parcel.parcels(id),
  courier_id    UUID         NOT NULL,                   -- ref users
  assigned_by   UUID         NOT NULL,                   -- ref users
  assigned_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  unassigned_at TIMESTAMPTZ,
  is_current    BOOLEAN      NOT NULL DEFAULT true,
  reason        VARCHAR(255),
  -- ADDED: a closed assignment must say when it closed
  CONSTRAINT assignments_closed_has_time CHECK (is_current OR unassigned_at IS NOT NULL)
);
-- one current courier per parcel
CREATE UNIQUE INDEX IF NOT EXISTS assignments_one_current_uq
  ON parcel.parcel_assignments (parcel_id) WHERE is_current;
CREATE INDEX IF NOT EXISTS assignments_courier_idx ON parcel.parcel_assignments (courier_id, is_current);


CREATE TABLE IF NOT EXISTS parcel.delivery_proofs (
  id               UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  parcel_id        UUID         NOT NULL REFERENCES parcel.parcels(id),
  recipient_name   VARCHAR(150),
  note             VARCHAR(500),
  photo_path       VARCHAR(255),
  photo_mime_type  VARCHAR(30)  CHECK (photo_mime_type IS NULL OR photo_mime_type IN ('image/jpeg','image/png')),
  photo_size_bytes INT          CHECK (photo_size_bytes IS NULL OR (photo_size_bytes > 0 AND photo_size_bytes <= 5242880)),
  captured_by      UUID         NOT NULL,                -- courier (ref users)
  created_at       TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT proofs_photo_fields CHECK (
    photo_path IS NULL OR (photo_mime_type IS NOT NULL AND photo_size_bytes IS NOT NULL)),
  CONSTRAINT proofs_not_empty CHECK (
    recipient_name IS NOT NULL OR note IS NOT NULL OR photo_path IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS proofs_parcel_idx ON parcel.delivery_proofs (parcel_id, created_at);


CREATE TABLE IF NOT EXISTS parcel.outbox_events (
  id           UUID         PRIMARY KEY,                 -- same value as the event's eventId
  event_type   VARCHAR(50)  NOT NULL
               CHECK (event_type IN ('parcel.created','parcel.assigned','parcel.status_changed')),
  routing_key  VARCHAR(60)  NOT NULL
               CHECK (routing_key IN ('parcel.created','parcel.assigned','parcel.status.changed')),
  payload      JSONB        NOT NULL,                    -- no addresses, phones, tokens or photos
  status       VARCHAR(12)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','PUBLISHED','FAILED')),
  attempts     INT          NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error   VARCHAR(255),
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  published_at TIMESTAMPTZ,
  -- ADDED: event type and routing key must belong together (underscore vs dots)
  CONSTRAINT outbox_type_key_pair CHECK (
    (event_type = 'parcel.created'        AND routing_key = 'parcel.created') OR
    (event_type = 'parcel.assigned'       AND routing_key = 'parcel.assigned') OR
    (event_type = 'parcel.status_changed' AND routing_key = 'parcel.status.changed'))
);
CREATE INDEX IF NOT EXISTS outbox_status_created ON parcel.outbox_events (status, created_at);


-- ADDED: status transitions enforced in the database (document section 9)
CREATE OR REPLACE FUNCTION parcel.enforce_status_transition() RETURNS trigger AS $$
BEGIN
  IF (OLD.status, NEW.status) IN (
       ('CREATED','PICKED_UP'),          ('CREATED','FAILED'),
       ('PICKED_UP','IN_TRANSIT'),       ('PICKED_UP','FAILED'),
       ('IN_TRANSIT','OUT_FOR_DELIVERY'),('IN_TRANSIT','FAILED'),
       ('OUT_FOR_DELIVERY','DELIVERED'), ('OUT_FOR_DELIVERY','FAILED')) THEN
    RETURN NEW;
  END IF;
  RAISE EXCEPTION 'INVALID_TRANSITION: % to %', OLD.status, NEW.status USING ERRCODE = '23514';
END;
$$ LANGUAGE plpgsql;


DROP TRIGGER IF EXISTS parcels_status_transition ON parcel.parcels;
CREATE TRIGGER parcels_status_transition
  BEFORE UPDATE OF status ON parcel.parcels
  FOR EACH ROW WHEN (OLD.status IS DISTINCT FROM NEW.status)
  EXECUTE FUNCTION parcel.enforce_status_transition();


-- ADDED: history rows are never changed or deleted
CREATE OR REPLACE FUNCTION parcel.forbid_history_change() RETURNS trigger AS $$
BEGIN
  RAISE EXCEPTION 'parcel_status_history is append only';
END;
$$ LANGUAGE plpgsql;


DROP TRIGGER IF EXISTS history_append_only ON parcel.parcel_status_history;
CREATE TRIGGER history_append_only
  BEFORE UPDATE OR DELETE ON parcel.parcel_status_history
  FOR EACH ROW EXECUTE FUNCTION parcel.forbid_history_change();


DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'parcel.parcels'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %s', t);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', t);
  END LOOP;
END $$;
