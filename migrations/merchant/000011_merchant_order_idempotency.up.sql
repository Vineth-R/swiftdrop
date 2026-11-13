-- Durable idempotency reservation for merchant order creation.
--
-- The merchant_orders table cannot hold a partial row before Parcel has
-- produced a parcel ID and tracking number. This table records the request
-- hash for an (merchant, Idempotency-Key) pair before the external Parcel
-- call, so a concurrent or retried request with a different payload is
-- rejected without creating another parcel. The row is completed in the same
-- local transaction as the merchant order and its initial timeline entry.

CREATE TABLE IF NOT EXISTS merchant.merchant_order_reservations (
  id              UUID         PRIMARY KEY DEFAULT gen_random_uuid(),
  merchant_id     UUID         NOT NULL REFERENCES merchant.merchants(id),
  idempotency_key VARCHAR(100) NOT NULL,
  request_hash    CHAR(64)     NOT NULL,
  status          VARCHAR(16)  NOT NULL DEFAULT 'PENDING'
                  CHECK (status IN ('PENDING','COMPLETED','FAILED')),
  order_id        UUID         REFERENCES merchant.merchant_orders(id),
  last_error      VARCHAR(255),
  created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
  CONSTRAINT merchant_order_reservations_key_uq UNIQUE (merchant_id, idempotency_key),
  CONSTRAINT merchant_order_reservations_key_not_blank CHECK (btrim(idempotency_key) <> ''),
  CONSTRAINT merchant_order_reservations_hash_format CHECK (request_hash ~ '^[0-9a-f]{64}$'),
  CONSTRAINT merchant_order_reservations_completed_order
    CHECK (status <> 'COMPLETED' OR order_id IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS merchant_order_reservations_status_idx
  ON merchant.merchant_order_reservations (status, updated_at);

DO $$
BEGIN
  DROP TRIGGER IF EXISTS set_updated_at ON merchant.merchant_order_reservations;
  CREATE TRIGGER set_updated_at BEFORE UPDATE ON merchant.merchant_order_reservations
    FOR EACH ROW EXECUTE FUNCTION public.set_updated_at();
END $$;
