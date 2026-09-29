package auth

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
	"testing"
)

func TestOpaqueTokenRoundTrip(t *testing.T) {
	t.Parallel()

	rawBytes := bytes.Repeat([]byte{0xa5}, opaqueTokenBytes)
	token, digest, err := generateOpaqueToken(bytes.NewReader(rawBytes))
	if err != nil {
		t.Fatalf("generate token: %v", err)
	}
	if strings.ContainsAny(token, "+/=") {
		t.Fatalf("token is not unpadded URL-safe base64: %q", token)
	}
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		t.Fatalf("decode token: %v", err)
	}
	if !bytes.Equal(decoded, rawBytes) {
		t.Fatalf("decoded token differs: got %x want %x", decoded, rawBytes)
	}

	wantDigest := sha256.Sum256(rawBytes)
	if digest != wantDigest {
		t.Fatalf("unexpected digest: got %x want %x", digest, wantDigest)
	}
	parsedDigest, err := DigestToken(token)
	if err != nil {
		t.Fatalf("digest token: %v", err)
	}
	if parsedDigest != digest {
		t.Fatalf("parsed digest differs: got %x want %x", parsedDigest, digest)
	}
}

func TestOpaqueTokensAreUnique(t *testing.T) {
	t.Parallel()

	first, _, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("first token: %v", err)
	}
	second, _, err := NewOpaqueToken()
	if err != nil {
		t.Fatalf("second token: %v", err)
	}
	if first == second {
		t.Fatal("two generated tokens are identical")
	}
}

func TestDigestTokenRejectsMalformedValues(t *testing.T) {
	t.Parallel()

	for _, token := range []string{
		"",
		"not*base64",
		base64.RawURLEncoding.EncodeToString(make([]byte, opaqueTokenBytes-1)),
		base64.RawURLEncoding.EncodeToString(make([]byte, opaqueTokenBytes+1)),
		base64.URLEncoding.EncodeToString(make([]byte, opaqueTokenBytes)),
	} {
		if _, err := DigestToken(token); !errors.Is(err, ErrInvalidToken) {
			t.Fatalf("DigestToken(%q): expected ErrInvalidToken, got %v", token, err)
		}
	}
}

func TestTokenDigestBytesReturnsCopy(t *testing.T) {
	t.Parallel()

	var digest TokenDigest
	digest[0] = 7
	copyOfDigest := digest.Bytes()
	copyOfDigest[0] = 99
	if digest[0] != 7 {
		t.Fatal("Bytes exposed the digest's backing array")
	}
}

func TestOpaqueTokenReportsRandomSourceFailure(t *testing.T) {
	t.Parallel()

	_, _, err := generateOpaqueToken(errReader{})
	if err == nil {
		t.Fatal("expected random source failure")
	}
}
