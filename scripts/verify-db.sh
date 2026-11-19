#!/usr/bin/env bash
# Read-only verification of the documented database baseline.
# It never inserts, updates, deletes, resets, or runs migrations.
set -euo pipefail

COMPOSE="${COMPOSE:-docker compose}"

sql="$(cat <<'SQL'
-- Keep these expected versions in step with each service head migration.
WITH checks(check_name, expected, actual) AS (
  SELECT 'schemas', 5, count(*)::int
  FROM information_schema.schemata
  WHERE schema_name IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
  UNION ALL
  SELECT 'tables', 31, count(*)::int
  FROM information_schema.tables
  WHERE table_schema IN ('identity', 'pricing', 'parcel', 'merchant', 'notification')
    AND table_type = 'BASE TABLE'
  UNION ALL
  SELECT 'email verification', 1, count(*)::int
  FROM information_schema.tables
  WHERE table_schema='identity' AND table_name='email_verification_tokens'
  UNION ALL
  SELECT 'merchant lifecycle', 1, count(*)::int
  FROM information_schema.tables
  WHERE table_schema='identity' AND table_name='merchant_lifecycle_transitions'
  UNION ALL
  SELECT 'merchant registration', 1, count(*)::int
  FROM information_schema.tables
  WHERE table_schema='merchant' AND table_name='merchant_registration_attempts'
  UNION ALL
  SELECT 'merchant transitions', 1, count(*)::int
  FROM information_schema.tables
  WHERE table_schema='merchant' AND table_name='merchant_status_transitions'
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
  UNION ALL
  SELECT 'notification retention index', 1, count(*)::int
  FROM pg_indexes
  WHERE schemaname = 'notification' AND indexname = 'notification_logs_created_at_idx'
  UNION ALL
  SELECT 'identity migration', 6, version FROM public.identity_schema_migrations
  UNION ALL
  SELECT 'pricing migration', 2, version FROM public.pricing_schema_migrations
  UNION ALL
  SELECT 'parcel migration', 8, version FROM public.parcel_schema_migrations
  UNION ALL
  SELECT 'merchant migration', 11, version FROM public.merchant_schema_migrations
  UNION ALL
  SELECT 'notification migration', 3, version FROM public.notification_schema_migrations
  UNION ALL
  SELECT 'identity dirty', 0, dirty::int FROM public.identity_schema_migrations
  UNION ALL
  SELECT 'pricing dirty', 0, dirty::int FROM public.pricing_schema_migrations
  UNION ALL
  SELECT 'parcel dirty', 0, dirty::int FROM public.parcel_schema_migrations
  UNION ALL
  SELECT 'merchant dirty', 0, dirty::int FROM public.merchant_schema_migrations
  UNION ALL
  SELECT 'notification dirty', 0, dirty::int FROM public.notification_schema_migrations
)
SELECT check_name, expected, actual FROM checks;
SQL
)"

container_url() {
  printf '%s' "$1" | sed -E 's#@(localhost|127\.0\.0\.1|\[::1\])([:/])#@host.docker.internal\2#'
}

if [ -n "${DATABASE_URL_DIRECT:-}" ]; then
  if [ "${VERIFY_USE_HOST_CLIENT:-}" = "1" ] && command -v psql >/dev/null 2>&1; then
    if ! result="$(printf '%s\n' "$sql" | PGOPTIONS="-c default_transaction_read_only=on" \
      psql "$DATABASE_URL_DIRECT" -X -v ON_ERROR_STOP=1 -At -F '|' -f -)"; then
      printf 'error: remote database verification query failed.\n' >&2
      exit 1
    fi
  elif ! result="$(printf '%s\n' "$sql" | docker run --rm -i --add-host host.docker.internal:host-gateway \
    -e PGURL="$(container_url "${DATABASE_URL_DIRECT}")" postgres:16-alpine \
    sh -c 'PGOPTIONS="-c default_transaction_read_only=on" psql "$PGURL" -v ON_ERROR_STOP=1 -At -F "|" -f -')"; then
    printf 'error: remote database verification query failed.\n' >&2
    exit 1
  fi
elif ! result="$(printf '%s\n' "$sql" | $COMPOSE exec -T postgres sh -c 'PGOPTIONS="-c default_transaction_read_only=on" psql -v ON_ERROR_STOP=1 -At -F "|" -U "$POSTGRES_USER" -d "$POSTGRES_DB"')"; then
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
