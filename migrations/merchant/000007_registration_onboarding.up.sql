-- Merchant registration, optional rejection reasons and lifecycle-bound session invalidation.

ALTER TABLE merchant.merchants
  ADD COLUMN IF NOT EXISTS business_address VARCHAR(255);

ALTER TABLE merchant.merchants
  ALTER COLUMN rejection_reason TYPE VARCHAR(1000);

ALTER TABLE merchant.merchants
  DROP CONSTRAINT IF EXISTS merchants_rejected_reason;

ALTER TABLE merchant.merchants
  ADD CONSTRAINT merchants_business_reg_no_present
  CHECK (business_reg_no IS NOT NULL AND btrim(business_reg_no) <> '') NOT VALID;

CREATE UNIQUE INDEX IF NOT EXISTS merchants_business_reg_no_uq
  ON merchant.merchants (lower(business_reg_no))
  WHERE business_reg_no IS NOT NULL;

CREATE INDEX IF NOT EXISTS merchants_business_name_lower_idx
  ON merchant.merchants (lower(business_name) text_pattern_ops);

ALTER TABLE merchant.merchants
  ADD COLUMN IF NOT EXISTS lifecycle_sequence BIGINT NOT NULL DEFAULT 0 CHECK (lifecycle_sequence >= 0);

CREATE TABLE IF NOT EXISTS merchant.merchant_registration_attempts (
  id              UUID         PRIMARY KEY,
  idempotency_key VARCHAR(128) NOT NULL UNIQUE,
  email           VARCHAR(255) NOT NULL,
  status          VARCHAR(16)  NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','COMPLETED','FAILED')),
  user_id         UUID,
  merchant_id     UUID REFERENCES merchant.merchants(id),
  last_error      VARCHAR(255),
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX IF NOT EXISTS merchant_registration_active_email_uq
  ON merchant.merchant_registration_attempts (lower(email))
  WHERE status IN ('PENDING','COMPLETED');
CREATE INDEX IF NOT EXISTS merchant_registration_email_idx
  ON merchant.merchant_registration_attempts (lower(email));

CREATE TABLE IF NOT EXISTS merchant.merchant_status_transitions (
  id            UUID         PRIMARY KEY,
  merchant_id   UUID         NOT NULL REFERENCES merchant.merchants(id),
  sequence      BIGINT       NOT NULL CHECK (sequence > 0),
  from_status   VARCHAR(12)  NOT NULL,
  to_status     VARCHAR(12)  NOT NULL,
  actor_user_id UUID         NOT NULL,
  reason        VARCHAR(1000),
  sync_status   VARCHAR(16)  NOT NULL DEFAULT 'PENDING'
                CHECK (sync_status IN ('PENDING','APPLIED','FAILED')),
  attempts      INT          NOT NULL DEFAULT 0 CHECK (attempts >= 0),
  last_error    VARCHAR(255),
  created_at    TIMESTAMPTZ  NOT NULL DEFAULT now(),
  applied_at    TIMESTAMPTZ,
  CONSTRAINT merchant_status_transition_sequence_uq UNIQUE (merchant_id, sequence)
);
CREATE INDEX IF NOT EXISTS merchant_status_transitions_merchant_idx
  ON merchant.merchant_status_transitions (merchant_id, created_at);

DO $$
BEGIN
  DROP TRIGGER IF EXISTS set_updated_at ON merchant.merchant_registration_attempts;
  CREATE TRIGGER set_updated_at BEFORE UPDATE ON merchant.merchant_registration_attempts
    FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();
END $$;
