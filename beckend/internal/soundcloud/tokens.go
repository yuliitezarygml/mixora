package soundcloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"net/http"
	"time"
)

// SQLTokens persists encrypted tokens and serializes rotation of single-use refresh tokens.
// A lost refresh response requires operator intervention; it must not trigger a token-creation loop.
type SQLTokens struct {
	db                   *pgxpool.Pool
	id, secret, provider string
	cipher               cipher.AEAD
	http                 *http.Client
	endpoint             string
}

func NewTokens(db *pgxpool.Pool, id, secret, key string) (*SQLTokens, error) {
	decoded, err := hex.DecodeString(key)
	if err != nil || len(decoded) != 32 {
		return nil, errors.New("TOKEN_ENCRYPTION_KEY must contain 64 hexadecimal characters")
	}
	block, err := aes.NewCipher(decoded)
	if err != nil {
		return nil, err
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	hash := sha256.Sum256([]byte(id))
	return &SQLTokens{db: db, id: id, secret: secret, provider: "soundcloud:" + hex.EncodeToString(hash[:]), cipher: aead, http: &http.Client{Timeout: 10 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}, endpoint: "https://secure.soundcloud.com/oauth/token"}, nil
}
func (s *SQLTokens) AccessToken(ctx context.Context) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	tx, err := s.db.Begin(ctx)
	if err != nil {
		return "", err
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(713026)"); err != nil {
		return "", err
	}
	var encrypted []byte
	var token Token
	err = tx.QueryRow(ctx, "SELECT encrypted_token FROM provider_tokens WHERE provider=$1", s.provider).Scan(&encrypted)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return "", err
	}
	if err == nil {
		data, err := s.open(encrypted)
		if err != nil {
			return "", errors.New("cannot decrypt provider token; check TOKEN_ENCRYPTION_KEY")
		}
		if err = json.Unmarshal(data, &token); err != nil {
			return "", err
		}
		if time.Now().Add(time.Minute).Before(token.ExpiresAt) {
			return token.AccessToken, nil
		}
	}
	token, err = exchange(ctx, s.http, s.endpoint, s.id, s.secret, token)
	if err != nil {
		return "", err
	}
	data, err := json.Marshal(token)
	if err != nil {
		return "", err
	}
	encrypted, err = s.seal(data)
	if err != nil {
		return "", err
	}
	if _, err = tx.Exec(ctx, "INSERT INTO provider_tokens(provider,encrypted_token) VALUES($1,$2) ON CONFLICT(provider) DO UPDATE SET encrypted_token=$2,updated_at=now()", s.provider, encrypted); err != nil {
		return "", err
	}
	if err = tx.Commit(ctx); err != nil {
		return "", err
	}
	return token.AccessToken, nil
}
func (s *SQLTokens) seal(data []byte) ([]byte, error) {
	nonce := make([]byte, s.cipher.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, err
	}
	return s.cipher.Seal(nonce, nonce, data, []byte(s.provider)), nil
}
func (s *SQLTokens) open(data []byte) ([]byte, error) {
	n := s.cipher.NonceSize()
	if len(data) < n {
		return nil, errors.New("invalid encrypted token")
	}
	return s.cipher.Open(nil, data[:n], data[n:], []byte(s.provider))
}
