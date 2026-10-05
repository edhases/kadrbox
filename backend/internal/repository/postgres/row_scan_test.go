package postgres

// Coverage for the row-mapping layer: the code that turns driver rows into
// domain values.
//
// pgx.Row and pgx.Rows are interfaces, so a fake implements them without a
// database. That is what makes scanUser (which maps NULL handling and three
// distinct error classes) and scanHistoryRows (which has to un-pointer seven
// nullable columns) reachable without a live Postgres.

import (
	"errors"
	"fmt"
	"math"
	"reflect"
	"testing"
	"time"

	"github.com/edhases/oxide-server/internal/domain"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
)

// ---- fake driver surface ----------------------------------------------------

// assignValue writes val into the scan destination the driver would have
// filled. It mirrors what pgx does: the destination is always a pointer, and a
// nil column becomes the zero value of the pointed-to type (which for **T is a
// nil pointer, and for *string is "").
func assignValue(dest any, val any) error {
	rv := reflect.ValueOf(dest)
	if !rv.IsValid() || rv.Kind() != reflect.Pointer || rv.IsNil() {
		return fmt.Errorf("scan destination is not a non-nil pointer: %T", dest)
	}
	elem := rv.Elem()
	if !elem.CanSet() {
		return fmt.Errorf("scan destination %T is not settable", dest)
	}
	if val == nil {
		elem.Set(reflect.Zero(elem.Type()))
		return nil
	}
	v := reflect.ValueOf(val)
	switch {
	case v.Type().AssignableTo(elem.Type()):
		elem.Set(v)
	case v.Type().ConvertibleTo(elem.Type()):
		elem.Set(v.Convert(elem.Type()))
	case elem.Kind() == reflect.Pointer && v.Type().AssignableTo(elem.Type().Elem()):
		// The **T destination case: pgx allocates the pointer and stores the
		// value. Reproduce that, or every nullable column in these two
		// repositories would be reported as an untestable fake limitation.
		allocated := reflect.New(elem.Type().Elem())
		allocated.Elem().Set(v)
		elem.Set(allocated)
	default:
		return fmt.Errorf("cannot assign %T to %s", val, elem.Type())
	}
	return nil
}

// fakeRow is one result row: a fixed list of column values.
type fakeRow struct {
	values []any
	err    error
	scanAt int
}

func (r *fakeRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	if len(dest) != len(r.values) {
		return fmt.Errorf("expected %d destinations, got %d", len(r.values), len(dest))
	}
	for i := range dest {
		if err := assignValue(dest[i], r.values[r.scanAt+i]); err != nil {
			return err
		}
	}
	r.scanAt += len(dest)
	return nil
}

var _ pgx.Row = (*fakeRow)(nil)

// fakeRows is a forward-only cursor over fakeRow, with a deferred error to
// model a connection dying mid-iteration.
type fakeRows struct {
	rows    []*fakeRow
	pos     int
	closed  bool
	err     error // returned by Err(), i.e. a mid-stream failure
	scanErr error
}

func (r *fakeRows) Next() bool {
	if r.pos >= len(r.rows) {
		return false
	}
	r.pos++
	return true
}

func (r *fakeRows) Scan(dest ...any) error {
	if r.scanErr != nil {
		return r.scanErr
	}
	if r.pos == 0 || r.pos > len(r.rows) {
		return errors.New("Scan called outside a Next cycle")
	}
	return r.rows[r.pos-1].Scan(dest...)
}

func (r *fakeRows) Err() error                    { return r.err }
func (r *fakeRows) Close()                        { r.closed = true }
func (r *fakeRows) CommandTag() pgconn.CommandTag { return pgconn.CommandTag{} }
func (r *fakeRows) TypeMap() *pgtype.Map          { return nil }
func (r *fakeRows) FieldDescriptions() []pgconn.FieldDescription {
	return nil
}
func (r *fakeRows) Values() ([]any, error) { return nil, errors.New("not implemented") }
func (r *fakeRows) RawValues() [][]byte    { return nil }
func (r *fakeRows) Conn() *pgx.Conn        { return nil }

var _ pgx.Rows = (*fakeRows)(nil)

// ---- scanUser ---------------------------------------------------------------

func TestCovScanUserMapsEveryColumn(t *testing.T) {
	id := uuid.New()
	repo := &UserRepository{}
	avatarURL, bio, discordID := "https://cdn/a.png", "bio", "discord-1"
	telegramID := int64(4242)
	created := time.Date(2024, 3, 1, 12, 0, 0, 0, time.UTC)
	updated := created.Add(time.Hour)

	got, err := repo.scanUser(&fakeRow{values: []any{
		id, "user@example.com", "hash", "user", &avatarURL, &bio,
		"admin", true, &telegramID, &discordID, created, updated,
	}})
	if err != nil {
		t.Fatalf("scanUser: %v", err)
	}

	want := &domain.User{
		ID:           id,
		Email:        "user@example.com",
		PasswordHash: "hash",
		Username:     "user",
		AvatarURL:    "https://cdn/a.png",
		Bio:          "bio",
		Role:         "admin",
		IsVerified:   true,
		TelegramID:   &telegramID,
		DiscordID:    &discordID,
		CreatedAt:    created,
		UpdatedAt:    updated,
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("scanUser()\n got %+v\nwant %+v", got, want)
	}
}

func TestCovScanUserNormalisesNullOptionalColumns(t *testing.T) {
	// avatar_url and bio are nullable, and a user who never set them must come
	// back as empty strings, not as a failed scan or a leftover pointer.
	repo := &UserRepository{}
	got, err := repo.scanUser(&fakeRow{values: []any{
		uuid.New(), "n@example.com", "hash", "n", nil, nil,
		"user", false, nil, nil, time.Now(), time.Now(),
	}})
	if err != nil {
		t.Fatalf("scanUser with NULL optionals: %v", err)
	}
	if got.AvatarURL != "" || got.Bio != "" {
		t.Errorf("NULL optionals became %q/%q, want empty strings", got.AvatarURL, got.Bio)
	}
	if got.TelegramID != nil || got.DiscordID != nil {
		t.Errorf("NULL telegram/discord ids became %v/%v, want nil", got.TelegramID, got.DiscordID)
	}
	if got.IsVerified {
		t.Error("IsVerified = true for a row that said false")
	}
}

func TestCovScanUserErrorClassification(t *testing.T) {
	cases := []struct {
		name    string
		err     error
		want    error
		wantSub string
	}{
		{
			name: "noRowsBecomesUserNotFound",
			err:  pgx.ErrNoRows,
			want: ErrUserNotFound,
		},
		{
			name: "wrappedNoRowsStillBecomesUserNotFound",
			err:  fmt.Errorf("query users: %w", pgx.ErrNoRows),
			want: ErrUserNotFound,
		},
		{
			name:    "emailUniqueViolationBecomesEmailTaken",
			err:     &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "users_email_key"},
			want:    ErrEmailTaken,
			wantSub: "",
		},
		{
			name:    "telegramUniqueViolationBecomesTelegramTaken",
			err:     &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "users_telegram_id_key"},
			want:    ErrTelegramTaken,
			wantSub: "",
		},
		{
			name: "unknownConstraintFallsBackToColumnMatching",
			err:  &pgconn.PgError{Code: pgUniqueViolation, ConstraintName: "users_pkey"},
			want: ErrUserAlreadyExists,
		},
		{
			name:    "unrelatedScanFailureIsWrapped",
			err:     errors.New("unexpected number of columns"),
			wantSub: "scan user: unexpected number of columns",
		},
	}

	repo := &UserRepository{}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := repo.scanUser(&fakeRow{err: tc.err})
			if err == nil {
				t.Fatalf("scanUser(%v) succeeded, want an error", tc.err)
			}
			if got != nil {
				t.Errorf("user = %+v, want nil alongside the error", got)
			}
			if tc.want != nil && !errors.Is(err, tc.want) {
				t.Errorf("error = %v, want it to wrap %v", err, tc.want)
			}
			if tc.wantSub != "" && err.Error() != tc.wantSub {
				t.Errorf("error = %q, want %q", err, tc.wantSub)
			}
		})
	}
}

// ---- scanHistoryRows -------------------------------------------------------

// historyRowValues builds the 18 columns scanHistoryRows expects, in the exact
// order its Scan call passes them.
func historyRowValues(h domain.WatchHistory) []any {
	return []any{
		h.ID, h.UserID, h.MediaID, h.ProviderID, h.Title,
		// The text columns are scanned into *string, so the driver hands over a
		// plain string (nil for NULL); the numeric ones are scanned into
		// **int/**float64, so those arrive already pointer-wrapped.
		strOrNil(h.PosterURL), h.Year, h.MediaType,
		h.Season, h.Episode, strOrNil(h.EpisodeTitle),
		h.PositionMs, h.DurationMs, strOrNil(h.LastStreamURL),
		strOrNil(h.Voiceover), h.Rating, strOrNil(derefStr(h.RatingSource)), h.WatchedAt,
	}
}

// strOrNil is the driver-level shape of a nullable TEXT column: a plain string
// when populated, nil when NULL.
func strOrNil(s string) any {
	if s == "" {
		return nil
	}
	return s
}

func derefStr(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func TestCovScanHistoryRowsMapsFullRows(t *testing.T) {
	userID := uuid.New()
	year := 2021
	season, episode := 2, 5
	rating := 8.5
	ratingSource := "imdb"
	watchedAt := time.Date(2025, 6, 1, 0, 0, 0, 0, time.UTC)

	entry := domain.WatchHistory{
		ID: uuid.New(), UserID: userID, MediaID: "m1", ProviderID: "uakino",
		Title: "Title", PosterURL: "https://cdn/p.jpg", Year: &year, MediaType: "series",
		Season: &season, Episode: &episode, EpisodeTitle: "Ep 5",
		PositionMs: 900, DurationMs: 1800, LastStreamURL: "https://cdn/s.m3u8",
		Voiceover: "uk", Rating: &rating, RatingSource: &ratingSource, WatchedAt: watchedAt,
	}

	rows := &fakeRows{rows: []*fakeRow{{values: historyRowValues(entry)}}}
	got, err := scanHistoryRows(rows)
	if err != nil {
		t.Fatalf("scanHistoryRows: %v", err)
	}
	if len(got) != 1 {
		t.Fatalf("got %d rows, want 1", len(got))
	}
	if !reflect.DeepEqual(got[0], entry) {
		t.Errorf("scanned row\n got %+v\nwant %+v", got[0], entry)
	}
}

func TestCovScanHistoryRowsKeepsNullColumnsNil(t *testing.T) {
	// A movie row has NULL season/episode; a partially-seen row has NULL
	// rating and no stream URL. Dereferencing those would panic mid-sync.
	entry := domain.WatchHistory{
		ID: uuid.New(), UserID: uuid.New(), MediaID: "m2", ProviderID: "tortuga",
		Title: "Movie", MediaType: "movie", PositionMs: 10, DurationMs: 100,
		WatchedAt: time.Now(),
	}
	rows := &fakeRows{rows: []*fakeRow{{values: historyRowValues(entry)}}}
	got, err := scanHistoryRows(rows)
	if err != nil {
		t.Fatalf("scanHistoryRows: %v", err)
	}
	if got[0].Season != nil || got[0].Episode != nil {
		t.Errorf("movie row got season=%v episode=%v, want both nil", got[0].Season, got[0].Episode)
	}
	if got[0].Rating != nil || got[0].RatingSource != nil {
		t.Errorf("row got rating=%v source=%v, want both nil", got[0].Rating, got[0].RatingSource)
	}
	if got[0].Year != nil {
		t.Errorf("row got year=%v, want nil", *got[0].Year)
	}
	if got[0].PosterURL != "" || got[0].EpisodeTitle != "" || got[0].LastStreamURL != "" || got[0].Voiceover != "" {
		t.Errorf("NULL strings did not collapse to empty: %+v", got[0])
	}
}

func TestCovScanHistoryRowsEmptyResultIsAnEmptySlice(t *testing.T) {
	got, err := scanHistoryRows(&fakeRows{})
	if err != nil {
		t.Fatalf("scanHistoryRows on no rows: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("got %d rows, want none", len(got))
	}
}

func TestCovScanHistoryRowsPropagatesScanFailure(t *testing.T) {
	rows := &fakeRows{rows: []*fakeRow{
		{values: historyRowValues(domain.WatchHistory{ID: uuid.New()})},
	}, scanErr: errors.New("converting driver.Value type uint64")}

	got, err := scanHistoryRows(rows)
	if err == nil {
		t.Fatal("a Scan failure was swallowed; a truncated history would look complete")
	}
	if got != nil {
		t.Errorf("rows = %+v, want nil alongside the error", got)
	}
	if !contains(err.Error(), "converting driver.Value") {
		t.Errorf("error = %q, want the driver's own message", err)
	}
}

func TestCovScanHistoryRowsChecksRowsErrAfterIteration(t *testing.T) {
	// The classic silent-truncation bug: Next() returns false because the
	// connection dropped, and without rows.Err() the caller sees a short but
	// "successful" history and caches it as complete.
	rows := &fakeRows{
		rows: []*fakeRow{{values: historyRowValues(domain.WatchHistory{ID: uuid.New()})}},
		err:  errors.New("conn closed"),
	}
	got, err := scanHistoryRows(rows)
	if err == nil {
		t.Fatal("rows.Err() was ignored after a partial iteration")
	}
	if got != nil {
		t.Errorf("rows = %+v, want nil so the caller cannot cache a truncated list", got)
	}
}

// ---- sanitisation ----------------------------------------------------------

func TestCovUpsertWatchHistoryRejectsNilBeforeTouchingThePool(t *testing.T) {
	// A nil entry with a nil pool: the guard must return before the Exec that
	// would otherwise nil-panic.
	repo := &HistoryRepository{}
	err := repo.UpsertWatchHistory(t.Context(), nil)
	if err == nil {
		t.Fatal("UpsertWatchHistory(nil) succeeded, want an error")
	}
	if !contains(err.Error(), "nil entry") {
		t.Errorf("error = %q, want it to name the nil entry", err)
	}
}

func TestCovSanitiseHistoryClampsEveryField(t *testing.T) {
	negativeYear, bigYear := -5, 40000
	zeroSeason, negativeEpisode := 0, -1
	high, low, nan, inf := 42.0, -1.0, math.NaN(), math.Inf(1)

	h := &domain.WatchHistory{
		PositionMs: -1,
		DurationMs: -5,
		Year:       &negativeYear,
		Season:     &zeroSeason,
		Episode:    &negativeEpisode,
		Rating:     &high,
		MediaType:  "  SERIES  ",
	}
	sanitiseHistory(h)

	if h.PositionMs != 0 {
		t.Errorf("PositionMs = %d, want 0 for a negative position", h.PositionMs)
	}
	if h.DurationMs != 0 {
		t.Errorf("DurationMs = %d, want 0 for a negative duration", h.DurationMs)
	}
	// Zero is this codebase's "unknown" sentinel for both year and episode
	// numbers, so those columns are dropped (nil) rather than clamped to 0.
	if h.Year == nil || *h.Year != 0 {
		t.Errorf("Year = %v, want a pointer to 0", h.Year)
	}
	if h.Season != nil || h.Episode != nil {
		t.Errorf("Season/Episode = %v/%v, want nil for non-positive numbers", h.Season, h.Episode)
	}
	if h.Rating != nil {
		t.Errorf("Rating = %v, want nil for an out-of-range score", *h.Rating)
	}
	if h.MediaType != "series" {
		t.Errorf("MediaType = %q, want %q", h.MediaType, "series")
	}

	// A position beyond the duration is clamped down, not left inconsistent.
	h2 := &domain.WatchHistory{PositionMs: 900, DurationMs: 300}
	sanitiseHistory(h2)
	if h2.PositionMs != 300 {
		t.Errorf("PositionMs = %d, want it clamped to the duration 300", h2.PositionMs)
	}

	// NaN and -Inf are rejected by PostgreSQL's float handling, and a rating
	// inside 0..10 survives untouched.
	for _, bad := range []*float64{nil, &nan, &inf, &low} {
		h3 := &domain.WatchHistory{Rating: bad}
		sanitiseHistory(h3)
		if bad != nil && h3.Rating != nil {
			t.Errorf("Rating %v survived sanitisation", *bad)
		}
	}
	good := 7.5
	h4 := &domain.WatchHistory{Rating: &good}
	sanitiseHistory(h4)
	if h4.Rating == nil || *h4.Rating != good {
		t.Errorf("Rating = %v, want the in-range value %v preserved", h4.Rating, good)
	}

	// An absurd year clamps rather than being erased.
	h5 := &domain.WatchHistory{Year: &bigYear}
	sanitiseHistory(h5)
	if h5.Year == nil || *h5.Year != 9999 {
		t.Errorf("Year = %v, want it clamped to 9999", h5.Year)
	}

	// nil is tolerated so a caller can sanitise an absent entry.
	sanitiseHistory(nil)
}

func TestCovResolveWatchedAtRefusesToMoveTimeForward(t *testing.T) {
	now := time.Date(2025, 1, 1, 12, 0, 0, 0, time.UTC)

	if got := resolveWatchedAt(time.Time{}, now); !got.Equal(now) {
		t.Errorf("resolveWatchedAt(zero) = %s, want the server clock %s", got, now)
	}
	past := now.Add(-time.Hour)
	if got := resolveWatchedAt(past, now); !got.Equal(past) {
		t.Errorf("resolveWatchedAt(past) = %s, want the client timestamp %s preserved", got, past)
	}
	// A device with a wrong clock must not be able to freeze the row.
	future := now.Add(2 * time.Hour)
	if got := resolveWatchedAt(future, now); !got.Equal(now) {
		t.Errorf("resolveWatchedAt(future) = %s, want it clamped to the server clock %s", got, now)
	}
}

func TestCovNormalisePageClampsPagination(t *testing.T) {
	cases := []struct {
		name                    string
		limit, offset, fallback int
		wantLimit, wantOffset   int
	}{
		{name: "defaultsWhenLimitIsZero", limit: 0, offset: 0, fallback: 50, wantLimit: 50, wantOffset: 0},
		{name: "defaultsWhenLimitIsNegative", limit: -3, offset: 0, fallback: 50, wantLimit: 50, wantOffset: 0},
		{name: "clampsNegativeOffset", limit: 20, offset: -5, fallback: 50, wantLimit: 20, wantOffset: 0},
		{name: "keepsValidValues", limit: 20, offset: 40, fallback: 50, wantLimit: 20, wantOffset: 40},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			limit, offset := normalisePage(tc.limit, tc.offset, tc.fallback)
			if limit != tc.wantLimit || offset != tc.wantOffset {
				t.Errorf("normalisePage(%d, %d, %d) = (%d, %d), want (%d, %d)",
					tc.limit, tc.offset, tc.fallback, limit, offset, tc.wantLimit, tc.wantOffset)
			}
		})
	}
}
