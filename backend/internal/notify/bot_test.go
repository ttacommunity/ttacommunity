package notify_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"slices"
	"strconv"
	"strings"
	"testing"

	"ttacommunity/backend/internal/notify"
)

// request is a call the fake Discord got: its method and path, and its body as JSON.
type request struct {
	Method, Path string
	Body         map[string]any
}

// fakeDiscord answers each call with answers' status and body for its "METHOD path", recording the calls in got;
// it refuses a call without the bot's token with 401.
func fakeDiscord(t *testing.T, answers map[string]string, got *[]request) *notify.Bot {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bot test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		req := request{Method: r.Method, Path: r.URL.RequestURI()}
		if b, _ := io.ReadAll(r.Body); len(b) > 0 {
			if err := json.Unmarshal(b, &req.Body); err != nil {
				t.Errorf("%s %s: body %q is not JSON", r.Method, r.URL, b)
			}
		}
		*got = append(*got, req)
		answer, ok := answers[r.Method+" "+r.URL.Path]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if answer == "" {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		_, _ = io.WriteString(w, answer)
	}))
	t.Cleanup(s.Close)
	return notify.NewBot(s.Client(), s.URL, "test-token")
}

func TestTheBotMakesAPrivateThread(t *testing.T) {
	var got []request
	bot := fakeDiscord(t, map[string]string{"POST /channels/200000000000000001/threads": `{"id":"300000000000000001"}`}, &got)
	id, err := bot.PrivateThread(context.Background(), "200000000000000001", "Round 1 · Game 1 · Ann vs Bob")
	if err != nil || id != "300000000000000001" {
		t.Fatalf("PrivateThread = %q, %v; want the thread's id", id, err)
	}
	want := map[string]any{"name": "Round 1 · Game 1 · Ann vs Bob", "type": float64(12), "invitable": false, "auto_archive_duration": float64(10080)}
	if len(got) != 1 || got[0].Method != "POST" || !equalJSON(got[0].Body, want) {
		t.Errorf("sent %+v, want a private thread no player can invite to, archived after a week", got)
	}
}

func TestTheBotFindsAMemberByTheirExactUsername(t *testing.T) {
	members := `[{"user":{"id":"100000000000000001","username":"ann_plays_more"}},{"user":{"id":"100000000000000002","username":"Ann_Plays"}}]`
	tests := []struct {
		name, username, want string
	}{
		{"one of several starting with it", "ann_plays", "100000000000000002"},
		{"none that is it", "ann", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []request
			bot := fakeDiscord(t, map[string]string{"GET /guilds/400000000000000001/members/search": members}, &got)
			id, err := bot.FindMember(context.Background(), "400000000000000001", tt.username)
			if err != nil || id != tt.want {
				t.Errorf("FindMember(%q) = %q, %v; want %q", tt.username, id, err, tt.want)
			}
			if len(got) != 1 || got[0].Path != "/guilds/400000000000000001/members/search?limit=100&query="+tt.username {
				t.Errorf("asked %+v, want a search for %q", got, tt.username)
			}
		})
	}
}

func TestTheBotAddsToAThreadAndPostsInIt(t *testing.T) {
	var got []request
	bot := fakeDiscord(t, map[string]string{
		"GET /channels/200000000000000001":                                   `{"id":"200000000000000001","guild_id":"400000000000000001"}`,
		"PUT /channels/300000000000000001/thread-members/100000000000000001": "",
		"POST /channels/300000000000000001/messages":                         `{"id":"500000000000000001"}`,
	}, &got)
	ctx := context.Background()
	if guild, err := bot.GuildOf(ctx, "200000000000000001"); err != nil || guild != "400000000000000001" {
		t.Errorf("GuildOf = %q, %v; want the channel's server", guild, err)
	}
	if err := bot.AddToThread(ctx, "300000000000000001", "100000000000000001"); err != nil {
		t.Errorf("AddToThread: %v", err)
	}
	if id, err := bot.Say(ctx, "300000000000000001", "Welcome <@100000000000000001>", []string{"100000000000000001"}); err != nil || id != "500000000000000001" {
		t.Errorf("Say = %q, %v; want the message's id", id, err)
	}
	want := map[string]any{"content": "Welcome <@100000000000000001>",
		"allowed_mentions": map[string]any{"parse": []any{}, "users": []any{"100000000000000001"}}}
	if len(got) != 3 || !equalJSON(got[2].Body, want) {
		t.Errorf("posted %+v, want the text, pinging only its players", got)
	}
}

func TestTheBotReportsDiscordsRefusalWithoutItsToken(t *testing.T) {
	var got []request
	bot := fakeDiscord(t, map[string]string{}, &got)
	err := bot.AddToThread(context.Background(), "300000000000000001", "100000000000000009")
	var refused *notify.RefusedError
	if !errors.As(err, &refused) || refused.Status != http.StatusNotFound {
		t.Errorf("AddToThread of someone not in the server: %v, want Discord's 404", err)
	}
	unauthorised := notify.NewBot(http.DefaultClient, "http://127.0.0.1:1", "secret-token")
	if _, err := unauthorised.GuildOf(context.Background(), "200000000000000001"); err == nil || strings.Contains(err.Error(), "secret-token") {
		t.Errorf("GuildOf with Discord down: %v, want an error that holds no token", err)
	}
}

// equalJSON says whether a and b are the same JSON object.
func equalJSON(a, b map[string]any) bool {
	x, _ := json.Marshal(a)
	y, _ := json.Marshal(b)
	return string(x) == string(y)
}

func TestTheBotEditsItsMessageWithoutPinging(t *testing.T) {
	var got []request
	bot := fakeDiscord(t, map[string]string{"PATCH /channels/300000000000000001/messages/500000000000000001": `{"id":"500000000000000001"}`}, &got)
	if err := bot.Edit(context.Background(), "300000000000000001", "500000000000000001", "Welcome again"); err != nil {
		t.Fatalf("Edit: %v", err)
	}
	want := map[string]any{"content": "Welcome again", "allowed_mentions": map[string]any{"parse": []any{}, "users": []any{}}}
	if len(got) != 1 || !equalJSON(got[0].Body, want) {
		t.Errorf("sent %+v, want the new text, pinging no one", got)
	}
}

func TestTheBotClosesAThreadLockingItWhenAllowed(t *testing.T) {
	tests := []struct {
		name   string
		status int
		want   []string
	}{
		{"allowed to lock", http.StatusOK, []string{`{"archived":true,"locked":true}`}},
		{"not allowed to lock", http.StatusForbidden, []string{`{"archived":true,"locked":true}`, `{"archived":true}`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var sent []string
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				b, _ := io.ReadAll(r.Body)
				sent = append(sent, string(b))
				if r.Method != http.MethodPatch || r.URL.Path != "/channels/300000000000000001" {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				if strings.Contains(string(b), "locked") {
					w.WriteHeader(tt.status)
				}
				_, _ = io.WriteString(w, `{"id":"300000000000000001"}`)
			}))
			defer s.Close()
			if err := notify.NewBot(s.Client(), s.URL, "test-token").CloseThread(context.Background(), "300000000000000001"); err != nil {
				t.Fatalf("CloseThread: %v", err)
			}
			if !slices.Equal(sent, tt.want) {
				t.Errorf("sent %q, want %q", sent, tt.want)
			}
		})
	}
}

func TestTheBotChecksWhatItMayDoInItsChannel(t *testing.T) {
	const everyone, botRole = "400000000000000001", "600000000000000001"
	answers := func(rolePerms string, flags int, overwrites string) map[string]string {
		return map[string]string{
			"GET /channels/200000000000000001":                          `{"guild_id":"400000000000000001","permission_overwrites":` + overwrites + `}`,
			"GET /users/@me":                                            `{"id":"100000000000000009"}`,
			"GET /applications/@me":                                     `{"flags":` + strconv.Itoa(flags) + `}`,
			"GET /guilds/400000000000000001":                            `{"owner_id":"100000000000000001","roles":[{"id":"` + everyone + `","permissions":"1024"},{"id":"` + botRole + `","permissions":"` + rolePerms + `"}]}`,
			"GET /guilds/400000000000000001/members/100000000000000009": `{"roles":["` + botRole + `"]}`,
		}
	}
	const threads = "343597383680" // Create Private Threads (1 << 36) and Send Messages in Threads (1 << 38)
	const all = "360777252864"     // and Manage Threads (1 << 34)
	tests := []struct {
		name       string
		answers    map[string]string
		wantNeeded []string
		wantWanted []string
	}{
		{"everything", answers(all, 1<<15, `[]`), nil, nil},
		{"no Manage Threads", answers(threads, 1<<15, `[]`), nil, []string{"Manage Threads"}},
		{"the channel hidden from @everyone, its role not let in", answers(all, 1<<14, `[{"id":"`+everyone+`","type":0,"allow":"0","deny":"1024"}]`),
			[]string{"View Channel"}, nil},
		{"no intent and no threads", answers("0", 0, `[]`),
			[]string{"Create Private Threads", "Send Messages in Threads", "the Server Members Intent (the app's Bot page)"}, []string{"Manage Threads"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var got []request
			err := fakeDiscord(t, tt.answers, &got).CheckChannel(context.Background(), "200000000000000001")
			var lacking *notify.LackingError
			if tt.wantNeeded == nil && tt.wantWanted == nil {
				if err != nil {
					t.Errorf("CheckChannel: %v, want nothing lacking", err)
				}
				return
			}
			if !errors.As(err, &lacking) || !slices.Equal(lacking.Needed, tt.wantNeeded) || !slices.Equal(lacking.Wanted, tt.wantWanted) {
				t.Errorf("CheckChannel: %v, want needed %q, wanted %q", err, tt.wantNeeded, tt.wantWanted)
			}
		})
	}
}

func TestTheBotPingsEachPersonOnce(t *testing.T) {
	var got []request
	bot := fakeDiscord(t, map[string]string{"POST /channels/300000000000000001/messages": `{"id":"500000000000000001"}`}, &got)
	if _, err := bot.Say(context.Background(), "300000000000000001", "Hello", []string{"100000000000000002", "100000000000000001", "100000000000000002"}); err != nil {
		t.Fatalf("Say: %v", err)
	}
	want := map[string]any{"parse": []any{}, "users": []any{"100000000000000001", "100000000000000002"}}
	if len(got) != 1 || !equalJSON(got[0].Body["allowed_mentions"].(map[string]any), want) {
		t.Errorf("sent %+v, want each user once: Discord refuses a repeat", got)
	}
}
