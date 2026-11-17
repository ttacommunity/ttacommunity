//go:build integration

package store

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/pressly/goose/v3"
)

var botAt = time.Date(2026, 11, 17, 6, 0, 0, 0, time.UTC)

func TestACommandKeepsTheSettingsAdminSaves(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	a, p := NewAdminStore(asAdminLogin(t, pool)), NewPlayerStore(asPlayerLogin(t, pool))
	if _, found, err := p.CommandSettings(ctx, "upsets"); err != nil || found {
		t.Fatalf("before any save: found %t, %v; want none", found, err)
	}
	for _, want := range []CommandSettings{
		{Name: "upsets", On: false, Reply: "always_shared", Format: 2, Results: 10},
		{Name: "upsets", On: true, Reply: "private", Format: 0, Results: 5},
		{Name: "roll", On: true, Reply: "shared"},
	} {
		if err := a.SetDiscordCommand(ctx, want); err != nil {
			t.Fatalf("SetDiscordCommand(%+v): %v", want, err)
		}
		got, found, err := p.CommandSettings(ctx, want.Name)
		if err != nil || !found || got != want {
			t.Errorf("CommandSettings(%s) = %+v, %t, %v; want %+v", want.Name, got, found, err, want)
		}
	}
	list, err := a.DiscordCommands(ctx)
	if err != nil || len(list) != 2 || list[0].Name != "roll" {
		t.Errorf("DiscordCommands = %+v, %v; want roll and upsets, by name", list, err)
	}
}

func TestACommandsSettingsAreCheckedByTheDatabase(t *testing.T) {
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, migrated(t)))
	good := CommandSettings{Name: "upsets", On: true, Reply: "private", Format: 2, Results: 5}
	for _, tt := range []struct {
		name string
		edit func(c *CommandSettings)
	}{
		{"an unknown reply", func(c *CommandSettings) { c.Reply = "loud" }},
		{"an unknown format", func(c *CommandSettings) { c.Format = 5 }},
		{"too many rows", func(c *CommandSettings) { c.Results = 11 }},
		{"a name that is not a command's", func(c *CommandSettings) { c.Name = "Not a command" }},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := good
			tt.edit(&c)
			if err := a.SetDiscordCommand(ctx, c); !errors.Is(err, ErrBadCommandSetting) {
				t.Errorf("SetDiscordCommand(%+v): err = %v, want %v", c, err, ErrBadCommandSetting)
			}
		})
	}
}

func TestTheBotKeepsWhenItsCommandsWereLastRegistered(t *testing.T) {
	ctx := context.Background()
	a := NewAdminStore(asAdminLogin(t, migrated(t)))
	if r, err := a.BotRegistration(ctx); err != nil || r.At != nil {
		t.Fatalf("before any registration: %+v, %v; want none", r, err)
	}
	for _, at := range []time.Time{botAt, botAt.Add(time.Hour)} {
		if err := a.RecordCommandsRegistered(ctx, []string{"rating", "roll"}, at); err != nil {
			t.Fatalf("RecordCommandsRegistered: %v", err)
		}
	}
	r, err := a.BotRegistration(ctx)
	if err != nil || r.At == nil || !r.At.Equal(botAt.Add(time.Hour)) || !slices.Equal(r.Commands, []string{"rating", "roll"}) {
		t.Errorf("registration = %+v, %v; want the last, at %v, with rating and roll", r, err, botAt.Add(time.Hour))
	}
}

func TestTheDiscordBotMigrationGoesDownAndUp(t *testing.T) {
	ctx := context.Background()
	pool := migrated(t)
	if err := NewAdminStore(asAdminLogin(t, pool)).SetDiscordCommand(ctx, CommandSettings{Name: "roll", Reply: "shared"}); err != nil {
		t.Fatalf("SetDiscordCommand: %v", err)
	}
	for _, step := range []func(p *goose.Provider) error{
		func(p *goose.Provider) error { _, err := p.DownTo(ctx, 87); return err },
		func(p *goose.Provider) error { _, err := p.Up(ctx); return err },
	} {
		if err := withRatingsMigrations(pool, step); err != nil {
			t.Fatalf("migrating: %v", err)
		}
	}
	if _, found, err := NewPlayerStore(asPlayerLogin(t, pool)).CommandSettings(ctx, "roll"); err != nil || found {
		t.Errorf("after down and up: found %t, %v; want nothing saved", found, err)
	}
}
