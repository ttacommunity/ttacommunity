-- Indexes for the player page's reads that scanned a whole run: a rank among one period's ratings (career peak,
-- matrix at a year's end), and a player's tournament records matched ignoring case.
-- +goose Up
CREATE INDEX rating_history_period ON rating_history (run_id, model_type, player_count, period_date, c_rating);
CREATE INDEX tournament_records_player_lower ON tournament_records (run_id, lower(player_name));

-- +goose Down
DROP INDEX tournament_records_player_lower;
DROP INDEX rating_history_period;
