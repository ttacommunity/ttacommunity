-- Discord threads for blitz rounds: each edition's organisers (members, pinged in every thread's welcome message)
-- and each round's deadline to play by, with the time zone it was set in, so admin shows it back as it was typed.
-- +goose Up
CREATE TABLE blitz_organisers (
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    member_id bigint NOT NULL REFERENCES members,
    added_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (tournament_id, member_id)
);
GRANT SELECT, INSERT, DELETE ON blitz_organisers TO ratings_admin;
-- The site never shows who organises: the default whole-table SELECT is taken back.
REVOKE SELECT ON blitz_organisers FROM ratings_read;
ALTER TABLE blitz_rounds
    ADD COLUMN deadline timestamptz,
    ADD COLUMN deadline_zone text CHECK (deadline_zone <> ''),
    ADD CONSTRAINT blitz_rounds_deadline_check CHECK ((deadline IS NULL) = (deadline_zone IS NULL));

-- +goose Down
ALTER TABLE blitz_rounds DROP COLUMN deadline, DROP COLUMN deadline_zone;
DROP TABLE blitz_organisers;
