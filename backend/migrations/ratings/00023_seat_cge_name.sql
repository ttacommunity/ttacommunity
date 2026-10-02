-- The name CGE shows for a seat, beside the name our match files give it:
-- they differ when the player was renamed, or the account deleted, since.

-- +goose Up
ALTER TABLE match_seats ADD COLUMN cge_name text;

-- +goose Down
ALTER TABLE match_seats DROP COLUMN cge_name;
