package grpcserver

import (
	"context"
	"errors"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthv1 "google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const (
	defaultReadinessInterval = 5 * time.Second
	defaultReadinessTimeout  = 2 * time.Second
)

type ReadinessCheck struct {
	Name  string
	Check func(context.Context) error
}

type Option func(*Server)

func WithReadinessCheck(
	name string,
	check func(context.Context) error,
) Option {
	return func(server *Server) {
		server.readinessChecks = append(server.readinessChecks, ReadinessCheck{
			Name:  name,
			Check: check,
		})
	}
}

func WithReadinessTiming(interval, timeout time.Duration) Option {
	return func(server *Server) {
		if interval > 0 {
			server.readinessInterval = interval
		}
		if timeout > 0 {
			server.readinessTimeout = timeout
		}
	}
}

type Server struct {
	log               *zap.Logger
	grpcServer        *grpc.Server
	healthServer      *health.Server
	readinessChecks   []ReadinessCheck
	readinessInterval time.Duration
	readinessTimeout  time.Duration
	readinessStatus   atomic.Int32
}

func New(
	log *zap.Logger,
	enableReflection bool,
	options ...Option,
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

	server := &Server{
		log:               log.Named("grpc_server"),
		grpcServer:        grpcServer,
		healthServer:      healthServer,
		readinessInterval: defaultReadinessInterval,
		readinessTimeout:  defaultReadinessTimeout,
	}

	for _, option := range options {
		option(server)
	}

	return server
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

	s.updateReadiness(ctx)

	readinessCtx, cancelReadiness := context.WithCancel(ctx)
	defer cancelReadiness()
	go s.monitorReadiness(readinessCtx)

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
		cancelReadiness()
		s.healthServer.SetServingStatus(
			"",
			healthv1.HealthCheckResponse_NOT_SERVING,
		)

		s.log.Info("stopping gRPC server")
		s.gracefulStop(shutdownTimeout)
		return nil
	}
}

func (s *Server) monitorReadiness(ctx context.Context) {
	ticker := time.NewTicker(s.readinessInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			s.updateReadiness(ctx)
		}
	}
}

func (s *Server) updateReadiness(ctx context.Context) {
	status := healthv1.HealthCheckResponse_SERVING
	var failedCheck string
	var checkErr error

	for _, readinessCheck := range s.readinessChecks {
		checkCtx, cancel := context.WithTimeout(ctx, s.readinessTimeout)
		err := readinessCheck.Check(checkCtx)
		cancel()
		if err != nil {
			status = healthv1.HealthCheckResponse_NOT_SERVING
			failedCheck = readinessCheck.Name
			checkErr = err
			break
		}
	}

	s.healthServer.SetServingStatus("", status)
	previous := healthv1.HealthCheckResponse_ServingStatus(
		s.readinessStatus.Swap(int32(status)),
	)
	if previous == status {
		return
	}

	if status == healthv1.HealthCheckResponse_SERVING {
		s.log.Info("gRPC readiness changed", zap.String("status", "serving"))
		return
	}

	s.log.Warn(
		"gRPC readiness changed",
		zap.String("status", "not_serving"),
		zap.String("failed_check", failedCheck),
		zap.Error(checkErr),
	)
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
