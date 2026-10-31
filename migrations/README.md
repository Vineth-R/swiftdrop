# Database migrations

SwiftDrop uses a single PostgreSQL 16 instance for local development, with one schema per service:

| Service      | Schema         | Migration directory       |
| ------------ | -------------- | ------------------------- |
| Identity     | `identity`     | `migrations/identity`     |
| Parcel       | `parcel`       | `migrations/parcel`       |
| Pricing      | `pricing`      | `migrations/pricing`      |
| Merchant     | `merchant`     | `migrations/merchant`     |
| Notification | `notification` | `migrations/notification` |

Each service owns its schema. A service must not read or write another service's schema directly. Cross-service data exchange belongs in HTTP APIs or RabbitMQ events.

## Naming convention

- `NNNNNN_short_description.up.sql`
- `NNNNNN_short_description.down.sql`

Use a zero-padded, monotonically increasing six-digit sequence. Applied migrations are immutable. A correction is a new migration, not an edit to an applied file.

`000001_initial_schema` creates the service schema. `000002_tables` is reserved by the platform foundation for the baseline tables from `docs/db.sql`. Other owners add new migrations from `000003`; do not renumber or reuse an applied version.

## Tool

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate). The Compose `migrate` service pins `migrate/migrate:v4.18.3`; no host installation is required.

## One-command workflow

Run everything from the repository root with PostgreSQL started:

```sh
docker compose up -d postgres
make migrate
```

`make migrate` bootstraps the shared `public.set_updated_at()` function (so pre-existing volumes work) and then applies every service's migrations in a safe order: identity, pricing, parcel, merchant, notification. Each service uses its own version table (`<service>_schema_migrations`).

Roll back the latest migration for every service (reverse order):

```sh
DATABASE_URL='postgres://swiftdrop:swiftdrop@localhost:5432/swiftdrop?sslmode=disable'

migrate \
  -path migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" \
  up

migrate \
  -path migrations/parcel \
  -database "${DATABASE_URL}&x-migrations-table=parcel_schema_migrations" \
  up

migrate \
  -path migrations/pricing \
  -database "${DATABASE_URL}&x-migrations-table=pricing_schema_migrations" \
  up

migrate \
  -path migrations/merchant \
  -database "${DATABASE_URL}&x-migrations-table=merchant_schema_migrations" \
  up
```

The migration version tables live in the default `public` schema. Service-owned tables must always use their service schema explicitly.

No service connects to PostgreSQL yet. HTTP services load `DATABASE_URL` for future use, but application code does not open a database connection. These migrations create the service schemas only; they do not create business tables or seed data.

Verify the schemas after applying:

```sh
make db-reset
```

`make db-reset` runs `docker compose down -v`, which deletes the PostgreSQL and RabbitMQ volumes. Confirm the target database before running it.

## Manual commands

```sh
DATABASE_URL='postgres://swiftdrop:swiftdrop@postgres:5432/swiftdrop?sslmode=disable'

docker compose run --rm migrate \
  -path /migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" up
```

The migration version tables live in the default `public` schema. Service-owned tables must always use their service schema explicitly.

## Startup policy

Services do not run migrations automatically at startup. Run migrations explicitly as part of local setup or a deployment pipeline so schema changes are observable, reviewable, and independent of application restarts.
