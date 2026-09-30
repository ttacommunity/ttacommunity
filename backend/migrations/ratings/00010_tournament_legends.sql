-- The Hall of Fame's all-time table for a run: each player's titles and
-- podium medals across the four major circuits (World Championship,
-- International, Intermezzo, Royal League), in the order the page lists them.

-- +goose Up
CREATE TABLE tournament_legends (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    position integer NOT NULL CHECK (position > 0),
    player_name text NOT NULL,
    world_titles integer NOT NULL,
    international_titles integer NOT NULL,
    intermezzo_titles integer NOT NULL,
    royal_league_titles integer NOT NULL,
    gold integer NOT NULL,
    silver integer NOT NULL,
    bronze integer NOT NULL,
    PRIMARY KEY (run_id, position)
);

-- +goose Down
DROP TABLE tournament_legends;
