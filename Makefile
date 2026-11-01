SHELL := /bin/sh

COMPOSE ?= docker compose
SERVICES := identity pricing parcel merchant notification
REVERSE_SERVICES := notification merchant parcel pricing identity

.PHONY: help up down logs migrate migrate-down db-reset check-routes

help:
	@echo "Targets: up down logs migrate migrate-down db-reset check-routes"

up:
	$(COMPOSE) up --build -d

down:
	$(COMPOSE) down

logs:
	$(COMPOSE) logs -f --tail=100

bootstrap-db:
	$(COMPOSE) exec -T postgres psql -v ON_ERROR_STOP=1 \
		-U "$${POSTGRES_USER:-swiftdrop}" -d "$${POSTGRES_DB:-swiftdrop}" \
		-c "CREATE OR REPLACE FUNCTION public.set_updated_at() RETURNS trigger AS \$$\$$ BEGIN NEW.updated_at := now(); RETURN NEW; END; \$$\$$ LANGUAGE plpgsql;"

migrate:
	bash scripts/migrate.sh

migrate-down:
	@echo "Batch rollback is disabled: Merchant 000006 is forward-only. Use reviewed per-service down commands against a confirmed disposable database."
	@exit 1

db-reset:
	@echo "WARNING: this deletes local PostgreSQL and RabbitMQ volumes."
	$(COMPOSE) down -v --remove-orphans
	$(COMPOSE) up -d postgres
	@echo "waiting for postgres to become healthy"
	@until [ "$$(docker inspect -f '{{.State.Health.Status}}' "$$($(COMPOSE) ps -q postgres)")" = "healthy" ]; do sleep 1; done
	$(MAKE) migrate

check-routes:
	./scripts/check-routes.sh
