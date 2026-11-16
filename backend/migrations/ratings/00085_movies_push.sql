-- Movies by YouTube's push notifications: each channel's subscription state, every pushed video with its TTA
-- verdict and an admin's override, and the functions the hub's callback writes through as the player login.

-- +goose Up
ALTER TABLE video_channels
    DROP COLUMN last_checked_at,
    ADD COLUMN subscribed_at timestamptz,
    ADD COLUMN lease_ends_at timestamptz,
    ADD COLUMN last_push_at timestamptz,
    ADD COLUMN last_error text,
    ADD COLUMN last_error_at timestamptz,
    ADD COLUMN backfilled_at timestamptz;

ALTER TABLE videos
    ADD COLUMN is_tta boolean NOT NULL DEFAULT true,
    ADD COLUMN tta_reason text NOT NULL DEFAULT 'stored before verdicts',
    ADD COLUMN tta_override boolean,
    ADD COLUMN details_fetched_at timestamptz,
    ADD COLUMN updated_at timestamptz;
UPDATE videos SET tta_override = false WHERE hidden;
DROP INDEX videos_published_idx;
ALTER TABLE videos DROP COLUMN hidden;
ALTER TABLE videos ALTER COLUMN is_tta DROP DEFAULT, ALTER COLUMN tta_reason DROP DEFAULT;
-- A video is shown when an admin says so or, without an override, when it is about TTA.
CREATE INDEX videos_shown_idx ON videos (published_at DESC) WHERE coalesce(tta_override, is_tta);

-- +goose StatementBegin
-- confirm_movie_hub answers the hub's check of a request: a subscribe for a linked channel, an unsubscribe for one
-- no longer linked. A confirmed subscribe records its lease.
CREATE FUNCTION confirm_movie_hub(channel text, mode text, lease_seconds integer, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
BEGIN
    IF mode = 'subscribe' THEN
        UPDATE video_channels
        SET subscribed_at = at,
            lease_ends_at = CASE WHEN lease_seconds > 0 THEN at + make_interval(secs => lease_seconds) END
        WHERE channel_id = channel;
        RETURN FOUND;
    ELSIF mode = 'unsubscribe' THEN
        RETURN NOT EXISTS (SELECT FROM video_channels WHERE channel_id = channel);
    END IF;
    RETURN false;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- save_movie_video stores or updates a linked channel's video with its verdict, keeping an admin's override and
-- leaving another channel's video alone; false when the channel is not linked. The hub's pushes and the daily job
-- both store through it.
CREATE FUNCTION save_movie_video(video text, channel text, video_title text, published timestamptz, thumbnail text,
                                 tta boolean, reason text, details_fetched timestamptz, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql
    SET search_path TO public, pg_temp
    AS $$
BEGIN
    IF NOT EXISTS (SELECT FROM video_channels WHERE channel_id = channel) THEN
        RETURN false;
    END IF;
    INSERT INTO videos (video_id, channel_id, title, published_at, thumbnail_url, is_tta, tta_reason, details_fetched_at,
                        created_at, updated_at)
    VALUES (video, channel, video_title, published, thumbnail, tta, reason, details_fetched, at, at)
    ON CONFLICT (video_id) DO UPDATE
    SET title = EXCLUDED.title, thumbnail_url = EXCLUDED.thumbnail_url, is_tta = EXCLUDED.is_tta,
        tta_reason = EXCLUDED.tta_reason, details_fetched_at = EXCLUDED.details_fetched_at, updated_at = EXCLUDED.updated_at
    WHERE videos.channel_id = EXCLUDED.channel_id;
    RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- store_pushed_video records the hub's push for a linked channel and saves its video, answering 'stored' or
-- 'no-channel'.
CREATE FUNCTION store_pushed_video(video text, channel text, video_title text, published timestamptz, thumbnail text,
                                   tta boolean, reason text, details_fetched timestamptz, at timestamptz) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
BEGIN
    UPDATE video_channels SET last_push_at = at WHERE channel_id = channel;
    IF NOT save_movie_video(video, channel, video_title, published, thumbnail, tta, reason, details_fetched, at) THEN
        RETURN 'no-channel';
    END IF;
    RETURN 'stored';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- remove_pushed_video drops a video the hub says its channel deleted.
CREATE FUNCTION remove_pushed_video(video text, channel text, at timestamptz) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    UPDATE video_channels SET last_push_at = at WHERE channel_id = channel;
    DELETE FROM videos WHERE video_id = video AND channel_id = channel;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION save_movie_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION save_movie_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz) TO ratings_admin;
REVOKE ALL ON FUNCTION confirm_movie_hub(text, text, integer, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION store_pushed_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION remove_pushed_video(text, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION confirm_movie_hub(text, text, integer, timestamptz) TO ratings_player;
GRANT EXECUTE ON FUNCTION store_pushed_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz) TO ratings_player;
GRANT EXECUTE ON FUNCTION remove_pushed_video(text, text, timestamptz) TO ratings_player;

-- +goose Down
DROP FUNCTION remove_pushed_video(text, text, timestamptz);
DROP FUNCTION store_pushed_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz);
DROP FUNCTION save_movie_video(text, text, text, timestamptz, text, boolean, text, timestamptz, timestamptz);
DROP FUNCTION confirm_movie_hub(text, text, integer, timestamptz);
DROP INDEX videos_shown_idx;
-- created_at is rebuilt after hidden, so the columns are in 84's order.
ALTER TABLE videos
    ADD COLUMN hidden boolean NOT NULL DEFAULT false,
    ADD COLUMN created_again timestamptz DEFAULT now();
UPDATE videos SET hidden = NOT coalesce(tta_override, is_tta), created_again = created_at;
ALTER TABLE videos DROP COLUMN created_at;
ALTER TABLE videos RENAME COLUMN created_again TO created_at;
ALTER TABLE videos ALTER COLUMN created_at SET NOT NULL;
DELETE FROM videos WHERE hidden AND tta_override IS NULL;
ALTER TABLE videos
    DROP COLUMN updated_at,
    DROP COLUMN details_fetched_at,
    DROP COLUMN tta_override,
    DROP COLUMN tta_reason,
    DROP COLUMN is_tta;
CREATE INDEX videos_published_idx ON videos (published_at DESC) WHERE NOT hidden;
ALTER TABLE video_channels
    DROP COLUMN backfilled_at,
    DROP COLUMN last_error_at,
    DROP COLUMN last_error,
    DROP COLUMN last_push_at,
    DROP COLUMN lease_ends_at,
    DROP COLUMN subscribed_at,
    ADD COLUMN last_checked_at timestamptz;
