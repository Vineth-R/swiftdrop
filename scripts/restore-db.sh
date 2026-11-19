#!/bin/sh
# Restores a verified custom-format backup into DATABASE_URL_DIRECT.
# Restoration, proof transfer and production cutover are separate
# approval-gated operations. This script never creates a backup and never
# deletes the target unless RESTORE_CLEAN=1 is explicitly set.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
backup="${1:-${BACKUP:-}}"
if [ -z "${backup}" ]; then
    echo "usage: $0 /path/to/swiftdrop-<timestamp>.dump" >&2
    exit 2
fi
if [ ! -f "${backup}" ]; then
    echo "error: backup file not found: ${backup}" >&2
    exit 2
fi
if [ -z "${DATABASE_URL_DIRECT:-}" ]; then
    echo "error: DATABASE_URL_DIRECT is required for a restore target" >&2
    exit 2
fi
if [ -z "${APP_ENV:-}" ]; then
    echo "error: APP_ENV must be explicitly set before restoring a database" >&2
    exit 1
fi
case "${APP_ENV}" in
    development | staging | production) ;;
    *)
        echo "error: APP_ENV must be development, staging, or production" >&2
        exit 1
        ;;
esac
if [ "${APP_ENV}" = "production" ] && [ "${ALLOW_PRODUCTION_RESTORE:-}" != "1" ]; then
    echo "error: refusing to restore with APP_ENV=production; set ALLOW_PRODUCTION_RESTORE=1 only after an approved cutover plan" >&2
    exit 1
fi

backup_dir="$(CDPATH= cd -- "$(dirname -- "${backup}")" && pwd)"
backup_name="$(basename -- "${backup}")"

container_url() {
    printf '%s' "$1" | sed -E 's#@(localhost|127\.0\.0\.1|\[::1\])([:/])#@host.docker.internal\2#'
}

if [ "${RESTORE_CLEAN:-}" != "1" ] && [ "${RESTORE_ALLOW_NONEMPTY:-}" != "1" ]; then
    target_sql="SELECT count(*) FROM information_schema.tables WHERE table_schema NOT IN ('pg_catalog','information_schema')"
    if command -v psql >/dev/null 2>&1; then
        target_objects="$(psql "$DATABASE_URL_DIRECT" -X -At -v ON_ERROR_STOP=1 -c "$target_sql")" || {
            echo "error: could not inspect the restore target; refusing an unchecked restore" >&2
            exit 1
        }
    else
        target_objects="$(docker run --rm --add-host host.docker.internal:host-gateway \
            -e PGURL="$(container_url "${DATABASE_URL_DIRECT}")" -e TARGET_SQL="$target_sql" postgres:16-alpine \
            sh -c 'psql "$PGURL" -X -At -v ON_ERROR_STOP=1 -c "$TARGET_SQL"')" || {
            echo "error: could not inspect the restore target; refusing an unchecked restore" >&2
            exit 1
        }
    fi
    if [ "$target_objects" != "0" ]; then
        echo "error: the restore target already contains $target_objects user table(s); refusing to overwrite" >&2
        echo "       use an empty disposable database, RESTORE_CLEAN=1 for an approved overwrite," >&2
        echo "       or RESTORE_ALLOW_NONEMPTY=1 for an intentional additive restore" >&2
        exit 1
    fi
fi

if [ -f "${backup}.sha256" ]; then
    if command -v sha256sum >/dev/null 2>&1; then
        ( cd "${backup_dir}" && sha256sum --check "$(basename -- "${backup}").sha256" )
    else
        ( cd "${backup_dir}" && shasum -a 256 --check "$(basename -- "${backup}").sha256" )
    fi
fi

args="--no-owner --no-privileges --single-transaction --exit-on-error"
if [ "${RESTORE_CLEAN:-}" = "1" ]; then
    args="${args} --clean --if-exists"
fi

if [ "${RESTORE_USE_HOST_CLIENT:-}" = "1" ] && command -v pg_restore >/dev/null 2>&1; then
    PGURL="${DATABASE_URL_DIRECT}" BACKUP_PATH="${backup}" sh -c \
        "pg_restore -d \"\$PGURL\" ${args} \"\$BACKUP_PATH\""
else
    docker run --rm --add-host host.docker.internal:host-gateway \
        -v "${backup_dir}:/backups:ro" -e PGURL="$(container_url "${DATABASE_URL_DIRECT}")" postgres:16-alpine \
        sh -c "pg_restore -d \"\$PGURL\" ${args} /backups/${backup_name}"
fi

echo "restore complete: ${backup_name}"

if [ "${SKIP_RESTORE_VALIDATION:-}" = "1" ]; then
    echo "warning: restore validation skipped by SKIP_RESTORE_VALIDATION=1" >&2
else
    VERIFY_USE_HOST_CLIENT="${RESTORE_USE_HOST_CLIENT:-}" DATABASE_URL_DIRECT="${DATABASE_URL_DIRECT}" "${root}/scripts/verify-db.sh"
    echo "restore validation passed"
fi

echo "next: run 'DATABASE_URL_DIRECT=... make migrate-status' and confirm the restored versions are clean."
echo "do not run 'make migrate' again on a restored database unless migrate-status reports a lower version."
