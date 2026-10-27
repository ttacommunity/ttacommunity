package httpapi

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

const asker = "100000000000000001"

// discordKey is an invented application key, from a fixed seed.
var discordKey = ed25519.NewKeyFromSeed(bytes.Repeat([]byte{7}, ed25519.SeedSize))

// fakeDiscord is the Discord application with discordKey, answering what Discord accounts are from members.
type fakeDiscord struct {
	members map[string]store.DiscordMember
	err     error
	asked   []string
}

func (f *fakeDiscord) PublicKey() ed25519.PublicKey { return discordKey.Public().(ed25519.PublicKey) }

func (f *fakeDiscord) Site() string { return "https://site.example" }

func (f *fakeDiscord) DiscordMember(_ context.Context, discordID string) (store.DiscordMember, error) {
	f.asked = append(f.asked, discordID)
	if f.err != nil {
		return store.DiscordMember{}, f.err
	}
	if m, ok := f.members[discordID]; ok {
		return m, nil
	}
	return store.DiscordMember{State: store.DiscordNoLogin}, nil
}

// discordRatings is fakeRatings with players by name and a name search's answer, recording each search's term.
type discordRatings struct {
	fakeRatings
	players  map[string]store.Player
	names    []string
	err      error
	searched []string
}

func (r *discordRatings) Player(_ context.Context, _ int64, name string) (store.Player, error) {
	r.calls++
	if r.err != nil {
		return store.Player{}, r.err
	}
	if p, ok := r.players[name]; ok {
		return p, nil
	}
	return store.Player{}, store.ErrPlayerNotFound
}

func (r *discordRatings) PlayerNamesLike(_ context.Context, _ int64, term string, limit int) ([]string, error) {
	r.calls++
	r.searched = append(r.searched, term)
	return r.names[:min(limit, len(r.names))], r.err
}

func rank(n int) *int { return &n }

// quill is an invented rated player with Glicko-D ratings for every format together and 2-player.
func quill() store.Player {
	last := time.Date(2026, 9, 30, 0, 0, 0, 0, time.UTC)
	return store.Player{
		Name: "Quill", Title: &store.Title{Code: "SGM"}, LastPlayed: &last, Games: map[int]int{0: 481, 2: 170},
		Ratings: []store.PlayerRating{
			{Model: "glicko2_std", Format: 0, Rank: rank(9), Rating: 1500, RD: 30, CRating: 1410, Opponents: 890},
			{Model: "glicko2_daneo", Format: 0, Rank: rank(2), Rating: 1972, RD: 20, CRating: 1912, Opponents: 890, WinRate: 71.7},
			{Model: "glicko2_daneo", Format: 2, Rating: 1900, RD: 25, CRating: 1825, Opponents: 170, WinRate: 66},
		},
	}
}

func newDiscordRatings() *discordRatings {
	names := make([]string, 30)
	for i := range names {
		names[i] = fmt.Sprintf("Player%d", i)
	}
	r := &discordRatings{players: map[string]store.Player{"Quill": quill()}, names: names}
	r.run = store.PublishedRun{ID: 2}
	return r
}

// discordDo posts interaction to the endpoint, signed with key (none when nil), and returns the answer.
func discordDo(ratings Ratings, discord DiscordApp, key ed25519.PrivateKey, interaction string, log *slog.Logger) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, "/v1/discord/interactions", strings.NewReader(interaction))
	req.Header.Set("Content-Type", "application/json")
	if key != nil {
		const ts = "1760000000"
		req.Header.Set("X-Signature-Timestamp", ts)
		req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(ed25519.Sign(key, []byte(ts+interaction))))
	}
	rec := httptest.NewRecorder()
	NewMux(ratings, nil, nil, discord, nil, log, &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

// discordReply decodes a 200 answer, failing the test on any other.
func discordReply(t *testing.T, rec *httptest.ResponseRecorder) DiscordReply {
	t.Helper()
	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200; body %s", rec.Code, rec.Body)
	}
	var r DiscordReply
	if err := json.Unmarshal(rec.Body.Bytes(), &r); err != nil {
		t.Fatalf("decoding %s: %v", rec.Body, err)
	}
	return r
}

// command is the command name used with options by the asker in a server.
func command(name, options string) string {
	return `{"type":2,"data":{"name":"` + name + `","options":[` + options + `]},"member":{"user":{"id":"` + asker + `"}}}`
}

func rating(options string) string { return command("rating", options) }

func TestTheDiscordEndpointTakesOnlyDiscordsRequests(t *testing.T) {
	other := ed25519.NewKeyFromSeed(bytes.Repeat([]byte{8}, ed25519.SeedSize))
	tests := []struct {
		name     string
		noApp    bool
		key      ed25519.PrivateKey
		altered  bool
		wantCode int
	}{
		{"no Discord application set up here", true, discordKey, false, http.StatusNotFound},
		{"no signature", false, nil, false, http.StatusUnauthorized},
		{"signed by another key", false, other, false, http.StatusUnauthorized},
		{"a body changed after signing", false, discordKey, true, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings, discord := newDiscordRatings(), &fakeDiscord{}
			var app DiscordApp = discord
			if tt.noApp {
				app = nil
			}
			body := rating("")
			req := httptest.NewRequest(http.MethodPost, "/v1/discord/interactions", strings.NewReader(body))
			if tt.key != nil {
				signed := body
				if tt.altered {
					signed = rating(`{"name":"player","type":3,"value":"Quill"}`)
				}
				req.Header.Set("X-Signature-Timestamp", "1760000000")
				req.Header.Set("X-Signature-Ed25519", hex.EncodeToString(ed25519.Sign(tt.key, []byte("1760000000"+signed))))
			}
			rec := httptest.NewRecorder()
			NewMux(ratings, nil, nil, app, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
			if rec.Code != tt.wantCode {
				t.Errorf("status = %d, want %d", rec.Code, tt.wantCode)
			}
			if ratings.calls != 0 || len(discord.asked) != 0 {
				t.Errorf("a refused request reached the store (%d ratings reads, %d accounts asked), want none", ratings.calls, len(discord.asked))
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNone {
				t.Errorf("Cache-Control = %q, want %q", got, cacheNone)
			}
		})
	}
}

func TestTheDiscordEndpointAnswersPing(t *testing.T) {
	r := discordReply(t, discordDo(newDiscordRatings(), &fakeDiscord{}, discordKey, `{"type":1}`, slog.New(slog.DiscardHandler)))
	if r.Type != 1 || r.Data != nil {
		t.Errorf("reply = %+v, want a bare PONG (type 1)", r)
	}
}

func TestTheDiscordEndpointRefusesWhatItDoesNotKnow(t *testing.T) {
	for name, body := range map[string]string{
		"another command":     `{"type":2,"data":{"name":"other"}}`,
		"a command with none": `{"type":2}`,
		"a button":            `{"type":3}`,
	} {
		t.Run(name, func(t *testing.T) {
			if rec := discordDo(newDiscordRatings(), &fakeDiscord{}, discordKey, body, slog.New(slog.DiscardHandler)); rec.Code != http.StatusBadRequest {
				t.Errorf("status = %d, want 400", rec.Code)
			}
		})
	}
}

func TestRatingSuggestsPlayers(t *testing.T) {
	suggest := func(typed string) string {
		return `{"type":4,"data":{"name":"rating","options":[{"name":"player","type":3,"value":"` + typed + `","focused":true}]}}`
	}
	tests := []struct {
		name         string
		typed        string
		searchErr    error
		wantChoices  int
		wantSearched []string
	}{
		{"the five strongest matches", " groz", nil, 5, []string{"groz"}},
		{"none before anything is typed", " ", nil, 0, nil},
		{"none when the search fails", "groz", errors.New("database is down"), 0, []string{"groz"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.searchErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, suggest(tt.typed), slog.New(slog.DiscardHandler)))
			if r.Type != 8 || r.Data == nil || r.Data.Choices == nil || len(*r.Data.Choices) != tt.wantChoices {
				t.Fatalf("reply = %+v, want %d choices", r, tt.wantChoices)
			}
			if !slices.Equal(ratings.searched, tt.wantSearched) {
				t.Errorf("searched for %q, want %q", ratings.searched, tt.wantSearched)
			}
		})
	}
}

func TestRatingWithAName(t *testing.T) {
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    string
	}{
		{"a rated player, to the asker alone", `{"name":"player","type":3,"value":"Quill"}`, nil, true, "Quill"},
		{"a rated player, shared with everyone", `{"name":"player","type":3,"value":"Quill"},{"name":"share","type":5,"value":true}`, nil, false, "Quill"},
		{"not shared when share is false", `{"name":"player","type":3,"value":"Quill"},{"name":"share","type":5,"value":false}`, nil, true, "Quill"},
		{"a format by its number", `{"name":"player","type":3,"value":"Quill"},{"name":"format","type":4,"value":2}`, nil, true, "2-player"},
		{"a name no player has, private though shared", `{"name":"player","type":3,"value":"Nobody"},{"name":"share","type":5,"value":true}`, nil, true, "Nobody"},
		{"a name no player has", `{"name":"player","type":3,"value":"Nobody"}`, nil, true, "Nobody"},
		{"a format the player has no rating in", `{"name":"player","type":3,"value":"Quill"},{"name":"format","type":4,"value":4}`, nil, true, "Quill"},
		{"the ratings not answering", `{"name":"player","type":3,"value":"Quill"}`, errors.New("database is down"), true, ""},
		{"no name at all", ``, nil, true, "/rating"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, rating(tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
		})
	}
}

func TestMyRatingShowsTheAskersOwn(t *testing.T) {
	tests := []struct {
		state       string
		wantPrivate bool
		wantPlayer  bool
	}{
		{store.DiscordFound, true, true},
		{store.DiscordNoPlayer, true, false},
		{store.DiscordNotLinked, true, false},
		{store.DiscordNoLogin, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			ratings := newDiscordRatings()
			discord := &fakeDiscord{members: map[string]store.DiscordMember{asker: {State: tt.state, Name: "Quill"}}}
			r := discordReply(t, discordDo(ratings, discord, discordKey, command("myrating", ""), slog.New(slog.DiscardHandler)))
			want := ""
			if tt.wantPlayer {
				want = "Quill"
			}
			wantMessage(t, r, tt.wantPrivate, want)
			if !slices.Equal(discord.asked, []string{asker}) {
				t.Errorf("asked about %q, want the asker %s alone", discord.asked, asker)
			}
		})
	}
}

func TestMyRatingFindsTheAskerInADirectMessage(t *testing.T) {
	discord := &fakeDiscord{members: map[string]store.DiscordMember{asker: {State: store.DiscordFound, Name: "Quill"}}}
	body := `{"type":2,"data":{"name":"myrating","options":[{"name":"format","type":4,"value":2},{"name":"share","type":5,"value":true}]},"user":{"id":"` + asker + `"}}`
	wantMessage(t, discordReply(t, discordDo(newDiscordRatings(), discord, discordKey, body, slog.New(slog.DiscardHandler))), false, "2-player")
}

func TestTheDiscordEndpointLogsNoDiscordIDOrName(t *testing.T) {
	for name, body := range map[string]string{"own": command("myrating", ""), "named": rating(`{"name":"player","type":3,"value":"Quill"}`)} {
		t.Run(name, func(t *testing.T) {
			var logs bytes.Buffer
			ratings := newDiscordRatings()
			ratings.err = errors.New("database is down")
			discord := &fakeDiscord{err: errors.New("database is down")}
			discordDo(ratings, discord, discordKey, body, slog.New(slog.NewJSONHandler(&logs, nil)))
			if !strings.Contains(logs.String(), "database is down") {
				t.Errorf("logs = %s, want the failure logged", logs.String())
			}
			for _, personal := range []string{asker, "Quill"} {
				if strings.Contains(logs.String(), personal) {
					t.Errorf("logs = %s, want no %q in them", logs.String(), personal)
				}
			}
		})
	}
}

func TestSummariseShowsGlickoDInTheFormatAsked(t *testing.T) {
	tests := []struct {
		name        string
		format      int
		wantOK      bool
		wantCRating float64
		wantGames   int
		wantRanked  bool
	}{
		{"every format together", 0, true, 1912, 481, true},
		{"2-player, not ranked", 2, true, 1825, 170, false},
		{"a format with no rating", 4, false, 0, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, ok := summarise(quill(), tt.format)
			if ok != tt.wantOK || s.CRating != tt.wantCRating || s.Games != tt.wantGames || (s.Rank != nil) != tt.wantRanked {
				t.Errorf("summarise(format %d) = %+v, %v; want ok %v, C-Rating %v, %d games, ranked %v",
					tt.format, s, ok, tt.wantOK, tt.wantCRating, tt.wantGames, tt.wantRanked)
			}
		})
	}
}

// wantMessage checks r is a message mentioning no one, private or not, and containing text.
func wantMessage(t *testing.T, r DiscordReply, private bool, text string) {
	t.Helper()
	if r.Type != 4 || r.Data == nil || r.Data.Content == nil {
		t.Fatalf("reply = %+v, want a message (type 4)", r)
	}
	if r.Data.AllowedMentions == nil || len(r.Data.AllowedMentions.Parse) != 0 {
		t.Errorf("allowed_mentions = %+v, want none allowed", r.Data.AllowedMentions)
	}
	if got := r.Data.Flags != nil && *r.Data.Flags == 64; got != private {
		t.Errorf("private = %v, want %v; content %q", got, private, *r.Data.Content)
	}
	if !strings.Contains(*r.Data.Content, text) {
		t.Errorf("content = %q, want it to contain %q", *r.Data.Content, text)
	}
}
