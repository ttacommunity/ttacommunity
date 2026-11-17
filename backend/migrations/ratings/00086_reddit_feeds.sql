-- Reddit feeds posted to Discord: the subreddits the reddit job reads, each post it has seen (so none is posted
-- twice), which endpoints take each feed, and the outbox's kinds: a new post, and a feed failing or reading again.
-- A feed not yet primed records what is there without posting it, so adding or resuming one posts no backlog. A
-- post's text is kept only until it is posted: what stays is its id and date.
-- +goose Up
CREATE TABLE reddit_feeds (
    feed_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    subreddit text NOT NULL CHECK (subreddit ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$'),
    paused boolean NOT NULL DEFAULT false,
    primed boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL,
    checked_at timestamptz,
    last_error text CHECK (char_length(last_error) <= 200),
    last_error_at timestamptz,
    -- Reads failed in a row since the last good one.
    failures integer NOT NULL DEFAULT 0 CHECK (failures >= 0)
);
CREATE UNIQUE INDEX reddit_feeds_subreddit ON reddit_feeds (lower(subreddit));

CREATE TABLE reddit_posts (
    reddit_post_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id bigint NOT NULL REFERENCES reddit_feeds ON DELETE CASCADE,
    post_id text NOT NULL CHECK (post_id ~ '^t3_[a-z0-9]{1,20}$'),
    kind text NOT NULL CHECK (kind IN ('text', 'image', 'video', 'link')),
    title text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
    url text NOT NULL DEFAULT '' CHECK (url = '' OR url ~ '^https://'),
    author text NOT NULL DEFAULT '' CHECK (char_length(author) <= 30),
    body text NOT NULL DEFAULT '',
    image_url text NOT NULL DEFAULT '' CHECK (image_url = '' OR image_url ~ '^https://'),
    published_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL,
    UNIQUE (feed_id, post_id)
);

CREATE TABLE notification_feed_subscriptions (
    endpoint_id bigint NOT NULL REFERENCES notification_endpoints ON DELETE CASCADE,
    feed_id bigint NOT NULL REFERENCES reddit_feeds ON DELETE CASCADE,
    PRIMARY KEY (endpoint_id, feed_id)
);
CREATE INDEX notification_feed_subscriptions_feed ON notification_feed_subscriptions (feed_id);

REVOKE SELECT ON reddit_feeds, reddit_posts, notification_feed_subscriptions FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON reddit_feeds, reddit_posts, notification_feed_subscriptions TO ratings_admin;
-- The posts seen go in a backup too: a restored feed without them would post its last 25 again.
GRANT SELECT ON reddit_feeds, reddit_posts, notification_feed_subscriptions TO ratings_backup;
-- The reddit job queues its notifications through the admin login.
GRANT INSERT ON notification_outbox TO ratings_admin;

ALTER TABLE notification_endpoints
    DROP CONSTRAINT notification_endpoints_events_check,
    ADD CONSTRAINT notification_endpoints_events_check
        CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'reddit_failing']);

ALTER TABLE notification_outbox
    ADD COLUMN reddit_post_id bigint REFERENCES reddit_posts ON DELETE CASCADE,
    ADD COLUMN feed_id bigint REFERENCES reddit_feeds ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP CONSTRAINT notification_outbox_check,
    ADD CONSTRAINT notification_outbox_kind_check
        CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'reddit_post', 'reddit_failed', 'reddit_recovered')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
               AND (kind = 'reddit_post') = (reddit_post_id IS NOT NULL)
               AND (kind IN ('reddit_failed', 'reddit_recovered')) = (feed_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind IN ('reddit_post', 'reddit_failed', 'reddit_recovered');
UPDATE notification_endpoints SET events = array_remove(events, 'reddit_failing');
ALTER TABLE notification_endpoints
    DROP CONSTRAINT notification_endpoints_events_check,
    ADD CONSTRAINT notification_endpoints_events_check CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag']);
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN reddit_post_id,
    DROP COLUMN feed_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL));
REVOKE INSERT ON notification_outbox FROM ratings_admin;
DROP TABLE notification_feed_subscriptions;
DROP TABLE reddit_posts;
DROP TABLE reddit_feeds;
