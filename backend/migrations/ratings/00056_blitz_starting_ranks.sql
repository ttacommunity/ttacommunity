-- How a blitz edition's Dutch pairing orders the players: starting ranks by rating (FIDE's way) or by lot, and
-- each entry's starting rank, fixed when round 1 is paired and cleared with the edition's rounds. The later rounds
-- may be paired at random too, as round 1 could.
-- +goose Up
ALTER TABLE blitz_editions
    ADD COLUMN pairing_ranks text NOT NULL DEFAULT 'rating'
        CONSTRAINT blitz_editions_pairing_ranks_check CHECK (pairing_ranks IN ('rating', 'lot')),
    DROP CONSTRAINT blitz_editions_pairing_later_check,
    ADD CONSTRAINT blitz_editions_pairing_later_check CHECK (pairing_later IN ('dutch', 'dutch-timezone', 'timezone', 'rating', 'random'));
ALTER TABLE blitz_entries
    ADD COLUMN starting_rank integer
        CONSTRAINT blitz_entries_starting_rank_check CHECK (starting_rank > 0);

-- +goose Down
ALTER TABLE blitz_entries DROP COLUMN starting_rank;
UPDATE blitz_editions SET pairing_later = 'dutch' WHERE pairing_later = 'random';
ALTER TABLE blitz_editions
    DROP COLUMN pairing_ranks,
    DROP CONSTRAINT blitz_editions_pairing_later_check,
    ADD CONSTRAINT blitz_editions_pairing_later_check CHECK (pairing_later IN ('dutch', 'dutch-timezone', 'timezone', 'rating'));
