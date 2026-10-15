-- Blitz tournaments: a series setting, each edition's format (Swiss rounds, then a knockout) and its entrants
-- (TTA username, Discord name and time zone). Rounds, matches and results come in later migrations.
-- +goose Up
ALTER TABLE series ADD COLUMN blitz boolean NOT NULL DEFAULT false;
GRANT UPDATE (blitz) ON series TO ratings_admin;

CREATE TABLE blitz_editions (
    tournament_id bigint PRIMARY KEY REFERENCES tournaments ON DELETE CASCADE,
    swiss_rounds integer NOT NULL CHECK (swiss_rounds BETWEEN 1 AND 30),
    knockout_size integer NOT NULL CHECK (knockout_size IN (0, 2, 4, 8, 16, 32))
);
CREATE TABLE blitz_entries (
    entry_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    discord_name text NOT NULL DEFAULT '' CHECK (char_length(discord_name) <= 100),
    zone text NOT NULL CHECK (zone <> ''),
    status text NOT NULL DEFAULT 'active' CHECK (status IN ('active', 'dropped')),
    joined_at timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX blitz_entries_name ON blitz_entries (tournament_id, lower(name));
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_editions, blitz_entries TO ratings_admin;
GRANT SELECT ON blitz_editions TO ratings_read;
-- The site's role never reads a Discord name: the whole-table SELECT every new table gets by default
-- (the first migration's default privileges) is taken back, and every column but that one granted.
REVOKE SELECT ON blitz_entries FROM ratings_read;
GRANT SELECT (entry_id, tournament_id, name, zone, status, joined_at) ON blitz_entries TO ratings_read;

-- +goose Down
DROP TABLE blitz_entries;
DROP TABLE blitz_editions;
ALTER TABLE series DROP COLUMN blitz;
