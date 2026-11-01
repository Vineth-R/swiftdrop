#!/usr/bin/env bash
# Uses only uniquely named databases created by this test; never resets app data.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/.."
databases=()
cleanup() {
  for database in "${databases[@]}"; do
    if [[ "$database" =~ ^swiftdrop_migration_test_[0-9_]+$ ]]; then
      printf 'DROP DATABASE "%s";\n' "$database" | docker compose exec -T postgres sh -c \
        'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null
    fi
  done
}
trap cleanup EXIT
query() {
  docker compose exec -T -e MIGRATION_DATABASE_NAME postgres sh -c \
    'exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "$MIGRATION_DATABASE_NAME"' <<< "$1"
}
for historical in migrations/merchant/legacy/*.sql; do
  file=$(basename "$historical")
  expected=$(awk -v filename="$file" '{ sub(/\r$/, ""); if ($2 == filename) print $1 }' migrations/merchant/legacy/git-blob-hashes.txt)
  test -n "$expected" || { echo "Missing historical migration hash: $file" >&2; exit 1; }
  actual=$(git hash-object --path="migrations/merchant/$file" "$historical")
  test "$expected" = "$actual" || { echo "Historical migration changed: $file" >&2; exit 1; }
done
for scenario in canonical legacy2 legacy5; do
  database="swiftdrop_migration_test_${RANDOM}_$$"
  printf 'CREATE DATABASE "%s";\n' "$database" | docker compose exec -T postgres sh -c \
    'exec psql -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null
  databases+=("$database")
  export MIGRATION_DATABASE_NAME="$database"
  query 'CREATE FUNCTION public.set_updated_at() RETURNS trigger AS $$ BEGIN NEW.updated_at := now(); RETURN NEW; END; $$ LANGUAGE plpgsql;' >/dev/null
  if [ "$scenario" != canonical ]; then
    steps=5
    if [ "$scenario" = legacy2 ]; then steps=2; fi
    docker compose run --rm -e MIGRATION_DATABASE_NAME -e "MIGRATION_STEPS=$steps" --entrypoint /bin/sh migrate -c '
      exec migrate -path /migrations/merchant/legacy -database "${DATABASE_URL%/*}/$MIGRATION_DATABASE_NAME?sslmode=disable&x-migrations-table=merchant_schema_migrations" up "$MIGRATION_STEPS"
    '
    # Preserve an existing legacy row across the real upgrade.
    query "INSERT INTO merchant.merchants(id,owner_user_id,business_name,contact_email,contact_phone,pickup_address,pickup_zone_id)
      VALUES(gen_random_uuid(),gen_random_uuid(),'migration fixture','fixture@example.invalid','fixture','fixture',gen_random_uuid());" >/dev/null
  fi
  bash scripts/migrate.sh merchant
  test "$(query 'SELECT version::text || chr(124) || dirty::text FROM public.merchant_schema_migrations;')" = '6|false'
  test "$(query "SELECT count(*) FROM information_schema.tables WHERE table_schema='merchant' AND table_type='BASE TABLE';")" = 7
  if [ "$scenario" != canonical ]; then test "$(query 'SELECT count(*) FROM merchant.merchants;')" = 1; fi
  test "$(query "SELECT count(*) FROM information_schema.columns WHERE table_schema='merchant' AND column_name='id' AND column_default IS NOT NULL;")" = 7
  bash scripts/migrate.sh merchant
  printf 'PASS merchant migration scenario: %s\n' "$scenario"
done
