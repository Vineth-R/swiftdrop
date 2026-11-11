-- Read-only inventory of the local SwiftDrop database before a Neon restore.
-- Run with:
--   docker compose exec -T postgres psql -X -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f - < scripts/audit-source-db.sql
\pset pager off

SELECT current_database() AS database,
       current_setting('server_version') AS pg_version,
       pg_size_pretty(pg_database_size(current_database())) AS size,
       pg_encoding_to_char(encoding) AS encoding,
       datcollate AS collate
FROM pg_database
WHERE datname = current_database();

SELECT schema_name
FROM information_schema.schemata
WHERE schema_name IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
ORDER BY 1;

SELECT table_schema, table_name,
       (xpath('/row/c/text()', query_to_xml(
           format('SELECT count(*) AS c FROM %I.%I', table_schema, table_name),
           false, true, '')))[1]::text::bigint AS exact_rows
FROM information_schema.tables
WHERE table_schema IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
  AND table_type = 'BASE TABLE'
ORDER BY 1, 2;

SELECT 'identity' AS service, version, dirty FROM public.identity_schema_migrations
UNION ALL SELECT 'pricing', version, dirty FROM public.pricing_schema_migrations
UNION ALL SELECT 'parcel', version, dirty FROM public.parcel_schema_migrations
UNION ALL SELECT 'merchant', version, dirty FROM public.merchant_schema_migrations
UNION ALL SELECT 'notification', version, dirty FROM public.notification_schema_migrations
ORDER BY 1;

SELECT count(*) AS proofs,
       count(photo_path) AS proofs_with_photo,
       count(*) FILTER (WHERE photo_path IS NULL) AS metadata_only
FROM parcel.delivery_proofs;

SELECT id, parcel_id, photo_path, photo_mime_type, photo_size_bytes, created_at
FROM parcel.delivery_proofs
WHERE photo_path IS NOT NULL
ORDER BY created_at, id;

SELECT extname FROM pg_extension ORDER BY 1;

SELECT n.nspname AS schema, p.proname AS function
FROM pg_proc p
JOIN pg_namespace n ON n.oid = p.pronamespace
WHERE n.nspname IN ('public', 'identity', 'pricing', 'parcel', 'merchant', 'notification')
ORDER BY 1, 2;

SELECT conrelid::regclass AS child, confrelid::regclass AS parent, conname
FROM pg_constraint
WHERE contype = 'f'
  AND connamespace IN ('identity'::regnamespace, 'pricing'::regnamespace,
                       'parcel'::regnamespace, 'merchant'::regnamespace,
                       'notification'::regnamespace)
  AND confrelid::regnamespace <> connamespace
ORDER BY 1, 2;

SELECT status, event_type, count(*)
FROM parcel.outbox_events
GROUP BY 1, 2
ORDER BY 1, 2;
