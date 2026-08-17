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
}

func TestLoadParsesValues(t *testing.T) {
	t.Setenv("IDENTITY_ENVIRONMENT", "test")
	t.Setenv("IDENTITY_GRPC_ADDR", "127.0.0.1:6000")
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "3s")
	t.Setenv("IDENTITY_LOG_LEVEL", "WARN")
	t.Setenv("IDENTITY_GRPC_REFLECTION", "true")

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
}

func TestLoadRejectsInvalidTimeout(t *testing.T) {
	t.Setenv("IDENTITY_SHUTDOWN_TIMEOUT", "0s")

	if _, err := Load(); err == nil {
		t.Fatal("Load() error = nil, want error")
	}
}
