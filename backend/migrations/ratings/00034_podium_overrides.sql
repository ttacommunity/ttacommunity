-- A season's podium as admin sets it, by hand: it beats a recorded placing and
-- one derived from the games. Who set it is in admin_audit.

-- +goose Up
CREATE TABLE tournament_podium_overrides (
    tournament  text NOT NULL,
    season      text NOT NULL,
    place       int  NOT NULL CHECK (place BETWEEN 1 AND 3),
    player_name text NOT NULL,
    note        text NOT NULL DEFAULT '',
    PRIMARY KEY (tournament, season, place)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON tournament_podium_overrides TO ratings_admin;

-- +goose Down
REVOKE SELECT, INSERT, UPDATE, DELETE ON tournament_podium_overrides FROM ratings_admin;
DROP TABLE tournament_podium_overrides;
