package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type applicationStub struct {
	user domain.User
	err  error
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

func TestRegisterMapsApplicationResponse(t *testing.T) {
	user := testUser(t, domain.UserStatusPending)
	server := NewServer(applicationStub{user: user}, zap.NewNop())

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
