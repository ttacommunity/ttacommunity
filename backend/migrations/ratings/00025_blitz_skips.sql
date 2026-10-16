-- Blitz skips: an entrant sitting out a Swiss round, by the round's number, so the round may be paired, deleted and
-- paired again without them. The site reads them to list who did not play a published round.
-- +goose Up
CREATE TABLE blitz_skips (
    entry_id bigint NOT NULL REFERENCES blitz_entries ON DELETE CASCADE,
    number integer NOT NULL CHECK (number > 0),
    PRIMARY KEY (entry_id, number)
);
GRANT SELECT, INSERT, DELETE ON blitz_skips TO ratings_admin;

-- +goose Down
DROP TABLE blitz_skips;
