package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

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
	row := store.BlitzEditionRow{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat}
	if f.format != (store.BlitzFormat{}) {
		row.Format = f.format
	}
	for _, e := range f.blitzEntrants() {
		switch e.Status {
		case "dropped":
			row.Dropped++
		case "pending":
			row.Pending++
		default:
			row.Entrants++
		}
	}
	return row
}

func (f *fakeAdmins) BlitzEditions(context.Context) ([]store.BlitzEditionRow, error) {
	return []store.BlitzEditionRow{f.blitzRow()}, nil
}

func (f *fakeAdmins) BlitzEdition(_ context.Context, slug, edition string) (store.BlitzEdition, error) {
	if slug != "us_open" || edition != "2026" {
		return store.BlitzEdition{}, store.ErrNoBlitzEdition
	}
	return store.BlitzEdition{BlitzEditionRow: f.blitzRow(), Entrants: f.blitzEntrants(), Rounds: slices.Clone(f.rounds)}, nil
}

func (f *fakeAdmins) SetBlitzFormat(_ context.Context, slug, edition string, format store.BlitzFormat) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
	}
	if format.KnockoutSize == 3 {
		return store.ErrBadBlitzFormat
	}
	f.format = format
	f.notes = append(f.notes, fmt.Sprintf("format %d/%d", format.SwissRounds, format.KnockoutSize))
	return nil
}

func (f *fakeAdmins) AddBlitzEntrants(_ context.Context, slug, edition string, entrants []store.BlitzEntrant) ([]int64, error) {
	if slug != "us_open" || edition != "2026" {
		return nil, store.ErrNoBlitzEdition
	}
	var ids, names = []int64{}, []string{}
	for _, e := range entrants {
		if strings.EqualFold(e.Name, "Ann") {
			return nil, &store.BlitzEntrantExistsError{Names: []string{"Ann"}}
		}
		if e.Name == "" {
			return nil, store.ErrBadBlitzEntrant
		}
		e.ID = int64(len(f.blitzEntrants()) + 7)
		if e.Status == "" {
			e.Status = "active"
		}
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
	if c.Status != nil && *c.Status != "active" && *c.Status != "dropped" {
		return store.ErrBadBlitzStatus
	}
	for i := range f.blitzEntrants() {
		if f.entrants[i].ID != id {
			continue
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
		f.notes = append(f.notes, fmt.Sprintf("set entrant %d %s", id, strings.Join(changed, " ")))
		return nil
	}
	return store.ErrNoBlitzEntrant
}

func (f *fakeAdmins) RemoveBlitzEntrant(_ context.Context, slug, edition string, id int64) error {
	if slug != "us_open" || edition != "2026" {
		return store.ErrNoBlitzEdition
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

	rec = do(http.MethodPost, "/v1/admin/blitz/us_open/2026/entrants", site, `{"entrants": [{"name": "bob", "discord_name": "bobby", "zone": " America/New_York "}, {"name": "cy", "zone": "UTC", "status": "pending"}]}`)
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
		{Id: 7, Name: "Ann", Player: "Ann", DiscordName: "ann#1", Zone: "Europe/Berlin", UtcOffset: "+02:00", Rating: &ann, Status: "active"},
		{Id: 8, Name: "bob", Player: "", DiscordName: "bobby", Zone: "America/New_York", UtcOffset: "-04:00", Rating: nil, Status: "active"},
		{Id: 9, Name: "cy", Player: "", DiscordName: "", Zone: "UTC", UtcOffset: "+00:00", Rating: nil, Status: "pending"},
	}
	if diff := cmp.Diff(wantEntrants, ed.Entrants); diff != "" {
		t.Errorf("entrants (-want +got):\n%s", diff)
	}
	if ed.Format.SwissRounds != 8 || ed.Format.KnockoutSize != 8 {
		t.Errorf("format = %+v, want the default 8/8", ed.Format)
	}

	for _, step := range []struct{ method, path, body string }{
		{http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/8", `{"status": "dropped"}`},
		{http.MethodPatch, "/v1/admin/blitz/us_open/2026", `{"swiss_rounds": 6, "knockout_size": 4}`},
		{http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/8", ""},
	} {
		if rec := do(step.method, step.path, site, step.body); rec.Code != http.StatusNoContent {
			t.Errorf("%s %s = %d %s, want 204", step.method, step.path, rec.Code, rec.Body)
		}
	}
	if want := "add entrants bob,cy|set entrant 8 status=dropped|format 6/4|remove entrant 8"; strings.Join(f.notes, "|") != want {
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
		{"a zone change to a non-zone", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/7", site, `{"zone": "GMT+2"}`, http.StatusBadRequest, "GMT+2"},
		{"changing an unknown entrant", http.MethodPatch, "/v1/admin/blitz/us_open/2026/entrants/99", site, `{"status": "dropped"}`, http.StatusNotFound, ""},
		{"removing an unknown entrant", http.MethodDelete, "/v1/admin/blitz/us_open/2026/entrants/99", site, "", http.StatusNotFound, ""},
		{"a knockout of three", http.MethodPatch, "/v1/admin/blitz/us_open/2026", site, `{"swiss_rounds": 8, "knockout_size": 3}`, http.StatusBadRequest, ""},
		{"a format for an unknown edition", http.MethodPatch, "/v1/admin/blitz/us_open/2027", site, `{"swiss_rounds": 8, "knockout_size": 8}`, http.StatusNotFound, ""},
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
