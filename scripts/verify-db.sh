#!/usr/bin/env bash
# Read-only verification of the documented database baseline.
# It never inserts, updates, deletes, resets, or runs migrations.
set -euo pipefail

COMPOSE="${COMPOSE:-docker compose}"

sql="$(cat <<'SQL'
WITH checks(check_name, expected, actual) AS (
  SELECT 'schemas', 5, count(*)::int
  FROM information_schema.schemata
  WHERE schema_name IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
  UNION ALL
  SELECT 'tables', 30, count(*)::int
  FROM information_schema.tables
  WHERE table_schema IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
    AND table_type = 'BASE TABLE'
  UNION ALL
  SELECT 'identity roles', 5, count(*)::int FROM identity.roles
  UNION ALL
  SELECT 'pricing zones', 5, count(*)::int FROM pricing.zones
  UNION ALL
  SELECT 'pricing rules', 25, count(*)::int FROM pricing.pricing_rules
  UNION ALL
  SELECT 'weight bands', 4, count(*)::int FROM pricing.weight_bands
  UNION ALL
  SELECT 'size surcharges', 4, count(*)::int FROM pricing.size_surcharges
)
SELECT check_name, expected, actual FROM checks;
SQL
)"

if ! result="$(printf '%s\n' "$sql" | $COMPOSE exec -T postgres sh -c 'PGOPTIONS="-c default_transaction_read_only=on" psql -v ON_ERROR_STOP=1 -At -F "|" -U "$POSTGRES_USER" -d "$POSTGRES_DB"')"; then
  printf 'error: database verification query failed.\n' >&2
  printf '       Ensure postgres is running and all migrations have been applied.\n' >&2
  exit 1
fi

status=0
printf '%s\n' "$result" | awk -F'|' '
  NF == 3 {
    if ($2 == $3) {
      printf "PASS  %-20s expected %s, found %s\n", $1, $2, $3
    } else {
      printf "FAIL  %-20s expected %s, found %s\n", $1, $2, $3
      status = 1
    }
  }
  END { exit status }
' || status=$?

if [ "$status" -eq 0 ]; then
  printf 'database verification passed\n'
else
  printf 'database verification failed\n' >&2
fi
exit "$status"
