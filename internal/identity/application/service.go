package application

import (
	"context"
	"errors"
	"fmt"
	"net/netip"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	passwordsecurity "github.com/ZheglY/family-tree-identity-service/internal/security/password"
	"github.com/google/uuid"
)

const (
	verificationTokenTTL = 24 * time.Hour
	maxDisplayNameLength = 100
	maxRawTokenLength    = 256
	maxPasswordBytes     = 1024
	maxUserAgentBytes    = 512
)

type Service struct {
	repository     Repository
	passwordHasher PasswordHasher
	tokenGenerator TokenGenerator
	accessSigner   AccessTokenSigner
	mailer         VerificationMailer
	refreshTTL     time.Duration
	newID          IDGenerator
	now            Clock
}

func NewService(
	repository Repository,
	passwordHasher PasswordHasher,
	tokenGenerator TokenGenerator,
	accessSigner AccessTokenSigner,
	mailer VerificationMailer,
	refreshTTL time.Duration,
) *Service {
	return &Service{
		repository:     repository,
		passwordHasher: passwordHasher,
		tokenGenerator: tokenGenerator,
		accessSigner:   accessSigner,
		mailer:         mailer,
		refreshTTL:     refreshTTL,
		newID:          uuid.New,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
}

type SessionMetadata struct {
	UserAgent string
	IPAddress string
}

type LoginCommand struct {
	Email    string
	Password string
	SessionMetadata
}

type RefreshSessionCommand struct {
	RefreshToken string
	SessionMetadata
}

type SessionResult struct {
	User                  domain.User
	AccessToken           string
	RefreshToken          string
	AccessTokenExpiresAt  time.Time
	RefreshTokenExpiresAt time.Time
}

type RegisterCommand struct {
	Email       string
	Password    string
	DisplayName string
}

func (s *Service) Register(
	ctx context.Context,
	command RegisterCommand,
) (domain.User, error) {
	email, err := domain.NewEmail(command.Email)
	if err != nil {
		return domain.User{}, err
	}

	displayName := strings.TrimSpace(command.DisplayName)
	if utf8.RuneCountInString(displayName) > maxDisplayNameLength {
		return domain.User{}, domain.ErrInvalidDisplayName
	}

	passwordHash, err := s.passwordHasher.Hash(command.Password)
	if err != nil {
		if errors.Is(err, passwordsecurity.ErrPolicy) {
			return domain.User{}, fmt.Errorf("%w: %v", domain.ErrWeakPassword, err)
		}
		return domain.User{}, fmt.Errorf("hash password: %w", err)
	}

	rawToken, tokenHash, err := s.tokenGenerator.Generate()
	if err != nil {
		return domain.User{}, fmt.Errorf("generate verification token: %w", err)
	}

	now := s.now()
	user := domain.NewUser(s.newID(), email, displayName, now)
	record := RegistrationRecord{
		User:                  user,
		PasswordHash:          passwordHash,
		VerificationTokenID:   s.newID(),
		VerificationTokenHash: tokenHash,
		VerificationExpiresAt: now.Add(verificationTokenTTL),
	}

	if err := s.repository.CreateRegistration(ctx, record); err != nil {
		return domain.User{}, err
	}

	if err := s.mailer.SendVerification(ctx, email, rawToken); err != nil {
		return domain.User{}, fmt.Errorf("send verification email: %w", err)
	}

	return user, nil
}

func (s *Service) VerifyEmail(
	ctx context.Context,
	rawToken string,
) (domain.User, error) {
	if rawToken == "" || len(rawToken) > maxRawTokenLength {
		return domain.User{}, domain.ErrVerificationTokenInvalid
	}

	return s.repository.VerifyEmail(
		ctx,
		s.tokenGenerator.Hash(rawToken),
		s.now(),
	)
}

func (s *Service) Login(
	ctx context.Context,
	command LoginCommand,
) (SessionResult, error) {
	metadata, err := normalizeSessionMetadata(command.SessionMetadata)
	if err != nil {
		return SessionResult{}, err
	}

	email, err := domain.NewEmail(command.Email)
	if err != nil {
		return SessionResult{}, s.rejectInvalidCredentials(command.Password)
	}

	identity, err := s.repository.FindLoginIdentity(ctx, email.Normalized())
	if errors.Is(err, domain.ErrInvalidCredentials) {
		return SessionResult{}, s.rejectInvalidCredentials(command.Password)
	}
	if err != nil {
		return SessionResult{}, err
	}

	if len(command.Password) > maxPasswordBytes {
		return SessionResult{}, s.rejectInvalidCredentials("")
	}
	passwordMatches, err := s.passwordHasher.Verify(
		command.Password,
		identity.PasswordHash,
	)
	if err != nil {
		return SessionResult{}, fmt.Errorf("verify password: %w", err)
	}
	if !passwordMatches {
		return SessionResult{}, domain.ErrInvalidCredentials
	}

	switch identity.User.Status {
	case domain.UserStatusPending:
		return SessionResult{}, domain.ErrEmailNotVerified
	case domain.UserStatusActive:
		// The account can create sessions.
	default:
		return SessionResult{}, domain.ErrAccountUnavailable
	}

	rawRefreshToken, refreshTokenHash, err := s.tokenGenerator.Generate()
	if err != nil {
		return SessionResult{}, fmt.Errorf("generate refresh token: %w", err)
	}

	now := s.now()
	sessionID := s.newID()
	accessToken, accessExpiresAt, err := s.accessSigner.Sign(
		identity.User.ID,
		sessionID,
		now,
	)
	if err != nil {
		return SessionResult{}, fmt.Errorf("issue access token: %w", err)
	}
	refreshExpiresAt := now.Add(s.refreshTTL)
	if err := s.repository.CreateSession(ctx, SessionRecord{
		ID:               sessionID,
		UserID:           identity.User.ID,
		RefreshTokenHash: refreshTokenHash,
		UserAgent:        metadata.UserAgent,
		IPAddress:        metadata.IPAddress,
		ExpiresAt:        refreshExpiresAt,
		CreatedAt:        now,
	}); err != nil {
		return SessionResult{}, err
	}

	return SessionResult{
		User:                  identity.User,
		AccessToken:           accessToken,
		RefreshToken:          rawRefreshToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: refreshExpiresAt,
	}, nil
}

func (s *Service) RefreshSession(
	ctx context.Context,
	command RefreshSessionCommand,
) (SessionResult, error) {
	if command.RefreshToken == "" || len(command.RefreshToken) > maxRawTokenLength {
		return SessionResult{}, domain.ErrRefreshTokenInvalid
	}
	metadata, err := normalizeSessionMetadata(command.SessionMetadata)
	if err != nil {
		return SessionResult{}, err
	}

	newRawToken, newTokenHash, err := s.tokenGenerator.Generate()
	if err != nil {
		return SessionResult{}, fmt.Errorf("generate refresh token: %w", err)
	}

	now := s.now()
	session, err := s.repository.RotateRefreshToken(
		ctx,
		s.tokenGenerator.Hash(command.RefreshToken),
		newTokenHash,
		metadata.UserAgent,
		metadata.IPAddress,
		now,
	)
	if err != nil {
		return SessionResult{}, err
	}

	accessToken, accessExpiresAt, err := s.accessSigner.Sign(
		session.User.ID,
		session.ID,
		now,
	)
	if err != nil {
		return SessionResult{}, fmt.Errorf("issue access token: %w", err)
	}

	return SessionResult{
		User:                  session.User,
		AccessToken:           accessToken,
		RefreshToken:          newRawToken,
		AccessTokenExpiresAt:  accessExpiresAt,
		RefreshTokenExpiresAt: session.ExpiresAt,
	}, nil
}

func (s *Service) Logout(ctx context.Context, rawRefreshToken string) error {
	if rawRefreshToken == "" || len(rawRefreshToken) > maxRawTokenLength {
		return nil
	}

	return s.repository.RevokeSession(
		ctx,
		s.tokenGenerator.Hash(rawRefreshToken),
		s.now(),
	)
}

func (s *Service) LogoutAll(
	ctx context.Context,
	userID uuid.UUID,
) (int64, error) {
	if userID == uuid.Nil {
		return 0, domain.ErrInvalidCredentials
	}

	return s.repository.RevokeAllSessions(ctx, userID, s.now())
}

func (s *Service) GetUser(
	ctx context.Context,
	userID uuid.UUID,
) (domain.User, error) {
	if userID == uuid.Nil {
		return domain.User{}, domain.ErrUserNotFound
	}
	return s.repository.GetUser(ctx, userID)
}

func (s *Service) ListSessions(
	ctx context.Context,
	userID uuid.UUID,
) ([]domain.UserSession, error) {
	if userID == uuid.Nil {
		return nil, domain.ErrUserNotFound
	}
	return s.repository.ListSessions(ctx, userID, s.now())
}

func (s *Service) RevokeSession(
	ctx context.Context,
	userID uuid.UUID,
	sessionID uuid.UUID,
) error {
	if userID == uuid.Nil || sessionID == uuid.Nil {
		return domain.ErrSessionNotFound
	}
	return s.repository.RevokeOwnedSession(ctx, userID, sessionID, s.now())
}

func (s *Service) rejectInvalidCredentials(candidate string) error {
	if len(candidate) > maxPasswordBytes {
		candidate = ""
	}
	if _, err := s.passwordHasher.Verify(candidate, s.passwordHasher.DummyHash()); err != nil {
		return fmt.Errorf("verify dummy password: %w", err)
	}

	return domain.ErrInvalidCredentials
}

func normalizeSessionMetadata(metadata SessionMetadata) (SessionMetadata, error) {
	metadata.UserAgent = strings.TrimSpace(metadata.UserAgent)
	metadata.IPAddress = strings.TrimSpace(metadata.IPAddress)
	if len(metadata.UserAgent) > maxUserAgentBytes {
		return SessionMetadata{}, domain.ErrInvalidSessionMetadata
	}
	if metadata.IPAddress == "" {
		return metadata, nil
	}

	address, err := netip.ParseAddr(metadata.IPAddress)
	if err != nil || address.Zone() != "" {
		return SessionMetadata{}, domain.ErrInvalidSessionMetadata
	}
	metadata.IPAddress = address.String()

	return metadata, nil
}
