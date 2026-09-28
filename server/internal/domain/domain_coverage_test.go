package domain_test

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
)

func TestCovDomainUserOmitsPasswordHash(t *testing.T) {
	u := domain.User{
		ID:           uuid.New(),
		Email:        "leak@oxide.film",
		PasswordHash: "$argon2id$v=19$super-secret-hash",
		Username:     "leaktest",
		Role:         "user",
	}
	raw, err := json.Marshal(u)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	s := string(raw)
	if strings.Contains(s, "password_hash") || strings.Contains(s, "PasswordHash") {
		t.Errorf("marshalled User must not expose password hash (protects /auth/me): %s", s)
	}
	if strings.Contains(s, "super-secret-hash") {
		t.Errorf("marshalled User leaks hash value: %s", s)
	}
}

func TestCovDomainMediaDetailsFlattened(t *testing.T) {
	d := domain.MediaDetails{
		MediaItem: domain.MediaItem{
			ID:         "m1",
			ProviderID: "p1",
			Title:      "Test Title",
			Type:       "movie",
			URL:        "http://example.com/m1",
		},
		Description: "desc",
	}
	raw, err := json.Marshal(d)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	// Поля вбудованого MediaItem — на верхньому рівні JSON (flattened, без вкладеності).
	for _, key := range []string{"id", "provider_id", "title", "type", "url", "description"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected top-level key %q in %s", key, raw)
		}
	}
	if _, ok := m["media_item"]; ok {
		t.Errorf("embedded MediaItem must not be nested under media_item: %s", raw)
	}
}

// TestCovDomainWatchHistoryNullables — ХАРАКТЕРИЗАЦІЯ: nullable-поля мають тег
// omitempty, тому nil *int/*float64/*string СЕРІАЛІЗУЮТЬСЯ ЯК ВІДСУТНІ КЛЮЧІ,
// а не як буквальний null (SQL NULL на вході, відсутність ключа на виході).
func TestCovDomainWatchHistoryNullables(t *testing.T) {
	w := domain.WatchHistory{
		ID:         uuid.New(),
		UserID:     uuid.New(),
		MediaID:    "m1",
		ProviderID: "p1",
		Title:      "T",
		PositionMs: 1,
		DurationMs: 2,
		WatchedAt:  time.Now(),
	}
	raw, err := json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, key := range []string{"year", "season", "episode", "rating", "rating_source"} {
		if _, ok := m[key]; ok {
			t.Errorf("expected key %q to be absent for nil pointer, got %s", key, raw)
		}
	}

	year, season, episode := 2024, 1, 2
	rating := 8.5
	source := "tmdb"
	w.Year, w.Season, w.Episode = &year, &season, &episode
	w.Rating, w.RatingSource = &rating, &source
	raw, err = json.Marshal(w)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	m = map[string]any{}
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	if m["year"] != float64(2024) || m["rating"] != 8.5 || m["rating_source"] != "tmdb" {
		t.Errorf("expected set nullables to round-trip, got %s", raw)
	}
}

// TestCovDomainWatchPartyStateCamelCase — ХАРАКТЕРИЗАЦІЯ: WatchPartyState
// використовує camelCase-ключі (hostId, mediaId, ...), що ВІДРІЗНЯЄТЬСЯ від
// snake_case решти контрактів. Flutter-клієнт залежить від цього формату.
func TestCovDomainWatchPartyStateCamelCase(t *testing.T) {
	s := domain.WatchPartyState{
		HostID:            "h1",
		MediaID:           "m1",
		StreamURL:         "http://example.com/s",
		CurrentPositionMs: 123,
		IsPlaying:         true,
		PlaybackSpeed:     1.5,
		UpdatedAtEpoch:    999,
	}
	raw, err := json.Marshal(s)
	if err != nil {
		t.Fatalf("marshal failed: %v", err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal failed: %v", err)
	}
	for _, key := range []string{"hostId", "mediaId", "streamUrl", "currentPositionMs", "isPlaying", "playbackSpeed", "updatedAtEpoch"} {
		if _, ok := m[key]; !ok {
			t.Errorf("expected camelCase key %q in %s", key, raw)
		}
	}
	for _, key := range []string{"host_id", "media_id", "stream_url", "current_position_ms", "is_playing", "playback_speed", "updated_at_epoch"} {
		if _, ok := m[key]; ok {
			t.Errorf("snake_case key %q must NOT appear in %s", key, raw)
		}
	}
}
