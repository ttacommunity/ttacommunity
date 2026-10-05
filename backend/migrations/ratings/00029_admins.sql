-- Who may use admin, in which role, and a record of every change they make.
-- ratings_admin is the role of the API's admin login: it reads everything and
-- writes only what admin edits. Later migrations grant it more as admin grows.

-- +goose Up
CREATE TABLE admins (
    discord_id text PRIMARY KEY CHECK (discord_id ~ '^[0-9]{5,25}$'),
    name       text NOT NULL CHECK (name <> ''),
    role       text NOT NULL CHECK (role IN ('admin', 'organiser')),
    added_at   timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE admin_audit (
    audit_id   bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    at         timestamptz NOT NULL DEFAULT now(),
    discord_id text NOT NULL,
    action     text NOT NULL CHECK (action <> ''),
    target     text NOT NULL,
    detail     jsonb NOT NULL DEFAULT '{}'
);

-- Discord ids are personal data: the public read role does not see them.
REVOKE SELECT ON admins, admin_audit FROM ratings_read;

-- +goose StatementBegin
-- Roles are per server: a database owner without CREATEROLE needs it made first (README, cluster database).
DO $$
BEGIN
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ratings_admin') THEN
        CREATE ROLE ratings_admin NOLOGIN;
    END IF;
END
$$;
-- +goose StatementEnd
GRANT USAGE ON SCHEMA public TO ratings_admin;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ratings_admin;
GRANT INSERT ON admin_audit TO ratings_admin;

-- +goose Down
-- The role stays: other databases on the server may use it.
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ratings_admin;
REVOKE USAGE ON SCHEMA public FROM ratings_admin;
DROP TABLE admin_audit;
DROP TABLE admins;
