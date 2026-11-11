DROP INDEX IF EXISTS merchant.merchant_orders_updated_idx;

ALTER TABLE merchant.merchant_orders
  DROP COLUMN IF EXISTS recipient_zone_id,
  DROP COLUMN IF EXISTS recipient_address,
  DROP COLUMN IF EXISTS recipient_phone,
  DROP COLUMN IF EXISTS pickup_zone_id,
  DROP COLUMN IF EXISTS pickup_city,
  DROP COLUMN IF EXISTS pickup_address,
  DROP COLUMN IF EXISTS pickup_phone,
  DROP COLUMN IF EXISTS pickup_name;
