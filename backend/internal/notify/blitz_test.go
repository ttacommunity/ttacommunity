package notify_test

import (
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const edition = "https://site.example/blitz/?t=us_open&e=2026"

func TestARoundPublishedListsItsGamesByeAndDeadline(t *testing.T) {
	deadline := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	round := func(news store.BlitzNews) store.Notification {
		news.Link = edition
		return store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Round: 3, Blitz: news}
	}
	tests := []struct {
		name               string
		n                  store.Notification
		title, description string
		fields             []notify.EmbedField
	}{
		{"games, a bye and a deadline",
			round(store.BlitzNews{Title: "Round 3", Games: [][2]string{{"Ann_1", "Bob"}, {"Cid", "Dee"}}, Byes: []string{"Eve"}, Deadline: &deadline}),
			"US Open 2026 · Round 3", "Ann\\_1 vs Bob\nCid vs Dee\nBye: Eve",
			[]notify.EmbedField{{Name: "Deadline", Value: "<t:1772992800:F> (<t:1772992800:R>)"}}},
		{"no deadline yet, no bye", round(store.BlitzNews{Title: "Round 3", Games: [][2]string{{"Ann", "Bob"}}}),
			"US Open 2026 · Round 3", "Ann vs Bob", nil},
		{"paired again", round(store.BlitzNews{Title: "Round 9 · Final", Games: [][2]string{{"Ann", "Bob"}}, Again: true}),
			"US Open 2026 · Round 9 · Final, paired again", "Ann vs Bob", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := notify.Payload(tt.n, store.NotificationEndpoint{}, admin).Embeds[0]
			if e.Title != tt.title || e.URL != edition || e.Description != tt.description || !equalFields(e.Fields, tt.fields) {
				t.Errorf("embed = %q %q %q %v, want %q linking the edition, %q, %v", e.Title, e.URL, e.Description, e.Fields, tt.title, tt.description, tt.fields)
			}
		})
	}
}

func TestAFinishedEditionNamesItsPodium(t *testing.T) {
	n := store.Notification{Kind: "blitz_finished", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Link: edition, Podium: []store.BlitzPlace{
		{Place: 1, Names: []string{"Ann_1"}}, {Place: 2, Names: []string{"Bob"}}, {Place: 3, Names: []string{"Cid", "Dee"}},
	}}}
	e := notify.Payload(n, store.NotificationEndpoint{}, admin).Embeds[0]
	want := "1st: **Ann\\_1**\n2nd: Bob\n3rd: Cid and Dee"
	if e.Title != "US Open 2026 is finished" || e.URL != edition || e.Description != want {
		t.Errorf("embed = %q %q %q, want US Open 2026 is finished linking the edition, %q", e.Title, e.URL, e.Description, want)
	}
}

func TestABlitzPostOpensAThreadNamedByItsTitleInAForum(t *testing.T) {
	n := store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Title: "Round 3", Games: [][2]string{{"Ann", "Bob"}}, Link: edition}}
	m := notify.Payload(n, store.NotificationEndpoint{Forum: true}, admin)
	if m.ThreadName != "US Open 2026 · Round 3" || m.Content != "Ann vs Bob" {
		t.Errorf("forum post = thread %q, content %q; want the round's title and its games", m.ThreadName, m.Content)
	}
}

func equalFields(a, b []notify.EmbedField) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
