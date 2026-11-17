-- The Discord bot. discord_bot, one row: when the api last registered the commands with Discord, and which Discord
-- then held. discord_commands: a command's settings as admins chose them (on or off, who sees a reply, the format
-- it shows when the asker gives none, how many rows a list shows); a command without a row has its defaults. The
-- player login reads the settings to answer commands; admin's login writes both.
-- +goose Up
CREATE TABLE discord_bot (
    one boolean PRIMARY KEY DEFAULT true CHECK (one),
    registered_at timestamptz NOT NULL,
    registered text[] NOT NULL
);

CREATE TABLE discord_commands (
    name text PRIMARY KEY CHECK (name ~ '^[a-z0-9_-]{1,32}$'),
    enabled boolean NOT NULL,
    reply text NOT NULL CHECK (reply IN ('private', 'shared', 'always_private', 'always_shared')),
    format integer NOT NULL CHECK (format IN (0, 2, 3, 4)),
    -- How many rows a list shows; NULL for a command that lists nothing.
    results integer CHECK (results BETWEEN 1 AND 10)
);

REVOKE SELECT ON discord_bot, discord_commands FROM ratings_read;
GRANT SELECT ON discord_commands TO ratings_player;
GRANT SELECT, INSERT, UPDATE ON discord_bot, discord_commands TO ratings_admin;
GRANT SELECT ON discord_bot, discord_commands TO ratings_backup;

-- +goose Down
DROP TABLE discord_commands;
DROP TABLE discord_bot;
