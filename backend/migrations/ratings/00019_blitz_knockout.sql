-- Blitz knockouts: each knockout match's place in the bracket and its players' seeds (null in a Swiss round). The
-- winners of slots 2k and 2k+1 meet in slot k of the next round; a drawn knockout game sends the higher seed on.
-- +goose Up
ALTER TABLE blitz_matches
    ADD COLUMN slot integer CHECK (slot >= 0),
    ADD COLUMN seed_a integer CHECK (seed_a >= 1),
    ADD COLUMN seed_b integer CHECK (seed_b >= 1),
    ADD CONSTRAINT blitz_matches_knockout_check CHECK ((slot IS NULL) = (seed_a IS NULL) AND (slot IS NULL) = (seed_b IS NULL));
CREATE UNIQUE INDEX blitz_matches_slot ON blitz_matches (round_id, slot);

-- +goose Down
DROP INDEX blitz_matches_slot;
ALTER TABLE blitz_matches DROP COLUMN slot, DROP COLUMN seed_a, DROP COLUMN seed_b;
