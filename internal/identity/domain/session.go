package domain

import (
	"time"

	"github.com/google/uuid"
)

type UserSession struct {
	ID         uuid.UUID
	UserID     uuid.UUID
	UserAgent  string
	IPAddress  string
	CreatedAt  time.Time
	LastUsedAt time.Time
	ExpiresAt  time.Time
}
