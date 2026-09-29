package auth

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
)

const opaqueTokenBytes = 32

var ErrInvalidToken = errors.New("invalid token")

// TokenDigest is the only representation of an opaque token that is stored in
// PostgreSQL. The raw token is returned once to the caller and must be treated
// like a password.
type TokenDigest [sha256.Size]byte

func NewOpaqueToken() (string, TokenDigest, error) {
	return generateOpaqueToken(rand.Reader)
}

func generateOpaqueToken(random io.Reader) (string, TokenDigest, error) {
	if random == nil {
		return "", TokenDigest{}, fmt.Errorf("token source is nil")
	}
	raw := make([]byte, opaqueTokenBytes)
	if _, err := io.ReadFull(random, raw); err != nil {
		return "", TokenDigest{}, fmt.Errorf("generate token: %w", err)
	}
	encoded := base64.RawURLEncoding.EncodeToString(raw)
	return encoded, sha256.Sum256(raw), nil
}

func DigestToken(encoded string) (TokenDigest, error) {
	raw, err := base64.RawURLEncoding.Strict().DecodeString(encoded)
	if err != nil || len(raw) != opaqueTokenBytes {
		return TokenDigest{}, ErrInvalidToken
	}
	return sha256.Sum256(raw), nil
}

func (d TokenDigest) Bytes() []byte {
	copyOfDigest := make([]byte, len(d))
	copy(copyOfDigest, d[:])
	return copyOfDigest
}
