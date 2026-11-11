#!/bin/sh
# Creates a verified custom-format backup outside the repository. Never commit
# dumps. The source URL is never printed.
set -eu

root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
compose="${COMPOSE:-docker compose}"
output_dir="${BACKUP_DIR:-${HOME}/swiftdrop-backups}"
timestamp="$(date -u +%Y%m%dT%H%M%SZ)"
output="${output_dir}/swiftdrop-${timestamp}.dump"
partial="${output}.partial"

source="${BACKUP_SOURCE:-}"
if [ "$source" = "compose" ] || { [ -z "$source" ] && [ -z "${DATABASE_URL:-}" ]; }; then
    source="compose"
elif [ "$source" = "url" ] || [ -n "${DATABASE_URL:-}" ]; then
    source="url"
else
    echo "error: BACKUP_SOURCE must be compose or url" >&2
    exit 2
fi

mkdir -p "${output_dir}"
chmod 700 "${output_dir}"

if [ "$source" = "compose" ]; then
    ${compose} exec -T postgres pg_dump \
        -U "${POSTGRES_USER:-swiftdrop}" -d "${POSTGRES_DB:-swiftdrop}" \
        -Fc --no-owner --no-privileges >"${partial}"
else
    if [ -z "${DATABASE_URL:-}" ]; then
        echo "error: DATABASE_URL is required for BACKUP_SOURCE=url" >&2
        exit 2
    fi
    docker run --rm -i -e PGURL="${DATABASE_URL}" postgres:16-alpine \
        sh -c 'pg_dump "$PGURL" -Fc --no-owner --no-privileges' >"${partial}"
fi

mv "${partial}" "${output}"
chmod 600 "${output}"

if ! docker run --rm -v "${output_dir}:/backups:ro" postgres:16-alpine \
    pg_restore --list "/backups/$(basename "${output}")" >/dev/null; then
    echo "error: backup verification failed; keeping the file for inspection" >&2
    exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
    ( cd "${output_dir}" && sha256sum "$(basename "${output}")" >"$(basename "${output}").sha256" )
else
    ( cd "${output_dir}" && shasum -a 256 "$(basename "${output}")" >"$(basename "${output}").sha256" )
fi

echo "backup: ${output}"
echo "checksum: ${output}.sha256"
echo "verified: pg_restore --list succeeded"
