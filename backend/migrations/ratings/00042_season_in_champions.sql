-- Whether a season's winner is listed among the tournament's champions, such as
-- the Hall of Fame's World Champions; a play-off can be left out.

-- +goose Up
ALTER TABLE tournament_seasons ADD COLUMN in_champions boolean NOT NULL DEFAULT true;
GRANT UPDATE (in_champions) ON tournament_seasons TO ratings_admin;

-- +goose Down
REVOKE UPDATE (in_champions) ON tournament_seasons FROM ratings_admin;
ALTER TABLE tournament_seasons DROP COLUMN in_champions;
