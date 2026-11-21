#!/bin/sh
# Seeds the complete local demo data set through the Docker Compose network.
#
# The five demo accounts (make seed), the approved merchant and its API key
# (make seed-merchant), and one parcel per canonical status (make seed-parcels)
# are created in order. Every step is idempotent.
#
# The seed commands run in a Go container attached to the Compose network
# because PostgreSQL is deliberately not published to the host. Secrets are
# passed through the environment, never written to the repository or logs.
set -eu

compose="${COMPOSE:-docker compose}"
root="$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)"
cd "$root"

fail() {
    printf 'error: %s\n' "$1" >&2
    exit 1
}

[ -f .env ] || fail "'.env' is missing. Run 'cp .env.example .env' and configure it."
[ -n "${SEED_MERCHANT_PASSWORD:-}" ] || fail "SEED_MERCHANT_PASSWORD is required for make seed-demo."
[ -n "${SEED_MERCHANT_API_KEY:-}" ] || fail "SEED_MERCHANT_API_KEY is required for make seed-demo. Generate one with: printf 'sdk_live_%s_%s\\n' \"\$(openssl rand -hex 8)\" \"\$(openssl rand -hex 24)\""

postgres_id="$($compose ps -q postgres 2>/dev/null || true)"
[ -n "$postgres_id" ] || fail "postgres is not running. Start the stack with 'make up' or 'make up-build'."

env_value() {
    docker inspect -f '{{range .Config.Env}}{{println .}}{{end}}' "$postgres_id" |
        sed -n "s/^$1=//p" | head -n 1
}

postgres_user="$(env_value POSTGRES_USER)"
postgres_password="$(env_value POSTGRES_PASSWORD)"
postgres_db="$(env_value POSTGRES_DB)"
[ -n "$postgres_user" ] && [ -n "$postgres_db" ] || fail "could not read the PostgreSQL credentials from the running container."
database_url="postgres://${postgres_user}:${postgres_password}@postgres:5432/${postgres_db}?sslmode=disable"

network="$(docker inspect -f '{{range $name, $_ := .NetworkSettings.Networks}}{{$name}}{{end}}' "$postgres_id")"
[ -n "$network" ] || fail "could not resolve the Compose network for postgres."

run_seed() {
    docker run --rm --network "$network" \
        --env-file "$root/.env" \
        -v "$root:/src:ro" -w /src \
        -e GOTOOLCHAIN=local \
        -e DATABASE_URL="$database_url" \
        -e IDENTITY_BASE_URL="http://identity:8081" \
        -e ALLOW_REMOTE_SEED=1 \
        -e SEED_ADMIN_PASSWORD="${SEED_ADMIN_PASSWORD:-}" \
        -e SEED_DISPATCHER_PASSWORD="${SEED_DISPATCHER_PASSWORD:-}" \
        -e SEED_COURIER_1_PASSWORD="${SEED_COURIER_1_PASSWORD:-}" \
        -e SEED_COURIER_2_PASSWORD="${SEED_COURIER_2_PASSWORD:-}" \
        -e SEED_CUSTOMER_PASSWORD="${SEED_CUSTOMER_PASSWORD:-}" \
        -e SEED_MERCHANT_PASSWORD="${SEED_MERCHANT_PASSWORD:-}" \
        -e SEED_MERCHANT_API_KEY="${SEED_MERCHANT_API_KEY:-}" \
        golang:1.27-alpine \
        go run "$1"
}

printf 'Seeding identity demo accounts...\n'
run_seed ./apps/services/identity/seed
printf 'Seeding the approved merchant and demo API key...\n'
run_seed ./apps/services/merchant/seed
printf 'Seeding demo parcels for every status...\n'
run_seed ./apps/services/parcel/seed
printf 'Demo seed complete. The API key is the value you supplied in SEED_MERCHANT_API_KEY.\n'
