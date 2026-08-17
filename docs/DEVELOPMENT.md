# Identity Service development

## Local PostgreSQL

Start PostgreSQL and wait for its health check:

```powershell
docker compose up -d --wait identity-postgres
```

The local instance exposes:

- application database: `identity`;
- integration-test database: `identity_test`;
- address: `localhost:5433`;
- development credentials: `identity / identity`.

These credentials are only defaults for the local Compose environment. `IDENTITY_POSTGRES_URL` is mandatory when `IDENTITY_ENVIRONMENT=production`.

## Migrations

Apply every pending migration:

```powershell
go run ./cmd/migrate up
```

Show the current version:

```powershell
go run ./cmd/migrate version
```

Rollback one migration:

```powershell
go run ./cmd/migrate down 1
```

Migration files are embedded into the command. Every version must have matching files:

```text
migrations/000002_description.up.sql
migrations/000002_description.down.sql
```

Applied migrations are protected by a checksum. Do not edit a migration after it has been applied outside a disposable development database; create a new version instead.

The runner uses a PostgreSQL transaction-level advisory lock, so concurrent migration processes cannot apply schema versions at the same time. Migrations must remain transaction-compatible and must not use statements such as `CREATE INDEX CONCURRENTLY`.

## Running the service

Apply migrations first, then start the gRPC service:

```powershell
go run ./cmd/migrate up
go run ./cmd/identity-service
```

The standard gRPC health service reports `NOT_SERVING` if its PostgreSQL readiness check fails.

## Tests

Unit tests do not require PostgreSQL. To include repository and migration integration tests:

```powershell
$env:IDENTITY_TEST_DATABASE_URL="postgres://identity:identity@localhost:5433/identity_test?sslmode=disable"
go test -count=1 ./...
```

Integration tests refuse to run against a database whose name does not contain `test`.
