CREATE TABLE IF NOT EXISTS merchant.merchants (
  id                UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  owner_user_id     UUID         NOT NULL UNIQUE,        -- ref users, one merchant per owner
  business_name     VARCHAR(150) NOT NULL,
  business_reg_no   VARCHAR(50),
  contact_email     VARCHAR(255) NOT NULL,
  contact_phone     VARCHAR(20)  NOT NULL,
  pickup_address    VARCHAR(255) NOT NULL,
  pickup_zone_id    UUID         NOT NULL,               -- ref zones
  status            VARCHAR(12)  NOT NULL DEFAULT 'PENDING'
                    CHECK (status IN ('PENDING','APPROVED','REJECTED','SUSPENDED')),
  approved_by       UUID,
  approved_at       TIMESTAMPTZ,
  rejection_reason  VARCHAR(255),
  suspension_reason VARCHAR(255),
  created_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: suspending needs a reason (API 8.8)
  CONSTRAINT merchants_suspended_reason CHECK (status <> 'SUSPENDED' OR suspension_reason IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS merchants_status_created ON merchant.merchants (status, created_at);


CREATE TABLE IF NOT EXISTS merchant.merchant_api_clients (
  id           UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id  UUID         NOT NULL REFERENCES merchant.merchants(id),
  name         VARCHAR(100) NOT NULL,
  key_prefix   VARCHAR(20)  NOT NULL,                    -- display only, not unique
  lookup_id    VARCHAR(16)  NOT NULL UNIQUE,             -- random segment used to find the row
  key_hash     CHAR(64)     NOT NULL,                    -- SHA-256 of the full key
  status       VARCHAR(10)  NOT NULL DEFAULT 'ACTIVE' CHECK (status IN ('ACTIVE','REVOKED')),
  last_used_at TIMESTAMPTZ,
  created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
  revoked_at   TIMESTAMPTZ
);
CREATE INDEX IF NOT EXISTS api_clients_merchant_status ON merchant.merchant_api_clients (merchant_id, status);


-- ADDED: at most 5 ACTIVE keys per merchant (service maps this to 409 KEY_LIMIT_REACHED)
CREATE OR REPLACE FUNCTION merchant.enforce_active_key_limit() RETURNS trigger AS $$
BEGIN
  IF NEW.status = 'ACTIVE' THEN
    PERFORM 1 FROM merchant.merchants WHERE id = NEW.merchant_id FOR UPDATE;   -- serialises concurrent requests
    IF (SELECT count(*) FROM merchant.merchant_api_clients
         WHERE merchant_id = NEW.merchant_id AND status = 'ACTIVE' AND id <> NEW.id) >= 5 THEN
      RAISE EXCEPTION 'KEY_LIMIT_REACHED' USING ERRCODE = '23514';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;


DROP TRIGGER IF EXISTS api_clients_key_limit ON merchant.merchant_api_clients;
CREATE TRIGGER api_clients_key_limit
  BEFORE INSERT OR UPDATE OF status ON merchant.merchant_api_clients
  FOR EACH ROW EXECUTE FUNCTION merchant.enforce_active_key_limit();


CREATE TABLE IF NOT EXISTS merchant.merchant_webhooks (
  id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id        UUID         NOT NULL UNIQUE REFERENCES merchant.merchants(id),   -- one webhook per merchant
  url                VARCHAR(500) NOT NULL,
  signing_secret_enc BYTEA        NOT NULL,              -- encrypted HMAC secret
  event_types        TEXT[]       NOT NULL,
  is_active          BOOLEAN      NOT NULL DEFAULT true,
  created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: HTTPS only (private and loopback targets are blocked in the service)
  CONSTRAINT webhooks_https_only CHECK (url ~* '^https://')
);


CREATE TABLE IF NOT EXISTS merchant.merchant_orders (
  id                UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id       UUID          NOT NULL REFERENCES merchant.merchants(id),
  external_order_id VARCHAR(100)  NOT NULL,
  idempotency_key   VARCHAR(100),
  request_hash      CHAR(64)      NOT NULL,              -- SHA-256 of the normalised payload
  parcel_id         UUID          NOT NULL UNIQUE,       -- ref parcels
  tracking_number   VARCHAR(20)   NOT NULL UNIQUE,
  recipient_name    VARCHAR(150)  NOT NULL,
  dropoff_city      VARCHAR(100)  NOT NULL,
  weight_kg         NUMERIC(6,2)  NOT NULL,
  size_category     VARCHAR(12)   NOT NULL CHECK (size_category IN ('SMALL','MEDIUM','LARGE','OVERSIZED')),
  description       VARCHAR(255),
  fee_amount        NUMERIC(12,2) NOT NULL,
  fee_breakdown     JSONB         NOT NULL,
  currency          CHAR(3)       NOT NULL DEFAULT 'LKR',
  last_known_status VARCHAR(20)   NOT NULL
                    CHECK (last_known_status IN ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  last_status_at    TIMESTAMPTZ   NOT NULL,              -- occurredAt of the last applied event
  created_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at        TIMESTAMPTZ   NOT NULL DEFAULT now(),
  CONSTRAINT merchant_orders_external_uq UNIQUE (merchant_id, external_order_id)
);
CREATE UNIQUE INDEX IF NOT EXISTS merchant_orders_idem_uq
  ON merchant.merchant_orders (merchant_id, idempotency_key) WHERE idempotency_key IS NOT NULL;
CREATE INDEX IF NOT EXISTS merchant_orders_created_idx ON merchant.merchant_orders (merchant_id, created_at);
CREATE INDEX IF NOT EXISTS merchant_orders_status_idx  ON merchant.merchant_orders (merchant_id, last_known_status);


CREATE TABLE IF NOT EXISTS merchant.webhook_delivery_attempts (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id        UUID         NOT NULL,
  webhook_id      UUID         NOT NULL REFERENCES merchant.merchant_webhooks(id),
  merchant_id     UUID         NOT NULL REFERENCES merchant.merchants(id),
  parcel_id       UUID         NOT NULL,                 -- ref parcels
  tracking_number VARCHAR(20)  NOT NULL,
  attempt_number  INT          NOT NULL CHECK (attempt_number BETWEEN 1 AND 6),
  http_status     INT,                                   -- NULL on timeout
  success         BOOLEAN      NOT NULL,
  error_message   VARCHAR(255),
  next_retry_at   TIMESTAMPTZ,
  dead_lettered   BOOLEAN      NOT NULL DEFAULT false,
  duration_ms     INT,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT webhook_attempt_uq UNIQUE (webhook_id, event_id, attempt_number)
);
CREATE INDEX IF NOT EXISTS wda_event_webhook_idx  ON merchant.webhook_delivery_attempts (event_id, webhook_id);
CREATE INDEX IF NOT EXISTS wda_merchant_created   ON merchant.webhook_delivery_attempts (merchant_id, created_at);
CREATE INDEX IF NOT EXISTS wda_merchant_tracking  ON merchant.webhook_delivery_attempts (merchant_id, tracking_number, created_at);
CREATE INDEX IF NOT EXISTS wda_merchant_success   ON merchant.webhook_delivery_attempts (merchant_id, success, created_at);


CREATE TABLE IF NOT EXISTS merchant.merchant_order_status_events (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  order_id        UUID         NOT NULL REFERENCES merchant.merchant_orders(id),
  event_id        UUID         NOT NULL UNIQUE,          -- RabbitMQ eventId, duplicates ignored
  previous_status VARCHAR(20),
  status          VARCHAR(20)  NOT NULL
                  CHECK (status IN ('CREATED','PICKED_UP','IN_TRANSIT','OUT_FOR_DELIVERY','DELIVERED','FAILED')),
  occurred_at     TIMESTAMPTZ  NOT NULL,
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS mose_order_occurred_idx ON merchant.merchant_order_status_events (order_id, occurred_at);


CREATE TABLE IF NOT EXISTS merchant.audit_logs (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID        NOT NULL,
  action        VARCHAR(60) NOT NULL,
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     UUID,
  details       JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS merchant_audit_entity_idx  ON merchant.audit_logs (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS merchant_audit_created_idx ON merchant.audit_logs (created_at);


DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'merchant.merchants','merchant.merchant_webhooks','merchant.merchant_orders'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %s', t);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', t);
  END LOOP;
END $$;
