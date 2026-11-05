-- LOCAL TEST DATA for the reports API. Never run this outside your own machine.
-- Safe to run again: it removes its own rows (sender_name = 'REPORT TEST') first.
--
-- parcel.parcel_status_history is append-only for application writes. This
-- local-only script briefly disables only that trigger inside its transaction
-- so it can remove its own history rows. PostgreSQL DDL is transactional, so
-- COMMIT or ROLLBACK restores the trigger even when the script fails. Run it
-- as the parcel schema owner (the local Compose PostgreSQL user).
--
-- Creates 60 parcels spread over the last 10 days (6 per day):
--   30 DELIVERED, 10 FAILED (4 / 3 / 3 over three reasons), 10 IN_TRANSIT, 10 CREATED.
-- Expected GET /reports/outcomes over a range covering all 10 days:
--   created 60, delivered 30, failed 10, inProgress 20, successRate 75, failureRate 25.

START TRANSACTION;

ALTER TABLE parcel.parcel_status_history DISABLE TRIGGER history_append_only;
DELETE FROM parcel.parcel_status_history
 WHERE parcel_id IN (SELECT id FROM parcel.parcels WHERE sender_name = 'REPORT TEST');
ALTER TABLE parcel.parcel_status_history ENABLE TRIGGER history_append_only;

DELETE FROM parcel.parcels WHERE sender_name = 'REPORT TEST';

INSERT INTO parcel.parcels (
  tracking_number, customer_user_id, sender_name, sender_phone,
  pickup_address, pickup_city, pickup_zone_id,
  recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
  weight_kg, size_category, status, failure_reason,
  price_amount, price_breakdown, created_by_user_id,
  created_at, updated_at, delivered_at, failed_at, delivered_to_name
)
SELECT
  'SDK' || upper(substr(md5('report-test-' || t.n), 1, 8)),
  '00000000-0000-0000-0000-0000000000c1'::uuid,
  'REPORT TEST', '+94770000000',
  '1 Test Road', 'Colombo', '00000000-0000-0000-0000-0000000000a1'::uuid,
  'Test Recipient', '+94771111111', '2 Test Lane', 'Kandy', '00000000-0000-0000-0000-0000000000a2'::uuid,
  2.00, 'SMALL', t.status, t.reason,
  550.00, '{"base_fee":450,"weight_charge":100,"size_surcharge":0}'::jsonb,
  '00000000-0000-0000-0000-0000000000c1'::uuid,
  t.created_at, t.created_at,
  CASE WHEN t.status = 'DELIVERED' THEN t.created_at + interval '1 hour' END,
  CASE WHEN t.status = 'FAILED'    THEN t.created_at + interval '2 hours' END,
  CASE WHEN t.status = 'DELIVERED' THEN 'Test Recipient' END
FROM (
  SELECT n,
         (((now() AT TIME ZONE 'UTC')::date - (n % 10)) + interval '6 hours' + (n % 6) * interval '1 hour') AT TIME ZONE 'UTC' AS created_at,
         CASE n % 6
           WHEN 0 THEN 'DELIVERED' WHEN 1 THEN 'DELIVERED' WHEN 2 THEN 'DELIVERED'
           WHEN 3 THEN 'FAILED' WHEN 4 THEN 'IN_TRANSIT' ELSE 'CREATED'
         END AS status,
         CASE WHEN n % 6 = 3 THEN
           (ARRAY['Recipient not available', 'Incorrect address', 'Refused by recipient'])[((n / 6) % 3) + 1]
         END AS reason
  FROM generate_series(1, 60) AS n
) AS t;

INSERT INTO parcel.parcel_status_history (parcel_id, previous_status, new_status, actor_role, created_at)
SELECT id, NULL, 'CREATED', 'CUSTOMER', created_at
  FROM parcel.parcels WHERE sender_name = 'REPORT TEST';

INSERT INTO parcel.parcel_status_history (parcel_id, previous_status, new_status, actor_role, failure_reason, created_at)
SELECT id, 'OUT_FOR_DELIVERY', status, 'COURIER', failure_reason, coalesce(delivered_at, failed_at)
  FROM parcel.parcels
 WHERE sender_name = 'REPORT TEST' AND status IN ('DELIVERED', 'FAILED');

COMMIT;
