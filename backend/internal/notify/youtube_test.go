package notify_test

import (
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

// youtubeLive is an invented YouTube stream going live, or ended after ran when ran is not zero.
func youtubeLive(ran time.Duration) store.Notification {
	n := store.Notification{Kind: "youtube_live", YouTube: store.YouTubeNotice{Channel: "Alice Casts", VideoID: "vid00000001",
		Title: "Friday *final*", ThumbnailURL: "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg", StartedAt: liveAt}}
	if ran > 0 {
		ended := liveAt.Add(ran)
		n.Kind, n.YouTube.EndedAt = "youtube_ended", &ended
	}
	return n
}

func TestAYouTubeStreamGoingLiveIsAnEmbedLinkingTheVideo(t *testing.T) {
	e := notify.Payload(youtubeLive(0), store.NotificationEndpoint{Look: store.EmbedLook{Colour: "#ff0000"}}, admin).Embeds[0]
	if e.Author == nil || e.Author.Name != "Alice Casts is live on YouTube" {
		t.Errorf("author = %+v, want Alice Casts is live on YouTube", e.Author)
	}
	if e.Title != `Friday \*final\*` || e.URL != "https://www.youtube.com/watch?v=vid00000001" || e.Color != 0xff0000 {
		t.Errorf("title, link, colour = %q %q %x, want the escaped title linking the video in the endpoint's colour", e.Title, e.URL, e.Color)
	}
	if e.Image == nil || !strings.HasPrefix(e.Image.URL, "https://i.ytimg.com/vi/vid00000001/hqdefault.jpg?t=") {
		t.Errorf("image = %+v, want the video's picture, its address made new for this stream", e.Image)
	}
	if e.Timestamp != "2026-11-18T08:00:00Z" {
		t.Errorf("timestamp = %q, want when it started", e.Timestamp)
	}
}

func TestAYouTubeStreamGoingLiveInAForumOpensAThreadNamedForIt(t *testing.T) {
	m := notify.Payload(youtubeLive(0), store.NotificationEndpoint{Forum: true}, admin)
	if m.ThreadName != "Alice Casts is live: Friday *final*" {
		t.Errorf("thread = %q, want the channel and title", m.ThreadName)
	}
}

func TestAYouTubeStreamEndedSaysHowLongItRanAndDropsThePicture(t *testing.T) {
	m := notify.Payload(youtubeLive(2*time.Hour+5*time.Minute), store.NotificationEndpoint{Forum: true}, admin)
	e := m.Embeds[0]
	if e.Author == nil || e.Author.Name != "Alice Casts was live on YouTube" || e.Description != "Streamed for 2 h 5 min" || e.Image != nil {
		t.Errorf("embed = %+v, want was live, how long and no picture", e)
	}
	if m.Content != "" || m.ThreadName != "" {
		t.Errorf("content, thread = %q %q, want neither: an edit keeps the message's text", m.Content, m.ThreadName)
	}
}
