-- A player's country, as an ISO 3166-1 alpha-2 code. It belongs to the
-- player, not a run: the API joins it onto every run's ratings.

-- +goose Up
ALTER TABLE players ADD COLUMN country_code text CHECK (country_code ~ '^[A-Z]{2}$');

-- +goose Down
ALTER TABLE players DROP COLUMN country_code;
