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

In development and test environments the verification-email adapter writes a one-time verification URL to the structured log. The raw token is never stored in PostgreSQL. Production startup is intentionally rejected until a real transactional email or outbox adapter is configured.

## Access and refresh tokens

Access tokens are short-lived JWTs signed with Ed25519. Generate a persistent local key pair with:

```powershell
go run ./cmd/access-keygen
```

Copy `IDENTITY_ACCESS_TOKEN_PRIVATE_KEY_BASE64` into the local `.env`. The command also prints the public key for diagnostics; the Identity gRPC API exposes the active public key and its validation parameters through `GetAccessTokenPublicKey`, so Family API can verify access tokens locally.

If the private key is omitted in development, the service creates an ephemeral key and logs a warning. Every outstanding access token becomes invalid after a restart. Production configuration requires a persistent private key.

Refresh tokens are opaque random values. PostgreSQL stores only their SHA-256 hashes. Every successful refresh replaces the current token and records the previous hash in `used_refresh_tokens`. Reusing a replaced token revokes the entire affected session. Session expiry is absolute and defaults to 30 days; access token expiry defaults to 15 minutes.

The gRPC server is an internal service boundary. Production deployment must restrict it to the private service network and add authenticated transport before exposing it outside that boundary.

## Protobuf

Install the pinned code generators:

```powershell
go install google.golang.org/protobuf/cmd/protoc-gen-go@v1.36.11
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@v1.6.2
```

Regenerate the `identity.v1` Go contract after changing a `.proto` file:

```powershell
./scripts/generate-proto.ps1
```

Generated files under `gen/identity/v1` are committed together with their source contract.

## Tests

Unit tests do not require PostgreSQL. To include repository and migration integration tests:

```powershell
$env:IDENTITY_TEST_DATABASE_URL="postgres://identity:identity@localhost:5433/identity_test?sslmode=disable"
go test -count=1 ./...
```

Integration tests refuse to run against a database whose name does not contain `test`.
