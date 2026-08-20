package domain

import (
	"fmt"
	"net/mail"
	"strings"
)

const maxEmailLength = 254

type Email struct {
	value      string
	normalized string
}

func NewEmail(raw string) (Email, error) {
	value := strings.TrimSpace(raw)
	if value == "" || len(value) > maxEmailLength {
		return Email{}, ErrInvalidEmail
	}

	parsed, err := mail.ParseAddress(value)
	if err != nil || parsed.Address != value {
		return Email{}, ErrInvalidEmail
	}

	parts := strings.Split(parsed.Address, "@")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return Email{}, ErrInvalidEmail
	}

	normalized := strings.ToLower(parsed.Address)
	return Email{
		value:      value,
		normalized: normalized,
	}, nil
}

func MustEmail(raw string) Email {
	email, err := NewEmail(raw)
	if err != nil {
		panic(fmt.Sprintf("invalid email %q", raw))
	}
	return email
}

func (e Email) String() string {
	return e.value
}

func (e Email) Normalized() string {
	return e.normalized
}
