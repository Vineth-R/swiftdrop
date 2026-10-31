#!/usr/bin/env bash
# Verifies that every public API v1.1 path group reaches its service through Nginx.
set -u

BASE_URL="${BASE_URL:-http://localhost:8080}"
failures=0

pass() { printf 'PASS  %s\n' "$1"; }
fail() { printf 'FAIL  %s\n' "$1"; failures=$((failures + 1)); }

status_of() {
  curl -s -o /dev/null -w '%{http_code}' --max-time 5 "$BASE_URL$1" || echo "000"
}

check_status() {
  local path="$1" expected="$2" description="$3"
  local code
  code="$(status_of "$path")"
  if [ "$code" = "$expected" ]; then pass "$description ($path -> $code)"; else fail "$description ($path -> $code, want $expected)"; fi
}

check_upstream() {
  local path="$1" description="$2"
  local body code
  body="$(curl -s --max-time 5 -w '\n%{http_code}' "$BASE_URL$path" || true)"
  code="$(printf '%s' "$body" | tail -n1)"
  # A route proves gateway forwarding when the upstream answers with its own
  # JSON error: 404 for unimplemented paths, 401/403 for auth-protected paths.
  if { [ "$code" = "404" ] && printf '%s' "$body" | grep -q '"code"'; } || [ "$code" = "401" ] || [ "$code" = "403" ]; then
    pass "$description ($path -> upstream $code)"
  else
    fail "$description ($path -> $code, expected an upstream JSON error)"
  fi
}

check_status "/healthz" "200" "gateway health"
check_status "/internal/pricing/quote" "404" "internal APIs are blocked"
check_status "/api/v1/ai/chat" "404" "ai service is a TODO"
check_status "/" "200" "web frontend"

check_upstream "/api/v1/auth/probe" "identity:auth"
check_upstream "/api/v1/users/probe" "identity:users"
check_upstream "/api/v1/admin/users/probe" "identity:admin-users"
check_upstream "/api/v1/couriers/probe" "identity:couriers"
check_upstream "/api/v1/pricing/probe" "pricing:pricing"
check_upstream "/api/v1/zones/probe" "pricing:zones"
check_upstream "/api/v1/admin/pricing/probe" "pricing:admin-pricing"
check_upstream "/api/v1/admin/zones/probe" "pricing:admin-zones"
check_upstream "/api/v1/parcels/probe" "parcel:parcels"
check_upstream "/api/v1/tracking/probe" "parcel:tracking"
check_upstream "/api/v1/dispatch/probe" "parcel:dispatch"
check_upstream "/api/v1/admin/parcels/probe" "parcel:admin-parcels"
check_upstream "/api/v1/courier/probe" "parcel:courier"
check_upstream "/api/v1/reports/probe" "parcel:reports"
check_upstream "/api/v1/merchants/probe" "merchant:merchants"
check_upstream "/api/v1/merchant/orders/probe" "merchant:merchant"
check_upstream "/api/v1/admin/merchants/probe" "merchant:admin-merchants"
check_status "/ws" "404" "realtime websocket route"

if [ "$failures" -ne 0 ]; then
  printf '\n%d route check(s) failed\n' "$failures"
  exit 1
fi

printf '\nall route checks passed\n'
