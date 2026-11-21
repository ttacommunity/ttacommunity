package main

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/blitz"
	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// fakeBot records each call as "what with what", knowing members by username, refusing to add the users in
// notMember with Discord's 404, and answering every call to step fail with errFailing.
type fakeBot struct {
	calls     []string
	members   map[string]string
	notMember []string
	fail      string
	lacking   error
	said      string
	edited    string
	pinged    []string
}

var errFailing = errors.New("discord is down")

func (f *fakeBot) answer(step string) error {
	f.calls = append(f.calls, step)
	if strings.HasPrefix(step, f.fail) && f.fail != "" {
		return errFailing
	}
	return nil
}

func (f *fakeBot) GuildOf(_ context.Context, channel string) (string, error) {
	return "400000000000000001", f.answer("guild of " + channel)
}

func (f *fakeBot) PrivateThread(_ context.Context, channel, name string) (string, error) {
	return "300000000000000001", f.answer("thread " + name)
}

func (f *fakeBot) FindMember(_ context.Context, _, username string) (string, error) {
	return f.members[username], f.answer("find " + username)
}

func (f *fakeBot) AddToThread(_ context.Context, _, user string) error {
	if err := f.answer("add " + user); err != nil {
		return err
	}
	if slices.Contains(f.notMember, user) {
		return &notify.RefusedError{Status: 404}
	}
	return nil
}

func (f *fakeBot) Say(_ context.Context, channel, text string, users []string) (string, error) {
	f.said, f.pinged = text, users
	return "500000000000000001", f.answer("say in " + channel)
}

func (f *fakeBot) Edit(_ context.Context, channel, msg, text string) error {
	f.edited = text
	return f.answer("edit " + msg + " in " + channel)
}

func (f *fakeBot) CheckChannel(_ context.Context, channel string) error {
	if err := f.answer("check " + channel); err != nil {
		return err
	}
	return f.lacking
}

func (f *fakeBot) CloseThread(_ context.Context, thread string) error {
	return f.answer("close " + thread)
}

// fakeThreads keeps one game's thread, as the store does, and a round's replaced threads.
type fakeThreads struct {
	thread   *store.BlitzDiscordThread
	replaced []string
	remind   []store.BlitzThreadToRemind
	kept     []string
}

func (f *fakeThreads) UnremindedBlitzThreads(context.Context, int64, int64) ([]store.BlitzThreadToRemind, error) {
	return f.remind, nil
}

func (f *fakeThreads) MarkBlitzThreadReminded(_ context.Context, _ int64, thread string, _ time.Time) error {
	f.kept = append(f.kept, "reminded "+thread)
	return nil
}

func (f *fakeThreads) ReplacedBlitzThreads(context.Context, int64, int64) ([]string, error) {
	return f.replaced, nil
}

func (f *fakeThreads) CloseBlitzThread(_ context.Context, _ int64, thread string, _ time.Time) error {
	f.replaced = slices.DeleteFunc(slices.Clone(f.replaced), func(id string) bool { return id == thread })
	f.kept = append(f.kept, "closed "+thread)
	return nil
}

func (f *fakeThreads) BlitzThread(context.Context, int64, int64) (*store.BlitzDiscordThread, error) {
	return f.thread, nil
}

func (f *fakeThreads) RecordBlitzThread(_ context.Context, _, _ int64, thread string, _ time.Time) error {
	f.thread = &store.BlitzDiscordThread{ID: thread}
	f.kept = append(f.kept, "made "+thread)
	return nil
}

func (f *fakeThreads) MarkBlitzThreadPosted(_ context.Context, _, _ int64, msg string, _ time.Time) error {
	f.thread.Posted, f.thread.MessageID = true, msg
	f.kept = append(f.kept, "posted")
	return nil
}

// threadOfAnnAndBob is round 1's thread for Ann (signed in with Discord) and Bob (by Discord username), organised by
// Carol (signed in).
func threadOfAnnAndBob() store.Notification {
	game := blitz.ThreadGame{Number: 1, A: blitz.Person{Name: "Ann", DiscordID: "100000000000000001"}, B: blitz.Person{Name: "Bob", Discord: "bob_plays"}}
	threads := blitz.Threads{Round: "Round 1", Deadline: time.Date(2026, 3, 4, 0, 0, 0, 0, time.UTC), Text: blitz.DefaultThreadText,
		Organisers: []blitz.Person{{Name: "Carol", DiscordID: "100000000000000003"}}, Games: []blitz.ThreadGame{game}}
	return store.Notification{Kind: "blitz_thread", MatchID: 9, Thread: store.BlitzThreadNews{Threads: threads, Game: game}}
}

var threadChannel = store.NotificationEndpoint{ID: 5, Kind: "bot_channel", ChannelID: "200000000000000001"}

func TestAThreadIsMadeTheirPeopleAddedAndItsMessagePosted(t *testing.T) {
	bot := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}}
	keep := &fakeThreads{}
	if err := deliverThread(context.Background(), keep, bot, threadOfAnnAndBob(), threadChannel, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("deliverThread: %v", err)
	}
	want := []string{"thread Round 1 · Game 1 · Ann vs Bob", "guild of 200000000000000001", "add 100000000000000001",
		"find bob_plays", "add 100000000000000002", "add 100000000000000003", "say in 300000000000000001"}
	if !slices.Equal(bot.calls, want) || !slices.Equal(keep.kept, []string{"made 300000000000000001", "posted"}) {
		t.Errorf("calls %q, kept %q; want %q, the thread kept then posted", bot.calls, keep.kept, want)
	}
	if !strings.Contains(bot.said, "Welcome <@100000000000000001>, <@100000000000000002>") || strings.Contains(bot.said, "please add") {
		t.Errorf("said %q, want both players mentioned and no one to add", bot.said)
	}
	if !slices.Equal(bot.pinged, []string{"100000000000000001", "100000000000000002", "100000000000000003"}) {
		t.Errorf("pinged %q, want the players and the organiser", bot.pinged)
	}
}

func TestAPlayerTheBotCannotReachIsLeftToTheOrganisers(t *testing.T) {
	tests := []struct {
		name string
		bot  *fakeBot
	}{
		{"not found by username", &fakeBot{}},
		{"not in the server", &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}, notMember: []string{"100000000000000002"}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if err := deliverThread(context.Background(), &fakeThreads{}, tt.bot, threadOfAnnAndBob(), threadChannel, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("deliverThread: %v", err)
			}
			if !strings.Contains(tt.bot.said, "Organisers: please add Bob (@bob_plays)") {
				t.Errorf("said %q, want the organisers asked to add Bob", tt.bot.said)
			}
			if slices.Contains(tt.bot.pinged, "100000000000000002") {
				t.Errorf("pinged %q, want Bob left out", tt.bot.pinged)
			}
		})
	}
}

func TestATriedThreadGoesOnFromWhereItStopped(t *testing.T) {
	tests := []struct {
		name      string
		thread    *store.BlitzDiscordThread
		wantFirst string
		wantCalls int
	}{
		{"made, not posted", &store.BlitzDiscordThread{ID: "300000000000000001"}, "guild of 200000000000000001", 6},
		{"posted", &store.BlitzDiscordThread{ID: "300000000000000001", Posted: true}, "", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}}
			if err := deliverThread(context.Background(), &fakeThreads{thread: tt.thread}, bot, threadOfAnnAndBob(), threadChannel, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("deliverThread: %v", err)
			}
			if len(bot.calls) != tt.wantCalls || tt.wantCalls > 0 && bot.calls[0] != tt.wantFirst {
				t.Errorf("calls %q, want %d starting %q: no second thread", bot.calls, tt.wantCalls, tt.wantFirst)
			}
		})
	}
}

func TestAFailingStepLeavesTheThreadToBeTriedAgain(t *testing.T) {
	tests := []struct {
		name, fail string
		wantKept   []string
	}{
		{"making the thread", "thread", nil},
		{"reading the server", "guild", []string{"made 300000000000000001"}},
		{"finding a player", "find", []string{"made 300000000000000001"}},
		{"adding someone", "add", []string{"made 300000000000000001"}},
		{"posting", "say", []string{"made 300000000000000001"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot, keep := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}, fail: tt.fail}, &fakeThreads{}
			err := deliverThread(context.Background(), keep, bot, threadOfAnnAndBob(), threadChannel, time.Now(), slog.New(slog.DiscardHandler))
			if !errors.Is(err, errFailing) || !slices.Equal(keep.kept, tt.wantKept) {
				t.Errorf("deliverThread: %v, kept %q; want Discord's failure, kept %q", err, keep.kept, tt.wantKept)
			}
		})
	}
}

func TestABotChannelWithoutTheBotIsRefused(t *testing.T) {
	err := deliver(context.Background(), &fakeKeeper{}, &fakePoster{}, nil, threadOfAnnAndBob(), threadChannel, "https://admin.example", time.Now(), slog.New(slog.DiscardHandler))
	if err == nil || !strings.Contains(err.Error(), "DISCORD_BOT_TOKEN") {
		t.Errorf("deliver without the bot: %v, want it to say the token is not set", err)
	}
}

func TestAMovedDeadlineEditsTheMessageAndTellsThePlayers(t *testing.T) {
	tests := []struct {
		name      string
		thread    *store.BlitzDiscordThread
		wantCalls int
	}{
		{"a thread posted", &store.BlitzDiscordThread{ID: "300000000000000001", Posted: true, MessageID: "500000000000000001"}, 7},
		{"a thread not posted yet", &store.BlitzDiscordThread{ID: "300000000000000001"}, 0},
		{"no thread", nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}}
			n := threadOfAnnAndBob()
			n.Kind = "blitz_deadline"
			if err := deliver(context.Background(), &fakeKeeper{fakeThreads: fakeThreads{thread: tt.thread}}, &fakePoster{}, bot, n, threadChannel,
				"https://admin.example", time.Now(), slog.New(slog.DiscardHandler)); err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if len(bot.calls) != tt.wantCalls {
				t.Fatalf("calls %q, want %d", bot.calls, tt.wantCalls)
			}
			if tt.wantCalls == 0 {
				return
			}
			if !slices.Contains(bot.calls, "edit 500000000000000001 in 300000000000000001") || !strings.Contains(bot.edited, "Welcome <@100000000000000001>") {
				t.Errorf("calls %q, edited %q; want the message edited with the new deadline", bot.calls, bot.edited)
			}
			if !strings.HasPrefix(bot.said, "<@100000000000000001>, <@100000000000000002>: the deadline") || !slices.Equal(bot.pinged, []string{"100000000000000001", "100000000000000002"}) {
				t.Errorf("said %q to %q, want the players told", bot.said, bot.pinged)
			}
		})
	}
}

func TestARoundPairedAgainClosesTheThreadsItDropped(t *testing.T) {
	bot := &fakeBot{}
	keep := &fakeKeeper{fakeThreads: fakeThreads{replaced: []string{"300000000000000007", "300000000000000008"}}}
	n := store.Notification{Kind: "blitz_replaced", RoundID: 4}
	if err := deliver(context.Background(), keep, &fakePoster{}, bot, n, threadChannel, "https://admin.example", time.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	want := []string{"say in 300000000000000007", "close 300000000000000007", "say in 300000000000000008", "close 300000000000000008"}
	if !slices.Equal(bot.calls, want) || !slices.Equal(keep.fakeThreads.kept, []string{"closed 300000000000000007", "closed 300000000000000008"}) || bot.said != blitz.ReplacedThread {
		t.Errorf("calls %q, kept %q, said %q; want each told and closed", bot.calls, keep.kept, bot.said)
	}
}

func TestDiscordLimitingTheBotStopsItsCallsForThePass(t *testing.T) {
	var limited bool
	calls := 0
	call := func(err error) func() error { return func() error { calls++; return err } }
	web := store.NotificationEndpoint{ID: 1, Kind: "webhook"}
	steps := []struct {
		name    string
		e       store.NotificationEndpoint
		answer  error
		wantRun bool
	}{
		{"a bot call answered", threadChannel, nil, true},
		{"a bot call limited", threadChannel, &notify.RefusedError{Status: 429}, true},
		{"the next bot call", threadChannel, nil, false},
		{"a webhook post", web, nil, true},
	}
	for _, s := range steps {
		before := calls
		err := paced(&limited, s.e, call(s.answer))
		if ran := calls > before; ran != s.wantRun {
			t.Errorf("%s: ran %t, want %t", s.name, ran, s.wantRun)
		}
		if !s.wantRun && err == nil {
			t.Errorf("%s: no error, want one so it is tried again", s.name)
		}
	}
}

func TestABotChannelsTestPostsOnlyWhenTheBotHasWhatItNeeds(t *testing.T) {
	needed := &notify.LackingError{Needed: []string{"View Channel"}}
	wanted := &notify.LackingError{Wanted: []string{"Manage Threads"}}
	tests := []struct {
		name      string
		lacking   error
		wantSaid  bool
		wantError error
	}{
		{"everything", nil, true, nil},
		{"only Manage Threads lacking", wanted, true, wanted},
		{"a permission it needs lacking", needed, false, needed},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			bot := &fakeBot{lacking: tt.lacking}
			err := adminDeps{bot: bot}.PostTestNotification(context.Background(), threadChannel)
			if said := slices.Contains(bot.calls, "say in 200000000000000001"); said != tt.wantSaid || !errors.Is(err, tt.wantError) {
				t.Errorf("posted %t, %v; want posted %t, %v", said, err, tt.wantSaid, tt.wantError)
			}
		})
	}
}

func TestANameThatIsNoUsernameIsNotSearchedFor(t *testing.T) {
	bot := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}}
	n := threadOfAnnAndBob()
	n.Thread.Game.B = blitz.Person{Name: "Bob", Discord: "Bob Plays#1234"}
	if err := deliverThread(context.Background(), &fakeThreads{}, bot, n, threadChannel, time.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("deliverThread: %v", err)
	}
	if slices.ContainsFunc(bot.calls, func(c string) bool { return strings.HasPrefix(c, "find") }) {
		t.Errorf("calls %q, want no search for a name that cannot be a username", bot.calls)
	}
	if strings.Contains(bot.said, "please add Bob") || !strings.Contains(bot.said, "Bob is one of the few people") {
		t.Errorf("said %q, want only the no-Discord sentence for Bob", bot.said)
	}
}

func TestAReminderPingsThePlayersOfEachThreadStillToPlay(t *testing.T) {
	n := threadOfAnnAndBob()
	n.Kind, n.RoundID = "blitz_reminder", 4
	n.Thread.ByMatch = map[int64]blitz.ThreadGame{9: n.Thread.Game}
	bot := &fakeBot{members: map[string]string{"bob_plays": "100000000000000002"}}
	keep := &fakeKeeper{fakeThreads: fakeThreads{remind: []store.BlitzThreadToRemind{{MatchID: 9, ThreadID: "300000000000000009"}, {MatchID: 10, ThreadID: "300000000000000010"}}}}
	if err := deliver(context.Background(), keep, &fakePoster{}, bot, n, threadChannel, "https://admin.example", time.Now(), slog.New(slog.DiscardHandler)); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if !strings.HasPrefix(bot.said, "<@100000000000000001>, <@100000000000000002>: a reminder") || !slices.Equal(bot.pinged, []string{"100000000000000001", "100000000000000002"}) {
		t.Errorf("said %q to %q, want the players reminded", bot.said, bot.pinged)
	}
	if !slices.Contains(bot.calls, "say in 300000000000000009") || slices.Contains(bot.calls, "say in 300000000000000010") {
		t.Errorf("calls %q, want only the thread of a game still to play", bot.calls)
	}
	if !slices.Equal(keep.fakeThreads.kept, []string{"reminded 300000000000000009"}) {
		t.Errorf("kept %q, want only that thread reminded", keep.fakeThreads.kept)
	}
}
