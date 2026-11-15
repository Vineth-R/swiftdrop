#!/usr/bin/env bash
set -euo pipefail

# Runs against the disposable real-stack Compose project. It stops RabbitMQ,
# proves consumer/relay-aware readiness fails, restarts the broker, then drives
# one real status event through the recovered stack.
project="${COMPOSE_PROJECT_NAME:?COMPOSE_PROJECT_NAME is required}"
compose=(docker compose -p "${project}")

ready() {
  "${compose[@]}" exec -T "$1" wget -qO- "http://127.0.0.1:$2/ready" >/dev/null 2>&1
}

wait_for() {
  local want="$1"
  shift
  local deadline=$((SECONDS + 120))
  while (( SECONDS < deadline )); do
    local ok=1
    for service in "$@"; do
      if ready "${service%%:*}" "${service##*:}"; then :; else ok=0; fi
    done
    if [[ "${want}" == "up" && "${ok}" == "1" ]]; then return 0; fi
    if [[ "${want}" == "down" && "${ok}" == "0" ]]; then return 0; fi
    sleep 1
  done
  echo "timed out waiting for services to be ${want}: $*" >&2
  return 1
}

services=(parcel:8082 merchant:8084 realtime:8085 notification-worker:8086 webhook-worker:8088)
unaffected=(identity:8081 pricing:8083)

wait_for up "${services[@]}" "${unaffected[@]}"
echo "baseline readiness: ok"

"${compose[@]}" stop rabbitmq >/dev/null
wait_for down "${services[@]}"
echo "RabbitMQ outage: dependent readiness failed"
for service in "${unaffected[@]}"; do
  ready "${service%%:*}" "${service##*:}"
done
echo "identity/pricing stayed ready"

"${compose[@]}" start rabbitmq >/dev/null
wait_for up "${services[@]}" "${unaffected[@]}"
echo "RabbitMQ recovery: all readiness checks passed"

REAL_STACK_BASE_URL="${REAL_STACK_BASE_URL:-http://localhost:8080}" \
  node tests/recovery/delivery-flow.mjs
echo "post-recovery status event: ok"
