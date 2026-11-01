-- Discord threads for blitz rounds again: each edition's organisers (members, pinged in every thread's welcome
-- message) and its own text for the thread messages, either part empty to leave it out; an edition without a text
-- row uses the default. Neither is shown on the site.
-- +goose Up
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

-- +goose Down
DROP TABLE blitz_thread_texts;
DROP TABLE blitz_organisers;
