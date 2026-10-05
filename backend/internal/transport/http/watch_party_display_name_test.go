package http

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
)

// Invisible runes used by the sanitiser tests, built from their code points
// rather than written literally.
//
// This is deliberate. A literal U+FEFF in a Go source file is a compile error,
// and a literal RLO or ZWJ is invisible to review, to grep and to most editors --
// which is precisely the reason this test exists. Keeping them as named
// constants means the source stays pure ASCII and every invisible input is
// visible as an intent, at the cost of a reader having to look the code up.
var (
	runeRLO  = string(rune(0x202E)) // right-to-left override
	runeLRO  = string(rune(0x202D)) // left-to-right override
	runeZWJ  = string(rune(0x200D)) // zero-width joiner
	runeZWNJ = string(rune(0x200C)) // zero-width non-joiner
	runeBOM  = string(rune(0xFEFF)) // byte order mark / zero-width no-break space
	runeSHY  = string(rune(0x00AD)) // soft hyphen
	runeNEL  = string(rune(0x0085)) // next line, a C1 control
)

// recordingIssuer captures exactly what the handler asked to be signed.
type recordingIssuer struct {
	userID   string
	userName string
	room     string
	isHost   bool
	err      error
}

func (r *recordingIssuer) IssueWatchPartyTicket(userID, userName, roomCode string, _ time.Duration) (string, error) {
	r.userID, r.userName, r.room, r.isHost = userID, userName, roomCode, false
	return "guest-ticket", r.err
}

func (r *recordingIssuer) IssueHostWatchPartyTicket(userID, userName, roomCode string, _ time.Duration) (string, error) {
	r.userID, r.userName, r.room, r.isHost = userID, userName, roomCode, true
	return "host-ticket", r.err
}

// issue runs the handler against a recording issuer and returns what was signed.
func issue(t *testing.T, body map[string]any) (*recordingIssuer, *httptest.ResponseRecorder) {
	t.Helper()

	encoded, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal body: %v", err)
	}

	issuer := &recordingIssuer{}
	h := NewWatchPartyHandler(issuer)

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/api/v1/watch-party/tickets", strings.NewReader(string(encoded)))
	req = req.WithContext(ticketCtx(uuid.New()))

	h.Issue(rec, req)
	return issuer, rec
}

// The display name is peer-supplied text that gets broadcast to every guest in
// the room and rendered in a list. Two properties matter, and neither is
// observable in the response body, so both are asserted on what the handler
// actually asked to be signed:
//
//  1. It reaches the ticket at all. The handler used to discard it and stamp the
//     raw UUID, which made every server-hosted room a list of UUIDs.
//  2. It cannot carry invisible characters or an unbounded payload onto every
//     guest's socket.
func TestCovWatchPartyIssueSanitisesTheDisplayName(t *testing.T) {
	// Bodies are marshalled by encoding/json rather than hand-written, so every
	// case is well-formed JSON. A raw control byte inside a JSON string is
	// invalid JSON, and the handler answers 400 for that reason alone -- a
	// parser behaviour that would mask what these cases are actually about.
	cases := []struct {
		name     string
		send     string
		wantName string
	}{
		{name: "plainName", send: "Вася", wantName: "Вася"},
		{name: "trimmed", send: "  Вася  ", wantName: "Вася"},
		{name: "innerWhitespaceCollapsed", send: "Ва   ся", wantName: "Ва ся"},
		{name: "newlineBecomesSpace", send: "Ва\nся", wantName: "Ва ся"},
		{name: "tabBecomesSpace", send: "Ва\tся", wantName: "Ва ся"},
		{name: "nullDropped", send: "Ва\x00ся", wantName: "Вася"},
		{name: "bellDropped", send: "Ва\x07ся", wantName: "Вася"},
		{name: "deleteDropped", send: "Ва\x7fся", wantName: "Вася"},
		{name: "c1ControlDropped", send: "Ва" + runeNEL + "ся", wantName: "Вася"},

		// A participant list is the perfect place to impersonate someone: RLO
		// can make a name render as something else entirely, and ZWJ, ZWNJ and
		// a BOM can make two different byte strings render identically.
		{name: "bidiOverrideDropped", send: "Ва" + runeRLO + "ся", wantName: "Вася"},
		{name: "bidiEmbedDropped", send: "Ва" + runeLRO + "ся", wantName: "Вася"},
		{name: "zeroWidthJoinerDropped", send: "Ва" + runeZWJ + "ся", wantName: "Вася"},
		{name: "zeroWidthNonJoinerDropped", send: "Ва" + runeZWNJ + "ся", wantName: "Вася"},
		{name: "bomDropped", send: "Ва" + runeBOM + "ся", wantName: "Вася"},
		{name: "softHyphenDropped", send: "Ва" + runeSHY + "ся", wantName: "Вася"},

		{name: "clampedTo32Runes", send: strings.Repeat("я", 40),
			wantName: strings.Repeat("я", 32)},
		// Clamping happens on runes, not bytes, so a multi-byte name is not
		// cut in the middle of a character.
		{name: "clampedOnRuneBoundary", send: strings.Repeat("я", 40) + "🎬",
			wantName: strings.Repeat("я", 32)},
		{name: "htmlIsNotEscapedAway", send: "<b>Вася</b>", wantName: "<b>Вася</b>"},
		{name: "emojiSurvives", send: "Вася 🎬", wantName: "Вася 🎬"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			issuer, rec := issue(t, map[string]any{
				"roomCode": "ROOM1",
				"userName": tc.send,
			})

			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			if issuer.userName != tc.wantName {
				t.Errorf("signed user_name = %q, want %q", issuer.userName, tc.wantName)
			}
			if issuer.room != "ROOM1" {
				t.Errorf("signed room = %q, want ROOM1", issuer.room)
			}
			if issuer.isHost {
				t.Error("signed a host ticket without asking for one")
			}
		})
	}
}

// An empty or invisible-only name must not blank the identity out: the hub shows
// this string to the other guests, so falling back to the user id is the only
// sane result.
func TestCovWatchPartyIssueFallsBackToTheUserIDForAnEmptyName(t *testing.T) {
	for _, tc := range []struct {
		name string
		send any
	}{
		{name: "emptyString", send: ""},
		{name: "spaces", send: "   "},
		{name: "whitespaceOnly", send: "\t\n"},
		{name: "onlyBidiOverride", send: runeRLO},
		{name: "onlyZeroWidth", send: runeZWJ},
		{name: "jsonNull", send: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			userID := uuid.New()
			issuer := &recordingIssuer{}
			h := NewWatchPartyHandler(issuer)

			encoded, err := json.Marshal(map[string]any{
				"roomCode": "ROOM1",
				"userName": tc.send,
			})
			if err != nil {
				t.Fatalf("marshal body: %v", err)
			}

			rec := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost,
				"/api/v1/watch-party/tickets", strings.NewReader(string(encoded)))
			req = req.WithContext(ticketCtx(userID))

			h.Issue(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
			}
			if issuer.userName != userID.String() {
				t.Errorf("signed user_name = %q, want the user id %q",
					issuer.userName, userID.String())
			}
		})
	}
}

// isHost must actually reach the issuer: without a host ticket the host is
// elected by arrival order, so whoever joined first would take over the room.
func TestCovWatchPartyIssueRequestsAHostTicketWhenAsked(t *testing.T) {
	issuer, rec := issue(t, map[string]any{
		"roomCode": "ROOM1",
		"isHost":   true,
		"userName": "Вася",
	})

	if rec.Code != http.StatusOK {
		t.Fatalf("got %d, want 200 (body %s)", rec.Code, rec.Body.String())
	}
	if !issuer.isHost {
		t.Error("isHost=true did not produce a host ticket")
	}

	var out struct {
		Ticket string `json:"ticket"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &out); err != nil {
		t.Fatalf("response is not JSON: %v", err)
	}
	if out.Ticket != "host-ticket" {
		t.Errorf("ticket = %q, want host-ticket", out.Ticket)
	}
}

// A bad room code is still rejected even when a name is supplied, so the name
// cannot become a side channel that skips validation.
func TestCovWatchPartyIssueStillRejectsBadRoomsWithAName(t *testing.T) {
	issuer, rec := issue(t, map[string]any{
		"roomCode": "AB",
		"userName": "Вася",
	})

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("got %d, want 400 (body %s)", rec.Code, rec.Body.String())
	}
	if issuer.room != "" {
		t.Error("a ticket was signed for a room that failed validation")
	}
}
