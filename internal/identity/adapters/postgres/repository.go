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

func (r *Repository) FindLoginIdentity(
	ctx context.Context,
	normalizedEmail string,
) (application.LoginIdentity, error) {
	var (
		user         domain.User
		emailValue   string
		passwordHash string
	)

	err := r.pool.QueryRow(ctx, `
		SELECT
			u.id, u.email, u.display_name, u.status, u.email_verified_at,
			u.created_at, u.updated_at, u.deleted_at, u.version,
			c.password_hash
		FROM users u
		JOIN user_credentials c ON c.user_id = u.id
		WHERE u.normalized_email = $1
		  AND u.deleted_at IS NULL
	`, normalizedEmail).Scan(
		&user.ID,
		&emailValue,
		&user.DisplayName,
		&user.Status,
		&user.EmailVerifiedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&user.Version,
		&passwordHash,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.LoginIdentity{}, domain.ErrInvalidCredentials
	}
	if err != nil {
		return application.LoginIdentity{}, fmt.Errorf("select login identity: %w", err)
	}

	email, err := domain.NewEmail(emailValue)
	if err != nil {
		return application.LoginIdentity{}, fmt.Errorf("read stored email: %w", err)
	}
	user.Email = email

	return application.LoginIdentity{
		User:         user,
		PasswordHash: passwordHash,
	}, nil
}

func (r *Repository) CreateSession(
	ctx context.Context,
	record application.SessionRecord,
) error {
	if _, err := r.pool.Exec(ctx, `
		INSERT INTO user_sessions (
			id, user_id, refresh_token_hash, user_agent, ip_address,
			expires_at, last_used_at, created_at
		) VALUES ($1, $2, $3, $4, NULLIF($5, '')::inet, $6, $7, $7)
	`,
		record.ID,
		record.UserID,
		record.RefreshTokenHash,
		record.UserAgent,
		record.IPAddress,
		record.ExpiresAt,
		record.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert user session: %w", err)
	}

	return nil
}

func (r *Repository) RotateRefreshToken(
	ctx context.Context,
	currentTokenHash string,
	newTokenHash string,
	userAgent string,
	ipAddress string,
	now time.Time,
) (application.SessionIdentity, error) {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return application.SessionIdentity{}, fmt.Errorf("begin refresh rotation transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		sessionID uuid.UUID
		expiresAt time.Time
		revokedAt *time.Time
		user      domain.User
		emailText string
	)
	err = tx.QueryRow(ctx, `
		SELECT
			s.id, s.expires_at, s.revoked_at,
			u.id, u.email, u.display_name, u.status, u.email_verified_at,
			u.created_at, u.updated_at, u.deleted_at, u.version
		FROM user_sessions s
		JOIN users u ON u.id = s.user_id
		WHERE s.refresh_token_hash = $1
		FOR UPDATE OF s, u
	`, currentTokenHash).Scan(
		&sessionID,
		&expiresAt,
		&revokedAt,
		&user.ID,
		&emailText,
		&user.DisplayName,
		&user.Status,
		&user.EmailVerifiedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&user.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return r.handlePossibleRefreshReuse(ctx, tx, currentTokenHash, now)
	}
	if err != nil {
		return application.SessionIdentity{}, fmt.Errorf("select user session: %w", err)
	}
	email, err := domain.NewEmail(emailText)
	if err != nil {
		return application.SessionIdentity{}, fmt.Errorf("read stored email: %w", err)
	}
	user.Email = email

	if revokedAt != nil {
		return application.SessionIdentity{}, domain.ErrSessionRevoked
	}
	if !now.Before(expiresAt) {
		return application.SessionIdentity{}, domain.ErrSessionExpired
	}
	if user.DeletedAt != nil || user.Status != domain.UserStatusActive {
		if _, err := tx.Exec(ctx, `
			UPDATE user_sessions SET revoked_at = $2 WHERE id = $1
		`, sessionID, now); err != nil {
			return application.SessionIdentity{}, fmt.Errorf("revoke unavailable account session: %w", err)
		}
		if err := tx.Commit(ctx); err != nil {
			return application.SessionIdentity{}, fmt.Errorf("commit unavailable account revocation: %w", err)
		}
		return application.SessionIdentity{}, domain.ErrAccountUnavailable
	}

	if _, err := tx.Exec(ctx, `
		INSERT INTO used_refresh_tokens (token_hash, session_id, used_at, expires_at)
		VALUES ($1, $2, $3, $4)
	`, currentTokenHash, sessionID, now, expiresAt); err != nil {
		return application.SessionIdentity{}, fmt.Errorf("record used refresh token: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE user_sessions
		SET refresh_token_hash = $2,
			user_agent = $3,
			ip_address = NULLIF($4, '')::inet,
			last_used_at = $5
		WHERE id = $1
	`, sessionID, newTokenHash, userAgent, ipAddress, now); err != nil {
		return application.SessionIdentity{}, fmt.Errorf("rotate refresh token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.SessionIdentity{}, fmt.Errorf("commit refresh rotation: %w", err)
	}

	return application.SessionIdentity{
		ID:        sessionID,
		User:      user,
		ExpiresAt: expiresAt,
	}, nil
}

func (r *Repository) handlePossibleRefreshReuse(
	ctx context.Context,
	tx pgx.Tx,
	tokenHash string,
	now time.Time,
) (application.SessionIdentity, error) {
	var sessionID uuid.UUID
	err := tx.QueryRow(ctx, `
		SELECT s.id
		FROM used_refresh_tokens t
		JOIN user_sessions s ON s.id = t.session_id
		WHERE t.token_hash = $1
		FOR UPDATE OF s
	`, tokenHash).Scan(&sessionID)
	if errors.Is(err, pgx.ErrNoRows) {
		return application.SessionIdentity{}, domain.ErrRefreshTokenInvalid
	}
	if err != nil {
		return application.SessionIdentity{}, fmt.Errorf("select used refresh token: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE id = $1
	`, sessionID, now); err != nil {
		return application.SessionIdentity{}, fmt.Errorf("revoke replayed session: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return application.SessionIdentity{}, fmt.Errorf("commit replayed session revocation: %w", err)
	}

	return application.SessionIdentity{}, domain.ErrRefreshTokenReused
}

func (r *Repository) RevokeSession(
	ctx context.Context,
	tokenHash string,
	now time.Time,
) error {
	if _, err := r.pool.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE refresh_token_hash = $1
		   OR id IN (
			SELECT session_id
			FROM used_refresh_tokens
			WHERE token_hash = $1
		   )
	`, tokenHash, now); err != nil {
		return fmt.Errorf("revoke user session: %w", err)
	}

	return nil
}

func (r *Repository) RevokeAllSessions(
	ctx context.Context,
	userID uuid.UUID,
	now time.Time,
) (int64, error) {
	result, err := r.pool.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = $2
		WHERE user_id = $1
		  AND revoked_at IS NULL
	`, userID, now)
	if err != nil {
		return 0, fmt.Errorf("revoke all user sessions: %w", err)
	}

	return result.RowsAffected(), nil
}

func (r *Repository) GetUser(
	ctx context.Context,
	userID uuid.UUID,
) (domain.User, error) {
	var (
		user      domain.User
		emailText string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT
			id, email, display_name, status, email_verified_at,
			created_at, updated_at, deleted_at, version
		FROM users
		WHERE id = $1
		  AND deleted_at IS NULL
	`, userID).Scan(
		&user.ID,
		&emailText,
		&user.DisplayName,
		&user.Status,
		&user.EmailVerifiedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&user.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user: %w", err)
	}
	email, err := domain.NewEmail(emailText)
	if err != nil {
		return domain.User{}, fmt.Errorf("read stored email: %w", err)
	}
	user.Email = email

	return user, nil
}

func (r *Repository) ListSessions(
	ctx context.Context,
	userID uuid.UUID,
	now time.Time,
) ([]domain.UserSession, error) {
	var userExists bool
	if err := r.pool.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1 FROM users WHERE id = $1 AND deleted_at IS NULL
		)
	`, userID).Scan(&userExists); err != nil {
		return nil, fmt.Errorf("check session owner: %w", err)
	}
	if !userExists {
		return nil, domain.ErrUserNotFound
	}

	rows, err := r.pool.Query(ctx, `
		SELECT
			id, user_id, user_agent, COALESCE(ip_address::text, ''),
			created_at, last_used_at, expires_at
		FROM user_sessions
		WHERE user_id = $1
		  AND revoked_at IS NULL
		  AND expires_at > $2
		ORDER BY last_used_at DESC, id
	`, userID, now)
	if err != nil {
		return nil, fmt.Errorf("select user sessions: %w", err)
	}
	defer rows.Close()

	sessions := make([]domain.UserSession, 0)
	for rows.Next() {
		var session domain.UserSession
		if err := rows.Scan(
			&session.ID,
			&session.UserID,
			&session.UserAgent,
			&session.IPAddress,
			&session.CreatedAt,
			&session.LastUsedAt,
			&session.ExpiresAt,
		); err != nil {
			return nil, fmt.Errorf("scan user session: %w", err)
		}
		sessions = append(sessions, session)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate user sessions: %w", err)
	}

	return sessions, nil
}

func (r *Repository) RevokeOwnedSession(
	ctx context.Context,
	userID uuid.UUID,
	sessionID uuid.UUID,
	now time.Time,
) error {
	result, err := r.pool.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = COALESCE(revoked_at, $3)
		WHERE id = $1
		  AND user_id = $2
	`, sessionID, userID, now)
	if err != nil {
		return fmt.Errorf("revoke owned session: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrSessionNotFound
	}

	return nil
}

func (r *Repository) GetPasswordCredential(
	ctx context.Context,
	userID uuid.UUID,
) (string, error) {
	var passwordHash string
	err := r.pool.QueryRow(ctx, `
		SELECT c.password_hash
		FROM user_credentials c
		JOIN users u ON u.id = c.user_id
		WHERE c.user_id = $1
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL
	`, userID).Scan(&passwordHash)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", domain.ErrAccountUnavailable
	}
	if err != nil {
		return "", fmt.Errorf("select password credential: %w", err)
	}
	return passwordHash, nil
}

func (r *Repository) ChangePassword(
	ctx context.Context,
	userID uuid.UUID,
	expectedCurrentHash string,
	newHash string,
	now time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password change transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	result, err := tx.Exec(ctx, `
		UPDATE user_credentials c
		SET password_hash = $3,
			password_changed_at = $4,
			updated_at = $4,
			failed_login_count = 0,
			locked_until = NULL
		FROM users u
		WHERE c.user_id = $1
		  AND c.password_hash = $2
		  AND u.id = c.user_id
		  AND u.status = 'active'
		  AND u.deleted_at IS NULL
	`, userID, expectedCurrentHash, newHash, now)
	if err != nil {
		return fmt.Errorf("update password credential: %w", err)
	}
	if result.RowsAffected() == 0 {
		return domain.ErrInvalidCredentials
	}

	if _, err := tx.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, userID, now); err != nil {
		return fmt.Errorf("revoke sessions after password change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password change transaction: %w", err)
	}
	return nil
}

func (r *Repository) FindPasswordResetUser(
	ctx context.Context,
	normalizedEmail string,
) (domain.User, error) {
	var (
		user      domain.User
		emailText string
	)
	err := r.pool.QueryRow(ctx, `
		SELECT
			id, email, display_name, status, email_verified_at,
			created_at, updated_at, deleted_at, version
		FROM users
		WHERE normalized_email = $1
		  AND deleted_at IS NULL
	`, normalizedEmail).Scan(
		&user.ID,
		&emailText,
		&user.DisplayName,
		&user.Status,
		&user.EmailVerifiedAt,
		&user.CreatedAt,
		&user.UpdatedAt,
		&user.DeletedAt,
		&user.Version,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select password reset user: %w", err)
	}
	if user.Status != domain.UserStatusActive {
		return domain.User{}, domain.ErrAccountUnavailable
	}

	email, err := domain.NewEmail(emailText)
	if err != nil {
		return domain.User{}, fmt.Errorf("read stored email: %w", err)
	}
	user.Email = email
	return user, nil
}

func (r *Repository) CreatePasswordReset(
	ctx context.Context,
	record application.PasswordResetRecord,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset request transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var lockedUserID uuid.UUID
	err = tx.QueryRow(ctx, `
		SELECT id
		FROM users
		WHERE id = $1
		  AND status = 'active'
		  AND deleted_at IS NULL
		FOR UPDATE
	`, record.UserID).Scan(&lockedUserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrAccountUnavailable
	}
	if err != nil {
		return fmt.Errorf("lock password reset user: %w", err)
	}

	if _, err := tx.Exec(ctx, `
		UPDATE one_time_tokens
		SET used_at = $2
		WHERE user_id = $1
		  AND purpose = 'reset_password'
		  AND used_at IS NULL
	`, record.UserID, record.CreatedAt); err != nil {
		return fmt.Errorf("invalidate previous password reset tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO one_time_tokens (
			id, user_id, purpose, token_hash, expires_at, created_at
		) VALUES ($1, $2, 'reset_password', $3, $4, $5)
	`,
		record.TokenID,
		record.UserID,
		record.TokenHash,
		record.ExpiresAt,
		record.CreatedAt,
	); err != nil {
		return fmt.Errorf("insert password reset token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset request transaction: %w", err)
	}
	return nil
}

func (r *Repository) ResetPassword(
	ctx context.Context,
	tokenHash string,
	newHash string,
	now time.Time,
) error {
	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin password reset transaction: %w", err)
	}
	defer func() {
		_ = tx.Rollback(ctx)
	}()

	var (
		tokenID   uuid.UUID
		userID    uuid.UUID
		expiresAt time.Time
		usedAt    *time.Time
		status    domain.UserStatus
		deletedAt *time.Time
	)
	err = tx.QueryRow(ctx, `
		SELECT
			t.id, t.user_id, t.expires_at, t.used_at,
			u.status, u.deleted_at
		FROM one_time_tokens t
		JOIN users u ON u.id = t.user_id
		WHERE t.token_hash = $1
		  AND t.purpose = 'reset_password'
		FOR UPDATE OF t, u
	`, tokenHash).Scan(
		&tokenID,
		&userID,
		&expiresAt,
		&usedAt,
		&status,
		&deletedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.ErrPasswordResetTokenInvalid
	}
	if err != nil {
		return fmt.Errorf("select password reset token: %w", err)
	}
	if usedAt != nil {
		return domain.ErrPasswordResetTokenUsed
	}
	if !now.Before(expiresAt) {
		return domain.ErrPasswordResetTokenExpired
	}
	if deletedAt != nil || status != domain.UserStatusActive {
		return domain.ErrAccountUnavailable
	}

	if _, err := tx.Exec(ctx, `
		UPDATE user_credentials
		SET password_hash = $2,
			password_changed_at = $3,
			updated_at = $3,
			failed_login_count = 0,
			locked_until = NULL
		WHERE user_id = $1
	`, userID, newHash, now); err != nil {
		return fmt.Errorf("reset password credential: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE user_sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1
	`, userID, now); err != nil {
		return fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE one_time_tokens
		SET used_at = $2
		WHERE id = $1
	`, tokenID, now); err != nil {
		return fmt.Errorf("mark password reset token used: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit password reset transaction: %w", err)
	}
	return nil
}

func isConstraint(err error, constraint string) bool {
	var postgresError *pgconn.PgError
	return errors.As(err, &postgresError) &&
		postgresError.Code == uniqueViolationCode &&
		postgresError.ConstraintName == constraint
}
