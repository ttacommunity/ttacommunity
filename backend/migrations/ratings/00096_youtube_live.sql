-- YouTube Live as one feed, of kind youtube and source movies: the live streams of Movies' channels. A stream is
-- noted when the hub pushes it, live or scheduled, and checked with the Data API at next_check_at until it ends.
-- Each endpoint's post of it is kept, to edit it to "was live".
-- +goose Up
ALTER TABLE feeds
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('twitch', 'rss', 'youtube')),
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')
        OR (kind = 'rss' AND source ~ '^https://[^\s/?#@]+(/\S*)?$' AND char_length(source) <= 2000)
        OR (kind = 'youtube' AND source = 'movies'));

CREATE TABLE youtube_streams (
    youtube_stream_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    feed_id bigint NOT NULL REFERENCES feeds ON DELETE CASCADE,
    video_id text NOT NULL UNIQUE REFERENCES videos ON DELETE CASCADE,
    title text NOT NULL CHECK (char_length(title) <= 300),
    started_at timestamptz,
    ended_at timestamptz CHECK (ended_at IS NULL OR started_at IS NOT NULL),
    next_check_at timestamptz CHECK (next_check_at IS NULL OR ended_at IS NULL),
    recorded_at timestamptz NOT NULL
);
CREATE INDEX youtube_streams_due ON youtube_streams (next_check_at) WHERE next_check_at IS NOT NULL;

CREATE TABLE youtube_posts (
    youtube_stream_id bigint NOT NULL REFERENCES youtube_streams ON DELETE CASCADE,
    endpoint_id bigint NOT NULL REFERENCES notification_endpoints ON DELETE CASCADE,
    message_id text NOT NULL CHECK (message_id ~ '^[0-9]{1,20}$'),
    channel_id text NOT NULL CHECK (channel_id ~ '^[0-9]{1,20}$'),
    PRIMARY KEY (youtube_stream_id, endpoint_id)
);

ALTER TABLE notification_outbox
    ADD COLUMN youtube_stream_id bigint REFERENCES youtube_streams ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended')),
    ADD CONSTRAINT notification_outbox_youtube_check CHECK ((kind IN ('youtube_live', 'youtube_ended')) = (youtube_stream_id IS NOT NULL));

REVOKE SELECT ON youtube_streams, youtube_posts FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON youtube_streams, youtube_posts TO ratings_admin;
GRANT SELECT ON youtube_streams, youtube_posts TO ratings_backup;

-- +goose StatementBegin
-- note_youtube_stream marks a linked channel's stored video, pushed live or scheduled, to be checked at at, when the
-- YouTube Live feed is there and not paused; a stream that ended is left alone.
CREATE FUNCTION note_youtube_stream(video text, channel text, at timestamptz) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    INSERT INTO youtube_streams (feed_id, video_id, title, next_check_at, recorded_at)
    SELECT f.feed_id, v.video_id, left(v.title, 300), at, at
    FROM feeds f, videos v
    WHERE f.kind = 'youtube' AND NOT f.paused AND v.video_id = video AND v.channel_id = channel
    ON CONFLICT (video_id) DO UPDATE SET next_check_at = EXCLUDED.next_check_at
    WHERE youtube_streams.ended_at IS NULL;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION note_youtube_stream(text, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION note_youtube_stream(text, text, timestamptz) TO ratings_player;

-- +goose Down
DROP FUNCTION note_youtube_stream(text, text, timestamptz);
DELETE FROM notification_outbox WHERE kind IN ('youtube_live', 'youtube_ended');
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_youtube_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN youtube_stream_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item'));
DROP TABLE youtube_posts, youtube_streams;
DELETE FROM feeds WHERE kind = 'youtube';
ALTER TABLE feeds
    DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT feeds_source_check CHECK (
        (kind = 'twitch' AND source ~ '^[A-Za-z0-9_]{3,25}$')
        OR (kind = 'rss' AND source ~ '^https://[^\s/?#@]+(/\S*)?$' AND char_length(source) <= 2000)),
    DROP CONSTRAINT feeds_kind_check,
    ADD CONSTRAINT feeds_kind_check CHECK (kind IN ('twitch', 'rss'));
