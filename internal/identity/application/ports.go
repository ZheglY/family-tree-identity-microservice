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

type LoginIdentity struct {
	User         domain.User
	PasswordHash string
}

type SessionRecord struct {
	ID               uuid.UUID
	UserID           uuid.UUID
	RefreshTokenHash string
	UserAgent        string
	IPAddress        string
	ExpiresAt        time.Time
	CreatedAt        time.Time
}

type SessionIdentity struct {
	ID        uuid.UUID
	User      domain.User
	ExpiresAt time.Time
}

type Repository interface {
	CreateRegistration(context.Context, RegistrationRecord) error
	VerifyEmail(context.Context, string, time.Time) (domain.User, error)
	FindLoginIdentity(context.Context, string) (LoginIdentity, error)
	CreateSession(context.Context, SessionRecord) error
	RotateRefreshToken(
		context.Context,
		string,
		string,
		string,
		string,
		time.Time,
	) (SessionIdentity, error)
	RevokeSession(context.Context, string, time.Time) error
	RevokeAllSessions(context.Context, uuid.UUID, time.Time) (int64, error)
	GetUser(context.Context, uuid.UUID) (domain.User, error)
	ListSessions(context.Context, uuid.UUID, time.Time) ([]domain.UserSession, error)
	RevokeOwnedSession(context.Context, uuid.UUID, uuid.UUID, time.Time) error
}

type PasswordHasher interface {
	Hash(string) (string, error)
	Verify(string, string) (bool, error)
	DummyHash() string
}

type TokenGenerator interface {
	Generate() (raw string, hash string, err error)
	Hash(raw string) string
}

type VerificationMailer interface {
	SendVerification(context.Context, domain.Email, string) error
}

type AccessTokenSigner interface {
	Sign(uuid.UUID, uuid.UUID, time.Time) (string, time.Time, error)
}

type IDGenerator func() uuid.UUID
type Clock func() time.Time
