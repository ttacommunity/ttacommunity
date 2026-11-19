package notify_test

import (
	"context"
	"encoding/json"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	players "ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const edition = "https://site.example/blitz/?t=us_open&e=2026"

// named are players without a Discord id, by username.
func named(names ...string) []players.Person {
	out := make([]players.Person, len(names))
	for i, n := range names {
		out[i] = players.Person{Name: n, Discord: n}
	}
	return out
}

func game(a, b players.Person) [2]players.Person { return [2]players.Person{a, b} }

func TestARoundPublishedIsTextOfItsTitleDeadlineAndPairings(t *testing.T) {
	deadline := time.Date(2026, 3, 8, 18, 0, 0, 0, time.UTC)
	ann := players.Person{Name: "Ann_1", Discord: "ann", DiscordID: "100000000000000061"}
	p := named("Bob", "Cid", "Dee", "Eve")
	round := func(news store.BlitzNews) store.Notification {
		news.Link = edition
		return store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Round: 3, Blitz: news}
	}
	const title = "**[US Open 2026 · Round 3](<" + edition + ">)**\n"
	tests := []struct {
		name, content string
		n             store.Notification
		users         []string
	}{
		{"games, a bye and a deadline",
			title + "Deadline: <t:1772992800:F> (<t:1772992800:R>)\n**Pairings**\nAnn\\_1 vs Bob\nCid vs Dee\nBye: Eve",
			round(store.BlitzNews{Title: "Round 3", Games: [][2]players.Person{game(players.Person{Name: "Ann_1"}, p[0]), game(p[1], p[2])}, Byes: p[3:], Deadline: &deadline}), nil},
		{"a player signed in with Discord: named, then tagged", title + "**Pairings**\nAnn\\_1 (<@100000000000000061>) vs Bob",
			round(store.BlitzNews{Title: "Round 3", Games: [][2]players.Person{game(ann, p[0])}}), []string{"100000000000000061"}},
		{"a bye signed in with Discord: tagged", title + "**Pairings**\nBob vs Cid\nBye: Ann\\_1 (<@100000000000000061>)",
			round(store.BlitzNews{Title: "Round 3", Games: [][2]players.Person{game(p[0], p[1])}, Byes: []players.Person{ann}}), []string{"100000000000000061"}},
		{"paired again", "**[US Open 2026 · Round 9 · Final, paired again](<" + edition + ">)**\n**Pairings**\nBob vs Cid",
			round(store.BlitzNews{Title: "Round 9 · Final", Games: [][2]players.Person{game(p[0], p[1])}, Again: true}), nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := notify.Payload(tt.n, store.NotificationEndpoint{}, admin)
			if m.Content != tt.content || !slices.Equal(m.Users, tt.users) || len(m.Embeds) != 0 {
				t.Errorf("content %q, users %q, embeds %d; want %q, %q, none", m.Content, m.Users, len(m.Embeds), tt.content, tt.users)
			}
		})
	}
}

func TestABlitzTitleIsShownAsWrittenInsideItsLink(t *testing.T) {
	tests := []struct{ name, series, round, want string }{
		{"a knockout stage's hyphen", "US Open", "Round 7 · Quarter-finals", "**[US Open 2026 · Round 7 · Quarter-finals](<" + edition + ">)**"},
		{"brackets that would end the link", "Cup [B]", "Round 1", "**[Cup (B) 2026 · Round 1](<" + edition + ">)**"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n := store.Notification{Kind: "blitz_round", Series: tt.series, Edition: "2026", Blitz: store.BlitzNews{Title: tt.round, Link: edition}}
			if got, _, _ := strings.Cut(notify.Payload(n, store.NotificationEndpoint{}, admin).Content, "\n"); got != tt.want {
				t.Errorf("title line = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestARoundFollowsTheEndpointsPrefixRoleNameAndThread(t *testing.T) {
	n := store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Title: "Round 1", Link: edition,
		Games: [][2]players.Person{game(named("Bob")[0], named("Cid")[0])}}}
	m := notify.Payload(n, store.NotificationEndpoint{Prefix: "[local]", RoleID: "100000000000000002", Username: "TTA", ThreadID: thread}, admin)
	want := `\[local\] <@&100000000000000002>` + "\n**[US Open 2026 · Round 1](<" + edition + ">)**\n**Pairings**\nBob vs Cid"
	if m.Content != want || m.Username != "TTA" || m.ThreadID != thread || !slices.Equal(m.Roles, []string{"100000000000000002"}) {
		t.Errorf("message = %+v, want content %q under the endpoint's name, in its thread, pinging its role", m, want)
	}
}

func TestAFinishedEditionIsTextOfItsFinalResults(t *testing.T) {
	ann := players.Person{Name: "Ann_1", Discord: "ann", DiscordID: "100000000000000061"}
	n := store.Notification{Kind: "blitz_finished", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Link: edition, Podium: []store.BlitzPlace{
		{Place: 1, People: []players.Person{ann}}, {Place: 2, People: named("Bob")}, {Place: 3, People: named("Cid", "Dee")},
	}}}
	m := notify.Payload(n, store.NotificationEndpoint{Forum: true}, admin)
	want := "**[US Open 2026 final results](<" + edition + ">)**\n1st: **Ann\\_1** (<@100000000000000061>)\n2nd: Bob\n3rd: Cid and Dee"
	if m.Content != want || len(m.Embeds) != 0 || !slices.Equal(m.Users, []string{"100000000000000061"}) || m.ThreadName != "US Open 2026 final results" {
		t.Errorf("message = %+v, want %q tagging Ann, in a thread named for it", m, want)
	}
}

func TestABlitzPostOpensAThreadNamedByItsTitleInAForum(t *testing.T) {
	n := store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Title: "Round 3", Games: [][2]players.Person{game(named("Ann")[0], named("Bob")[0])}, Link: edition}}
	m := notify.Payload(n, store.NotificationEndpoint{Forum: true}, admin)
	if want := "**[US Open 2026 · Round 3](<" + edition + ">)**\n**Pairings**\nAnn vs Bob"; m.ThreadName != "US Open 2026 · Round 3" || m.Content != want {
		t.Errorf("forum post = thread %q, content %q; want the round's title and %q", m.ThreadName, m.Content, want)
	}
}

func TestSendAllowsOnlyTheUsersARoundTags(t *testing.T) {
	var got map[string]any
	s := discord(t, http.StatusNoContent, &got)
	ann := players.Person{Name: "Ann", DiscordID: "100000000000000061"}
	n := store.Notification{Kind: "blitz_round", Series: "US Open", Edition: "2026", Blitz: store.BlitzNews{Title: "Round 1",
		Games: [][2]players.Person{game(ann, named("Bob")[0])}, Byes: []players.Person{ann}}}
	if err := notify.NewDiscord(s.Client()).Send(context.Background(), s.URL, notify.Payload(n, store.NotificationEndpoint{}, admin)); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if mentions, _ := json.Marshal(got["allowed_mentions"]); string(mentions) != `{"parse":[],"users":["100000000000000061"]}` {
		t.Errorf("allowed_mentions = %s, want only Ann, once", mentions)
	}
}
