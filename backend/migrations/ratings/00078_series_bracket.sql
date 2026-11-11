-- A series shown as a knockout bracket: each season's stages as rounds of groups or matches, from the first to the final.
-- +goose Up
ALTER TABLE series ADD COLUMN bracket boolean NOT NULL DEFAULT false;
GRANT UPDATE (bracket) ON series TO ratings_admin;

-- +goose Down
ALTER TABLE series DROP COLUMN bracket;
