-- Community members: everyone who takes part in anything we run, keyed by their TTA username, with the CGE account
-- and ratings player attached as they become known, Discord, email, country, time zone and a community ban. Blitz
-- entrants become references to members; their copied name, Discord name and zone move here.
-- +goose Up
CREATE TABLE members (
    member_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tta_name text NOT NULL CHECK (tta_name <> '' AND char_length(tta_name) <= 100),
    cge_user integer UNIQUE REFERENCES player_accounts,
    player_id bigint UNIQUE REFERENCES players,
    discord_id text UNIQUE CHECK (discord_id ~ '^[0-9]{17,20}$'),
    discord_name text NOT NULL DEFAULT '' CHECK (char_length(discord_name) <= 100),
    email text NOT NULL DEFAULT '' CHECK (char_length(email) <= 254),
    country_code text CHECK (country_code ~ '^[A-Z]{2}$'),
    zone text NOT NULL DEFAULT 'UTC' CHECK (zone <> ''),
    banned boolean NOT NULL DEFAULT false,
    ban_reason text NOT NULL DEFAULT '' CHECK (char_length(ban_reason) <= 1000),
    banned_at timestamptz,
    created_at timestamptz NOT NULL DEFAULT now(),
    CHECK (banned = (banned_at IS NOT NULL))
);
CREATE UNIQUE INDEX members_tta_name ON members (lower(tta_name));

-- One member per entrant name; the CGE account's spelling first, then the ratings', then the entry's.
INSERT INTO members (tta_name, discord_name, zone, cge_user, player_id)
SELECT DISTINCT ON (lower(e.name)) coalesce(a.name, p.name, e.name), e.discord_name, e.zone, a.cge_user, p.player_id
FROM blitz_entries e
LEFT JOIN LATERAL (SELECT player_id, name FROM players WHERE lower(name) = lower(e.name) ORDER BY player_id LIMIT 1) p ON true
LEFT JOIN LATERAL (SELECT cge_user, name FROM player_accounts WHERE lower(name) = lower(e.name) AND NOT deleted
                   ORDER BY cge_user LIMIT 1) a ON true
ORDER BY lower(e.name), e.joined_at, e.entry_id;

ALTER TABLE blitz_entries ADD COLUMN member_id bigint REFERENCES members;
UPDATE blitz_entries e SET member_id = m.member_id FROM members m WHERE lower(m.tta_name) = lower(e.name);
ALTER TABLE blitz_entries
    ALTER COLUMN member_id SET NOT NULL,
    DROP COLUMN name,
    DROP COLUMN discord_name,
    DROP COLUMN zone;
CREATE UNIQUE INDEX blitz_entries_member ON blitz_entries (tournament_id, member_id);
-- Nothing personal is left on an entry, so the site's role may read the whole table again.
GRANT SELECT ON blitz_entries TO ratings_read;

GRANT SELECT, INSERT, UPDATE, DELETE ON members TO ratings_admin;
-- The site's role never reads a Discord id or name, an email or a ban: the default whole-table SELECT is taken back.
REVOKE SELECT ON members FROM ratings_read;
GRANT SELECT (member_id, tta_name, cge_user, player_id, country_code, zone, created_at) ON members TO ratings_read;

-- +goose Down
ALTER TABLE blitz_entries
    ADD COLUMN name text,
    ADD COLUMN discord_name text NOT NULL DEFAULT '' CHECK (char_length(discord_name) <= 100),
    ADD COLUMN zone text;
UPDATE blitz_entries e SET name = m.tta_name, discord_name = m.discord_name, zone = m.zone FROM members m WHERE m.member_id = e.member_id;
ALTER TABLE blitz_entries
    ALTER COLUMN name SET NOT NULL,
    ALTER COLUMN zone SET NOT NULL,
    ADD CHECK (name <> '' AND char_length(name) <= 100),
    ADD CHECK (zone <> ''),
    DROP COLUMN member_id;
CREATE UNIQUE INDEX blitz_entries_name ON blitz_entries (tournament_id, lower(name));
REVOKE SELECT ON blitz_entries FROM ratings_read;
GRANT SELECT (entry_id, tournament_id, name, zone, status, joined_at) ON blitz_entries TO ratings_read;
DROP TABLE members;
