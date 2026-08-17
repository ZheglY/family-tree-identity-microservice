package token

import "testing"

func TestGenerateReturnsRawTokenAndHash(t *testing.T) {
	generator := NewGenerator()
	raw, hash, err := generator.Generate()
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if raw == "" || hash == "" {
		t.Fatal("token and hash must not be empty")
	}
	if raw == hash {
		t.Fatal("raw token must not equal stored hash")
	}
	if got := Hash(raw); got != hash {
		t.Fatalf("Hash(raw) = %q, want %q", got, hash)
	}
}
