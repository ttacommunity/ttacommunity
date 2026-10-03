-- A player's title as admin sets it, replacing the one the engine derives from
-- the next run; '' takes the title away. Who set it is in admin_audit.

-- +goose Up
CREATE TABLE player_title_overrides (
    player_name text PRIMARY KEY,
    title       text NOT NULL CHECK (title IN ('SGM', 'GM', 'M', 'P', 'G', 'S', 'B', 'W', '')),
    reason      text NOT NULL CHECK (reason <> '')
);
GRANT SELECT, INSERT, UPDATE, DELETE ON player_title_overrides TO ratings_admin;

-- +goose Down
REVOKE SELECT, INSERT, UPDATE, DELETE ON player_title_overrides FROM ratings_admin;
DROP TABLE player_title_overrides;
