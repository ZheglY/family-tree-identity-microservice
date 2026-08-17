package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/ZheglY/family-tree-identity-service/internal/security/accesstoken"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type applicationStub struct {
	user          domain.User
	sessionResult application.SessionResult
	revokedCount  int64
	err           error
}

func (a applicationStub) Register(
	context.Context,
	application.RegisterCommand,
) (domain.User, error) {
	return a.user, a.err
}

func (a applicationStub) VerifyEmail(
	context.Context,
	string,
) (domain.User, error) {
	return a.user, a.err
}

func (a applicationStub) Login(
	context.Context,
	application.LoginCommand,
) (application.SessionResult, error) {
	return a.sessionResult, a.err
}

func (a applicationStub) RefreshSession(
	context.Context,
	application.RefreshSessionCommand,
) (application.SessionResult, error) {
	return a.sessionResult, a.err
}

func (a applicationStub) Logout(context.Context, string) error {
	return a.err
}

func (a applicationStub) LogoutAll(context.Context, uuid.UUID) (int64, error) {
	return a.revokedCount, a.err
}

func TestRegisterMapsApplicationResponse(t *testing.T) {
	user := testUser(t, domain.UserStatusPending)
	server := NewServer(applicationStub{user: user}, zap.NewNop(), testPublicKey())

	response, err := server.Register(context.Background(), &identityv1.RegisterRequest{
		Email:       user.Email.String(),
		Password:    "correct horse battery staple",
		DisplayName: user.DisplayName,
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if response.GetUser().GetId() != user.ID.String() {
		t.Fatalf("user ID = %q, want %q", response.GetUser().GetId(), user.ID)
	}
	if !response.GetVerificationRequired() {
		t.Fatal("verification_required = false, want true")
	}
}

func TestRegisterMapsDuplicateEmail(t *testing.T) {
	server := NewServer(
		applicationStub{err: domain.ErrEmailAlreadyExists},
		zap.NewNop(),
		testPublicKey(),
	)

	_, err := server.Register(context.Background(), &identityv1.RegisterRequest{})
	if got, want := status.Code(err), codes.AlreadyExists; got != want {
		t.Fatalf("gRPC code = %s, want %s", got, want)
	}
}

func TestVerifyEmailHidesUnexpectedError(t *testing.T) {
	server := NewServer(
		applicationStub{err: errors.New("database details")},
		zap.NewNop(),
		testPublicKey(),
	)

	_, err := server.VerifyEmail(
		context.Background(),
		&identityv1.VerifyEmailRequest{Token: "token"},
	)
	if got, want := status.Code(err), codes.Internal; got != want {
		t.Fatalf("gRPC code = %s, want %s", got, want)
	}
	if status.Convert(err).Message() != "internal server error" {
		t.Fatalf("unexpected public error: %v", err)
	}
}

func TestLoginMapsSessionResponse(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	result := application.SessionResult{
		User:                  testUser(t, domain.UserStatusActive),
		AccessToken:           "access-token",
		RefreshToken:          "refresh-token",
		AccessTokenExpiresAt:  now.Add(15 * time.Minute),
		RefreshTokenExpiresAt: now.Add(24 * time.Hour),
	}
	server := NewServer(
		applicationStub{sessionResult: result},
		zap.NewNop(),
		testPublicKey(),
	)

	response, err := server.Login(context.Background(), &identityv1.LoginRequest{
		Email:    "family@example.com",
		Password: "correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if response.GetAccessToken() != "access-token" || response.GetRefreshToken() != "refresh-token" {
		t.Fatalf("unexpected session response: %#v", response)
	}
}

func TestRefreshHidesReplayDetails(t *testing.T) {
	server := NewServer(
		applicationStub{err: domain.ErrRefreshTokenReused},
		zap.NewNop(),
		testPublicKey(),
	)

	_, err := server.RefreshSession(
		context.Background(),
		&identityv1.RefreshSessionRequest{RefreshToken: "replayed"},
	)
	if got, want := status.Code(err), codes.Unauthenticated; got != want {
		t.Fatalf("gRPC code = %s, want %s", got, want)
	}
}

func TestGetAccessTokenPublicKey(t *testing.T) {
	server := NewServer(applicationStub{}, zap.NewNop(), testPublicKey())

	response, err := server.GetAccessTokenPublicKey(
		context.Background(),
		&identityv1.GetAccessTokenPublicKeyRequest{},
	)
	if err != nil {
		t.Fatalf("GetAccessTokenPublicKey() error = %v", err)
	}
	if response.GetKeyId() != "test-key" || response.GetAlgorithm() != "EdDSA" {
		t.Fatalf("unexpected public key response: %#v", response)
	}
}

func testPublicKey() accesstoken.PublicKeyInfo {
	return accesstoken.PublicKeyInfo{
		KeyID:           "test-key",
		Algorithm:       "EdDSA",
		PublicKeyBase64: "public-key",
		Issuer:          "test-identity",
		Audience:        "test-family-api",
	}
}

func testUser(t *testing.T, status domain.UserStatus) domain.User {
	t.Helper()

	email, err := domain.NewEmail("family@example.com")
	if err != nil {
		t.Fatalf("NewEmail() error = %v", err)
	}
	return domain.User{
		ID:          uuid.New(),
		Email:       email,
		DisplayName: "Family Member",
		Status:      status,
		CreatedAt:   time.Now().UTC(),
		UpdatedAt:   time.Now().UTC(),
		Version:     1,
	}
}
