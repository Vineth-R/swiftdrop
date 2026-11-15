-- Distinguish pickup proof from delivery proof so the courier workflow can
-- show both photos without guessing from free-text notes.
--
-- Existing rows are generic delivery proofs, so the DEFAULT backfills them as
-- DELIVERY. New pickup confirmation writes PICKUP. The column is additive and
-- does not change the existing photo or recipient constraints.

ALTER TABLE parcel.delivery_proofs
  ADD COLUMN IF NOT EXISTS proof_kind VARCHAR(20) NOT NULL DEFAULT 'DELIVERY';

ALTER TABLE parcel.delivery_proofs
  DROP CONSTRAINT IF EXISTS delivery_proofs_proof_kind_check;

ALTER TABLE parcel.delivery_proofs
  ADD CONSTRAINT delivery_proofs_proof_kind_check
  CHECK (proof_kind IN ('PICKUP', 'DELIVERY'));

CREATE INDEX IF NOT EXISTS proofs_parcel_kind_idx
  ON parcel.delivery_proofs (parcel_id, proof_kind, created_at);
