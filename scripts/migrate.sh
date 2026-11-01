#!/usr/bin/env bash
# Docker-only local migration runner, also usable from Git Bash on Windows.
set -euo pipefail
export MSYS_NO_PATHCONV=1
cd "$(dirname "$0")/.."

query() {
  docker compose exec -T -e MIGRATION_DATABASE_NAME postgres sh -c \
    'exec psql -v ON_ERROR_STOP=1 -At -U "$POSTGRES_USER" -d "${MIGRATION_DATABASE_NAME:-$POSTGRES_DB}"' <<< "$1"
}

if [ -n "${MIGRATION_DATABASE_NAME:-}" ] && ! [[ "$MIGRATION_DATABASE_NAME" =~ ^[a-zA-Z0-9_]+$ ]]; then
  echo 'MIGRATION_DATABASE_NAME must be a plain database identifier.' >&2; exit 1
fi

query 'CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS $$ BEGIN NEW.updated_at := now(); RETURN NEW; END; $$ LANGUAGE plpgsql;' >/dev/null
services=("$@")
if [ "${#services[@]}" -eq 0 ]; then services=(identity pricing parcel merchant notification); fi
for service in "${services[@]}"; do
  case "$service" in identity|pricing|parcel|merchant|notification) ;; *) echo 'Unknown migration service' >&2; exit 1 ;; esac
  history=canonical
  if [ "$service" = merchant ]; then
    version_table=$(query "SELECT to_regclass('public.merchant_schema_migrations');")
    if [ -n "$version_table" ]; then
      metadata=$(query 'SELECT version::text || chr(124) || dirty::text FROM public.merchant_schema_migrations;')
      case "$metadata" in
        *'|true') echo 'Merchant migration state is dirty; investigate without force/reset.' >&2; exit 1 ;;
        '3|false'|'4|false'|'5|false') history=legacy ;;
        '2|false')
          tables=$(query "SELECT count(*) FROM information_schema.tables WHERE table_schema='merchant' AND table_type='BASE TABLE';")
          case "$tables" in 1) history=legacy ;; 7) history=canonical ;; *) echo 'Ambiguous Merchant version 2 schema; refusing to guess.' >&2; exit 1 ;; esac
          ;;
        '1|false'|'6|false'|'') ;;
        *) echo 'Unsupported Merchant migration metadata; refusing to guess.' >&2; exit 1 ;;
      esac
    fi
  fi
  printf 'Migrating %s (%s history)\n' "$service" "$history"
  docker compose run --rm -e "MIGRATION_SERVICE=$service" -e "MIGRATION_HISTORY=$history" -e MIGRATION_DATABASE_NAME \
    --entrypoint /bin/sh migrate -c '
      set -eu
      if [ -n "${MIGRATION_DATABASE_NAME:-}" ]; then
        DATABASE_URL="${DATABASE_URL%/*}/$MIGRATION_DATABASE_NAME?sslmode=disable"
      fi
      source="/migrations/$MIGRATION_SERVICE"
      if [ "$MIGRATION_HISTORY" = legacy ]; then
        source=/tmp/merchant-migrations
        mkdir -p "$source"
        cp /migrations/merchant/legacy/*.sql "$source/"
        cp /migrations/merchant/000006_*.sql "$source/"
      fi
      exec migrate -path "$source" -database "${DATABASE_URL}&x-migrations-table=${MIGRATION_SERVICE}_schema_migrations" up
    '
done
