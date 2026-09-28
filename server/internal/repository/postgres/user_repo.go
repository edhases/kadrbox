package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrUserNotFound      = errors.New("user not found")
	ErrUserAlreadyExists = errors.New("user with this email already exists")
)

type UserRepository struct {
	pool *pgxpool.Pool
}

func NewUserRepository(pool *pgxpool.Pool) *UserRepository {
	return &UserRepository{pool: pool}
}

// CreateUser створює нового користувача
func (r *UserRepository) CreateUser(ctx context.Context, email, passwordHash, username string) (*domain.User, error) {
	query := `
		INSERT INTO users (email, password_hash, username, role, created_at, updated_at)
		VALUES ($1, $2, $3, 'user', NOW(), NOW())
		RETURNING id, email, password_hash, username, avatar_url, bio, role, created_at, updated_at
	`
	var user domain.User
	var avatarURL, bio *string

	err := r.pool.QueryRow(ctx, query, email, passwordHash, username).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Username,
		&avatarURL,
		&bio,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if isDuplicateKeyError(err) {
			return nil, ErrUserAlreadyExists
		}
		return nil, fmt.Errorf("create user query: %w", err)
	}

	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}

	return &user, nil
}

// GetUserByEmail шукає користувача за email для логіну
func (r *UserRepository) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, created_at, updated_at
		FROM users
		WHERE email = $1
	`
	var user domain.User
	var avatarURL, bio *string

	err := r.pool.QueryRow(ctx, query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Username,
		&avatarURL,
		&bio,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by email: %w", err)
	}

	if avatarURL != nil {
		user.AvatarURL = *avatarURL
	}
	if bio != nil {
		user.Bio = *bio
	}

	return &user, nil
}

// GetUserByID отримує профіль за UUID
func (r *UserRepository) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	query := `
		SELECT id, email, password_hash, username, avatar_url, bio, role, created_at, updated_at
		FROM users
		WHERE id = $1
	`
	var user domain.User
	var avatarURL, bio *string

	err := r.pool.QueryRow(ctx, query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Username,
		&avatarURL,
		&bio,
		&user.Role,
		&user.CreatedAt,
		&user.UpdatedAt,
	)
	if err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, ErrUserNotFound
		}
		return nil, fmt.Errorf("get user by id: %w", err)
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
	return err != nil && (errors.Is(err, pgx.ErrNoRows) == false && (len(err.Error()) > 0 && (err.Error() == "ERROR: duplicate key value violates unique constraint \"users_email_key\" (SQLSTATE 23505)" || (len(err.Error()) > 5 && err.Error()[:5] == "23505"))))
}
