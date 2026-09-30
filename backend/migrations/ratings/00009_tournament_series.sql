-- Each series' game and edition counts for a run, as the series page states
-- them, and trophyboards for series that are not leagues: a board's order is
-- the engine's, and cups and ladders carry what their boards show instead of
-- (or beside) medals.

-- +goose Up
CREATE TABLE tournament_series (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    series text NOT NULL,
    games integer NOT NULL,
    editions integer NOT NULL,
    PRIMARY KEY (run_id, series)
);

ALTER TABLE tournament_boards
    ADD COLUMN position integer,
    ADD COLUMN details text,
    ADD COLUMN games integer,
    ADD COLUMN wins integer,
    ADD COLUMN win_rate double precision,
    ALTER COLUMN gold DROP NOT NULL,
    ALTER COLUMN silver DROP NOT NULL,
    ALTER COLUMN bronze DROP NOT NULL,
    ALTER COLUMN title DROP NOT NULL,
    ALTER COLUMN first_season DROP NOT NULL;

-- +goose Down
-- Boards of the new kinds have no medals or title, so they go first.
DELETE FROM tournament_boards WHERE gold IS NULL OR title IS NULL OR first_season IS NULL;
ALTER TABLE tournament_boards
    ALTER COLUMN first_season SET NOT NULL,
    ALTER COLUMN title SET NOT NULL,
    ALTER COLUMN bronze SET NOT NULL,
    ALTER COLUMN silver SET NOT NULL,
    ALTER COLUMN gold SET NOT NULL,
    DROP COLUMN win_rate,
    DROP COLUMN wins,
    DROP COLUMN games,
    DROP COLUMN details,
    DROP COLUMN position;
DROP TABLE tournament_series;
