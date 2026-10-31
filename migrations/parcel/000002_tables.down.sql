DROP TRIGGER IF EXISTS history_append_only ON parcel.parcel_status_history;
DROP TRIGGER IF EXISTS parcels_status_transition ON parcel.parcels;
DROP FUNCTION IF EXISTS parcel.forbid_history_change();
DROP FUNCTION IF EXISTS parcel.enforce_status_transition();

DROP TABLE IF EXISTS parcel.outbox_events;
DROP TABLE IF EXISTS parcel.delivery_proofs;
DROP TABLE IF EXISTS parcel.parcel_assignments;
DROP TABLE IF EXISTS parcel.parcel_status_history;
DROP TABLE IF EXISTS parcel.parcels;
