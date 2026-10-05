-- A game whose result is in doubt (every score 0, with nothing to say who
-- won) is unconfirmed: kept and shown, but not rated, until it is confirmed.

-- +goose Up
ALTER TABLE matches DROP CONSTRAINT matches_state_check;
ALTER TABLE matches ADD CONSTRAINT matches_state_check CHECK (state IN ('played', 'never_played', 'unconfirmed'));

-- +goose Down
UPDATE matches SET state = 'played' WHERE state = 'unconfirmed';
ALTER TABLE matches DROP CONSTRAINT matches_state_check;
ALTER TABLE matches ADD CONSTRAINT matches_state_check CHECK (state IN ('played', 'never_played'));
