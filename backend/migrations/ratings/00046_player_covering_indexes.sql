-- The per-player indexes carry what the player page reads, so a player's games, rivals, career and history are
-- read from the index alone, one player's entries together, instead of from rows scattered through the table.
-- The engine vacuums both tables after each run, which index-only reads need.
-- +goose Up
DROP INDEX pairwise_matches_player_a_games;
DROP INDEX pairwise_matches_player_b_games;
DROP INDEX rating_history_player;
CREATE INDEX pairwise_matches_player_a_games ON pairwise_matches (run_id, player_a)
    INCLUDE (match_id, player_count, date, outcome_a, player_b);
CREATE INDEX pairwise_matches_player_b_games ON pairwise_matches (run_id, player_b)
    INCLUDE (match_id, player_count, date, outcome_a, player_a);
CREATE INDEX rating_history_player ON rating_history (run_id, player_name, player_count, period_date)
    INCLUDE (model_type, rating, rd, c_rating);

-- +goose Down
DROP INDEX pairwise_matches_player_a_games;
DROP INDEX pairwise_matches_player_b_games;
DROP INDEX rating_history_player;
CREATE INDEX pairwise_matches_player_a_games ON pairwise_matches (run_id, player_a) INCLUDE (match_id, player_count);
CREATE INDEX pairwise_matches_player_b_games ON pairwise_matches (run_id, player_b) INCLUDE (match_id, player_count);
CREATE INDEX rating_history_player ON rating_history (run_id, player_name, player_count, period_date);
