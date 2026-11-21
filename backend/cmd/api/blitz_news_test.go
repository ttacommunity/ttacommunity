package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
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

// signedIn is ed as admin reads it: Ann with the Discord id of her sign-in.
func signedIn(ed store.BlitzEdition) store.BlitzEdition {
	ed.Entrants = slices.Clone(ed.Entrants)
	ed.Entrants[0].DiscordID = "100000000000000052"
	return ed
}

func TestWithBlitzNews(t *testing.T) {
	unplayed := finalWon()
	unplayed.Rounds = slices.Clone(unplayed.Rounds)
	unplayed.Rounds[1].Matches = []store.BlitzMatch{{A: 1, B: unplayed.Rounds[1].Matches[0].B}}
	closes := time.Date(2026, 3, 8, 0, 0, 0, 0, time.UTC)
	taking, shut := store.BlitzEdition{}, store.BlitzEdition{}
	taking.Format, taking.SignUps = store.BlitzFormat{SwissRounds: 5, KnockoutSize: 4}, store.SignUps{Open: true, ClosesAt: &closes, Taking: true}
	shut.SignUps = store.SignUps{Open: true, ClosesAt: &closes}
	shown := stubEditions{"us_open 2026": finalWon(), "open 2026": unplayed, "taking 2026": taking, "shut 2026": shut}
	full := stubEditions{"us_open 2026": signedIn(finalWon()), "hidden 2026": signedIn(finalWon()), "open 2026": signedIn(unplayed),
		"taking 2026": taking, "shut 2026": shut}
	tests := []struct {
		name      string
		n         store.Notification
		wantPost  bool
		wantGames [][2]blitz.Person
		wantFirst string
		// wantCloses is a sign-up notice's closing time, with its edition's format.
		wantCloses *time.Time
		wantErr    bool
	}{
		{name: "a round published", n: store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 2},
			wantPost: true, wantGames: [][2]blitz.Person{{{Name: "Ann", DiscordID: "100000000000000052"}, {Name: "Bob"}}}},
		{name: "a round gone since", n: store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 3}},
		{name: "a reminder of a game to play", n: store.Notification{Kind: "blitz_reminder", Slug: "open", Edition: "2026", Round: 2},
			wantPost: true, wantGames: [][2]blitz.Person{{{Name: "Ann", DiscordID: "100000000000000052"}, {Name: "Bob"}}}},
		{name: "a reminder once every game is played", n: store.Notification{Kind: "blitz_reminder", Slug: "us_open", Edition: "2026", Round: 2}},
		{name: "an edition finished", n: store.Notification{Kind: "blitz_finished", Slug: "us_open", Edition: "2026"}, wantPost: true, wantFirst: "Ann"},
		{name: "sign-ups taking entries", n: store.Notification{Kind: "blitz_sign_ups", Slug: "taking", Edition: "2026"}, wantPost: true, wantCloses: &closes},
		{name: "sign-ups no longer taking entries", n: store.Notification{Kind: "blitz_sign_ups", Slug: "shut", Edition: "2026"}},
		{name: "sign-ups of an edition the site does not show", n: store.Notification{Kind: "blitz_sign_ups", Slug: "hidden", Edition: "2026"}},
		{name: "an edition the site does not show", n: store.Notification{Kind: "blitz_round", Slug: "hidden", Edition: "2026", Round: 1}},
		{name: "an edition that cannot be read", n: store.Notification{Kind: "blitz_round", Slug: "broken", Edition: "2026", Round: 1}, wantErr: true},
		{name: "another kind", n: store.Notification{Kind: "sign_up"}, wantPost: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, post, err := withBlitzNews(context.Background(), shown, full, "https://site.example", tt.n)
			if (err != nil) != tt.wantErr || post != tt.wantPost {
				t.Fatalf("withBlitzNews = post %v, %v; want post %v, error %v", post, err, tt.wantPost, tt.wantErr)
			}
			if !post || tt.n.Kind == "sign_up" {
				return
			}
			if link := "https://site.example/blitz/?t=" + tt.n.Slug + "&e=2026"; got.Blitz.Link != link {
				t.Errorf("link = %q, want %q", got.Blitz.Link, link)
			}
			if tt.wantGames != nil && !slices.Equal(got.Blitz.Games, tt.wantGames) {
				t.Errorf("games = %+v, want %+v", got.Blitz.Games, tt.wantGames)
			}
			if tt.wantCloses != nil && (got.Blitz.Closes == nil || !got.Blitz.Closes.Equal(*tt.wantCloses) || got.Blitz.Format != (store.BlitzFormat{SwissRounds: 5, KnockoutSize: 4})) {
				t.Errorf("sign-ups = closing %v, format %+v; want closing %v, 5 rounds and a knockout of 4", got.Blitz.Closes, got.Blitz.Format, *tt.wantCloses)
			}
			if tt.wantFirst != "" && (len(got.Blitz.Podium) == 0 || len(got.Blitz.Podium[0].People) != 1 || got.Blitz.Podium[0].People[0].Name != tt.wantFirst) {
				t.Errorf("podium = %+v, want %s first", got.Blitz.Podium, tt.wantFirst)
			}
		})
	}
}

func TestARoundPairedAgainKeepsSaying(t *testing.T) {
	n := store.Notification{Kind: "blitz_round", Slug: "us_open", Edition: "2026", Round: 2, Blitz: store.BlitzNews{Again: true}}
	ed := stubEditions{"us_open 2026": finalWon()}
	got, _, err := withBlitzNews(context.Background(), ed, ed, "https://site.example", n)
	if err != nil || !got.Blitz.Again {
		t.Errorf("again = %v, %v; want it kept from the outbox", got.Blitz.Again, err)
	}
}

func TestWithBlitzNewsFillsAGamesThread(t *testing.T) {
	deadline := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	unplayed := finalWon()
	unplayed.Rounds = slices.Clone(unplayed.Rounds)
	unplayed.Rounds[1].Deadline = &deadline
	unplayed.Rounds[1].Matches = []store.BlitzMatch{{ID: 9, A: 1, B: unplayed.Rounds[1].Matches[0].B}}
	shown := stubEditions{"open 2026": unplayed, "us_open 2026": finalWon()}
	full := stubEditions{"open 2026": signedIn(unplayed), "hidden 2026": signedIn(unplayed), "us_open 2026": signedIn(finalWon())}
	tests := []struct {
		name     string
		slug     string
		wantPost bool
	}{
		{"a game to play", "open", true},
		{"a game played", "us_open", false},
		{"an edition the site does not show", "hidden", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := store.Notification{Kind: "blitz_thread", Slug: tt.slug, Edition: "2026", Round: 2, MatchID: 9}
			got, post, err := withBlitzNews(context.Background(), shown, full, "https://site.example", n)
			if err != nil || post != tt.wantPost {
				t.Fatalf("withBlitzNews = post %v, %v; want post %v", post, err, tt.wantPost)
			}
			if post && (got.Thread.Game.A != blitz.Person{Name: "Ann", DiscordID: "100000000000000052"} || !got.Thread.Threads.Deadline.Equal(deadline)) {
				t.Errorf("thread = %+v, want Ann's game with her Discord id, by the round's deadline", got.Thread)
			}
		})
	}
}

func TestWithBlitzNewsKeysAReminderGamesByMatch(t *testing.T) {
	deadline := time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC)
	ed := finalWon()
	ed.Rounds = slices.Clone(ed.Rounds)
	ed.Rounds[1].Deadline = &deadline
	ed.Rounds[1].Matches = []store.BlitzMatch{{ID: 9, A: 1, B: ed.Rounds[1].Matches[0].B}}
	editions := stubEditions{"open 2026": ed}
	got, post, err := withBlitzNews(context.Background(), editions, editions, "https://site.example", store.Notification{Kind: "blitz_reminder", Slug: "open", Edition: "2026", Round: 2})
	if err != nil || !post || got.Thread.ByMatch[9].B.Name != "Bob" || !got.Thread.Threads.Deadline.Equal(deadline) {
		t.Errorf("withBlitzNews = %+v, %t, %v; want match 9's game and the deadline for its thread", got.Thread, post, err)
	}
}
