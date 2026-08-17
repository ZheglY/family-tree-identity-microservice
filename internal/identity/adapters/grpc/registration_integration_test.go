package grpc

import (
	"context"
	"crypto/ed25519"
	"encoding/base64"
	"net"
	"os"
	"testing"
	"time"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/adapters/postgres"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/ZheglY/family-tree-identity-service/internal/security/accesstoken"
	passwordsecurity "github.com/ZheglY/family-tree-identity-service/internal/security/password"
	tokensecurity "github.com/ZheglY/family-tree-identity-service/internal/security/token"
	"github.com/ZheglY/family-tree-identity-service/internal/testdatabase"
	"github.com/ZheglY/family-tree-identity-service/migrations"
	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	gogrpc "google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
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
	accessSigner, err := accesstoken.NewEphemeralSigner(
		"integration-key",
		"test-identity",
		"test-family-api",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("create access signer: %v", err)
	}
	service := application.NewService(
		postgres.NewRepository(testDatabase.Pool),
		passwordsecurity.NewHasher(),
		tokensecurity.NewGenerator(),
		accessSigner,
		mailer,
		30*24*time.Hour,
	)
	transport := NewServer(service, zap.NewNop(), accessSigner.PublicKeyInfo())

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

	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
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

	loginResponse, err := client.Login(ctx, &identityv1.LoginRequest{
		Email:     "FAMILY@example.com",
		Password:  "correct horse battery staple",
		UserAgent: "integration browser",
		IpAddress: "127.0.0.1",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if loginResponse.GetAccessToken() == "" || loginResponse.GetRefreshToken() == "" {
		t.Fatalf("login did not return both tokens: %#v", loginResponse)
	}

	publicKeyResponse, err := client.GetAccessTokenPublicKey(
		ctx,
		&identityv1.GetAccessTokenPublicKeyRequest{},
	)
	if err != nil {
		t.Fatalf("GetAccessTokenPublicKey() error = %v", err)
	}
	publicKeyBytes, err := base64.StdEncoding.DecodeString(publicKeyResponse.GetPublicKeyBase64())
	if err != nil {
		t.Fatalf("decode access token public key: %v", err)
	}
	claims := &accesstoken.Claims{}
	parsedAccessToken, err := jwt.ParseWithClaims(
		loginResponse.GetAccessToken(),
		claims,
		func(*jwt.Token) (any, error) { return ed25519.PublicKey(publicKeyBytes), nil },
		jwt.WithValidMethods([]string{accesstoken.Algorithm}),
		jwt.WithIssuer(publicKeyResponse.GetIssuer()),
		jwt.WithAudience(publicKeyResponse.GetAudience()),
		jwt.WithExpirationRequired(),
	)
	if err != nil || !parsedAccessToken.Valid {
		t.Fatalf("parse issued access token: valid=%t error=%v", parsedAccessToken != nil && parsedAccessToken.Valid, err)
	}
	if claims.Subject != registerResponse.GetUser().GetId() || claims.SessionID == "" {
		t.Fatalf("unexpected access token claims: %#v", claims)
	}
	getUserResponse, err := client.GetUser(ctx, &identityv1.GetUserRequest{
		UserId: registerResponse.GetUser().GetId(),
	})
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if getUserResponse.GetUser().GetEmail() != "family@example.com" {
		t.Fatalf("GetUser() response = %#v", getUserResponse.GetUser())
	}
	listSessionsResponse, err := client.ListSessions(ctx, &identityv1.ListSessionsRequest{
		UserId: registerResponse.GetUser().GetId(),
	})
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(listSessionsResponse.GetSessions()) != 1 ||
		listSessionsResponse.GetSessions()[0].GetId() != claims.SessionID {
		t.Fatalf("ListSessions() response = %#v", listSessionsResponse.GetSessions())
	}

	var storedRefreshHash string
	if err := testDatabase.Pool.QueryRow(ctx, `
		SELECT refresh_token_hash FROM user_sessions WHERE id = $1
	`, claims.SessionID).Scan(&storedRefreshHash); err != nil {
		t.Fatalf("read stored refresh token hash: %v", err)
	}
	if got, want := storedRefreshHash, tokensecurity.Hash(loginResponse.GetRefreshToken()); got != want {
		t.Fatalf("stored refresh hash = %q, want %q", got, want)
	}
	if storedRefreshHash == loginResponse.GetRefreshToken() {
		t.Fatal("raw refresh token was stored")
	}

	firstRefreshToken := loginResponse.GetRefreshToken()
	refreshResponse, err := client.RefreshSession(ctx, &identityv1.RefreshSessionRequest{
		RefreshToken: firstRefreshToken,
		UserAgent:    "integration browser updated",
		IpAddress:    "2001:db8::1",
	})
	if err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if refreshResponse.GetRefreshToken() == firstRefreshToken {
		t.Fatal("refresh token was not rotated")
	}

	_, err = client.RefreshSession(ctx, &identityv1.RefreshSessionRequest{
		RefreshToken: firstRefreshToken,
	})
	if got, want := status.Code(err), codes.Unauthenticated; got != want {
		t.Fatalf("replayed refresh code = %s, want %s", got, want)
	}
	_, err = client.RefreshSession(ctx, &identityv1.RefreshSessionRequest{
		RefreshToken: refreshResponse.GetRefreshToken(),
	})
	if got, want := status.Code(err), codes.Unauthenticated; got != want {
		t.Fatalf("refresh after replay code = %s, want %s", got, want)
	}

	logoutSession, err := client.Login(ctx, &identityv1.LoginRequest{
		Email:    "family@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("second Login() error = %v", err)
	}
	if _, err := client.Logout(ctx, &identityv1.LogoutRequest{
		RefreshToken: logoutSession.GetRefreshToken(),
	}); err != nil {
		t.Fatalf("Logout() error = %v", err)
	}
	_, err = client.RefreshSession(ctx, &identityv1.RefreshSessionRequest{
		RefreshToken: logoutSession.GetRefreshToken(),
	})
	if got, want := status.Code(err), codes.Unauthenticated; got != want {
		t.Fatalf("refresh after logout code = %s, want %s", got, want)
	}

	activeRefreshTokens := make([]string, 0, 2)
	for range 2 {
		response, err := client.Login(ctx, &identityv1.LoginRequest{
			Email:    "family@example.com",
			Password: "correct horse battery staple",
		})
		if err != nil {
			t.Fatalf("Login() before logout all error = %v", err)
		}
		activeRefreshTokens = append(activeRefreshTokens, response.GetRefreshToken())
	}
	listSessionsResponse, err = client.ListSessions(ctx, &identityv1.ListSessionsRequest{
		UserId: registerResponse.GetUser().GetId(),
	})
	if err != nil {
		t.Fatalf("ListSessions() before revoke error = %v", err)
	}
	if len(listSessionsResponse.GetSessions()) != 2 {
		t.Fatalf("active session count = %d, want 2", len(listSessionsResponse.GetSessions()))
	}
	if _, err := client.RevokeSession(ctx, &identityv1.RevokeSessionRequest{
		UserId:    registerResponse.GetUser().GetId(),
		SessionId: listSessionsResponse.GetSessions()[0].GetId(),
	}); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	logoutAllResponse, err := client.LogoutAll(ctx, &identityv1.LogoutAllRequest{
		UserId: registerResponse.GetUser().GetId(),
	})
	if err != nil {
		t.Fatalf("LogoutAll() error = %v", err)
	}
	if logoutAllResponse.GetRevokedSessionCount() != 1 {
		t.Fatalf("revoked session count = %d, want 1", logoutAllResponse.GetRevokedSessionCount())
	}
	for _, refreshToken := range activeRefreshTokens {
		_, err := client.RefreshSession(ctx, &identityv1.RefreshSessionRequest{
			RefreshToken: refreshToken,
		})
		if got, want := status.Code(err), codes.Unauthenticated; got != want {
			t.Fatalf("refresh after logout all code = %s, want %s", got, want)
		}
	}
}
