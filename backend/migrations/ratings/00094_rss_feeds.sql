-- RSS and Atom feeds as feeds of kind rss: the feed's https address is the source, its title the display name, as
-- the feeds job last read it. Each item seen is kept, as Reddit's posts are, so none is posted twice; its text only
-- until it is posted. Word of a feed failing or reading again is for every kind now: reddit_failing becomes
-- feed_failing, and its notifications feed_failed and feed_recovered.
-- +goose Up
ALTER TABLE feeds
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit', 'twitch', 'rss')),
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'reddit' AND source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$')
        OR (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')
        OR (kind = 'rss' AND source ~ '^https://[^\s/?#@]+(/\S*)?$' AND char_length(source) <= 2000)),
    DROP CONSTRAINT feeds_display_name_check,
    ADD CONSTRAINT feeds_display_name_check CHECK (char_length(display_name) <= 100);

CREATE TABLE rss_items (
    rss_item_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id bigint NOT NULL REFERENCES feeds ON DELETE CASCADE,
    item_id text NOT NULL CHECK (char_length(item_id) BETWEEN 1 AND 1000),
    title text NOT NULL DEFAULT '' CHECK (char_length(title) <= 300),
    url text NOT NULL DEFAULT '' CHECK (url = '' OR url ~ '^https?://'),
    summary text NOT NULL DEFAULT '' CHECK (char_length(summary) <= 3000),
    image_url text NOT NULL DEFAULT '' CHECK (image_url = '' OR image_url ~ '^https://'),
    published_at timestamptz NOT NULL,
    recorded_at timestamptz NOT NULL,
    UNIQUE (feed_id, item_id)
);

REVOKE SELECT ON rss_items FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON rss_items TO ratings_admin;
GRANT SELECT ON rss_items TO ratings_backup;

ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
UPDATE notification_endpoints SET events = array_replace(events, 'reddit_failing', 'feed_failing');
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing']);

ALTER TABLE notification_outbox
    ADD COLUMN rss_item_id bigint REFERENCES rss_items ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP CONSTRAINT notification_outbox_check;
UPDATE notification_outbox SET kind = CASE kind WHEN 'reddit_failed' THEN 'feed_failed' ELSE 'feed_recovered' END
    WHERE kind IN ('reddit_failed', 'reddit_recovered');
ALTER TABLE notification_outbox
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'reddit_post', 'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
               AND (kind = 'reddit_post') = (reddit_post_id IS NOT NULL)
               AND (kind IN ('feed_failed', 'feed_recovered')) = (feed_id IS NOT NULL)
               AND (kind = 'rss_item') = (rss_item_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind = 'rss_item'
    OR (kind IN ('feed_failed', 'feed_recovered') AND feed_id IN (SELECT feed_id FROM feeds WHERE kind = 'rss'));
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN rss_item_id;
UPDATE notification_outbox SET kind = CASE kind WHEN 'feed_failed' THEN 'reddit_failed' ELSE 'reddit_recovered' END
    WHERE kind IN ('feed_failed', 'feed_recovered');
ALTER TABLE notification_outbox
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'reddit_post', 'reddit_failed', 'reddit_recovered', 'twitch_live', 'twitch_ended')),
    ADD CONSTRAINT notification_outbox_check
        CHECK ((kind = 'sign_up') = (account_id IS NOT NULL) AND (kind = 'link_request') = (request_id IS NOT NULL)
               AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
               AND (kind = 'reddit_post') = (reddit_post_id IS NOT NULL)
               AND (kind IN ('reddit_failed', 'reddit_recovered')) = (feed_id IS NOT NULL));

ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
UPDATE notification_endpoints SET events = array_replace(events, 'feed_failing', 'reddit_failing');
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'reddit_failing']);

DROP TABLE rss_items;
DELETE FROM feeds WHERE kind = 'rss';
ALTER TABLE feeds
    DROP CONSTRAINT feeds_display_name_check,
    ADD CONSTRAINT feeds_display_name_check CHECK (char_length(display_name) <= 50),
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'reddit' AND source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$')
        OR (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')),
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit', 'twitch'));
