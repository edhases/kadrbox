package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound       = errors.New("user not found")
	ErrUserAlreadyExists  = errors.New("user with this email already exists")
	ErrTokenNotFound      = errors.New("verification token not found or expired")
	ErrResetTokenNotFound = errors.New("reset token not found or expired")
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser створює нового користувача (is_verified = false за замовчуванням)
func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash, username string) (*domain.User, error) {
	query := `
		INSERT INTO users (email, password_hash, username, role, is_verified, created_at, updated_at)
		VALUES ($1, $2, $3, 'user', FALSE, NOW(), NOW())
		RETURNING id, email, password_hash, username, avatar_url, bio, role, is_verified, created_at, updated_at
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, email, passwordHash, username))
}

// GetUserByEmail шукає користувача за email
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, created_at, updated_at
		FROM users WHERE email = $1
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, email))
}

// GetUserByID отримує профіль за UUID
func (r *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, created_at, updated_at
		FROM users WHERE id = $1
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, id))
}

// MarkEmailVerified підтверджує email і видаляє всі токени цього юзера
func (r *UserRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx,
		`UPDATE users SET is_verified = TRUE, updated_at = NOW() WHERE id = $1`, userID)
	if err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	if _, err := r.pool.Exec(ctx,
		`DELETE FROM email_verifications WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete email verification token: %w", err)
	}
	return nil
}

// CreateVerificationToken зберігає токен підтвердження (TTL 24 год), видаляє старі
func (r *UserRepository) CreateVerificationToken(ctx context.Context, userID uuid.UUID, token string) error {
	// Видаляємо попередні токени цього юзера
	_, _ = r.pool.Exec(ctx, `DELETE FROM email_verifications WHERE user_id = $1`, userID)

	_, err := r.pool.Exec(ctx,
		`INSERT INTO email_verifications (user_id, token, expires_at)
		 VALUES ($1, $2, NOW() + INTERVAL '24 hours')`,
		userID, token)
	if err != nil {
		return fmt.Errorf("create verification token: %w", err)
	}
	return nil
}

// GetUserByVerificationToken повертає user_id якщо токен валідний (не прострочений)
func (r *UserRepository) GetUserByVerificationToken(ctx context.Context, token string) (uuid.UUID, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM email_verifications WHERE token = $1`, token,
	).Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrTokenNotFound
		}
		return uuid.Nil, fmt.Errorf("get verification token: %w", err)
	}
	if time.Now().After(expiresAt) {
		return uuid.Nil, ErrTokenNotFound
	}
	return userID, nil
}

// UpdateProfile оновлює ім'я, біографію та аватар користувача
func (r *UserRepository) UpdateProfile(ctx context.Context, id uuid.UUID, username, bio, avatarURL string) (*domain.User, error) {
	query := `
		UPDATE users
		SET username = CASE WHEN $2 <> '' THEN $2 ELSE username END,
		    bio = CASE WHEN $3 <> '' THEN $3 ELSE bio END,
		    avatar_url = CASE WHEN $4 <> '' THEN $4 ELSE avatar_url END,
		    updated_at = NOW()
		WHERE id = $1
		RETURNING id, email, password_hash, username, avatar_url, bio, role, is_verified, created_at, updated_at
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, id, username, bio, avatarURL))
}

// UpdatePassword встановлює новий хеш пароля
func (r *UserRepository) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	cmd, err := r.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, id, passwordHash)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// DeleteUser повністю видаляє користувача (CASCADE видалить пов'язані дані)
func (r *UserRepository) DeleteUser(ctx context.Context, id uuid.UUID) error {
	cmd, err := r.pool.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return ErrUserNotFound
	}
	return nil
}

// CreatePasswordResetToken створює токен відновлення пароля (дійсний 1 годину)
func (r *UserRepository) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, token string) error {
	_, _ = r.pool.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID)
	_, err := r.pool.Exec(ctx,
		`INSERT INTO password_resets (user_id, token, expires_at)
		 VALUES ($1, $2, NOW() + INTERVAL '1 hour')`,
		userID, token)
	if err != nil {
		return fmt.Errorf("create password reset token: %w", err)
	}
	return nil
}

// GetUserByPasswordResetToken повертає userID якщо токен відновлення валідний
func (r *UserRepository) GetUserByPasswordResetToken(ctx context.Context, token string) (uuid.UUID, error) {
	var userID uuid.UUID
	var expiresAt time.Time
	err := r.pool.QueryRow(ctx,
		`SELECT user_id, expires_at FROM password_resets WHERE token = $1`, token,
	).Scan(&userID, &expiresAt)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrResetTokenNotFound
		}
		return uuid.Nil, fmt.Errorf("get password reset token: %w", err)
	}
	if time.Now().After(expiresAt) {
		return uuid.Nil, ErrResetTokenNotFound
	}
	return userID, nil
}

// MarkPasswordResetUsed видаляє токен після успішного скидання пароля
func (r *UserRepository) MarkPasswordResetUsed(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete password reset token: %w", err)
	}
	return nil
}

// ---- helpers ----------------------------------------------------------------

func (r *UserRepository) scanUser(row pgx.Row) (*domain.User, error) {
	var user domain.User
	var avatarURL, bio *string
	err := row.Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Username,
		&avatarURL,
		&bio,
		&user.Role,
		&user.IsVerified,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("scan user: %w", err)
	}
	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}
	return &user, nil
}

func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return containsStr(msg, "23505") || containsStr(msg, "duplicate key value violates unique constraint")
}

func containsStr(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
