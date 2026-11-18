-- Twitch streamers as feeds: a streamer's login is the source, their Twitch user id the remote id. Each has its
-- EventSub subscriptions (going live, going offline), the streams it went live with, the Discord message each
-- endpoint was posted (kept to edit it to "was live"), and the push messages already handled (Twitch may send one
-- twice). An endpoint's subscription to a feed gets options: categories and keywords to match, and "was live".
-- +goose Up
ALTER TABLE feeds
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit', 'twitch')),
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'reddit' AND source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$')
        OR (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')),
    ADD COLUMN remote_id text CONSTRAINT feeds_remote_id_check CHECK (remote_id ~ '^[0-9]{1,20}$'),
    ADD COLUMN display_name text NOT NULL DEFAULT '' CONSTRAINT feeds_display_name_check CHECK (char_length(display_name) <= 50),
    ADD CONSTRAINT feeds_remote_kind_check CHECK ((kind = 'twitch') = (remote_id IS NOT NULL));
CREATE UNIQUE INDEX feeds_kind_remote ON feeds (kind, remote_id) WHERE remote_id IS NOT NULL;

ALTER TABLE notification_feed_subscriptions
    ADD COLUMN categories text[] NOT NULL DEFAULT '{}'
        CONSTRAINT notification_feed_subscriptions_categories_check CHECK (cardinality(categories) <= 20 AND array_position(categories, '') IS NULL),
    ADD COLUMN keywords text[] NOT NULL DEFAULT '{}'
        CONSTRAINT notification_feed_subscriptions_keywords_check CHECK (cardinality(keywords) <= 20 AND array_position(keywords, '') IS NULL),
    ADD COLUMN was_live boolean NOT NULL DEFAULT false;

CREATE TABLE twitch_subscriptions (
    feed_id bigint NOT NULL REFERENCES feeds ON DELETE CASCADE,
    type text NOT NULL CHECK (type IN ('stream.online', 'stream.offline')),
    subscription_id text NOT NULL CHECK (subscription_id ~ '^[A-Za-z0-9-]{1,100}$'),
    status text NOT NULL CHECK (char_length(status) BETWEEN 1 AND 100),
    updated_at timestamptz NOT NULL,
    PRIMARY KEY (feed_id, type)
);
CREATE UNIQUE INDEX twitch_subscriptions_id ON twitch_subscriptions (subscription_id);

CREATE TABLE twitch_streams (
    twitch_stream_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id bigint NOT NULL REFERENCES feeds ON DELETE CASCADE,
    stream_id text NOT NULL UNIQUE CHECK (stream_id ~ '^[0-9]{1,20}$'),
    title text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
    category text NOT NULL DEFAULT '' CHECK (char_length(category) <= 100),
    thumbnail_url text NOT NULL DEFAULT '' CHECK (thumbnail_url = '' OR thumbnail_url ~ '^https://'),
    started_at timestamptz NOT NULL,
    ended_at timestamptz,
    recorded_at timestamptz NOT NULL
);
CREATE INDEX twitch_streams_open ON twitch_streams (feed_id) WHERE ended_at IS NULL;

CREATE TABLE twitch_posts (
    twitch_stream_id bigint NOT NULL REFERENCES twitch_streams ON DELETE CASCADE,
    endpoint_id bigint NOT NULL REFERENCES notification_endpoints ON DELETE CASCADE,
    message_id text NOT NULL CHECK (message_id ~ '^[0-9]{1,20}$'),
    channel_id text NOT NULL CHECK (channel_id ~ '^[0-9]{1,20}$'),
    PRIMARY KEY (twitch_stream_id, endpoint_id)
);

CREATE TABLE twitch_messages (
    message_id text PRIMARY KEY CHECK (char_length(message_id) BETWEEN 1 AND 100),
    received_at timestamptz NOT NULL
);

ALTER TABLE notification_outbox
    ADD COLUMN twitch_stream_id bigint REFERENCES twitch_streams ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'reddit_post', 'reddit_failed', 'reddit_recovered', 'twitch_live', 'twitch_ended')),
    ADD CONSTRAINT notification_outbox_twitch_check CHECK ((kind IN ('twitch_live', 'twitch_ended')) = (twitch_stream_id IS NOT NULL));

REVOKE SELECT ON twitch_subscriptions, twitch_streams, twitch_posts, twitch_messages FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON twitch_subscriptions, twitch_streams, twitch_posts, twitch_messages TO ratings_admin;
GRANT SELECT ON twitch_subscriptions, twitch_streams, twitch_posts TO ratings_backup;

-- +goose Down
DELETE FROM notification_outbox WHERE kind IN ('twitch_live', 'twitch_ended');
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_twitch_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN twitch_stream_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'reddit_post', 'reddit_failed', 'reddit_recovered'));
DROP TABLE twitch_messages, twitch_posts, twitch_streams, twitch_subscriptions;
ALTER TABLE notification_feed_subscriptions DROP COLUMN categories, DROP COLUMN keywords, DROP COLUMN was_live;
DELETE FROM feeds WHERE kind = 'twitch';
DROP INDEX feeds_kind_remote;
ALTER TABLE feeds
    DROP CONSTRAINT feeds_remote_kind_check,
    DROP COLUMN display_name,
    DROP COLUMN remote_id,
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (kind <> 'reddit' OR source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$'),
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit'));
