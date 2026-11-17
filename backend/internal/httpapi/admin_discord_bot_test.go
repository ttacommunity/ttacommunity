package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"ttacommunity/backend/internal/store"
)

func (f *fakeAdmins) DiscordCommands(context.Context) ([]store.CommandSettings, error) {
	return []store.CommandSettings{{Name: "upsets", On: false, Reply: "always_shared", Format: 2, Results: 8}}, nil
}

func (f *fakeAdmins) SetDiscordCommand(_ context.Context, c store.CommandSettings) error {
	if c.Format == 5 {
		return store.ErrBadCommandSetting
	}
	f.notes = append(f.notes, fmt.Sprintf("%+v", c))
	return nil
}

func (f *fakeAdmins) BotRegistration(context.Context) (store.BotRegistration, error) {
	at := time.Date(2026, 11, 17, 6, 0, 0, 0, time.UTC)
	return store.BotRegistration{At: &at, Commands: []string{"rating", "roll"}}, nil
}

func (f *fakeAdmins) DiscordAppID() string { return "100000000000000003" }

func TestAnAdminSeesEachCommandWithItsSettings(t *testing.T) {
	rec := adminDoAs(t, &fakeAdmins{}, "111111111111111111", http.MethodGet, "/v1/admin/discord-bot", "", "")
	var bot DiscordBot
	if err := json.Unmarshal(rec.Body.Bytes(), &bot); err != nil || rec.Code != http.StatusOK {
		t.Fatalf("GET bot = %d %s, want 200", rec.Code, rec.Body)
	}
	got := map[string]DiscordBotCommand{}
	for _, c := range bot.Commands {
		got[c.Name] = c
	}
	tests := []struct {
		name, want string
	}{
		{"rating", "on true private format 0 results <nil>"},
		{"roll", "on true shared format <nil> results <nil>"},
		{"choose", "on true shared format <nil> results <nil>"},
		{"permutation", "on true shared format <nil> results <nil>"},
		{"rivals", "on true private format 0 results 5"},
		{"upsets", "on false always_shared format 2 results 8"},
	}
	for _, tt := range tests {
		c := got[tt.name]
		show := func(p *int) string {
			if p == nil {
				return "<nil>"
			}
			return fmt.Sprint(*p)
		}
		if s := fmt.Sprintf("on %t %s format %s results %s", c.On, c.Reply, show(c.Format), show(c.Results)); s != tt.want {
			t.Errorf("/%s = %s, want %s", tt.name, s, tt.want)
		}
	}
	if bot.RegisteredAt == nil || bot.InstallUrl == nil || !strings.Contains(*bot.InstallUrl, "client_id=100000000000000003") {
		t.Errorf("bot = %+v, want when it was registered and its install link", bot)
	}
}

func TestAnAdminSavesACommandsSettings(t *testing.T) {
	const site, owner = "https://ttac-admin.example", "111111111111111111"
	tests := []struct {
		name, command, body, want string
	}{
		{"a list command", "upsets", `{"on": false, "reply": "always_shared", "format": 3, "results": 10}`, "{Name:upsets On:false Reply:always_shared Format:3 Results:10}"},
		{"defaults for what is not given", "rivals", `{"on": true, "reply": "shared"}`, "{Name:rivals On:true Reply:shared Format:0 Results:5}"},
		{"a command without format or rows", "roll", `{"on": true, "reply": "always_private"}`, "{Name:roll On:true Reply:always_private Format:0 Results:0}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDoAs(t, f, owner, http.MethodPut, "/v1/admin/discord-bot/commands/"+tt.command, site, tt.body); rec.Code != http.StatusNoContent {
				t.Fatalf("PUT = %d %s, want 204", rec.Code, rec.Body)
			}
			if got := strings.Join(f.notes, "|"); got != tt.want {
				t.Errorf("saved %s, want %s", got, tt.want)
			}
			if want := owner + " discord.command.set " + tt.command; strings.Join(f.audits, "|") != want {
				t.Errorf("audits = %q, want %q", f.audits, want)
			}
		})
	}
}

func TestDiscordCommandChangesAreRefusedWhenTheyShouldBe(t *testing.T) {
	const site, owner, org = "https://ttac-admin.example", "111111111111111111", "222222222222222222"
	good := `{"on": true, "reply": "private"}`
	tests := []struct {
		name, who, method, path, origin, body string
		want                                  int
	}{
		{"an organiser looks", org, http.MethodGet, "/v1/admin/discord-bot", "", "", http.StatusForbidden},
		{"an organiser changes", org, http.MethodPut, "/v1/admin/discord-bot/commands/rating", site, good, http.StatusForbidden},
		{"another site", owner, http.MethodPut, "/v1/admin/discord-bot/commands/rating", "https://evil.example", good, http.StatusForbidden},
		{"a command the bot does not have", owner, http.MethodPut, "/v1/admin/discord-bot/commands/pizza", site, good, http.StatusNotFound},
		{"an unknown reply", owner, http.MethodPut, "/v1/admin/discord-bot/commands/rating", site, `{"on": true, "reply": "loud"}`, http.StatusBadRequest},
		{"a format for a command without", owner, http.MethodPut, "/v1/admin/discord-bot/commands/roll", site, `{"on": true, "reply": "private", "format": 2}`, http.StatusBadRequest},
		{"rows for a command that lists nothing", owner, http.MethodPut, "/v1/admin/discord-bot/commands/rating", site, `{"on": true, "reply": "private", "results": 3}`, http.StatusBadRequest},
		{"a value the database refuses", owner, http.MethodPut, "/v1/admin/discord-bot/commands/rating", site, `{"on": true, "reply": "private", "format": 5}`, http.StatusBadRequest},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := &fakeAdmins{}
			if rec := adminDoAs(t, f, tt.who, tt.method, tt.path, tt.origin, tt.body); rec.Code != tt.want {
				t.Errorf("%s %s = %d %s, want %d", tt.method, tt.path, rec.Code, rec.Body, tt.want)
			}
			if len(f.notes) != 0 || len(f.audits) != 0 {
				t.Errorf("changes = %q, audits = %q; want none", f.notes, f.audits)
			}
		})
	}
}
