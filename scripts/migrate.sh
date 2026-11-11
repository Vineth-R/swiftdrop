#!/bin/sh
# Applies, inspects or rolls back every service's migrations inside the pinned
# migrate container. DATABASE_URL_DIRECT is preferred for destructive and
# schema operations; DATABASE_URL remains the fallback for local development.
# The URL is never echoed so credentials cannot leak into logs.
set -eu

url="${DATABASE_URL_DIRECT:-${DATABASE_URL:-}}"

direction="${1:-up}"
case "$direction" in
    up | down | status | create) ;;
    *)
        echo "usage: $0 [up|down|status|create]" >&2
        exit 2
        ;;
esac

services="${MIGRATE_SERVICES:-identity pricing parcel merchant notification}"
case "$url" in
    *\?*) separator='&' ;;
    *) separator='?' ;;
esac

if [ "$direction" = "create" ]; then
    service="${2:-}"
    name="${3:-}"
    case " $services " in
        *" $service "*) ;;
        *)
            echo "error: service must be one of: $services" >&2
            exit 2
            ;;
    esac
    case "$name" in
        '' | *[!a-z0-9_]*)
            echo "error: migration name must contain only lowercase letters, digits and underscores" >&2
            exit 2
            ;;
    esac
    exec migrate create -ext sql -dir "/migrations/$service" -seq "$name"
fi

if [ "$direction" = "down" ]; then
    if [ -z "${APP_ENV:-}" ]; then
        echo "error: APP_ENV must be explicitly set before rolling back migrations" >&2
        exit 1
    fi
    if [ "${APP_ENV:-}" = "production" ] && [ "${ALLOW_PRODUCTION_ROLLBACK:-}" != "1" ]; then
        echo "error: refusing to roll back migrations with APP_ENV=production; set ALLOW_PRODUCTION_ROLLBACK=1 only after an approved rollback plan" >&2
        exit 1
    fi
    services="notification merchant parcel pricing identity"
fi

if [ -z "$url" ]; then
    echo "error: DATABASE_URL_DIRECT or DATABASE_URL is required" >&2
    exit 2
fi

local_max_version() {
    find "/migrations/$1" -maxdepth 1 -name '*.up.sql' 2>/dev/null |
        sed 's#.*/##' | cut -d_ -f1 | sed 's/^0*//' | sort -n | tail -n1
}

current_version() {
    # Reading the version only needs the database. An empty source directory
    # avoids failing when a legacy version has no file in the current tree.
    mkdir -p /tmp/migrate-version
    migrate -path /tmp/migrate-version -database "$1" version 2>&1 |
        sed -n 's/^\([0-9][0-9]*\).*/\1/p'
}

status=0
for service in $services; do
    database_url="${url}${separator}x-migrations-table=${service}_schema_migrations"
    local_max="$(local_max_version "$service")"
    [ -n "$local_max" ] || local_max=0
    case "$direction" in
        up)
            current="$(current_version "$database_url")"
            if [ -n "$current" ] && [ "$current" -gt "$local_max" ]; then
                printf 'warning: %s database version %s is ahead of local migrations (%s); skipping up to avoid rewriting history\n' "$service" "$current" "$local_max" >&2
                continue
            fi
            echo "migrating ${service} (up)" >&2
            migrate -path "/migrations/${service}" -database "$database_url" up
            ;;
        down)
            echo "migrating ${service} (down)" >&2
            migrate -path "/migrations/${service}" -database "$database_url" down 1
            ;;
        status)
            output="$(current_version "$database_url")"
            if [ -z "$output" ]; then
                printf '%s: no applied migrations or unreadable version\n' "$service"
                status=1
                continue
            fi
            printf '%s: %s\n' "$service" "$output"
            current="$output"
            if [ -n "$current" ] && [ "$current" -gt "$local_max" ]; then
                printf 'warning: %s database version %s is ahead of local migrations (%s)\n' "$service" "$current" "$local_max" >&2
            fi
            ;;
    esac
done

exit "$status"
