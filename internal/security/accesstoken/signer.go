package accesstoken

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
)

const (
	Algorithm = "EdDSA"
	TokenType = "at+jwt"
)

var ErrInvalidConfiguration = errors.New("invalid access token configuration")

type Claims struct {
	SessionID string `json:"sid"`
	TokenUse  string `json:"token_use"`
	jwt.RegisteredClaims
}

type PublicKeyInfo struct {
	KeyID           string
	Algorithm       string
	PublicKeyBase64 string
	Issuer          string
	Audience        string
}

type Signer struct {
	privateKey ed25519.PrivateKey
	publicKey  ed25519.PublicKey
	keyID      string
	issuer     string
	audience   string
	ttl        time.Duration
}

func NewSigner(
	privateKeyBase64 string,
	keyID string,
	issuer string,
	audience string,
	ttl time.Duration,
) (*Signer, error) {
	privateKeyBytes, err := decodeBase64(privateKeyBase64)
	if err != nil {
		return nil, fmt.Errorf("%w: decode private key: %v", ErrInvalidConfiguration, err)
	}
	if len(privateKeyBytes) != ed25519.PrivateKeySize {
		return nil, fmt.Errorf(
			"%w: private key must contain %d bytes",
			ErrInvalidConfiguration,
			ed25519.PrivateKeySize,
		)
	}

	return newSigner(ed25519.PrivateKey(privateKeyBytes), keyID, issuer, audience, ttl)
}

func NewEphemeralSigner(
	keyID string,
	issuer string,
	audience string,
	ttl time.Duration,
) (*Signer, error) {
	_, privateKey, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("generate Ed25519 key: %w", err)
	}

	return newSigner(privateKey, keyID, issuer, audience, ttl)
}

func newSigner(
	privateKey ed25519.PrivateKey,
	keyID string,
	issuer string,
	audience string,
	ttl time.Duration,
) (*Signer, error) {
	keyID = strings.TrimSpace(keyID)
	issuer = strings.TrimSpace(issuer)
	audience = strings.TrimSpace(audience)
	if keyID == "" || issuer == "" || audience == "" || ttl <= 0 {
		return nil, fmt.Errorf(
			"%w: key ID, issuer, audience and positive TTL are required",
			ErrInvalidConfiguration,
		)
	}

	privateKeyCopy := append(ed25519.PrivateKey(nil), privateKey...)
	publicKey, ok := privateKey.Public().(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%w: derive Ed25519 public key", ErrInvalidConfiguration)
	}

	return &Signer{
		privateKey: privateKeyCopy,
		publicKey:  append(ed25519.PublicKey(nil), publicKey...),
		keyID:      keyID,
		issuer:     issuer,
		audience:   audience,
		ttl:        ttl,
	}, nil
}

func (s *Signer) Sign(
	userID uuid.UUID,
	sessionID uuid.UUID,
	now time.Time,
) (string, time.Time, error) {
	if userID == uuid.Nil || sessionID == uuid.Nil {
		return "", time.Time{}, fmt.Errorf("sign access token: user and session IDs are required")
	}

	now = now.UTC()
	expiresAt := now.Add(s.ttl)
	claims := Claims{
		SessionID: sessionID.String(),
		TokenUse:  "access",
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer:    s.issuer,
			Subject:   userID.String(),
			Audience:  jwt.ClaimStrings{s.audience},
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			NotBefore: jwt.NewNumericDate(now),
			IssuedAt:  jwt.NewNumericDate(now),
			ID:        uuid.NewString(),
		},
	}

	token := jwt.NewWithClaims(jwt.SigningMethodEdDSA, claims)
	token.Header["kid"] = s.keyID
	token.Header["typ"] = TokenType

	value, err := token.SignedString(s.privateKey)
	if err != nil {
		return "", time.Time{}, fmt.Errorf("sign access token: %w", err)
	}

	return value, expiresAt, nil
}

func (s *Signer) PublicKeyInfo() PublicKeyInfo {
	return PublicKeyInfo{
		KeyID:           s.keyID,
		Algorithm:       Algorithm,
		PublicKeyBase64: base64.StdEncoding.EncodeToString(s.publicKey),
		Issuer:          s.issuer,
		Audience:        s.audience,
	}
}

func decodeBase64(value string) ([]byte, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return nil, errors.New("value is empty")
	}

	decoded, err := base64.StdEncoding.DecodeString(value)
	if err == nil {
		return decoded, nil
	}

	return base64.RawStdEncoding.DecodeString(value)
}
