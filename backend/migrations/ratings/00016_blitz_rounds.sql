-- Blitz rounds and their matches: a round pairs every active entrant once (one may have the bye, entry_b null), is
-- a draft until published, and keeps the criteria and seed that made it, so a pairing can be reproduced. An entrant
-- may be pending: named but not yet confirmed, so neither paired nor shown on the site.
-- +goose Up
CREATE TABLE blitz_rounds (
    round_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    number integer NOT NULL CHECK (number > 0),
    kind text NOT NULL DEFAULT 'swiss' CHECK (kind IN ('swiss', 'knockout')),
    timezone boolean NOT NULL DEFAULT true,
    rating boolean NOT NULL DEFAULT false,
    seed bigint NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz,
    UNIQUE (tournament_id, number)
);
CREATE TABLE blitz_matches (
    match_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    round_id bigint NOT NULL REFERENCES blitz_rounds ON DELETE CASCADE,
    entry_a bigint NOT NULL REFERENCES blitz_entries,
    entry_b bigint REFERENCES blitz_entries,
    CHECK (entry_a <> entry_b)
);
CREATE INDEX blitz_matches_round ON blitz_matches (round_id);
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_rounds, blitz_matches TO ratings_admin;
GRANT SELECT ON blitz_rounds, blitz_matches TO ratings_read;
ALTER TABLE blitz_entries DROP CONSTRAINT blitz_entries_status_check;
ALTER TABLE blitz_entries ADD CONSTRAINT blitz_entries_status_check CHECK (status IN ('pending', 'active', 'dropped'));

-- +goose Down
UPDATE blitz_entries SET status = 'dropped' WHERE status = 'pending';
ALTER TABLE blitz_entries DROP CONSTRAINT blitz_entries_status_check;
ALTER TABLE blitz_entries ADD CONSTRAINT blitz_entries_status_check CHECK (status IN ('active', 'dropped'));
DROP TABLE blitz_matches;
DROP TABLE blitz_rounds;
