package application

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/google/uuid"
)

type repositoryStub struct {
	created RegistrationRecord
	user    domain.User
	err     error
}

func (r *repositoryStub) CreateRegistration(
	_ context.Context,
	record RegistrationRecord,
) error {
	r.created = record
	return r.err
}

func (r *repositoryStub) VerifyEmail(
	context.Context,
	string,
	time.Time,
) (domain.User, error) {
	return r.user, r.err
}

type passwordHasherStub struct {
	hash string
	err  error
}

func (h passwordHasherStub) Hash(string) (string, error) {
	return h.hash, h.err
}

type tokenGeneratorStub struct{}

func (tokenGeneratorStub) Generate() (string, string, error) {
	return "raw-token", "token-hash", nil
}

func (tokenGeneratorStub) Hash(string) string {
	return "token-hash"
}

type mailerStub struct {
	email domain.Email
	token string
	err   error
}

func (m *mailerStub) SendVerification(
	_ context.Context,
	email domain.Email,
	token string,
) error {
	m.email = email
	m.token = token
	return m.err
}

func TestRegisterCreatesPendingUser(t *testing.T) {
	repository := &repositoryStub{}
	mailer := &mailerStub{}
	service := NewService(
		repository,
		passwordHasherStub{hash: "password-hash"},
		tokenGeneratorStub{},
		mailer,
	)

	fixedNow := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	ids := []uuid.UUID{uuid.New(), uuid.New()}
	service.now = func() time.Time { return fixedNow }
	service.newID = func() uuid.UUID {
		id := ids[0]
		ids = ids[1:]
		return id
	}

	user, err := service.Register(context.Background(), RegisterCommand{
		Email:       "Family@Example.COM",
		Password:    "correct horse battery staple",
		DisplayName: " Family Member ",
	})
	if err != nil {
		t.Fatalf("Register() error = %v", err)
	}

	if user.Status != domain.UserStatusPending {
		t.Fatalf("user status = %q, want pending", user.Status)
	}
	if repository.created.PasswordHash != "password-hash" {
		t.Fatalf("stored password hash = %q", repository.created.PasswordHash)
	}
	if repository.created.VerificationTokenHash != "token-hash" {
		t.Fatalf("stored token hash = %q", repository.created.VerificationTokenHash)
	}
	if got, want := repository.created.VerificationExpiresAt, fixedNow.Add(24*time.Hour); !got.Equal(want) {
		t.Fatalf("token expiry = %s, want %s", got, want)
	}
	if mailer.token != "raw-token" {
		t.Fatalf("mailer token = %q, want raw token", mailer.token)
	}
}

func TestRegisterDoesNotSendEmailWhenRepositoryFails(t *testing.T) {
	repository := &repositoryStub{err: domain.ErrEmailAlreadyExists}
	mailer := &mailerStub{}
	service := NewService(
		repository,
		passwordHasherStub{hash: "password-hash"},
		tokenGeneratorStub{},
		mailer,
	)

	_, err := service.Register(context.Background(), RegisterCommand{
		Email:    "family@example.com",
		Password: "correct horse battery staple",
	})
	if !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("Register() error = %v, want duplicate email", err)
	}
	if mailer.token != "" {
		t.Fatal("mailer was called after repository failure")
	}
}
