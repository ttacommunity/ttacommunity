package httpapi

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// gameOf names a game other than the first in a fake's notes, as " game <n>".
func gameOf(number int) string {
	if number == 1 {
		return ""
	}
	return fmt.Sprintf(" game %d", number)
}

func (f *fakeSignIns) ReportBlitzGame(_ context.Context, hash []byte, slug, edition string, match int64, number int, g store.BlitzGame, _ time.Time) error {
	if f.reportErr != nil {
		return f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("report %s %s %d%s by %s: %s %g/%s/turn %d %g/%s/turn %d at %s", slug, edition, match, gameOf(number), f.sessions[string(hash)].Username,
		g.ShareCode, g.A.Culture, g.A.Outcome, g.A.Turn, g.B.Culture, g.B.Outcome, g.B.Turn, g.Finished.UTC().Format(time.RFC3339)))
	return nil
}

func (f *fakeSignIns) ConfirmBlitzResult(_ context.Context, hash []byte, slug, edition string, match int64, number int, _ time.Time) error {
	if f.reportErr != nil {
		return f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("confirm %s %s %d%s by %s", slug, edition, match, gameOf(number), f.sessions[string(hash)].Username))
	return nil
}

func (f *fakeSignIns) FlagBlitzResult(_ context.Context, hash []byte, slug, edition string, match int64, number int, reason string, _ time.Time) error {
	if f.reportErr != nil {
		return f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("flag %s %s %d%s by %s: %s", slug, edition, match, gameOf(number), f.sessions[string(hash)].Username, reason))
	return nil
}

func (f *fakeSignIns) AnswerBlitzBestOf(_ context.Context, hash []byte, slug, edition string, match int64, answer string, _ time.Time) error {
	if f.reportErr != nil {
		return f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("best of %s %s %d by %s: %s", slug, edition, match, f.sessions[string(hash)].Username, answer))
	return nil
}

// playerDoWith is playerDo for a route that also reads ratings.
func playerDoWith(t *testing.T, ratings Ratings, signIns SignIns, method, path, origin, body string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	if origin != "" {
		req.Header.Set("Origin", origin)
	}
	for _, c := range cookies {
		req.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	NewMux(ratings, nil, signIns, nil, nil, nil, nil, slog.New(slog.DiscardHandler), &recordedRequests{}, stepClock(time.Millisecond)).ServeHTTP(rec, req)
	return rec
}

// reportsEdition is us_open 2026 with Ann (entrant 7, member 1007) v bob (8, member 1008) in published round 1, and
// Cy (9, member 1009) on the bye; change adjusts Ann's match.
func reportsEdition(change func(ed *store.BlitzEdition, m *store.BlitzMatch)) *store.BlitzEdition {
	at := time.Date(2026, 10, 12, 12, 0, 0, 0, time.UTC)
	bob := int64(8)
	ed := store.BlitzEdition{
		BlitzEditionRow: store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat,
			Points: blitz.FIDEScoring, Round: new(1), PairedRound: new(1)},
		Entrants: []store.BlitzEntrant{
			{ID: 7, MemberID: 1007, Name: "Ann", Zone: "UTC", Status: "active"},
			{ID: 8, MemberID: 1008, Name: "bob", Zone: "UTC", Status: "active"},
			{ID: 9, MemberID: 1009, Name: "Cy", Zone: "UTC", Status: "active"},
		},
		Rounds: []store.BlitzRound{{ID: 101, Number: 1, Kind: "swiss", Method: "timezone", PublishedAt: &at, BestOf: 1,
			Matches: []store.BlitzMatch{{ID: 21, A: 7, B: &bob, BestOf: 1}, {ID: 22, A: 9, BestOf: 1}}}},
	}
	if change != nil {
		change(&ed, &ed.Rounds[0].Matches[0])
	}
	return &ed
}

// reportedByAnn makes Ann's match a 50–40 game she reported, standing as report.
func reportedByAnn(report string) func(*store.BlitzEdition, *store.BlitzMatch) {
	return func(_ *store.BlitzEdition, m *store.BlitzMatch) {
		at, game, ann := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC), int64(500), int64(7)
		m.Result, m.GameID, m.DecidedAt, m.ReportedBy, m.Report = "a", &game, &at, &ann, report
		m.Game = &store.BlitzGame{ShareCode: "REP-1", A: store.BlitzSide{Culture: 50, Outcome: "finished"}, B: store.BlitzSide{Culture: 40, Outcome: "finished"}, Finished: at}
		if report == "flagged" {
			m.FlagReason = "I won"
		}
		*m = withGames(*m, "a")
	}
}

// withGames gives a match one game, as the store reads it, its first game as m's with result (none for ""), and makes
// it one game.
func withGames(m store.BlitzMatch, result string) store.BlitzMatch {
	m.BestOf, m.Games = 1, []store.BlitzMatchGame{}
	if m.Game != nil {
		m.Games = []store.BlitzMatchGame{{Number: 1, GameID: *m.GameID, Game: *m.Game, Result: result, ReportedBy: m.ReportedBy, Report: m.Report, FlagReason: m.FlagReason}}
	}
	return m
}

// oneEach makes Ann's match a best of 3 at one game each: Ann reported her win in game 1, bob his in game 2.
func oneEach(ed *store.BlitzEdition, m *store.BlitzMatch) {
	ed.Rounds[0].BestOf = 3
	at, ann, bob := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC), int64(7), int64(8)
	game := store.BlitzGame{ShareCode: "BO3-1", A: store.BlitzSide{Culture: 50, Outcome: "finished"}, B: store.BlitzSide{Culture: 40, Outcome: "finished"}, Finished: at}
	m.BestOf, m.Games = 3, []store.BlitzMatchGame{
		{Number: 1, GameID: 500, Game: game, Result: "a", ReportedBy: &ann, Report: "reported"},
		{Number: 2, GameID: 501, Game: game, Result: "b", ReportedBy: &bob, Report: "reported"},
	}
}

func member(id int64, name string) store.Account {
	return store.Account{ID: id, Provider: "discord", Username: strings.ToLower(name), Member: &store.LinkedMember{ID: id, Name: name}}
}

func TestAPlayerReadsTheirMatchAndWhatTheyMayDo(t *testing.T) {
	tests := []struct {
		name    string
		account store.Account
		edition *store.BlitzEdition
		want    map[string]string // JSON paths in the answer
	}{
		{"not linked", store.Account{ID: 1, Provider: "discord", Username: "newcomer"}, reportsEdition(nil),
			map[string]string{"match": `null`, "reports": `[]`}},
		{"not entered", member(1050, "Eve"), reportsEdition(reportedByAnn("reported")), map[string]string{"match": `null`, "reports": `[]`}},
		{"undecided", member(1008, "bob"), reportsEdition(nil), map[string]string{
			"match.id": `21`, "match.round": `1`, "match.a": `"Ann"`, "match.b": `"bob"`, "match.me": `"b"`, "match.result": `null`,
			"match.report": `null`, "match.reported_by": `null`, "match.actions": `["report"]`}},
		{"their own report", member(1007, "Ann"), reportsEdition(reportedByAnn("reported")), map[string]string{
			"match.me": `"a"`, "match.result": `"a"`, "match.report": `"reported"`, "match.reported_by": `"a"`, "match.actions": `["report"]`,
			"match.game.share_code": `"REP-1"`}},
		{"the opponent's report", member(1008, "bob"), reportsEdition(reportedByAnn("reported")), map[string]string{
			"match.reported_by": `"a"`, "match.actions": `["confirm", "flag"]`}},
		{"flagged", member(1007, "Ann"), reportsEdition(reportedByAnn("flagged")), map[string]string{
			"match.report": `"flagged"`, "match.flag_reason": `"I won"`, "match.actions": `[]`}},
		{"an organiser's ruling", member(1008, "bob"), reportsEdition(func(_ *store.BlitzEdition, m *store.BlitzMatch) {
			at := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
			m.Result, m.Ruling, m.DecidedAt = "b", "Ann did not show", &at
		}), map[string]string{"match.ruled": `true`, "match.actions": `[]`}},
		{"the next round paired", member(1008, "bob"), reportsEdition(func(ed *store.BlitzEdition, _ *store.BlitzMatch) {
			ed.PairedRound = new(2)
		}), map[string]string{"match.id": `21`, "match.actions": `[]`}},
		{"a bye", member(1009, "Cy"), reportsEdition(nil), map[string]string{"match.id": `22`, "match.b": `null`, "match.actions": `[]`, "reports": `[]`,
			"match.next_game": `null`}},
		{"a one-game round", member(1008, "bob"), reportsEdition(nil), map[string]string{
			"match.best_of": `1`, "match.best_of_offered": `false`, "match.best_of_actions": `[]`, "match.next_game": `1`, "match.games": `[]`}},
		{"the opponent's proposal", member(1008, "bob"), reportsEdition(func(ed *store.BlitzEdition, m *store.BlitzMatch) {
			ed.Rounds[0].BestOf, m.ProposedBy = 3, new(int64(7))
		}), map[string]string{"match.best_of_offered": `true`, "match.proposed_by": `"a"`, "match.best_of_actions": `["accept", "decline"]`}},
		{"their own proposal", member(1007, "Ann"), reportsEdition(func(ed *store.BlitzEdition, m *store.BlitzMatch) {
			ed.Rounds[0].BestOf, m.ProposedBy = 3, new(int64(7))
		}), map[string]string{"match.best_of_actions": `["withdraw"]`}},
		{"a best of 3 at one game each", member(1008, "bob"), reportsEdition(oneEach), map[string]string{
			"match.best_of": `3`, "match.best_of_actions": `[]`, "match.next_game": `3`,
			"match.games.0.number": `1`, "match.games.0.result": `"a"`, "match.games.0.reported_by": `"a"`, "match.games.0.actions": `["confirm", "flag"]`,
			"match.games.1.number": `2`, "match.games.1.result": `"b"`, "match.games.1.reported_by": `"b"`, "match.games.1.actions": `["report"]`,
			"reports": `[{"round": 1, "game": 1, "reported_by": "Ann"}, {"round": 1, "game": 2, "reported_by": "bob"}]`}},
		{"another entrant's report", member(1009, "Cy"), reportsEdition(reportedByAnn("confirmed")), map[string]string{
			"reports": `[{"round": 1, "a": "Ann", "reported_by": "Ann", "confirmed": true}]`}},
		{"a dropped entrant", member(1009, "Cy"), reportsEdition(func(ed *store.BlitzEdition, m *store.BlitzMatch) {
			reportedByAnn("reported")(ed, m)
			ed.Entrants[2].Status = "withdrawn"
		}), map[string]string{"reports": `[{"round": 1, "a": "Ann", "reported_by": "Ann", "confirmed": false}]`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			cookie := sessionFor(f, "token", tt.account)
			rec := playerDoWith(t, &fakeRatings{blitzEdition: tt.edition}, f, http.MethodGet, "/v1/me/blitz/us_open/2026", siteOrigin, "", cookie)
			if rec.Code != http.StatusOK {
				t.Fatalf("GET = %d %s, want 200", rec.Code, rec.Body)
			}
			for path, want := range tt.want {
				wantJSON(t, rec.Body.Bytes(), path, want)
			}
			if got := rec.Header().Get("Cache-Control"); got != cacheNone {
				t.Errorf("Cache-Control = %q, want %q", got, cacheNone)
			}
		})
	}
}

func TestAPlayersMatchIsRefusedWhenItShouldBe(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", member(1008, "bob"))
	ratings := &fakeRatings{blitzEdition: reportsEdition(nil)}
	tests := []struct {
		name, path string
		cookies    []*http.Cookie
		want       int
	}{
		{"signed out", "/v1/me/blitz/us_open/2026", nil, http.StatusUnauthorized},
		{"no such edition", "/v1/me/blitz/us_open/2025", []*http.Cookie{cookie}, http.StatusNotFound},
		{"a bad tournament", "/v1/me/blitz/US%20Open/2026", []*http.Cookie{cookie}, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := playerDoWith(t, ratings, f, http.MethodGet, tt.path, siteOrigin, "", tt.cookies...); rec.Code != tt.want {
				t.Errorf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
		})
	}
}

func TestPlayersReportConfirmAndFlag(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", member(1008, "bob"))
	for _, r := range []struct{ method, path, body string }{
		{http.MethodPut, "/v1/me/blitz/us_open/2026/matches/21/games/1",
			`{"share_code": "REP-1", "a": {"culture": 50, "outcome": "finished", "turn": 2}, "b": {"culture": 40, "outcome": "finished", "turn": 1}, "finished_on": "2026-10-10", "finished_time": "21:30", "zone": "Europe/Berlin"}`},
		{http.MethodPost, "/v1/me/blitz/us_open/2026/matches/21/games/2/confirm", ""},
		{http.MethodPost, "/v1/me/blitz/us_open/2026/matches/21/games/3/flag", `{"reason": "I won on culture"}`},
		{http.MethodPost, "/v1/me/blitz/us_open/2026/matches/21/best-of", `{"answer": "propose"}`},
	} {
		if rec := playerDo(t, f, r.method, r.path, siteOrigin, r.body, cookie); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", r.method, r.path, rec.Code, rec.Body)
		}
	}
	want := "report us_open 2026 21 by bob: REP-1 50/finished/turn 2 40/finished/turn 1 at 2026-10-10T19:30:00Z|" +
		"confirm us_open 2026 21 game 2 by bob|flag us_open 2026 21 game 3 by bob: I won on culture|best of us_open 2026 21 by bob: propose"
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("what reached the store = %q, want %q", got, want)
	}
}

func TestBestOfAnswersAreRefusedWithTheStatusTheyNeed(t *testing.T) {
	tests := []struct {
		name, body string
		err        error
		want       int
	}{
		{"an answer that does not fit", `{"answer": "accept"}`, store.ErrBadBlitzBestOf, http.StatusBadRequest},
		{"a round of one-game matches", `{"answer": "propose"}`, store.ErrBlitzBestOfNotOffered, http.StatusConflict},
		{"settled", `{"answer": "propose"}`, store.ErrBlitzBestOfClosed, http.StatusConflict},
		{"someone else's match", `{"answer": "propose"}`, store.ErrNotYourMatch, http.StatusForbidden},
		{"an answer that is none", `{"answer": "maybe"}`, nil, http.StatusBadRequest},
		{"no answer", ``, nil, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.reportErr = tt.err
			cookie := sessionFor(f, "token", member(1008, "bob"))
			if rec := playerDo(t, f, http.MethodPost, "/v1/me/blitz/us_open/2026/matches/21/best-of", siteOrigin, tt.body, cookie); rec.Code != tt.want {
				t.Errorf("POST best-of %s = %d %s, want %d", tt.body, rec.Code, rec.Body, tt.want)
			}
			if tt.err == nil && len(f.notes) != 0 {
				t.Errorf("a refused answer reached the store: %q", f.notes)
			}
		})
	}
}

func TestPlayersReportsAreRefusedWithTheStatusTheyNeed(t *testing.T) {
	game := `{"share_code": "REP-1", "a": {"culture": 50, "outcome": "finished", "turn": 1}, "b": {"culture": 40, "outcome": "finished", "turn": 2}, "finished_on": "2026-10-10", "finished_time": "21:30", "zone": "UTC"}`
	tests := []struct {
		name string
		err  error
		want int
	}{
		{"no share code", store.ErrBlitzShareCodeNeeded, http.StatusBadRequest},
		{"a bad game", store.ErrBadBlitzGame, http.StatusBadRequest},
		{"a bye", store.ErrBlitzBye, http.StatusBadRequest},
		{"finished after now", store.ErrBlitzFinishedAhead, http.StatusBadRequest},
		{"an ended session", store.ErrNoSession, http.StatusUnauthorized},
		{"not linked", store.ErrNotLinked, http.StatusForbidden},
		{"banned", store.ErrMemberBanned, http.StatusForbidden},
		{"someone else's match", store.ErrNotYourMatch, http.StatusForbidden},
		{"no such edition", store.ErrNoBlitzEdition, http.StatusNotFound},
		{"no such match", store.ErrNoBlitzMatch, http.StatusNotFound},
		{"the next round paired", store.ErrBlitzRoundClosed, http.StatusConflict},
		{"locked", store.ErrBlitzResultLocked, http.StatusConflict},
		{"a share code taken", &store.BlitzShareCodeError{GameID: 55}, http.StatusConflict},
		{"a game the match does not take", store.ErrNoBlitzGame, http.StatusNotFound},
		{"a game after the match is decided", store.ErrBlitzGameNotNeeded, http.StatusConflict},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.reportErr = tt.err
			cookie := sessionFor(f, "token", member(1008, "bob"))
			if rec := playerDo(t, f, http.MethodPut, "/v1/me/blitz/us_open/2026/matches/21/games/1", siteOrigin, game, cookie); rec.Code != tt.want {
				t.Errorf("PUT game = %d %s, want %d", rec.Code, rec.Body, tt.want)
			}
		})
	}
	t.Run("a game finishing after now", func(t *testing.T) {
		f := newFakeSignIns()
		cookie := sessionFor(f, "token", member(1008, "bob"))
		ahead := strings.Replace(game, `"finished_on": "2026-10-10"`, `"finished_on": "2026-10-12"`, 1)
		rec := playerDo(t, f, http.MethodPut, "/v1/me/blitz/us_open/2026/matches/21/games/1", siteOrigin, ahead, cookie)
		if rec.Code != http.StatusBadRequest || !strings.Contains(rec.Body.String(), "after now") {
			t.Errorf("PUT game finishing after now = %d %s, want 400 saying so", rec.Code, rec.Body)
		}
		if len(f.notes) != 0 {
			t.Errorf("a refused game reached the store: %q", f.notes)
		}
	})
	t.Run("a flag of the wrong length", func(t *testing.T) {
		f := newFakeSignIns()
		f.reportErr = store.ErrBadBlitzFlag
		cookie := sessionFor(f, "token", member(1008, "bob"))
		if rec := playerDo(t, f, http.MethodPost, "/v1/me/blitz/us_open/2026/matches/21/games/1/flag", siteOrigin, `{"reason": "x"}`, cookie); rec.Code != http.StatusBadRequest {
			t.Errorf("POST flag = %d %s, want 400", rec.Code, rec.Body)
		}
	})
	t.Run("from another site", func(t *testing.T) {
		f := newFakeSignIns()
		cookie := sessionFor(f, "token", member(1008, "bob"))
		for _, path := range []string{"/v1/me/blitz/us_open/2026/matches/21/games/1/confirm", "/v1/me/blitz/us_open/2026/matches/21/games/1/flag"} {
			if rec := playerDo(t, f, http.MethodPost, path, "https://evil.example", `{"reason": "x"}`, cookie); rec.Code != http.StatusForbidden {
				t.Errorf("POST %s from another site = %d, want 403", path, rec.Code)
			}
		}
		if len(f.notes) != 0 {
			t.Errorf("refused changes reached the store: %q", f.notes)
		}
	})
}

func TestTheSiteShowsWhenResultsChangedButNotWhoReported(t *testing.T) {
	for _, report := range []string{"reported", "confirmed", "flagged"} {
		rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: reportsEdition(reportedByAnn(report))}, "/v1/blitz/us_open/2026")
		if rec.Code != http.StatusOK {
			t.Fatalf("GET %s = %d %s", report, rec.Code, rec.Body)
		}
		wantJSON(t, rec.Body.Bytes(), "rounds.0.matches.0.result", `"a"`)
		wantJSON(t, rec.Body.Bytes(), "results_updated_at", `"2026-10-13T20:00:00Z"`)
		for _, hidden := range []string{"reported", "confirmed", "flag", "I won"} {
			if strings.Contains(rec.Body.String(), hidden) {
				t.Errorf("%s: the public edition says %q: %s", report, hidden, rec.Body)
			}
		}
	}
	rec, _ := serve(t, &fakeRatings{run: store.PublishedRun{ID: 7}, blitzEdition: reportsEdition(nil)}, "/v1/blitz/us_open/2026")
	wantJSON(t, rec.Body.Bytes(), "results_updated_at", `null`)
}

func (f *fakeAdmins) KeepBlitzReport(_ context.Context, slug, edition string, id int64, number int) error {
	m, err := f.findMatch(slug, edition, id)
	if err != nil {
		return err
	}
	if m.Report != "flagged" {
		return store.ErrBlitzNotFlagged
	}
	m.Report = "resolved"
	f.notes = append(f.notes, fmt.Sprintf("keep %d%s", id, gameOf(number)))
	return nil
}

func TestAnOrganiserSeesAFlagAndKeepsTheReport(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	ann, game := int64(7), int64(500)
	at := time.Date(2026, 10, 13, 20, 0, 0, 0, time.UTC)
	m := &f.rounds[0].Matches[0]
	m.Result, m.GameID, m.DecidedAt, m.ReportedBy, m.Report, m.FlagReason = "a", &game, &at, &ann, "flagged", "I won"
	m.Game = &store.BlitzGame{ShareCode: "REP-1", A: store.BlitzSide{Culture: 50, Outcome: "finished"}, B: store.BlitzSide{Culture: 40, Outcome: "finished"}, Finished: at}

	wantJSON(t, adminDo(t, f, http.MethodGet, "/v1/admin/blitz", site, "").Body.Bytes(), "0.flagged", `1`)
	rec := adminDo(t, f, http.MethodGet, "/v1/admin/blitz/us_open/2026", site, "")
	wantJSON(t, rec.Body.Bytes(), "rounds.0.matches.0.reported_by", `7`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.matches.0.report", `"flagged"`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.matches.0.flag_reason", `"I won"`)
	wantJSON(t, rec.Body.Bytes(), "rounds.0.matches.1.report", `null`)

	if rec := adminDo(t, f, http.MethodPost, "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/games/1/keep", site, ""); rec.Code != http.StatusNoContent {
		t.Fatalf("POST keep = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.audits) != 1 || f.audits[0] != org+" blitz.match.keep 1010" {
		t.Errorf("audits = %q, want the keep", f.audits)
	}
	tests := []struct {
		name, path, origin string
		want               int
	}{
		{"not flagged", "/v1/admin/blitz/us_open/2026/rounds/101/matches/1011/games/1/keep", site, http.StatusConflict},
		{"no such match", "/v1/admin/blitz/us_open/2026/rounds/101/matches/9999/games/1/keep", site, http.StatusNotFound},
		{"no such edition", "/v1/admin/blitz/us_open/2025/rounds/101/matches/1010/games/1/keep", site, http.StatusNotFound},
		{"from another site", "/v1/admin/blitz/us_open/2026/rounds/101/matches/1010/games/1/keep", "https://evil.example", http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec := adminDo(t, f, http.MethodPost, tt.path, tt.origin, ""); rec.Code != tt.want {
				t.Errorf("POST %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
		})
	}
	if !slices.Contains(f.notes, "keep 1010") {
		t.Errorf("changes = %q, want the keep of 1010", f.notes)
	}
}
