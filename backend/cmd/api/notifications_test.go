package main

import (
	"context"
	"errors"
	"log/slog"
	"slices"
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// fakePoster records each call, answering edits with editErr.
type fakePoster struct {
	calls   []string
	editErr error
}

func (f *fakePoster) Send(context.Context, string, notify.WebhookMessage) error {
	f.calls = append(f.calls, "send")
	return nil
}

func (f *fakePoster) Post(context.Context, string, notify.WebhookMessage) (store.DiscordPost, error) {
	f.calls = append(f.calls, "post")
	return store.DiscordPost{MessageID: "100000000000000011", ChannelID: "100000000000000012"}, nil
}

func (f *fakePoster) Edit(_ context.Context, _ store.NotificationEndpoint, p store.DiscordPost, _ notify.WebhookMessage) error {
	f.calls = append(f.calls, "edit "+p.MessageID)
	return f.editErr
}

// fakeKeeper records the posts kept.
type fakeKeeper struct{ kept []string }

func (f *fakeKeeper) RecordTwitchPost(_ context.Context, ref, endpoint int64, p store.DiscordPost) error {
	f.kept = append(f.kept, p.MessageID)
	return nil
}

func TestDeliverPostsKeepsAndEditsAsEachNotificationNeeds(t *testing.T) {
	ended := time.Date(2026, 11, 18, 10, 0, 0, 0, time.UTC)
	wasLive := store.NotificationEndpoint{ID: 1, Feeds: []int64{7}, FeedOptions: []store.FeedOptions{{Feed: 7, WasLive: true}}}
	plain := store.NotificationEndpoint{ID: 2, Feeds: []int64{7}}
	posted := map[int64]store.DiscordPost{1: {MessageID: "100000000000000011"}}
	tests := []struct {
		name      string
		n         store.Notification
		e         store.NotificationEndpoint
		editErr   error
		wantCalls []string
		wantKept  []string
		wantErr   bool
	}{
		{"a stream going live, edited when it ends", store.Notification{Kind: "twitch_live", FeedID: 7, StreamRef: 3}, wasLive,
			nil, []string{"post"}, []string{"100000000000000011"}, false},
		{"a stream going live, left as posted", store.Notification{Kind: "twitch_live", FeedID: 7, StreamRef: 3}, plain,
			nil, []string{"send"}, nil, false},
		{"a stream ended", store.Notification{Kind: "twitch_ended", FeedID: 7, Twitch: store.TwitchNotice{EndedAt: &ended, Posts: posted}}, wasLive,
			nil, []string{"edit 100000000000000011"}, nil, false},
		{"a stream ended whose post was deleted", store.Notification{Kind: "twitch_ended", FeedID: 7, Twitch: store.TwitchNotice{EndedAt: &ended, Posts: posted}}, wasLive,
			&notify.RefusedError{Status: 404}, []string{"edit 100000000000000011"}, nil, false},
		{"a stream ended, Discord failing", store.Notification{Kind: "twitch_ended", FeedID: 7, Twitch: store.TwitchNotice{EndedAt: &ended, Posts: posted}}, wasLive,
			errors.New("down"), []string{"edit 100000000000000011"}, nil, true},
		{"anything else", store.Notification{Kind: "sign_up"}, plain, nil, []string{"send"}, nil, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			p, k := &fakePoster{editErr: tt.editErr}, &fakeKeeper{}
			err := deliver(context.Background(), k, p, tt.n, tt.e, "https://admin.example", slog.New(slog.DiscardHandler))
			if (err != nil) != tt.wantErr || !slices.Equal(p.calls, tt.wantCalls) || !slices.Equal(k.kept, tt.wantKept) {
				t.Errorf("deliver: %v, calls %q, kept %q; want error %t, %q, %q", err, p.calls, k.kept, tt.wantErr, tt.wantCalls, tt.wantKept)
			}
		})
	}
}
