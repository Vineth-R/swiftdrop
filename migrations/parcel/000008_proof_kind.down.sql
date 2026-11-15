DROP INDEX IF EXISTS parcel.proofs_parcel_kind_idx;

ALTER TABLE parcel.delivery_proofs
  DROP CONSTRAINT IF EXISTS delivery_proofs_proof_kind_check;

ALTER TABLE parcel.delivery_proofs
  DROP COLUMN IF EXISTS proof_kind;
