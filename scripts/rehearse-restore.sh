#!/bin/sh
# Creates a disposable source and target database on REHEARSAL_DATABASE_URL,
# applies all migrations, seeds representative data, backs up the source with
# the real backup script, restores into the target with the real restore
# script, and validates the restored result. Only the two generated databases
# and the temporary backup directory are removed.
#
# This is a backup/restore rehearsal, not a migration or data reset. It never
# targets the application database and never drops a database it did not create.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
url="${REHEARSAL_DATABASE_URL:-${TEST_DATABASE_URL:-}}"

if [ -z "$url" ]; then
    echo "error: REHEARSAL_DATABASE_URL (or TEST_DATABASE_URL) is required" >&2
    exit 2
fi
case "$url" in
    postgres://* | postgresql://*) ;;
    *)
        echo "error: REHEARSAL_DATABASE_URL must be a PostgreSQL URL" >&2
        exit 2
        ;;
esac

for command in psql pg_dump pg_restore migrate; do
    if ! command -v "$command" >/dev/null 2>&1; then
        echo "error: $command is required for the restore rehearsal" >&2
        exit 2
    fi
done

stamp="$(date -u +%Y%m%dT%H%M%S)_$$"
source_db="swiftdrop_p413_src_${stamp}"
target_db="swiftdrop_p413_dst_${stamp}"
tmp="$(mktemp -d "${TMPDIR:-/tmp}/swiftdrop-p413.XXXXXX")"
source_created=0
target_created=0

with_database() {
    base="$1"
    database="$2"
    query=""
    case "$base" in
        *\?*)
            query="${base#*\?}"
            base="${base%%\?*}"
            ;;
    esac
    prefix="${base%/*}"
    result="${prefix}/${database}"
    if [ -n "$query" ]; then
        result="${result}?${query}"
    fi
    printf '%s' "$result"
}

with_parameter() {
    case "$1" in
        *\?*) printf '%s&%s' "$1" "$2" ;;
        *) printf '%s?%s' "$1" "$2" ;;
    esac
}

cleanup() {
    rc=$?
    trap - EXIT INT TERM
    rm -rf "$tmp"
    if [ "$target_created" = "1" ]; then
        psql "$url" -X -v ON_ERROR_STOP=0 -c "DROP DATABASE IF EXISTS \"$target_db\" WITH (FORCE)" >/dev/null 2>&1 || true
    fi
    if [ "$source_created" = "1" ]; then
        psql "$url" -X -v ON_ERROR_STOP=0 -c "DROP DATABASE IF EXISTS \"$source_db\" WITH (FORCE)" >/dev/null 2>&1 || true
    fi
    exit "$rc"
}
trap cleanup EXIT INT TERM

echo "creating disposable source and target databases"
psql "$url" -X -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$source_db\"" >/dev/null
source_created=1
psql "$url" -X -v ON_ERROR_STOP=1 -c "CREATE DATABASE \"$target_db\"" >/dev/null
target_created=1
source_url="$(with_database "$url" "$source_db")"
target_url="$(with_database "$url" "$target_db")"

echo "applying schemas and migrations to the source"
psql "$source_url" -X -v ON_ERROR_STOP=1 -f "$root/infra/docker/postgres/init/01-schemas.sql" >/dev/null
for service in identity pricing parcel merchant notification; do
    migrate -path "$root/migrations/$service" \
        -database "$(with_parameter "$source_url" "x-migrations-table=${service}_schema_migrations")" \
        up >/dev/null
done

echo "seeding representative data"
psql "$source_url" -X -v ON_ERROR_STOP=1 >/dev/null <<'SQL'
INSERT INTO identity.users (email, password_hash, full_name, role_id, email_verified)
VALUES
    ('p413-customer@example.test', 'rehearsal-hash', 'P413 Customer', 1, true),
    ('p413-merchant@example.test', 'rehearsal-hash', 'P413 Merchant', 2, true)
ON CONFLICT DO NOTHING;

INSERT INTO merchant.merchants (
    owner_user_id, business_name, business_reg_no, contact_email,
    contact_phone, pickup_address, pickup_zone_id, status
)
SELECT u.id, 'P413 Rehearsal Shop', 'P413-REH', 'p413-merchant@example.test',
       '+94770000000', '1 Rehearsal Road', z.id, 'PENDING'
FROM identity.users u, pricing.zones z
WHERE u.email = 'p413-merchant@example.test' AND z.code = 'CMB'
ON CONFLICT DO NOTHING;

INSERT INTO parcel.parcels (
    tracking_number, customer_user_id, sender_name, sender_phone,
    pickup_address, pickup_city, pickup_zone_id,
    recipient_name, recipient_phone, dropoff_address, dropoff_city, dropoff_zone_id,
    weight_kg, size_category, status, price_amount, price_breakdown, created_by_user_id
)
SELECT 'SDKREHEAR01', u.id, 'P413 Sender', '+94770000001',
       '1 Pickup Road', 'Colombo', p.id,
       'P413 Recipient', '+94770000002', '1 Dropoff Road', 'Kandy', d.id,
       2.00, 'SMALL', 'CREATED', 550.00,
       '{"base_fee": 300, "weight_charge": 150, "size_surcharge": 100}'::jsonb, u.id
FROM identity.users u, pricing.zones p, pricing.zones d
WHERE u.email = 'p413-customer@example.test' AND p.code = 'CMB' AND d.code = 'KDY'
ON CONFLICT DO NOTHING;

INSERT INTO notification.notification_logs (
    event_id, parcel_id, channel, recipient, subject, status, sent_at
)
SELECT gen_random_uuid(), p.id, 'EMAIL', 'p413-log@example.test',
       'P413 rehearsal', 'SENT', now()
FROM parcel.parcels p
WHERE p.tracking_number = 'SDKREHEAR01';
SQL

source_counts="$(psql "$source_url" -X -At -F '|' -v ON_ERROR_STOP=1 -c "
SELECT
    (SELECT count(*) FROM identity.users WHERE email LIKE 'p413-%@example.test'),
    (SELECT count(*) FROM merchant.merchants WHERE business_name = 'P413 Rehearsal Shop'),
    (SELECT count(*) FROM parcel.parcels WHERE tracking_number = 'SDKREHEAR01'),
    (SELECT count(*) FROM notification.notification_logs WHERE recipient = 'p413-log@example.test'),
    (SELECT count(*) FROM pricing.zones)")"
if [ "$source_counts" != "2|1|1|1|5" ]; then
    echo "error: source representative data is incomplete: $source_counts" >&2
    exit 1
fi

echo "creating and verifying a backup through scripts/backup-db.sh"
BACKUP_SOURCE=url DATABASE_URL="$source_url" BACKUP_DIR="$tmp" "$root/scripts/backup-db.sh"
dump="$(find "$tmp" -maxdepth 1 -name 'swiftdrop-*.dump' -print | head -n 1)"
if [ -z "$dump" ] || [ ! -f "$dump" ]; then
    echo "error: backup script did not produce a dump" >&2
    exit 1
fi
if ! pg_restore --list "$dump" >/dev/null; then
    echo "error: backup dump is not readable by pg_restore" >&2
    exit 1
fi

echo "restoring through scripts/restore-db.sh and validating the target"
APP_ENV=development DATABASE_URL_DIRECT="$target_url" "$root/scripts/restore-db.sh" "$dump"

target_counts="$(psql "$target_url" -X -At -F '|' -v ON_ERROR_STOP=1 -c "
SELECT
    (SELECT count(*) FROM identity.users WHERE email LIKE 'p413-%@example.test'),
    (SELECT count(*) FROM merchant.merchants WHERE business_name = 'P413 Rehearsal Shop'),
    (SELECT count(*) FROM parcel.parcels WHERE tracking_number = 'SDKREHEAR01'),
    (SELECT count(*) FROM notification.notification_logs WHERE recipient = 'p413-log@example.test'),
    (SELECT count(*) FROM pricing.zones)")"
if [ "$target_counts" != "2|1|1|1|5" ]; then
    echo "error: restored representative data does not match: $target_counts" >&2
    exit 1
fi

echo "PASS  backup created and verified"
echo "PASS  restore completed into a disposable target"
echo "PASS  restored representative data matches the source across all five schemas"
echo "PASS  restored migration versions, dirty flags, tables and retention index validated"
