-- Options for notifications. A Reddit feed: which kinds of post it sends. An endpoint: paused, the name and picture
-- its messages show, posting each as a new thread of a forum channel, a role pinged with each, how a Reddit post
-- looks in it (the embed's colour, how much of the post's text, the image, the author), and how its last post went.
-- +goose Up
ALTER TABLE reddit_feeds
    ADD COLUMN kinds text[] NOT NULL DEFAULT ARRAY['text', 'image', 'video', 'link']
        CHECK (kinds <@ ARRAY['text', 'image', 'video', 'link']);

ALTER TABLE notification_endpoints
    ADD COLUMN paused boolean NOT NULL DEFAULT false,
    -- Discord refuses a webhook name holding "discord" or "clyde".
    ADD COLUMN username text NOT NULL DEFAULT '' CHECK (char_length(username) <= 80 AND username !~* '(discord|clyde)'),
    ADD COLUMN avatar_url text NOT NULL DEFAULT '' CHECK (avatar_url = '' OR (avatar_url ~ '^https://\S+$' AND char_length(avatar_url) <= 500)),
    ADD COLUMN forum boolean NOT NULL DEFAULT false,
    ADD COLUMN role_id text NOT NULL DEFAULT '' CHECK (role_id = '' OR role_id ~ '^[0-9]{17,20}$'),
    ADD COLUMN colour text NOT NULL DEFAULT '#ff4500' CHECK (colour ~ '^#[0-9a-f]{6}$'),
    ADD COLUMN show_text text NOT NULL DEFAULT 'full' CHECK (show_text IN ('full', 'short', 'none')),
    ADD COLUMN show_image boolean NOT NULL DEFAULT true,
    ADD COLUMN show_author boolean NOT NULL DEFAULT true,
    ADD COLUMN last_sent_at timestamptz,
    ADD COLUMN last_error text CHECK (char_length(last_error) <= 200),
    ADD COLUMN last_error_at timestamptz;

-- +goose Down
ALTER TABLE notification_endpoints
    DROP COLUMN paused, DROP COLUMN username, DROP COLUMN avatar_url, DROP COLUMN forum, DROP COLUMN role_id,
    DROP COLUMN colour, DROP COLUMN show_text, DROP COLUMN show_image, DROP COLUMN show_author,
    DROP COLUMN last_sent_at, DROP COLUMN last_error, DROP COLUMN last_error_at;
ALTER TABLE reddit_feeds DROP COLUMN kinds;
