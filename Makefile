SHELL := /bin/sh

COMPOSE ?= docker compose
BUILD_SERVICES ?= identity parcel pricing merchant realtime ai-assistant notification-worker webhook-worker web
BUILD_FLAGS ?=
MIGRATE_IMAGE ?= migrate/migrate:v4.18.3
MIGRATE_RUN = docker run --rm \
	-v "$(CURDIR)/migrations:/migrations:ro" \
	-v "$(CURDIR)/scripts/migrate.sh:/usr/local/bin/migrate-all:ro" \
	--entrypoint /usr/local/bin/migrate-all \
	-e DATABASE_URL -e DATABASE_URL_DIRECT -e APP_ENV -e ALLOW_PRODUCTION_ROLLBACK -e MIGRATE_SERVICES \
	$(MIGRATE_IMAGE)

.PHONY: help build up up-build down logs preflight project-name require-images ensure-postgres bootstrap-db migrate migrate-remote migrate-down migrate-status migrate-create storage-up storage-down db-audit db-backup db-backup-remote db-restore proof-transfer proof-transfer-apply proof-reconcile db-verify db-reset seed check-routes test test-db test-storage test-pooled test-access coverage coverage-merchant email-preview email-send-preview seed-merchant

help:
	@echo "Targets:"
	@echo "  make build          build all application images without starting them"
	@echo "  make up             start existing images, migrate, and verify gateway routes"
	@echo "  make up-build       build images, then run the full make up workflow"
	@echo "  make down           stop the stack without deleting data"
	@echo "  make logs           follow service logs"
	@echo "  make migrate        apply all service migrations (DATABASE_URL_DIRECT when set)"
	@echo "  make migrate-status print each service migration version and dirty state"
	@echo "  make migrate-create SERVICE=parcel NAME=description  create a new migration pair"
	@echo "  make storage-up     start local S3-compatible storage (VersityGW)"
	@echo "  make db-audit       read-only source database inventory"
	@echo "  make db-backup      verified local PostgreSQL backup outside the repo"
	@echo "  make proof-transfer dry-run local proof files into object storage"
	@echo "  make proof-reconcile compare database proof keys with stored objects"
	@echo "  make db-verify      verify the documented database baseline (read-only)"
	@echo "  make seed           create the five deterministic demo users"
	@echo "  make seed-merchant  create and approve the deterministic merchant demo account"
	@echo "  make migrate-down   roll back one migration per service (destructive)"
	@echo "  make check-routes   verify gateway routing against the running stack"
	@echo "  make project-name   print the resolved Compose project name"
	@echo "  make db-reset       delete local volumes and rebuild (destructive)"
	@echo "  make test           run every Go test with the race detector (no database needed)"
	@echo "  make test-db        run PostgreSQL integration tests against TEST_DATABASE_URL"
	@echo "  make test-access    run the cross-service access tests against the running stack"
	@echo "  make coverage       run the Go tests and print coverage per service and in total"
	@echo "  make coverage-merchant  enforce at least 60% merchant coverage with PostgreSQL tests"
	@echo "  make email-preview  render every email to tmp/email-preview/index.html"
	@echo "  make email-send-preview  send every email to Mailpit (http://localhost:8025)"

project-name:
	@config="$$($(COMPOSE) config 2>/dev/null)" || { \
		echo "error: '$(COMPOSE) config' failed; fix the Compose configuration or COMPOSE_PROJECT_NAME, then run '$(COMPOSE) config' to see the details." >&2; \
		exit 1; \
	}; \
	name="$$(printf '%s\n' "$$config" | awk '/^name:/{print $$2; exit}')"; \
	if [ -z "$$name" ]; then \
		echo "error: could not resolve the Compose project name from '$(COMPOSE) config'." >&2; \
		exit 1; \
	fi; \
	printf '%s\n' "$$name"

preflight:
	@if [ ! -f .env ] && { [ -z "$${POSTGRES_USER:-}" ] || [ -z "$${POSTGRES_DB:-}" ]; }; then \
		echo "error: .env is missing and POSTGRES_USER/POSTGRES_DB are not exported." >&2; \
		echo "       Run 'cp .env.example .env' and configure it, or export the required variables." >&2; \
		exit 1; \
	fi
	@$(COMPOSE) config --quiet
	@app_env="$$($(COMPOSE) config 2>/dev/null | awk '/^[[:space:]]+APP_ENV:/{sub(/^[[:space:]]*APP_ENV:[[:space:]]*/, ""); print; exit}')"; \
	app_env="$$(printf '%s' "$$app_env" | sed "s/^[[:space:]]*//;s/[[:space:]]*$$//;s/^[\"']//;s/[\"']$$//;s/^[[:space:]]*//;s/[[:space:]]*$$//")"; \
	app_env="$$(printf '%s' "$$app_env" | tr '[:upper:]' '[:lower:]')"; \
	case "$$app_env" in \
		development|staging|production) ;; \
		"") echo "error: APP_ENV is required. Set APP_ENV=development, staging, or production in .env or export it." >&2; exit 1 ;; \
		*) echo "error: APP_ENV must be one of: development, staging, production." >&2; exit 1 ;; \
	esac
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
	COMPOSE="$(COMPOSE)" ./scripts/bootstrap-db.sh

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

migrate:
	@if [ -n "$${DATABASE_URL_DIRECT:-}" ]; then \
		$(MAKE) migrate-remote ARGS=up; \
	else \
		$(MAKE) preflight; \
		./scripts/bootstrap-db.sh; \
		$(COMPOSE) run --rm --entrypoint /usr/local/bin/migrate-all migrate up; \
	fi

migrate-remote:
	@if [ "$(ARGS)" = "up" ]; then ./scripts/bootstrap-db.sh; fi
	$(MIGRATE_RUN) $(ARGS)

migrate-down:
	@if [ -n "$${DATABASE_URL_DIRECT:-}" ]; then \
		$(MAKE) migrate-remote ARGS=down; \
	else \
		$(MAKE) preflight; \
		$(COMPOSE) run --rm --entrypoint /usr/local/bin/migrate-all migrate down; \
	fi

migrate-status:
	@if [ -n "$${DATABASE_URL_DIRECT:-}" ]; then \
		$(MAKE) migrate-remote ARGS=status; \
	else \
		$(MAKE) preflight; \
		$(COMPOSE) run --rm --entrypoint /usr/local/bin/migrate-all migrate status; \
	fi

migrate-create:
	@test -n "$(SERVICE)" || { echo "error: SERVICE is required, for example SERVICE=parcel" >&2; exit 2; }
	@test -n "$(NAME)" || { echo "error: NAME is required, for example NAME=add_index" >&2; exit 2; }
	docker run --rm -v "$(CURDIR)/migrations:/migrations" \
		-v "$(CURDIR)/scripts/migrate.sh:/usr/local/bin/migrate-all:ro" \
		--entrypoint /usr/local/bin/migrate-all \
		-e MIGRATE_SERVICES="$(SERVICE)" \
		$(MIGRATE_IMAGE) create "$(SERVICE)" "$(NAME)"

storage-up:
	$(COMPOSE) --profile storage up -d --wait s3
	$(COMPOSE) --profile storage run --rm s3-init

storage-down:
	$(COMPOSE) --profile storage stop s3

db-audit:
	@if [ -n "$${DATABASE_URL_DIRECT:-}" ]; then \
		docker run --rm -i -e PGURL="$${DATABASE_URL_DIRECT}" postgres:16-alpine \
			sh -c 'psql "$$PGURL" -X -v ON_ERROR_STOP=1 -f -' \
			< scripts/audit-source-db.sql; \
	else \
		$(MAKE) ensure-postgres; \
		$(COMPOSE) exec -T postgres sh -c 'psql -X -v ON_ERROR_STOP=1 -U "$$POSTGRES_USER" -d "$$POSTGRES_DB" -f -' \
			< scripts/audit-source-db.sql; \
	fi

db-backup: ensure-postgres
	BACKUP_SOURCE=compose ./scripts/backup-db.sh

db-backup-remote:
	BACKUP_SOURCE=url ./scripts/backup-db.sh

db-restore:
	@test -n "$${BACKUP:-}" || { echo "error: BACKUP=/path/to/dump is required" >&2; exit 2; }
	./scripts/restore-db.sh "$${BACKUP}"

proof-transfer:
	go run ./apps/services/parcel/proof-transfer

proof-transfer-apply:
	go run ./apps/services/parcel/proof-transfer -apply

proof-reconcile:
	go run ./apps/services/parcel/proof-transfer -reconcile

db-verify:
	@COMPOSE="$(COMPOSE)" ./scripts/verify-db.sh

seed:
	@if [ -z "$${DATABASE_URL:-}" ]; then \
		echo "error: DATABASE_URL is required. Export it or point it at a reachable disposable database." >&2; \
		exit 1; \
	fi
	go run ./apps/services/identity/seed

seed-merchant:
	@if [ -z "$${DATABASE_URL:-}" ]; then \
		echo "error: DATABASE_URL is required. Export it or point it at a reachable disposable database." >&2; \
		exit 1; \
	fi
	@if [ -z "$${SEED_MERCHANT_PASSWORD:-}" ]; then \
		echo "error: SEED_MERCHANT_PASSWORD is required for make seed-merchant (no default is provided)." >&2; \
		exit 1; \
	fi
	go run ./apps/services/merchant/seed

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

coverage-merchant:
	@if [ -z "$${TEST_DATABASE_URL:-}" ]; then \
		echo "error: TEST_DATABASE_URL is required for merchant coverage." >&2; \
		exit 1; \
	fi
	TEST_DATABASE_URL="$${TEST_DATABASE_URL}" go test -race -count=1 -tags=integration -covermode=atomic -coverprofile=merchant-coverage.out ./apps/services/merchant/...
	@go tool cover -func=merchant-coverage.out | tail -1
	@percent="$$(go tool cover -func=merchant-coverage.out | awk '/^total:/ {gsub(/%/,"",$$3); print $$3}')"; \
	 awk -v p="$$percent" 'BEGIN { if (p + 0 < 60) exit 1 }' || { \
		echo "error: merchant coverage $$percent% is below the required 60%." >&2; \
		exit 1; \
	 }

email-preview:
	go run ./packages/go-common/emails/cmd/preview -out tmp/email-preview

email-send-preview:
	go run ./packages/go-common/emails/cmd/preview -out tmp/email-preview -send

test-storage:
	@if [ -z "$${STORAGE_TEST_ENDPOINT:-}" ]; then \
		echo "error: STORAGE_TEST_ENDPOINT is required for object storage integration tests." >&2; \
		echo "       Run 'make storage-up' and export the STORAGE_TEST_* variables." >&2; \
		exit 1; \
	fi
	STORAGE_TEST_ENDPOINT="$${STORAGE_TEST_ENDPOINT}" \
	STORAGE_TEST_ACCESS_KEY="$${STORAGE_TEST_ACCESS_KEY}" \
	STORAGE_TEST_SECRET_KEY="$${STORAGE_TEST_SECRET_KEY}" \
	STORAGE_TEST_BUCKET="$${STORAGE_TEST_BUCKET:-delivery-proofs-test}" \
	go test -race -count=1 -tags=integration,storage -v ./packages/go-common/objectstore

test-pooled:
	@if [ -z "$${TEST_POOLED_DATABASE_URL:-}" ]; then \
		echo "error: TEST_POOLED_DATABASE_URL is required for the PgBouncer compatibility test." >&2; \
		exit 1; \
	fi
	TEST_POOLED_DATABASE_URL="$${TEST_POOLED_DATABASE_URL}" \
	go test -race -count=1 -tags=integration -v ./packages/go-common/postgres -run TestPooledTransactionCompatibility

test-db:
	@if [ -z "$${TEST_DATABASE_URL:-}" ]; then \
		echo "error: TEST_DATABASE_URL is required for database integration tests." >&2; \
		echo "       The URL must allow CREATE DATABASE; tests create and drop isolated databases." >&2; \
		exit 1; \
	fi
	TEST_DATABASE_URL="$${TEST_DATABASE_URL}" go test -race -count=1 -tags=integration -v ./tests/integration/database/... ./apps/services/pricing/internal/http/... ./apps/services/notification-worker/internal/idempotency/... ./apps/services/notification-worker/internal/notificationlog/... ./apps/services/identity/internal/seed/... ./apps/services/merchant/internal/http/... ./apps/services/merchant/internal/repository/... ./apps/services/merchant/internal/seed/...
	@command -v migrate >/dev/null 2>&1 || { \
		echo "error: the golang-migrate 'migrate' CLI must be on PATH for parcel PostgreSQL tests." >&2; \
		exit 1; \
	}
	PARCEL_TEST_DATABASE_URL="$${PARCEL_TEST_DATABASE_URL:-$${TEST_DATABASE_URL}}" \
	TEST_DATABASE_URL="$${TEST_DATABASE_URL}" \
	go test -race -count=1 -v -tags=integration,postgres -run '^(TestParcel|TestCourier)' ./tests/integration ./tests/integration/parceltest ./apps/services/parcel/internal/repository ./apps/services/parcel/internal/http ./apps/services/parcel/internal/courier

check-routes:
	@base="$$($(COMPOSE) port nginx 80 2>/dev/null | sed 's/.*://')"; \
	if [ -z "$$base" ]; then \
		echo "error: nginx is not running. Run 'make up' or start the stack first." >&2; \
		exit 1; \
	fi; \
	BASE_URL="http://localhost:$$base" ./scripts/check-routes.sh
