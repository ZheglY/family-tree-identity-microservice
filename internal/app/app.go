package app

import (
	"context"
	"fmt"

	"github.com/ZheglY/family-tree-identity-service/internal/config"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/grpcserver"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/logger"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/postgres"
)

func Run(ctx context.Context, cfg config.Config) error {
	log, err := logger.New(logger.Config{
		ServiceName: cfg.App.Name,
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

	server := grpcserver.New(
		log,
		cfg.GRPC.Reflection,
		grpcserver.WithReadinessCheck("postgres", database.Ping),
		grpcserver.WithReadinessTiming(
			cfg.GRPC.ReadinessCheckInterval,
			cfg.GRPC.ReadinessCheckTimeout,
		),
	)
	if err := server.Run(
		ctx,
		cfg.GRPC.Address,
		cfg.GRPC.ShutdownTimeout,
	); err != nil {
		return fmt.Errorf("run gRPC server: %w", err)
	}

	return nil
}
