package migrations

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/ZheglY/family-tree-identity-service/internal/testdatabase"
	"go.uber.org/zap"
)

func TestRunnerUpDownIntegration(t *testing.T) {
	databaseURL := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IDENTITY_TEST_DATABASE_URL is not set")
	}

	testDatabase, err := testdatabase.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open isolated test database: %v", err)
	}
	t.Cleanup(func() {
		if err := testDatabase.Close(); err != nil {
			t.Errorf("close isolated test database: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	pool := testDatabase.Pool

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
	if version != 2 {
		t.Fatalf("current version = %d, want 2", version)
	}

	var usersTableExists bool
	if err := pool.QueryRow(
		ctx,
		"SELECT to_regclass('users') IS NOT NULL",
	).Scan(&usersTableExists); err != nil {
		t.Fatalf("check users table: %v", err)
	}
	if !usersTableExists {
		t.Fatal("users table does not exist after Up()")
	}

	if err := runner.Down(ctx, 1); err != nil {
		t.Fatalf("Down() error = %v", err)
	}
	version, err = runner.CurrentVersion(ctx)
	if err != nil {
		t.Fatalf("CurrentVersion() after Down error = %v", err)
	}
	if version != 1 {
		t.Fatalf("version after Down = %d, want 1", version)
	}

	if err := runner.Up(ctx); err != nil {
		t.Fatalf("restore schema after test: %v", err)
	}
}
