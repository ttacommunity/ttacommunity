-- Players are looked up by name without case (lower(name) = lower($1)), once per row on the community
-- leaderboard and player pages; without this index each lookup reads every player.
-- +goose Up
CREATE INDEX players_lower_name ON players (lower(name));

-- +goose Down
DROP INDEX players_lower_name;
