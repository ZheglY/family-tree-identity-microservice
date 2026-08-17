package migrations

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestRunnerUpDownIntegration(t *testing.T) {
	databaseURL := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IDENTITY_TEST_DATABASE_URL is not set")
	}

	poolConfig, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	if !strings.Contains(strings.ToLower(poolConfig.ConnConfig.Database), "test") {
		t.Fatalf("refusing to migrate non-test database %q", poolConfig.ConnConfig.Database)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	pool, err := pgxpool.NewWithConfig(ctx, poolConfig)
	if err != nil {
		t.Fatalf("create PostgreSQL pool: %v", err)
	}
	defer pool.Close()

	runner, err := NewRunner(pool, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}

	if err := runner.Down(ctx, len(runner.migrations)); err != nil {
		t.Fatalf("initial cleanup: %v", err)
	}

	if err := runner.Up(ctx); err != nil {
		t.Fatalf("Up() error = %v", err)
	}
	if err := runner.Up(ctx); err != nil {
		t.Fatalf("second Up() must be idempotent: %v", err)
	}

	version, err := runner.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion() error = %v", err)
	}
	if version != 1 {
		t.Fatalf("current version = %d, want 1", version)
	}

	var usersTableExists bool
	if err := pool.QueryRow(
		ctx,
		"SELECT to_regclass('public.users') IS NOT NULL",
	).Scan(&usersTableExists); err != nil {
		t.Fatalf("check users table: %v", err)
	}
	if !usersTableExists {
		t.Fatal("users table does not exist after Up()")
	}

	if err := runner.Down(ctx, 1); err != nil {
		t.Fatalf("Down() error = %v", err)
	}

	if err := runner.Up(ctx); err != nil {
		t.Fatalf("restore schema after test: %v", err)
	}
}
