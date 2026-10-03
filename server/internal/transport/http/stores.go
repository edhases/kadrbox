package http

// Narrow, consumer-side interfaces over the persistence layer.
//
// These handlers previously took concrete `*postgres.UserRepository`,
// `*postgres.FavoritesRepository`, `*postgres.HistoryRepository`,
// `*postgres.CacheRepository` and `*redisRepo.RedisClient`. That coupling made
// 20 auth endpoints, 4 sync endpoints and the search cache untestable without
// a live Postgres and Redis: every request path returned early or could not be
// constructed at all, which is why this package sat at 30% coverage.
//
// The concrete types satisfy these interfaces unchanged, so wiring in
// cmd/api is untouched. Each interface lists only the methods its consumer
// actually calls, so it cannot drift into pretending to be the whole store.

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/edhases/oxide-server/internal/domain"
)

// UserStore covers every `userRepo` call made by AuthHandler.
type UserStore interface {
	CreateUser(ctx context.Context, email, passwordHash, username string) (*domain.User, error)
	CreateOAuthUser(ctx context.Context, email, passwordHash, username, avatarURL string, telegramID *int64, discordID *string) (*domain.User, error)
	GetUserByEmail(ctx context.Context, email string) (*domain.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error)
	GetUserByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error)
	GetUserByDiscordID(ctx context.Context, discordID string) (*domain.User, error)
	UpdateProfile(ctx context.Context, id uuid.UUID, username, bio, avatarURL string) (*domain.User, error)
	UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error
	DeleteUser(ctx context.Context, id uuid.UUID) error
	MarkEmailVerified(ctx context.Context, userID uuid.UUID) error

	CreateVerificationToken(ctx context.Context, userID uuid.UUID, token string) error
	GetUserByVerificationToken(ctx context.Context, token string) (uuid.UUID, error)

	CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, token string) error
	GetUserByPasswordResetToken(ctx context.Context, token string) (uuid.UUID, error)
	MarkPasswordResetUsed(ctx context.Context, userID uuid.UUID) error
	// ConsumePasswordResetTokenAndUpdatePassword burns the reset token and writes
	// the new hash in one transaction, returning the affected user id. ResetPassword
	// must prefer this over the GetUserByPasswordResetToken + MarkPasswordResetUsed
	// + UpdatePassword sequence, which left a live reset token next to a freshly
	// written password whenever an error was dropped.
	ConsumePasswordResetTokenAndUpdatePassword(ctx context.Context, token, newPasswordHash string) (uuid.UUID, error)

	LinkTelegram(ctx context.Context, userID uuid.UUID, telegramID int64) error
	LinkDiscord(ctx context.Context, userID uuid.UUID, discordID string) error
	UnlinkTelegram(ctx context.Context, userID uuid.UUID) error
	UnlinkDiscord(ctx context.Context, userID uuid.UUID) error
}

// RefreshStore covers the Redis calls made by AuthHandler. It is deliberately
// separate from UserStore: refresh-token revocation is the one piece of auth
// state that is allowed to be ephemeral, and mixing the two would let a test
// fake accidentally make revocation durable.
//
// ConsumeRefreshToken must read and delete the token in a single step and
// return (userID, reused=true, nil) when the token is absent but was previously
// consumed: a replay means an attacker holds a copy, so the caller responds by
// revoking every session of that user.
//
// RevokeAllForUser is what makes a password change, a password reset and an
// account deletion actually end the old sessions instead of leaving them usable
// for the full 30-day refresh TTL.
type RefreshStore interface {
	StoreRefreshToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error
	GetUserIDByRefreshToken(ctx context.Context, token string) (uuid.UUID, error)
	RevokeRefreshToken(ctx context.Context, token string) error
	ConsumeRefreshToken(ctx context.Context, token string) (uuid.UUID, bool, error)
	RevokeAllForUser(ctx context.Context, userID uuid.UUID) (int, error)
}

// OAuthStateStore persists OAuth `state` records. Set has a short TTL;
// Consume must be atomic so a callback can be redeemed only once.
//
// It is a separate interface because the state record is not a session: it
// lives for minutes, is single-use, and is written on an unauthenticated
// request. Keeping it separate stops a session-oriented fake from being used as
// if it also stored login attempts.
type OAuthStateStore interface {
	SetOAuthState(ctx context.Context, state string, payload []byte, ttl time.Duration) error
	ConsumeOAuthState(ctx context.Context, state string) ([]byte, error)
}

// The concrete Redis client satisfies both; that is asserted in
// stores_conformance_test.go rather than here, so this package depends only on
// its own abstractions and never imports a concrete repository.

// EventPublisher is the single Redis call made by the watch-party hub.
type EventPublisher interface {
	PublishWatchPartyEvent(ctx context.Context, roomCode string, event *domain.WatchPartyEvent) error
}

// FavoritesStore is retired: SyncHandler now depends on the narrower
// FavoritesMutator, plus the optional capabilities PaginatedFavorites
// (SQL LIMIT/OFFSET + COUNT) and AtomicFavoritesStore (single-statement
// toggle). Keeping a second, unpaginated duplicate here only forced the
// postgres repository to satisfy a contract the handler no longer uses.

// HistoryStore covers every `historyRepo` call made by SyncHandler.
type HistoryStore interface {
	UpsertWatchHistory(ctx context.Context, h *domain.WatchHistory) error
	GetUserHistory(ctx context.Context, userID uuid.UUID, limit, offset int) ([]domain.WatchHistory, error)
	GetContinueWatching(ctx context.Context, userID uuid.UUID, limit int) ([]domain.WatchHistory, error)
}

// ContentCache covers the two `cacheRepo` calls made by ContentHandler.
type ContentCache interface {
	Get(ctx context.Context, key string, target interface{}) (bool, error)
	Set(ctx context.Context, key, providerID, contentType string, data interface{}, ttl time.Duration) error
}

// The concrete repositories satisfy every interface above. That is asserted in
// stores_conformance_test.go rather than here, so this package depends only on
// its own abstractions and never imports a concrete repository.
