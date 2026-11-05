-- Each run's whole board at the start of each leaderboard delta window: every player rated by then, idle
-- months' RD growth included, which rating_history (players who played that month) cannot give. Runs already
-- held get an empty partition.
-- +goose Up
CREATE TABLE rating_windows (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    model_type text NOT NULL,
    player_count smallint NOT NULL,
    delta_window text NOT NULL CHECK (delta_window IN ('month', 'quarter', 'year')),
    period_date date NOT NULL,
    player_id bigint NOT NULL,
    rating double precision NOT NULL,
    rd double precision NOT NULL,
    c_rating double precision NOT NULL,
    rank integer NOT NULL CHECK (rank > 0),
    PRIMARY KEY (run_id, model_type, player_count, delta_window, player_id)
) PARTITION BY LIST (run_id);
GRANT SELECT ON rating_windows TO ratings_read, ratings_admin;

-- +goose StatementBegin
DO $$
DECLARE
    r bigint;
BEGIN
    FOR r IN SELECT run_id FROM rating_runs LOOP
        EXECUTE format('CREATE TABLE %I PARTITION OF rating_windows FOR VALUES IN (%s)', 'rating_windows_run' || r, r);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
DROP TABLE rating_windows;
