package password

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"golang.org/x/crypto/argon2"
)

const (
	minimumPasswordCharacters = 15
	maximumPasswordBytes      = 1024
)

var (
	ErrInvalidHash = errors.New("invalid Argon2id hash")
	ErrPolicy      = errors.New("password does not satisfy policy")
)

type Params struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

type Hasher struct {
	params Params
}

func NewHasher() *Hasher {
	return &Hasher{
		params: Params{
			Memory:      19 * 1024,
			Iterations:  2,
			Parallelism: 1,
			SaltLength:  16,
			KeyLength:   32,
		},
	}
}

func Validate(candidate string) error {
	if utf8.RuneCountInString(candidate) < minimumPasswordCharacters ||
		len([]byte(candidate)) > maximumPasswordBytes {
		return fmt.Errorf(
			"%w: password must contain at least %d characters and at most %d bytes",
			ErrPolicy,
			minimumPasswordCharacters,
			maximumPasswordBytes,
		)
	}

	return nil
}

func (h *Hasher) Hash(candidate string) (string, error) {
	if err := Validate(candidate); err != nil {
		return "", err
	}

	salt := make([]byte, h.params.SaltLength)
	if _, err := rand.Read(salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}

	derivedKey := argon2.IDKey(
		[]byte(candidate),
		salt,
		h.params.Iterations,
		h.params.Memory,
		h.params.Parallelism,
		h.params.KeyLength,
	)

	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2.Version,
		h.params.Memory,
		h.params.Iterations,
		h.params.Parallelism,
		base64.RawStdEncoding.EncodeToString(salt),
		base64.RawStdEncoding.EncodeToString(derivedKey),
	), nil
}

func (h *Hasher) Verify(candidate, encodedHash string) (bool, error) {
	params, salt, expectedKey, err := parseHash(encodedHash)
	if err != nil {
		return false, err
	}

	actualKey := argon2.IDKey(
		[]byte(candidate),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	return subtle.ConstantTimeCompare(actualKey, expectedKey) == 1, nil
}

func parseHash(encodedHash string) (Params, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[1] != "argon2id" {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var version int
	if _, err := fmt.Sscanf(parts[2], "v=%d", &version); err != nil || version != argon2.Version {
		return Params{}, nil, nil, ErrInvalidHash
	}

	var params Params
	if _, err := fmt.Sscanf(
		parts[3],
		"m=%d,t=%d,p=%d",
		&params.Memory,
		&params.Iterations,
		&params.Parallelism,
	); err != nil || params.Memory == 0 || params.Iterations == 0 || params.Parallelism == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if params.Memory > 1024*1024 || params.Iterations > 10 || params.Parallelism > 16 {
		return Params{}, nil, nil, ErrInvalidHash
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil || len(salt) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}

	expectedKey, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil || len(expectedKey) == 0 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	if len(salt) > 64 || len(expectedKey) > 64 {
		return Params{}, nil, nil, ErrInvalidHash
	}
	params.SaltLength = uint32(len(salt))
	params.KeyLength = uint32(len(expectedKey))

	return params, salt, expectedKey, nil
}
