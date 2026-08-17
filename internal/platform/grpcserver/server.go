package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

type Server struct {
	log          *zap.Logger
	grpcServer   *grpc.Server
	healthServer *health.Server
}

func New(
	log *zap.Logger,
	enableReflection bool,
) *Server {
	grpcServer := grpc.NewServer()
	healthServer := health.NewServer()

	healthv1.RegisterHealthServer(
		grpcServer,
		healthServer,
	)

	if enableReflection {
		reflection.Register(grpcServer)
	}

	return &Server{
		log:          log.Named("grpc_server"),
		grpcServer:   grpcServer,
		healthServer: healthServer,
	}
}

func (s *Server) Run(
	ctx context.Context,
	address string,
	shutdownTimeout time.Duration,
) error {
	listener, err := net.Listen("tcp", address)
	if err != nil {
		return fmt.Errorf(
			"listen on %s: %w",
			address,
			err,
		)
	}

	return s.Serve(ctx, listener, shutdownTimeout)
}

// Serve starts the gRPC server on an already-created listener. Keeping this
// method separate makes startup and graceful shutdown testable without a real
// TCP port.
func (s *Server) Serve(
	ctx context.Context,
	listener net.Listener,
	shutdownTimeout time.Duration,
) error {
	if shutdownTimeout <= 0 {
		return fmt.Errorf("gRPC shutdown timeout must be positive")
	}

	defer listener.Close()

	s.healthServer.SetServingStatus(
		"",
		healthv1.HealthCheckResponse_SERVING,
	)

	s.log.Info("gRPC server started", zap.String("address", listener.Addr().String()))

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.grpcServer.Serve(listener)
	}()

	select {
	case err := <-errCh:
		if err == nil || errors.Is(err, grpc.ErrServerStopped) {
			return nil
		}

		return fmt.Errorf("serve gRPC: %w", err)

	case <-ctx.Done():
		s.healthServer.SetServingStatus(
			"",
			healthv1.HealthCheckResponse_NOT_SERVING,
		)

		s.log.Info("stopping gRPC server")
		s.gracefulStop(shutdownTimeout)
		return nil
	}
}

func (s *Server) gracefulStop(timeout time.Duration) {
	stopped := make(chan struct{})
	go func() {
		s.grpcServer.GracefulStop()
		close(stopped)
	}()

	timer := time.NewTimer(timeout)
	defer timer.Stop()

	select {
	case <-stopped:
		s.log.Info("gRPC server stopped gracefully")
	case <-timer.C:
		s.log.Warn("gRPC graceful shutdown timed out")
		s.grpcServer.Stop()
		<-stopped
	}
}
