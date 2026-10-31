#!/bin/sh
# Applies or rolls back every service's migrations inside the migrate container.
# DATABASE_URL is provided by Compose from the configured environment; it is
# never echoed so credentials cannot leak into logs.
set -eu

: "${DATABASE_URL:?DATABASE_URL is required}"

direction="${1:-up}"
case "$direction" in
    up | down) ;;
    *)
        echo "usage: $0 [up|down]" >&2
        exit 2
        ;;
esac

if [ "$direction" = "up" ]; then
    services="identity pricing parcel merchant notification"
else
    services="notification merchant parcel pricing identity"
fi

case "$DATABASE_URL" in
    *\?*) separator='&' ;;
    *) separator='?' ;;
esac

for service in $services; do
    echo "migrating ${service} (${direction})"
    database_url="${DATABASE_URL}${separator}x-migrations-table=${service}_schema_migrations"
    if [ "$direction" = "up" ]; then
        migrate -path "/migrations/${service}" -database "$database_url" up
    else
        migrate -path "/migrations/${service}" -database "$database_url" down 1
    fi
done
