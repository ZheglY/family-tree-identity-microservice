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
		"TRUNCATE one_time_tokens, user_sessions, user_credentials, users CASCADE",
	); err != nil {
		t.Fatalf("truncate identity tables: %v", err)
	}
}
