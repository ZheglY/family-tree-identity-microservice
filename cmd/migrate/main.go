package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strconv"
	"syscall"

	"github.com/ZheglY/family-tree-identity-service/internal/config"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/logger"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/postgres"
	"github.com/ZheglY/family-tree-identity-service/migrations"
)

func main() {
	ctx, stop := signal.NotifyContext(
		context.Background(),
		syscall.SIGINT,
		syscall.SIGTERM,
	)
	defer stop()

	if err := run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "migration failed:", err)
		os.Exit(1)
	}
}

func run(ctx context.Context, arguments []string) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load config: %w", err)
	}

	log, err := logger.New(logger.Config{
		ServiceName: cfg.App.Name + "-migrate",
		Environment: cfg.App.Environment,
		Level:       cfg.Logger.Level,
	})
	if err != nil {
		return fmt.Errorf("initialize logger: %w", err)
	}
	defer func() {
		_ = log.Sync()
	}()

	database, err := postgres.Open(
		ctx,
		postgres.Config{
			URL:               cfg.Postgres.URL,
			MaxConnections:    cfg.Postgres.MaxConnections,
			MinConnections:    cfg.Postgres.MinConnections,
			MaxConnLifetime:   cfg.Postgres.MaxConnLifetime,
			MaxConnIdleTime:   cfg.Postgres.MaxConnIdleTime,
			HealthCheckPeriod: cfg.Postgres.HealthCheckPeriod,
			ConnectTimeout:    cfg.Postgres.ConnectTimeout,
		},
		log,
	)
	if err != nil {
		return fmt.Errorf("initialize PostgreSQL: %w", err)
	}
	defer database.Close()

	runner, err := migrations.NewRunner(database.Native(), log)
	if err != nil {
		return fmt.Errorf("initialize migration runner: %w", err)
	}

	action := "up"
	if len(arguments) > 0 {
		action = arguments[0]
	}

	switch action {
	case "up":
		if err := runner.Up(ctx); err != nil {
			return err
		}
	case "down":
		steps := 1
		if len(arguments) > 1 {
			steps, err = strconv.Atoi(arguments[1])
			if err != nil || steps <= 0 {
				return fmt.Errorf("down steps must be a positive integer")
			}
		}
		if err := runner.Down(ctx, steps); err != nil {
			return err
		}
	case "version":
		version, err := runner.CurrentVersion(ctx)
		if err != nil {
			return err
		}
		fmt.Println(version)
	default:
		return fmt.Errorf("unknown action %q; use up, down [steps], or version", action)
	}

	return nil
}
