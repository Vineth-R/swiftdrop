SHELL := /bin/sh

COMPOSE ?= docker compose
COMPOSE_PARALLEL_LIMIT ?= 1
export COMPOSE_PARALLEL_LIMIT

.PHONY: help build up up-build down logs preflight ensure-postgres bootstrap-db migrate migrate-down db-reset check-routes

help:
	@echo "Targets:"
	@echo "  make build          build all application images without starting them"
	@echo "  make up             start existing images, migrate, and verify gateway routes"
	@echo "  make up-build       build images, then run the full make up workflow"
	@echo "  make down           stop the stack without deleting data"
	@echo "  make logs           follow service logs"
	@echo "  make migrate        apply all service migrations"
	@echo "  make migrate-down   roll back one migration per service (destructive)"
	@echo "  make check-routes   verify gateway routing against the running stack"
	@echo "  make db-reset       delete local volumes and rebuild (destructive)"

preflight:
	@if [ ! -f .env ] && { [ -z "$${POSTGRES_USER:-}" ] || [ -z "$${POSTGRES_DB:-}" ]; }; then \
		echo "error: .env is missing and POSTGRES_USER/POSTGRES_DB are not exported." >&2; \
		echo "       Run 'cp .env.example .env' and configure it, or export the required variables." >&2; \
		exit 1; \
	fi
	@$(COMPOSE) config --quiet

ensure-postgres:
	@$(COMPOSE) ps --status running --services 2>/dev/null | grep -qx postgres || { \
		echo "error: postgres is not running. Start it with 'docker compose up -d postgres' or run 'make up'." >&2; \
		exit 1; \
	}

bootstrap-db: ensure-postgres
	$(COMPOSE) exec -T postgres sh -c 'psql -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -f /docker-entrypoint-initdb.d/01-schemas.sql'

build: preflight
	$(COMPOSE) build

up: preflight
	$(COMPOSE) up -d --wait --wait-timeout 300
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

db-reset:
	@echo "WARNING: this deletes local PostgreSQL and RabbitMQ volumes."
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) up -d postgres
	@echo "waiting for postgres to become healthy"
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' "$$($(COMPOSE) ps -q postgres)")" = "healthy" ]; do sleep 1; done
	$(MAKE) migrate

check-routes:
	@base="$$($(COMPOSE) port nginx 80 2>/dev/null | sed 's/.*://')"; \
	if [ -z "$$base" ]; then \
		echo "error: nginx is not running. Run 'make up' or start the stack first." >&2; \
		exit 1; \
	fi; \
	BASE_URL="http://localhost:$$base" ./scripts/check-routes.sh
