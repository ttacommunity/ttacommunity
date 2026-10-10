-- Each run's catalogue spans: the games, players and first and last game of every series, season and stage, counted
-- once as the run is made instead of on every catalogue request. A row with no season is the whole series; with no
-- stage, the whole season. Runs already held are counted here, from today's games.
-- +goose Up
CREATE TABLE catalogue_spans (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    series_id bigint NOT NULL,
    tournament_id bigint,
    stage_id bigint,
    games integer NOT NULL,
    players integer NOT NULL,
    first_game date NOT NULL,
    last_game date NOT NULL,
    UNIQUE NULLS NOT DISTINCT (run_id, series_id, tournament_id, stage_id)
);
GRANT SELECT ON catalogue_spans TO ratings_read, ratings_admin;

-- +goose StatementBegin
CREATE FUNCTION derive_catalogue_spans(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO catalogue_spans (run_id, series_id, tournament_id, stage_id, games, players, first_game, last_game)
    SELECT id, s.series_id, s.tournament_id, st.stage_id,
           count(DISTINCT g.game_id), count(DISTINCT p.player_name), min(g.date), max(g.date)
    FROM games g
    JOIN seats p USING (game_id)
    JOIN groups gr USING (group_id)
    JOIN stages st USING (stage_id)
    JOIN tournaments s USING (tournament_id)
    WHERE g.state = 'played'
    GROUP BY GROUPING SETS ((s.series_id), (s.series_id, s.tournament_id), (s.series_id, s.tournament_id, st.stage_id))
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION derive_catalogue_spans(bigint) FROM PUBLIC;

-- +goose StatementBegin
DO $$ BEGIN PERFORM derive_catalogue_spans(run_id) FROM rating_runs; END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION derive_catalogue_spans(bigint);
DROP TABLE catalogue_spans;
