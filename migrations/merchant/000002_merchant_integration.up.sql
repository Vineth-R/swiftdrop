CREATE EXTENSION IF NOT EXISTS pgcrypto;

CREATE TABLE merchant.merchants (
    id UUID PRIMARY KEY,
    owner_user_id UUID NOT NULL UNIQUE,
    business_name TEXT NOT NULL CHECK (btrim(business_name) <> ''),
    business_reg_no TEXT CHECK (business_reg_no IS NULL OR btrim(business_reg_no) <> ''),
    contact_email TEXT NOT NULL CHECK (btrim(contact_email) <> ''),
    contact_phone TEXT NOT NULL CHECK (btrim(contact_phone) <> ''),
    pickup_address TEXT NOT NULL CHECK (btrim(pickup_address) <> ''),
    pickup_zone_id UUID NOT NULL,
    status TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING', 'APPROVED', 'REJECTED', 'SUSPENDED')),
    approved_by UUID,
    approved_at TIMESTAMPTZ,
    rejection_reason TEXT,
    suspension_reason TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    CHECK (
        (status = 'APPROVED' AND approved_by IS NOT NULL AND approved_at IS NOT NULL)
        OR (status <> 'APPROVED')
    ),
    CHECK ((status <> 'REJECTED') OR rejection_reason IS NOT NULL),
    CHECK ((status <> 'SUSPENDED') OR suspension_reason IS NOT NULL)
);

CREATE INDEX merchants_status_created_at_idx
    ON merchant.merchants (status, created_at DESC);

CREATE TABLE merchant.merchant_api_clients (
    id UUID PRIMARY KEY,
    merchant_id UUID NOT NULL REFERENCES merchant.merchants (id),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    key_prefix TEXT NOT NULL CHECK (btrim(key_prefix) <> ''),
    lookup_id VARCHAR(16) NOT NULL UNIQUE,
    key_hash CHAR(64) NOT NULL CHECK (key_hash ~ '^[0-9a-f]{64}$'),
    status TEXT NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE', 'REVOKED')),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ
);

CREATE INDEX merchant_api_clients_merchant_id_idx
    ON merchant.merchant_api_clients (merchant_id, created_at DESC);

CREATE UNIQUE INDEX merchant_api_clients_active_name_idx
    ON merchant.merchant_api_clients (merchant_id, name)
    WHERE status = 'ACTIVE';

CREATE TABLE merchant.merchant_orders (
    id UUID PRIMARY KEY,
    merchant_id UUID NOT NULL REFERENCES merchant.merchants (id),
    external_order_id TEXT NOT NULL CHECK (btrim(external_order_id) <> ''),
    idempotency_key VARCHAR(100) CHECK (idempotency_key IS NULL OR btrim(idempotency_key) <> ''),
    request_hash CHAR(64) NOT NULL CHECK (request_hash ~ '^[0-9a-f]{64}$'),
    parcel_id UUID NOT NULL,
    tracking_number TEXT NOT NULL UNIQUE CHECK (btrim(tracking_number) <> ''),
    recipient_name TEXT NOT NULL CHECK (btrim(recipient_name) <> ''),
    dropoff_city TEXT NOT NULL CHECK (btrim(dropoff_city) <> ''),
    weight_kg NUMERIC(6, 2) NOT NULL CHECK (weight_kg > 0),
    size_category VARCHAR(12) NOT NULL CHECK (size_category IN ('SMALL', 'MEDIUM', 'LARGE')),
    description TEXT,
    fee_amount NUMERIC(12, 2) NOT NULL CHECK (fee_amount >= 0),
    fee_breakdown JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(fee_breakdown) = 'object'),
    currency CHAR(3) NOT NULL CHECK (currency ~ '^[A-Z]{3}$'),
    last_known_status VARCHAR(20) NOT NULL CHECK (last_known_status IN ('CREATED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED')),
    last_status_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (merchant_id, external_order_id)
);

CREATE UNIQUE INDEX merchant_orders_merchant_idempotency_idx
    ON merchant.merchant_orders (merchant_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

CREATE INDEX merchant_orders_merchant_created_at_idx
    ON merchant.merchant_orders (merchant_id, created_at DESC);

CREATE INDEX merchant_orders_merchant_status_created_at_idx
    ON merchant.merchant_orders (merchant_id, last_known_status, created_at DESC);

CREATE TABLE merchant.merchant_order_status_events (
    id UUID PRIMARY KEY,
    order_id UUID NOT NULL REFERENCES merchant.merchant_orders (id),
    event_id UUID NOT NULL UNIQUE,
    previous_status TEXT,
    status VARCHAR(20) NOT NULL CHECK (status IN ('CREATED', 'PICKED_UP', 'IN_TRANSIT', 'OUT_FOR_DELIVERY', 'DELIVERED', 'FAILED')),
    occurred_at TIMESTAMPTZ NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX merchant_order_status_events_order_occurred_at_idx
    ON merchant.merchant_order_status_events (order_id, occurred_at ASC);

CREATE TABLE merchant.audit_logs (
    id UUID PRIMARY KEY,
    actor_user_id UUID NOT NULL,
    action TEXT NOT NULL CHECK (action IN ('APPROVED', 'REJECTED', 'SUSPENDED')),
    entity_type VARCHAR(40) NOT NULL,
    entity_id UUID,
    details JSONB,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX audit_logs_entity_idx ON merchant.audit_logs (entity_type, entity_id);
CREATE INDEX audit_logs_created_at_idx ON merchant.audit_logs (created_at DESC);
