-- What of a rated tournament counts towards ratings: each season can be left
-- out, and only the player counts listed are rated. A game left out still
-- shows in its tournament's history, marked not rated.

-- +goose Up
ALTER TABLE tournament_seasons ADD COLUMN rated boolean NOT NULL DEFAULT true;
ALTER TABLE tournaments ADD COLUMN rated_player_counts int[] NOT NULL DEFAULT '{2,3,4}'
    CHECK (rated_player_counts <@ '{2,3,4}');
GRANT UPDATE (rated) ON tournament_seasons TO ratings_admin;
GRANT UPDATE (rated_player_counts) ON tournaments TO ratings_admin;

-- +goose Down
REVOKE UPDATE (rated_player_counts) ON tournaments FROM ratings_admin;
REVOKE UPDATE (rated) ON tournament_seasons FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN rated_player_counts;
ALTER TABLE tournament_seasons DROP COLUMN rated;
