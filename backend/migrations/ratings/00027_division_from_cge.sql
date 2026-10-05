-- A league game named without its division takes the group CGE records for
-- it: a third source of a game's division, beside its name and a recorded one.

-- +goose Up
ALTER TABLE tournament_games DROP CONSTRAINT tournament_games_division_source_check;
ALTER TABLE tournament_games ADD CONSTRAINT tournament_games_division_source_check CHECK (division_source IN ('name', 'recorded', 'cge'));

-- +goose Down
DELETE FROM tournament_games WHERE division_source = 'cge';
ALTER TABLE tournament_games DROP CONSTRAINT tournament_games_division_source_check;
ALTER TABLE tournament_games ADD CONSTRAINT tournament_games_division_source_check CHECK (division_source IN ('name', 'recorded'));
