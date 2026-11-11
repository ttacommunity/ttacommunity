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

// NostradamusBoard answers wc-2031: Bea linked to member Beatrix, and a bot who missed round S2.
func (f *fakeRatings) NostradamusBoard(_ context.Context, slug string) (store.NostradamusBoard, error) {
	if slug != "wc-2031" {
		return store.NostradamusBoard{}, store.ErrNoNostradamus
	}
	return store.NostradamusBoard{Slug: slug, Name: "Nostradamus #9", Rounds: []string{"S1", "S2"}, Standings: []store.NostradamusStanding{
		{Name: "Bea", Member: "Beatrix", Total: 20, Rounds: []store.NostradamusRoundScore{{Label: "S1", Points: 14}, {Label: "S2", Points: 6}}},
		{Name: "Seed", Bot: true, Total: 4, Rounds: []store.NostradamusRoundScore{{Label: "S1", Points: 4}, {Label: "S2", Missed: true}}},
	}}, nil
}

// NostradamusParticipant answers Bea in wc-2031, with one scored answer and one not, and notes the time it was asked at.
func (f *fakeRatings) NostradamusParticipant(_ context.Context, slug, name string, now time.Time) (store.NostradamusParticipant, error) {
	if slug != "wc-2031" || !strings.EqualFold(name, "bea") {
		return store.NostradamusParticipant{}, store.ErrNoNostradamus
	}
	f.histAt = now
	points := 14
	return store.NostradamusParticipant{Name: "Bea", Rounds: []store.NostradamusParticipantRound{{Label: "S1", Name: "Stage 1",
		ClosesAt: time.Date(2031, 4, 1, 12, 0, 0, 0, time.UTC), Points: &points, Answers: []store.NostradamusAnswer{
			{Question: store.NostradamusQuestion{Label: "A", Prompt: "Who wins?", Kind: "pick", Points: 4, Outcome: []string{"Ann"}},
				Answer: []string{"Ann"}, Token: true, Points: &points},
			{Question: store.NostradamusQuestion{Label: "B", Kind: "number", Points: 8, NearPoints: 4}, Answer: []string{"3"}},
		}}}}, nil
}

func TestNostradamusEditionsAndBoard(t *testing.T) {
	rec, _ := serve(t, &fakeRatings{}, "/v1/nostradamus")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET editions = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "editions", `[{"slug": "wc-2031", "name": "Nostradamus #9", "series": "worlds", "tournament": "2031"}]`)
	rec, _ = serve(t, &fakeRatings{}, "/v1/nostradamus/wc-2031")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET board = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"rounds": ["S1", "S2"], "standings": [
		{"name": "Bea", "member": "Beatrix", "bot": false, "total": 20, "rounds": [{"round": "S1", "points": 14}, {"round": "S2", "points": 6}]},
		{"name": "Seed", "member": null, "bot": true, "rounds": [{}, {"round": "S2", "points": 0, "missed": true}]}]}`)
	if got := rec.Header().Get("Cache-Control"); got != "public, max-age=60" {
		t.Errorf("Cache-Control = %q, want a minute", got)
	}
}

func TestNostradamusParticipantShowsAnswersAsOfTheRequest(t *testing.T) {
	f := &fakeRatings{}
	rec, _ := serve(t, f, "/v1/nostradamus/wc-2031/participants/bea")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "rounds.0", `{"round": "S1", "closes_at": "2031-04-01T12:00:00Z", "points": 14, "missed": false, "answers": [
		{"question": {"label": "A", "kind": "pick", "outcome": ["Ann"]}, "answer": ["Ann"], "token": true, "points": 14},
		{"question": {"label": "B", "near_points": 4, "outcome": null}, "answer": ["3"], "token": false, "points": null}]}`)
	if f.histAt.IsZero() {
		t.Error("the participant was read at no time, want the request's")
	}
}

func TestNostradamusUnknownsAreNotFound(t *testing.T) {
	for _, path := range []string{"/v1/nostradamus/wc-2099", "/v1/nostradamus/wc-2031/participants/zed", "/v1/nostradamus/wc-2099/participants/bea"} {
		if rec, _ := serve(t, &fakeRatings{}, path); rec.Code != http.StatusNotFound {
			t.Errorf("GET %s = %d, want 404", path, rec.Code)
		}
	}
}

func (f *fakeAdmins) NostradamusForAdmin(_ context.Context, slug string) (store.AdminNostradamus, error) {
	if slug != "wc-2031" {
		return store.AdminNostradamus{}, store.ErrNoNostradamus
	}
	return store.AdminNostradamus{Slug: slug, Name: "Nostradamus #9", Rounds: []store.AdminNostradamusRound{{Label: "S1", Name: "Stage 1",
		Questions: []store.NostradamusQuestion{{ID: 5, Label: "A", Kind: "set", Points: 6}}}},
		Participants: []store.AdminNostradamusParticipant{{ID: 8, Name: "Bea", Member: "Beatrix"}, {ID: 9, Name: "Seed", Bot: true}}}, nil
}

// SetNostradamusOutcome refuses question 99 as not in the edition, and an outcome of "bad" as not of its kind.
func (f *fakeAdmins) SetNostradamusOutcome(_ context.Context, slug string, question int64, outcome []string) error {
	switch {
	case question == 99:
		return store.ErrNoNostradamus
	case len(outcome) > 0 && outcome[0] == "bad":
		return store.ErrBadNostradamus
	}
	f.notes = append(f.notes, fmt.Sprintf("outcome %s %d %q", slug, question, outcome))
	return nil
}

// LinkNostradamusParticipant refuses member Zed as none, and participant 99 as not in the edition.
func (f *fakeAdmins) LinkNostradamusParticipant(_ context.Context, slug string, participant int64, member string) error {
	switch {
	case member == "Zed":
		return store.ErrNoMember
	case participant == 99:
		return store.ErrNoNostradamus
	}
	f.notes = append(f.notes, fmt.Sprintf("link %s %d %q", slug, participant, member))
	return nil
}

func TestAnOrganiserReadsANostradamusEdition(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/nostradamus", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET editions = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "editions", `[{"slug": "wc-2031", "name": "Nostradamus #9"}]`)
	rec = adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/nostradamus/wc-2031", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "", `{"rounds": [{"round": "S1", "questions": [{"id": 5, "question": {"label": "A", "kind": "set", "outcome": null}}]}],
		"participants": [{"id": 8, "name": "Bea", "member": "Beatrix"}, {"id": 9, "member": null, "bot": true}]}`)
}

func TestAnOrganiserSetsAnOutcomeAndLinksAParticipantAudited(t *testing.T) {
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPut, "/v1/admin/nostradamus/wc-2031/questions/5/outcome", challengeAdminSite,
		`{"outcome": [" Cy ", "Ann"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT outcome = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDo(t, f, http.MethodPut, "/v1/admin/nostradamus/wc-2031/participants/8/member", challengeAdminSite,
		`{"member": " Beatrix "}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PUT member = %d %s, want 204", rec.Code, rec.Body)
	}
	if got, want := strings.Join(f.notes, "|"), `outcome wc-2031 5 ["Cy" "Ann"]|link wc-2031 8 "Beatrix"`; got != want {
		t.Errorf("what reached the store = %q, want %q", got, want)
	}
	if got, want := strings.Join(f.audits, "|"), "222222222222222222 nostradamus.outcome.set 5|222222222222222222 nostradamus.member.link 8"; got != want {
		t.Errorf("audits = %q, want %q", got, want)
	}
}

func TestNostradamusChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	outcome, member := "/v1/admin/nostradamus/wc-2031/questions/5/outcome", "/v1/admin/nostradamus/wc-2031/participants/8/member"
	tests := []struct {
		name, path, origin, body string
		want                     int
	}{
		{"an outcome not of the kind", outcome, challengeAdminSite, `{"outcome": ["bad"]}`, http.StatusBadRequest},
		{"an empty name in an outcome", outcome, challengeAdminSite, `{"outcome": ["Ann", "  "]}`, http.StatusBadRequest},
		{"no outcome given", outcome, challengeAdminSite, `{}`, http.StatusBadRequest},
		{"a question not in the edition", "/v1/admin/nostradamus/wc-2031/questions/99/outcome", challengeAdminSite, `{"outcome": ["Ann"]}`, http.StatusNotFound},
		{"an outcome not from the admin site", outcome, "https://elsewhere.example", `{"outcome": ["Ann"]}`, http.StatusForbidden},
		{"no such member", member, challengeAdminSite, `{"member": "Zed"}`, http.StatusNotFound},
		{"a participant not in the edition", "/v1/admin/nostradamus/wc-2031/participants/99/member", challengeAdminSite, `{"member": "Beatrix"}`, http.StatusNotFound},
		{"a link not from the admin site", member, "https://elsewhere.example", `{"member": "Beatrix"}`, http.StatusForbidden},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPut, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PUT %s %s = %d %s, want %d", tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.audits) != 0 || len(f.notes) != 0 {
				t.Errorf("a refused change reached the store (%q) or was audited (%q)", f.notes, f.audits)
			}
		})
	}
}

func TestNostradamusAdminIsRefusedToOneWhoIsNotAnAdmin(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDoAs(t, f, "999999999999999999", http.MethodPut, "/v1/admin/nostradamus/wc-2031/questions/5/outcome", challengeAdminSite, `{"outcome": ["Ann"]}`)
	if rec.Code != http.StatusForbidden {
		t.Errorf("PUT as a stranger = %d, want 403", rec.Code)
	}
	if len(f.notes) != 0 {
		t.Errorf("a stranger's change reached the store: %q", f.notes)
	}
}
