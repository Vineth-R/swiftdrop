-- M8 merchant order snapshots.
--
-- The baseline merchant_orders table keeps the list/detail summary fields only.
-- The merchant order API also needs the pickup and recipient contact snapshot
-- that was sent to Parcel so a later merchant profile or pricing change cannot
-- rewrite the order history. These columns are nullable so the migration is
-- forward-compatible with any pre-M8 rows; every order written by the M8 API
-- populates them.
--
-- This forward migration follows the existing merchant business-registration
-- chain (000008 compatibility and 000009 restored constraint) on development.

ALTER TABLE merchant.merchant_orders
  ADD COLUMN IF NOT EXISTS pickup_name      VARCHAR(150),
  ADD COLUMN IF NOT EXISTS pickup_phone     VARCHAR(20),
  ADD COLUMN IF NOT EXISTS pickup_address   VARCHAR(255),
  ADD COLUMN IF NOT EXISTS pickup_city      VARCHAR(100),
  ADD COLUMN IF NOT EXISTS pickup_zone_id   UUID,
  ADD COLUMN IF NOT EXISTS recipient_phone  VARCHAR(20),
  ADD COLUMN IF NOT EXISTS recipient_address VARCHAR(255),
  ADD COLUMN IF NOT EXISTS recipient_zone_id UUID;

-- The merchant order list supports createdAt and updatedAt sorting. created_at
-- is already indexed with the merchant scope; this adds the updatedAt variant.
CREATE INDEX IF NOT EXISTS merchant_orders_updated_idx
  ON merchant.merchant_orders (merchant_id, updated_at);
