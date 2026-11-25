-- A podium set in admin may have players sharing a place (three level on every tiebreak share second); each player is
-- on a season's podium once.
-- +goose Up
ALTER TABLE tournament_podium_overrides DROP CONSTRAINT tournament_podium_overrides_pkey;
ALTER TABLE tournament_podium_overrides ADD PRIMARY KEY (tournament, season, player_id);

-- +goose Down
-- Refused while a place is shared: remove that podium in admin first.
ALTER TABLE tournament_podium_overrides DROP CONSTRAINT tournament_podium_overrides_pkey;
ALTER TABLE tournament_podium_overrides ADD PRIMARY KEY (tournament, season, place);
