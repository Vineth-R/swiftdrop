-- Additive index for the authenticated customer parcel list (API v1.1
-- section 4.2). The query filters by customer_user_id, applies optional
-- created_at bounds, and orders by created_at then id in the same direction.
--
-- A single ascending btree on (customer_user_id, created_at, id) supports both
-- ORDER BY directions with a backward index scan, so no duplicate descending
-- index is needed. The existing parcels_customer_idx only covers the owner
-- predicate and forces an explicit sort.
--
-- This migration is forward-only and does not modify any applied migration.

BEGIN;

CREATE INDEX IF NOT EXISTS idx_parcels_customer_created_id
  ON parcel.parcels (customer_user_id, created_at, id);

COMMIT;
