-- A season still being played has standings but no podium yet: each standings row says whether its season is finished.
-- +goose Up
ALTER TABLE tournament_standings ADD COLUMN finished boolean NOT NULL DEFAULT true;

-- +goose Down
ALTER TABLE tournament_standings DROP COLUMN finished;
