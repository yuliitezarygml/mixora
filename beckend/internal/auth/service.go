package auth

import (
	"context"
	"errors"
	"fmt"
	stdmail "net/mail"
	"strings"
	"time"
	"unicode/utf8"
)

var (
	ErrInvalidCredentials = errors.New("invalid email or password")
	ErrAccountInactive    = errors.New("account is not active")
	ErrInvalidEmail       = errors.New("invalid email")
	ErrInvalidDisplayName = errors.New("invalid display name")
	ErrWeakPassword       = errors.New("password must contain between 12 and 72 bytes")
)

// This valid-but-impossible hash keeps the unknown-email login path close in
// cost to the known-email path without storing or exposing a real credential.
const dummyPasswordHash = "$argon2id$v=19$m=19456,t=2,p=1$AAAAAAAAAAAAAAAAAAAAAA$AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA"

type Options struct {
	PasswordParams   PasswordParams
	SessionTTL       time.Duration
	VerificationTTL  time.Duration
	PasswordResetTTL time.Duration
}

func DefaultOptions() Options {
	return Options{
		PasswordParams:   DefaultPasswordParams(),
		SessionTTL:       30 * 24 * time.Hour,
		VerificationTTL:  24 * time.Hour,
		PasswordResetTTL: time.Hour,
	}
}

func (o Options) validate() error {
	if err := o.PasswordParams.validate(); err != nil {
		return err
	}
	if o.SessionTTL < time.Hour {
		return fmt.Errorf("session TTL must be at least one hour")
	}
	if o.VerificationTTL <= 0 {
		return fmt.Errorf("verification TTL must be positive")
	}
	if o.PasswordResetTTL <= 0 {
		return fmt.Errorf("password reset TTL must be positive")
	}
	return nil
}

type Service struct {
	store   *Store
	options Options
	now     func() time.Time
}

func NewService(store *Store, options Options) (*Service, error) {
	if store == nil {
		return nil, fmt.Errorf("auth service requires a store")
	}
	if err := options.validate(); err != nil {
		return nil, fmt.Errorf("auth options: %w", err)
	}
	return &Service{store: store, options: options, now: time.Now}, nil
}

type Registration struct {
	User                      User
	EmailVerificationToken    string
	EmailVerificationDeadline time.Time
}

func (s *Service) Register(
	ctx context.Context,
	email string,
	password string,
	displayName string,
) (Registration, error) {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return Registration{}, err
	}
	displayName = strings.TrimSpace(displayName)
	if displayName == "" || utf8.RuneCountInString(displayName) > 80 {
		return Registration{}, ErrInvalidDisplayName
	}
	if err := validatePassword(password); err != nil {
		return Registration{}, err
	}

	passwordHash, err := HashPassword(password, s.options.PasswordParams)
	if err != nil {
		return Registration{}, fmt.Errorf("hash password: %w", err)
	}
	rawToken, tokenHash, err := NewOpaqueToken()
	if err != nil {
		return Registration{}, err
	}
	deadline := s.now().UTC().Add(s.options.VerificationTTL)
	user, err := s.store.CreateUserWithVerification(
		ctx,
		normalizedEmail,
		passwordHash,
		displayName,
		tokenHash,
		deadline,
	)
	if err != nil {
		return Registration{}, err
	}
	return Registration{
		User:                      user,
		EmailVerificationToken:    rawToken,
		EmailVerificationDeadline: deadline,
	}, nil
}

type Login struct {
	User         User
	Session      Session
	SessionToken string
}

func (s *Service) Login(
	ctx context.Context,
	email string,
	password string,
	metadata SessionMetadata,
) (Login, error) {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		_, _ = VerifyPassword(dummyPasswordHash, password)
		return Login{}, ErrInvalidCredentials
	}
	storedUser, err := s.store.UserByEmail(ctx, normalizedEmail)
	if errors.Is(err, ErrRecordNotFound) {
		_, _ = VerifyPassword(dummyPasswordHash, password)
		return Login{}, ErrInvalidCredentials
	}
	if err != nil {
		return Login{}, err
	}
	valid, err := VerifyPassword(storedUser.PasswordHash, password)
	if err != nil {
		return Login{}, fmt.Errorf("verify stored password: %w", err)
	}
	if !valid {
		return Login{}, ErrInvalidCredentials
	}
	if storedUser.Status != "active" {
		return Login{}, ErrAccountInactive
	}

	rawToken, tokenHash, err := NewOpaqueToken()
	if err != nil {
		return Login{}, err
	}
	session, err := s.store.CreateSession(
		ctx,
		storedUser.ID,
		tokenHash,
		metadata,
		s.now().UTC().Add(s.options.SessionTTL),
	)
	if err != nil {
		return Login{}, err
	}
	return Login{User: storedUser.User, Session: session, SessionToken: rawToken}, nil
}

func (s *Service) Authenticate(ctx context.Context, rawToken string) (User, Session, error) {
	tokenHash, err := DigestToken(rawToken)
	if err != nil {
		return User{}, Session{}, ErrInvalidToken
	}
	user, session, err := s.store.UserAndSessionByToken(ctx, tokenHash, s.now().UTC())
	if errors.Is(err, ErrRecordNotFound) {
		return User{}, Session{}, ErrInvalidToken
	}
	return user, session, err
}

func (s *Service) Logout(ctx context.Context, rawToken string) error {
	tokenHash, err := DigestToken(rawToken)
	if err != nil {
		// Logout is intentionally idempotent, including for malformed tokens.
		return nil
	}
	return s.store.RevokeSession(ctx, tokenHash, s.now().UTC())
}

func (s *Service) VerifyEmail(ctx context.Context, rawToken string) (User, error) {
	tokenHash, err := DigestToken(rawToken)
	if err != nil {
		return User{}, ErrTokenUnavailable
	}
	return s.store.ConsumeVerificationToken(ctx, tokenHash, s.now().UTC())
}

type PasswordReset struct {
	User      User
	Token     string
	ExpiresAt time.Time
}

// BeginPasswordReset returns nil for unknown or inactive accounts. HTTP callers
// must always return the same generic response so this result cannot be used for
// account enumeration.
func (s *Service) BeginPasswordReset(ctx context.Context, email string) (*PasswordReset, error) {
	normalizedEmail, err := normalizeEmail(email)
	if err != nil {
		return nil, nil
	}
	storedUser, err := s.store.UserByEmail(ctx, normalizedEmail)
	if errors.Is(err, ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if storedUser.Status != "active" {
		return nil, nil
	}
	rawToken, tokenHash, err := NewOpaqueToken()
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	expiresAt := now.Add(s.options.PasswordResetTTL)
	if err := s.store.CreateEmailToken(
		ctx,
		storedUser.ID,
		"reset",
		tokenHash,
		expiresAt,
		now,
	); err != nil {
		return nil, err
	}
	return &PasswordReset{User: storedUser.User, Token: rawToken, ExpiresAt: expiresAt}, nil
}

func (s *Service) ResetPassword(ctx context.Context, rawToken, password string) (User, error) {
	if err := validatePassword(password); err != nil {
		return User{}, err
	}
	tokenHash, err := DigestToken(rawToken)
	if err != nil {
		return User{}, ErrTokenUnavailable
	}
	passwordHash, err := HashPassword(password, s.options.PasswordParams)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.store.ConsumePasswordResetToken(ctx, tokenHash, passwordHash, s.now().UTC())
}

func (s *Service) ChangePassword(
	ctx context.Context,
	userID string,
	currentPassword string,
	newPassword string,
) (User, error) {
	if err := validatePassword(newPassword); err != nil {
		return User{}, err
	}
	storedUser, err := s.store.UserByID(ctx, userID)
	if errors.Is(err, ErrRecordNotFound) {
		return User{}, ErrInvalidCredentials
	}
	if err != nil {
		return User{}, err
	}
	valid, err := VerifyPassword(storedUser.PasswordHash, currentPassword)
	if err != nil {
		return User{}, fmt.Errorf("verify stored password: %w", err)
	}
	if !valid {
		return User{}, ErrInvalidCredentials
	}
	passwordHash, err := HashPassword(newPassword, s.options.PasswordParams)
	if err != nil {
		return User{}, fmt.Errorf("hash password: %w", err)
	}
	return s.store.UpdatePassword(ctx, userID, passwordHash, s.now().UTC())
}

func normalizeEmail(value string) (string, error) {
	normalized := strings.ToLower(strings.TrimSpace(value))
	if normalized == "" || len(normalized) > 320 || strings.ContainsAny(normalized, "\r\n") {
		return "", ErrInvalidEmail
	}
	address, err := stdmail.ParseAddress(normalized)
	if err != nil || address.Address != normalized {
		return "", ErrInvalidEmail
	}
	return normalized, nil
}

func validatePassword(password string) error {
	if len(password) < 12 || len(password) > 72 {
		return ErrWeakPassword
	}
	return nil
}
