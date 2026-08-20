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
	created        RegistrationRecord
	createdSession SessionRecord
	loginIdentity  LoginIdentity
	rotatedSession SessionIdentity
	user           domain.User
	err            error
	loginErr       error
	sessionErr     error
	rotateErr      error
	revokedHash    string
	sessions       []domain.UserSession
	passwordHash   string
	changedUserID  uuid.UUID
	changedOldHash string
	changedNewHash string
	resetUser      domain.User
	resetUserErr   error
	passwordReset  PasswordResetRecord
	resetTokenHash string
	resetNewHash   string
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

func (r *repositoryStub) FindLoginIdentity(
	context.Context,
	string,
) (LoginIdentity, error) {
	return r.loginIdentity, r.loginErr
}

func (r *repositoryStub) CreateSession(_ context.Context, record SessionRecord) error {
	r.createdSession = record
	return r.sessionErr
}

func (r *repositoryStub) RotateRefreshToken(
	_ context.Context,
	currentHash string,
	_ string,
	_ string,
	_ string,
	_ time.Time,
) (SessionIdentity, error) {
	r.revokedHash = currentHash
	return r.rotatedSession, r.rotateErr
}

func (r *repositoryStub) RevokeSession(
	_ context.Context,
	tokenHash string,
	_ time.Time,
) error {
	r.revokedHash = tokenHash
	return r.sessionErr
}

func (r *repositoryStub) RevokeAllSessions(
	context.Context,
	uuid.UUID,
	time.Time,
) (int64, error) {
	return 2, r.sessionErr
}

func (r *repositoryStub) GetUser(context.Context, uuid.UUID) (domain.User, error) {
	return r.user, r.err
}

func (r *repositoryStub) ListSessions(
	context.Context,
	uuid.UUID,
	time.Time,
) ([]domain.UserSession, error) {
	return r.sessions, r.err
}

func (r *repositoryStub) RevokeOwnedSession(
	context.Context,
	uuid.UUID,
	uuid.UUID,
	time.Time,
) error {
	return r.err
}

func (r *repositoryStub) GetPasswordCredential(
	context.Context,
	uuid.UUID,
) (string, error) {
	return r.passwordHash, r.err
}

func (r *repositoryStub) ChangePassword(
	_ context.Context,
	userID uuid.UUID,
	currentHash string,
	newHash string,
	_ time.Time,
) error {
	r.changedUserID = userID
	r.changedOldHash = currentHash
	r.changedNewHash = newHash
	return r.err
}

func (r *repositoryStub) FindPasswordResetUser(
	context.Context,
	string,
) (domain.User, error) {
	return r.resetUser, r.resetUserErr
}

func (r *repositoryStub) CreatePasswordReset(
	_ context.Context,
	record PasswordResetRecord,
) error {
	r.passwordReset = record
	return r.err
}

func (r *repositoryStub) ResetPassword(
	_ context.Context,
	tokenHash string,
	newHash string,
	_ time.Time,
) error {
	r.resetTokenHash = tokenHash
	r.resetNewHash = newHash
	return r.err
}

type passwordHasherStub struct {
	hash  string
	match bool
	err   error
}

func (h passwordHasherStub) Hash(string) (string, error) {
	return h.hash, h.err
}

func (h passwordHasherStub) Verify(string, string) (bool, error) {
	return h.match, h.err
}

func (h passwordHasherStub) DummyHash() string {
	return "dummy-password-hash"
}

type tokenGeneratorStub struct{}

func (tokenGeneratorStub) Generate() (string, string, error) {
	return "raw-token", "token-hash", nil
}

func (tokenGeneratorStub) Hash(string) string {
	return "token-hash"
}

type accessSignerStub struct {
	token     string
	expiresAt time.Time
	err       error
}

func (s accessSignerStub) Sign(
	uuid.UUID,
	uuid.UUID,
	time.Time,
) (string, time.Time, error) {
	return s.token, s.expiresAt, s.err
}

type mailerStub struct {
	email      domain.Email
	token      string
	resetEmail domain.Email
	resetToken string
	err        error
}

func (m *mailerStub) SendPasswordReset(
	_ context.Context,
	email domain.Email,
	token string,
) error {
	m.resetEmail = email
	m.resetToken = token
	return m.err
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
		accessSignerStub{},
		mailer,
		30*24*time.Hour,
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
		accessSignerStub{},
		mailer,
		30*24*time.Hour,
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

func TestLoginCreatesSessionAndReturnsTokenPair(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	accessExpiresAt := now.Add(15 * time.Minute)
	user := activeUser(t, now)
	repository := &repositoryStub{
		loginIdentity: LoginIdentity{
			User:         user,
			PasswordHash: "stored-password-hash",
		},
	}
	service := NewService(
		repository,
		passwordHasherStub{match: true},
		tokenGeneratorStub{},
		accessSignerStub{token: "access-token", expiresAt: accessExpiresAt},
		&mailerStub{},
		30*24*time.Hour,
	)
	service.now = func() time.Time { return now }
	sessionID := uuid.New()
	service.newID = func() uuid.UUID { return sessionID }

	result, err := service.Login(context.Background(), LoginCommand{
		Email:    "FAMILY@example.com",
		Password: "correct horse battery staple",
		SessionMetadata: SessionMetadata{
			UserAgent: " browser ",
			IPAddress: "127.0.0.1",
		},
	})
	if err != nil {
		t.Fatalf("Login() error = %v", err)
	}
	if result.AccessToken != "access-token" || result.RefreshToken != "raw-token" {
		t.Fatalf("unexpected token pair: %#v", result)
	}
	if repository.createdSession.ID != sessionID ||
		repository.createdSession.RefreshTokenHash != "token-hash" {
		t.Fatalf("created session = %#v", repository.createdSession)
	}
	if repository.createdSession.UserAgent != "browser" {
		t.Fatalf("user agent = %q", repository.createdSession.UserAgent)
	}
	if got, want := repository.createdSession.ExpiresAt, now.Add(30*24*time.Hour); !got.Equal(want) {
		t.Fatalf("refresh expiry = %s, want %s", got, want)
	}
}

func TestLoginRejectsUnknownAccount(t *testing.T) {
	service := NewService(
		&repositoryStub{loginErr: domain.ErrInvalidCredentials},
		passwordHasherStub{},
		tokenGeneratorStub{},
		accessSignerStub{},
		&mailerStub{},
		30*24*time.Hour,
	)

	_, err := service.Login(context.Background(), LoginCommand{
		Email:    "missing@example.com",
		Password: "wrong password value",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("Login() error = %v, want invalid credentials", err)
	}
}

func TestRefreshSessionReturnsRotatedToken(t *testing.T) {
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	user := activeUser(t, now)
	repository := &repositoryStub{rotatedSession: SessionIdentity{
		ID:        uuid.New(),
		User:      user,
		ExpiresAt: now.Add(24 * time.Hour),
	}}
	service := NewService(
		repository,
		passwordHasherStub{},
		tokenGeneratorStub{},
		accessSignerStub{token: "new-access", expiresAt: now.Add(15 * time.Minute)},
		&mailerStub{},
		30*24*time.Hour,
	)
	service.now = func() time.Time { return now }

	result, err := service.RefreshSession(context.Background(), RefreshSessionCommand{
		RefreshToken: "old-refresh",
	})
	if err != nil {
		t.Fatalf("RefreshSession() error = %v", err)
	}
	if result.AccessToken != "new-access" || result.RefreshToken != "raw-token" {
		t.Fatalf("unexpected rotated tokens: %#v", result)
	}
	if repository.revokedHash != "token-hash" {
		t.Fatalf("current hash = %q, want token-hash", repository.revokedHash)
	}
}

func TestChangePasswordVerifiesCurrentPasswordAndRevokesSessions(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	repository := &repositoryStub{passwordHash: "current-hash"}
	service := NewService(
		repository,
		passwordHasherStub{hash: "new-hash", match: true},
		tokenGeneratorStub{},
		accessSignerStub{},
		&mailerStub{},
		30*24*time.Hour,
	)
	service.now = func() time.Time { return now }

	err := service.ChangePassword(context.Background(), ChangePasswordCommand{
		UserID:          userID,
		CurrentPassword: "current password value",
		NewPassword:     "new correct horse battery staple",
	})
	if err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	if repository.changedUserID != userID ||
		repository.changedOldHash != "current-hash" ||
		repository.changedNewHash != "new-hash" {
		t.Fatalf("password change = user %s old %q new %q", repository.changedUserID, repository.changedOldHash, repository.changedNewHash)
	}
}

func TestChangePasswordRejectsWrongCurrentPassword(t *testing.T) {
	repository := &repositoryStub{passwordHash: "current-hash"}
	service := NewService(
		repository,
		passwordHasherStub{hash: "new-hash", match: false},
		tokenGeneratorStub{},
		accessSignerStub{},
		&mailerStub{},
		30*24*time.Hour,
	)

	err := service.ChangePassword(context.Background(), ChangePasswordCommand{
		UserID:          uuid.New(),
		CurrentPassword: "wrong password value",
		NewPassword:     "new correct horse battery staple",
	})
	if !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("ChangePassword() error = %v, want invalid credentials", err)
	}
	if repository.changedUserID != uuid.Nil {
		t.Fatal("repository changed password after current password mismatch")
	}
}

func TestForgotPasswordCreatesExpiringHashedToken(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	user := activeUser(t, now)
	repository := &repositoryStub{resetUser: user}
	mailer := &mailerStub{}
	service := NewService(
		repository,
		passwordHasherStub{},
		tokenGeneratorStub{},
		accessSignerStub{},
		mailer,
		30*24*time.Hour,
	)
	service.now = func() time.Time { return now }
	tokenID := uuid.New()
	service.newID = func() uuid.UUID { return tokenID }

	if err := service.ForgotPassword(context.Background(), "FAMILY@example.com"); err != nil {
		t.Fatalf("ForgotPassword() error = %v", err)
	}
	if repository.passwordReset.TokenID != tokenID ||
		repository.passwordReset.UserID != user.ID ||
		repository.passwordReset.TokenHash != "token-hash" {
		t.Fatalf("password reset record = %#v", repository.passwordReset)
	}
	if got, want := repository.passwordReset.ExpiresAt, now.Add(time.Hour); !got.Equal(want) {
		t.Fatalf("reset expiry = %s, want %s", got, want)
	}
	if mailer.resetToken != "raw-token" || mailer.resetEmail != user.Email {
		t.Fatalf("reset mail = email %q token %q", mailer.resetEmail, mailer.resetToken)
	}
}

func TestForgotPasswordDoesNotRevealUnknownAccount(t *testing.T) {
	repository := &repositoryStub{resetUserErr: domain.ErrUserNotFound}
	mailer := &mailerStub{}
	service := NewService(
		repository,
		passwordHasherStub{},
		tokenGeneratorStub{},
		accessSignerStub{},
		mailer,
		30*24*time.Hour,
	)

	if err := service.ForgotPassword(context.Background(), "missing@example.com"); err != nil {
		t.Fatalf("ForgotPassword() error = %v, want generic success", err)
	}
	if mailer.resetToken != "" || repository.passwordReset.TokenID != uuid.Nil {
		t.Fatal("password reset material was created for an unknown account")
	}
}

func TestResetPasswordHashesPasswordAndConsumesToken(t *testing.T) {
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	repository := &repositoryStub{}
	service := NewService(
		repository,
		passwordHasherStub{hash: "new-hash"},
		tokenGeneratorStub{},
		accessSignerStub{},
		&mailerStub{},
		30*24*time.Hour,
	)
	service.now = func() time.Time { return now }

	if err := service.ResetPassword(context.Background(), ResetPasswordCommand{
		Token:       "raw-token",
		NewPassword: "new correct horse battery staple",
	}); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	if repository.resetTokenHash != "token-hash" || repository.resetNewHash != "new-hash" {
		t.Fatalf("reset values = token %q hash %q", repository.resetTokenHash, repository.resetNewHash)
	}
}

func activeUser(t *testing.T, now time.Time) domain.User {
	t.Helper()
	email, err := domain.NewEmail("family@example.com")
	if err != nil {
		t.Fatalf("NewEmail() error = %v", err)
	}
	user := domain.NewUser(uuid.New(), email, "Family Member", now)
	user.Status = domain.UserStatusActive
	return user
}
