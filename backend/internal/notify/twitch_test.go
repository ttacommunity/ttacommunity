package notify_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

var liveAt = time.Date(2026, 11, 18, 8, 0, 0, 0, time.UTC)

// live is an invented stream going live, or ended after ran when ran is not zero.
func live(ran time.Duration) store.Notification {
	n := store.Notification{Kind: "twitch_live", Twitch: store.TwitchNotice{Login: "some_streamer", DisplayName: "Some_Streamer",
		Title: "Friday *games*", Category: "Through the Ages", ThumbnailURL: "https://static-cdn.jtvnw.net/previews-ttv/live_user_some_streamer-1280x720.jpg",
		StartedAt: liveAt}}
	if ran > 0 {
		ended := liveAt.Add(ran)
		n.Kind, n.Twitch.EndedAt = "twitch_ended", &ended
	}
	return n
}

func TestAStreamGoingLiveIsAnEmbedLinkingTheStream(t *testing.T) {
	e := notify.Payload(live(0), store.NotificationEndpoint{Look: store.EmbedLook{Colour: "#9146ff"}}, admin).Embeds[0]
	if e.Author == nil || e.Author.Name != "Some_Streamer is live on Twitch" {
		t.Errorf("author = %+v, want Some_Streamer is live on Twitch", e.Author)
	}
	if e.Title != `Friday \*games\*` || e.URL != "https://www.twitch.tv/some_streamer" || e.Color != 0x9146ff {
		t.Errorf("title, link, colour = %q %q %x, want the escaped title linking the channel in the endpoint's colour", e.Title, e.URL, e.Color)
	}
	if len(e.Fields) != 1 || e.Fields[0].Name != "Category" || e.Fields[0].Value != "Through the Ages" {
		t.Errorf("fields = %+v, want the category", e.Fields)
	}
	if e.Image == nil || !strings.HasPrefix(e.Image.URL, "https://static-cdn.jtvnw.net/previews-ttv/live_user_some_streamer-1280x720.jpg?t=") {
		t.Errorf("image = %+v, want the preview, its address made new for this stream", e.Image)
	}
	if e.Timestamp != "2026-11-18T08:00:00Z" {
		t.Errorf("timestamp = %q, want when it started", e.Timestamp)
	}
}

func TestAStreamGoingLiveInAForumOpensAThreadNamedForIt(t *testing.T) {
	m := notify.Payload(live(0), store.NotificationEndpoint{Forum: true}, admin)
	if m.ThreadName != "Some_Streamer is live: Friday *games*" {
		t.Errorf("thread = %q, want the streamer and title", m.ThreadName)
	}
}

func TestAStreamEndedSaysHowLongItRanAndDropsTheLivePreview(t *testing.T) {
	tests := []struct {
		name string
		ran  time.Duration
		want string
	}{
		{"under an hour", 45 * time.Minute, "Streamed for 45 min"},
		{"hours and minutes", 2*time.Hour + 5*time.Minute, "Streamed for 2 h 5 min"},
		{"whole hours", 3 * time.Hour, "Streamed for 3 h"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := notify.Payload(live(tt.ran), store.NotificationEndpoint{Forum: true}, admin)
			e := m.Embeds[0]
			if e.Author == nil || e.Author.Name != "Some_Streamer was live on Twitch" || e.Description != tt.want || e.Image != nil {
				t.Errorf("embed = %+v, want was live, %q and no preview", e, tt.want)
			}
			if m.Content != "" || m.ThreadName != "" {
				t.Errorf("content, thread = %q %q, want neither: an edit keeps the message's text", m.Content, m.ThreadName)
			}
		})
	}
}

func TestPostReturnsTheMessageAndItsChannel(t *testing.T) {
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Query().Get("wait") != "true" {
			t.Errorf("request = %s %s, want a POST waiting for the message", r.Method, r.URL)
		}
		_, _ = io.WriteString(w, `{"id":"100000000000000011","channel_id":"100000000000000012"}`)
	}))
	t.Cleanup(s.Close)
	got, err := notify.NewDiscord(s.Client()).Post(context.Background(), s.URL, notify.Payload(live(0), store.NotificationEndpoint{}, admin))
	if err != nil || got != (store.DiscordPost{MessageID: "100000000000000011", ChannelID: "100000000000000012"}) {
		t.Errorf("Post = %+v, %v; want the message and channel", got, err)
	}
}

func TestEditPatchesTheMessageInItsThreadOnlyForAForum(t *testing.T) {
	tests := []struct {
		name       string
		forum      bool
		wantThread string
	}{
		{"a channel's message", false, ""},
		{"a forum thread's", true, "100000000000000012"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body map[string]any
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPatch || r.URL.Path != "/messages/100000000000000011" || r.URL.Query().Get("thread_id") != tt.wantThread {
					t.Errorf("request = %s %s, want a PATCH of the message, thread %q", r.Method, r.URL, tt.wantThread)
				}
				_ = json.NewDecoder(r.Body).Decode(&body)
				w.WriteHeader(http.StatusOK)
			}))
			t.Cleanup(s.Close)
			e := store.NotificationEndpoint{URL: s.URL, Forum: tt.forum}
			post := store.DiscordPost{MessageID: "100000000000000011", ChannelID: "100000000000000012"}
			if err := notify.NewDiscord(s.Client()).Edit(context.Background(), e, post, notify.Payload(live(time.Hour), e, admin)); err != nil {
				t.Fatalf("Edit: %v", err)
			}
			if _, ok := body["content"]; ok || body["embeds"] == nil {
				t.Errorf("edit = %v, want its embed and no content", body)
			}
			if mentions, _ := json.Marshal(body["allowed_mentions"]); string(mentions) != `{"parse":[]}` {
				t.Errorf("allowed_mentions = %s, want none", mentions)
			}
		})
	}
}
