package grpc

import (
	"context"
	"net"
	"os"
	"testing"
	"time"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/adapters/postgres"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	passwordsecurity "github.com/ZheglY/family-tree-identity-service/internal/security/password"
	tokensecurity "github.com/ZheglY/family-tree-identity-service/internal/security/token"
	"github.com/ZheglY/family-tree-identity-service/internal/testdatabase"
	"github.com/ZheglY/family-tree-identity-service/migrations"
	"go.uber.org/zap"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"
)

type captureMailer struct {
	token string
}

func (m *captureMailer) SendVerification(
	_ context.Context,
	_ domain.Email,
	token string,
) error {
	m.token = token
	return nil
}

func TestRegistrationVerticalSliceIntegration(t *testing.T) {
	databaseURL := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IDENTITY_TEST_DATABASE_URL is not set")
	}

	testDatabase, err := testdatabase.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open isolated test database: %v", err)
	}
	t.Cleanup(func() {
		if err := testDatabase.Close(); err != nil {
			t.Errorf("close isolated test database: %v", err)
		}
	})

	migrationRunner, err := migrations.NewRunner(testDatabase.Pool, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	if err := migrationRunner.Up(context.Background()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	mailer := &captureMailer{}
	service := application.NewService(
		postgres.NewRepository(testDatabase.Pool),
		passwordsecurity.NewHasher(),
		tokensecurity.NewGenerator(),
		mailer,
	)
	transport := NewServer(service, zap.NewNop())

	listener := bufconn.Listen(1024 * 1024)
	grpcServer := gogrpc.NewServer()
	identityv1.RegisterIdentityServiceServer(grpcServer, transport)
	go func() {
		_ = grpcServer.Serve(listener)
	}()
	t.Cleanup(grpcServer.Stop)

	clientConnection, err := gogrpc.NewClient(
		"passthrough:///bufnet",
		gogrpc.WithContextDialer(func(context.Context, string) (net.Conn, error) {
			return listener.Dial()
		}),
		gogrpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatalf("create gRPC client: %v", err)
	}
	t.Cleanup(func() { _ = clientConnection.Close() })
	client := identityv1.NewIdentityServiceClient(clientConnection)

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	registerResponse, err := client.Register(ctx, &identityv1.RegisterRequest{
		Email:       "family@example.com",
		Password:    "correct horse battery staple",
		DisplayName: "Family Member",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}
	if registerResponse.GetUser().GetStatus() != identityv1.UserStatus_USER_STATUS_PENDING {
		t.Fatalf("registered status = %s, want pending", registerResponse.GetUser().GetStatus())
	}
	if mailer.token == "" {
		t.Fatal("verification token was not delivered")
	}

	var (
		storedPasswordHash string
		storedTokenHash    string
	)
	if err := testDatabase.Pool.QueryRow(ctx, `
		SELECT c.password_hash, t.token_hash
		FROM user_credentials c
		JOIN one_time_tokens t ON t.user_id = c.user_id
	`).Scan(&storedPasswordHash, &storedTokenHash); err != nil {
		t.Fatalf("read stored credential material: %v", err)
	}
	if storedPasswordHash == "correct horse battery staple" {
		t.Fatal("raw password was stored")
	}
	if got, want := storedTokenHash, tokensecurity.Hash(mailer.token); got != want {
		t.Fatalf("stored token hash = %q, want %q", got, want)
	}

	verifyResponse, err := client.VerifyEmail(ctx, &identityv1.VerifyEmailRequest{
		Token: mailer.token,
	})
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if verifyResponse.GetUser().GetStatus() != identityv1.UserStatus_USER_STATUS_ACTIVE {
		t.Fatalf("verified status = %s, want active", verifyResponse.GetUser().GetStatus())
	}
}
