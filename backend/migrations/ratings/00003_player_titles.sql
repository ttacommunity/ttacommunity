-- Tournament titles (GM, M, P, G, S, B, W, WC) the engine derives for each
-- run. Only players with a title have a row.

-- +goose Up
CREATE TABLE player_titles (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    title text NOT NULL,
    title_count integer NOT NULL DEFAULT 0,
    badge_reason text NOT NULL,
    PRIMARY KEY (run_id, player_name)
);

-- +goose Down
DROP TABLE player_titles;
