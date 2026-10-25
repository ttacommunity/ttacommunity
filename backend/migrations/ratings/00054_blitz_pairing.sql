-- How a blitz edition's Swiss rounds are paired, round 1 and the rounds after it, and the method each Swiss round
-- was paired with, from the time-zone and rating switches it replaces. Editions so far keep the time-zone pairing
-- they had; a new edition uses FIDE's Dutch system. The organisers and thread text of the Discord threads go with
-- the threads. The Down brings back their shape, empty.
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
ALTER TABLE blitz_rounds
    ADD CONSTRAINT blitz_rounds_method_kind_check CHECK ((kind = 'swiss') = (method IS NOT NULL)),
    DROP COLUMN timezone,
    DROP COLUMN rating;

DROP TABLE blitz_thread_texts;
DROP TABLE blitz_organisers;

-- +goose Down
CREATE TABLE blitz_organisers (
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    member_id bigint NOT NULL REFERENCES members,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tournament_id, member_id)
);
GRANT SELECT, INSERT, DELETE ON blitz_organisers TO ratings_admin;
REVOKE SELECT ON blitz_organisers FROM ratings_read;
CREATE TABLE blitz_thread_texts (
    tournament_id bigint PRIMARY KEY REFERENCES blitz_editions ON DELETE CASCADE,
    message text NOT NULL CONSTRAINT blitz_thread_texts_message_check CHECK (char_length(message) <= 1500),
    no_discord text NOT NULL CONSTRAINT blitz_thread_texts_no_discord_check CHECK (char_length(no_discord) <= 300)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_thread_texts TO ratings_admin;
REVOKE SELECT ON blitz_thread_texts FROM ratings_read;
GRANT SELECT ON blitz_organisers, blitz_thread_texts TO ratings_backup;

ALTER TABLE blitz_rounds ADD COLUMN timezone boolean NOT NULL DEFAULT true, ADD COLUMN rating boolean NOT NULL DEFAULT false;
UPDATE blitz_rounds SET timezone = method IN ('dutch-timezone', 'timezone'), rating = method = 'rating' WHERE kind = 'swiss';
ALTER TABLE blitz_rounds DROP COLUMN method;
ALTER TABLE blitz_editions DROP COLUMN pairing_first, DROP COLUMN pairing_later;
