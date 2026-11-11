#!/usr/bin/env bash
# Verifies that every public API v1.1 path group reaches its service through Nginx.
set -u

if ! command -v curl >/dev/null 2>&1; then
  printf 'error: curl is required to verify gateway routes.\n' >&2
  exit 1
fi

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
  # JSON error: 404 for unimplemented paths, 401/403 for auth-protected paths,
  # and 405 with the upstream method-not-allowed code. Nginx-generated errors
  # are HTML and do not carry a JSON error code.
  if { [ "$code" = "404" ] && printf '%s' "$body" | grep -q '"code"'; } || \
     { [ "$code" = "405" ] && printf '%s' "$body" | grep -q '"code":"METHOD_NOT_ALLOWED"'; } || \
     [ "$code" = "401" ] || [ "$code" = "403" ]; then
    pass "$description ($path -> upstream $code)"
  else
    fail "$description ($path -> $code, expected an upstream JSON error)"
  fi
}

check_ai_chat_route() {
  local path="$1" description="$2"
  local body code
  # An empty message is rejected by the AI service itself, so a 422 proves the
  # gateway reached the upstream without needing a configured model provider.
  body="$(curl -s --max-time 5 -w '\n%{http_code}' \
    -X POST "$BASE_URL$path" \
    -H 'Content-Type: application/json' \
    -d '{"message":"","conversationId":null}' || true)"
  code="$(printf '%s' "$body" | tail -n1)"
  if [ "$code" = "422" ] && printf '%s' "$body" | grep -q '"code":"VALIDATION_ERROR"'; then
    pass "$description ($path -> upstream $code)"
  else
    fail "$description ($path -> $code, expected upstream 422 VALIDATION_ERROR)"
  fi
}

check_notification_admin_route() {
  local path="$1" description="$2"
  local body code
  body="$(curl -s --max-time 5 -w '\n%{http_code}' "$BASE_URL$path" || true)"
  code="$(printf '%s' "$body" | tail -n1)"
  # The route is deliberately fail-closed with 503 when JWT_ACCESS_SECRET is
  # unset. Both 401 and 503 are valid upstream JSON responses; connection
  # failures, gateway errors and unexpected statuses still fail the check.
  if { [ "$code" = "401" ] || [ "$code" = "503" ]; } && printf '%s' "$body" | grep -q '"code"'; then
    pass "$description ($path -> upstream $code)"
  else
    fail "$description ($path -> $code, expected upstream 401 or 503 JSON error)"
  fi
}

# A redirect the gateway makes itself must stay on the address the caller used:
# a relative Location, so the port is kept, and the query string carried over.
check_redirect() {
  local path="$1" expected_code="$2" expected_location="$3" description="$4"
  local headers code location
  headers="$(curl -s -o /dev/null -D - --max-time 5 "$BASE_URL$path" | tr -d '\r' || true)"
  code="$(printf '%s' "$headers" | awk 'NR==1 {print $2}')"
  location="$(printf '%s' "$headers" | awk 'tolower($1)=="location:" {print $2}')"
  if [ "$code" = "$expected_code" ] && [ "$location" = "$expected_location" ]; then
    pass "$description ($path -> $code $location)"
  else
    fail "$description ($path -> $code ${location:-no Location}, want $expected_code $expected_location)"
  fi
}

check_status "/healthz" "200" "gateway health"
check_status "/internal/pricing/quote" "404" "internal APIs are blocked"
check_ai_chat_route "/api/v1/ai/chat" "ai-assistant:chat"
check_status "/api/v1/ai/health" "404" "AI health probes are internal"
check_status "/api/v1/ai/ready" "404" "AI readiness probes are internal"
check_status "/" "200" "web frontend"

check_upstream "/api/v1/auth/probe" "identity:auth"
check_upstream "/api/v1/users/probe" "identity:users"
check_upstream "/api/v1/admin/users/probe" "identity:admin-users"
check_notification_admin_route "/api/v1/admin/notification-logs" "notification-worker:admin-notification-logs"
check_notification_admin_route "/api/v1/admin/notification-logs/" "notification-worker:admin-notification-logs"
check_upstream "/api/v1/couriers/probe" "identity:couriers"
check_upstream "/api/v1/pricing/probe" "pricing:pricing"
check_upstream "/api/v1/zones/probe" "pricing:zones"
check_upstream "/api/v1/admin/pricing/probe" "pricing:admin-pricing"
check_upstream "/api/v1/admin/zones/probe" "pricing:admin-zones"
check_upstream "/api/v1/parcels/probe" "parcel:parcels"
check_upstream "/api/v1/tracking/probe/probe" "parcel:tracking"
check_upstream "/api/v1/dispatch/probe" "parcel:dispatch"
check_upstream "/api/v1/admin/parcels/probe" "parcel:admin-parcels"
check_upstream "/api/v1/courier/probe" "parcel:courier"
check_upstream "/api/v1/reports/probe" "parcel:reports"
check_upstream "/api/v1/merchants/probe" "merchant:merchants"
check_upstream "/api/v1/merchant/orders/probe" "merchant:merchant"
check_upstream "/api/v1/admin/merchants/probe" "merchant:admin-merchants"
check_status "/ws" "426" "realtime websocket route"

check_redirect "/api/v1/couriers" "308" "/api/v1/couriers/" "no-slash redirect is relative"
check_redirect "/api/v1/couriers?isAvailable=true&page=2" "308" "/api/v1/couriers/?isAvailable=true&page=2" "no-slash redirect keeps the query"
check_redirect "/api/v1/merchants" "308" "/api/v1/merchants/" "merchants no-slash redirect is relative"
check_redirect "/api/v1/dispatch" "301" "/api/v1/dispatch/" "nginx's own slash redirect is relative"

if [ "$failures" -ne 0 ]; then
  printf '\n%d route check(s) failed\n' "$failures"
  exit 1
fi

printf '\nall route checks passed\n'
