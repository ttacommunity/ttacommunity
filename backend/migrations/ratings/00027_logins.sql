-- Player sign-in: each way a player signs in (a Discord account, for now) is a login, linked to a member when the
-- member's recorded Discord name matches or an admin or organiser approves the player's request; and the logins'
-- server-side sessions. The api's player login (role ratings_player) writes only these, and a linked member's
-- country and zone. The member's Discord id, never written, gives way to the logins.
-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    -- Roles are per server: another database may have made it, and an owner without CREATEROLE can migrate once
    -- it exists.
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ratings_player') THEN
        CREATE ROLE ratings_player NOLOGIN;
    END IF;
END $$;
-- +goose StatementEnd

CREATE TABLE logins (
    login_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    provider text NOT NULL CHECK (provider IN ('discord')),
    subject text NOT NULL CHECK (subject <> '' AND char_length(subject) <= 255),
    username text NOT NULL CHECK (username <> '' AND char_length(username) <= 100),
    display_name text NOT NULL DEFAULT '' CHECK (char_length(display_name) <= 100),
    member_id bigint REFERENCES members,
    linked_how text CHECK (linked_how IN ('match', 'approved')),
    linked_at timestamptz,
    -- Off once an admin unlinks it, so a wrong match by Discord name is not made again at the next sign-in.
    auto_link boolean NOT NULL DEFAULT true,
    created_at timestamptz NOT NULL,
    signed_in_at timestamptz NOT NULL,
    UNIQUE (provider, subject),
    CHECK (provider <> 'discord' OR subject ~ '^[0-9]{17,20}$'),
    CHECK ((member_id IS NULL) = (linked_how IS NULL) AND (member_id IS NULL) = (linked_at IS NULL))
);
-- A member has at most one login of each provider.
CREATE UNIQUE INDEX logins_member_provider ON logins (member_id, provider) WHERE member_id IS NOT NULL;

CREATE TABLE link_requests (
    request_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    login_id bigint NOT NULL REFERENCES logins ON DELETE CASCADE,
    tta_name text NOT NULL CHECK (tta_name <> '' AND char_length(tta_name) <= 100),
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'approved', 'refused')),
    reason text NOT NULL DEFAULT '' CHECK (char_length(reason) <= 500),
    requested_at timestamptz NOT NULL,
    decided_at timestamptz,
    CHECK ((status = 'pending') = (decided_at IS NULL))
);
CREATE UNIQUE INDEX link_requests_one_pending ON link_requests (login_id) WHERE status = 'pending';
CREATE INDEX link_requests_login ON link_requests (login_id, request_id);

CREATE TABLE player_sessions (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    login_id bigint NOT NULL REFERENCES logins ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL
);
CREATE INDEX player_sessions_login ON player_sessions (login_id);

ALTER TABLE members DROP COLUMN discord_id;

-- Personal data: the site's role gets none of it (new tables get its SELECT by default).
REVOKE ALL ON logins, link_requests, player_sessions FROM ratings_read;
GRANT SELECT, INSERT, UPDATE ON logins, link_requests TO ratings_player;
GRANT SELECT, INSERT, UPDATE, DELETE ON player_sessions TO ratings_player;
GRANT SELECT (member_id, tta_name, player_id, discord_name, country_code, zone, banned) ON members TO ratings_player;
GRANT UPDATE (country_code, zone) ON members TO ratings_player;
GRANT SELECT (player_id, name) ON players TO ratings_player;
-- A signed-in admin or organiser is shown the way to admin; admin itself still checks Access.
GRANT SELECT (discord_id, role) ON admins TO ratings_player;
GRANT SELECT, UPDATE, DELETE ON logins TO ratings_admin;
GRANT SELECT, UPDATE ON link_requests TO ratings_admin;

-- +goose Down
ALTER TABLE members ADD COLUMN discord_id text UNIQUE CHECK (discord_id ~ '^[0-9]{17,20}$');
DROP TABLE player_sessions, link_requests, logins;
REVOKE ALL ON members, players, admins FROM ratings_player;
