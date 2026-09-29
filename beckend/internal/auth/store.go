package auth

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrEmailTaken       = errors.New("email is already registered")
	ErrRecordNotFound   = errors.New("record not found")
	ErrTokenUnavailable = errors.New("token is expired, consumed, or invalid")
)

type User struct {
	ID              string     `json:"id"`
	Email           string     `json:"email"`
	DisplayName     string     `json:"display_name"`
	EmailVerifiedAt *time.Time `json:"email_verified_at,omitempty"`
	Status          string     `json:"status"`
	Plus            bool       `json:"plus"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

type Session struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	DeviceID   string    `json:"device_id,omitempty"`
	UserAgent  string    `json:"user_agent,omitempty"`
	IPAddress  string    `json:"ip_address,omitempty"`
	ExpiresAt  time.Time `json:"expires_at"`
	LastSeenAt time.Time `json:"last_seen_at"`
	CreatedAt  time.Time `json:"created_at"`
}

type SessionMetadata struct {
	DeviceID  string
	UserAgent string
	IPAddress string
}

type Store struct {
	pool *pgxpool.Pool
}

func NewStore(pool *pgxpool.Pool) (*Store, error) {
	if pool == nil {
		return nil, fmt.Errorf("auth store requires a database pool")
	}
	return &Store{pool: pool}, nil
}

func (s *Store) CreateUserWithVerification(
	ctx context.Context,
	email string,
	passwordHash string,
	displayName string,
	tokenHash TokenDigest,
	tokenExpiresAt time.Time,
) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin user registration: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := scanUser(tx.QueryRow(ctx, `
		INSERT INTO users (email, password_hash, display_name)
		VALUES ($1, $2, $3)
		RETURNING id::text, email, display_name, email_verified_at,
		          status, plus, created_at, updated_at`,
		email, passwordHash, displayName,
	))
	if err != nil {
		if isUniqueViolation(err) {
			return User{}, ErrEmailTaken
		}
		return User{}, fmt.Errorf("insert user: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_tokens (user_id, purpose, token_hash, expires_at)
		VALUES ($1, 'verify', $2, $3)`,
		user.ID, tokenHash.Bytes(), tokenExpiresAt,
	); err != nil {
		return User{}, fmt.Errorf("insert verification token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit user registration: %w", err)
	}
	return user, nil
}

type userWithPassword struct {
	User
	PasswordHash string
}

func (s *Store) UserByEmail(ctx context.Context, email string) (userWithPassword, error) {
	var result userWithPassword
	var verified sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, password_hash, display_name, email_verified_at,
		       status, plus, created_at, updated_at
		FROM users
		WHERE lower(email) = lower($1)`, email,
	).Scan(
		&result.ID,
		&result.Email,
		&result.PasswordHash,
		&result.DisplayName,
		&verified,
		&result.Status,
		&result.Plus,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return userWithPassword{}, ErrRecordNotFound
	}
	if err != nil {
		return userWithPassword{}, fmt.Errorf("find user by email: %w", err)
	}
	if verified.Valid {
		result.EmailVerifiedAt = &verified.Time
	}
	return result, nil
}

func (s *Store) UserByID(ctx context.Context, userID string) (userWithPassword, error) {
	var result userWithPassword
	var verified sql.NullTime
	err := s.pool.QueryRow(ctx, `
		SELECT id::text, email, password_hash, display_name, email_verified_at,
		       status, plus, created_at, updated_at
		FROM users
		WHERE id = $1`, userID,
	).Scan(
		&result.ID,
		&result.Email,
		&result.PasswordHash,
		&result.DisplayName,
		&verified,
		&result.Status,
		&result.Plus,
		&result.CreatedAt,
		&result.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return userWithPassword{}, ErrRecordNotFound
	}
	if err != nil {
		return userWithPassword{}, fmt.Errorf("find user by id: %w", err)
	}
	if verified.Valid {
		result.EmailVerifiedAt = &verified.Time
	}
	return result, nil
}

func (s *Store) CreateSession(
	ctx context.Context,
	userID string,
	tokenHash TokenDigest,
	metadata SessionMetadata,
	expiresAt time.Time,
) (Session, error) {
	var session Session
	err := s.pool.QueryRow(ctx, `
		INSERT INTO sessions (
			user_id, token_hash, device_id, user_agent, ip_address, expires_at
		)
		VALUES ($1, $2, NULLIF($3, ''), NULLIF($4, ''), NULLIF($5, '')::inet, $6)
		RETURNING id::text, user_id::text, COALESCE(device_id, ''),
		          COALESCE(user_agent, ''), COALESCE(ip_address::text, ''),
		          expires_at, last_seen_at, created_at`,
		userID,
		tokenHash.Bytes(),
		metadata.DeviceID,
		metadata.UserAgent,
		metadata.IPAddress,
		expiresAt,
	).Scan(
		&session.ID,
		&session.UserID,
		&session.DeviceID,
		&session.UserAgent,
		&session.IPAddress,
		&session.ExpiresAt,
		&session.LastSeenAt,
		&session.CreatedAt,
	)
	if err != nil {
		return Session{}, fmt.Errorf("create session: %w", err)
	}
	return session, nil
}

func (s *Store) UserAndSessionByToken(
	ctx context.Context,
	tokenHash TokenDigest,
	now time.Time,
) (User, Session, error) {
	var user User
	var session Session
	var verified sql.NullTime
	err := s.pool.QueryRow(ctx, `
		WITH touched AS (
			UPDATE sessions
			SET last_seen_at = $2
			WHERE token_hash = $1
			  AND revoked_at IS NULL
			  AND expires_at > $2
			RETURNING id, user_id, device_id, user_agent, ip_address,
			          expires_at, last_seen_at, created_at
		)
		SELECT u.id::text, u.email, u.display_name, u.email_verified_at,
		       u.status, u.plus, u.created_at, u.updated_at,
		       t.id::text, t.user_id::text, COALESCE(t.device_id, ''),
		       COALESCE(t.user_agent, ''), COALESCE(t.ip_address::text, ''),
		       t.expires_at, t.last_seen_at, t.created_at
		FROM touched t
		JOIN users u ON u.id = t.user_id
		WHERE u.status = 'active'`, tokenHash.Bytes(), now,
	).Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&verified,
		&user.Status,
		&user.Plus,
		&user.CreatedAt,
		&user.UpdatedAt,
		&session.ID,
		&session.UserID,
		&session.DeviceID,
		&session.UserAgent,
		&session.IPAddress,
		&session.ExpiresAt,
		&session.LastSeenAt,
		&session.CreatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, Session{}, ErrRecordNotFound
	}
	if err != nil {
		return User{}, Session{}, fmt.Errorf("find session: %w", err)
	}
	if verified.Valid {
		user.EmailVerifiedAt = &verified.Time
	}
	return user, session, nil
}

func (s *Store) RevokeSession(ctx context.Context, tokenHash TokenDigest, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE token_hash = $1`, tokenHash.Bytes(), now,
	)
	if err != nil {
		return fmt.Errorf("revoke session: %w", err)
	}
	return nil
}

func (s *Store) RevokeUserSessions(ctx context.Context, userID string, now time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1`, userID, now,
	)
	if err != nil {
		return fmt.Errorf("revoke user sessions: %w", err)
	}
	return nil
}

func (s *Store) CreateEmailToken(
	ctx context.Context,
	userID string,
	purpose string,
	tokenHash TokenDigest,
	expiresAt time.Time,
	now time.Time,
) error {
	if purpose != "verify" && purpose != "reset" {
		return fmt.Errorf("unsupported email token purpose %q", purpose)
	}
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return fmt.Errorf("begin email token: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	if _, err := tx.Exec(ctx, `
		UPDATE email_tokens
		SET consumed_at = $3
		WHERE user_id = $1 AND purpose = $2 AND consumed_at IS NULL`,
		userID, purpose, now,
	); err != nil {
		return fmt.Errorf("invalidate old email tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		INSERT INTO email_tokens (user_id, purpose, token_hash, expires_at)
		VALUES ($1, $2, $3, $4)`,
		userID, purpose, tokenHash.Bytes(), expiresAt,
	); err != nil {
		return fmt.Errorf("insert email token: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return fmt.Errorf("commit email token: %w", err)
	}
	return nil
}

func (s *Store) ConsumeVerificationToken(
	ctx context.Context,
	tokenHash TokenDigest,
	now time.Time,
) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin email verification: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE email_tokens
		SET consumed_at = $2
		WHERE token_hash = $1
		  AND purpose = 'verify'
		  AND consumed_at IS NULL
		  AND expires_at > $2
		RETURNING user_id::text`, tokenHash.Bytes(), now,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrTokenUnavailable
	}
	if err != nil {
		return User{}, fmt.Errorf("consume verification token: %w", err)
	}

	user, err := scanUser(tx.QueryRow(ctx, `
		UPDATE users
		SET email_verified_at = COALESCE(email_verified_at, $2), updated_at = $2
		WHERE id = $1
		RETURNING id::text, email, display_name, email_verified_at,
		          status, plus, created_at, updated_at`, userID, now,
	))
	if err != nil {
		return User{}, fmt.Errorf("verify user email: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit email verification: %w", err)
	}
	return user, nil
}

func (s *Store) ConsumePasswordResetToken(
	ctx context.Context,
	tokenHash TokenDigest,
	passwordHash string,
	now time.Time,
) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin password reset: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var userID string
	err = tx.QueryRow(ctx, `
		UPDATE email_tokens
		SET consumed_at = $2
		WHERE token_hash = $1
		  AND purpose = 'reset'
		  AND consumed_at IS NULL
		  AND expires_at > $2
		RETURNING user_id::text`, tokenHash.Bytes(), now,
	).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrTokenUnavailable
	}
	if err != nil {
		return User{}, fmt.Errorf("consume password reset token: %w", err)
	}

	user, err := updatePassword(ctx, tx, userID, passwordHash, now)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE email_tokens
		SET consumed_at = COALESCE(consumed_at, $2)
		WHERE user_id = $1 AND purpose = 'reset'`, userID, now,
	); err != nil {
		return User{}, fmt.Errorf("invalidate password reset tokens: %w", err)
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1`, userID, now,
	); err != nil {
		return User{}, fmt.Errorf("revoke sessions after password reset: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit password reset: %w", err)
	}
	return user, nil
}

func (s *Store) UpdatePassword(
	ctx context.Context,
	userID string,
	passwordHash string,
	now time.Time,
) (User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return User{}, fmt.Errorf("begin password change: %w", err)
	}
	defer func() { _ = tx.Rollback(ctx) }()

	user, err := updatePassword(ctx, tx, userID, passwordHash, now)
	if err != nil {
		return User{}, err
	}
	if _, err := tx.Exec(ctx, `
		UPDATE sessions
		SET revoked_at = COALESCE(revoked_at, $2)
		WHERE user_id = $1`, userID, now,
	); err != nil {
		return User{}, fmt.Errorf("revoke sessions after password change: %w", err)
	}
	if err := tx.Commit(ctx); err != nil {
		return User{}, fmt.Errorf("commit password change: %w", err)
	}
	return user, nil
}

func updatePassword(
	ctx context.Context,
	tx pgx.Tx,
	userID string,
	passwordHash string,
	now time.Time,
) (User, error) {
	user, err := scanUser(tx.QueryRow(ctx, `
		UPDATE users
		SET password_hash = $2, updated_at = $3
		WHERE id = $1 AND status = 'active'
		RETURNING id::text, email, display_name, email_verified_at,
		          status, plus, created_at, updated_at`, userID, passwordHash, now,
	))
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrRecordNotFound
	}
	if err != nil {
		return User{}, fmt.Errorf("update password: %w", err)
	}
	return user, nil
}

type rowScanner interface {
	Scan(dest ...any) error
}

func scanUser(row rowScanner) (User, error) {
	var user User
	var verified sql.NullTime
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.DisplayName,
		&verified,
		&user.Status,
		&user.Plus,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		return User{}, err
	}
	if verified.Valid {
		user.EmailVerifiedAt = &verified.Time
	}
	return user, nil
}

func isUniqueViolation(err error) bool {
	var pgError *pgconn.PgError
	return errors.As(err, &pgError) && pgError.Code == "23505"
}
