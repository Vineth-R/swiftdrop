CREATE TABLE IF NOT EXISTS parcel.parcels (
  id                 UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  tracking_number    VARCHAR(20)   NOT NULL UNIQUE,
  customer_user_id   UUID,
  merchant_id        UUID,
  external_order_id  VARCHAR(100),
  idempotency_key    VARCHAR(100),
  sender_name        VARCHAR(150)  NOT NULL,
  sender_phone       VARCHAR(20)   NOT NULL,
  pickup_address     VARCHAR(255)  NOT NULL,
  pickup_city        VARCHAR(100)  NOT NULL,
  pickup_zone_id     UUID          NOT NULL,
  recipient_name     VARCHAR(150)  NOT NULL,
  recipient_phone    VARCHAR(20)   NOT NULL,
  dropoff_address    VARCHAR(255)  NOT NULL,
  dropoff_city       VARCHAR(100)  NOT NULL,
  dropoff_zone_id    UUID          NOT NULL,
  weight_kg          NUMERIC(6,2)  NOT NULL CHECK (weight_kg > 0),
  size_category      VARCHAR(12)   NOT NULL
                     CHECK (size_category IN ('SMALL','MEDIUM','LARGE','OVERSIZED')),
  description        VARCHAR(255),
  status             VARCHAR(20)   NOT NULL DEFAULT 'CREATED'
                     CHECK (status IN ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  failure_reason     VARCHAR(255),
  current_courier_id UUID,
  price_amount       NUMERIC(12,2) NOT NULL CHECK (price_amount >= 0),
  currency           CHAR(3)       NOT NULL DEFAULT 'LKR',
  price_breakdown    JSONB         NOT NULL,
  created_by_user_id UUID          NOT NULL,
  created_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ   NOT NULL DEFAULT now(),
  delivered_at       TIMESTAMPTZ,
  failed_at          TIMESTAMPTZ,
  delivered_to_name  VARCHAR(150),
  CONSTRAINT parcels_one_owner CHECK ((customer_user_id IS NOT NULL) <> (merchant_id IS NOT NULL)),
  CONSTRAINT parcels_tracking_prefix CHECK (
    (customer_user_id IS NOT NULL AND tracking_number LIKE 'SDK%') OR
    (merchant_id      IS NOT NULL AND tracking_number LIKE 'SDN%')),
  CONSTRAINT parcels_failed_fields CHECK (
    status <> 'FAILED' OR (failure_reason IS NOT NULL AND char_length(failure_reason) >= 5 AND failed_at IS NOT NULL)),
  CONSTRAINT parcels_delivered_fields CHECK (
    status <> 'DELIVERED' OR (delivered_at IS NOT NULL AND delivered_to_name IS NOT NULL)),
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
  parcel_id       UUID         NOT NULL REFERENCES parcel.parcels(id) ON DELETE CASCADE,
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

