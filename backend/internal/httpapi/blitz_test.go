package httpapi

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/go-cmp/cmp"

	"ttacommunity/backend/internal/store"
)

// The fake's shown blitz edition is us_open 2026 with Ann (known to the ratings) and bob (not) still in.

func (f *fakeRatings) BlitzEditions(context.Context) ([]store.BlitzEditionRow, error) {
	return []store.BlitzEditionRow{{Slug: "us_open", Name: "US Open", Edition: "2026", Format: store.DefaultBlitzFormat, Entrants: 2}}, nil
}

func (f *fakeRatings) BlitzEdition(ctx context.Context, slug, edition string) (store.BlitzEdition, error) {
	if slug != "us_open" || edition != "2026" {
		return store.BlitzEdition{}, store.ErrNoBlitzEdition
	}
	rows, _ := f.BlitzEditions(ctx)
	return store.BlitzEdition{BlitzEditionRow: rows[0], Entrants: []store.BlitzEntrant{
		{ID: 7, Name: "Ann", Player: "Ann", Zone: "Europe/Berlin", Status: "active"},
		{ID: 8, Name: "bob", Zone: "America/New_York", Status: "active"},
	}}, nil
}

func TestTheSiteListsBlitzEditionsAndTheirEntrants(t *testing.T) {
	ratings := &fakeRatings{run: store.PublishedRun{ID: 7}, rated: map[string]store.Rated{"Ann": {Rating: 1700, RD: 60}}}
	rec, _ := serve(t, ratings, "/v1/blitz")
	var list BlitzEditions
	if err := json.Unmarshal(rec.Body.Bytes(), &list); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/blitz = %d %s", rec.Code, rec.Body)
	}
	if len(list.Editions) != 1 || list.Editions[0].Tournament != "us_open" || list.Editions[0].Entrants != 2 {
		t.Errorf("editions = %+v, want us_open 2026 with 2 entrants", list.Editions)
	}
	if got := rec.Header().Get("Cache-Control"); got != cacheCurrent {
		t.Errorf("Cache-Control = %q, want %q", got, cacheCurrent)
	}

	rec, _ = serve(t, ratings, "/v1/blitz/us_open/2026")
	var ed BlitzEdition
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET /v1/blitz/us_open/2026 = %d %s", rec.Code, rec.Body)
	}
	ann := 1700.0
	want := []BlitzEntrant{
		{Name: "Ann", Zone: "Europe/Berlin", UtcOffset: "+02:00", Rating: &ann},
		{Name: "bob", Zone: "America/New_York", UtcOffset: "-04:00", Rating: nil},
	}
	if diff := cmp.Diff(want, ed.Entrants); diff != "" {
		t.Errorf("entrants (-want +got):\n%s", diff)
	}
	if strings.Contains(strings.ToLower(rec.Body.String()), "discord") {
		t.Errorf("the site's edition mentions Discord: %s", rec.Body)
	}

	rec, _ = serve(t, &fakeRatings{runErr: store.ErrNoPublishedRun}, "/v1/blitz/us_open/2026")
	if err := json.Unmarshal(rec.Body.Bytes(), &ed); err != nil || rec.Code != http.StatusOK || ed.Entrants[0].Rating != nil {
		t.Errorf("GET edition with no published run = %d %s, want 200 with no ratings", rec.Code, rec.Body)
	}
}

func TestTheSiteBlitzRoutesRefuseWhatTheyShould(t *testing.T) {
	tests := []struct {
		name, path string
		want       int
	}{
		{"a bad slug", "/v1/blitz/Bad!/2026", http.StatusBadRequest},
		{"an unknown edition", "/v1/blitz/us_open/2027", http.StatusNotFound},
		{"an unknown tournament", "/v1/blitz/worlds/2026", http.StatusNotFound},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if rec, _ := serve(t, &fakeRatings{}, tt.path); rec.Code != tt.want {
				t.Errorf("GET %s = %d %s, want %d", tt.path, rec.Code, rec.Body, tt.want)
			}
		})
	}
}
