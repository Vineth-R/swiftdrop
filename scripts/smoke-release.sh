#!/usr/bin/env bash
# Release smoke test: register, verify, log in, refresh and log out through the
# public gateway. Registration requires email verification, so the script reads
# the single-use link from Mailpit. It never prints passwords, tokens or the
# verification link.
set -euo pipefail

BASE_URL="${BASE_URL:-http://localhost:8080}"
MAILPIT_URL="${MAILPIT_URL:-http://localhost:8025}"
WORK="$(mktemp -d)"
JAR="${WORK}/cookies.txt"
trap 'rm -rf "${WORK}"' EXIT

fail() {
    printf 'FAIL  %s\n' "$1" >&2
    exit 1
}
pass() { printf 'PASS  %s\n' "$1"; }

expect_status() {
    local want="$1" got="$2" label="$3"
    if [ "$got" != "$want" ]; then
        fail "${label} returned ${got}, want ${want}"
    fi
    pass "${label}"
}

command -v jq >/dev/null 2>&1 || fail "jq is required"
command -v curl >/dev/null 2>&1 || fail "curl is required"

suffix="$(date +%s)-$$"
email="release-smoke-${suffix}@example.test"
password="ReleaseSmoke1!"

register_body="$(jq -nc --arg email "$email" --arg password "$password" \
    '{email:$email,password:$password,fullName:"Release Smoke",phone:"+94770000000"}')"
status="$(curl -sS -o "${WORK}/register.json" -w '%{http_code}' \
    -H 'Content-Type: application/json' -d "$register_body" \
    "${BASE_URL}/api/v1/auth/register")"
expect_status 201 "$status" "registration"

token=""
for _ in $(seq 1 30); do
    message_id="$(curl -sS "${MAILPIT_URL}/api/v1/search?query=to:${email}" | jq -r '.messages[0].ID // empty')"
    if [ -n "$message_id" ]; then
        link="$(curl -sS "${MAILPIT_URL}/api/v1/message/${message_id}" |
            jq -r '(.Text // "") + "\n" + (.HTML // "")' |
            grep -o 'verify-email?token=[^"<> ]*' | head -n 1 || true)"
        if [ -n "$link" ]; then
            token="${link#verify-email?token=}"
            token="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.unquote(sys.argv[1]))' "$token")"
            break
        fi
    fi
    sleep 1
done
[ -n "$token" ] || fail "verification email did not arrive"

verify_body="$(jq -nc --arg token "$token" '{token:$token}')"
status="$(curl -sS -o "${WORK}/verify.json" -w '%{http_code}' \
    -H 'Content-Type: application/json' -d "$verify_body" \
    "${BASE_URL}/api/v1/auth/verify-email")"
expect_status 200 "$status" "email verification"

login_body="$(jq -nc --arg email "$email" --arg password "$password" \
    '{email:$email,password:$password}')"
status="$(curl -sS -c "$JAR" -b "$JAR" -o "${WORK}/login.json" -w '%{http_code}' \
    -H 'Content-Type: application/json' -d "$login_body" \
    "${BASE_URL}/api/v1/auth/login")"
expect_status 200 "$status" "login"

status="$(curl -sS -c "$JAR" -b "$JAR" -o "${WORK}/refresh.json" -w '%{http_code}' \
    -X POST "${BASE_URL}/api/v1/auth/refresh")"
expect_status 200 "$status" "refresh"

status="$(curl -sS -c "$JAR" -b "$JAR" -o /dev/null -w '%{http_code}' \
    -X POST "${BASE_URL}/api/v1/auth/logout")"
case "$status" in
    200 | 204) pass "logout" ;;
    *) fail "logout returned ${status}, want 200 or 204" ;;
esac

printf 'Release auth smoke passed.\n'
