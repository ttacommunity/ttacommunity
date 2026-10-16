-- An edition's own text for its Discord thread messages: the welcome message and the sentence for a player
-- without Discord, placeholders and all. An edition without a row uses the default text.
-- +goose Up
CREATE TABLE blitz_thread_texts (
    tournament_id bigint PRIMARY KEY REFERENCES blitz_editions ON DELETE CASCADE,
    message text NOT NULL CONSTRAINT blitz_thread_texts_message_check CHECK (char_length(message) BETWEEN 1 AND 1500),
    no_discord text NOT NULL CONSTRAINT blitz_thread_texts_no_discord_check CHECK (char_length(no_discord) BETWEEN 1 AND 300)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_thread_texts TO ratings_admin;
-- The site never shows the thread text: the default whole-table SELECT is taken back.
REVOKE SELECT ON blitz_thread_texts FROM ratings_read;

-- +goose Down
DROP TABLE blitz_thread_texts;
