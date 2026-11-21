-- DB v1.1 section 5.1 requires a total equal to all three price components.
-- PostgreSQL CHECK accepts NULL; JSON null components must not bypass the total.
-- Preserve applied baseline migrations and validate existing rows without repairs.
BEGIN;

ALTER TABLE parcel.parcels DROP CONSTRAINT parcels_price_breakdown;
ALTER TABLE parcel.parcels ADD CONSTRAINT parcels_price_breakdown CHECK ((
  price_breakdown->'base_fee' IS NOT NULL AND
  price_breakdown->'weight_charge' IS NOT NULL AND
  price_breakdown->'size_surcharge' IS NOT NULL AND
  price_amount = (price_breakdown->>'base_fee')::numeric
               + (price_breakdown->>'weight_charge')::numeric
               + (price_breakdown->>'size_surcharge')::numeric
) IS TRUE) NOT VALID;
ALTER TABLE parcel.parcels VALIDATE CONSTRAINT parcels_price_breakdown;

COMMIT;
