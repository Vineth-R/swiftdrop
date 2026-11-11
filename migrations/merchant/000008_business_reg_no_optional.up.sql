-- The recovered 000007 registration onboarding migration requires a business
-- registration number for every new merchant. The current Merchant service and
-- API v1.1 treat registrationNumber as optional, so that constraint rejects
-- otherwise valid creates. Keep the recovered columns/tables, but restore the
-- baseline optionality with a forward migration.
ALTER TABLE merchant.merchants
  DROP CONSTRAINT IF EXISTS merchants_business_reg_no_present;
