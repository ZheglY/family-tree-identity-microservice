package app

import (
	"context"
	"fmt"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/config"
	identitygrpc "github.com/ZheglY/family-tree-identity-service/internal/identity/adapters/grpc"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/adapters/mailer"
	identitypostgres "github.com/ZheglY/family-tree-identity-service/internal/identity/adapters/postgres"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/grpcserver"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/logger"
	"github.com/ZheglY/family-tree-identity-service/internal/platform/postgres"
	passwordsecurity "github.com/ZheglY/family-tree-identity-service/internal/security/password"
	tokensecurity "github.com/ZheglY/family-tree-identity-service/internal/security/token"
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

	if cfg.App.Environment == "production" {
		return fmt.Errorf("production verification mailer is not configured")
	}

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

	verificationMailer, err := mailer.NewLogMailer(
		log,
		cfg.Email.VerificationURL,
	)
	if err != nil {
		return fmt.Errorf("initialize verification mailer: %w", err)
	}

	identityRepository := identitypostgres.NewRepository(database.Native())
	identityService := application.NewService(
		identityRepository,
		passwordsecurity.NewHasher(),
		tokensecurity.NewGenerator(),
		verificationMailer,
	)
	identityGRPCServer := identitygrpc.NewServer(identityService, log)

	server := grpcserver.New(
		log,
		cfg.GRPC.Reflection,
		grpcserver.WithReadinessCheck("postgres", database.Ping),
		grpcserver.WithReadinessTiming(
			cfg.GRPC.ReadinessCheckInterval,
			cfg.GRPC.ReadinessCheckTimeout,
		),
	)
	identityv1.RegisterIdentityServiceServer(
		server.Registrar(),
		identityGRPCServer,
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
