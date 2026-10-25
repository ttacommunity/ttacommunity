-- How a blitz edition's Swiss rounds are paired, round 1 and the rounds after it, and the method each Swiss round
-- was paired with, from its time-zone and rating switches for the rounds so far. Editions so far keep the time-zone
-- pairing they had; a new edition uses FIDE's Dutch system. It only adds, so the api before it keeps working while
-- it runs: the switches stay until a later migration.
-- +goose Up
ALTER TABLE blitz_editions
    ADD COLUMN pairing_first text NOT NULL DEFAULT 'timezone'
        CONSTRAINT blitz_editions_pairing_first_check CHECK (pairing_first IN ('dutch', 'dutch-timezone', 'timezone', 'random')),
    ADD COLUMN pairing_later text NOT NULL DEFAULT 'timezone'
        CONSTRAINT blitz_editions_pairing_later_check CHECK (pairing_later IN ('dutch', 'dutch-timezone', 'timezone', 'rating'));
ALTER TABLE blitz_editions ALTER COLUMN pairing_first SET DEFAULT 'dutch', ALTER COLUMN pairing_later SET DEFAULT 'dutch';

ALTER TABLE blitz_rounds ADD COLUMN method text
    CONSTRAINT blitz_rounds_method_check CHECK (method IN ('dutch', 'dutch-timezone', 'timezone', 'rating', 'random'));
UPDATE blitz_rounds SET method = CASE WHEN timezone THEN 'timezone' WHEN rating THEN 'rating' ELSE 'random' END
WHERE kind = 'swiss';
ALTER TABLE blitz_rounds ADD CONSTRAINT blitz_rounds_method_kind_check CHECK (kind = 'swiss' OR method IS NULL);

-- +goose Down
ALTER TABLE blitz_rounds DROP COLUMN method;
ALTER TABLE blitz_editions DROP COLUMN pairing_first, DROP COLUMN pairing_later;
