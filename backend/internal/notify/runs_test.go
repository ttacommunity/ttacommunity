package notify_test

import (
	"strings"
	"testing"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const leaderboard = "https://site.example/ratings/"

func TestARunPublishedLinksTheLeaderboardWithItsCounts(t *testing.T) {
	tests := []struct {
		name string
		run  store.RunNotice
		want string
	}{
		{"with a run before", store.RunNotice{ID: 7, BeforeID: 6, Games: 205123, TitlesGained: 5, TitlesLost: 2, Link: leaderboard},
			"Covers 205,123 games. Titles gained: 5 · titles lost: 2"},
		{"the first run", store.RunNotice{ID: 7, Games: 12, Link: leaderboard}, "Covers 12 games."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := notify.Payload(store.Notification{Kind: "ratings_run", Run: tt.run}, store.NotificationEndpoint{}, admin).Embeds[0]
			if e.Title != "New ratings are out" || e.URL != leaderboard || e.Description != tt.want {
				t.Errorf("embed = %q %q %q, want New ratings are out linking the leaderboard, %q", e.Title, e.URL, e.Description, tt.want)
			}
		})
	}
}

func TestTopTenChangesNameWhoMoved(t *testing.T) {
	n := store.Notification{Kind: "ratings_top10", Run: store.RunNotice{ID: 7, BeforeID: 6, Link: leaderboard},
		Top10: store.TopTenChange{NumberOne: "Player_*One*", In: []string{"Player_B", "Player_C"}, Out: []string{"Player_D"}}}
	e := notify.Payload(n, store.NotificationEndpoint{}, admin).Embeds[0]
	want := "New number one: **Player\\_\\*One\\***\nInto the top 10: Player\\_B, Player\\_C\nOut of the top 10: Player\\_D"
	if e.Title != "Top 10 changes" || e.URL != leaderboard || e.Description != want {
		t.Errorf("embed = %q %q %q, want Top 10 changes linking the leaderboard, %q", e.Title, e.URL, e.Description, want)
	}
	n.Top10 = store.TopTenChange{In: []string{"Player_B"}, Out: []string{"Player_D"}}
	if d := notify.Payload(n, store.NotificationEndpoint{}, admin).Embeds[0].Description; strings.Contains(d, "number one") {
		t.Errorf("description = %q, want no number one line when it is the same", d)
	}
}
