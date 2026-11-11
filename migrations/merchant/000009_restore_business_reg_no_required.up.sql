-- Restore the mandatory business registration number invariant after
-- 000008_business_reg_no_optional. The application validates the format; the
-- database must reject NULL, empty and whitespace-only values.
--
-- The constraint is added NOT VALID and then validated so existing rows are
-- checked without holding a long validation lock during the ADD. If any
-- existing row is invalid, VALIDATE fails and the migration must be corrected
-- through an authorized data fix; do not invent registration numbers.
ALTER TABLE merchant.merchants
  DROP CONSTRAINT IF EXISTS merchants_business_reg_no_present;

ALTER TABLE merchant.merchants
  ADD CONSTRAINT merchants_business_reg_no_present
  CHECK (business_reg_no IS NOT NULL AND business_reg_no ~ '[^[:space:]]') NOT VALID;

ALTER TABLE merchant.merchants
  VALIDATE CONSTRAINT merchants_business_reg_no_present;
