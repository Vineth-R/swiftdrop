DROP TRIGGER IF EXISTS set_updated_at ON merchant.merchant_registration_attempts;
DROP TABLE IF EXISTS merchant.merchant_status_transitions;
DROP TABLE IF EXISTS merchant.merchant_registration_attempts;
ALTER TABLE merchant.merchants DROP COLUMN IF EXISTS lifecycle_sequence;
DROP INDEX IF EXISTS merchants_business_name_lower_idx;
DROP INDEX IF EXISTS merchants_business_reg_no_uq;
ALTER TABLE merchant.merchants DROP CONSTRAINT IF EXISTS merchants_business_reg_no_present;
ALTER TABLE merchant.merchants
  ALTER COLUMN rejection_reason TYPE VARCHAR(255)
  USING left(rejection_reason, 255);
ALTER TABLE merchant.merchants
  ADD CONSTRAINT merchants_rejected_reason
  CHECK (status <> 'REJECTED' OR rejection_reason IS NOT NULL) NOT VALID;
ALTER TABLE merchant.merchants DROP COLUMN IF EXISTS business_address;
