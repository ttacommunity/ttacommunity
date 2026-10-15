-- Each run's record for every player by format and year: opponents met, wins, losses, draws and the last game, counted
-- once as the run is made instead of on every year leaderboard request. Runs already held are counted here.
-- +goose Up
CREATE TABLE player_year_records (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_count smallint NOT NULL,
    year integer NOT NULL,
    player_name text NOT NULL,
    opponents integer NOT NULL,
    wins integer NOT NULL,
    losses integer NOT NULL,
    draws integer NOT NULL,
    last_played date NOT NULL,
    PRIMARY KEY (run_id, player_count, year, player_name)
);
GRANT SELECT ON player_year_records TO ratings_read, ratings_admin;

-- +goose StatementBegin
CREATE FUNCTION derive_player_year_records(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO player_year_records (run_id, player_count, year, player_name, opponents, wins, losses, draws, last_played)
    SELECT id, player_count, extract(year FROM date)::integer, name, count(*),
           count(*) FILTER (WHERE outcome = 1), count(*) FILTER (WHERE outcome = 0),
           count(*) FILTER (WHERE outcome = 0.5), max(date)
    FROM (SELECT player_count, player_a AS name, outcome_a AS outcome, date FROM pairwise_matches WHERE run_id = id
          UNION ALL
          SELECT player_count, player_b, 1 - outcome_a, date FROM pairwise_matches WHERE run_id = id) mine
    GROUP BY player_count, extract(year FROM date), name
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION derive_player_year_records(bigint) FROM PUBLIC;

-- +goose StatementBegin
DO $$ BEGIN PERFORM derive_player_year_records(run_id) FROM rating_runs; END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION derive_player_year_records(bigint);
DROP TABLE player_year_records;
