package postgres

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestOpenAndPingIntegration(t *testing.T) {
	databaseURL := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IDENTITY_TEST_DATABASE_URL is not set")
	}

	parsed, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatalf("parse test database URL: %v", err)
	}
	if !strings.Contains(strings.ToLower(parsed.ConnConfig.Database), "test") {
		t.Fatalf("refusing to use non-test database %q", parsed.ConnConfig.Database)
	}

	pool, err := Open(
		context.Background(),
		Config{
			URL:               databaseURL,
			MaxConnections:    2,
			MinConnections:    1,
			MaxConnLifetime:   time.Minute,
			MaxConnIdleTime:   time.Minute,
			HealthCheckPeriod: time.Second,
			ConnectTimeout:    5 * time.Second,
		},
		zap.NewNop(),
	)
	if err != nil {
		t.Fatalf("Open() error = %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		t.Fatalf("Ping() error = %v", err)
	}
}
