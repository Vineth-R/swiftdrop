CREATE TABLE merchant.merchant_orders (
    id UUID PRIMARY KEY,
    merchant_id UUID NOT NULL REFERENCES merchant.merchants (id),
    external_order_id VARCHAR(100) NOT NULL CHECK (btrim(external_order_id) <> ''),
    idempotency_key VARCHAR(100) CHECK (idempotency_key IS NULL OR btrim(idempotency_key) <> ''),
    request_hash CHAR(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    parcel_id UUID NOT NULL UNIQUE,
    tracking_number VARCHAR(20) NOT NULL UNIQUE CHECK (btrim(tracking_number) <> ''),
    recipient_name VARCHAR(150) NOT NULL CHECK (btrim(recipient_name) <> ''),
    dropoff_city VARCHAR(100) NOT NULL CHECK (btrim(dropoff_city) <> ''),
    weight_kg NUMERIC(6,2) NOT NULL CHECK (weight_kg > 0),
    size_category VARCHAR(12) NOT NULL CHECK (size_category IN ('SMALL', 'MEDIUM', 'LARGE', 'OVERSIZED')),
    description VARCHAR(255),
    fee_amount NUMERIC(12,2) NOT NULL CHECK (fee_amount >= 0),
    fee_breakdown JSONB NOT NULL CHECK (jsonb_typeof(fee_breakdown) = 'object'),
    currency CHAR(3) NOT NULL DEFAULT 'LKR' CHECK (currency = 'LKR'),
    last_known_status VARCHAR(20) NOT NULL CHECK (last_known_status IN ('CREATED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED')),
    last_status_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, external_order_id)
);

CREATE UNIQUE INDEX merchant_orders_merchant_idempotency_idx ON merchant.merchant_orders (merchant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX merchant_orders_merchant_created_at_idx ON merchant.merchant_orders (merchant_id, created_at DESC);
CREATE INDEX merchant_orders_merchant_status_idx ON merchant.merchant_orders (merchant_id, last_known_status, created_at DESC);

CREATE TABLE merchant.merchant_order_status_events (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES merchant.merchant_orders (id),
    event_id UUID NOT NULL UNIQUE,
    previous_status VARCHAR(20),
    status VARCHAR(20) NOT NULL CHECK (status IN ('CREATED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED')),
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX merchant_order_status_events_order_occurred_at_idx ON merchant.merchant_order_status_events (order_id, occurred_at);
