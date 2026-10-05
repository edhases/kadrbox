package postgres

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/edhases/kadrbox-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

// pgUniqueViolation is SQLSTATE 23505. Kept as a named constant so the
// classification below never re-derives it from an error string.
const pgUniqueViolation = "23505"

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
	// ErrEmailTaken is the same sentinel as ErrUserAlreadyExists under a name
	// that says which unique constraint was hit; existing callers comparing
	// against ErrUserAlreadyExists keep working.
	ErrEmailTaken    = ErrUserAlreadyExists
	ErrTelegramTaken = errors.New("telegram id is already linked to another account")
	ErrDiscordTaken  = errors.New("discord id is already linked to another account")

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
		RETURNING id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, normalizeEmail(email), passwordHash, username))
}

// GetUserByEmail шукає користувача за email.
// LOWER() on both sides because the stored value is normalised on write but
// pre-existing rows are not rewritten (see migration 000006): a case-different
// request must still find the account instead of creating a second one.
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, email))
}

// GetUserByID отримує профіль за UUID
func (r *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
		FROM users WHERE id = $1
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, id))
}

// MarkEmailVerified підтверджує email і видаляє всі токени цього юзера.
// One statement, therefore one transaction: the two previous separate Exec
// calls could leave a verified user with a live token when the DELETE failed,
// and the caller saw an error for a change that had already happened.
func (r *UserRepository) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	// The token DELETE is a data-modifying CTE: PostgreSQL always runs it to
	// completion even though the outer SELECT does not read its output.
	const query = `
		WITH verified AS (
			UPDATE users SET is_verified = TRUE, updated_at = NOW()
			WHERE id = $1
			RETURNING id
		), purged AS (
			DELETE FROM email_verifications WHERE user_id = $1
		)
		SELECT COUNT(*) FROM verified
	`
	var updated int
	if err := r.pool.QueryRow(ctx, query, userID).Scan(&updated); err != nil {
		return fmt.Errorf("mark email verified: %w", err)
	}
	if updated == 0 {
		return ErrUserNotFound
	}
	return nil
}

// CreateVerificationToken зберігає токен підтвердження (TTL 24 год), замінюючи попередній.
// A single upsert instead of DELETE-then-INSERT: the old pair threw the DELETE
// error away, so a failing INSERT left the user with no token at all while the
// handler had already answered 200.
func (r *UserRepository) CreateVerificationToken(ctx context.Context, userID uuid.UUID, token string) error {
	const query = `
		INSERT INTO email_verifications (user_id, token, expires_at)
		VALUES ($1, $2, NOW() + INTERVAL '24 hours')
		ON CONFLICT (user_id) DO UPDATE
		SET token = EXCLUDED.token,
		    expires_at = EXCLUDED.expires_at,
		    created_at = NOW()
	`
	if _, err := r.pool.Exec(ctx, query, userID, token); err != nil {
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
		RETURNING id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
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

// ConsumePasswordResetTokenAndUpdatePassword атомарно спалює токен скидання
// пароля і записує новий хеш.
//
// The handler used to call MarkPasswordResetUsed and UpdatePassword as two
// independent statements and only checked the first error, so a replayed reset
// link stayed usable for the whole 1-hour TTL. Here the DELETE ... RETURNING is
// the transaction's gate: if the token is unknown or expired nothing is written
// at all, and a second call with the same token cannot succeed because the
// first one consumed the row.
// ConsumePasswordResetTokenAndUpdatePassword burns the token and writes the new
// hash in one transaction, and returns the affected user id so the caller can
// revoke that account's sessions without a second lookup.
func (r *UserRepository) ConsumePasswordResetTokenAndUpdatePassword(ctx context.Context, token, newPasswordHash string) (uuid.UUID, error) {
	if strings.TrimSpace(token) == "" {
		return uuid.Nil, ErrResetTokenNotFound
	}

	tx, err := r.pool.Begin(ctx)
	if err != nil {
		return uuid.Nil, fmt.Errorf("begin password reset tx: %w", err)
	}
	defer func() {
		if rbErr := tx.Rollback(context.WithoutCancel(ctx)); rbErr != nil && !errors.Is(rbErr, pgx.ErrTxClosed) {
			log.Printf("[Postgres] rollback password reset tx: %v", rbErr)
		}
	}()

	// Expiry is part of the DELETE predicate, so the row is gone from the result
	// set exactly when it must not be redeemable.
	var userID uuid.UUID
	err = tx.QueryRow(ctx,
		`DELETE FROM password_resets WHERE token = $1 AND expires_at > NOW() RETURNING user_id`, token,
	).Scan(&userID)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return uuid.Nil, ErrResetTokenNotFound
		}
		return uuid.Nil, fmt.Errorf("consume password reset token: %w", err)
	}

	cmd, err := tx.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, userID, newPasswordHash)
	if err != nil {
		return uuid.Nil, fmt.Errorf("update password after reset: %w", err)
	}
	if cmd.RowsAffected() == 0 {
		return uuid.Nil, ErrUserNotFound
	}

	if err := tx.Commit(ctx); err != nil {
		return uuid.Nil, fmt.Errorf("commit password reset tx: %w", err)
	}
	return userID, nil
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
	const query = `
		INSERT INTO password_resets (user_id, token, expires_at)
		VALUES ($1, $2, NOW() + INTERVAL '1 hour')
		ON CONFLICT (user_id) DO UPDATE
		SET token = EXCLUDED.token,
		    expires_at = EXCLUDED.expires_at,
		    created_at = NOW()
	`
	if _, err := r.pool.Exec(ctx, query, userID, token); err != nil {
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

// MarkPasswordResetUsed видаляє токен після успішного скидання пароля.
// Kept for callers that must not change the password; ResetPassword should use
// ConsumePasswordResetTokenAndUpdatePassword so the burn and the write are one
// transaction.
func (r *UserRepository) MarkPasswordResetUsed(ctx context.Context, userID uuid.UUID) error {
	if _, err := r.pool.Exec(ctx, `DELETE FROM password_resets WHERE user_id = $1`, userID); err != nil {
		return fmt.Errorf("delete password reset token: %w", err)
	}
	return nil
}

// GetUserByTelegramID шукає користувача за Telegram ID
func (r *UserRepository) GetUserByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
		FROM users WHERE telegram_id = $1
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, telegramID))
}

// GetUserByDiscordID шукає користувача за Discord ID
func (r *UserRepository) GetUserByDiscordID(ctx context.Context, discordID string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
		FROM users WHERE discord_id = $1
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, discordID))
}

// LinkTelegram прив'язує Telegram ID до існуючого користувача
func (r *UserRepository) LinkTelegram(ctx context.Context, userID uuid.UUID, telegramID int64) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET telegram_id = $2, updated_at = NOW() WHERE id = $1`, userID, telegramID)
	return err
}

// LinkDiscord прив'язує Discord ID до існуючого користувача
func (r *UserRepository) LinkDiscord(ctx context.Context, userID uuid.UUID, discordID string) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET discord_id = $2, updated_at = NOW() WHERE id = $1`, userID, discordID)
	return err
}

// UnlinkTelegram відв'язує Telegram ID від користувача
func (r *UserRepository) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET telegram_id = NULL, updated_at = NOW() WHERE id = $1`, userID)
	return err
}

// UnlinkDiscord відв'язує Discord ID від користувача
func (r *UserRepository) UnlinkDiscord(ctx context.Context, userID uuid.UUID) error {
	_, err := r.pool.Exec(ctx, `UPDATE users SET discord_id = NULL, updated_at = NOW() WHERE id = $1`, userID)
	return err
}

// CreateOAuthUser створює верифікованого користувача через OAuth (Google/Telegram/Discord)
func (r *UserRepository) CreateOAuthUser(ctx context.Context, email, passwordHash, username, avatarURL string, telegramID *int64, discordID *string) (*domain.User, error) {
	query := `
		INSERT INTO users (email, password_hash, username, avatar_url, role, is_verified, telegram_id, discord_id, created_at, updated_at)
		VALUES ($1, $2, $3, NULLIF($4, ''), 'user', TRUE, $5, $6, NOW(), NOW())
		RETURNING id, email, password_hash, username, avatar_url, bio, role, is_verified, telegram_id, discord_id, created_at, updated_at
	`
	return r.scanUser(r.pool.QueryRow(ctx, query, normalizeEmail(email), passwordHash, username, avatarURL, telegramID, discordID))
}

// ---- helpers ----------------------------------------------------------------

// normalizeEmail mirrors the handler's own normalisation so the database never
// has to reason about case or stray whitespace again.
func normalizeEmail(email string) string {
	return strings.ToLower(strings.TrimSpace(email))
}

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
		&user.TelegramID,
		&user.DiscordID,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		if sentinel, ok := classifyUserConstraint(err); ok {
			return nil, sentinel
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

// classifyUserConstraint maps a failed INSERT onto the specific unique
// constraint it violated, reporting false when err is not a unique violation.
// Four different constraints can fire on this table and reporting all of them
// as "email already exists" sent password-reset mail and account-linking
// failures down the wrong path.
func classifyUserConstraint(err error) (error, bool) {
	if err == nil {
		return nil, false
	}

	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == pgUniqueViolation {
		switch pgErr.ConstraintName {
		case "users_email_key", "idx_users_email_lower":
			return ErrEmailTaken, true
		case "users_telegram_id_key":
			return ErrTelegramTaken, true
		case "users_discord_id_key":
			return ErrDiscordTaken, true
		}
		// Constraint names depend on how the table was created, so fall back to
		// matching the column name inside the constraint.
		switch {
		case strings.Contains(pgErr.ConstraintName, "email"):
			return ErrEmailTaken, true
		case strings.Contains(pgErr.ConstraintName, "telegram"):
			return ErrTelegramTaken, true
		case strings.Contains(pgErr.ConstraintName, "discord"):
			return ErrDiscordTaken, true
		}
		return ErrUserAlreadyExists, true
	}

	// Non-pgconn paths (a wrapped driver error, an already-stringified error from
	// an interceptor) still get the historical behaviour.
	if isDuplicateKeyError(err) {
		return ErrUserAlreadyExists, true
	}
	return nil, false
}

// isDuplicateKeyError reports whether err is a unique-constraint violation. The
// typed check is authoritative; the string match is a last resort for errors
// that never surfaced as *pgconn.PgError.
func isDuplicateKeyError(err error) bool {
	if err == nil {
		return false
	}
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code == pgUniqueViolation
	}
	msg := err.Error()
	return strings.Contains(msg, pgUniqueViolation) ||
		strings.Contains(msg, "duplicate key value violates unique constraint")
}
