#!/bin/sh
# Prunes audit logs and webhook delivery attempts according to the approved
# retention policy. Dry-run is the default. Set APPLY=1 to delete rows.
#
# This script never touches notification logs (the notification worker owns
# that cleanup), processed events, merchant orders, reservations, parcel data,
# or proof objects.
#
# Defaults:
#   AUDIT_RETENTION_DAYS=365
#   WEBHOOK_ATTEMPT_RETENTION_DAYS=90
#
# Use BACKUP_SOURCE=compose (default when DATABASE_URL is unset) or
# BACKUP_SOURCE=url with DATABASE_URL. The URL is never printed.
set -eu

audit_days="${AUDIT_RETENTION_DAYS:-365}"
attempt_days="${WEBHOOK_ATTEMPT_RETENTION_DAYS:-90}"
apply="${APPLY:-0}"

case "$audit_days" in ''|*[!0-9]*) echo "error: AUDIT_RETENTION_DAYS must be a positive integer" >&2; exit 2;; esac
case "$attempt_days" in ''|*[!0-9]*) echo "error: WEBHOOK_ATTEMPT_RETENTION_DAYS must be a positive integer" >&2; exit 2;; esac
if [ "$audit_days" -lt 30 ] || [ "$attempt_days" -lt 30 ]; then
    echo "error: retention windows below 30 days are refused; confirm the policy" >&2
    exit 2
fi
case "$apply" in 0|1) ;; *) echo "error: APPLY must be 0 or 1" >&2; exit 2;; esac

sql="$(cat <<SQL
WITH candidates AS (
  SELECT 'identity.audit_logs' AS table_name, count(*)::bigint AS rows
  FROM identity.audit_logs
  WHERE created_at < now() - interval '${audit_days} days'
  UNION ALL
  SELECT 'merchant.audit_logs', count(*)::bigint
  FROM merchant.audit_logs
  WHERE created_at < now() - interval '${audit_days} days'
  UNION ALL
  SELECT 'pricing.audit_logs', count(*)::bigint
  FROM pricing.audit_logs
  WHERE created_at < now() - interval '${audit_days} days'
  UNION ALL
  SELECT 'merchant.webhook_delivery_attempts', count(*)::bigint
  FROM merchant.webhook_delivery_attempts
  WHERE created_at < now() - interval '${attempt_days} days'
)
SELECT table_name, rows FROM candidates ORDER BY table_name;
SQL
)"

if [ "$apply" = "1" ]; then
    sql="${sql}
DELETE FROM identity.audit_logs WHERE created_at < now() - interval '${audit_days} days';
DELETE FROM merchant.audit_logs WHERE created_at < now() - interval '${audit_days} days';
DELETE FROM pricing.audit_logs WHERE created_at < now() - interval '${audit_days} days';
DELETE FROM merchant.webhook_delivery_attempts WHERE created_at < now() - interval '${attempt_days} days';
"
fi

source="${BACKUP_SOURCE:-}"
if [ "$source" = "compose" ] || { [ -z "$source" ] && [ -z "${DATABASE_URL:-}" ]; }; then
    ${COMPOSE:-docker compose} exec -T postgres psql \
        -U "${POSTGRES_USER:-swiftdrop}" -d "${POSTGRES_DB:-swiftdrop}" \
        -v ON_ERROR_STOP=1 -X <<SQL
${sql}
SQL
else
    if [ -z "${DATABASE_URL:-}" ]; then
        echo "error: DATABASE_URL is required for BACKUP_SOURCE=url" >&2
        exit 2
    fi
    docker run --rm -i -e PGURL="${DATABASE_URL}" postgres:16-alpine \
        sh -c 'psql "$PGURL" -v ON_ERROR_STOP=1 -X -f -' <<SQL
${sql}
SQL
fi

if [ "$apply" = "1" ]; then
    echo "retention prune applied: audit=${audit_days}d webhook_attempts=${attempt_days}d"
else
    echo "dry run only; set APPLY=1 to delete the reported rows"
fi
