-- Each player's best title over their whole career, and the year they held
-- it, as the engine derives it for each run. Inactive players have one too.

-- +goose Up
CREATE TABLE player_peak_titles (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    peak_title text NOT NULL,
    peak_year integer,
    PRIMARY KEY (run_id, player_name)
);

-- +goose Down
DROP TABLE player_peak_titles;
