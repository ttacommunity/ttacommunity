-- The lot drawn when round 1 is paired is the standings' last tie-break, so the edition's hidden draw goes.
-- +goose Up
ALTER TABLE blitz_editions DROP COLUMN tiebreak_seed;

-- +goose Down
ALTER TABLE blitz_editions ADD COLUMN tiebreak_seed bigint NOT NULL DEFAULT floor(random() * 2147483647)::bigint;
