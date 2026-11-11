#!/bin/sh
# Restores a verified custom-format backup into DATABASE_URL_DIRECT.
# Restoration, proof transfer and production cutover are separate
# approval-gated operations. This script never creates a backup and never
# deletes the target unless RESTORE_CLEAN=1 is explicitly set.
set -eu

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

docker run --rm -v "${backup_dir}:/backups:ro" -e PGURL="${DATABASE_URL_DIRECT}" postgres:16-alpine \
    sh -c "pg_restore -d \"\$PGURL\" ${args} /backups/${backup_name}"

echo "restore complete: ${backup_name}"
echo "next: run 'DATABASE_URL_DIRECT=... make migrate-status' and confirm the restored versions are clean."
echo "do not run 'make migrate' again on a restored database unless migrate-status reports a lower version."
