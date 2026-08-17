package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/ZheglY/family-tree-identity-service/internal/identity/application"
	"github.com/ZheglY/family-tree-identity-service/internal/identity/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const uniqueViolationCode = "23505"

type Repository struct {
	pool *pgxpool.Pool
}

func NewRepository(pool *pgxpool.Pool) *Repository {
	return &Repository{pool: pool}
}

func (r *Repository) CreateRegistration(
	ctx context.Context,
	record application.RegistrationRecord,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin registration transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	user := record.User
	if _, err := tx.Exec(ctx, `
		INSERT INTO users (
			id, email, normalized_email, display_name, status,
			created_at, updated_at, version
		) VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`,
		user.ID,
		user.Email.String(),
		user.Email.Normalized(),
		user.DisplayName,
		user.Status,
		user.CreatedAt,
		user.UpdatedAt,
		user.Version,
	); err != nil {
		if isConstraint(err, "users_normalized_email_active_uq") {
			return domain.ErrEmailAlreadyExists
		}
		return fmt.Errorf("insert user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO user_credentials (
			user_id, password_hash, password_changed_at, created_at, updated_at
		) VALUES ($1, $2, $3, $3, $3)
	`,
		user.ID,
		record.PasswordHash,
		user.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert user credentials: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO one_time_tokens (
			id, user_id, purpose, token_hash, expires_at, created_at
		) VALUES ($1, $2, 'verify_email', $3, $4, $5)
	`,
		record.VerificationTokenID,
		user.ID,
		record.VerificationTokenHash,
		record.VerificationExpiresAt,
		user.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert verification token: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit registration transaction: %w", err)
	}

	return nil
}

func (r *Repository) VerifyEmail(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) (domain.User, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return domain.User{}, fmt.Errorf("begin email verification transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		tokenID     uuid.UUID
		expiresAt   time.Time
		usedAt      *time.Time
		userID      uuid.UUID
		emailValue  string
		displayName string
		status      domain.UserStatus
		verifiedAt  *time.Time
		createdAt   time.Time
		updatedAt   time.Time
		deletedAt   *time.Time
		version     int
	)

	err = tx.QueryRow(ctx, `
		SELECT
			t.id, t.expires_at, t.used_at,
			u.id, u.email, u.display_name, u.status,
			u.email_verified_at, u.created_at, u.updated_at, u.deleted_at, u.version
		FROM one_time_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		  AND t.purpose = 'verify_email'
		  AND u.deleted_at IS NULL
		FOR UPDATE OF t, u
	`, tokenHash).Scan(
		&tokenID,
		&expiresAt,
		&usedAt,
		&userID,
		&emailValue,
		&displayName,
		&status,
		&verifiedAt,
		&createdAt,
		&updatedAt,
		&deletedAt,
		&version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrVerificationTokenInvalid
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select verification token: %w", err)
	}

	if usedAt != nil {
		return domain.User{}, domain.ErrVerificationTokenUsed
	}
	if !now.Before(expiresAt) {
		return domain.User{}, domain.ErrVerificationTokenExpired
	}

	if _, err := tx.Exec(ctx, `
		UPDATE users
		SET status = 'active',
			email_verified_at = $2,
			updated_at = $2,
			version = version + 1
		WHERE id = $1
	`, userID, now); err != nil {
		return domain.User{}, fmt.Errorf("activate user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE one_time_tokens
		SET used_at = $2
		WHERE id = $1
	`, tokenID, now); err != nil {
		return domain.User{}, fmt.Errorf("mark verification token used: %w", err)
	}

	if err := tx.Commit(ctx); err != nil {
		return domain.User{}, fmt.Errorf("commit email verification transaction: %w", err)
	}

	email, err := domain.NewEmail(emailValue)
	if err != nil {
		return domain.User{}, fmt.Errorf("read stored email: %w", err)
	}

	return domain.User{
		ID:              userID,
		Email:           email,
		DisplayName:     displayName,
		Status:          domain.UserStatusActive,
		EmailVerifiedAt: &now,
		CreatedAt:       createdAt,
		UpdatedAt:       now,
		DeletedAt:       deletedAt,
		Version:         version + 1,
	}, nil
}

func isConstraint(err error, constraint string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) &&
		postgresError.Code == uniqueViolationCode &&
		postgresError.ConstraintName == constraint
}
