-- Each of a blitz edition's round groups (round 1, the later rounds) pairs by its own order: ratings, a lot or
-- chance. The one rank order becomes both, the edition records what fixed its starting ranks, and each Swiss round
-- records the order it was paired by (the rounds so far broke ties by rating, or were random throughout).
-- +goose Up
ALTER TABLE blitz_editions
    ADD COLUMN pairing_first_order text NOT NULL DEFAULT 'rating'
        CONSTRAINT blitz_editions_pairing_first_order_check CHECK (pairing_first_order IN ('rating', 'lot', 'random')),
    ADD COLUMN pairing_later_order text NOT NULL DEFAULT 'rating'
        CONSTRAINT blitz_editions_pairing_later_order_check CHECK (pairing_later_order IN ('rating', 'lot', 'random')),
    ADD COLUMN ranks_fixed_by text
        CONSTRAINT blitz_editions_ranks_fixed_by_check CHECK (ranks_fixed_by IN ('rating', 'lot'));
UPDATE blitz_editions SET pairing_first_order = pairing_ranks, pairing_later_order = pairing_ranks;
UPDATE blitz_editions e SET ranks_fixed_by = pairing_ranks
WHERE EXISTS (SELECT 1 FROM blitz_entries x WHERE x.tournament_id = e.tournament_id AND x.starting_rank IS NOT NULL);
ALTER TABLE blitz_editions DROP COLUMN pairing_ranks;

ALTER TABLE blitz_rounds ADD COLUMN pairing_order text
    CONSTRAINT blitz_rounds_pairing_order_check CHECK (pairing_order IN ('rating', 'lot', 'random'));
UPDATE blitz_rounds SET pairing_order = CASE WHEN method = 'random' THEN 'random' ELSE 'rating' END WHERE kind = 'swiss';
ALTER TABLE blitz_rounds ADD CONSTRAINT blitz_rounds_order_kind_check CHECK ((kind = 'swiss') = (pairing_order IS NOT NULL));

-- +goose Down
ALTER TABLE blitz_rounds DROP CONSTRAINT blitz_rounds_order_kind_check, DROP COLUMN pairing_order;
ALTER TABLE blitz_editions ADD COLUMN pairing_ranks text NOT NULL DEFAULT 'rating'
    CONSTRAINT blitz_editions_pairing_ranks_check CHECK (pairing_ranks IN ('rating', 'lot'));
UPDATE blitz_editions SET pairing_ranks = coalesce(ranks_fixed_by, CASE WHEN 'lot' IN (pairing_first_order, pairing_later_order) THEN 'lot' ELSE 'rating' END);
ALTER TABLE blitz_editions DROP COLUMN pairing_first_order, DROP COLUMN pairing_later_order, DROP COLUMN ranks_fixed_by;
