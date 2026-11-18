-- Reddit feeds go: Reddit no longer serves its RSS feeds. The feeds of kind reddit, their posts and subscriptions,
-- the kinds of post a feed sent, the reddit_post notification and an endpoint's "show the author" option, which only
-- Reddit posts used, are dropped. Down brings the structure back, empty.
-- +goose Up
DELETE FROM notification_outbox WHERE kind = 'reddit_post'
    OR (kind IN ('feed_failed', 'feed_recovered') AND feed_id IN (SELECT feed_id FROM feeds WHERE kind = 'reddit'));
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN reddit_post_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
               AND (kind IN ('feed_failed', 'feed_recovered')) = (feed_id IS NOT NULL)
               AND (kind = 'rss_item') = (rss_item_id IS NOT NULL));

DROP TABLE reddit_posts;
DELETE FROM feeds WHERE kind = 'reddit';
ALTER TABLE feeds
    DROP COLUMN post_kinds,
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('twitch', 'rss')),
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')
        OR (kind = 'rss' AND source ~ '^https://[^\s/?#@]+(/\S*)?$' AND char_length(source) <= 2000));

ALTER TABLE notification_endpoints DROP COLUMN show_author;

-- +goose Down
ALTER TABLE notification_endpoints ADD COLUMN show_author boolean NOT NULL DEFAULT true;

ALTER TABLE feeds
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'reddit' AND source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$')
        OR (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')
        OR (kind = 'rss' AND source ~ '^https://[^\s/?#@]+(/\S*)?$' AND char_length(source) <= 2000)),
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit', 'twitch', 'rss')),
    ADD COLUMN post_kinds text[] NOT NULL DEFAULT ARRAY['text', 'image', 'gallery', 'video', 'link']
        CONSTRAINT feeds_post_kinds_check CHECK (post_kinds <@ ARRAY['text', 'image', 'gallery', 'video', 'link']);

CREATE TABLE reddit_posts (
    reddit_post_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id bigint NOT NULL REFERENCES feeds ON DELETE CASCADE,
    post_id text NOT NULL CHECK (post_id ~ '^t3_[a-z0-9]{1,20}$'),
    kind text NOT NULL CONSTRAINT reddit_posts_kind_check CHECK (kind IN ('text', 'image', 'gallery', 'video', 'link')),
    title text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
    url text NOT NULL DEFAULT '' CHECK (url = '' OR url ~ '^https://'),
    author text NOT NULL DEFAULT '' CHECK (char_length(author) <= 30),
    body text NOT NULL DEFAULT '',
    image_url text NOT NULL DEFAULT '' CHECK (image_url = '' OR image_url ~ '^https://'),
    published_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL,
    UNIQUE (feed_id, post_id)
);
REVOKE SELECT ON reddit_posts FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON reddit_posts TO ratings_admin;
GRANT SELECT ON reddit_posts TO ratings_backup;

ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD COLUMN reddit_post_id bigint REFERENCES reddit_posts ON DELETE CASCADE,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'reddit_post', 'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
               AND (kind = 'reddit_post') = (reddit_post_id IS NOT NULL)
               AND (kind IN ('feed_failed', 'feed_recovered')) = (feed_id IS NOT NULL)
               AND (kind = 'rss_item') = (rss_item_id IS NOT NULL));
