package httpapi

import (
	"fmt"
	"net/http"
	"slices"
	"strings"
	"testing"
)

func TestAdminTournamentsListsEachInBrief(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/admin/tournaments = %d %s", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"slug": "worlds", "kind": "cup", "rated": true, "games": 3, "first_game": "2024-01-02", "cge_about": "CGE's."}`)
}

func TestAdminTournamentGivesItsSettingsSeasonsAndStages(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/admin/tournaments/worlds = %d %s", rec.Code, rec.Body)
	}
	body := rec.Body.Bytes()
	wantJSON(t, body, "", `{"slug": "worlds", "kind": "cup", "original_name": "World Championships", "top_divisions": []}`)
	wantJSON(t, body, "", `{"visible": true, "rated": true, "community": true, "blitz": true}`)
	wantJSON(t, body, "", `{"divisions": [{"name": "Final", "tier": 1}, {"name": "Semi-final", "tier": 2}], "super_gm": true, "record_min_games": 3}`)
	wantJSON(t, body, "seasons.0", `{"label": "2025", "original_name": "2025 Edition", "games": 3, "finished": 2, "players": 5}`)
	wantJSON(t, body, "seasons.0.stages.0", `{"label": "Stage 1", "original_name": "Groups", "note": "n", "number": 1, "games": 2, "finished": 1, "players": 5}`)
	if rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/nope", "", ""); rec.Code != http.StatusNotFound {
		t.Errorf("GET nope = %d, want 404", rec.Code)
	}
}

func TestAnOrganiserChangesATournamentAndItIsAudited(t *testing.T) {
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{"community": true}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || f.settings[0].Community == nil || !*f.settings[0].Community || f.settings[0].Visible != nil {
		t.Errorf("settings = %+v, want only community on", f.settings)
	}
	if len(f.audits) != 1 || f.audits[0] != "222222222222222222 tournament.settings worlds" {
		t.Errorf("audits = %q", f.audits)
	}
	rec = adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", "https://ttac-admin.example", `{"blitz": true}`)
	if rec.Code != http.StatusNoContent || len(f.settings) != 2 || f.settings[1].Blitz == nil || !*f.settings[1].Blitz {
		t.Errorf("PATCH blitz = %d %s, settings %+v; want 204 with blitz on", rec.Code, rec.Body, f.settings)
	}
	f.settings, f.audits = f.settings[:1], f.audits[:1]

	rec = adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", "https://ttac-admin.example",
		`{"stage_notes": {"Stage 1": "The groups."}}`)
	if rec.Code != http.StatusNoContent || len(f.notes) != 1 || f.notes[0] != "2025|Stage 1|The groups." || len(f.audits) != 2 {
		t.Errorf("PATCH stage note = %d, notes %q, audits %q", rec.Code, f.notes, f.audits)
	}
}

func TestAdminAddsAndRemovesASeasonWithoutGames(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, method, path, body string
		want                     int
		change                   string
	}{
		{"add", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": "2021"}`, http.StatusNoContent, "add season 2021"},
		{"add one that exists", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": "2025"}`, http.StatusBadRequest, ""},
		{"add an empty label", http.MethodPost, "/v1/admin/tournaments/worlds/seasons", `{"label": " "}`, http.StatusBadRequest, ""},
		{"add to no tournament", http.MethodPost, "/v1/admin/tournaments/nope/seasons", `{"label": "1"}`, http.StatusNotFound, ""},
		{"remove", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons/2023%20Play%20Off", "", http.StatusNoContent, "remove season 2023 Play Off"},
		{"remove one with games", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons/2025", "", http.StatusBadRequest, ""},
		{"remove one that does not exist", http.MethodDelete, "/v1/admin/tournaments/worlds/seasons/1999", "", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, tt.method, tt.path, site, tt.body)
			if rec.Code != tt.want {
				t.Fatalf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			changed := strings.Join(f.notes, "|")
			if changed != tt.change || (tt.change != "") != (len(f.audits) == 1) {
				t.Errorf("changes %q, audits %q; want %q, audited when changed", changed, f.audits, tt.change)
			}
		})
	}
}

func TestAnOrganiserSetsAndClearsASeasonsPodium(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", site,
		`{"podium": {"podium": [{"place": 1, "name": "alder440"}, {"place": 2, "name": "kittiwake"}], "note": "CGE final"}}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH podium = %d %s, want 204", rec.Code, rec.Body)
	}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", site, `{"clear_podium": true}`); rec.Code != http.StatusNoContent {
		t.Errorf("PATCH clear podium = %d %s, want 204", rec.Code, rec.Body)
	}
	if want := "podium 2025 [{1 alder440} {2 kittiwake}] CGE final|clear podium 2025"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	if want := org + " tournament.season worlds|" + org + " tournament.season worlds"; strings.Join(f.audits, "|") != want {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestPodiumChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, path, origin, body string
		want                     int
	}{
		{"another site", "/v1/admin/tournaments/worlds/seasons/2025", "https://evil.example", `{"podium": {"podium": [{"place": 1, "name": "a"}], "note": ""}}`, http.StatusForbidden},
		{"no such season", "/v1/admin/tournaments/worlds/seasons/1999", site, `{"podium": {"podium": [{"place": 1, "name": "a"}], "note": ""}}`, http.StatusNotFound},
		{"not a podium", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [{"place": 1, "name": "twice"}], "note": ""}}`, http.StatusBadRequest},
		{"no such player", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [{"place": 1, "name": "ghost"}], "note": ""}}`, http.StatusBadRequest},
		{"nobody on it", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [], "note": ""}}`, http.StatusBadRequest},
		{"four places", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [{"place": 1, "name": "a"}, {"place": 2, "name": "b"}, {"place": 3, "name": "c"}, {"place": 4, "name": "d"}], "note": ""}}`, http.StatusBadRequest},
		{"a long note", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [{"place": 1, "name": "a"}], "note": "` + strings.Repeat("x", 501) + `"}}`, http.StatusBadRequest},
		{"set and cleared at once", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"podium": {"podium": [{"place": 1, "name": "a"}], "note": ""}, "clear_podium": true}`, http.StatusBadRequest},
		{"clearing no such season", "/v1/admin/tournaments/worlds/seasons/1999", site, `{"clear_podium": true}`, http.StatusNotFound},
		{"a long organiser", "/v1/admin/tournaments/worlds/seasons/2025", site, `{"organiser": "` + strings.Repeat("x", 101) + `"}`, http.StatusBadRequest},
		{"an organiser for no such season", "/v1/admin/tournaments/worlds/seasons/1999", site, `{"organiser": "Ember_Wolf"}`, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDo(t, f, http.MethodPatch, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("PATCH %s %s = %d %s, want %d", tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAdminTournamentShowsASeasonsPodiumAndASuggestedOne(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	wantJSON(t, rec.Body.Bytes(), "seasons.0.podium", `[{"place":1,"name":"alder440"}]`)
	wantJSON(t, rec.Body.Bytes(), "seasons.0.podium_note", `"CGE"`)
	wantJSON(t, rec.Body.Bytes(), "seasons.0.suggested_podium", `[{"place":1,"name":"birch550"}]`)
	wantJSON(t, rec.Body.Bytes(), "seasons.0.suggested_from", `"final_stage"`)
}

func TestAnOrganiserWritesATournamentsCardRegulationsAndStories(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name, body, change string
	}{
		{"a card", `{"card": {"icon": "🏆", "tagline": "Annual", "summary": "The pinnacle.", "subtitle": "4-Player", "cadence": "Yearly"}}`,
			"card 🏆|Annual|The pinnacle.|4-Player|Yearly"},
		{"regulations", `{"regulations": "Best of three."}`, "regulations Best of three."},
		{"stories, in order", `{"stories": [{"player": "alder440", "badge": "", "title": "2025", "text": "Won."}, {"player": "pipit4", "badge": "", "title": "2024", "text": "Second."}]}`,
			"story alder440|2025,story pipit4|2024"},
		{"all three", `{"card": {"icon": "x", "tagline": "", "summary": ""}, "regulations": "", "stories": []}`, "card x||||,regulations "},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, tt.body); rec.Code != http.StatusNoContent {
				t.Fatalf("PATCH %s = %d %s, want 204", tt.body, rec.Code, rec.Body)
			}
			if strings.Join(f.notes, ",") != tt.change || strings.Join(f.audits, "|") != org+" tournament.settings worlds" {
				t.Errorf("changes %q, audits %q; want %q, audited once", f.notes, f.audits, tt.change)
			}
		})
	}
}

func TestCardRegulationsAndStoriesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site = "https://ttac-admin.example"
	tests := []struct {
		name, path, origin, body string
		want                     int
	}{
		{"a card from another site", "/v1/admin/tournaments/worlds", "https://evil.example", `{"card": {"icon": "", "tagline": "", "summary": ""}}`, http.StatusForbidden},
		{"a card for no tournament", "/v1/admin/tournaments/nope", site, `{"card": {"icon": "", "tagline": "", "summary": ""}}`, http.StatusNotFound},
		{"a card too long for the store", "/v1/admin/tournaments/worlds", site, `{"card": {"icon": "toolong", "tagline": "", "summary": ""}}`, http.StatusBadRequest},
		{"regulations for no tournament", "/v1/admin/tournaments/nope", site, `{"regulations": "x"}`, http.StatusNotFound},
		{"regulations too long", "/v1/admin/tournaments/worlds", site, `{"regulations": "` + strings.Repeat("x", 10001) + `"}`, http.StatusBadRequest},
		{"stories for no tournament", "/v1/admin/tournaments/nope", site, `{"stories": []}`, http.StatusNotFound},
		{"a story without a title", "/v1/admin/tournaments/worlds", site, `{"stories": [{"player": "p", "badge": "", "title": "", "text": "x"}]}`, http.StatusBadRequest},
		{"thirteen stories", "/v1/admin/tournaments/worlds", site, `{"stories": [` + strings.TrimSuffix(strings.Repeat(`{"player": "p", "badge": "", "title": "t", "text": "x"},`, 13), ",") + `]}`, http.StatusBadRequest},
		{"regulations after a setting", "/v1/admin/tournaments/worlds", site, `{"visible": true, "regulations": "` + strings.Repeat("x", 10001) + `"}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPatch, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("PATCH %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
}

func TestAdminTournamentGivesItsCardRegulationsAndStories(t *testing.T) {
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	body := rec.Body.Bytes()
	wantJSON(t, body, "card", `{"tagline": "Annual World Championship"}`)
	wantJSON(t, body, "", `{"regulations": "Mine.", "cge_regulations": "CGE's.", "format_summary": "Knockout."}`)
	wantJSON(t, body, "stories.0", `{"player": "alder440"}`)
}

func TestAnOrganiserSetsWhatOfATournamentIsRated(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"rated_player_counts": [2, 4]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH player counts = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || !slices.Equal(f.settings[0].RatedPlayerCounts, []int{2, 4}) {
		t.Errorf("settings = %+v, want player counts 2 and 4", f.settings)
	}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"kind": "league", "top_divisions": ["Final"]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH kind = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 2 || f.settings[1].Kind != "league" || !slices.Equal(f.settings[1].TopDivisions, []string{"Final"}) {
		t.Errorf("settings = %+v, want a league with a Final", f.settings)
	}
	f.settings, f.audits = f.settings[:1], f.audits[:1]
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2023", site, `{"rated": false}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH season rated = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "season 2023 rated false" || strings.Join(f.audits, "|") != org+" tournament.settings worlds|"+org+" tournament.season worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"a 5-player count", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"rated_player_counts": [2, 5]}`, http.StatusBadRequest},
		{"an unknown kind", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"kind": "tour"}`, http.StatusBadRequest},
		{"a season that does not exist", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/1999", site, `{"rated": false}`, http.StatusNotFound},
		{"a season of no tournament", http.MethodPatch, "/v1/admin/tournaments/nope/seasons/2023", site, `{"rated": false}`, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	wantJSON(t, rec.Body.Bytes(), "rated_player_counts", `[3, 4]`)
	wantJSON(t, rec.Body.Bytes(), "seasons.0.rated", `true`)
}

func TestOnlyAnAdminSetsALeaguesRules(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	f := &fakeAdmins{}
	for _, body := range []string{`{"divisions": [{"name": "Gold", "tier": 1}]}`, `{"super_gm": true}`, `{"record_min_games": 4}`,
		`{"visible": true, "super_gm": false}`} {
		if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, body); rec.Code != http.StatusForbidden {
			t.Errorf("an organiser's PATCH %s = %d %s, want 403", body, rec.Code, rec.Body)
		}
	}
	if len(f.settings) != 0 || len(f.audits) != 0 {
		t.Fatalf("after organisers' refused changes: settings %+v, audits %q; want none", f.settings, f.audits)
	}

	rec := adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/tournaments/worlds", site,
		`{"divisions": [{"name": "Gold", "tier": 1}, {"name": "Silver", "tier": 2}], "super_gm": true, "record_min_games": 4}`)
	if rec.Code != http.StatusNoContent {
		t.Fatalf("an admin's PATCH = %d %s, want 204", rec.Code, rec.Body)
	}
	got := f.settings[0]
	if fmt.Sprint(got.Divisions) != "[{Gold 1} {Silver 2}]" || got.SuperGM == nil || !*got.SuperGM || got.RecordMinGames == nil || *got.RecordMinGames != 4 {
		t.Errorf("settings = %+v, want Gold 1, Silver 2, Super GM, 4 games", got)
	}
	if strings.Join(f.audits, "|") != owner+" tournament.settings worlds" {
		t.Errorf("audits = %q, want the admin's change", f.audits)
	}
	if rec := adminDoAs(t, f, owner, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"divisions": [{"name": "Gold", "tier": 8}]}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a tier of 8 = %d %s, want 400", rec.Code, rec.Body)
	}
}

func TestAnOrganiserSetsScoringAndTimeoutPenalties(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"points_3p": [5, 2, 0], "points_4p": [6, 3, 1, 0]}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH scoring = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || !slices.Equal(f.settings[0].Points[3], []float64{5, 2, 0}) || !slices.Equal(f.settings[0].Points[4], []float64{6, 3, 1, 0}) {
		t.Errorf("settings = %+v, want 3p 5-2-0 and 4p 6-3-1-0", f.settings)
	}
	if _, set := f.settings[0].Points[2]; set {
		t.Errorf("settings = %+v, want 2p left as it is", f.settings)
	}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", site, `{"timeout_penalty": -1}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH season penalty = %d %s, want 204", rec.Code, rec.Body)
	}
	if strings.Join(f.notes, "|") != "season 2025 penalty -1" || strings.Join(f.audits, "|") != org+" tournament.settings worlds|"+org+" tournament.season worlds" {
		t.Errorf("changes %q, audits %q", f.notes, f.audits)
	}
	for _, tt := range []struct {
		name, method, path, origin, body string
		want                             int
	}{
		{"points not one per place", http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"points_3p": [5, 2]}`, http.StatusBadRequest},
		{"a positive penalty", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2025", site, `{"timeout_penalty": 1}`, http.StatusBadRequest},
		{"a season that does not exist", http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/1999", site, `{"timeout_penalty": -1}`, http.StatusNotFound},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.settings)+len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %+v %q %q", f.settings, f.notes, f.audits)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	wantJSON(t, rec.Body.Bytes(), "", `{"points_2p": [], "points_3p": [5, 2, 0], "points_4p": [6, 3, 1, 0]}`)
	wantJSON(t, rec.Body.Bytes(), "seasons.0.timeout_penalty", `-1`)
}

func TestAnOrganiserWritesATournamentsFormatInBrief(t *testing.T) {
	const site = "https://ttac-admin.example"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"format_summary": "Knockout duels."}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH format_summary = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || f.settings[0].FormatSummary == nil || *f.settings[0].FormatSummary != "Knockout duels." {
		t.Errorf("settings = %+v, want the format in brief", f.settings)
	}
	f = &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"format_summary": "`+strings.Repeat("é", 301)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a 301-character format in brief = %d %s, want 400", rec.Code, rec.Body)
	}
	if len(f.settings)+len(f.audits) != 0 {
		t.Errorf("a refused format in brief still changed something: %+v %q", f.settings, f.audits)
	}
}

func TestAnOrganiserNamesATournamentsSeasonsAndStages(t *testing.T) {
	const site = "https://ttac-admin.example"
	f := &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"season_name": "Edition", "stage_name": ""}`); rec.Code != http.StatusNoContent {
		t.Fatalf("PATCH names = %d %s, want 204", rec.Code, rec.Body)
	}
	if len(f.settings) != 1 || f.settings[0].SeasonName == nil || *f.settings[0].SeasonName != "Edition" ||
		f.settings[0].StageName == nil || *f.settings[0].StageName != "" {
		t.Errorf("settings = %+v, want season Edition and the stage's default", f.settings)
	}
	f = &fakeAdmins{}
	if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds", site, `{"season_name": "`+strings.Repeat("x", 33)+`"}`); rec.Code != http.StatusBadRequest {
		t.Errorf("a 33-character name = %d %s, want 400", rec.Code, rec.Body)
	}
	if len(f.settings)+len(f.audits) != 0 {
		t.Errorf("a refused name still changed something: %+v %q", f.settings, f.audits)
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	wantJSON(t, rec.Body.Bytes(), "", `{"season_name": "", "stage_name": "Round"}`)
}

func TestAnOrganiserChangesASeasonAndItIsAuditedOnce(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	tests := []struct {
		name, body, change string
	}{
		{"out of the community leaderboard", `{"in_community": false}`, "season 2023 Play Off community false"},
		{"out of the champions", `{"in_champions": false}`, "season 2023 Play Off champions false"},
		{"run on this site", `{"run_here": true}`, "season 2023 Play Off run here true"},
		{"its note", `{"note": "Replayed."}`, "2023 Play Off||Replayed."},
		{"its organiser, trimmed", `{"organiser": " Ember_Wolf "}`, "season 2023 Play Off organiser Ember_Wolf"},
		{"its organiser removed", `{"organiser": ""}`, "season 2023 Play Off organiser "},
		{"several at once, the podium first", `{"rated": true, "in_champions": true, "podium": {"podium": [{"place": 1, "name": "a"}], "note": ""}}`,
			"podium 2023 Play Off [{1 a}] |season 2023 Play Off rated true|season 2023 Play Off champions true"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDo(t, f, http.MethodPatch, "/v1/admin/tournaments/worlds/seasons/2023%20Play%20Off", site, tt.body); rec.Code != http.StatusNoContent {
				t.Fatalf("PATCH %s = %d %s, want 204", tt.body, rec.Code, rec.Body)
			}
			if strings.Join(f.notes, "|") != tt.change || strings.Join(f.audits, "|") != org+" tournament.season worlds" {
				t.Errorf("changes %q, audits %q; want %q, audited once", f.notes, f.audits, tt.change)
			}
		})
	}
	rec := adminDo(t, &fakeAdmins{}, http.MethodGet, "/v1/admin/tournaments/worlds", "", "")
	wantJSON(t, rec.Body.Bytes(), "seasons.0", `{"in_community": true, "in_champions": true, "run_here": true,
		"organiser": "Ember_Wolf", "cge_organiser": "Lantern_Keeper"}`)
}
