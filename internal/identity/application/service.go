package application

import (
	"context"
	"errors"
	"fmt"
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
)

type Service struct {
	repository     Repository
	passwordHasher PasswordHasher
	tokenGenerator TokenGenerator
	mailer         VerificationMailer
	newID          IDGenerator
	now            Clock
}

func NewService(
	repository Repository,
	passwordHasher PasswordHasher,
	tokenGenerator TokenGenerator,
	mailer VerificationMailer,
) *Service {
	return &Service{
		repository:     repository,
		passwordHasher: passwordHasher,
		tokenGenerator: tokenGenerator,
		mailer:         mailer,
		newID:          uuid.New,
		now: func() time.Time {
			return time.Now().UTC()
		},
	}
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
