package postgres

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/ZheglY/family-tree-identity-service/internal/testdatabase"
	"github.com/ZheglY/family-tree-identity-service/migrations"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"go.uber.org/zap"
)

func TestRepositoryRegistrationAndVerificationIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "Family@Example.COM", "token-hash", now)

	if err := repository.CreateRegistration(context.Background(), record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}

	if err := repository.CreateRegistration(
		context.Background(),
		registrationRecord(t, "family@example.com", "another-token-hash", now),
	); !errors.Is(err, domain.ErrEmailAlreadyExists) {
		t.Fatalf("duplicate registration error = %v, want email exists", err)
	}

	user, err := repository.VerifyEmail(context.Background(), "token-hash", now.Add(time.Hour))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if user.Status != domain.UserStatusActive || user.EmailVerifiedAt == nil {
		t.Fatalf("verified user = %#v", user)
	}

	if _, err := repository.VerifyEmail(
		context.Background(),
		"token-hash",
		now.Add(2*time.Hour),
	); !errors.Is(err, domain.ErrVerificationTokenUsed) {
		t.Fatalf("reused token error = %v, want token used", err)
	}
}

func TestRepositoryRejectsExpiredVerificationTokenIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	createdAt := time.Now().UTC().Add(-48 * time.Hour)
	record := registrationRecord(t, "expired@example.com", "expired-token-hash", createdAt)
	record.VerificationExpiresAt = createdAt.Add(24 * time.Hour)
	if err := repository.CreateRegistration(context.Background(), record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}

	if _, err := repository.VerifyEmail(
		context.Background(),
		"expired-token-hash",
		time.Now().UTC(),
	); !errors.Is(err, domain.ErrVerificationTokenExpired) {
		t.Fatalf("VerifyEmail() error = %v, want token expired", err)
	}
}

func TestRepositoryRotatesRefreshTokenAndRevokesReplayIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "session@example.com", "verification-hash", now)
	if err := repository.CreateRegistration(ctx, record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}
	user, err := repository.VerifyEmail(ctx, "verification-hash", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}

	identity, err := repository.FindLoginIdentity(ctx, user.Email.Normalized())
	if err != nil {
		t.Fatalf("FindLoginIdentity() error = %v", err)
	}
	if identity.User.ID != user.ID || identity.PasswordHash != record.PasswordHash {
		t.Fatalf("login identity = %#v", identity)
	}

	sessionID := uuid.New()
	expiresAt := now.Add(30 * 24 * time.Hour)
	if err := repository.CreateSession(ctx, application.SessionRecord{
		ID:               sessionID,
		UserID:           user.ID,
		RefreshTokenHash: "refresh-hash-1",
		UserAgent:        "first browser",
		IPAddress:        "127.0.0.1",
		ExpiresAt:        expiresAt,
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	rotated, err := repository.RotateRefreshToken(
		ctx,
		"refresh-hash-1",
		"refresh-hash-2",
		"second browser",
		"2001:db8::1",
		now.Add(time.Hour),
	)
	if err != nil {
		t.Fatalf("RotateRefreshToken() error = %v", err)
	}
	if rotated.ID != sessionID || rotated.User.ID != user.ID || !rotated.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("rotated session = %#v", rotated)
	}

	var usedCount int
	if err := pool.QueryRow(ctx, `
		SELECT count(*) FROM used_refresh_tokens WHERE token_hash = 'refresh-hash-1'
	`).Scan(&usedCount); err != nil {
		t.Fatalf("count used refresh tokens: %v", err)
	}
	if usedCount != 1 {
		t.Fatalf("used refresh token count = %d, want 1", usedCount)
	}

	if _, err := repository.RotateRefreshToken(
		ctx,
		"refresh-hash-1",
		"refresh-hash-3",
		"attacker",
		"",
		now.Add(2*time.Hour),
	); !errors.Is(err, domain.ErrRefreshTokenReused) {
		t.Fatalf("replayed rotation error = %v, want refresh token reused", err)
	}
	if _, err := repository.RotateRefreshToken(
		ctx,
		"refresh-hash-2",
		"refresh-hash-4",
		"browser",
		"",
		now.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("rotation after replay error = %v, want session revoked", err)
	}
}

func TestRepositoryRevokesOneAndAllSessionsIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "logout@example.com", "logout-verification", now)
	if err := repository.CreateRegistration(ctx, record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}
	user, err := repository.VerifyEmail(ctx, "logout-verification", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}

	for index, hash := range []string{"logout-hash-1", "logout-hash-2"} {
		if err := repository.CreateSession(ctx, application.SessionRecord{
			ID:               uuid.New(),
			UserID:           user.ID,
			RefreshTokenHash: hash,
			ExpiresAt:        now.Add(24 * time.Hour),
			CreatedAt:        now.Add(time.Duration(index) * time.Second),
		}); err != nil {
			t.Fatalf("CreateSession(%d) error = %v", index, err)
		}
	}

	if err := repository.RevokeSession(ctx, "logout-hash-1", now.Add(time.Hour)); err != nil {
		t.Fatalf("RevokeSession() error = %v", err)
	}
	revoked, err := repository.RevokeAllSessions(ctx, user.ID, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("RevokeAllSessions() error = %v", err)
	}
	if revoked != 1 {
		t.Fatalf("revoked count = %d, want 1", revoked)
	}
}

func TestRepositoryGetsUserAndManagesOwnedSessionsIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	ctx := context.Background()
	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "account@example.com", "account-verification", now)
	if err := repository.CreateRegistration(ctx, record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}
	user, err := repository.VerifyEmail(ctx, "account-verification", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}

	storedUser, err := repository.GetUser(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetUser() error = %v", err)
	}
	if storedUser.ID != user.ID || storedUser.Status != domain.UserStatusActive {
		t.Fatalf("stored user = %#v", storedUser)
	}
	if _, err := repository.GetUser(ctx, uuid.New()); !errors.Is(err, domain.ErrUserNotFound) {
		t.Fatalf("missing GetUser() error = %v, want user not found", err)
	}

	activeSessionID := uuid.New()
	for _, session := range []application.SessionRecord{
		{
			ID:               activeSessionID,
			UserID:           user.ID,
			RefreshTokenHash: "account-refresh-active",
			UserAgent:        "active browser",
			IPAddress:        "127.0.0.1",
			ExpiresAt:        now.Add(24 * time.Hour),
			CreatedAt:        now,
		},
		{
			ID:               uuid.New(),
			UserID:           user.ID,
			RefreshTokenHash: "account-refresh-expired",
			UserAgent:        "expired browser",
			ExpiresAt:        now.Add(time.Hour),
			CreatedAt:        now,
		},
	} {
		if err := repository.CreateSession(ctx, session); err != nil {
			t.Fatalf("CreateSession() error = %v", err)
		}
	}

	sessions, err := repository.ListSessions(ctx, user.ID, now.Add(2*time.Hour))
	if err != nil {
		t.Fatalf("ListSessions() error = %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != activeSessionID {
		t.Fatalf("active sessions = %#v", sessions)
	}

	if err := repository.RevokeOwnedSession(
		ctx,
		uuid.New(),
		activeSessionID,
		now.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrSessionNotFound) {
		t.Fatalf("foreign revoke error = %v, want session not found", err)
	}
	if err := repository.RevokeOwnedSession(
		ctx,
		user.ID,
		activeSessionID,
		now.Add(3*time.Hour),
	); err != nil {
		t.Fatalf("RevokeOwnedSession() error = %v", err)
	}
	if err := repository.RevokeOwnedSession(
		ctx,
		user.ID,
		activeSessionID,
		now.Add(4*time.Hour),
	); err != nil {
		t.Fatalf("second RevokeOwnedSession() must be idempotent: %v", err)
	}
	sessions, err = repository.ListSessions(ctx, user.ID, now.Add(3*time.Hour))
	if err != nil {
		t.Fatalf("ListSessions() after revoke error = %v", err)
	}
	if len(sessions) != 0 {
		t.Fatalf("sessions after revoke = %#v", sessions)
	}
}

func TestRepositoryChangesPasswordAndRevokesSessionsIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "change-password@example.com", "change-verification", now)
	if err := repository.CreateRegistration(ctx, record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}
	user, err := repository.VerifyEmail(ctx, "change-verification", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	if err := repository.CreateSession(ctx, application.SessionRecord{
		ID:               uuid.New(),
		UserID:           user.ID,
		RefreshTokenHash: "change-refresh",
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	currentHash, err := repository.GetPasswordCredential(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetPasswordCredential() error = %v", err)
	}
	if err := repository.ChangePassword(
		ctx,
		user.ID,
		currentHash,
		"$argon2id$new-password-hash",
		now.Add(time.Hour),
	); err != nil {
		t.Fatalf("ChangePassword() error = %v", err)
	}
	changedHash, err := repository.GetPasswordCredential(ctx, user.ID)
	if err != nil {
		t.Fatalf("GetPasswordCredential() after change error = %v", err)
	}
	if changedHash != "$argon2id$new-password-hash" {
		t.Fatalf("changed password hash = %q", changedHash)
	}
	if _, err := repository.RotateRefreshToken(
		ctx,
		"change-refresh",
		"change-refresh-new",
		"",
		"",
		now.Add(2*time.Hour),
	); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("refresh after password change error = %v, want session revoked", err)
	}
	if err := repository.ChangePassword(
		ctx,
		user.ID,
		currentHash,
		"$argon2id$stale-write",
		now.Add(3*time.Hour),
	); !errors.Is(err, domain.ErrInvalidCredentials) {
		t.Fatalf("stale password change error = %v, want invalid credentials", err)
	}
}

func TestRepositoryResetsPasswordWithSingleUseTokenIntegration(t *testing.T) {
	pool := openTestPool(t)
	repository := NewRepository(pool)
	truncateIdentityTables(t, pool)

	ctx := context.Background()
	now := time.Date(2026, time.August, 20, 12, 0, 0, 0, time.UTC)
	record := registrationRecord(t, "reset-password@example.com", "reset-verification", now)
	if err := repository.CreateRegistration(ctx, record); err != nil {
		t.Fatalf("CreateRegistration() error = %v", err)
	}
	user, err := repository.VerifyEmail(ctx, "reset-verification", now.Add(time.Minute))
	if err != nil {
		t.Fatalf("VerifyEmail() error = %v", err)
	}
	resetUser, err := repository.FindPasswordResetUser(ctx, user.Email.Normalized())
	if err != nil || resetUser.ID != user.ID {
		t.Fatalf("FindPasswordResetUser() = %#v, %v", resetUser, err)
	}
	if err := repository.CreateSession(ctx, application.SessionRecord{
		ID:               uuid.New(),
		UserID:           user.ID,
		RefreshTokenHash: "reset-refresh",
		ExpiresAt:        now.Add(24 * time.Hour),
		CreatedAt:        now,
	}); err != nil {
		t.Fatalf("CreateSession() error = %v", err)
	}

	firstRecord := application.PasswordResetRecord{
		TokenID:   uuid.New(),
		UserID:    user.ID,
		TokenHash: "reset-hash-1",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(2 * time.Minute),
	}
	if err := repository.CreatePasswordReset(ctx, firstRecord); err != nil {
		t.Fatalf("CreatePasswordReset(first) error = %v", err)
	}
	secondRecord := application.PasswordResetRecord{
		TokenID:   uuid.New(),
		UserID:    user.ID,
		TokenHash: "reset-hash-2",
		ExpiresAt: now.Add(time.Hour),
		CreatedAt: now.Add(3 * time.Minute),
	}
	if err := repository.CreatePasswordReset(ctx, secondRecord); err != nil {
		t.Fatalf("CreatePasswordReset(second) error = %v", err)
	}
	if err := repository.ResetPassword(
		ctx,
		"reset-hash-1",
		"$argon2id$unused-password-hash",
		now.Add(4*time.Minute),
	); !errors.Is(err, domain.ErrPasswordResetTokenUsed) {
		t.Fatalf("superseded reset error = %v, want token used", err)
	}
	if err := repository.ResetPassword(
		ctx,
		"reset-hash-2",
		"$argon2id$recovered-password-hash",
		now.Add(5*time.Minute),
	); err != nil {
		t.Fatalf("ResetPassword() error = %v", err)
	}
	changedHash, err := repository.GetPasswordCredential(ctx, user.ID)
	if err != nil || changedHash != "$argon2id$recovered-password-hash" {
		t.Fatalf("password after reset = %q, %v", changedHash, err)
	}
	if _, err := repository.RotateRefreshToken(
		ctx,
		"reset-refresh",
		"reset-refresh-new",
		"",
		"",
		now.Add(6*time.Minute),
	); !errors.Is(err, domain.ErrSessionRevoked) {
		t.Fatalf("refresh after reset error = %v, want session revoked", err)
	}
	if err := repository.ResetPassword(
		ctx,
		"reset-hash-2",
		"$argon2id$reused-password-hash",
		now.Add(7*time.Minute),
	); !errors.Is(err, domain.ErrPasswordResetTokenUsed) {
		t.Fatalf("reused reset error = %v, want token used", err)
	}

	expiredRecord := application.PasswordResetRecord{
		TokenID:   uuid.New(),
		UserID:    user.ID,
		TokenHash: "reset-hash-expired",
		ExpiresAt: now.Add(10 * time.Minute),
		CreatedAt: now.Add(8 * time.Minute),
	}
	if err := repository.CreatePasswordReset(ctx, expiredRecord); err != nil {
		t.Fatalf("CreatePasswordReset(expired) error = %v", err)
	}
	if err := repository.ResetPassword(
		ctx,
		"reset-hash-expired",
		"$argon2id$late-password-hash",
		now.Add(11*time.Minute),
	); !errors.Is(err, domain.ErrPasswordResetTokenExpired) {
		t.Fatalf("expired reset error = %v, want token expired", err)
	}
}

func registrationRecord(
	t *testing.T,
	emailValue string,
	tokenHash string,
	now time.Time,
) application.RegistrationRecord {
	t.Helper()

	email, err := domain.NewEmail(emailValue)
	if err != nil {
		t.Fatalf("NewEmail() error = %v", err)
	}

	return application.RegistrationRecord{
		User:                  domain.NewUser(uuid.New(), email, "Family Member", now),
		PasswordHash:          "$argon2id$test-password-hash",
		VerificationTokenID:   uuid.New(),
		VerificationTokenHash: tokenHash,
		VerificationExpiresAt: now.Add(24 * time.Hour),
	}
}

func openTestPool(t *testing.T) *pgxpool.Pool {
	t.Helper()

	databaseURL := os.Getenv("IDENTITY_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("IDENTITY_TEST_DATABASE_URL is not set")
	}

	testDatabase, err := testdatabase.Open(context.Background(), databaseURL)
	if err != nil {
		t.Fatalf("open isolated test database: %v", err)
	}
	t.Cleanup(func() {
		if err := testDatabase.Close(); err != nil {
			t.Errorf("close isolated test database: %v", err)
		}
	})
	pool := testDatabase.Pool

	runner, err := migrations.NewRunner(pool, zap.NewNop())
	if err != nil {
		t.Fatalf("NewRunner() error = %v", err)
	}
	if err := runner.Up(context.Background()); err != nil {
		t.Fatalf("apply migrations: %v", err)
	}

	return pool
}

func truncateIdentityTables(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()

	if _, err := pool.Exec(
		context.Background(),
		"TRUNCATE used_refresh_tokens, one_time_tokens, user_sessions, user_credentials, users CASCADE",
	); err != nil {
		t.Fatalf("truncate identity tables: %v", err)
	}
}
