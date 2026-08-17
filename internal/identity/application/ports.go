package application

import (
	"context"
	"time"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/google/uuid"
)

type RegistrationRecord struct {
	User                  domain.User
	PasswordHash          string
	VerificationTokenID   uuid.UUID
	VerificationTokenHash string
	VerificationExpiresAt time.Time
}

type Repository interface {
	CreateRegistration(context.Context, RegistrationRecord) error
	VerifyEmail(context.Context, string, time.Time) (domain.User, error)
}

type PasswordHasher interface {
	Hash(string) (string, error)
}

type TokenGenerator interface {
	Generate() (raw string, hash string, err error)
	Hash(raw string) string
}

type VerificationMailer interface {
	SendVerification(context.Context, domain.Email, string) error
}

type IDGenerator func() uuid.UUID
type Clock func() time.Time
