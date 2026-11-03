SHELL := /bin/sh

COMPOSE ?= docker compose
BUILD_SERVICES ?= identity parcel pricing merchant realtime notification-worker webhook-worker web
BUILD_FLAGS ?=

.PHONY: help build up up-build down logs preflight require-images ensure-postgres bootstrap-db migrate migrate-down db-reset seed check-routes test test-db test-access coverage

help:
	@echo "Targets:"
	@echo "  make build          build all application images without starting them"
	@echo "  make up             start existing images, migrate, and verify gateway routes"
	@echo "  make up-build       build images, then run the full make up workflow"
	@echo "  make down           stop the stack without deleting data"
	@echo "  make logs           follow service logs"
	@echo "  make migrate        apply all service migrations"
	@echo "  make seed           create the five deterministic demo users"
	@echo "  make migrate-down   roll back one migration per service (destructive)"
	@echo "  make check-routes   verify gateway routing against the running stack"
	@echo "  make db-reset       delete local volumes and rebuild (destructive)"
	@echo "  make test           run every Go test with the race detector (no database needed)"
	@echo "  make test-db        run PostgreSQL integration tests against TEST_DATABASE_URL"
	@echo "  make test-access    run the cross-service access tests against the running stack"
	@echo "  make coverage       run the Go tests and print coverage per service and in total"

preflight:
	@if [ ! -f .env ] && { [ -z "$${POSTGRES_USER:-}" ] || [ -z "$${POSTGRES_DB:-}" ]; }; then \
		echo "error: .env is missing and POSTGRES_USER/POSTGRES_DB are not exported." >&2; \
		echo "       Run 'cp .env.example .env' and configure it, or export the required variables." >&2; \
		exit 1; \
	fi
	@$(COMPOSE) config --quiet
	@jwt="$$($(COMPOSE) config 2>/dev/null | awk '/^[[:space:]]+JWT_ACCESS_SECRET:/{sub(/^[[:space:]]*JWT_ACCESS_SECRET:[[:space:]]*/, ""); print; exit}')"; \
	jwt="$$(printf '%s' "$$jwt" | sed "s/^[[:space:]]*//;s/[[:space:]]*$$//;s/^[\"']//;s/[\"']$$//;s/^[[:space:]]*//;s/[[:space:]]*$$//")"; \
	if [ -z "$$jwt" ]; then \
		echo "error: JWT_ACCESS_SECRET is required. Set it in .env or export it." >&2; \
		exit 1; \
	fi

require-images:
	@images="$$($(COMPOSE) config --images 2>/dev/null)"; \
	missing=0; \
	for service in $(BUILD_SERVICES); do \
		image="$$(printf '%s\n' "$$images" | grep -E "(^|/).*-$${service}$$" | head -n1)"; \
		if [ -z "$$image" ] || ! docker image inspect "$$image" >/dev/null 2>&1; then \
			echo "error: image for $$service is missing. Run 'make build' or 'make up-build'." >&2; \
			missing=1; \
		fi; \
	done; \
	test "$$missing" -eq 0

ensure-postgres:
	@$(COMPOSE) ps --status running --services 2>/dev/null | grep -qx postgres || { \
		echo "error: postgres is not running. Start it with 'docker compose up -d postgres' or run 'make up'." >&2; \
		exit 1; \
	}

bootstrap-db: ensure-postgres
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -f /docker-entrypoint-initdb.d/01-schemas.sql'

build: preflight
	@for service in $(BUILD_SERVICES); do \
		echo "Building $$service"; \
		$(COMPOSE) build $(BUILD_FLAGS) "$$service" || exit 1; \
	done

up: preflight require-images
	$(COMPOSE) up -d --no-build --wait --wait-timeout 300
	$(MAKE) migrate
	$(MAKE) check-routes

up-build: preflight build
	$(MAKE) up

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100

migrate: preflight bootstrap-db
	$(COMPOSE) run --rm --entrypoint /usr/local/bin/migrate-all migrate up

migrate-down: preflight bootstrap-db
	$(COMPOSE) run --rm --entrypoint /usr/local/bin/migrate-all migrate down

seed:
	@if [ -z "$${DATABASE_URL:-}" ]; then \
		echo "error: DATABASE_URL is required. Export it or point it at a reachable disposable database." >&2; \
		exit 1; \
	fi
	go run ./apps/services/identity/seed

db-reset:
	@echo "WARNING: this deletes local PostgreSQL and RabbitMQ volumes."
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) up -d postgres
	@echo "waiting for postgres to become healthy"
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' "$$($(COMPOSE) ps -q postgres)")" = "healthy" ]; do sleep 1; done
	$(MAKE) migrate

test:
	go test -race -count=1 ./...

test-access:
	@base="$$($(COMPOSE) port nginx 80 2>/dev/null | sed 's/.*://')"; \
	if [ -z "$$base" ]; then \
		echo "error: nginx is not running. Run 'make up' first (and 'make up-build' if your images are old)." >&2; \
		exit 1; \
	fi; \
	TEST_BASE_URL="http://localhost:$$base" go test -tags=access -count=1 -v ./tests/access/...

coverage:
	./scripts/coverage.sh

test-db:
	@if [ -z "$${TEST_DATABASE_URL:-}" ]; then \
		echo "error: TEST_DATABASE_URL is required for database integration tests." >&2; \
		echo "       The URL must allow CREATE DATABASE; tests create and drop isolated databases." >&2; \
		exit 1; \
	fi
	TEST_DATABASE_URL="$${TEST_DATABASE_URL}" go test -race -count=1 -tags=integration -v ./tests/integration/database/... ./apps/services/pricing/internal/http/... ./apps/services/notification-worker/internal/idempotency/... ./apps/services/notification-worker/internal/notificationlog/... ./apps/services/identity/internal/seed/... ./apps/services/merchant/internal/http/...

check-routes:
	@base="$$($(COMPOSE) port nginx 80 2>/dev/null | sed 's/.*://')"; \
	if [ -z "$$base" ]; then \
		echo "error: nginx is not running. Run 'make up' or start the stack first." >&2; \
		exit 1; \
	fi; \
	BASE_URL="http://localhost:$$base" ./scripts/check-routes.sh
