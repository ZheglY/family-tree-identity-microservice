package domain

import "errors"

var (
	ErrInvalidEmail              = errors.New("invalid email")
	ErrInvalidDisplayName        = errors.New("invalid display name")
	ErrWeakPassword              = errors.New("weak password")
	ErrEmailAlreadyExists        = errors.New("email already exists")
	ErrVerificationTokenInvalid  = errors.New("verification token is invalid")
	ErrVerificationTokenExpired  = errors.New("verification token is expired")
	ErrVerificationTokenUsed     = errors.New("verification token is already used")
	ErrInvalidCredentials        = errors.New("invalid credentials")
	ErrEmailNotVerified          = errors.New("email is not verified")
	ErrAccountUnavailable        = errors.New("account is unavailable")
	ErrInvalidSessionMetadata    = errors.New("invalid session metadata")
	ErrRefreshTokenInvalid       = errors.New("refresh token is invalid")
	ErrRefreshTokenReused        = errors.New("refresh token was already used")
	ErrSessionExpired            = errors.New("session is expired")
	ErrSessionRevoked            = errors.New("session is revoked")
	ErrUserNotFound              = errors.New("user is not found")
	ErrSessionNotFound           = errors.New("session is not found")
	ErrPasswordResetTokenInvalid = errors.New("password reset token is invalid")
	ErrPasswordResetTokenExpired = errors.New("password reset token is expired")
	ErrPasswordResetTokenUsed    = errors.New("password reset token is already used")
)
