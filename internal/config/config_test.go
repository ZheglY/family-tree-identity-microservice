package config

import (
	"testing"
	"time"
)

func TestLoadUsesDefaults(t *testing.T) {
	t.Setenv("IDENTITY_ENVIRONMENT", "")
	t.Setenv("IDENTITY_GRPC_ADDR", "")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "")
	t.Setenv("IDENTITY_LOG_LEVEL", "")
	t.Setenv("IDENTITY_GRPC_REFLECTION", "")
	setPostgresEnvironment(t, "")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.App.Environment, defaultEnvironment; got != want {
		t.Fatalf("environment = %q, want %q", got, want)
	}
	if got, want := cfg.GRPC.Address, defaultGRPCAddress; got != want {
		t.Fatalf("gRPC address = %q, want %q", got, want)
	}
	if got, want := cfg.GRPC.ShutdownTimeout, defaultShutdownTimeout; got != want {
		t.Fatalf("shutdown timeout = %s, want %s", got, want)
	}
	if got, want := cfg.Logger.Level, defaultLogLevel; got != want {
		t.Fatalf("log level = %q, want %q", got, want)
	}
	if cfg.GRPC.Reflection {
		t.Fatal("gRPC reflection enabled by default")
	}
	if got, want := cfg.Postgres.URL, defaultPostgresURL; got != want {
		t.Fatalf("PostgreSQL URL = %q, want %q", got, want)
	}
	if got, want := cfg.Postgres.MaxConnections, int32(defaultPostgresMaxConnections); got != want {
		t.Fatalf("max PostgreSQL connections = %d, want %d", got, want)
	}
}

func TestLoadParsesValues(t *testing.T) {
	t.Setenv("IDENTITY_ENVIRONMENT", "test")
	t.Setenv("IDENTITY_GRPC_ADDR", "127.0.0.1:6000")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("IDENTITY_LOG_LEVEL", "WARN")
	t.Setenv("IDENTITY_GRPC_REFLECTION", "true")
	setPostgresEnvironment(t, "")
	t.Setenv("IDENTITY_POSTGRES_MAX_CONNECTIONS", "20")
	t.Setenv("IDENTITY_POSTGRES_MIN_CONNECTIONS", "2")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if got, want := cfg.GRPC.ShutdownTimeout, 3*time.Second; got != want {
		t.Fatalf("shutdown timeout = %s, want %s", got, want)
	}
	if !cfg.GRPC.Reflection {
		t.Fatal("gRPC reflection was not enabled")
	}
	if got, want := cfg.Postgres.MaxConnections, int32(20); got != want {
		t.Fatalf("max PostgreSQL connections = %d, want %d", got, want)
	}
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	setPostgresEnvironment(t, "")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "0s")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRejectsInvalidPostgresPoolBounds(t *testing.T) {
	setPostgresEnvironment(t, "")
	t.Setenv("IDENTITY_POSTGRES_MAX_CONNECTIONS", "2")
	t.Setenv("IDENTITY_POSTGRES_MIN_CONNECTIONS", "3")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func TestLoadRequiresPostgresURLInProduction(t *testing.T) {
	setPostgresEnvironment(t, "")
	t.Setenv("IDENTITY_ENVIRONMENT", "production")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}

func setPostgresEnvironment(t *testing.T, value string) {
	t.Helper()

	keys := []string{
		"IDENTITY_POSTGRES_URL",
		"IDENTITY_POSTGRES_MAX_CONNECTIONS",
		"IDENTITY_POSTGRES_MIN_CONNECTIONS",
		"IDENTITY_POSTGRES_MAX_CONN_LIFETIME",
		"IDENTITY_POSTGRES_MAX_CONN_IDLE_TIME",
		"IDENTITY_POSTGRES_HEALTH_CHECK_PERIOD",
		"IDENTITY_POSTGRES_CONNECT_TIMEOUT",
		"IDENTITY_GRPC_READINESS_CHECK_INTERVAL",
		"IDENTITY_GRPC_READINESS_CHECK_TIMEOUT",
	}

	for _, key := range keys {
		t.Setenv(key, value)
	}
}
