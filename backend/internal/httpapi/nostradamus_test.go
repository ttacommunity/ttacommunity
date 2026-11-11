package httpapi

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (f *fakeRatings) NostradamusEditions(context.Context) ([]store.NostradamusEdition, error) {
	return []store.NostradamusEdition{{Slug: "wc-2031", Name: "Nostradamus #9", Series: "worlds", Tournament: "2031"}}, nil
}

// nostradamusGame is wc-2031: a closed round S1 (Bea linked to Beatrix, a bot who missed it) and an open round S2.
func nostradamusGame() store.NostradamusGame {
	closes := time.Date(2031, 4, 1, 12, 0, 0, 0, time.UTC)
	return store.NostradamusGame{
		NostradamusEdition: store.NostradamusEdition{Slug: "wc-2031", Name: "Nostradamus #9", Series: "worlds", Tournament: "2031"},
		Rounds: []store.NostradamusRound{
			{ID: 1, Label: "S1", Name: "Stage 1", Zone: "Europe/Berlin", Tokens: 3, TokenPoints: 10, ClosesAt: &closes, PublishedAt: &closes, ClosedAt: &closes,
				Questions: []store.NostradamusQuestion{{ID: 5, Label: "Win", Prompt: "Who wins?", Kind: "pick", Points: 4, Picks: 1,
					Choices: []store.NostradamusChoice{{Label: "Ann", Seed: 2}, {Label: "Cy"}}, Outcome: []string{"Ann"},
					Counts: []store.NostradamusCount{{Value: "Ann", Answers: 3}}}}},
			{ID: 2, Label: "S2", Name: "Stage 2", Zone: "UTC", Tokens: 3, TokenPoints: 10, ClosesAt: &closes, PublishedAt: &closes,
				Questions: []store.NostradamusQuestion{{ID: 6, Label: "Top", Kind: "set", Points: 6, Picks: 2}}},
		},
		Scored: []string{"S1"},
		Standings: []store.NostradamusStanding{
			{Name: "Bea", Member: "Beatrix", Total: 14, Rounds: []store.NostradamusRoundScore{{Label: "S1", Points: 14}}},
			{Name: "Seed", Bot: true, Rounds: []store.NostradamusRoundScore{{Label: "S1", Missed: true}}},
		},
	}
}

func (f *fakeRatings) NostradamusGame(_ context.Context, slug string) (store.NostradamusGame, error) {
	if slug != "wc-2031" {
		return store.NostradamusGame{}, store.ErrNoNostradamus
	}
	return nostradamusGame(), nil
}

func TestNostradamusEditionsAndGame(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/nostradamus")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET editions = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "editions", `[{"slug": "wc-2031", "name": "Nostradamus #9", "series": "worlds", "tournament": "2031"}]`)
	rec, _ = serve(t, &fakeRatings{}, "/v1/nostradamus/wc-2031")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "rounds", `[
		{"label": "S1", "zone": "Europe/Berlin", "closes_at": "2031-04-01T12:00:00Z", "published": true, "closed": true, "questions": [
			{"id": 5, "kind": "pick", "picks": 1, "choices": [{"label": "Ann"}, {"label": "Cy"}], "outcome": ["Ann"], "counts": [{"value": "Ann", "answers": 3}]}]},
		{"label": "S2", "closed": false, "questions": [{"kind": "set", "picks": 2, "choices": [], "outcome": null, "counts": []}]}]`)
	wantJSON(t, rec.Body.Bytes(), "standings", `[{"name": "Bea", "member": "Beatrix", "total": 14, "rounds": [{"round": "S1", "points": 14}]},
		{"name": "Seed", "member": null, "bot": true, "rounds": [{"round": "S1", "points": 0, "missed": true}]}]`)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want a minute", got)
	}
	if rec, _ := serve(t, &fakeRatings{}, "/v1/nostradamus/wc-2099"); rec.Code != http.StatusNotFound {
		t.Errorf("GET an unknown game = %d, want 404", rec.Code)
	}
}

func (f *fakeSignIns) MyNostradamus(_ context.Context, hash []byte, slug string, _ time.Time) (store.MyNostradamus, error) {
	if f.reportErr != nil {
		return store.MyNostradamus{}, f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("mine in %s by %s", slug, f.sessions[string(hash)].Username))
	return store.MyNostradamus{Status: "accepted", Answers: map[int64]store.NostradamusGiven{6: {Question: 6, Answer: []string{"Ann", "Cy"}, Token: true}}}, nil
}

func (f *fakeSignIns) RequestNostradamusEntry(_ context.Context, hash []byte, slug string, _ time.Time) (string, error) {
	if f.reportErr != nil {
		return "", f.reportErr
	}
	f.notes = append(f.notes, fmt.Sprintf("join %s by %s", slug, f.sessions[string(hash)].Username))
	return "requested", nil
}

func (f *fakeSignIns) SubmitNostradamusAnswers(_ context.Context, hash []byte, round int64, answers []store.NostradamusGiven, _ time.Time) (int, error) {
	if f.reportErr != nil {
		return 0, f.reportErr
	}
	for _, a := range answers {
		f.notes = append(f.notes, fmt.Sprintf("round %d by %s: %d %q %t", round, f.sessions[string(hash)].Username, a.Question, a.Answer, a.Token))
	}
	return len(answers), nil
}

func TestAPlayerJoinsReadsAndAnswersTheirNostradamus(t *testing.T) {
	f := newFakeSignIns()
	cookie := sessionFor(f, "token", member(1007, "ann"))
	rec := playerDo(t, f, http.MethodPost, "/v1/me/nostradamus/wc-2031/entry", siteOrigin, "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("POST entry = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"status": "requested"}`)
	rec = playerDo(t, f, http.MethodGet, "/v1/me/nostradamus/wc-2031", "", "", cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET mine = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"status": "accepted", "answers": [{"question": 6, "answer": ["Ann", "Cy"], "token": true}]}`)
	if got := rec.Header().Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control = %q, want no-store", got)
	}
	rec = playerDo(t, f, http.MethodPut, "/v1/me/nostradamus/wc-2031/rounds/2/answers", siteOrigin,
		`{"answers": [{"question": 6, "answer": [" Ann ", "Cy"], "token": true}]}`, cookie)
	if rec.Code != http.StatusOK {
		t.Fatalf("PUT answers = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "saved", `1`)
	want := `join wc-2031 by ann|mine in wc-2031 by ann|round 2 by ann: 6 ["Ann" "Cy"] true`
	if got := strings.Join(f.notes, "|"); got != want {
		t.Errorf("what reached the store = %q, want %q", got, want)
	}
}

func TestPlayersNostradamusIsRefusedWithTheStatusItNeeds(t *testing.T) {
	answers, path := `{"answers": [{"question": 6, "answer": ["Ann"], "token": false}]}`, "/v1/me/nostradamus/wc-2031/rounds/2/answers"
	tests := []struct {
		name, method, path, origin, body string
		err                              error
		want                             int
	}{
		{"an ended session", http.MethodPut, path, siteOrigin, answers, store.ErrNoSession, http.StatusUnauthorized},
		{"not linked", http.MethodPut, path, siteOrigin, answers, store.ErrNotLinked, http.StatusForbidden},
		{"not accepted", http.MethodPut, path, siteOrigin, answers, store.ErrNotEntered, http.StatusForbidden},
		{"a round not open", http.MethodPut, path, siteOrigin, answers, store.ErrRoundNotOpen, http.StatusConflict},
		{"after the deadline", http.MethodPut, path, siteOrigin, answers, store.ErrPastDeadline, http.StatusConflict},
		{"an answer the rules refuse", http.MethodPut, path, siteOrigin, answers, store.ErrBadNostradamus, http.StatusBadRequest},
		{"an empty answer", http.MethodPut, path, siteOrigin, `{"answers": [{"question": 6, "answer": [" "], "token": false}]}`, nil, http.StatusBadRequest},
		{"answers not from the site", http.MethodPut, path, "https://elsewhere.example", answers, nil, http.StatusForbidden},
		{"joining no game", http.MethodPost, "/v1/me/nostradamus/wc-2099/entry", siteOrigin, "", store.ErrNoNostradamus, http.StatusNotFound},
		{"joining banned", http.MethodPost, "/v1/me/nostradamus/wc-2031/entry", siteOrigin, "", store.ErrMemberBanned, http.StatusForbidden},
		{"reading signed out", http.MethodGet, "/v1/me/nostradamus/wc-2031", "", "", store.ErrNoSession, http.StatusUnauthorized},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFakeSignIns()
			f.reportErr = tt.err
			cookie := sessionFor(f, "token", member(1007, "ann"))
			if rec := playerDo(t, f, tt.method, tt.path, tt.origin, tt.body, cookie); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.err == nil && len(f.notes) > 0 {
				t.Errorf("a refused request reached the store: %v", f.notes)
			}
		})
	}
}

func (f *fakeAdmins) NostradamusEditions(context.Context) ([]store.NostradamusEdition, error) {
	return []store.NostradamusEdition{{Slug: "wc-2031", Name: "Nostradamus #9", Series: "worlds", Tournament: "2031"}}, nil
}

func (f *fakeAdmins) NostradamusGame(_ context.Context, slug string) (store.NostradamusGame, error) {
	if slug != "wc-2031" {
		return store.NostradamusGame{}, store.ErrNoNostradamus
	}
	g := nostradamusGame()
	g.Participants = []store.NostradamusParticipant{{ID: 8, Name: "Bea", Member: "Beatrix", DiscordName: "bea_d", Status: "accepted"},
		{ID: 9, Name: "Seed", Bot: true, BotRule: "seed", Status: "accepted"}}
	return g, nil
}

// nostradamusChange notes a change, or refuses it with err when set; ids of 99 are not in the game.
func (f *fakeAdmins) nostradamusChange(note string, ids ...int64) error {
	for _, id := range ids {
		if id == 99 {
			return store.ErrNoNostradamus
		}
	}
	if f.nostradamusErr != nil {
		return f.nostradamusErr
	}
	f.notes = append(f.notes, note)
	return nil
}

func (f *fakeAdmins) AddNostradamusEdition(_ context.Context, e store.NostradamusEdition) error {
	return f.nostradamusChange(fmt.Sprintf("edition %s %q over %s %s", e.Slug, e.Name, e.Series, e.Tournament))
}

func (f *fakeAdmins) AddNostradamusRound(_ context.Context, slug string, d store.NostradamusRoundDraft) (int64, error) {
	return 3, f.nostradamusChange(fmt.Sprintf("round %s %s %q %d×%d", slug, d.Label, d.Name, d.Tokens, d.TokenPoints))
}

func (f *fakeAdmins) SetNostradamusRound(_ context.Context, slug string, round int64, d store.NostradamusRoundDraft) error {
	return f.nostradamusChange(fmt.Sprintf("set round %d %s", round, d.Label), round)
}

func (f *fakeAdmins) RemoveNostradamusRound(_ context.Context, slug string, round int64) error {
	return f.nostradamusChange(fmt.Sprintf("remove round %d", round), round)
}

func (f *fakeAdmins) AddNostradamusQuestion(_ context.Context, slug string, round int64, q store.NostradamusQuestion) (int64, error) {
	return 7, f.nostradamusChange(fmt.Sprintf("question in %d: %s %s ×%d %+v", round, q.Label, q.Kind, q.Picks, q.Choices), round)
}

func (f *fakeAdmins) SetNostradamusQuestion(_ context.Context, slug string, question int64, q store.NostradamusQuestion) error {
	return f.nostradamusChange(fmt.Sprintf("set question %d %s", question, q.Label), question)
}

func (f *fakeAdmins) RemoveNostradamusQuestion(_ context.Context, slug string, question int64) error {
	return f.nostradamusChange(fmt.Sprintf("remove question %d", question), question)
}

func (f *fakeAdmins) PublishNostradamusRound(_ context.Context, slug string, round int64, closes time.Time, zone string, _ time.Time) error {
	return f.nostradamusChange(fmt.Sprintf("publish %d until %s in %s", round, closes.UTC().Format(time.RFC3339), zone), round)
}

func (f *fakeAdmins) CloseNostradamusRound(_ context.Context, slug string, round int64, _ time.Time) error {
	return f.nostradamusChange(fmt.Sprintf("close %d", round), round)
}

func (f *fakeAdmins) SetNostradamusOutcome(_ context.Context, slug string, question int64, outcome []string) error {
	return f.nostradamusChange(fmt.Sprintf("outcome %d %q", question, outcome), question)
}

func (f *fakeAdmins) NostradamusRoundAnswers(_ context.Context, slug string, round int64) ([]store.NostradamusAnswer, error) {
	if round == 99 {
		return nil, store.ErrNoNostradamus
	}
	points := 14
	return []store.NostradamusAnswer{{Participant: "Bea", Question: 5, Answer: []string{"Ann"}, Token: true, Points: &points}}, nil
}

func (f *fakeAdmins) SetNostradamusBotAnswers(_ context.Context, slug string, round, participant int64, answers []store.NostradamusGiven, _ time.Time) error {
	return f.nostradamusChange(fmt.Sprintf("bot %d in %d: %d answers", participant, round, len(answers)), round, participant)
}

func (f *fakeAdmins) AddNostradamusParticipant(_ context.Context, slug string, e store.NostradamusEntrant) (int64, error) {
	return 10, f.nostradamusChange(fmt.Sprintf("add %s %q %q", e.Name, e.DiscordName, e.BotRule))
}

func (f *fakeAdmins) SetNostradamusStatus(_ context.Context, slug string, participant int64, status string) error {
	return f.nostradamusChange(fmt.Sprintf("status %d %s", participant, status), participant)
}

func (f *fakeAdmins) LinkNostradamusParticipant(_ context.Context, slug string, participant int64, member string) error {
	if member == "Zed" {
		return store.ErrNoMember
	}
	return f.nostradamusChange(fmt.Sprintf("link %d %q", participant, member), participant)
}

func TestAnOrganiserReadsANostradamusGame(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/nostradamus", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET games = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "editions", `[{"slug": "wc-2031"}]`)
	rec = adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/nostradamus/wc-2031", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET game = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "rounds.0.questions.0.choices", `[{"label": "Ann", "seed": 2}, {"label": "Cy", "seed": 0}]`)
	wantJSON(t, rec.Body.Bytes(), "participants", `[{"id": 8, "member": "Beatrix", "discord_name": "bea_d", "status": "accepted", "bot_rule": ""},
		{"id": 9, "bot": true, "bot_rule": "seed", "member": null}]`)
	rec = adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/nostradamus/wc-2031/rounds/1/answers", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET answers = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "answers", `[{"participant": "Bea", "question": 5, "answer": ["Ann"], "token": true, "points": 14}]`)
}

func TestAnOrganiserRunsANostradamusRoundAudited(t *testing.T) {
	f := &fakeAdmins{}
	steps := []struct{ method, path, body string }{
		{http.MethodPost, "/v1/admin/nostradamus", `{"slug": "wc-2032", "name": "Nostradamus #10", "series": "worlds", "tournament": "2032"}`},
		{http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds", `{"label": "S3", "name": " Stage 3 ", "tokens": 3, "token_points": 10}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/rounds/3", `{"label": "S3", "name": "Stage 3", "tokens": 2, "token_points": 10}`},
		{http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds/3/questions", `{"label": "Top", "prompt": "", "kind": "set", "points": 6, "near_points": 0,
			"picks": 2, "asks_last": false, "choices": [{"label": "Ann", "seed": 1, "leaderboard_rank": 0}, {"label": "Cy", "seed": 0, "leaderboard_rank": 4}]}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/questions/7", `{"label": "Top2", "prompt": "", "kind": "set", "points": 6, "near_points": 0,
			"picks": 2, "asks_last": false, "choices": []}`},
		{http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds/3/publish", `{"day": "2031-06-01", "time": "18:00", "zone": "Europe/Berlin"}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/rounds/3/bots/9/answers", `{"answers": [{"question": 7, "answer": ["Ann", "Cy"], "token": false}]}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/questions/7/outcome", `{"outcome": [" Cy ", "Ann"]}`},
		{http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds/3/close", ``},
		{http.MethodPost, "/v1/admin/nostradamus/wc-2031/participants", `{"name": "Dee", "discord_name": "dee_d"}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/participants/8/status", `{"status": "refused"}`},
		{http.MethodPut, "/v1/admin/nostradamus/wc-2031/participants/8/member", `{"member": " Beatrix "}`},
		{http.MethodDelete, "/v1/admin/nostradamus/wc-2031/questions/7", ``},
		{http.MethodDelete, "/v1/admin/nostradamus/wc-2031/rounds/3", ``},
	}
	for _, s := range steps {
		if rec := adminDo(t, f, s.method, s.path, challengeAdminSite, s.body); rec.Code >= 300 {
			t.Fatalf("%s %s = %d %s", s.method, s.path, rec.Code, rec.Body)
		}
	}
	want := []string{
		`edition wc-2032 "Nostradamus #10" over worlds 2032`,
		`round wc-2031 S3 " Stage 3 " 3×10`,
		`set round 3 S3`,
		`question in 3: Top set ×2 [{Label:Ann Seed:1 LeaderboardRank:0} {Label:Cy Seed:0 LeaderboardRank:4}]`,
		`set question 7 Top2`,
		`publish 3 until 2031-06-01T16:00:00Z in Europe/Berlin`,
		`bot 9 in 3: 1 answers`,
		`outcome 7 ["Cy" "Ann"]`,
		`close 3`,
		`add Dee "dee_d" ""`,
		`status 8 refused`,
		`link 8 "Beatrix"`,
		`remove question 7`,
		`remove round 3`,
	}
	if got := strings.Join(f.notes, "\n"); got != strings.Join(want, "\n") {
		t.Errorf("what reached the store:\n%s\nwant:\n%s", got, strings.Join(want, "\n"))
	}
	if len(f.audits) != len(steps) {
		t.Errorf("audits = %d %q, want one per change (%d)", len(f.audits), f.audits, len(steps))
	}
}

func TestNostradamusChangesAreRefusedWhenTheyShould(t *testing.T) {
	round, outcome := "/v1/admin/nostradamus/wc-2031/rounds/3", "/v1/admin/nostradamus/wc-2031/questions/7/outcome"
	tests := []struct {
		name, method, path, origin, body string
		err                              error
		want                             int
	}{
		{"a stage the round refuses", http.MethodPost, round + "/close", challengeAdminSite, ``, store.ErrNostradamusRoundState, http.StatusConflict},
		{"an outcome the rules refuse", http.MethodPut, outcome, challengeAdminSite, `{"outcome": ["x"]}`, store.ErrBadNostradamus, http.StatusBadRequest},
		{"an empty name in an outcome", http.MethodPut, outcome, challengeAdminSite, `{"outcome": ["Ann", "  "]}`, nil, http.StatusBadRequest},
		{"no such round", http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds/99/close", challengeAdminSite, ``, nil, http.StatusNotFound},
		{"a game over no tournament", http.MethodPost, "/v1/admin/nostradamus", challengeAdminSite,
			`{"slug": "wc-2099", "name": "N", "series": "worlds", "tournament": "2099"}`, store.ErrNoTournament, http.StatusNotFound},
		{"publishing in no zone", http.MethodPost, round + "/publish", challengeAdminSite, `{"day": "2031-06-01", "time": "18:00", "zone": "Mars/Base"}`, nil, http.StatusBadRequest},
		{"a member already in", http.MethodPost, "/v1/admin/nostradamus/wc-2031/participants", challengeAdminSite, `{"name": "Bea"}`, store.ErrBadNostradamus, http.StatusBadRequest},
		{"a banned member", http.MethodPost, "/v1/admin/nostradamus/wc-2031/participants", challengeAdminSite, `{"name": "Bea"}`, store.ErrMemberBanned, http.StatusBadRequest},
		{"linking no member", http.MethodPut, "/v1/admin/nostradamus/wc-2031/participants/8/member", challengeAdminSite, `{"member": "Zed"}`, nil, http.StatusNotFound},
		{"not from the admin site", http.MethodPost, round + "/close", "https://elsewhere.example", ``, nil, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{nostradamusErr: tt.err}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || len(f.notes) != 0 {
				t.Errorf("a refused change reached the store (%q) or was audited (%q)", f.notes, f.audits)
			}
		})
	}
}

func TestNostradamusAdminIsRefusedToOneWhoIsNotAnAdmin(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, "999999999999999999", http.MethodPost, "/v1/admin/nostradamus/wc-2031/rounds/3/close", challengeAdminSite, ``)
	if rec.Code != http.StatusForbidden {
		t.Errorf("POST as a stranger = %d, want 403", rec.Code)
	}
	if len(f.notes) != 0 {
		t.Errorf("a stranger's change reached the store: %q", f.notes)
	}
}
