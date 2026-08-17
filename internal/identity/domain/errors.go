package domain

import "errors"

var (
	ErrInvalidEmail             = errors.New("invalid email")
	ErrInvalidDisplayName       = errors.New("invalid display name")
	ErrWeakPassword             = errors.New("weak password")
	ErrEmailAlreadyExists       = errors.New("email already exists")
	ErrVerificationTokenInvalid = errors.New("verification token is invalid")
	ErrVerificationTokenExpired = errors.New("verification token is expired")
	ErrVerificationTokenUsed    = errors.New("verification token is already used")
)
