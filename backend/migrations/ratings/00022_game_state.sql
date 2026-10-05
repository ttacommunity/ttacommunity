-- Whether a game was played. A game every player timed out of is never
-- played: kept and shown, but not rated, given no points or place, and not
-- counted as a game played.

-- +goose Up
ALTER TABLE matches ADD COLUMN state text NOT NULL DEFAULT 'played' CHECK (state IN ('played', 'never_played'));

-- +goose Down
ALTER TABLE matches DROP COLUMN state;
