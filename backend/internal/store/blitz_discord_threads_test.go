//go:build integration

package store

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// channelID is an invented Discord channel id, numbered n.
func channelID(n int) string { return fmt.Sprintf("20000000000000000%d", n) }

// botChannel adds a bot channel named name, numbered n, taking blitz_threads, and returns its id.
func botChannel(t *testing.T, a *AdminStore, name string, n int) int64 {
	t.Helper()
	id, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: name, Kind: "bot_channel", ChannelID: channelID(n),
		Format: "discord", Events: []string{"blitz_threads"}}, reportsAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint(%s): %v", name, err)
	}
	return id
}

// threadsSent sends what is due, making and posting each game's thread on its endpoint as made says, and returns
// each as "endpoint round A v B".
func threadsSent(t *testing.T, a *AdminStore, made bool) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), time.Now().Add(time.Hour), 50, func(ctx context.Context, n Notification, e NotificationEndpoint) error {
		if n.Kind != "blitz_thread" {
			return nil
		}
		got = append(got, fmt.Sprintf("%s %d %s v %s", e.Name, n.Round, n.A, n.B))
		if !made {
			return nil
		}
		if err := a.RecordBlitzThread(ctx, n.MatchID, e.ID, fmt.Sprintf("3000000000000000%02d", n.MatchID%100), reportsAt); err != nil {
			return err
		}
		return a.MarkBlitzThreadPosted(ctx, n.MatchID, e.ID, "500000000000000001", reportsAt)
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

// setDeadline sets round's deadline a day from now.
func setDeadline(t *testing.T, a *AdminStore, round int64) {
	t.Helper()
	if err := a.SetBlitzRoundDeadline(context.Background(), "us_open", "2026", round, time.Now().Add(24*time.Hour).UTC(), "UTC"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline: %v", err)
	}
}

func TestABotChannelIsKeptWithItsChannel(t *testing.T) {
	a, _, _ := outboxFixture(t)
	ctx := context.Background()
	id := botChannel(t, a, "Threads", 1)
	got, err := a.NotificationEndpoint(ctx, id)
	if err != nil {
		t.Fatalf("NotificationEndpoint: %v", err)
	}
	if line := fmt.Sprintf("%s %s %q %q", got.Kind, got.ChannelID, got.URL, got.Events); line != `bot_channel `+channelID(1)+` "" ["blitz_threads"]` {
		t.Errorf("the bot channel = %s, want its kind, channel and event, no URL", line)
	}
	got.Kind, got.ChannelID = "webhook", channelID(2)
	if err := a.SetNotificationEndpoint(ctx, got); err != nil {
		t.Fatalf("SetNotificationEndpoint: %v", err)
	}
	if got, err := a.NotificationEndpoint(ctx, id); err != nil || got.Kind != "bot_channel" || got.ChannelID != channelID(2) {
		t.Errorf("after a change = %s %s, %v; want still a bot channel, in the new channel", got.Kind, got.ChannelID, err)
	}
	web, err := a.NotificationEndpoint(ctx, endpoint(t, a, "Webhook", 2, "blitz_round"))
	if err != nil || web.Kind != "webhook" || web.ChannelID != "" {
		t.Errorf("a webhook = %q %q, %v; want kind webhook with no channel", web.Kind, web.ChannelID, err)
	}
}

func TestTheDatabaseRefusesABadBotChannel(t *testing.T) {
	good := NotificationEndpoint{Name: "Threads", Kind: "bot_channel", ChannelID: channelID(1), Format: "discord", Events: []string{"blitz_threads"}}
	tests := []struct {
		name   string
		change func(e *NotificationEndpoint)
	}{
		{"another kind", func(e *NotificationEndpoint) { e.Kind = "email" }},
		{"no channel", func(e *NotificationEndpoint) { e.ChannelID = "" }},
		{"a channel that is not an id", func(e *NotificationEndpoint) { e.ChannelID = "general" }},
		{"a webhook URL too", func(e *NotificationEndpoint) { e.URL = webhook(1) }},
		{"a webhook's event", func(e *NotificationEndpoint) { e.Events = []string{"blitz_threads", "blitz_round"} }},
		{"a webhook taking threads", func(e *NotificationEndpoint) { e.Kind, e.ChannelID, e.URL = "webhook", "", webhook(1) }},
		{"a webhook with a channel", func(e *NotificationEndpoint) {
			e.Kind, e.URL, e.Events = "webhook", webhook(1), []string{"blitz_round"}
		}},
	}
	a, _, _ := outboxFixture(t)
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := good
			tt.change(&e)
			if _, err := a.AddNotificationEndpoint(context.Background(), e, reportsAt); !errors.Is(err, ErrBadNotificationEndpoint) {
				t.Errorf("AddNotificationEndpoint: %v, want ErrBadNotificationEndpoint", err)
			}
		})
	}
}

func TestARoundsThreadsAreQueuedOncePublishedWithADeadline(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy := ids[0], ids[1], ids[2]
	markDropped(t, a, ids[3])
	botChannel(t, a, "Threads", 1)
	endpoint(t, a, "Posts", 2, "blitz_round")
	round := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, 0})
	if got := threadsSent(t, a, true); len(got) != 0 {
		t.Errorf("threads of a round without a deadline = %q, want none yet", got)
	}
	setDeadline(t, a, round)
	if got, want := threadsSent(t, a, true), []string{"Threads 1 Ann v Bob"}; !slices.Equal(got, want) {
		t.Errorf("threads once the deadline is set = %q, want %q: the bye gets none", got, want)
	}
	setDeadline(t, a, round)
	if got := threadsSent(t, a, true); len(got) != 0 {
		t.Errorf("threads after the deadline is moved = %q, want none: they are made", got)
	}
}

func TestARoundPublishedWithADeadlineQueuesItsThreads(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	botChannel(t, a, "Threads", 1)
	r, err := a.AddBlitzRound(ctx, "us_open", "2026", BlitzRound{Method: "dutch", Order: "rating", Seed: 1,
		Matches: []BlitzMatch{{A: ids[0], B: &ids[1]}, {A: ids[2], B: &ids[3]}}})
	if err != nil {
		t.Fatalf("AddBlitzRound: %v", err)
	}
	setDeadline(t, a, r.ID)
	if got := threadsSent(t, a, true); len(got) != 0 {
		t.Errorf("threads of a draft = %q, want none", got)
	}
	if err := a.PublishBlitzRound(ctx, "us_open", "2026", r.ID); err != nil {
		t.Fatalf("PublishBlitzRound: %v", err)
	}
	if got, want := threadsSent(t, a, true), []string{"Threads 1 Ann v Bob", "Threads 1 Cy v Dee"}; !slices.Equal(got, want) {
		t.Errorf("threads once published = %q, want %q", got, want)
	}
}

func TestNoThreadIsQueuedWithoutABotChannel(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	endpoint(t, a, "Posts", 1, "blitz_round")
	setDeadline(t, a, publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]}))
	if got := rowsOf(t, a.pool.(*pgxpool.Pool), "SELECT kind FROM notification_outbox WHERE kind = 'blitz_thread'"); len(got) != 0 {
		t.Errorf("queued = %q, want no thread", got)
	}
}

func TestAThreadNotMadeIsQueuedAgainWhenTheDeadlineIsMoved(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	botChannel(t, a, "Threads", 1)
	round := publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	setDeadline(t, a, round)
	threadsSent(t, a, false)
	setDeadline(t, a, round)
	if got, want := threadsSent(t, a, true), []string{"Threads 1 Ann v Bob", "Threads 1 Cy v Dee"}; !slices.Equal(got, want) {
		t.Errorf("threads never made = %q, want %q", got, want)
	}
}

func TestAGamePairedAgainKeepsItsThread(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	botChannel(t, a, "Threads", 1)
	first := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	setDeadline(t, a, first)
	threadsSent(t, a, true)
	deleteRound(t, a, first)
	again := publishedRound(t, a, [2]int64{bob, ann}, [2]int64{dee, cy})
	setDeadline(t, a, again)
	if got := threadsSent(t, a, true); len(got) != 0 {
		t.Errorf("threads of the same games paired again = %q, want none: they have one", got)
	}
	deleteRound(t, a, again)
	other := publishedRound(t, a, [2]int64{ann, cy}, [2]int64{bob, dee})
	setDeadline(t, a, other)
	if got, want := threadsSent(t, a, true), []string{"Threads 1 Ann v Cy", "Threads 1 Bob v Dee"}; !slices.Equal(got, want) {
		t.Errorf("threads of other games = %q, want %q", got, want)
	}
}

func TestAGamesThreadIsKeptPerEndpointUntilPosted(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	one, two := botChannel(t, a, "One", 1), botChannel(t, a, "Two", 2)
	round := publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	var match int64
	if err := a.pool.QueryRow(ctx, "SELECT match_id FROM blitz_matches WHERE round_id = $1 AND entry_a = $2", round, ids[0]).Scan(&match); err != nil {
		t.Fatalf("reading the match: %v", err)
	}
	if got, err := a.BlitzThread(ctx, match, one); err != nil || got != nil {
		t.Fatalf("BlitzThread before = %+v, %v; want none", got, err)
	}
	if err := a.RecordBlitzThread(ctx, match, one, "300000000000000001", reportsAt); err != nil {
		t.Fatalf("RecordBlitzThread: %v", err)
	}
	if got, err := a.BlitzThread(ctx, match, one); err != nil || got == nil || *got != (BlitzDiscordThread{ID: "300000000000000001"}) {
		t.Errorf("BlitzThread once made = %+v, %v; want its id, not posted", got, err)
	}
	if got, err := a.BlitzThread(ctx, match, two); err != nil || got != nil {
		t.Errorf("BlitzThread on the other endpoint = %+v, %v; want none", got, err)
	}
	if err := a.MarkBlitzThreadPosted(ctx, match, one, "500000000000000001", reportsAt); err != nil {
		t.Fatalf("MarkBlitzThreadPosted: %v", err)
	}
	if got, err := a.BlitzThread(ctx, match, one); err != nil || got == nil || !got.Posted || got.MessageID != "500000000000000001" {
		t.Errorf("BlitzThread once posted = %+v, %v; want posted, with its message", got, err)
	}
}

func TestMigration103DropsOnlyBotChannelsAndThreadsGoingDown(t *testing.T) {
	pool, a := blitzFixture(t)
	endpoint(t, a, "Posts", 1, "blitz_round")
	botChannel(t, a, "Threads", 2)
	migrateTo(t, pool, 102, true)
	if got := rowsOf(t, pool, "SELECT name FROM notification_endpoints"); !slices.Equal(got, []string{"Posts"}) {
		t.Errorf("endpoints at 102 = %q, want only the webhook", got)
	}
	migrateTo(t, pool, 103, false)
}

// noticesSent sends what is due and returns each deadline or replaced-thread notice as "endpoint kind round", with
// the game's players for a deadline.
func noticesSent(t *testing.T, a *AdminStore) []string {
	t.Helper()
	var got []string
	if _, err := a.SendNotifications(context.Background(), time.Now().Add(time.Hour), 50, func(_ context.Context, n Notification, e NotificationEndpoint) error {
		switch n.Kind {
		case "blitz_deadline":
			got = append(got, fmt.Sprintf("%s %s %d %s v %s", e.Name, n.Kind, n.Round, n.A, n.B))
		case "blitz_replaced":
			got = append(got, fmt.Sprintf("%s %s %d", e.Name, n.Kind, n.Round))
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	slices.Sort(got)
	return got
}

// moveDeadline sets round's deadline hours from now.
func moveDeadline(t *testing.T, a *AdminStore, round int64, hours int) {
	t.Helper()
	at := time.Now().Truncate(time.Hour).Add(time.Duration(hours) * time.Hour).UTC()
	if err := a.SetBlitzRoundDeadline(context.Background(), "us_open", "2026", round, at, "UTC"); err != nil {
		t.Fatalf("SetBlitzRoundDeadline: %v", err)
	}
}

func TestMovingADeadlineQueuesANoticeForEachThreadPosted(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	botChannel(t, a, "Threads", 1)
	round := publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	moveDeadline(t, a, round, 24)
	threadsSent(t, a, true)
	if got := noticesSent(t, a); len(got) != 0 {
		t.Errorf("notices with the first deadline = %q, want none", got)
	}
	moveDeadline(t, a, round, 48)
	if got, want := noticesSent(t, a), []string{"Threads blitz_deadline 1 Ann v Bob", "Threads blitz_deadline 1 Cy v Dee"}; !slices.Equal(got, want) {
		t.Errorf("notices once moved = %q, want %q", got, want)
	}
	moveDeadline(t, a, round, 48)
	if got := noticesSent(t, a); len(got) != 0 {
		t.Errorf("notices for the same deadline again = %q, want none", got)
	}
}

func TestMovingADeadlineBeforeThreadsArePostedQueuesNoNotice(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	botChannel(t, a, "Threads", 1)
	round := publishedRound(t, a, [2]int64{ids[0], ids[1]}, [2]int64{ids[2], ids[3]})
	moveDeadline(t, a, round, 24)
	moveDeadline(t, a, round, 48)
	if got := noticesSent(t, a); len(got) != 0 {
		t.Errorf("notices = %q, want none: the threads post the deadline as it is then", got)
	}
}

func TestARoundPairedAgainClosesTheThreadsOfGamesItDropped(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	ep := botChannel(t, a, "Threads", 1)
	first := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	moveDeadline(t, a, first, 24)
	threadsSent(t, a, true)
	deleteRound(t, a, first)
	same := publishedRound(t, a, [2]int64{bob, ann}, [2]int64{dee, cy})
	if got := noticesSent(t, a); len(got) != 0 {
		t.Errorf("notices for the same games = %q, want none", got)
	}
	deleteRound(t, a, same)
	other := publishedRound(t, a, [2]int64{ann, cy}, [2]int64{bob, dee})
	if got, want := noticesSent(t, a), []string{"Threads blitz_replaced 1"}; !slices.Equal(got, want) {
		t.Errorf("notices for other games = %q, want %q", got, want)
	}
	replaced, err := a.ReplacedBlitzThreads(ctx, other, ep)
	if err != nil || len(replaced) != 2 {
		t.Fatalf("ReplacedBlitzThreads = %q, %v; want the two dropped games' threads", replaced, err)
	}
	if err := a.CloseBlitzThread(ctx, ep, replaced[0], reportsAt); err != nil {
		t.Fatalf("CloseBlitzThread: %v", err)
	}
	if got, err := a.ReplacedBlitzThreads(ctx, other, ep); err != nil || !slices.Equal(got, replaced[1:]) {
		t.Errorf("ReplacedBlitzThreads once one is closed = %q, %v; want %q", got, err, replaced[1:])
	}
}

func TestAGameBackAfterItsThreadWasClosedGetsANewOne(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	ann, bob, cy, dee := ids[0], ids[1], ids[2], ids[3]
	ep := botChannel(t, a, "Threads", 1)
	first := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	moveDeadline(t, a, first, 24)
	threadsSent(t, a, true)
	deleteRound(t, a, first)
	other := publishedRound(t, a, [2]int64{ann, cy}, [2]int64{bob, dee})
	moveDeadline(t, a, other, 24)
	threadsSent(t, a, true)
	closed, err := a.ReplacedBlitzThreads(ctx, other, ep)
	if err != nil {
		t.Fatalf("ReplacedBlitzThreads: %v", err)
	}
	for _, id := range closed {
		if err := a.CloseBlitzThread(ctx, ep, id, reportsAt); err != nil {
			t.Fatalf("CloseBlitzThread: %v", err)
		}
	}
	deleteRound(t, a, other)
	back := publishedRound(t, a, [2]int64{ann, bob}, [2]int64{cy, dee})
	moveDeadline(t, a, back, 24)
	if got, want := threadsSent(t, a, true), []string{"Threads 1 Ann v Bob", "Threads 1 Cy v Dee"}; !slices.Equal(got, want) {
		t.Errorf("threads of games back = %q, want %q: their old ones are closed", got, want)
	}
}

func TestABotChannelIsRemindedOnlyInThreadsOfGamesUnplayed(t *testing.T) {
	a, _, ids := blitzWithEntrants(t)
	ctx := context.Background()
	now := time.Now()
	ep, err := a.AddNotificationEndpoint(ctx, NotificationEndpoint{Name: "Threads", Kind: "bot_channel", ChannelID: channelID(1), Format: "discord",
		Events: []string{"blitz_threads", "blitz_reminder"}, BlitzReminderHours: 24}, reportsAt)
	if err != nil {
		t.Fatalf("AddNotificationEndpoint: %v", err)
	}
	round := roundDue(t, a, ids, now.Add(60*time.Hour))
	threadsSent(t, a, true)
	if _, err := a.pool.Exec(ctx, "UPDATE blitz_matches SET result = 'a', decided_at = now(), ruling = 'forfeit' WHERE round_id = $1 AND entry_a = $2", round, ids[0]); err != nil {
		t.Fatalf("deciding Ann's game: %v", err)
	}
	if _, err := a.QueueBlitzReminders(ctx, now.Add(37*time.Hour)); err != nil {
		t.Fatalf("QueueBlitzReminders: %v", err)
	}
	var reminded []int64
	if _, err := a.SendNotifications(ctx, now.Add(37*time.Hour), 10, func(_ context.Context, n Notification, _ NotificationEndpoint) error {
		if n.Kind == "blitz_reminder" {
			reminded = append(reminded, n.RoundID)
		}
		return nil
	}); err != nil {
		t.Fatalf("SendNotifications: %v", err)
	}
	if !slices.Equal(reminded, []int64{round}) {
		t.Errorf("reminders sent = %v, want one for round %d", reminded, round)
	}
	due, err := a.UnremindedBlitzThreads(ctx, round, ep)
	if err != nil || len(due) != 1 {
		t.Fatalf("UnremindedBlitzThreads = %+v, %v; want only Cy and Dee's thread", due, err)
	}
	var cyDee int64
	if err := a.pool.QueryRow(ctx, "SELECT match_id FROM blitz_matches WHERE round_id = $1 AND entry_a = $2", round, ids[2]).Scan(&cyDee); err != nil {
		t.Fatalf("reading Cy and Dee's match: %v", err)
	}
	if due[0].MatchID != cyDee || due[0].ThreadID == "" {
		t.Errorf("thread to remind = %+v, want Cy and Dee's match %d with its thread", due[0], cyDee)
	}
	if err := a.MarkBlitzThreadReminded(ctx, ep, due[0].ThreadID, reportsAt); err != nil {
		t.Fatalf("MarkBlitzThreadReminded: %v", err)
	}
	if again, err := a.UnremindedBlitzThreads(ctx, round, ep); err != nil || len(again) != 0 {
		t.Errorf("UnremindedBlitzThreads once reminded = %+v, %v; want none", again, err)
	}
}

func TestABotChannelTakesOnlyThreadsAndTheirReminders(t *testing.T) {
	a, _, _ := outboxFixture(t)
	_, err := a.AddNotificationEndpoint(context.Background(), NotificationEndpoint{Name: "Threads", Kind: "bot_channel", ChannelID: channelID(1),
		Format: "discord", Events: []string{"blitz_threads", "blitz_reminder"}}, reportsAt)
	if err != nil {
		t.Errorf("a bot channel taking reminders: %v, want it kept", err)
	}
}
