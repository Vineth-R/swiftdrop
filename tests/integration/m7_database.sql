-- M7.1 database verification: Database Design v1.1 sections 5.1, 5.4, 7.1,
-- 7.2 and API v1.1 sections 6.3-6.4. This is a psql script, not a migration.
--
-- Prerequisites: a disposable PostgreSQL 16 database whose name starts with
-- swiftdrop_m7_test_, the existing public.set_updated_at() bootstrap, and the
-- parcel and notification migrations applied through version 000002.
-- Example, after configuring PGHOST/PGPORT/PGUSER and authentication:
-- psql -X -d swiftdrop_m7_test_local -f tests/integration/m7_database.sql
--
-- Run in a dedicated connection. ON_ERROR_STOP makes any failed assertion exit
-- psql unsuccessfully; disconnecting rolls back an unfinished transaction.
-- On success, ROLLBACK removes all fixtures and session-local helper functions.
-- No migrations, schema rollbacks, truncation, or trigger disabling occur here.
-- Photo checks cover metadata only: 5 MiB = 5,242,880 bytes. File signatures,
-- generated storage names, authorization, and blank input handling belong to
-- the future courier API. An empty proof here means all three content fields
-- (recipient_name, note, photo_path) are NULL, matching the applied baseline.

\set ON_ERROR_STOP on

BEGIN;
SET LOCAL statement_timeout = '30s';
SET LOCAL lock_timeout = '2s';
SET LOCAL TIME ZONE 'UTC';
SET LOCAL search_path = pg_catalog, pg_temp;

DO $guard$
BEGIN
  IF left(current_database(), char_length('swiftdrop_m7_test_')) <> 'swiftdrop_m7_test_' THEN
    RAISE EXCEPTION 'M7 tests require a disposable database named swiftdrop_m7_test_*';
  END IF;
END;
$guard$;

CREATE FUNCTION pg_temp.m7_assert(passed BOOLEAN, test_name TEXT) RETURNS VOID AS $$
BEGIN
  IF passed IS DISTINCT FROM true THEN
    RAISE EXCEPTION 'FAIL: %', test_name;
  END IF;
  RAISE NOTICE 'PASS: %', test_name;
END;
$$ LANGUAGE plpgsql;

-- Each expected error has its own subtransaction. A rejected statement leaves
-- the fixture intact. Match the cause so unrelated errors cannot pass a test.
CREATE FUNCTION pg_temp.m7_expect_error(
  test_name TEXT,
  test_sql TEXT,
  expected_state TEXT,
  expected_constraint TEXT DEFAULT NULL,
  expected_column TEXT DEFAULT NULL
) RETURNS VOID AS $$
DECLARE
  actual_state TEXT;
  actual_constraint TEXT;
  actual_column TEXT;
BEGIN
  BEGIN
    EXECUTE test_sql;
  EXCEPTION WHEN OTHERS THEN
    GET STACKED DIAGNOSTICS
      actual_state = RETURNED_SQLSTATE,
      actual_constraint = CONSTRAINT_NAME,
      actual_column = COLUMN_NAME;
    IF actual_state IS DISTINCT FROM expected_state
       OR (expected_constraint IS NOT NULL AND actual_constraint IS DISTINCT FROM expected_constraint)
       OR (expected_column IS NOT NULL AND actual_column IS DISTINCT FROM expected_column) THEN
      RAISE EXCEPTION 'FAIL: % (SQLSTATE %, constraint %, column %)',
        test_name, actual_state, actual_constraint, actual_column;
    END IF;
    RAISE NOTICE 'PASS: %', test_name;
    RETURN;
  END;
  RAISE EXCEPTION 'FAIL: % (expected SQLSTATE %, statement succeeded)', test_name, expected_state;
END;
$$ LANGUAGE plpgsql;

DO $catalog$
DECLARE
  expected RECORD;
  actual RECORD;
BEGIN
  -- Assert the documented columns, not the complete table shape: future
  -- additive columns do not invalidate the M7.1 contract.
  FOR expected IN
    SELECT * FROM (VALUES
      ('parcel', 'delivery_proofs', 'id', 'uuid', NULL::INTEGER, 'NO'),
      ('parcel', 'delivery_proofs', 'parcel_id', 'uuid', NULL, 'NO'),
      ('parcel', 'delivery_proofs', 'recipient_name', 'varchar', 150, 'YES'),
      ('parcel', 'delivery_proofs', 'note', 'varchar', 500, 'YES'),
      ('parcel', 'delivery_proofs', 'photo_path', 'varchar', 255, 'YES'),
      ('parcel', 'delivery_proofs', 'photo_mime_type', 'varchar', 30, 'YES'),
      ('parcel', 'delivery_proofs', 'photo_size_bytes', 'int4', NULL, 'YES'),
      ('parcel', 'delivery_proofs', 'captured_by', 'uuid', NULL, 'NO'),
      ('parcel', 'delivery_proofs', 'created_at', 'timestamptz', NULL, 'NO'),
      ('notification', 'notification_logs', 'id', 'uuid', NULL, 'NO'),
      ('notification', 'notification_logs', 'event_id', 'uuid', NULL, 'NO'),
      ('notification', 'notification_logs', 'parcel_id', 'uuid', NULL, 'NO'),
      ('notification', 'notification_logs', 'channel', 'varchar', 10, 'NO'),
      ('notification', 'notification_logs', 'recipient', 'varchar', 255, 'NO'),
      ('notification', 'notification_logs', 'subject', 'varchar', 200, 'YES'),
      ('notification', 'notification_logs', 'status', 'varchar', 10, 'NO'),
      ('notification', 'notification_logs', 'error_message', 'varchar', 255, 'YES'),
      ('notification', 'notification_logs', 'created_at', 'timestamptz', NULL, 'NO'),
      ('notification', 'notification_logs', 'sent_at', 'timestamptz', NULL, 'YES'),
      ('notification', 'processed_events', 'consumer', 'varchar', 40, 'NO'),
      ('notification', 'processed_events', 'event_id', 'uuid', NULL, 'NO'),
      ('notification', 'processed_events', 'processed_at', 'timestamptz', NULL, 'NO'),
      ('parcel', 'parcels', 'delivered_at', 'timestamptz', NULL, 'YES'),
      ('parcel', 'parcels', 'failed_at', 'timestamptz', NULL, 'YES'),
      ('parcel', 'parcels', 'failure_reason', 'varchar', 255, 'YES'),
      ('parcel', 'parcels', 'delivered_to_name', 'varchar', 150, 'YES')
    ) AS columns(schema_name, table_name, column_name, udt_name, max_length, nullable)
  LOOP
    SELECT c.udt_name, c.character_maximum_length, c.is_nullable INTO actual
    FROM information_schema.columns c
    WHERE c.table_schema = expected.schema_name
      AND c.table_name = expected.table_name AND c.column_name = expected.column_name;
    PERFORM pg_temp.m7_assert(
      FOUND AND actual.udt_name IS NOT DISTINCT FROM expected.udt_name
        AND actual.character_maximum_length IS NOT DISTINCT FROM expected.max_length
        AND actual.is_nullable IS NOT DISTINCT FROM expected.nullable,
      format('%s.%s.%s type, length and nullability',
        expected.schema_name, expected.table_name, expected.column_name));
  END LOOP;

  FOR expected IN
    SELECT * FROM (VALUES
      ('parcel.delivery_proofs', 'p', ARRAY['id']),
      ('notification.notification_logs', 'p', ARRAY['id']),
      ('notification.notification_logs', 'u', ARRAY['event_id', 'channel', 'recipient']),
      ('notification.processed_events', 'p', ARRAY['consumer', 'event_id'])
    ) AS keys(table_name, constraint_type, column_names)
  LOOP
    PERFORM pg_temp.m7_assert(EXISTS (
      SELECT 1 FROM pg_constraint c
      WHERE c.conrelid = expected.table_name::REGCLASS
        AND c.contype::TEXT = expected.constraint_type AND c.convalidated
        AND ARRAY(
          SELECT a.attname::TEXT
          FROM unnest(c.conkey) WITH ORDINALITY AS k(attnum, position)
          JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum
          ORDER BY k.position
        ) = expected.column_names
    ), format('%s key (%s)', expected.table_name, array_to_string(expected.column_names, ', ')));
  END LOOP;

  PERFORM pg_temp.m7_assert(EXISTS (
    SELECT 1 FROM pg_constraint c
    WHERE c.conrelid = 'parcel.delivery_proofs'::REGCLASS AND c.contype = 'f'
      AND c.confrelid = 'parcel.parcels'::REGCLASS AND c.convalidated
      AND c.conkey = ARRAY[(SELECT attnum FROM pg_attribute
        WHERE attrelid = c.conrelid AND attname = 'parcel_id' AND NOT attisdropped)]
      AND c.confkey = ARRAY[(SELECT attnum FROM pg_attribute
        WHERE attrelid = c.confrelid AND attname = 'id' AND NOT attisdropped)]
  ), 'delivery proof parcel_id references parcel.parcels.id');

  PERFORM pg_temp.m7_assert(EXISTS (
    SELECT 1 FROM pg_index i
    WHERE i.indrelid = 'parcel.delivery_proofs'::REGCLASS
      AND i.indisvalid AND i.indisready AND i.indpred IS NULL AND i.indnkeyatts = 2
      AND ARRAY(
        SELECT a.attname::TEXT
        FROM unnest(i.indkey::SMALLINT[]) WITH ORDINALITY AS k(attnum, position)
        JOIN pg_attribute a ON a.attrelid = i.indrelid AND a.attnum = k.attnum
        WHERE k.position <= i.indnkeyatts ORDER BY k.position
      ) = ARRAY['parcel_id', 'created_at']
  ), 'delivery proofs have a usable (parcel_id, created_at) index');
END;
$catalog$;

DO $behavior$
DECLARE
  failed_parcel_id UUID := gen_random_uuid();
  delivered_parcel_id UUID := gen_random_uuid();
  courier_id UUID := gen_random_uuid();
  photo_proof_id UUID;
  proof parcel.delivery_proofs%ROWTYPE;
  notification_log notification.notification_logs%ROWTYPE;
  processed_event notification.processed_events%ROWTYPE;
  test_case RECORD;
  column_name TEXT;
  allowed_status TEXT;
  stored_status TEXT;
  stored_reason TEXT;
  stored_name TEXT;
  stored_time TIMESTAMPTZ;
BEGIN
  -- Synthetic fixture IDs belonging to other services remain plain UUID refs.
  -- Correct ownership, tracking prefixes and price totals keep unrelated
  -- parcel constraints from masking the M7 assertions.
  INSERT INTO parcel.parcels (
    id, tracking_number, customer_user_id, sender_name, sender_phone,
    pickup_address, pickup_city, pickup_zone_id, recipient_name, recipient_phone,
    dropoff_address, dropoff_city, dropoff_zone_id, weight_kg, size_category,
    price_amount, price_breakdown, created_by_user_id
  )
  SELECT fixture.id, 'SDK' || upper(left(replace(fixture.id::TEXT, '-', ''), 8)),
    gen_random_uuid(), 'M7 Sender', '+94770000001',
    'M7 pickup fixture', 'Colombo', gen_random_uuid(), 'M7 Recipient', '+94770000002',
    'M7 dropoff fixture', 'Kandy', gen_random_uuid(), 2.00, 'SMALL',
    550.00, '{"base_fee":450,"weight_charge":100,"size_surcharge":0}'::JSONB, gen_random_uuid()
  FROM (VALUES
    (failed_parcel_id), (delivered_parcel_id)
  ) AS fixture(id);

  PERFORM pg_temp.m7_assert((SELECT count(*) = 2 FROM parcel.parcels
    WHERE id IN (failed_parcel_id, delivered_parcel_id) AND status = 'CREATED'
      AND failed_at IS NULL AND failure_reason IS NULL
      AND delivered_at IS NULL AND delivered_to_name IS NULL),
    'nonterminal parcels allow absent delivery and failure fields');

  -- Delivery proofs: optional content, defaults and valid boundary values.
  INSERT INTO parcel.delivery_proofs (parcel_id, recipient_name, captured_by)
  VALUES (failed_parcel_id, 'M7 Recipient', courier_id) RETURNING * INTO proof;
  PERFORM pg_temp.m7_assert(proof.id IS NOT NULL AND proof.created_at = now()
    AND proof.note IS NULL AND proof.photo_path IS NULL
    AND proof.photo_mime_type IS NULL AND proof.photo_size_bytes IS NULL,
    'recipient-only proof uses ID/time defaults and permits absent photo metadata');

  INSERT INTO parcel.delivery_proofs (
    parcel_id, photo_path, photo_mime_type, photo_size_bytes, captured_by
  ) VALUES (failed_parcel_id, 'm7-proof.jpg', 'image/jpeg', 1, courier_id)
  RETURNING id INTO photo_proof_id;

  FOR test_case IN
    SELECT * FROM (VALUES
      ('note-only proof', NULL::TEXT, 'M7 note', NULL::TEXT, NULL::TEXT, NULL::INTEGER),
      ('JPEG at 5 MiB and text limits', repeat('r', 150), repeat('n', 500),
        repeat('p', 251) || '.jpg', 'image/jpeg', 5242880),
      ('PNG at one byte', NULL, NULL, 'm7-proof.png', 'image/png', 1),
      ('PNG at 5 MiB', NULL, NULL, 'm7-proof.png', 'image/png', 5242880)
    ) AS cases(name, recipient, note, path, mime, size_bytes)
  LOOP
    INSERT INTO parcel.delivery_proofs (
      parcel_id, recipient_name, note, photo_path, photo_mime_type, photo_size_bytes, captured_by
    ) VALUES (failed_parcel_id, test_case.recipient, test_case.note, test_case.path,
      test_case.mime, test_case.size_bytes, courier_id);
    PERFORM pg_temp.m7_assert(FOUND, test_case.name);
  END LOOP;
  PERFORM pg_temp.m7_assert((SELECT count(*) = 6 FROM parcel.delivery_proofs
    WHERE parcel_id = failed_parcel_id), 'multiple proofs per parcel are allowed');

  FOREACH column_name IN ARRAY ARRAY['id', 'parcel_id', 'captured_by', 'created_at'] LOOP
    PERFORM pg_temp.m7_expect_error('proof requires ' || column_name,
      format('UPDATE parcel.delivery_proofs SET %I = NULL WHERE id = %L', column_name, photo_proof_id),
      '23502', NULL, column_name);
  END LOOP;

  FOR test_case IN
    SELECT * FROM (VALUES
      ('proof rejects unknown parcel', 'parcel_id = gen_random_uuid()', '23503',
        'delivery_proofs_parcel_id_fkey'),
      ('proof rejects GIF', $$photo_mime_type = 'image/gif'$$, '23514',
        'delivery_proofs_photo_mime_type_check'),
      ('proof rejects non-image MIME', $$photo_mime_type = 'text/plain'$$, '23514',
        'delivery_proofs_photo_mime_type_check'),
      ('proof rejects zero bytes', 'photo_size_bytes = 0', '23514',
        'delivery_proofs_photo_size_bytes_check'),
      ('proof rejects negative bytes', 'photo_size_bytes = -1', '23514',
        'delivery_proofs_photo_size_bytes_check'),
      ('proof rejects 5 MiB plus one byte', 'photo_size_bytes = 5242881', '23514',
        'delivery_proofs_photo_size_bytes_check'),
      ('photo requires MIME', 'photo_mime_type = NULL', '23514', 'proofs_photo_fields'),
      ('photo requires size', 'photo_size_bytes = NULL', '23514', 'proofs_photo_fields'),
      ('photo requires both metadata fields', 'photo_mime_type = NULL, photo_size_bytes = NULL',
        '23514', 'proofs_photo_fields'),
      ('proof rejects absent content',
        'recipient_name = NULL, note = NULL, photo_path = NULL, photo_mime_type = NULL, photo_size_bytes = NULL',
        '23514', 'proofs_not_empty'),
      ('proof rejects metadata without content', 'recipient_name = NULL, note = NULL, photo_path = NULL',
        '23514', 'proofs_not_empty'),
      ('proof recipient limit is 150', $$recipient_name = repeat('r', 151)$$, '22001', NULL),
      ('proof note limit is 500', $$note = repeat('n', 501)$$, '22001', NULL),
      ('proof path limit is 255', $$photo_path = repeat('p', 256)$$, '22001', NULL)
    ) AS cases(name, assignments, sqlstate, constraint_name)
  LOOP
    PERFORM pg_temp.m7_expect_error(test_case.name,
      format('UPDATE parcel.delivery_proofs SET %s WHERE id = %L', test_case.assignments, photo_proof_id),
      test_case.sqlstate, test_case.constraint_name);
  END LOOP;
  PERFORM pg_temp.m7_expect_error('proof primary key rejects duplicate ID',
    format('INSERT INTO parcel.delivery_proofs SELECT * FROM parcel.delivery_proofs WHERE id = %L', proof.id),
    '23505', 'delivery_proofs_pkey');
  PERFORM pg_temp.m7_expect_error('referenced parcel cannot be deleted',
    format('DELETE FROM parcel.parcels WHERE id = %L', failed_parcel_id),
    '23503', 'delivery_proofs_parcel_id_fkey');

  -- Notification logs: defaults, enum checks and deduplication scope.
  INSERT INTO notification.notification_logs (event_id, parcel_id, channel, recipient)
  VALUES (gen_random_uuid(), failed_parcel_id, 'EMAIL', 'm7.fixture@example.invalid')
  RETURNING * INTO notification_log;
  PERFORM pg_temp.m7_assert(notification_log.id IS NOT NULL AND notification_log.created_at = now()
    AND notification_log.status = 'PENDING' AND notification_log.subject IS NULL
    AND notification_log.error_message IS NULL AND notification_log.sent_at IS NULL,
    'notification defaults to PENDING with ID/time and nullable optional fields');

  FOREACH allowed_status IN ARRAY ARRAY['PENDING', 'SENT', 'FAILED'] LOOP
    UPDATE notification.notification_logs SET status = allowed_status
    WHERE id = notification_log.id RETURNING status INTO stored_status;
    PERFORM pg_temp.m7_assert(FOUND AND stored_status = allowed_status,
      'notification accepts status ' || allowed_status);
  END LOOP;
  FOREACH column_name IN ARRAY ARRAY[
    'id', 'event_id', 'parcel_id', 'channel', 'recipient', 'status', 'created_at'
  ] LOOP
    PERFORM pg_temp.m7_expect_error('notification requires ' || column_name,
      format('UPDATE notification.notification_logs SET %I = NULL WHERE id = %L',
        column_name, notification_log.id), '23502', NULL, column_name);
  END LOOP;
  FOR test_case IN
    SELECT * FROM (VALUES
      ('notification rejects unsupported channel', $$channel = 'PUSH'$$,
        'notification_logs_channel_check'),
      ('notification rejects lowercase channel', $$channel = 'email'$$,
        'notification_logs_channel_check'),
      ('notification rejects unsupported status', $$status = 'QUEUED'$$,
        'notification_logs_status_check'),
      ('notification rejects lowercase status', $$status = 'sent'$$,
        'notification_logs_status_check')
    ) AS cases(name, assignments, constraint_name)
  LOOP
    PERFORM pg_temp.m7_expect_error(test_case.name,
      format('UPDATE notification.notification_logs SET %s WHERE id = %L',
        test_case.assignments, notification_log.id), '23514', test_case.constraint_name);
  END LOOP;
  PERFORM pg_temp.m7_expect_error('notification rejects duplicate event/channel/recipient',
    format('INSERT INTO notification.notification_logs (event_id, parcel_id, channel, recipient)
      VALUES (%L, %L, %L, %L)', notification_log.event_id, failed_parcel_id,
      notification_log.channel, notification_log.recipient), '23505', 'notification_dedupe_uq');
  PERFORM pg_temp.m7_expect_error('notification primary key rejects duplicate ID',
    format('INSERT INTO notification.notification_logs (id, event_id, parcel_id, channel, recipient)
      VALUES (%L, gen_random_uuid(), %L, %L, %L)', notification_log.id, failed_parcel_id,
      notification_log.channel, notification_log.recipient), '23505', 'notification_logs_pkey');

  -- Vary one dedupe component at a time. The recipient is opaque database
  -- metadata; using the same synthetic value for SMS tests key scope only.
  INSERT INTO notification.notification_logs (event_id, parcel_id, channel, recipient) VALUES
    (notification_log.event_id, failed_parcel_id, 'SMS', notification_log.recipient),
    (notification_log.event_id, failed_parcel_id, 'EMAIL', 'm7.other@example.invalid'),
    (gen_random_uuid(), failed_parcel_id, 'EMAIL', notification_log.recipient);
  PERFORM pg_temp.m7_assert((SELECT count(*) = 3 FROM notification.notification_logs
    WHERE event_id = notification_log.event_id) AND
    (SELECT count(*) = 2 FROM notification.notification_logs
      WHERE channel = notification_log.channel AND recipient = notification_log.recipient),
    'notification permits distinct channel, recipient or event');

  -- Processed events deduplicate per consumer, not globally per event.
  INSERT INTO notification.processed_events (consumer, event_id)
  VALUES ('m7.notification', gen_random_uuid()) RETURNING * INTO processed_event;
  PERFORM pg_temp.m7_assert(processed_event.processed_at = now(),
    'processed event receives its processing timestamp by default');
  PERFORM pg_temp.m7_expect_error('processed events reject same consumer and event',
    format('INSERT INTO notification.processed_events (consumer, event_id)
      VALUES (%L, %L)', processed_event.consumer, processed_event.event_id),
    '23505', 'processed_events_pkey');
  INSERT INTO notification.processed_events (consumer, event_id) VALUES
    ('m7.other', processed_event.event_id), (processed_event.consumer, gen_random_uuid());
  PERFORM pg_temp.m7_assert((SELECT count(*) = 2 FROM notification.processed_events
    WHERE event_id = processed_event.event_id) AND
    (SELECT count(*) = 2 FROM notification.processed_events WHERE consumer = processed_event.consumer),
    'processed event permits another consumer or event');
  FOREACH column_name IN ARRAY ARRAY['consumer', 'event_id', 'processed_at'] LOOP
    PERFORM pg_temp.m7_expect_error('processed event requires ' || column_name,
      format('UPDATE notification.processed_events SET %I = NULL WHERE consumer = %L AND event_id = %L',
        column_name, processed_event.consumer, processed_event.event_id), '23502', NULL, column_name);
  END LOOP;
  PERFORM pg_temp.m7_expect_error('processed consumer limit is 40',
    format('UPDATE notification.processed_events SET consumer = repeat(''c'', 41)
      WHERE consumer = %L AND event_id = %L', processed_event.consumer, processed_event.event_id), '22001');

  -- FAILED tests use the allowed CREATED -> FAILED transition. Match the
  -- field constraint, rather than accidentally passing on a transition error.
  FOR test_case IN
    SELECT * FROM (VALUES
      ('FAILED requires reason', 'failure_reason = NULL, failed_at = now()',
        '23514', 'parcels_failed_fields'),
      ('FAILED requires timestamp', $$failure_reason = 'Valid reason', failed_at = NULL$$,
        '23514', 'parcels_failed_fields'),
      ('FAILED rejects empty reason', $$failure_reason = '', failed_at = now()$$,
        '23514', 'parcels_failed_fields'),
      ('FAILED rejects four-character reason', $$failure_reason = 'abcd', failed_at = now()$$,
        '23514', 'parcels_failed_fields'),
      ('FAILED rejects 256-character reason', $$failure_reason = repeat('r', 256), failed_at = now()$$,
        '22001', NULL)
    ) AS cases(name, assignments, sqlstate, constraint_name)
  LOOP
    PERFORM pg_temp.m7_expect_error(test_case.name,
      format('UPDATE parcel.parcels SET status = ''FAILED'', %s WHERE id = %L',
        test_case.assignments, failed_parcel_id), test_case.sqlstate, test_case.constraint_name);
  END LOOP;
  UPDATE parcel.parcels SET status = 'FAILED', failure_reason = 'abcde', failed_at = now()
  WHERE id = failed_parcel_id RETURNING failure_reason, failed_at INTO stored_reason, stored_time;
  PERFORM pg_temp.m7_assert(FOUND AND char_length(stored_reason) = 5 AND stored_time = now(),
    'FAILED accepts five-character reason with timestamp');
  UPDATE parcel.parcels SET failure_reason = repeat(chr(233), 255)
  WHERE id = failed_parcel_id RETURNING failure_reason INTO stored_reason;
  PERFORM pg_temp.m7_assert(FOUND AND char_length(stored_reason) = 255,
    'FAILED accepts 255 characters, including multibyte text');

  -- Reach OUT_FOR_DELIVERY normally so DELIVERED field tests cannot be
  -- mistaken for invalid-transition tests. No triggers are disabled.
  UPDATE parcel.parcels SET status = 'PICKED_UP' WHERE id = delivered_parcel_id;
  UPDATE parcel.parcels SET status = 'IN_TRANSIT' WHERE id = delivered_parcel_id;
  UPDATE parcel.parcels SET status = 'OUT_FOR_DELIVERY' WHERE id = delivered_parcel_id;
  FOR test_case IN
    SELECT * FROM (VALUES
      ('DELIVERED requires timestamp', $$delivered_to_name = 'M7 Recipient', delivered_at = NULL$$,
        '23514', 'parcels_delivered_fields'),
      ('DELIVERED requires recipient', 'delivered_to_name = NULL, delivered_at = now()',
        '23514', 'parcels_delivered_fields'),
      ('DELIVERED recipient limit is 150', $$delivered_to_name = repeat('r', 151), delivered_at = now()$$,
        '22001', NULL)
    ) AS cases(name, assignments, sqlstate, constraint_name)
  LOOP
    PERFORM pg_temp.m7_expect_error(test_case.name,
      format('UPDATE parcel.parcels SET status = ''DELIVERED'', %s WHERE id = %L',
        test_case.assignments, delivered_parcel_id), test_case.sqlstate, test_case.constraint_name);
  END LOOP;
  UPDATE parcel.parcels SET status = 'DELIVERED', delivered_to_name = repeat('r', 150), delivered_at = now()
  WHERE id = delivered_parcel_id RETURNING delivered_to_name, delivered_at INTO stored_name, stored_time;
  PERFORM pg_temp.m7_assert(FOUND AND char_length(stored_name) = 150 AND stored_time = now(),
    'DELIVERED accepts recipient at length limit with timestamp');
END;
$behavior$;

ROLLBACK;
\echo M7.1 database checks passed; all test fixtures and helpers rolled back.
