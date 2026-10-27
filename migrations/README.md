# Database migrations

SwiftDrop uses a single PostgreSQL instance for local development, with one schema per service:

| Service  | Schema     | Migration directory   |
| -------- | ---------- | --------------------- |
| Identity | `identity` | `migrations/identity` |
| Parcel   | `parcel`   | `migrations/parcel`   |
| Pricing  | `pricing`  | `migrations/pricing`  |
| Merchant | `merchant` | `migrations/merchant` |

Each service owns its schema. A service must not read or write another service's schema directly. Cross-service data exchange belongs in HTTP APIs or RabbitMQ events.

## Naming convention

- `NNNNNN_short_description.up.sql`
- `NNNNNN_short_description.down.sql`

Use a zero-padded, monotonically increasing six-digit sequence. Applied migrations are immutable. A correction is a new migration, not an edit to an applied file.

The initial migration for each service creates its schema. Business tables will be added by later migrations and must be schema-qualified, for example `parcel.parcels`.

## Tool

Migrations use [golang-migrate](https://github.com/golang-migrate/migrate). Install the CLI with PostgreSQL support:

```sh
go install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@v4.18.3
```

The CLI is a developer tool and is not part of the application's Go module.

## Running migrations

Run the commands below from the repository root against a reachable PostgreSQL 16 instance. For local development, start the Compose database and wait for it to report healthy:

```sh
docker compose up -d postgres
docker compose ps postgres
```

The connection string matches the defaults in `.env.example`; update it if you changed `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`, or `POSTGRES_PORT`.

Use a unique migration version table per service so the shared database does not mix migration histories:

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
psql "${DATABASE_URL}" -c '\dn'
```

## Rolling back

Roll back one migration for a service:

```sh
migrate \
  -path migrations/identity \
  -database "${DATABASE_URL}&x-migrations-table=identity_schema_migrations" \
  down 1
```

The initial down migrations drop the service schema with `CASCADE`. This is destructive. Confirm the target database before running a rollback.

## Startup policy

Services do not run migrations automatically at startup. Run migrations explicitly as part of local setup or a deployment pipeline so schema changes are observable, reviewable, and independent of application restarts.
