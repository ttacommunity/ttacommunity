-- Rating results as versioned runs. The engine writes a whole run, and
-- current_run names the one readers see. See docs/architecture.md#rating-runs.

-- +goose Up
CREATE TABLE rating_runs (
    run_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    engine_version text NOT NULL,
    input_fingerprint text NOT NULL,
    status text NOT NULL DEFAULT 'running' CHECK (status IN ('running', 'complete', 'failed')),
    started_at timestamptz NOT NULL DEFAULT now(),
    finished_at timestamptz,
    published_at timestamptz,
    UNIQUE (run_id, status)
);

-- One row at most. The foreign key on (run_id, status) means only a complete
-- run can be current, and a current run cannot leave 'complete'.
CREATE TABLE current_run (
    only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row),
    run_id bigint NOT NULL,
    status text NOT NULL DEFAULT 'complete' CHECK (status = 'complete'),
    FOREIGN KEY (run_id, status) REFERENCES rating_runs (run_id, status)
);

CREATE TABLE pairwise_matches (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    pairwise_id bigint GENERATED ALWAYS AS IDENTITY,
    match_id bigint NOT NULL REFERENCES matches,
    date date NOT NULL,
    tournament text NOT NULL,
    player_count smallint NOT NULL,
    player_a text NOT NULL,
    player_b text NOT NULL,
    score_a double precision NOT NULL,
    score_b double precision NOT NULL,
    outcome_a double precision NOT NULL,
    weight double precision NOT NULL,
    glicko_eligible boolean NOT NULL,
    PRIMARY KEY (run_id, pairwise_id)
);

CREATE INDEX pairwise_matches_date ON pairwise_matches (run_id, date);
CREATE INDEX pairwise_matches_player_a ON pairwise_matches (run_id, player_a);
CREATE INDEX pairwise_matches_player_b ON pairwise_matches (run_id, player_b);
CREATE INDEX pairwise_matches_match ON pairwise_matches (match_id);

CREATE TABLE player_ratings (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    model_type text NOT NULL,
    player_count smallint NOT NULL,
    player_name text NOT NULL,
    rating double precision NOT NULL,
    rd double precision NOT NULL,
    sigma double precision NOT NULL,
    c_rating double precision NOT NULL,
    rank integer,
    rank_delta integer NOT NULL DEFAULT 0,
    games_played integer NOT NULL DEFAULT 0,
    opponents_count integer NOT NULL DEFAULT 0,
    wins integer NOT NULL DEFAULT 0,
    losses integer NOT NULL DEFAULT 0,
    draws integer NOT NULL DEFAULT 0,
    win_rate double precision NOT NULL DEFAULT 0,
    last_played date,
    PRIMARY KEY (run_id, model_type, player_count, player_name)
);

CREATE INDEX player_ratings_rank ON player_ratings (run_id, model_type, player_count, rank);

CREATE TABLE rating_history (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    model_type text NOT NULL,
    player_count smallint NOT NULL,
    player_name text NOT NULL,
    period_date date NOT NULL,
    rating double precision NOT NULL,
    rd double precision NOT NULL,
    c_rating double precision NOT NULL,
    PRIMARY KEY (run_id, model_type, player_count, player_name, period_date)
);

CREATE INDEX rating_history_player ON rating_history (run_id, player_name, player_count, period_date);

CREATE TABLE walk_forward_calibration (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    model_type text NOT NULL,
    player_count smallint NOT NULL,
    period_month text NOT NULL,
    matches_evaluated integer NOT NULL,
    brier_score double precision NOT NULL,
    ece double precision NOT NULL,
    log_loss double precision,
    accuracy double precision,
    bin_data jsonb,
    PRIMARY KEY (run_id, model_type, player_count, period_month)
);

CREATE TABLE standard_calibration (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    model_type text NOT NULL,
    player_count smallint NOT NULL,
    period_month text NOT NULL DEFAULT 'AGGREGATED',
    matches_evaluated integer NOT NULL,
    brier_score double precision NOT NULL,
    ece double precision NOT NULL,
    log_loss double precision,
    accuracy double precision,
    bin_data jsonb,
    PRIMARY KEY (run_id, model_type, player_count, period_month)
);

-- +goose Down
DROP TABLE standard_calibration;
DROP TABLE walk_forward_calibration;
DROP TABLE rating_history;
DROP TABLE player_ratings;
DROP TABLE pairwise_matches;
DROP TABLE current_run;
DROP TABLE rating_runs;
