package grpcserver

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/test/bufconn"
)

func TestServerReportsServingAndStops(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := New(zap.NewNop(), false)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- server.Serve(ctx, listener, time.Second)
	}()

	clientConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cancel()
		t.Fatalf("create gRPC client: %v", err)
	}
	defer clientConn.Close()

	healthClient := grpc_health_v1.NewHealthClient(clientConn)
	response, err := healthClient.Check(
		context.Background(),
		&grpc_health_v1.HealthCheckRequest{},
	)
	if err != nil {
		cancel()
		t.Fatalf("check gRPC health: %v", err)
	}

	if got, want := response.Status, grpc_health_v1.HealthCheckResponse_SERVING; got != want {
		cancel()
		t.Fatalf("health status = %s, want %s", got, want)
	}

	cancel()

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server did not stop")
	}
}

func TestServerReportsNotServingWhenDependencyFails(t *testing.T) {
	listener := bufconn.Listen(1024 * 1024)
	server := New(
		zap.NewNop(),
		false,
		WithReadinessCheck("postgres", func(context.Context) error {
			return errors.New("database unavailable")
		}),
	)
	ctx, cancel := context.WithCancel(context.Background())
	errCh := make(chan error, 1)

	go func() {
		errCh <- server.Serve(ctx, listener, time.Second)
	}()

	clientConn, err := grpc.NewClient(
		"passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		cancel()
		t.Fatalf("create gRPC client: %v", err)
	}
	defer clientConn.Close()

	healthClient := grpc_health_v1.NewHealthClient(clientConn)
	response, err := healthClient.Check(
		context.Background(),
		&grpc_health_v1.HealthCheckRequest{},
	)
	if err != nil {
		cancel()
		t.Fatalf("check gRPC health: %v", err)
	}
	if got, want := response.Status, grpc_health_v1.HealthCheckResponse_NOT_SERVING; got != want {
		cancel()
		t.Fatalf("health status = %s, want %s", got, want)
	}

	cancel()
	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("Serve() error = %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("gRPC server did not stop")
	}
}

func TestServerRejectsInvalidShutdownTimeout(t *testing.T) {
	listener := bufconn.Listen(1024)
	defer listener.Close()

	server := New(zap.NewNop(), false)
	if err := server.Serve(context.Background(), listener, 0); err == nil {
		t.Fatal("Serve() error = nil, want error")
	}
}
