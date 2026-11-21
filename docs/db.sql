-- =====================================================================
-- SwiftDrop database (PostgreSQL 16)
-- Built from: SwiftDrop Database Design Document v1.1 (aligned with API v1.1)
-- Run:  psql "$DATABASE_URL" -v ON_ERROR_STOP=1 -f db.sql
-- Safe to run twice (IF NOT EXISTS, ON CONFLICT DO NOTHING).
--
-- Layout: five schemas, 26 tables, no cross schema foreign keys.
--   identity     roles, users, courier_profiles, refresh_tokens, oauth_states,
--                password_reset_tokens, audit_logs                      (7)
--   pricing      zones, pricing_rules, weight_bands, size_surcharges,
--                audit_logs                                             (5)
--   parcel       parcels, parcel_status_history, parcel_assignments,
--                delivery_proofs, outbox_events                         (5)
--   merchant     merchants, merchant_api_clients, merchant_webhooks,
--                merchant_orders, webhook_delivery_attempts,
--                merchant_order_status_events, audit_logs               (7)
--   notification notification_logs, processed_events                    (2)
--
-- Additive migrations above this baseline add four more tables:
--   identity 000004 email_verification_tokens
--   identity 000006 merchant_lifecycle_transitions
--   merchant 000007 merchant_registration_attempts, merchant_status_transitions
-- merchant 000008 drops the database-level business-registration constraint and
-- adds no tables; merchant 000009 restores the mandatory constraint after the
-- compatibility window. The complete migrated chain is 31 tables. See
-- migrations/README.md.
--
-- ADDED BEYOND THE DOCUMENT (marked "ADDED" below):
--   1  DEFAULT gen_random_uuid() on UUID keys (document says app generated)
--   2  set_updated_at() trigger on every table that has updated_at
--   3  Parcel status transition trigger (document section 9, now enforced)
--   4  parcel_status_history is append only (trigger)
--   5  Tracking number prefix must match owner (SDK customer, SDN merchant)
--   6  DELIVERED needs delivered_to_name, FAILED needs failed_at and a
--      reason of at least 5 characters
--   7  price_amount must equal the sum of price_breakdown
--   8  Trigger limiting a merchant to 5 ACTIVE API keys (KEY_LIMIT_REACHED)
--   9  Only one unused password reset token per user
--   10 oauth_states.redirect_to must be a relative path
--   11 Webhook URL must start with https://
--   12 parcel_assignments: closed rows need unassigned_at
--   13 outbox_events: event_type and routing_key must be a valid pair
--   14 merchants: SUSPENDED needs suspension_reason
--   15 users: must have a password or a Google subject
--   16 Seed values the document left open (zone codes, same zone fees)
-- =====================================================================


BEGIN;


CREATE SCHEMA IF NOT EXISTS identity;
CREATE SCHEMA IF NOT EXISTS pricing;
CREATE SCHEMA IF NOT EXISTS parcel;
CREATE SCHEMA IF NOT EXISTS merchant;
CREATE SCHEMA IF NOT EXISTS notification;


-- ADDED: shared helper that keeps updated_at current
CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $$
BEGIN
  NEW.updated_at := now();
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;


-- =====================================================================
-- 1. IDENTITY SCHEMA  (Identity and User service)
-- =====================================================================


CREATE TABLE IF NOT EXISTS identity.roles (
  id          SMALLINT     PRIMARY KEY,
  name        VARCHAR(30)  NOT NULL UNIQUE
              CHECK (name IN ('CUSTOMER','MERCHANT','COURIER','DISPATCHER','ADMIN')),
  description VARCHAR(255)
);


CREATE TABLE IF NOT EXISTS identity.users (
  id                 UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  email              VARCHAR(255) NOT NULL,
  password_hash      VARCHAR(100),                       -- bcrypt, NULL for Google only accounts
  full_name          VARCHAR(150) NOT NULL CHECK (char_length(full_name) BETWEEN 2 AND 150),
  phone              VARCHAR(20),
  role_id            SMALLINT     NOT NULL REFERENCES identity.roles(id),
  auth_provider      VARCHAR(10)  NOT NULL DEFAULT 'LOCAL' CHECK (auth_provider IN ('LOCAL','GOOGLE')),
  google_sub         VARCHAR(100) UNIQUE,
  email_verified     BOOLEAN      NOT NULL DEFAULT false,
  is_active          BOOLEAN      NOT NULL DEFAULT true,
  failed_login_count INT          NOT NULL DEFAULT 0 CHECK (failed_login_count >= 0),
  locked_until       TIMESTAMPTZ,
  last_login_at      TIMESTAMPTZ,
  created_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at         TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: every account needs some way to sign in
  CONSTRAINT users_has_login_method CHECK (password_hash IS NOT NULL OR google_sub IS NOT NULL)
);
CREATE UNIQUE INDEX IF NOT EXISTS users_email_lower_uq ON identity.users (lower(email));
CREATE INDEX IF NOT EXISTS users_role_idx        ON identity.users (role_id);
CREATE INDEX IF NOT EXISTS users_role_active_idx ON identity.users (role_id, is_active);


CREATE TABLE IF NOT EXISTS identity.courier_profiles (
  user_id        UUID        PRIMARY KEY REFERENCES identity.users(id),
  vehicle_type   VARCHAR(30),
  vehicle_number VARCHAR(20),
  home_zone_id   UUID,                                   -- ref pricing.zones (no FK)
  is_available   BOOLEAN     NOT NULL DEFAULT true,
  created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS identity.refresh_tokens (
  id          UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id     UUID         NOT NULL REFERENCES identity.users(id),
  token_hash  CHAR(64)     NOT NULL UNIQUE,              -- SHA-256 hex, raw token never stored
  expires_at  TIMESTAMPTZ  NOT NULL,
  revoked_at  TIMESTAMPTZ,
  replaced_by UUID,
  user_agent  VARCHAR(255),
  ip_address  INET,
  created_at  TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS refresh_tokens_user_idx    ON identity.refresh_tokens (user_id);
CREATE INDEX IF NOT EXISTS refresh_tokens_expires_idx ON identity.refresh_tokens (expires_at);


CREATE TABLE IF NOT EXISTS identity.oauth_states (
  state                VARCHAR(64)  PRIMARY KEY,
  code_verifier        VARCHAR(128),
  redirect_to          VARCHAR(255),
  browser_binding_hash CHAR(64),                         -- SHA-256 of the browser-binding cookie; NULL on pre-000003 rows
  expires_at           TIMESTAMPTZ  NOT NULL,
  created_at           TIMESTAMPTZ  NOT NULL DEFAULT now(),
  -- ADDED: only relative paths, so the login cannot redirect to another site
  CONSTRAINT oauth_redirect_relative CHECK (redirect_to IS NULL OR (redirect_to LIKE '/%' AND redirect_to NOT LIKE '//%'))
);
CREATE INDEX IF NOT EXISTS oauth_states_expires_idx ON identity.oauth_states (expires_at);


CREATE TABLE IF NOT EXISTS identity.password_reset_tokens (
  id           UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  user_id      UUID        NOT NULL REFERENCES identity.users(id),
  token_hash   CHAR(64)    NOT NULL UNIQUE,              -- SHA-256 of the emailed token
  expires_at   TIMESTAMPTZ NOT NULL,                     -- 30 minutes after creation
  used_at      TIMESTAMPTZ,
  requested_ip INET,
  created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS reset_tokens_user_idx    ON identity.password_reset_tokens (user_id);
CREATE INDEX IF NOT EXISTS reset_tokens_expires_idx ON identity.password_reset_tokens (expires_at);
-- ADDED: only the newest link works. Mark older unused tokens as used BEFORE inserting a new one.
CREATE UNIQUE INDEX IF NOT EXISTS reset_tokens_one_unused_uq
  ON identity.password_reset_tokens (user_id) WHERE used_at IS NULL;


CREATE TABLE IF NOT EXISTS identity.audit_logs (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID        NOT NULL,                    -- ref users
  action        VARCHAR(60) NOT NULL,
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     UUID,
  details       JSONB,                                   -- never secrets, tokens or passwords
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS identity_audit_entity_idx  ON identity.audit_logs (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS identity_audit_created_idx ON identity.audit_logs (created_at);


-- =====================================================================
-- 2. PRICING SCHEMA  (Pricing and Zones service)
-- =====================================================================


CREATE TABLE IF NOT EXISTS pricing.zones (
  id         UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  code       VARCHAR(20)  NOT NULL UNIQUE,               -- 409 ZONE_CODE_EXISTS
  name       VARCHAR(100) NOT NULL,
  districts  TEXT[],
  is_active  BOOLEAN      NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ  NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS pricing.pricing_rules (
  id                  UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  origin_zone_id      UUID          NOT NULL REFERENCES pricing.zones(id),
  destination_zone_id UUID          NOT NULL REFERENCES pricing.zones(id),
  base_fee            NUMERIC(12,2) NOT NULL CHECK (base_fee >= 0),
  currency            CHAR(3)       NOT NULL DEFAULT 'LKR',
  is_active           BOOLEAN       NOT NULL DEFAULT true,
  updated_by          UUID,                              -- admin user (ref users)
  created_at          TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at          TIMESTAMPTZ   NOT NULL DEFAULT now()
);
-- one active rule per zone pair (409 RULE_EXISTS)
CREATE UNIQUE INDEX IF NOT EXISTS pricing_rules_active_pair_uq
  ON pricing.pricing_rules (origin_zone_id, destination_zone_id) WHERE is_active;


CREATE TABLE IF NOT EXISTS pricing.weight_bands (
  id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  min_weight_kg NUMERIC(6,2)  NOT NULL CHECK (min_weight_kg >= 0),   -- exclusive
  max_weight_kg NUMERIC(6,2)  NOT NULL,                              -- inclusive
  charge        NUMERIC(12,2) NOT NULL CHECK (charge >= 0),
  is_active     BOOLEAN       NOT NULL DEFAULT true,
  created_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
  updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now(),
  CONSTRAINT weight_bands_range CHECK (max_weight_kg > min_weight_kg),
  -- active bands must not overlap (409 BAND_OVERLAP)
  CONSTRAINT weight_bands_no_overlap
    EXCLUDE USING gist (numrange(min_weight_kg, max_weight_kg, '(]') WITH &&) WHERE (is_active)
);


CREATE TABLE IF NOT EXISTS pricing.size_surcharges (
  id            UUID          PRIMARY KEY DEFAULT gen_random_uuid(),
  size_category VARCHAR(12)   NOT NULL UNIQUE
                CHECK (size_category IN ('SMALL','MEDIUM','LARGE','OVERSIZED')),
  surcharge     NUMERIC(12,2) NOT NULL CHECK (surcharge >= 0),
  updated_at    TIMESTAMPTZ   NOT NULL DEFAULT now()
);


CREATE TABLE IF NOT EXISTS pricing.audit_logs (
  id            UUID        PRIMARY KEY DEFAULT gen_random_uuid(),
  actor_user_id UUID        NOT NULL,
  action        VARCHAR(60) NOT NULL,
  entity_type   VARCHAR(40) NOT NULL,
  entity_id     UUID,
  details       JSONB,
  created_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS pricing_audit_entity_idx  ON pricing.audit_logs (entity_type, entity_id);
CREATE INDEX IF NOT EXISTS pricing_audit_created_idx ON pricing.audit_logs (created_at);


-- =====================================================================
-- 3. PARCEL SCHEMA  (Parcel Operations service)
-- =====================================================================


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


-- =====================================================================
-- 4. MERCHANT SCHEMA  (Merchant Integration service)
-- =====================================================================


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
  -- M8 snapshots: the pickup and recipient details sent to Parcel, so a later
  -- merchant profile or pricing change cannot rewrite the order history.
  pickup_name        VARCHAR(150),
  pickup_phone       VARCHAR(20),
  pickup_address     VARCHAR(255),
  pickup_city        VARCHAR(100),
  pickup_zone_id     UUID,
  recipient_name    VARCHAR(150)  NOT NULL,
  recipient_phone    VARCHAR(20),
  recipient_address  VARCHAR(255),
  recipient_zone_id  UUID,
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
CREATE INDEX IF NOT EXISTS merchant_orders_updated_idx ON merchant.merchant_orders (merchant_id, updated_at);
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


-- =====================================================================
-- 5. NOTIFICATION SCHEMA  (Notification Worker)
-- Password reset emails are sent by Identity and are never logged here.
-- =====================================================================


CREATE TABLE IF NOT EXISTS notification.notification_logs (
  id            UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id      UUID         NOT NULL,
  parcel_id     UUID         NOT NULL,
  channel       VARCHAR(10)  NOT NULL CHECK (channel IN ('EMAIL','SMS')),
  recipient     VARCHAR(255) NOT NULL,                   -- mask in logs
  subject       VARCHAR(200),
  status        VARCHAR(10)  NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENT','FAILED')),
  error_message VARCHAR(255),
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
  sent_at       TIMESTAMPTZ,
  CONSTRAINT notification_dedupe_uq UNIQUE (event_id, channel, recipient)
);


CREATE TABLE IF NOT EXISTS notification.processed_events (
  consumer     VARCHAR(40) NOT NULL,
  event_id     UUID        NOT NULL,
  processed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  PRIMARY KEY (consumer, event_id)
);


-- =====================================================================
-- ADDED: updated_at triggers on every table that has the column
-- =====================================================================
DO $$
DECLARE t text;
BEGIN
  FOREACH t IN ARRAY ARRAY[
    'identity.users','identity.courier_profiles',
    'pricing.zones','pricing.pricing_rules','pricing.weight_bands','pricing.size_surcharges',
    'parcel.parcels',
    'merchant.merchants','merchant.merchant_webhooks','merchant.merchant_orders'
  ] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON %s', t);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON %s FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', t);
  END LOOP;
END $$;


-- =====================================================================
-- SEED DATA (document section 10)
-- Demo users are NOT seeded here: passwords come from environment variables
-- and are hashed by the Go seed command (`make seed`: admin, dispatcher, 2 couriers, customer).
-- =====================================================================


INSERT INTO identity.roles (id, name, description) VALUES
  (1, 'CUSTOMER',   'Sends and tracks parcels'),
  (2, 'MERCHANT',   'Online store that creates orders through the API'),
  (3, 'COURIER',    'Collects and delivers assigned parcels'),
  (4, 'DISPATCHER', 'Assigns couriers and monitors workload'),
  (5, 'ADMIN',      'Manages users, merchants, zones, pricing and reports')
ON CONFLICT DO NOTHING;


-- Zone codes CMB, KDY, GLE come from the document; GMP and JFN are chosen here.
INSERT INTO pricing.zones (code, name, districts) VALUES
  ('CMB', 'Colombo', ARRAY['Colombo']),
  ('GMP', 'Gampaha', ARRAY['Gampaha','Negombo']),
  ('KDY', 'Kandy',   ARRAY['Kandy']),
  ('GLE', 'Galle',   ARRAY['Galle']),
  ('JFN', 'Jaffna',  ARRAY['Jaffna'])
ON CONFLICT DO NOTHING;


-- Illustrative LKR base fees, same fee both directions. CMB to KDY = 450.00 as in the API example.
WITH fees(a, b, fee) AS (VALUES
  ('CMB','CMB',250.00), ('GMP','GMP',250.00), ('KDY','KDY',250.00), ('GLE','GLE',250.00), ('JFN','JFN',250.00),
  ('CMB','GMP',300.00), ('CMB','KDY',450.00), ('CMB','GLE',450.00), ('CMB','JFN',750.00),
  ('GMP','KDY',450.00), ('GMP','GLE',500.00), ('GMP','JFN',700.00),
  ('KDY','GLE',600.00), ('KDY','JFN',650.00),
  ('GLE','JFN',850.00)),
pairs AS (
  SELECT a, b, fee FROM fees
  UNION
  SELECT b, a, fee FROM fees)
INSERT INTO pricing.pricing_rules (origin_zone_id, destination_zone_id, base_fee)
SELECT zo.id, zd.id, p.fee
FROM pairs p
JOIN pricing.zones zo ON zo.code = p.a
JOIN pricing.zones zd ON zd.code = p.b
ON CONFLICT DO NOTHING;


-- Lower bound exclusive, upper bound inclusive. The 10 to 25 kg charge is illustrative.
INSERT INTO pricing.weight_bands (min_weight_kg, max_weight_kg, charge) VALUES
  (0,  1,  0.00),
  (1,  5,  100.00),
  (5,  10, 250.00),
  (10, 25, 500.00)
ON CONFLICT DO NOTHING;


INSERT INTO pricing.size_surcharges (size_category, surcharge) VALUES
  ('SMALL', 0.00), ('MEDIUM', 150.00), ('LARGE', 400.00), ('OVERSIZED', 900.00)
ON CONFLICT DO NOTHING;


COMMIT;


-- =====================================================================
-- CHECKS to run after loading (expected: 26 tables, fee 550.00)
-- =====================================================================
-- SELECT count(*) FROM information_schema.tables
--  WHERE table_schema IN ('identity','pricing','parcel','merchant','notification');
--
-- SELECT r.base_fee + w.charge + s.surcharge AS fee      -- 2 kg SMALL, Colombo to Kandy
--   FROM pricing.pricing_rules r
--   JOIN pricing.zones zo ON zo.id = r.origin_zone_id      AND zo.code = 'CMB'
--   JOIN pricing.zones zd ON zd.id = r.destination_zone_id AND zd.code = 'KDY'
--   JOIN pricing.weight_bands w ON 2 > w.min_weight_kg AND 2 <= w.max_weight_kg AND w.is_active
--   JOIN pricing.size_surcharges s ON s.size_category = 'SMALL'
--  WHERE r.is_active;


-- =====================================================================
-- OPTIONAL: one database login per service, so each service can only write
-- its own schema (matches "each service is the only writer of its tables").
-- Uncomment, change the passwords, and run once as a superuser.
-- =====================================================================
-- CREATE ROLE identity_svc     LOGIN PASSWORD 'CHANGE_ME';
-- CREATE ROLE pricing_svc      LOGIN PASSWORD 'CHANGE_ME';
-- CREATE ROLE parcel_svc       LOGIN PASSWORD 'CHANGE_ME';
-- CREATE ROLE merchant_svc     LOGIN PASSWORD 'CHANGE_ME';
-- CREATE ROLE notification_svc LOGIN PASSWORD 'CHANGE_ME';
-- GRANT USAGE ON SCHEMA identity     TO identity_svc;
-- GRANT USAGE ON SCHEMA pricing      TO pricing_svc;
-- GRANT USAGE ON SCHEMA parcel       TO parcel_svc;
-- GRANT USAGE ON SCHEMA merchant     TO merchant_svc;
-- GRANT USAGE ON SCHEMA notification TO notification_svc;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA identity     TO identity_svc;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA pricing      TO pricing_svc;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA parcel       TO parcel_svc;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA merchant     TO merchant_svc;
-- GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA notification TO notification_svc;
