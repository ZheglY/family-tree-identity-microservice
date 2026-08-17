package grpc

import (
	"context"
	"errors"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type IdentityApplication interface {
	Register(context.Context, application.RegisterCommand) (domain.User, error)
	VerifyEmail(context.Context, string) (domain.User, error)
}

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	application IdentityApplication
	log         *zap.Logger
}

func NewServer(
	identityApplication IdentityApplication,
	log *zap.Logger,
) *Server {
	return &Server{
		application: identityApplication,
		log:         log.Named("identity_grpc"),
	}
}

func (s *Server) Register(
	ctx context.Context,
	request *identityv1.RegisterRequest,
) (*identityv1.RegisterResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	user, err := s.application.Register(ctx, application.RegisterCommand{
		Email:       request.GetEmail(),
		Password:    request.GetPassword(),
		DisplayName: request.GetDisplayName(),
	})
	if err != nil {
		return nil, s.mapError("register user", err)
	}

	return &identityv1.RegisterResponse{
		User:                 mapUser(user),
		VerificationRequired: true,
	}, nil
}

func (s *Server) VerifyEmail(
	ctx context.Context,
	request *identityv1.VerifyEmailRequest,
) (*identityv1.VerifyEmailResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	user, err := s.application.VerifyEmail(ctx, request.GetToken())
	if err != nil {
		return nil, s.mapError("verify email", err)
	}

	return &identityv1.VerifyEmailResponse{User: mapUser(user)}, nil
}

func (s *Server) mapError(operation string, err error) error {
	switch {
	case errors.Is(err, domain.ErrInvalidEmail):
		return status.Error(codes.InvalidArgument, "email is invalid")
	case errors.Is(err, domain.ErrInvalidDisplayName):
		return status.Error(codes.InvalidArgument, "display name is invalid")
	case errors.Is(err, domain.ErrWeakPassword):
		return status.Error(codes.InvalidArgument, "password does not satisfy policy")
	case errors.Is(err, domain.ErrEmailAlreadyExists):
		return status.Error(codes.AlreadyExists, "email is already registered")
	case errors.Is(err, domain.ErrVerificationTokenInvalid):
		return status.Error(codes.InvalidArgument, "verification token is invalid")
	case errors.Is(err, domain.ErrVerificationTokenExpired),
		errors.Is(err, domain.ErrVerificationTokenUsed):
		return status.Error(codes.FailedPrecondition, "verification token cannot be used")
	default:
		s.log.Error(operation, zap.Error(err))
		return status.Error(codes.Internal, "internal server error")
	}
}

func mapUser(user domain.User) *identityv1.User {
	return &identityv1.User{
		Id:          user.ID.String(),
		Email:       user.Email.String(),
		DisplayName: user.DisplayName,
		Status:      mapUserStatus(user.Status),
	}
}

func mapUserStatus(userStatus domain.UserStatus) identityv1.UserStatus {
	switch userStatus {
	case domain.UserStatusPending:
		return identityv1.UserStatus_USER_STATUS_PENDING
	case domain.UserStatusActive:
		return identityv1.UserStatus_USER_STATUS_ACTIVE
	case domain.UserStatusBlocked:
		return identityv1.UserStatus_USER_STATUS_BLOCKED
	case domain.UserStatusDeleting:
		return identityv1.UserStatus_USER_STATUS_DELETING
	default:
		return identityv1.UserStatus_USER_STATUS_UNSPECIFIED
	}
}
