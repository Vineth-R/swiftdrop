-- Forward-only convergence of the original baseline and the legacy M8 chain.
-- Existing rows and the stronger legacy constraints are deliberately retained.
CREATE TABLE IF NOT EXISTS merchant.merchant_webhooks (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id UUID NOT NULL UNIQUE REFERENCES merchant.merchants(id),
  url VARCHAR(500) NOT NULL,
  signing_secret_enc BYTEA NOT NULL,
  event_types TEXT[] NOT NULL,
  is_active BOOLEAN NOT NULL DEFAULT true,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT webhooks_https_only CHECK (url ~* '^https://')
);
CREATE TABLE IF NOT EXISTS merchant.webhook_delivery_attempts (
  id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
  event_id UUID NOT NULL,
  webhook_id UUID NOT NULL REFERENCES merchant.merchant_webhooks(id),
  merchant_id UUID NOT NULL REFERENCES merchant.merchants(id),
  parcel_id UUID NOT NULL,
  tracking_number VARCHAR(20) NOT NULL,
  attempt_number INT NOT NULL CHECK (attempt_number BETWEEN 1 AND 6),
  http_status INT,
  success BOOLEAN NOT NULL,
  error_message VARCHAR(255),
  next_retry_at TIMESTAMPTZ,
  dead_lettered BOOLEAN NOT NULL DEFAULT false,
  duration_ms INT,
  created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
  CONSTRAINT webhook_attempt_uq UNIQUE (webhook_id, event_id, attempt_number)
);
CREATE INDEX IF NOT EXISTS wda_event_webhook_idx ON merchant.webhook_delivery_attempts(event_id, webhook_id);
CREATE INDEX IF NOT EXISTS wda_merchant_created ON merchant.webhook_delivery_attempts(merchant_id, created_at);
CREATE INDEX IF NOT EXISTS wda_merchant_tracking ON merchant.webhook_delivery_attempts(merchant_id, tracking_number, created_at);
CREATE INDEX IF NOT EXISTS wda_merchant_success ON merchant.webhook_delivery_attempts(merchant_id, success, created_at);

ALTER TABLE merchant.merchants ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE merchant.merchant_api_clients ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE merchant.merchant_orders ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE merchant.merchant_order_status_events ALTER COLUMN id SET DEFAULT gen_random_uuid();
ALTER TABLE merchant.audit_logs ALTER COLUMN id SET DEFAULT gen_random_uuid();

CREATE OR REPLACE FUNCTION merchant.enforce_active_key_limit() RETURNS trigger AS $$
BEGIN
  IF NEW.status = 'ACTIVE' THEN
    PERFORM 1 FROM merchant.merchants WHERE id = NEW.merchant_id FOR UPDATE;
    IF (SELECT count(*) FROM merchant.merchant_api_clients
        WHERE merchant_id = NEW.merchant_id AND status = 'ACTIVE' AND id <> NEW.id) >= 5 THEN
      RAISE EXCEPTION 'KEY_LIMIT_REACHED' USING ERRCODE = '23514';
    END IF;
  END IF;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
DROP TRIGGER IF EXISTS api_clients_key_limit ON merchant.merchant_api_clients;
CREATE TRIGGER api_clients_key_limit BEFORE INSERT OR UPDATE OF status
  ON merchant.merchant_api_clients FOR EACH ROW EXECUTE FUNCTION merchant.enforce_active_key_limit();

DO $$
DECLARE table_name text;
BEGIN
  FOREACH table_name IN ARRAY ARRAY['merchants', 'merchant_orders', 'merchant_webhooks'] LOOP
    EXECUTE format('DROP TRIGGER IF EXISTS set_updated_at ON merchant.%I', table_name);
    EXECUTE format('CREATE TRIGGER set_updated_at BEFORE UPDATE ON merchant.%I FOR EACH ROW EXECUTE FUNCTION public.set_updated_at()', table_name);
  END LOOP;
END $$;
