package auth

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"
)

func testPasswordParams() PasswordParams {
	return PasswordParams{
		Memory:      8 * 1024,
		Iterations:  1,
		Parallelism: 1,
		SaltLength:  16,
		KeyLength:   32,
	}
}

func TestHashAndVerifyPassword(t *testing.T) {
	t.Parallel()

	hash, err := hashPassword(
		"a sufficiently long password",
		testPasswordParams(),
		bytes.NewReader(bytes.Repeat([]byte{0x42}, 16)),
	)
	if err != nil {
		t.Fatalf("hash password: %v", err)
	}
	if !strings.HasPrefix(hash, "$argon2id$v=19$m=8192,t=1,p=1$") {
		t.Fatalf("unexpected PHC hash: %q", hash)
	}

	valid, err := VerifyPassword(hash, "a sufficiently long password")
	if err != nil {
		t.Fatalf("verify password: %v", err)
	}
	if !valid {
		t.Fatal("correct password was rejected")
	}

	valid, err = VerifyPassword(hash, "a different password")
	if err != nil {
		t.Fatalf("verify wrong password: %v", err)
	}
	if valid {
		t.Fatal("wrong password was accepted")
	}
}

func TestHashPasswordUsesFreshSalt(t *testing.T) {
	t.Parallel()

	first, err := HashPassword("a sufficiently long password", testPasswordParams())
	if err != nil {
		t.Fatalf("first hash: %v", err)
	}
	second, err := HashPassword("a sufficiently long password", testPasswordParams())
	if err != nil {
		t.Fatalf("second hash: %v", err)
	}
	if first == second {
		t.Fatal("two hashes unexpectedly use the same salt")
	}
}

func TestVerifyPasswordRejectsMalformedOrDangerousHashes(t *testing.T) {
	t.Parallel()

	cases := []string{
		"",
		"$argon2i$v=19$m=8192,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=16$m=8192,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=8192,t=1,p=1,trailing$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=1,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=1048577,t=1,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=8192,t=0,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=8192,t=1,p=0$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
		"$argon2id$v=19$m=8192,t=1,p=1$not*base64$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA",
	}
	for _, encoded := range cases {
		encoded := encoded
		t.Run(encoded, func(t *testing.T) {
			_, err := VerifyPassword(encoded, "password does not matter")
			if !errors.Is(err, ErrInvalidPasswordHash) {
				t.Fatalf("expected ErrInvalidPasswordHash, got %v", err)
			}
		})
	}
}

func TestHashPasswordReportsRandomSourceFailure(t *testing.T) {
	t.Parallel()

	_, err := hashPassword("a sufficiently long password", testPasswordParams(), errReader{})
	if err == nil {
		t.Fatal("expected random source failure")
	}
}

func TestPasswordLengthContract(t *testing.T) {
	t.Parallel()

	if !errors.Is(validatePassword(strings.Repeat("x", 11)), ErrWeakPassword) {
		t.Fatal("11-byte password must be rejected")
	}
	if err := validatePassword(strings.Repeat("x", 12)); err != nil {
		t.Fatalf("12-byte password must be accepted: %v", err)
	}
	if err := validatePassword(strings.Repeat("x", 72)); err != nil {
		t.Fatalf("72-byte password must be accepted: %v", err)
	}
	if !errors.Is(validatePassword(strings.Repeat("x", 73)), ErrWeakPassword) {
		t.Fatal("73-byte password must be rejected")
	}
}

type errReader struct{}

func (errReader) Read([]byte) (int, error) {
	return 0, io.ErrUnexpectedEOF
}
