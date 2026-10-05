package http_test

// In-memory implementations of the handler-facing store interfaces.
//
// These are not stubs: they hold real state in maps, enforce the same
// uniqueness rules the SQL layer does (email collision, token single use), and
// return the same error shapes. That lets the handler tests assert on genuine
// multi-step behaviour — register, verify, log in, refresh, revoke — instead
// of asserting that a hand-written return value was passed through.

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/edhases/oxide-server/internal/repository/postgres"
	transporthttp "github.com/edhases/oxide-server/internal/transport/http"
)

// ErrNotFound mirrors the repository's sentinel for a missing row.
var ErrNotFound = errors.New("not found")

// ErrConflict mirrors the repository's sentinel for a uniqueness violation.
var ErrConflict = errors.New("already exists")

type memUserStore struct {
	mu sync.Mutex

	byID    map[uuid.UUID]*domain.User
	byEmail map[string]uuid.UUID

	telegramIDs map[int64]uuid.UUID
	discordIDs  map[string]uuid.UUID

	verificationTokens map[string]uuid.UUID
	resetTokens        map[string]uuid.UUID

	// Counters let tests assert that a flow called the store the right number
	// of times, e.g. that a used reset token is not silently reusable.
	updatesProfile  int
	updatesPassword int
	deletes         int
	markedVerified  int
	linkedTelegram  int
	linkedDiscord   int
	unlinkedTm      int
	unlinkedDisc    int
}

func newMemUserStore() *memUserStore {
	return &memUserStore{
		byID:               map[uuid.UUID]*domain.User{},
		byEmail:            map[string]uuid.UUID{},
		telegramIDs:        map[int64]uuid.UUID{},
		discordIDs:         map[string]uuid.UUID{},
		verificationTokens: map[string]uuid.UUID{},
		resetTokens:        map[string]uuid.UUID{},
	}
}

func (s *memUserStore) CreateUser(ctx context.Context, email, passwordHash, username string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	key := strings.ToLower(email)
	if _, exists := s.byEmail[key]; exists {
		return nil, ErrConflict
	}
	u := &domain.User{
		ID:           uuid.New(),
		Email:        email,
		PasswordHash: passwordHash,
		Username:     username,
		Role:         "user",
		CreatedAt:    time.Now(),
		UpdatedAt:    time.Now(),
	}
	s.byID[u.ID] = u
	s.byEmail[key] = u.ID
	clone := *u
	return &clone, nil
}

func (s *memUserStore) CreateOAuthUser(ctx context.Context, email, passwordHash, username, avatarURL string, telegramID *int64, discordID *string) (*domain.User, error) {
	u, err := s.CreateUser(ctx, email, passwordHash, username)
	if err != nil {
		return nil, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	stored := s.byID[u.ID]
	stored.AvatarURL = avatarURL
	// An OAuth identity is verified by the provider, not by email.
	stored.IsVerified = true
	if telegramID != nil {
		stored.TelegramID = telegramID
		s.telegramIDs[*telegramID] = stored.ID
	}
	if discordID != nil {
		stored.DiscordID = discordID
		s.discordIDs[*discordID] = stored.ID
	}
	clone := *stored
	return &clone, nil
}

func (s *memUserStore) GetUserByEmail(ctx context.Context, email string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.byEmail[strings.ToLower(email)]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *s.byID[id]
	return &clone, nil
}

func (s *memUserStore) GetUserByID(ctx context.Context, id uuid.UUID) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *u
	return &clone, nil
}

func (s *memUserStore) GetUserByTelegramID(ctx context.Context, telegramID int64) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.telegramIDs[telegramID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *s.byID[id]
	return &clone, nil
}

func (s *memUserStore) GetUserByDiscordID(ctx context.Context, discordID string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.discordIDs[discordID]
	if !ok {
		return nil, ErrNotFound
	}
	clone := *s.byID[id]
	return &clone, nil
}

func (s *memUserStore) UpdateProfile(ctx context.Context, id uuid.UUID, username, bio, avatarURL string) (*domain.User, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[id]
	if !ok {
		return nil, ErrNotFound
	}
	s.updatesProfile++
	if username != "" {
		u.Username = username
	}
	u.Bio = bio
	if avatarURL != "" {
		u.AvatarURL = avatarURL
	}
	u.UpdatedAt = time.Now()
	clone := *u
	return &clone, nil
}

func (s *memUserStore) UpdatePassword(ctx context.Context, id uuid.UUID, passwordHash string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	s.updatesPassword++
	u.PasswordHash = passwordHash
	u.UpdatedAt = time.Now()
	return nil
}

func (s *memUserStore) DeleteUser(ctx context.Context, id uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[id]
	if !ok {
		return ErrNotFound
	}
	s.deletes++
	delete(s.byID, id)
	delete(s.byEmail, strings.ToLower(u.Email))
	if u.TelegramID != nil {
		delete(s.telegramIDs, *u.TelegramID)
	}
	if u.DiscordID != nil {
		delete(s.discordIDs, *u.DiscordID)
	}
	return nil
}

func (s *memUserStore) MarkEmailVerified(ctx context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[userID]
	if !ok {
		return ErrNotFound
	}
	s.markedVerified++
	u.IsVerified = true
	return nil
}

func (s *memUserStore) CreateVerificationToken(ctx context.Context, userID uuid.UUID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[userID]; !ok {
		return ErrNotFound
	}
	s.verificationTokens[token] = userID
	return nil
}

func (s *memUserStore) GetUserByVerificationToken(ctx context.Context, token string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.verificationTokens[token]
	if !ok {
		return uuid.Nil, ErrNotFound
	}
	return id, nil
}

func (s *memUserStore) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[userID]; !ok {
		return ErrNotFound
	}
	s.resetTokens[token] = userID
	return nil
}

// ConsumePasswordResetTokenAndUpdatePassword mirrors the repository's single
// transaction: the token is burned and the hash written together, so a replay
// fails and a failure cannot leave a new password beside a live token.
func (s *memUserStore) ConsumePasswordResetTokenAndUpdatePassword(ctx context.Context, token, newPasswordHash string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	userID, ok := s.resetTokens[token]
	if !ok {
		return uuid.Nil, postgres.ErrResetTokenNotFound
	}
	delete(s.resetTokens, token)
	user, ok := s.byID[userID]
	if !ok {
		return uuid.Nil, postgres.ErrUserNotFound
	}
	user.PasswordHash = newPasswordHash
	return userID, nil
}

// GetUserByPasswordResetToken consumes the token, matching the SQL layer's
// single-use guarantee: a replayed reset link must not work twice.
func (s *memUserStore) GetUserByPasswordResetToken(ctx context.Context, token string) (uuid.UUID, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	id, ok := s.resetTokens[token]
	if !ok {
		return uuid.Nil, ErrNotFound
	}
	delete(s.resetTokens, token)
	return id, nil
}

func (s *memUserStore) MarkPasswordResetUsed(ctx context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, ok := s.byID[userID]; !ok {
		return ErrNotFound
	}
	for token, id := range s.resetTokens {
		if id == userID {
			delete(s.resetTokens, token)
		}
	}
	return nil
}

func (s *memUserStore) LinkTelegram(ctx context.Context, userID uuid.UUID, telegramID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[userID]
	if !ok {
		return ErrNotFound
	}
	s.linkedTelegram++
	u.TelegramID = &telegramID
	s.telegramIDs[telegramID] = userID
	return nil
}

func (s *memUserStore) LinkDiscord(ctx context.Context, userID uuid.UUID, discordID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[userID]
	if !ok {
		return ErrNotFound
	}
	s.linkedDiscord++
	u.DiscordID = &discordID
	s.discordIDs[discordID] = userID
	return nil
}

func (s *memUserStore) UnlinkTelegram(ctx context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[userID]
	if !ok {
		return ErrNotFound
	}
	s.unlinkedTm++
	if u.TelegramID != nil {
		delete(s.telegramIDs, *u.TelegramID)
		u.TelegramID = nil
	}
	return nil
}

func (s *memUserStore) UnlinkDiscord(ctx context.Context, userID uuid.UUID) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	u, ok := s.byID[userID]
	if !ok {
		return ErrNotFound
	}
	s.unlinkedDisc++
	if u.DiscordID != nil {
		delete(s.discordIDs, *u.DiscordID)
		u.DiscordID = nil
	}
	return nil
}

// memRefreshStore holds refresh tokens in a map with expiry, mirroring Redis
// semantics closely enough that TTL and revocation can be asserted.
type memRefreshStore struct {
	mu sync.Mutex

	tokens map[string]memRefreshEntry

	storeErr error
	getErr   error

	stores  int
	gets    int
	revokes int
	lastTTL time.Duration
}

type memRefreshEntry struct {
	userID    uuid.UUID
	expiresAt time.Time
}

func newMemRefreshStore() *memRefreshStore {
	return &memRefreshStore{tokens: map[string]memRefreshEntry{}}
}

func (s *memRefreshStore) StoreRefreshToken(ctx context.Context, token string, userID uuid.UUID, ttl time.Duration) error {
	if s.storeErr != nil {
		return s.storeErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stores++
	s.lastTTL = ttl
	s.tokens[token] = memRefreshEntry{userID: userID, expiresAt: time.Now().Add(ttl)}
	return nil
}

func (s *memRefreshStore) GetUserIDByRefreshToken(ctx context.Context, token string) (uuid.UUID, error) {
	if s.getErr != nil {
		return uuid.Nil, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	entry, ok := s.tokens[token]
	if !ok {
		return uuid.Nil, ErrNotFound
	}
	if time.Now().After(entry.expiresAt) {
		// An expired token behaves as absent, exactly as Redis would.
		delete(s.tokens, token)
		return uuid.Nil, ErrNotFound
	}
	return entry.userID, nil
}

func (s *memRefreshStore) RevokeRefreshToken(ctx context.Context, token string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.revokes++
	delete(s.tokens, token)
	return nil
}

func (s *memRefreshStore) liveTokens() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.tokens)
}

// memCacheStore records cache traffic so the content handler's hit and miss
// paths can be asserted.
type memCacheStore struct {
	mu sync.Mutex

	values map[string][]byte
	gets   int
	sets   int

	lastKey      string
	lastProvider string
	lastType     string
	lastTTL      time.Duration
	setErr       error
	getErr       error
}

func newMemCacheStore() *memCacheStore {
	return &memCacheStore{values: map[string][]byte{}}
}

func (s *memCacheStore) Get(ctx context.Context, key string, target interface{}) (bool, error) {
	if s.getErr != nil {
		return false, s.getErr
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.gets++
	raw, ok := s.values[key]
	if !ok {
		return false, nil
	}
	// Decode into the caller's target using the same encoding the Set path
	// uses, so a round trip is genuinely lossless.
	return unmarshalInto(raw, target), nil
}

func (s *memCacheStore) Set(ctx context.Context, key, providerID, contentType string, data interface{}, ttl time.Duration) error {
	if s.setErr != nil {
		return s.setErr
	}
	raw, err := marshalFrom(data)
	if err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.sets++
	s.lastKey = key
	s.lastProvider = providerID
	s.lastType = contentType
	s.lastTTL = ttl
	s.values[key] = raw
	return nil
}

func marshalFrom(data interface{}) ([]byte, error) {
	return json.Marshal(data)
}

func unmarshalInto(raw []byte, target interface{}) bool {
	return json.Unmarshal(raw, target) == nil
}

// Conformance: the in-memory stores must satisfy the interfaces the handlers
// actually declare, or the whole approach silently stops compiling.
var (
	_ transporthttp.UserStore    = (*memUserStore)(nil)
	_ transporthttp.RefreshStore = (*memRefreshStore)(nil)
	_ transporthttp.ContentCache = (*memCacheStore)(nil)
)
