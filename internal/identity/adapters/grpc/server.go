package grpc

import (
	"context"
	"errors"

	identityv1 "github.com/ZheglY/family-tree-identity-service/gen/identity/v1"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/ZheglY/family-tree-identity-service/internal/security/accesstoken"
	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type IdentityApplication interface {
	Register(context.Context, application.RegisterCommand) (domain.User, error)
	VerifyEmail(context.Context, string) (domain.User, error)
	Login(context.Context, application.LoginCommand) (application.SessionResult, error)
	RefreshSession(context.Context, application.RefreshSessionCommand) (application.SessionResult, error)
	Logout(context.Context, string) error
	LogoutAll(context.Context, uuid.UUID) (int64, error)
	GetUser(context.Context, uuid.UUID) (domain.User, error)
	ListSessions(context.Context, uuid.UUID) ([]domain.UserSession, error)
	RevokeSession(context.Context, uuid.UUID, uuid.UUID) error
	ChangePassword(context.Context, application.ChangePasswordCommand) error
	ForgotPassword(context.Context, string) error
	ResetPassword(context.Context, application.ResetPasswordCommand) error
}

type Server struct {
	identityv1.UnimplementedIdentityServiceServer
	application IdentityApplication
	log         *zap.Logger
	publicKey   accesstoken.PublicKeyInfo
}

func NewServer(
	identityApplication IdentityApplication,
	log *zap.Logger,
	publicKey accesstoken.PublicKeyInfo,
) *Server {
	return &Server{
		application: identityApplication,
		log:         log.Named("identity_grpc"),
		publicKey:   publicKey,
	}
}

func (s *Server) Login(
	ctx context.Context,
	request *identityv1.LoginRequest,
) (*identityv1.SessionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	result, err := s.application.Login(ctx, application.LoginCommand{
		Email:    request.GetEmail(),
		Password: request.GetPassword(),
		SessionMetadata: application.SessionMetadata{
			UserAgent: request.GetUserAgent(),
			IPAddress: request.GetIpAddress(),
		},
	})
	if err != nil {
		return nil, s.mapError("login", err)
	}

	return mapSession(result), nil
}

func (s *Server) RefreshSession(
	ctx context.Context,
	request *identityv1.RefreshSessionRequest,
) (*identityv1.SessionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}

	result, err := s.application.RefreshSession(ctx, application.RefreshSessionCommand{
		RefreshToken: request.GetRefreshToken(),
		SessionMetadata: application.SessionMetadata{
			UserAgent: request.GetUserAgent(),
			IPAddress: request.GetIpAddress(),
		},
	})
	if err != nil {
		return nil, s.mapError("refresh session", err)
	}

	return mapSession(result), nil
}

func (s *Server) Logout(
	ctx context.Context,
	request *identityv1.LogoutRequest,
) (*identityv1.LogoutResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if err := s.application.Logout(ctx, request.GetRefreshToken()); err != nil {
		return nil, s.mapError("logout", err)
	}

	return &identityv1.LogoutResponse{}, nil
}

func (s *Server) LogoutAll(
	ctx context.Context,
	request *identityv1.LogoutAllRequest,
) (*identityv1.LogoutAllResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is invalid")
	}

	revokedCount, err := s.application.LogoutAll(ctx, userID)
	if err != nil {
		return nil, s.mapError("logout all", err)
	}

	return &identityv1.LogoutAllResponse{RevokedSessionCount: revokedCount}, nil
}

func (s *Server) GetAccessTokenPublicKey(
	context.Context,
	*identityv1.GetAccessTokenPublicKeyRequest,
) (*identityv1.GetAccessTokenPublicKeyResponse, error) {
	return &identityv1.GetAccessTokenPublicKeyResponse{
		KeyId:           s.publicKey.KeyID,
		Algorithm:       s.publicKey.Algorithm,
		PublicKeyBase64: s.publicKey.PublicKeyBase64,
		Issuer:          s.publicKey.Issuer,
		Audience:        s.publicKey.Audience,
	}, nil
}

func (s *Server) GetUser(
	ctx context.Context,
	request *identityv1.GetUserRequest,
) (*identityv1.GetUserResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is invalid")
	}

	user, err := s.application.GetUser(ctx, userID)
	if err != nil {
		return nil, s.mapError("get user", err)
	}

	return &identityv1.GetUserResponse{User: mapUser(user)}, nil
}

func (s *Server) ListSessions(
	ctx context.Context,
	request *identityv1.ListSessionsRequest,
) (*identityv1.ListSessionsResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is invalid")
	}

	sessions, err := s.application.ListSessions(ctx, userID)
	if err != nil {
		return nil, s.mapError("list sessions", err)
	}
	responseSessions := make([]*identityv1.UserSession, 0, len(sessions))
	for _, session := range sessions {
		responseSessions = append(responseSessions, &identityv1.UserSession{
			Id:             session.ID.String(),
			UserAgent:      session.UserAgent,
			IpAddress:      session.IPAddress,
			CreatedAtUnix:  session.CreatedAt.Unix(),
			LastUsedAtUnix: session.LastUsedAt.Unix(),
			ExpiresAtUnix:  session.ExpiresAt.Unix(),
		})
	}

	return &identityv1.ListSessionsResponse{Sessions: responseSessions}, nil
}

func (s *Server) RevokeSession(
	ctx context.Context,
	request *identityv1.RevokeSessionRequest,
) (*identityv1.RevokeSessionResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is invalid")
	}
	sessionID, err := uuid.Parse(request.GetSessionId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "session ID is invalid")
	}
	if err := s.application.RevokeSession(ctx, userID, sessionID); err != nil {
		return nil, s.mapError("revoke session", err)
	}

	return &identityv1.RevokeSessionResponse{}, nil
}

func (s *Server) ChangePassword(
	ctx context.Context,
	request *identityv1.ChangePasswordRequest,
) (*identityv1.ChangePasswordResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	userID, err := uuid.Parse(request.GetUserId())
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, "user ID is invalid")
	}

	if err := s.application.ChangePassword(ctx, application.ChangePasswordCommand{
		UserID:          userID,
		CurrentPassword: request.GetCurrentPassword(),
		NewPassword:     request.GetNewPassword(),
	}); err != nil {
		return nil, s.mapError("change password", err)
	}
	return &identityv1.ChangePasswordResponse{}, nil
}

func (s *Server) ForgotPassword(
	ctx context.Context,
	request *identityv1.ForgotPasswordRequest,
) (*identityv1.ForgotPasswordResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if err := s.application.ForgotPassword(ctx, request.GetEmail()); err != nil {
		// The public response must not reveal whether the account exists or whether
		// delivery failed. Operators still receive the original error in logs.
		s.log.Error("forgot password", zap.Error(err))
	}
	return &identityv1.ForgotPasswordResponse{}, nil
}

func (s *Server) ResetPassword(
	ctx context.Context,
	request *identityv1.ResetPasswordRequest,
) (*identityv1.ResetPasswordResponse, error) {
	if request == nil {
		return nil, status.Error(codes.InvalidArgument, "request is required")
	}
	if err := s.application.ResetPassword(ctx, application.ResetPasswordCommand{
		Token:       request.GetToken(),
		NewPassword: request.GetNewPassword(),
	}); err != nil {
		return nil, s.mapError("reset password", err)
	}
	return &identityv1.ResetPasswordResponse{}, nil
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
	case errors.Is(err, domain.ErrPasswordResetTokenInvalid):
		return status.Error(codes.InvalidArgument, "password reset token is invalid")
	case errors.Is(err, domain.ErrPasswordResetTokenExpired),
		errors.Is(err, domain.ErrPasswordResetTokenUsed):
		return status.Error(codes.FailedPrecondition, "password reset token cannot be used")
	case errors.Is(err, domain.ErrInvalidSessionMetadata):
		return status.Error(codes.InvalidArgument, "session metadata is invalid")
	case errors.Is(err, domain.ErrInvalidCredentials):
		return status.Error(codes.Unauthenticated, "email or password is invalid")
	case errors.Is(err, domain.ErrEmailNotVerified):
		return status.Error(codes.FailedPrecondition, "email verification is required")
	case errors.Is(err, domain.ErrAccountUnavailable):
		return status.Error(codes.PermissionDenied, "account is unavailable")
	case errors.Is(err, domain.ErrRefreshTokenInvalid),
		errors.Is(err, domain.ErrRefreshTokenReused),
		errors.Is(err, domain.ErrSessionExpired),
		errors.Is(err, domain.ErrSessionRevoked):
		return status.Error(codes.Unauthenticated, "session is invalid")
	case errors.Is(err, domain.ErrUserNotFound),
		errors.Is(err, domain.ErrSessionNotFound):
		return status.Error(codes.NotFound, "resource was not found")
	default:
		s.log.Error(operation, zap.Error(err))
		return status.Error(codes.Internal, "internal server error")
	}
}

func mapSession(result application.SessionResult) *identityv1.SessionResponse {
	return &identityv1.SessionResponse{
		User:                      mapUser(result.User),
		AccessToken:               result.AccessToken,
		RefreshToken:              result.RefreshToken,
		AccessTokenExpiresAtUnix:  result.AccessTokenExpiresAt.Unix(),
		RefreshTokenExpiresAtUnix: result.RefreshTokenExpiresAt.Unix(),
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
