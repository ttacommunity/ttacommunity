package notify_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"ttacommunity/backend/internal/notify"
	"ttacommunity/backend/internal/store"
)

const thread = "100000000000000021"

func TestAnEndpointWithAThreadPostsIntoItWithoutOpeningOne(t *testing.T) {
	for _, forum := range []bool{true, false} {
		e := store.NotificationEndpoint{Forum: forum, ThreadID: thread}
		m := notify.Payload(store.Notification{Kind: "link_request", Name: "Eve", SignIns: []string{"discord"}}, e, admin)
		if m.ThreadID != thread || m.ThreadName != "" || len(m.Embeds) != 1 || m.Embeds[0].Description == "" {
			t.Errorf("forum %t: thread %q, name %q, embeds %+v; want into the thread, its embed whole", forum, m.ThreadID, m.ThreadName, m.Embeds)
		}
	}
}

func TestSendAndPostNameTheThread(t *testing.T) {
	var got []string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = append(got, r.URL.RawQuery)
		_, _ = io.WriteString(w, `{"id":"100000000000000011","channel_id":"`+thread+`"}`)
	}))
	t.Cleanup(s.Close)
	d := notify.NewDiscord(s.Client())
	m := notify.Payload(store.Notification{Kind: "link_request", Name: "Eve"}, store.NotificationEndpoint{Forum: true, ThreadID: thread}, admin)
	if err := d.Send(context.Background(), s.URL, m); err != nil {
		t.Fatalf("Send: %v", err)
	}
	if _, err := d.Post(context.Background(), s.URL, m); err != nil {
		t.Fatalf("Post: %v", err)
	}
	if want := []string{"thread_id=" + thread, "thread_id=" + thread + "&wait=true"}; len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("queries = %q, want %q", got, want)
	}
}

func TestEditFindsAMessageInTheEndpointsThread(t *testing.T) {
	var query string
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { query = r.URL.RawQuery }))
	t.Cleanup(s.Close)
	e := store.NotificationEndpoint{URL: s.URL, ThreadID: thread}
	if err := notify.NewDiscord(s.Client()).Edit(context.Background(), e, store.DiscordPost{MessageID: "100000000000000011", ChannelID: thread}, notify.WebhookMessage{}); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	if query != "thread_id="+thread {
		t.Errorf("query = %q, want thread_id=%s", query, thread)
	}
}
