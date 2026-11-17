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
	// commands are the settings saved for each command; settingsErr, when set, is what reading them answers.
	commands    map[string]store.CommandSettings
	settingsErr error
}

func (f *fakeDiscord) CommandSettings(_ context.Context, name string) (store.CommandSettings, bool, error) {
	c, ok := f.commands[name]
	return c, ok, f.settingsErr
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
	players     map[string]store.Player
	names       []string
	err         error
	searched    []string
	versusAsked []string
	rivalsAsked []string
	seatsAsked  []string
	upsetsAsked []int
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

// Versus answers Quill's games with Rook, by either's name, and no other pair's.
func (r *discordRatings) Versus(_ context.Context, _ int64, name, other string, rated, unrated bool) (string, string, []store.PlayerMatch, error) {
	r.calls++
	r.versusAsked = append(r.versusAsked, fmt.Sprintf("%s/%s rated=%v unrated=%v", name, other, rated, unrated))
	if r.err != nil {
		return "", "", nil, r.err
	}
	if _, ok := r.players[name]; !ok || (other != "Rook" && other != "Quill") {
		return "", "", nil, store.ErrPlayerNotFound
	}
	day := time.Date(2026, 10, 9, 0, 0, 0, 0, time.UTC)
	return name, other, []store.PlayerMatch{
		{Date: day, Tournament: "Autumn Cup", PlayerCount: 2, Seats: []store.Seat{{Name: other, Score: 80}, {Name: name, Score: 95}}},
		{Date: day.AddDate(0, 0, -7), Tournament: "Autumn Cup", PlayerCount: 3,
			Seats: []store.Seat{{Name: name, Score: 50}, {Name: "Third", Score: 70}, {Name: other, Score: 90}}},
	}, nil
}

// PlayerRivals answers two rivals for Quill in any format, recording what was asked.
func (r *discordRatings) PlayerRivals(_ context.Context, _ int64, name string, format int, rated, unrated bool) (string, []store.Rival, error) {
	r.calls++
	r.rivalsAsked = append(r.rivalsAsked, fmt.Sprintf("%s format=%d rated=%v unrated=%v", name, format, rated, unrated))
	if r.err != nil {
		return "", nil, r.err
	}
	if _, ok := r.players[name]; !ok {
		return "", nil, store.ErrPlayerNotFound
	}
	return name, []store.Rival{{Name: "Rook", Games: 3, Wins: 2, Draws: 1}, {Name: "Wren", Games: 2, Losses: 2}}, nil
}

// PlayerSeats answers Quill's 2-player seats, recording what was asked.
func (r *discordRatings) PlayerSeats(_ context.Context, _ int64, name string, unrated bool) (store.PlayerSeatStats, error) {
	r.calls++
	r.seatsAsked = append(r.seatsAsked, fmt.Sprintf("%s unrated=%v", name, unrated))
	if r.err != nil {
		return store.PlayerSeatStats{}, r.err
	}
	if _, ok := r.players[name]; !ok {
		return store.PlayerSeatStats{}, store.ErrPlayerNotFound
	}
	return store.PlayerSeatStats{Name: name, Groups: []store.PlayerSeatGroup{{PlayerCount: 2, TotalGames: 10, WinRate: 60,
		Seats: []store.PlayerSeatRow{{Seat: 1, Games: 5, WinRate: 80, DeltaWinRate: 20}, {Seat: 2, Games: 5, WinRate: 40, DeltaWinRate: -20}}}}}, nil
}

// UpsetPairs answers a duel the stronger player lost, and a trio whose favourite timed out, recording the format.
func (r *discordRatings) UpsetPairs(_ context.Context, _ int64, format int) (time.Time, time.Time, []store.UpsetPair, error) {
	r.calls++
	r.upsetsAsked = append(r.upsetsAsked, format)
	if r.err != nil {
		return time.Time{}, time.Time{}, nil, r.err
	}
	day := time.Date(2026, 10, 20, 0, 0, 0, 0, time.UTC)
	duel := []store.UpsetPair{
		{GameID: 1, Date: day, Tournament: "Cup", PlayerCount: 2, Name: "Quill", Other: "Rook", Rating: 1900, RD: 50, OtherRating: 1500, OtherRD: 80, Score: 0},
		{GameID: 1, Date: day, Tournament: "Cup", PlayerCount: 2, Name: "Rook", Other: "Quill", Rating: 1500, RD: 80, OtherRating: 1900, OtherRD: 50, Score: 1},
	}
	trio := []store.UpsetPair{{GameID: 2, Date: day, Tournament: "Cup", PlayerCount: 3, Name: "Wren", Other: "Rook", Rating: 2000, RD: 50,
		OtherRating: 1400, OtherRD: 80, Score: 0, TimedOut: true}}
	pairs := map[int][]store.UpsetPair{0: append(duel, trio...), 2: duel, 3: trio}[format]
	return day.AddDate(0, 0, -16), day.AddDate(0, 0, 14), pairs, nil
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
	NewMux(ratings, nil, nil, discord, nil, nil, log, &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
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
			NewMux(ratings, nil, nil, app, nil, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
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

func TestSuggestionsAreForTheOptionBeingTyped(t *testing.T) {
	body := `{"type":4,"data":{"name":"versus","options":[{"name":"player","type":3,"value":"Quill"},` +
		`{"name":"other","type":3,"value":"groz","focused":true}]}}`
	ratings := newDiscordRatings()
	discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, body, slog.New(slog.DiscardHandler)))
	if !slices.Equal(ratings.searched, []string{"groz"}) {
		t.Errorf("searched for %q, want the focused option's [groz]", ratings.searched)
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

func TestRollIsSharedUnlessAskedPrivate(t *testing.T) {
	tests := []struct {
		name        string
		options     string
		wantPrivate bool
		wantText    string
	}{
		{"shared by default", `{"name":"dice","type":3,"value":"2d6+1"}`, false, "2d6+1"},
		{"private when asked", `{"name":"dice","type":3,"value":"d20"},{"name":"private","type":5,"value":true}`, true, "1d20"},
		{"refused dice, to the asker alone", `{"name":"dice","type":3,"value":"500d6"}`, true, "1 to 100 dice"},
		{"no dice at all", ``, true, "2d6"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("roll", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
			if ratings.calls != 0 {
				t.Errorf("a roll read the ratings %d times, want none", ratings.calls)
			}
		})
	}
}

func TestChooseAndPermutationAreSharedUnlessAskedPrivate(t *testing.T) {
	items := func(v string) string { return `{"name":"items","type":3,"value":"` + v + `"}` }
	count := func(n int) string { return fmt.Sprintf(`{"name":"count","type":4,"value":%d}`, n) }
	const private = `,{"name":"private","type":5,"value":true}`
	tests := []struct {
		name, command, options string
		wantPrivate            bool
		wantText               string
	}{
		{"choose: shared by default, in the order given", "choose", count(2) + "," + items("Alex, Beta"), false, "Alex, Beta"},
		{"choose: private when asked", "choose", count(1) + "," + items("Alex") + private, true, "Alex"},
		{"choose: more than given, to the asker alone", "choose", count(3) + "," + items("Alex, Beta"), true, "can't be chosen"},
		{"choose: no count", "choose", items("Alex, Beta"), true, "can't be chosen"},
		{"choose: no entries", "choose", count(1) + "," + items(" , "), true, "can't be chosen"},
		{"choose: over 100 entries", "choose", count(1) + "," + items(strings.Repeat("a,", 101)), true, "can't be chosen"},
		{"permutation: shared by default", "permutation", items("Alex"), false, "Alex"},
		{"permutation: private when asked", "permutation", items("Alex, Beta") + "," + count(1) + private, true, ""},
		{"permutation: a count of none", "permutation", items("Alex, Beta") + "," + count(0), true, "can't be shuffled"},
		{"permutation: no entries", "permutation", ``, true, "can't be shuffled"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command(tt.command, tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
			if ratings.calls != 0 {
				t.Errorf("/%s read the ratings %d times, want none", tt.command, ratings.calls)
			}
		})
	}
}

func TestPermutationOrdersEveryEntryWhenNoCountIsGiven(t *testing.T) {
	r := discordReply(t, discordDo(newDiscordRatings(), &fakeDiscord{}, discordKey,
		command("permutation", `{"name":"items","type":3,"value":"Alex, Beta, Claus"}`), slog.New(slog.DiscardHandler)))
	wantMessage(t, r, false, "")
	got := strings.Split(*r.Data.Content, ", ")
	slices.Sort(got)
	if !slices.Equal(got, []string{"Alex", "Beta", "Claus"}) {
		t.Errorf("content = %q, want Alex, Beta and Claus each once", *r.Data.Content)
	}
}

func TestVersus(t *testing.T) {
	two := func(a, b string) string {
		return `{"name":"player","type":3,"value":"` + a + `"},{"name":"other","type":3,"value":"` + b + `"}`
	}
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    string
	}{
		{"the record, to the asker alone", two("Quill", "Rook"), nil, true, "**Quill** 1 · **Rook** 1"},
		{"shared with everyone", two("Quill", "Rook") + `,{"name":"share","type":5,"value":true}`, nil, false, "2 games"},
		{"in one format", two("Quill", "Rook") + `,{"name":"format","type":4,"value":3}`, nil, true, "**Quill** 0 · **Rook** 1"},
		{"a name no player has, private though shared", two("Quill", "Nobody") + `,{"name":"share","type":5,"value":true}`, nil, true, "Nobody"},
		{"the same player twice", two("Quill", "quill"), nil, true, "two different players"},
		{"the ratings not answering", two("Quill", "Rook"), errors.New("database is down"), true, "try again"},
		{"no other player", `{"name":"player","type":3,"value":"Quill"}`, nil, true, "/versus"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("versus", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
		})
	}
}

func TestVersusCountsRatedGamesAsTheSiteDoes(t *testing.T) {
	ratings := newDiscordRatings()
	discordDo(ratings, &fakeDiscord{}, discordKey, command("versus", `{"name":"player","type":3,"value":"Quill"},{"name":"other","type":3,"value":"Rook"}`), slog.New(slog.DiscardHandler))
	if want := []string{"Quill/Rook rated=true unrated=false"}; !slices.Equal(ratings.versusAsked, want) {
		t.Errorf("asked Versus for %q, want %q", ratings.versusAsked, want)
	}
}

func TestRivals(t *testing.T) {
	quill := `{"name":"player","type":3,"value":"Quill"}`
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    string
		wantAsked   []string
	}{
		{"to the asker alone, win rate as the site's", quill, nil, true, "83.3% won", []string{"Quill format=0 rated=true unrated=false"}},
		{"shared, in one format", quill + `,{"name":"format","type":4,"value":2},{"name":"share","type":5,"value":true}`, nil, false, "Wren",
			[]string{"Quill format=2 rated=true unrated=false"}},
		{"a name no player has, private though shared", `{"name":"player","type":3,"value":"Nobody"},{"name":"share","type":5,"value":true}`, nil, true, "Nobody",
			[]string{"Nobody format=0 rated=true unrated=false"}},
		{"the ratings not answering", quill, errors.New("database is down"), true, "try again", []string{"Quill format=0 rated=true unrated=false"}},
		{"no name at all", ``, nil, true, "/rivals", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("rivals", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
			if !slices.Equal(ratings.rivalsAsked, tt.wantAsked) {
				t.Errorf("asked PlayerRivals for %q, want %q", ratings.rivalsAsked, tt.wantAsked)
			}
		})
	}
}

func TestCompare(t *testing.T) {
	players := func(names ...string) string {
		opts := make([]string, len(names))
		for i, n := range names {
			opts[i] = fmt.Sprintf(`{"name":"player%d","type":3,"value":"%s"}`, i+1, n)
		}
		return strings.Join(opts, ",")
	}
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    []string
	}{
		{"two players, to the asker alone", players("Quill", "Rook"), nil, true, []string{"**Quill** [SGM]", "**Rook**"}},
		{"four players, shared", players("Quill", "Rook", "Wren", "Finch") + `,{"name":"share","type":5,"value":true}`, nil, false, []string{"**Finch**"}},
		{"a name no player has, private though shared", players("Quill", "Nobody") + `,{"name":"share","type":5,"value":true}`, nil, true, []string{"Nobody"}},
		{"the same player twice", players("Quill", "quill"), nil, true, []string{"different players"}},
		{"the ratings not answering", players("Quill", "Rook"), errors.New("database is down"), true, []string{"try again"}},
		{"one player only", players("Quill"), nil, true, []string{"/compare"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			for _, n := range []string{"Rook", "Wren", "Finch"} {
				ratings.players[n] = store.Player{Name: n, Ratings: []store.PlayerRating{{Model: "glicko2_daneo", Rating: 1500, RD: 100}}}
			}
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("compare", tt.options), slog.New(slog.DiscardHandler)))
			for _, want := range tt.wantText {
				wantMessage(t, r, tt.wantPrivate, want)
			}
		})
	}
}

func TestSeats(t *testing.T) {
	quill := `{"name":"player","type":3,"value":"Quill"}`
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    string
		wantAsked   []string
	}{
		{"rated games, to the asker alone", quill, nil, true, "seat 1 80.0% (+20.0)", []string{"Quill unrated=false"}},
		{"shared, in one format", quill + `,{"name":"format","type":4,"value":2},{"name":"share","type":5,"value":true}`, nil, false, "2-player",
			[]string{"Quill unrated=false"}},
		{"a name no player has, private though shared", `{"name":"player","type":3,"value":"Nobody"},{"name":"share","type":5,"value":true}`, nil, true, "Nobody",
			[]string{"Nobody unrated=false"}},
		{"the ratings not answering", quill, errors.New("database is down"), true, "try again", []string{"Quill unrated=false"}},
		{"no name at all", ``, nil, true, "/seats", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("seats", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
			if !slices.Equal(ratings.seatsAsked, tt.wantAsked) {
				t.Errorf("asked PlayerSeats for %q, want %q", ratings.seatsAsked, tt.wantAsked)
			}
		})
	}
}

func TestUpsets(t *testing.T) {
	tests := []struct {
		name        string
		options     string
		readErr     error
		wantPrivate bool
		wantText    string
		notWant     string
		wantFormat  []int
	}{
		{"the favourite's loss, to the asker alone", ``, nil, true, "**Quill** (1900) lost to **Rook** (1500), 90% to win", "", []int{0}},
		{"a favourite who timed out left out", ``, nil, true, "Quill", "Wren", []int{0}},
		{"shared, in one format", `{"name":"format","type":4,"value":2},{"name":"share","type":5,"value":true}`, nil, false, "rated 2-player games", "", []int{2}},
		{"none in a format", `{"name":"format","type":4,"value":3}`, nil, true, "No upsets", "", []int{3}},
		{"the ratings not answering", ``, errors.New("database is down"), true, "try again", "", []int{0}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			ratings.err = tt.readErr
			r := discordReply(t, discordDo(ratings, &fakeDiscord{}, discordKey, command("upsets", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, tt.wantText)
			if tt.notWant != "" && strings.Contains(*r.Data.Content, tt.notWant) {
				t.Errorf("content = %q, want no %q", *r.Data.Content, tt.notWant)
			}
			if !slices.Equal(ratings.upsetsAsked, tt.wantFormat) {
				t.Errorf("asked UpsetPairs for formats %v, want %v", ratings.upsetsAsked, tt.wantFormat)
			}
		})
	}
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

func TestATurnedOffCommandSaysSoToTheAskerAlone(t *testing.T) {
	for _, tt := range []struct{ name, body string }{
		{"rating", rating(`{"name":"player","type":3,"value":"Quill"},{"name":"share","type":5,"value":true}`)},
		{"roll", command("roll", `{"name":"dice","type":3,"value":"2d6"}`)},
		{"choose", command("choose", `{"name":"count","type":4,"value":1},{"name":"items","type":3,"value":"Alex"}`)},
		{"permutation", command("permutation", `{"name":"items","type":3,"value":"Alex"}`)},
		{"upsets", command("upsets", `{"name":"share","type":5,"value":true}`)},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			app := &fakeDiscord{commands: map[string]store.CommandSettings{tt.name: {Name: tt.name, On: false, Reply: "always_shared"}}}
			r := discordReply(t, discordDo(ratings, app, discordKey, tt.body, slog.New(slog.DiscardHandler)))
			wantMessage(t, r, true, "/"+tt.name+" is turned off")
			if ratings.calls != 0 {
				t.Errorf("the ratings were asked %d times, want none for a command turned off", ratings.calls)
			}
		})
	}
}

func TestWhoSeesAReplyFollowsTheCommandsSetting(t *testing.T) {
	quill := `{"name":"player","type":3,"value":"Quill"}`
	share := func(v bool) string { return fmt.Sprintf(`,{"name":"share","type":5,"value":%t}`, v) }
	tests := []struct {
		name        string
		reply       string
		readErr     error
		options     string
		wantPrivate bool
	}{
		{"nothing saved, not given", "", nil, quill, true},
		{"nothing saved, shared", "", nil, quill + share(true), false},
		{"shared unless private, not given", "shared", nil, quill, false},
		{"shared unless private, asked private", "shared", nil, quill + share(false), true},
		{"private unless shared, asked shared", "private", nil, quill + share(true), false},
		{"always private, asked shared", "always_private", nil, quill + share(true), true},
		{"always shared, asked private", "always_shared", nil, quill + share(false), false},
		{"the settings not answering: the default", "always_shared", errors.New("database is down"), quill, true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &fakeDiscord{settingsErr: tt.readErr}
			if tt.reply != "" {
				app.commands = map[string]store.CommandSettings{"rating": {Name: "rating", On: true, Reply: tt.reply}}
			}
			r := discordReply(t, discordDo(newDiscordRatings(), app, discordKey, rating(tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, "Quill")
		})
	}
}

func TestRollsReplyFollowsItsSettingThroughItsPrivateOption(t *testing.T) {
	dice := `{"name":"dice","type":3,"value":"2d6"}`
	private := func(v bool) string { return fmt.Sprintf(`,{"name":"private","type":5,"value":%t}`, v) }
	tests := []struct {
		name, reply, options string
		wantPrivate          bool
	}{
		{"nothing saved: shared", "", dice, false},
		{"private unless shared, not given", "private", dice, true},
		{"private unless shared, asked public", "private", dice + private(false), false},
		{"always shared, asked private", "always_shared", dice + private(true), false},
		{"always private, asked public", "always_private", dice + private(false), true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			app := &fakeDiscord{}
			if tt.reply != "" {
				app.commands = map[string]store.CommandSettings{"roll": {Name: "roll", On: true, Reply: tt.reply}}
			}
			r := discordReply(t, discordDo(newDiscordRatings(), app, discordKey, command("roll", tt.options), slog.New(slog.DiscardHandler)))
			wantMessage(t, r, tt.wantPrivate, "2d6")
		})
	}
}

func TestACommandsDefaultFormatIsUsedWhenNoneIsGiven(t *testing.T) {
	quill := `{"name":"player","type":3,"value":"Quill"}`
	tests := []struct {
		name, options string
		wantAsked     string
	}{
		{"none given", quill, "Quill format=2 rated=true unrated=false"},
		{"one given", quill + `,{"name":"format","type":4,"value":3}`, "Quill format=3 rated=true unrated=false"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ratings := newDiscordRatings()
			app := &fakeDiscord{commands: map[string]store.CommandSettings{"rivals": {Name: "rivals", On: true, Reply: "private", Format: 2}}}
			discordReply(t, discordDo(ratings, app, discordKey, command("rivals", tt.options), slog.New(slog.DiscardHandler)))
			if !slices.Equal(ratings.rivalsAsked, []string{tt.wantAsked}) {
				t.Errorf("asked %q, want %q", ratings.rivalsAsked, tt.wantAsked)
			}
		})
	}
}

func TestAListCommandShowsAsManyRowsAsSet(t *testing.T) {
	app := &fakeDiscord{commands: map[string]store.CommandSettings{"rivals": {Name: "rivals", On: true, Reply: "private", Results: 1}}}
	r := discordReply(t, discordDo(newDiscordRatings(), app, discordKey, command("rivals", `{"name":"player","type":3,"value":"Quill"}`), slog.New(slog.DiscardHandler)))
	wantMessage(t, r, true, "Rook")
	if strings.Contains(*r.Data.Content, "Wren") {
		t.Errorf("reply = %q, want only the first rival", *r.Data.Content)
	}
}
