-- An endpoint may post into an existing thread of its webhook's channel, such as a forum post, by the thread's id,
-- instead of to the channel ('' for none).
-- +goose Up
ALTER TABLE notification_endpoints ADD COLUMN thread_id text NOT NULL DEFAULT ''
    CHECK (thread_id = '' OR thread_id ~ '^[0-9]{17,20}$');

-- +goose Down
ALTER TABLE notification_endpoints DROP COLUMN thread_id;
