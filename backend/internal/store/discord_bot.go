// The Discord bot: each command's settings as admins chose them, and when the api last registered the commands
// with Discord. A command without saved settings has its defaults, which the api knows.
package store

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// CommandSettings are a command's settings: on or off; who sees a reply (private or shared unless the asker says
// otherwise, or always_private or always_shared); the format it shows when the asker gives none (0 for every
// format); and how many rows a list shows (0 for a command that lists nothing).
type CommandSettings struct {
	Name    string
	On      bool
	Reply   string
	Format  int
	Results int
}

// BotRegistration is when the api last registered the commands with Discord (nil for never), and which Discord
// then held.
type BotRegistration struct {
	At       *time.Time
	Commands []string
}

// ErrBadCommandSetting means settings the database refuses.
var ErrBadCommandSetting = errors.New("a command's reply is private, shared, always_private or always_shared, its format 0, 2, 3 or 4, and its rows 1 to 10")

const commandColumns = "name, enabled, reply, format, coalesce(results, 0)"

func scanCommand(row pgx.CollectableRow) (CommandSettings, error) {
	var c CommandSettings
	err := row.Scan(&c.Name, &c.On, &c.Reply, &c.Format, &c.Results)
	return c, err
}

// CommandSettings reads the settings saved for the command name, with false when none are.
func (p *PlayerStore) CommandSettings(ctx context.Context, name string) (CommandSettings, bool, error) {
	rows, err := p.pool.Query(ctx, "SELECT "+commandColumns+" FROM discord_commands WHERE name = $1", name)
	if err != nil {
		return CommandSettings{}, false, fmt.Errorf("reading a Discord command's settings: %w", err)
	}
	c, err := pgx.CollectExactlyOneRow(rows, scanCommand)
	if errors.Is(err, pgx.ErrNoRows) {
		return CommandSettings{}, false, nil
	}
	if err != nil {
		return CommandSettings{}, false, fmt.Errorf("reading a Discord command's settings: %w", err)
	}
	return c, true, nil
}

// DiscordCommands lists the settings saved for any command, by name.
func (a *AdminStore) DiscordCommands(ctx context.Context) ([]CommandSettings, error) {
	rows, err := a.pool.Query(ctx, "SELECT "+commandColumns+" FROM discord_commands ORDER BY name")
	if err != nil {
		return nil, fmt.Errorf("listing Discord commands' settings: %w", err)
	}
	list, err := pgx.CollectRows(rows, scanCommand)
	if err != nil {
		return nil, fmt.Errorf("listing Discord commands' settings: %w", err)
	}
	return list, nil
}

// SetDiscordCommand saves a command's settings, or returns ErrBadCommandSetting.
func (a *AdminStore) SetDiscordCommand(ctx context.Context, c CommandSettings) error {
	var results *int
	if c.Results != 0 {
		results = &c.Results
	}
	_, err := a.pool.Exec(ctx, `INSERT INTO discord_commands (name, enabled, reply, format, results) VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (name) DO UPDATE SET enabled = EXCLUDED.enabled, reply = EXCLUDED.reply, format = EXCLUDED.format, results = EXCLUDED.results`,
		c.Name, c.On, c.Reply, c.Format, results)
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23514" {
		return ErrBadCommandSetting
	}
	if err != nil {
		return fmt.Errorf("saving a Discord command's settings: %w", err)
	}
	return nil
}

// BotRegistration reads when the commands were last registered with Discord.
func (a *AdminStore) BotRegistration(ctx context.Context) (BotRegistration, error) {
	var r BotRegistration
	err := a.pool.QueryRow(ctx, "SELECT registered_at, registered FROM discord_bot").Scan(&r.At, &r.Commands)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return BotRegistration{}, fmt.Errorf("reading the Discord bot's registration: %w", err)
	}
	return r, nil
}

// RecordCommandsRegistered keeps that the api registered the commands names with Discord at now.
func (a *AdminStore) RecordCommandsRegistered(ctx context.Context, names []string, now time.Time) error {
	if _, err := a.pool.Exec(ctx, `INSERT INTO discord_bot (registered_at, registered) VALUES ($1, $2)
		ON CONFLICT (one) DO UPDATE SET registered_at = EXCLUDED.registered_at, registered = EXCLUDED.registered`, now, names); err != nil {
		return fmt.Errorf("recording the Discord bot's commands: %w", err)
	}
	return nil
}
