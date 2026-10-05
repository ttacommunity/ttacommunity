-- A note can be shown beside a stage as well as a season: stage '' is the
-- season's own note.

-- +goose Up
ALTER TABLE tournament_season_notes ADD COLUMN stage text NOT NULL DEFAULT '';
ALTER TABLE tournament_season_notes DROP CONSTRAINT tournament_season_notes_pkey;
ALTER TABLE tournament_season_notes ADD PRIMARY KEY (tournament, season, stage);

-- +goose Down
DELETE FROM tournament_season_notes WHERE stage <> '';
ALTER TABLE tournament_season_notes DROP CONSTRAINT tournament_season_notes_pkey;
ALTER TABLE tournament_season_notes ADD PRIMARY KEY (tournament, season);
ALTER TABLE tournament_season_notes DROP COLUMN stage;
