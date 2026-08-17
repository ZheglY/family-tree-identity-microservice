package app

import (
	"context"
	"fmt"

	"github.com/ZheglY/family-tree-identity-service/internal/config"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/grpcserver"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/logger"
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

	server := grpcserver.New(log, cfg.GRPC.Reflection)
	if err := server.Run(
		ctx,
		cfg.GRPC.Address,
		cfg.GRPC.ShutdownTimeout,
	); err != nil {
		return fmt.Errorf("run gRPC server: %w", err)
	}

	return nil
}
