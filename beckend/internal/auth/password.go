package auth

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"

	"golang.org/x/crypto/argon2"
)

const argon2Version = 19

var ErrInvalidPasswordHash = errors.New("invalid password hash")

// PasswordParams controls the cost of Argon2id password hashing. Memory is in
// KiB. The defaults follow OWASP's 19 MiB / 2 iteration baseline and should be
// tuned on production hardware before launch.
type PasswordParams struct {
	Memory      uint32
	Iterations  uint32
	Parallelism uint8
	SaltLength  uint32
	KeyLength   uint32
}

func DefaultPasswordParams() PasswordParams {
	return PasswordParams{
		Memory:      19 * 1024,
		Iterations:  2,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func (p PasswordParams) validate() error {
	if p.Memory < 8*1024 || p.Memory > 1024*1024 {
		return fmt.Errorf("argon2 memory must be between 8 MiB and 1 GiB")
	}
	if p.Iterations == 0 || p.Iterations > 20 {
		return fmt.Errorf("argon2 iterations must be between 1 and 20")
	}
	if p.Parallelism == 0 || p.Parallelism > 16 {
		return fmt.Errorf("argon2 parallelism must be between 1 and 16")
	}
	if p.SaltLength < 16 || p.SaltLength > 64 {
		return fmt.Errorf("argon2 salt length must be between 16 and 64 bytes")
	}
	if p.KeyLength < 16 || p.KeyLength > 64 {
		return fmt.Errorf("argon2 key length must be between 16 and 64 bytes")
	}
	return nil
}

// HashPassword returns a PHC-formatted Argon2id hash using crypto/rand.
func HashPassword(password string, params PasswordParams) (string, error) {
	return hashPassword(password, params, rand.Reader)
}

func hashPassword(password string, params PasswordParams, random io.Reader) (string, error) {
	if err := params.validate(); err != nil {
		return "", err
	}
	if random == nil {
		return "", fmt.Errorf("password salt source is nil")
	}

	salt := make([]byte, params.SaltLength)
	if _, err := io.ReadFull(random, salt); err != nil {
		return "", fmt.Errorf("generate password salt: %w", err)
	}
	key := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		params.KeyLength,
	)

	encoding := base64.RawStdEncoding
	return fmt.Sprintf(
		"$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s",
		argon2Version,
		params.Memory,
		params.Iterations,
		params.Parallelism,
		encoding.EncodeToString(salt),
		encoding.EncodeToString(key),
	), nil
}

// VerifyPassword compares a password with a PHC-formatted Argon2id hash. It
// rejects unreasonable cost parameters before allocating memory, preventing a
// corrupted database value from becoming a denial-of-service primitive.
func VerifyPassword(encodedHash, password string) (bool, error) {
	params, salt, expected, err := decodePasswordHash(encodedHash)
	if err != nil {
		return false, err
	}
	actual := argon2.IDKey(
		[]byte(password),
		salt,
		params.Iterations,
		params.Memory,
		params.Parallelism,
		uint32(len(expected)),
	)
	return subtle.ConstantTimeCompare(expected, actual) == 1, nil
}

func decodePasswordHash(encodedHash string) (PasswordParams, []byte, []byte, error) {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 || parts[0] != "" || parts[1] != "argon2id" {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	if parts[2] != "v="+strconv.Itoa(argon2Version) {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	var memory, iterations uint32
	var parallelism uint8
	if _, err := fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism); err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	// Sscanf accepts trailing input, so compare with the canonical representation.
	if parts[3] != fmt.Sprintf("m=%d,t=%d,p=%d", memory, iterations, parallelism) {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	encoding := base64.RawStdEncoding
	salt, err := encoding.Strict().DecodeString(parts[4])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	expected, err := encoding.Strict().DecodeString(parts[5])
	if err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}

	params := PasswordParams{
		Memory:      memory,
		Iterations:  iterations,
		Parallelism: parallelism,
		SaltLength:  uint32(len(salt)),
		KeyLength:   uint32(len(expected)),
	}
	if err := params.validate(); err != nil {
		return PasswordParams{}, nil, nil, ErrInvalidPasswordHash
	}
	return params, salt, expected, nil
}
