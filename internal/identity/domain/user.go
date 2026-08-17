package domain

import (
	"strings"
	"time"

	"github.com/google/uuid"
)

type UserStatus string

const (
	UserStatusPending  UserStatus = "pending"
	UserStatusActive   UserStatus = "active"
	UserStatusBlocked  UserStatus = "blocked"
	UserStatusDeleting UserStatus = "deleting"
)

type User struct {
	ID              uuid.UUID
	Email           Email
	DisplayName     string
	Status          UserStatus
	EmailVerifiedAt *time.Time
	CreatedAt       time.Time
	UpdatedAt       time.Time
	DeletedAt       *time.Time
	Version         int
}

func NewUser(
	id uuid.UUID,
	email Email,
	displayName string,
	now time.Time,
) User {
	return User{
		ID:          id,
		Email:       email,
		DisplayName: strings.TrimSpace(displayName),
		Status:      UserStatusPending,
		CreatedAt:   now,
		UpdatedAt:   now,
		Version:     1,
	}
}
