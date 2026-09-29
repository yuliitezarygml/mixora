package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/mail"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/crypto/bcrypt"
)

var (
	ErrInvalid     = errors.New("invalid email, name or password (12–72 bytes required)")
	ErrCredentials = errors.New("invalid email or password")
	ErrConflict    = errors.New("email already registered")
)

const SessionTTL = 7 * 24 * time.Hour

type User struct {
	ID          string `json:"id"`
	Email       string `json:"email"`
	DisplayName string `json:"display_name"`
	Plus        bool   `json:"plus"`
}
type Service struct {
	db        *pgxpool.Pool
	dummyHash []byte
}

func New(db *pgxpool.Pool) *Service {
	hash, _ := bcrypt.GenerateFromPassword([]byte("mixora-dummy-password"), bcrypt.DefaultCost)
	return &Service{db: db, dummyHash: hash}
}
func NormalizeEmail(email string) (string, bool) {
	email = strings.ToLower(strings.TrimSpace(email))
	parsed, err := mail.ParseAddress(email)
	return email, err == nil && parsed.Address == email && len(email) <= 254
}
func (s *Service) Register(ctx context.Context, email, password, name string) (User, error) {
	email, valid := NormalizeEmail(email)
	name = strings.TrimSpace(name)
	if !valid || len(password) < 12 || len(password) > 72 || utf8.RuneCountInString(name) < 1 || utf8.RuneCountInString(name) > 80 {
		return User{}, ErrInvalid
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return User{}, err
	}
	var u User
	err = s.db.QueryRow(ctx, "INSERT INTO users(email,password_hash,display_name) VALUES($1,$2,$3) RETURNING id,email,display_name,plus", email, string(hash), name).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Plus)
	var pgerr *pgconn.PgError
	if errors.As(err, &pgerr) && pgerr.Code == "23505" {
		return User{}, ErrConflict
	}
	return u, err
}
func (s *Service) Login(ctx context.Context, email, password string) (User, string, error) {
	email, _ = NormalizeEmail(email)
	var u User
	var hash string
	err := s.db.QueryRow(ctx, "SELECT id,email,display_name,plus,password_hash FROM users WHERE email=$1", email).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Plus, &hash)
	if errors.Is(err, pgx.ErrNoRows) {
		_ = bcrypt.CompareHashAndPassword(s.dummyHash, []byte(password))
		return u, "", ErrCredentials
	}
	if err != nil {
		return u, "", err
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		return User{}, "", ErrCredentials
	}
	token := make([]byte, 32)
	if _, err = rand.Read(token); err != nil {
		return User{}, "", err
	}
	raw := hex.EncodeToString(token)
	_, err = s.db.Exec(ctx, "INSERT INTO sessions(token_hash,user_id,expires_at) VALUES($1,$2,$3)", digest(raw), u.ID, time.Now().Add(SessionTTL))
	return u, raw, err
}
func (s *Service) Authenticate(ctx context.Context, token string) (User, error) {
	var u User
	if len(token) != 64 {
		return u, ErrCredentials
	}
	err := s.db.QueryRow(ctx, "SELECT u.id,u.email,u.display_name,u.plus FROM sessions s JOIN users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()", digest(token)).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Plus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrCredentials
	}
	return u, err
}
func (s *Service) SetPlus(ctx context.Context, userID string, plus bool) (User, error) {
	var u User
	err := s.db.QueryRow(ctx, "UPDATE users SET plus=$2 WHERE id=$1 RETURNING id,email,display_name,plus", userID, plus).Scan(&u.ID, &u.Email, &u.DisplayName, &u.Plus)
	if errors.Is(err, pgx.ErrNoRows) {
		err = ErrCredentials
	}
	return u, err
}
func (s *Service) Logout(ctx context.Context, token string) error {
	_, err := s.db.Exec(ctx, "DELETE FROM sessions WHERE token_hash=$1", digest(token))
	return err
}
func digest(token string) string { h := sha256.Sum256([]byte(token)); return hex.EncodeToString(h[:]) }
