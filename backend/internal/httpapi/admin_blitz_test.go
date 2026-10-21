package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"regexp"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/store"
)

// The fake's one blitz edition is us_open 2026, with Ann entered (id 7).

func (f *fakeAdmins) blitzEntrants() []store.BlitzEntrant {
	if f.entrants == nil {
		f.entrants = []store.BlitzEntrant{{ID: 7, Name: "Ann", Player: "Ann", DiscordName: "ann#1", Zone: "Europe/Berlin", Status: "active"}}
	}
	return f.entrants
}

func (f *fakeAdmins) blitzRow() store.BlitzEditionRow {
	row := store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat, Points: blitz.FIDEScoring}
	if f.format != (store.BlitzFormat{}) {
		row.Format = f.format
	}
	if f.points != (blitz.Scoring{}) {
		row.Points = f.points
	}
	for _, r := range f.rounds {
		if r.PublishedAt != nil && (row.Round == nil || r.Number > *row.Round) {
			row.Round = new(r.Number)
		}
		for _, m := range r.Matches {
			if m.Report == "flagged" {
				row.Flagged++
			}
		}
	}
	for _, e := range f.blitzEntrants() {
		switch e.Status {
		case "dropped":
			row.Dropped++
		default:
			row.Entrants++
		}
	}
	for _, r := range f.rounds {
		if w, ok := finalWinner(r); ok {
			for _, e := range f.blitzEntrants() {
				if e.ID == w {
					row.Champion = new(e.Name)
				}
			}
		}
	}
	return row
}

// finalWinner is the winner of a published knockout round of one match, the final.
func finalWinner(r store.BlitzRound) (int64, bool) {
	if r.Kind != "knockout" || r.PublishedAt == nil || len(r.Matches) != 1 || r.Matches[0].B == nil {
		return 0, false
	}
	switch m := r.Matches[0]; m.Result {
	case "a":
		return m.A, true
	case "b":
		return *m.B, true
	}
	return 0, false
}

func (f *fakeAdmins) BlitzEditions(context.Context) ([]store.BlitzEditionRow, error) {
	return []store.BlitzEditionRow{f.blitzRow()}, nil
}

func (f *fakeAdmins) BlitzEdition(_ context.Context, slug, edition string) (store.BlitzEdition, error) {
	if slug != "us_open" || edition != "2026" {
		return store.BlitzEdition{}, store.ErrNoBlitzEdition
	}
	return store.BlitzEdition{BlitzEditionRow: f.blitzRow(), Entrants: f.blitzEntrants(), Rounds: slices.Clone(f.rounds), Organisers: slices.Clone(f.organisers), ThreadText: f.threadText}, nil
}

func (f *fakeAdmins) SetBlitzFormat(_ context.Context, slug, edition string, format store.BlitzFormat) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	if format.KnockoutSize == 3 {
		return store.ErrBadBlitzFormat
	}
	if slices.ContainsFunc(f.rounds, func(r store.BlitzRound) bool { return r.Kind == "knockout" }) {
		return store.ErrBlitzFormatLocked
	}
	f.format = format
	f.notes = append(f.notes, fmt.Sprintf("format %d/%d", format.SwissRounds, format.KnockoutSize))
	return nil
}

func (f *fakeAdmins) SetBlitzPoints(_ context.Context, slug, edition string, points blitz.Scoring) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	if slices.ContainsFunc(f.rounds, func(r store.BlitzRound) bool { return r.Kind == "knockout" }) {
		return store.ErrBlitzPointsLocked
	}
	f.points = points
	f.notes = append(f.notes, fmt.Sprintf("points %g/%g/%g/%g/%g/%g/%g", points.Win, points.Draw, points.Loss, points.ForfeitWin, points.ForfeitLoss, points.Bye, points.Skip))
	return nil
}

func (f *fakeAdmins) AddBlitzEntrants(_ context.Context, slug, edition string, entrants []store.BlitzEntrant) ([]int64, error) {
	if slug != "us_open" || edition != "2026" {
		return nil, store.ErrNoBlitzEdition
	}
	f.added = append(f.added, entrants...)
	var ids, names = []int64{}, []string{}
	for _, e := range entrants {
		if e.MemberID > 0 {
			m, err := f.findMember(e.MemberID)
			if err != nil {
				return nil, err
			}
			if m.Banned {
				return nil, store.ErrMemberBanned
			}
			e.Name, e.Zone, e.Country = m.Name, m.Zone, m.Country
		}
		if strings.EqualFold(e.Name, "Ann") {
			return nil, &store.BlitzEntrantExistsError{Names: []string{"Ann"}}
		}
		if e.Name == "" {
			return nil, store.ErrBadBlitzEntrant
		}
		e.ID = int64(len(f.blitzEntrants()) + 7)
		if e.MemberID == 0 {
			e.MemberID = 1000 + e.ID
		}
		e.Status = "active"
		f.entrants = append(f.entrants, e)
		ids, names = append(ids, e.ID), append(names, e.Name)
	}
	f.notes = append(f.notes, "add entrants "+strings.Join(names, ","))
	return ids, nil
}

func (f *fakeAdmins) SetBlitzEntrant(_ context.Context, slug, edition string, id int64, c store.BlitzEntrantChange) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	if c.Status != nil && !slices.Contains([]string{"active", "dropped"}, *c.Status) {
		return store.ErrBadBlitzStatus
	}
	for i := range f.blitzEntrants() {
		if f.entrants[i].ID != id {
			continue
		}
		if c.Status != nil && *c.Status == "dropped" && !f.played(id) {
			return store.ErrBlitzEntrantNotPlayed
		}
		if c.Status != nil && *c.Status == "dropped" && slices.Contains(f.entrants[i].Skips, f.nextFakeRound()) {
			return store.ErrBlitzEntrantSkipping
		}
		var changed []string
		if c.Status != nil {
			f.entrants[i].Status = *c.Status
			changed = append(changed, "status="+*c.Status)
		}
		if c.Zone != nil {
			f.entrants[i].Zone = *c.Zone
			changed = append(changed, "zone="+*c.Zone)
		}
		if c.DiscordName != nil {
			f.entrants[i].DiscordName = *c.DiscordName
			changed = append(changed, "discord="+*c.DiscordName)
		}
		if c.Email != nil {
			f.entrants[i].Email = *c.Email
			changed = append(changed, "email="+*c.Email)
		}
		if c.Country != nil {
			f.entrants[i].Country = strings.ToUpper(*c.Country)
			changed = append(changed, "country="+*c.Country)
		}
		f.notes = append(f.notes, fmt.Sprintf("set entrant %d %s", id, strings.Join(changed, " ")))
		return nil
	}
	return store.ErrNoBlitzEntrant
}

// played says whether an entrant has a match with a result, or a bye in a published round.
func (f *fakeAdmins) played(id int64) bool {
	for _, r := range f.rounds {
		for _, m := range r.Matches {
			if (m.A == id || (m.B != nil && *m.B == id)) && (m.Result != "" || (m.B == nil && r.PublishedAt != nil)) {
				return true
			}
		}
	}
	return false
}

func (f *fakeAdmins) RemoveBlitzEntrant(_ context.Context, slug, edition string, id int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	if f.played(id) {
		return store.ErrBlitzEntrantPlayed
	}
	for ri, r := range f.rounds {
		var kept []store.BlitzMatch
		for _, m := range r.Matches {
			named := m.A == id || (m.B != nil && *m.B == id)
			if named && r.Kind == "knockout" {
				return store.ErrBlitzEntrantInKnockout
			}
			if !named {
				kept = append(kept, m)
			}
		}
		f.rounds[ri].Matches = kept
	}
	for i, e := range f.blitzEntrants() {
		if e.ID == id {
			f.entrants = append(f.entrants[:i], f.entrants[i+1:]...)
			f.notes = append(f.notes, fmt.Sprintf("remove entrant %d", id))
			return nil
		}
	}
	return store.ErrNoBlitzEntrant
}

func (f *fakeAdmins) BlitzCandidates(_ context.Context, slug, edition, q string, limit int) ([]store.Candidate, error) {
	if slug != "us_open" || edition != "2026" {
		return nil, store.ErrNoBlitzEdition
	}
	f.searched = fmt.Sprintf("%s %s %q %d", slug, edition, q, limit)
	return f.candidates, nil
}

func TestAnOrganiserFindsWhomToEnter(t *testing.T) {
	const org = "222222222222222222"
	f := &fakeAdmins{candidates: []store.Candidate{
		{Name: "Ann", Player: "Ann", Country: "GB"},
		{MemberID: 101, Name: "anna", DiscordName: "anna#1", Zone: "US East", Banned: true},
	}}
	rec := adminDoAs(t, f, org, http.MethodGet, "/v1/admin/blitz/us_open/2026/candidates?q=%20an%20", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET candidates = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"member": null, "name": "Ann", "player": "Ann", "country": "GB", "zone": null, "banned": false}`)
	wantJSON(t, rec.Body.Bytes(), "1", `{"member": 101, "name": "anna", "discord_name": "anna#1", "country": null, "zone": "US East", "banned": true}`)
	if want := `us_open 2026 "an" 5`; f.searched != want {
		t.Errorf("searched = %s, want %s", f.searched, want)
	}
}

func (f *fakeAdmins) People(_ context.Context, q string, limit int) ([]store.Candidate, error) {
	f.searched = fmt.Sprintf("%q %d", q, limit)
	return f.candidates, nil
}

func TestAnOrganiserFindsPeople(t *testing.T) {
	f := &fakeAdmins{candidates: []store.Candidate{{MemberID: 101, Name: "anna", Zone: "Unknown"}}}
	rec := adminDoAs(t, f, "222222222222222222", http.MethodGet, "/v1/admin/people?q=%20an%20", "", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("GET people = %d %s, want 200", rec.Code, rec.Body)
	}
	wantJSON(t, rec.Body.Bytes(), "0", `{"member": 101, "name": "anna", "zone": "Unknown"}`)
	if want := `"an" 5`; f.searched != want {
		t.Errorf("searched = %s, want %s", f.searched, want)
	}
	for _, q := range []string{"?q=a", "", "?q=" + strings.Repeat("x", 101)} {
		if rec := adminDoAs(t, f, "222222222222222222", http.MethodGet, "/v1/admin/people"+q, "", ""); rec.Code != http.StatusBadRequest {
			t.Errorf("GET people%s = %d, want 400", q, rec.Code)
		}
	}
}

func TestAdminListsTheRegionsAfterUnknown(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "222222222222222222", http.MethodGet, "/v1/admin/regions", "", "")
	var got []Region
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK || len(got) != len(blitz.Regions)+1 {
		t.Fatalf("GET regions = %d %s, want Unknown and %d regions", rec.Code, rec.Body, len(blitz.Regions))
	}
	if want := (Region{Name: blitz.Unknown}); got[0] != want {
		t.Errorf("first = %+v, want %+v", got[0], want)
	}
	first := blitz.Regions[0]
	if got[1].Name != first.Name || got[1].Zone != first.Zone || !regexp.MustCompile(`^[+-]\d\d:\d\d$`).MatchString(got[1].UtcOffset) {
		t.Errorf("second = %+v, want %s keeping %s's time, with its offset", got[1], first.Name, first.Zone)
	}
}

func TestAdminListsTheCountriesAMemberMayHave(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "222222222222222222", http.MethodGet, "/v1/admin/countries", "", "")
	var got []string
	if err := json.Unmarshal(rec.Body.Bytes(), &got); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET countries = %d %s, want 200", rec.Code, rec.Body)
	}
	if diff := cmp.Diff(store.CountryCodes(), got); diff != "" {
		t.Errorf("countries (-want +got):\n%s", diff)
	}
}

func TestEnteringAMemberPassesOnOnlyTheDetailsGiven(t *testing.T) {
	f := &fakeAdmins{members: []store.Member{{ID: 101, Name: "bob", Zone: "UTC"}}}
	body := `{"entrants": [{"member": 101, "email": "bob@example.test"}, {"name": "dee"}]}`
	if rec := adminDoAs(t, f, "222222222222222222", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", "https://ttac-admin.example", body); rec.Code != http.StatusCreated {
		t.Fatalf("POST entrants = %d %s, want 201", rec.Code, rec.Body)
	}
	want := []store.BlitzEntrant{{MemberID: 101, Email: "bob@example.test"}, {Name: "dee", Zone: blitz.Unknown}}
	if diff := cmp.Diff(want, f.added); diff != "" {
		t.Errorf("entrants given to the store, a member's zone left alone and a new one's Unknown (-want +got):\n%s", diff)
	}
}

func TestAnOrganiserFillsABlitzEdition(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700, RD: 60}}}
	do := func(method, path, origin, body string) *httptest.ResponseRecorder {
		return adminDoWith(t, ratings, f, org, method, path, origin, body)
	}

	rec := do(http.MethodGet, "/v1/admin/blitz", "", "")
	var list []AdminBlitzEditionRow
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || len(list) != 1 || list[0].Tournament != "us_open" || list[0].Entrants != 1 {
		t.Fatalf("GET blitz = %d %s, want us_open 2026 with one entrant", rec.Code, rec.Body)
	}

	rec = do(http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": [{"name": "bob", "discord_name": "bobby", "zone": " America/New_York "}, {"name": "cy", "zone": "UTC"}]}`)
	var ids BlitzEntrantIds
	if err := json.Unmarshal(rec.Body.Bytes(), &ids); err != nil || rec.Code != http.StatusCreated || len(ids.Ids) != 2 || ids.Ids[0] != 8 {
		t.Fatalf("POST entrants = %d %s, want 201 with ids 8 and 9", rec.Code, rec.Body)
	}

	rec = do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "", "")
	var ed AdminBlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET edition = %d %s", rec.Code, rec.Body)
	}
	ann := 1700.0
	wantEntrants := []AdminBlitzEntrant{
		{Id: 7, Member: 0, Name: "Ann", Player: "Ann", DiscordName: "ann#1", Zone: "Europe/Berlin", UtcOffset: "+02:00", Rating: &ann, Status: "active", Skips: []int{}},
		{Id: 8, Member: 1008, Name: "bob", Player: "", DiscordName: "bobby", Zone: "America/New_York", UtcOffset: "-04:00", Rating: nil, Status: "active", Skips: []int{}},
		{Id: 9, Member: 1009, Name: "cy", Player: "", DiscordName: "", Zone: "UTC", UtcOffset: "+00:00", Rating: nil, Status: "active", Skips: []int{}},
	}
	if diff := cmp.Diff(wantEntrants, ed.Entrants); diff != "" {
		t.Errorf("entrants (-want +got):\n%s", diff)
	}
	if ed.Format.SwissRounds != 8 || ed.Format.KnockoutSize != 8 {
		t.Errorf("format = %+v, want the default 8/8", ed.Format)
	}

	for _, step := range []struct{ method, path, body string }{
		{http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/8", `{"zone": "UTC"}`},
		{http.MethodPatch, "/v1/admin/blitz/us_open/2026", `{"swiss_rounds": 6, "knockout_size": 4}`},
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/8", ""},
	} {
		if rec := do(step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if want := "add entrants bob,cy|set entrant 8 zone=UTC|format 6/4|remove entrant 8"; strings.Join(f.notes, "|") != want {
		t.Errorf("changes = %q, want %q", f.notes, want)
	}
	want := []string{"blitz.entrants.add us_open/2026", "blitz.entrant.set 8", "blitz.format us_open/2026", "blitz.entrant.remove 8"}
	for i, w := range want {
		if i >= len(f.audits) || f.audits[i] != org+" "+w {
			t.Errorf("audit %d = %q, want %q", i, f.audits, org+" "+w)
		}
	}
}

func TestBlitzChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name, method, path, origin, body string
		want                             int
		wantSaid                         string
	}{
		{"a bad slug", http.MethodGet, "/v1/admin/blitz/Bad!/2026", "", "", http.StatusBadRequest, ""},
		{"an unknown edition", http.MethodGet, "/v1/admin/blitz/us_open/2027", "", "", http.StatusNotFound, ""},
		{"another site", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", "https://evil.example", `{"entrants": [{"name": "x", "zone": "UTC"}]}`, http.StatusForbidden, ""},
		{"no entrants", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": []}`, http.StatusBadRequest, ""},
		{"a zone that is not an IANA name", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": [{"name": "x", "zone": "UTC"}, {"name": "y", "zone": "EST"}]}`, http.StatusBadRequest, "row 2"},
		{"a name already entered", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": [{"name": "ann", "zone": "UTC"}]}`, http.StatusConflict, "Ann"},
		{"entrants for an unknown edition", http.MethodPost, "/v1/admin/blitz/us_open/2027/entrants", site, `{"entrants": [{"name": "x", "zone": "UTC"}]}`, http.StatusNotFound, ""},
		{"an empty name", http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": [{"name": "", "zone": "UTC"}]}`, http.StatusBadRequest, ""},
		{"nothing to change", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{}`, http.StatusBadRequest, ""},
		{"a status that is not active or dropped", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{"status": "gone"}`, http.StatusBadRequest, ""},
		{"pending, no longer a status", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{"status": "pending"}`, http.StatusBadRequest, ""},
		{"a zone change to a non-zone", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{"zone": "GMT+2"}`, http.StatusBadRequest, "GMT+2"},
		{"dropping an entrant who has not played", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{"status": "dropped"}`, http.StatusConflict, "remove them"},
		{"changing an unknown entrant", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/99", site, `{"status": "dropped"}`, http.StatusNotFound, ""},
		{"removing an unknown entrant", http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/99", site, "", http.StatusNotFound, ""},
		{"a knockout of three", http.MethodPatch, "/v1/admin/blitz/us_open/2026", site, `{"swiss_rounds": 8, "knockout_size": 3}`, http.StatusBadRequest, ""},
		{"a format for an unknown edition", http.MethodPatch, "/v1/admin/blitz/us_open/2027", site, `{"swiss_rounds": 8, "knockout_size": 8}`, http.StatusNotFound, ""},
		{"a search of one letter", http.MethodGet, "/v1/admin/blitz/us_open/2026/candidates?q=%20a%20", "", "", http.StatusBadRequest, ""},
		{"a search of nothing", http.MethodGet, "/v1/admin/blitz/us_open/2026/candidates", "", "", http.StatusBadRequest, ""},
		{"a search too long", http.MethodGet, "/v1/admin/blitz/us_open/2026/candidates?q=" + strings.Repeat("x", 101), "", "", http.StatusBadRequest, ""},
		{"candidates for an unknown edition", http.MethodGet, "/v1/admin/blitz/us_open/2027/candidates?q=ann", "", "", http.StatusNotFound, ""},
		{"candidates for a bad slug", http.MethodGet, "/v1/admin/blitz/Bad!/2026/candidates?q=ann", "", "", http.StatusBadRequest, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			rec := adminDoAs(t, f, owner, tt.method, tt.path, tt.origin, tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if tt.wantSaid != "" && !strings.Contains(rec.Body.String(), tt.wantSaid) {
				t.Errorf("%s %s said %s, want it to name %q", tt.method, tt.path, rec.Body, tt.wantSaid)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("a refused change still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}

func TestAnOrganiserSetsTheEditionsPointsThenResetsThem(t *testing.T) {
	const site, org = "https://ttac-admin.example", "222222222222222222"
	f := &fakeAdmins{}
	roundOnePublished(f)
	at := time.Date(2026, 10, 15, 9, 0, 0, 0, time.UTC)
	f.rounds[0].Matches[1].Result, f.rounds[0].Matches[1].DecidedAt, f.rounds[0].Matches[1].GameID = "a", &at, new(int64(500))
	do := func(method, path, body string, want int) []byte {
		t.Helper()
		rec := adminDoAs(t, f, org, method, path, site, body)
		if rec.Code != want {
			t.Fatalf("%s %s = %d %s, want %d", method, path, rec.Code, rec.Body, want)
		}
		return rec.Body.Bytes()
	}
	bobs := func(body []byte) float64 {
		t.Helper()
		var ed AdminBlitzEdition
		if err := json.Unmarshal(body, &ed); err != nil {
			t.Fatal(err)
		}
		for _, s := range ed.Standings {
			if s.Name == "bob" {
				return s.Points
			}
		}
		return -1
	}
	const fide = `{"win": 1, "draw": 0.5, "loss": 0, "forfeit_win": 1, "forfeit_loss": 0, "bye": 1, "skip": 0}`
	const ours = `{"win": 3, "draw": 1, "loss": 0, "forfeit_win": 2.5, "forfeit_loss": 0, "bye": 3, "skip": 0.5}`
	body := do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "", http.StatusOK)
	wantJSON(t, body, "points", fide)
	wantJSON(t, body, "fide_points", "true")

	do(http.MethodPut, "/v1/admin/blitz/us_open/2026/points", ours, http.StatusNoContent)
	body = do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "", http.StatusOK)
	wantJSON(t, body, "points", ours)
	wantJSON(t, body, "fide_points", "false")
	if got := bobs(body); got != 3 {
		t.Errorf("bob, who won round 1's game = %g points, want the edition's 3", got)
	}

	do(http.MethodDelete, "/v1/admin/blitz/us_open/2026/points", "", http.StatusNoContent)
	body = do(http.MethodGet, "/v1/admin/blitz/us_open/2026", "", http.StatusOK)
	wantJSON(t, body, "points", fide)
	if got := bobs(body); got != 1 {
		t.Errorf("bob after the reset = %g points, want FIDE's 1", got)
	}
	want := []string{org + " blitz.points us_open/2026", org + " blitz.points.reset us_open/2026"}
	if !slices.Equal(f.audits, want) {
		t.Errorf("audits = %q, want %q", f.audits, want)
	}
}

func TestBadPointsAreRefused(t *testing.T) {
	const path = "/v1/admin/blitz/us_open/2026/points"
	points := func(win, draw, loss, bye float64) string {
		return fmt.Sprintf(`{"win": %g, "draw": %g, "loss": %g, "forfeit_win": 1, "forfeit_loss": 0, "bye": %g, "skip": 0}`, win, draw, loss, bye)
	}
	knockout := func(f *fakeAdmins) {
		f.rounds = append(f.rounds, store.BlitzRound{ID: 109, Number: 9, Kind: "knockout"})
	}
	tests := []struct {
		name, method, path, body string
		change                   func(*fakeAdmins)
		want                     int
	}{
		{"a win over 10", http.MethodPut, path, points(10.5, 0.5, 0, 1), nil, http.StatusBadRequest},
		{"a quarter", http.MethodPut, path, points(1, 0.5, 0, 0.25), nil, http.StatusBadRequest},
		{"a draw above a win", http.MethodPut, path, points(1, 1.5, 0, 1), nil, http.StatusBadRequest},
		{"points with the knockout paired", http.MethodPut, path, points(3, 1, 0, 3), knockout, http.StatusConflict},
		{"a reset with the knockout paired", http.MethodDelete, path, "", knockout, http.StatusConflict},
		{"an unknown edition's points", http.MethodPut, "/v1/admin/blitz/us_open/2027/points", points(3, 1, 0, 3), nil, http.StatusNotFound},
		{"an unknown edition's reset", http.MethodDelete, "/v1/admin/blitz/us_open/2027/points", "", nil, http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			roundOnePublished(f)
			if tt.change != nil {
				tt.change(f)
			}
			rec := adminDo(t, f, tt.method, tt.path, "https://ttac-admin.example", tt.body)
			if rec.Code != tt.want {
				t.Errorf("%s %s %s = %d %s, want %d", tt.method, tt.path, tt.body, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes)+len(f.audits) != 0 {
				t.Errorf("refused points still changed something: %q %q", f.notes, f.audits)
			}
		})
	}
}
