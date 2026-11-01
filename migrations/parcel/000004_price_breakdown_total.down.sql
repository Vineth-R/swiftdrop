-- Restore the original baseline constraint without changing tables or data.
BEGIN;

ALTER TABLE parcel.parcels DROP CONSTRAINT parcels_price_breakdown;
ALTER TABLE parcel.parcels ADD CONSTRAINT parcels_price_breakdown CHECK (
  price_breakdown->'base_fee' IS NOT NULL AND
  price_breakdown->'weight_charge' IS NOT NULL AND
  price_breakdown->'size_surcharge' IS NOT NULL AND
  price_amount = (price_breakdown->>'base_fee')::numeric
               + (price_breakdown->>'weight_charge')::numeric
               + (price_breakdown->>'size_surcharge')::numeric
);

COMMIT;
