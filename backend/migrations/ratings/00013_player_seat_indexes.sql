-- Index-only reads for a player's seats: their seats, the games they were rated in during a run, each game's winners,
-- and which games were played. The new indexes are built before the old ones go, so reads carry on while they build.
-- +goose Up
CREATE INDEX seats_player_games ON seats (player_name) INCLUDE (game_id, starting_position, seat, place, culture);
CREATE INDEX seats_winners ON seats (game_id) WHERE place = 1;
CREATE INDEX games_played ON games (game_id) WHERE state = 'played';
CREATE INDEX pairwise_matches_player_a_games ON pairwise_matches (run_id, player_a) INCLUDE (match_id, player_count);
CREATE INDEX pairwise_matches_player_b_games ON pairwise_matches (run_id, player_b) INCLUDE (match_id, player_count);
DROP INDEX seats_player_name;
DROP INDEX pairwise_matches_player_a;
DROP INDEX pairwise_matches_player_b;

-- +goose Down
CREATE INDEX seats_player_name ON seats (player_name);
CREATE INDEX pairwise_matches_player_a ON pairwise_matches (run_id, player_a);
CREATE INDEX pairwise_matches_player_b ON pairwise_matches (run_id, player_b);
DROP INDEX seats_player_games;
DROP INDEX seats_winners;
DROP INDEX games_played;
DROP INDEX pairwise_matches_player_a_games;
DROP INDEX pairwise_matches_player_b_games;
