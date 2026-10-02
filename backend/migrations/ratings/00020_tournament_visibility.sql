-- The site shows a tournament that is rated (in a ranking), or that admin has
-- made visible though not rated; every other tournament is hidden.

-- +goose Up
ALTER TABLE tournaments ADD COLUMN visible boolean NOT NULL DEFAULT false;
ALTER TABLE cge_tournaments ADD COLUMN visible boolean NOT NULL DEFAULT false;

-- +goose Down
ALTER TABLE cge_tournaments DROP COLUMN visible;
ALTER TABLE tournaments DROP COLUMN visible;
