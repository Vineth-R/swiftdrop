-- M9 constraint test. Run after `make migrate` (tables come from migrations/merchant/000002_tables).
-- Everything runs inside one transaction and is rolled back, so it leaves no data.
-- Read the NOTICE lines: every line must start with PASS.
-- Merchant status is PENDING because 000006 requires approved_by and approved_at for APPROVED.
BEGIN;

INSERT INTO merchant.merchants (id, owner_user_id, business_name, business_reg_no, contact_email,
    contact_phone, pickup_address, pickup_zone_id, status, created_at, updated_at)
VALUES ('00000000-0000-4000-8000-000000000001', '00000000-0000-4000-8000-0000000000a1',
    'Test Shop', 'M9-0001', 't@shop.lk', '+94112223344', '1 Test Rd',
    '00000000-0000-4000-8000-0000000000b1', 'PENDING', now(), now());

INSERT INTO merchant.merchant_webhooks VALUES
 ('00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001',
  'https://shop.lk/hook', '\x00', ARRAY['parcel.status_changed'], true, now(), now());

-- Test 1: a second webhook for the same merchant must be refused
DO $$
BEGIN
  BEGIN
    INSERT INTO merchant.merchant_webhooks VALUES
     ('00000000-0000-4000-8000-000000000011', '00000000-0000-4000-8000-000000000001',
      'https://shop.lk/hook2', '\x00', ARRAY['x'], true, now(), now());
    RAISE NOTICE 'FAIL test 1: second webhook was accepted';
  EXCEPTION WHEN unique_violation THEN
    RAISE NOTICE 'PASS test 1: second webhook for same merchant refused';
  END;
END $$;

-- Test 2: the same (webhook_id, event_id, attempt_number) twice must be refused
INSERT INTO merchant.webhook_delivery_attempts
 (id, event_id, webhook_id, merchant_id, parcel_id, tracking_number, attempt_number, success, created_at)
VALUES ('00000000-0000-4000-8000-000000000101', '00000000-0000-4000-8000-0000000000e1',
 '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001',
 '00000000-0000-4000-8000-0000000000c1', 'SDN4T8W2LC6', 1, false, now());

DO $$
BEGIN
  BEGIN
    INSERT INTO merchant.webhook_delivery_attempts
     (id, event_id, webhook_id, merchant_id, parcel_id, tracking_number, attempt_number, success, created_at)
    VALUES ('00000000-0000-4000-8000-000000000102', '00000000-0000-4000-8000-0000000000e1',
     '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001',
     '00000000-0000-4000-8000-0000000000c1', 'SDN4T8W2LC6', 1, false, now());
    RAISE NOTICE 'FAIL test 2: duplicate attempt was accepted';
  EXCEPTION WHEN unique_violation THEN
    RAISE NOTICE 'PASS test 2: duplicate attempt refused';
  END;
END $$;

-- Test 3: attempt_number must be between 1 and 6
DO $$
BEGIN
  BEGIN
    INSERT INTO merchant.webhook_delivery_attempts
     (id, event_id, webhook_id, merchant_id, parcel_id, tracking_number, attempt_number, success, created_at)
    VALUES ('00000000-0000-4000-8000-000000000103', '00000000-0000-4000-8000-0000000000e2',
     '00000000-0000-4000-8000-000000000010', '00000000-0000-4000-8000-000000000001',
     '00000000-0000-4000-8000-0000000000c1', 'SDN4T8W2LC6', 7, false, now());
    RAISE NOTICE 'FAIL test 3: attempt_number 7 was accepted';
  EXCEPTION WHEN check_violation THEN
    RAISE NOTICE 'PASS test 3: attempt_number 7 refused';
  END;
END $$;

-- Test 4: a webhook URL that is not HTTPS must be refused (uses a second merchant)
INSERT INTO merchant.merchants (id, owner_user_id, business_name, business_reg_no, contact_email,
    contact_phone, pickup_address, pickup_zone_id, status, created_at, updated_at)
VALUES ('00000000-0000-4000-8000-000000000002', '00000000-0000-4000-8000-0000000000a2',
    'Test Shop 2', 'M9-0002', 't2@shop.lk', '+94112223355', '2 Test Rd',
    '00000000-0000-4000-8000-0000000000b1', 'PENDING', now(), now());
DO $$
BEGIN
  BEGIN
    INSERT INTO merchant.merchant_webhooks VALUES
     ('00000000-0000-4000-8000-000000000012', '00000000-0000-4000-8000-000000000002',
      'http://shop.lk/hook', '\x00', ARRAY['x'], true, now(), now());
    RAISE NOTICE 'FAIL test 4: http URL was accepted';
  EXCEPTION WHEN check_violation THEN
    RAISE NOTICE 'PASS test 4: non-HTTPS URL refused';
  END;
END $$;

ROLLBACK;
