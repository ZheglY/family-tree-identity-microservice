package accesstoken

import (
	"crypto/ed25519"
	"encoding/base64"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

func TestSignerCreatesStrictlyVerifiableAccessToken(t *testing.T) {
	publicKey, privateKey, err := ed25519.GenerateKey(nil)
	if err != nil {
		t.Fatalf("GenerateKey() error = %v", err)
	}
	signer, err := NewSigner(
		base64.StdEncoding.EncodeToString(privateKey),
		"test-key",
		"test-identity",
		"test-family-api",
		15*time.Minute,
	)
	if err != nil {
		t.Fatalf("NewSigner() error = %v", err)
	}

	now := time.Date(2026, time.August, 17, 12, 0, 0, 0, time.UTC)
	userID := uuid.New()
	sessionID := uuid.New()
	signed, expiresAt, err := signer.Sign(userID, sessionID, now)
	if err != nil {
		t.Fatalf("Sign() error = %v", err)
	}

	claims := &Claims{}
	token, err := jwt.ParseWithClaims(
		signed,
		claims,
		func(token *jwt.Token) (any, error) { return publicKey, nil },
		jwt.WithValidMethods([]string{Algorithm}),
		jwt.WithIssuer("test-identity"),
		jwt.WithAudience("test-family-api"),
		jwt.WithExpirationRequired(),
		jwt.WithTimeFunc(func() time.Time { return now.Add(time.Minute) }),
	)
	if err != nil {
		t.Fatalf("ParseWithClaims() error = %v", err)
	}
	if !token.Valid {
		t.Fatal("token is not valid")
	}
	if token.Header["typ"] != TokenType || token.Header["kid"] != "test-key" {
		t.Fatalf("unexpected token headers: %#v", token.Header)
	}
	if claims.Subject != userID.String() || claims.SessionID != sessionID.String() {
		t.Fatalf("unexpected claims: %#v", claims)
	}
	if claims.TokenUse != "access" {
		t.Fatalf("token_use = %q, want access", claims.TokenUse)
	}
	if !expiresAt.Equal(now.Add(15 * time.Minute)) {
		t.Fatalf("expires at = %s", expiresAt)
	}
}

func TestSignerRejectsInvalidConfiguration(t *testing.T) {
	if _, err := NewSigner("not-base64", "key", "issuer", "audience", time.Minute); err == nil {
		t.Fatal("NewSigner() error = nil, want error")
	}
	if _, err := NewEphemeralSigner("", "issuer", "audience", time.Minute); err == nil {
		t.Fatal("NewEphemeralSigner() error = nil, want error")
	}
}
