-- Each player's best title in each calendar year, as the engine derives it
-- for each run: the best championship division played that year, or World
-- Champion in the year they won it. The player page scores career years by it.

-- +goose Up
CREATE TABLE player_yearly_titles (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    year integer NOT NULL,
    title text NOT NULL,
    PRIMARY KEY (run_id, player_name, year)
);

-- +goose Down
DROP TABLE player_yearly_titles;
