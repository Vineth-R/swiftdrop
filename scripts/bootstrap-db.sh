#!/bin/sh
# Creates the five service schemas and public.set_updated_at(). This is
# idempotent and must run before migrations on a fresh database.
#
# DATABASE_URL_DIRECT targets a remote database (Neon); otherwise the local
# Compose postgres service is used. The URL is passed through the container
# environment and is never echoed.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
bootstrap="${root}/infra/docker/postgres/init/01-schemas.sql"
compose="${COMPOSE:-docker compose}"

if [ -n "${DATABASE_URL_DIRECT:-}" ]; then
    exec docker run --rm -i \
        -e PGURL="${DATABASE_URL_DIRECT}" \
        postgres:16-alpine \
        sh -c 'psql "$PGURL" -X -v ON_ERROR_STOP=1 -f -' <"${bootstrap}"
fi

${compose} exec -T postgres \
    sh -c 'psql -X -v ON_ERROR_STOP=1 -U "$POSTGRES_USER" -d "$POSTGRES_DB" -f -' <"${bootstrap}"
