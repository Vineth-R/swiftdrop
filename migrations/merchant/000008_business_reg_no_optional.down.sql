ALTER TABLE merchant.merchants
  ADD CONSTRAINT merchants_business_reg_no_present
  CHECK (business_reg_no IS NOT NULL AND btrim(business_reg_no) <> '') NOT VALID;
