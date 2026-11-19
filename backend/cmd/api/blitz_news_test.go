package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

// stubEditions holds the editions the site shows, by "slug edition".
type stubEditions map[string]store.BlitzEdition

func (s stubEditions) BlitzEdition(_ context.Context, slug, edition string) (store.BlitzEdition, error) {
	if slug == "broken" {
		return store.BlitzEdition{}, errors.New("the database is down")
	}
	ed, ok := s[slug+" "+edition]
	if !ok {
		return store.BlitzEdition{}, store.ErrNoBlitzEdition
	}
	return ed, nil
}

// finalWon is a Swiss round and a final, Ann beating Bob, of an edition of one Swiss round and a knockout of two.
func finalWon() store.BlitzEdition {
	at := time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)
	bob := int64(2)
	ed := store.BlitzEdition{Entrants: []store.BlitzEntrant{{ID: 1, Name: "Ann"}, {ID: 2, Name: "Bob"}}, Rounds: []store.BlitzRound{
		{Number: 1, Kind: "swiss", PublishedAt: &at, Matches: []store.BlitzMatch{{A: 1, B: &bob, Result: "a"}}},
		{Number: 2, Kind: "knockout", PublishedAt: &at, Matches: []store.BlitzMatch{{A: 1, B: &bob, Result: "a"}}},
	}}
	ed.Format = store.BlitzFormat{SwissRounds: 1, KnockoutSize: 2}
	return ed
}

func TestWithBlitzNews(t *testing.T) {
	editions := stubEditions{"us_open 2026": finalWon()}
	const link = "https://site.example/blitz/?t=us_open&e=2026"
	tests := []struct {
		name      string
		n         store.Notification
		wantPost  bool
		wantGames [][2]string
		wantFirst string
		wantErr   bool
	}{
		{name: "a round published", n: store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 2},
			wantPost: true, wantGames: [][2]string{{"Ann", "Bob"}}},
		{name: "a round gone since", n: store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 3}},
		{name: "an edition finished", n: store.Notification{Kind: "blitz_finished", Slug: "us_open", Edition: "2026"}, wantPost: true, wantFirst: "Ann"},
		{name: "an edition the site does not show", n: store.Notification{Kind: "blitz_round", Slug: "hidden", Edition: "2026", Round: 1}},
		{name: "an edition that cannot be read", n: store.Notification{Kind: "blitz_round", Slug: "broken", Edition: "2026", Round: 1}, wantErr: true},
		{name: "another kind", n: store.Notification{Kind: "sign_up"}, wantPost: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, post, err := withBlitzNews(context.Background(), editions, "https://site.example", tt.n)
			if (err != nil) != tt.wantErr || post != tt.wantPost {
				t.Fatalf("withBlitzNews = post %v, %v; want post %v, error %v", post, err, tt.wantPost, tt.wantErr)
			}
			if !post || tt.n.Kind == "sign_up" {
				return
			}
			if got.Blitz.Link != link {
				t.Errorf("link = %q, want %q", got.Blitz.Link, link)
			}
			if tt.wantGames != nil && !slices.Equal(got.Blitz.Games, tt.wantGames) {
				t.Errorf("games = %q, want %q", got.Blitz.Games, tt.wantGames)
			}
			if tt.wantFirst != "" && (len(got.Blitz.Podium) == 0 || !slices.Equal(got.Blitz.Podium[0].Names, []string{tt.wantFirst})) {
				t.Errorf("podium = %+v, want %s first", got.Blitz.Podium, tt.wantFirst)
			}
		})
	}
}

func TestARoundPairedAgainKeepsSaying(t *testing.T) {
	n := store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 2, Blitz: store.BlitzNews{Again: true}}
	got, _, err := withBlitzNews(context.Background(), stubEditions{"us_open 2026": finalWon()}, "https://site.example", n)
	if err != nil || !got.Blitz.Again {
		t.Errorf("again = %v, %v; want it kept from the outbox", got.Blitz.Again, err)
	}
}
