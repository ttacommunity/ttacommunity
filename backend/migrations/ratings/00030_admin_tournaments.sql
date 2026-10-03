-- Tournament settings admin edits: whether a tournament counts in the overall
-- community leaderboard, whether it is shown and rated, and its notes.
-- The leaderboard flag starts on for the tournaments the organisers count.

-- +goose Up
ALTER TABLE tournaments ADD COLUMN in_community_leaderboard boolean NOT NULL DEFAULT false;
UPDATE tournaments SET in_community_leaderboard = true
WHERE slug IN ('worlds', 'international', 'intermezzo', 'royal_league', 'wimbledon', 'australian_open',
               'survivors_cup', 'mercurial_ladder', 'sodium_ladder', 'eiffel_tower', 'leaderboard_trophy');

GRANT UPDATE (visible, in_community_leaderboard) ON tournaments TO ratings_admin;
GRANT INSERT, DELETE ON ranking_tournaments TO ratings_admin;
GRANT INSERT, UPDATE, DELETE ON tournament_season_notes TO ratings_admin;

-- +goose Down
REVOKE INSERT, UPDATE, DELETE ON tournament_season_notes FROM ratings_admin;
REVOKE INSERT, DELETE ON ranking_tournaments FROM ratings_admin;
REVOKE UPDATE (visible, in_community_leaderboard) ON tournaments FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN in_community_leaderboard;
