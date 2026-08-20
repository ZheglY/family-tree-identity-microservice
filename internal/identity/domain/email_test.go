package domain

import "testing"

func TestNewEmailNormalizesForIdentity(t *testing.T) {
	email, err := NewEmail("  Family.Example@Example.COM  ")
	if err != nil {
		t.Fatalf("NewEmail() error = %v", err)
	}

	if got, want := email.String(), "Family.Example@Example.COM"; got != want {
		t.Fatalf("email value = %q, want %q", got, want)
	}
	if got, want := email.Normalized(), "family.example@example.com"; got != want {
		t.Fatalf("normalized email = %q, want %q", got, want)
	}
}

func TestNewEmailRejectsDisplayAddress(t *testing.T) {
	if _, err := NewEmail("Family <family@example.com>"); err == nil {
		t.Fatal("NewEmail() error = nil, want error")
	}
}
