-- YouTube creator channels and TTA community videos for the Movies feed, plus each player account's last-seen time.
-- +goose Up
CREATE TABLE video_channels (
    channel_id text PRIMARY KEY CHECK (channel_id ~ '^UC[A-Za-z0-9_-]{22}$'),
    name text NOT NULL CHECK (btrim(name) <> ''),
    player_id bigint REFERENCES players(player_id) ON DELETE SET NULL,
    include_all boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    last_checked_at timestamptz
);

CREATE TABLE videos (
    video_id text PRIMARY KEY CHECK (video_id ~ '^[A-Za-z0-9_-]{11}$'),
    channel_id text NOT NULL REFERENCES video_channels(channel_id) ON DELETE CASCADE,
    title text NOT NULL,
    published_at timestamptz NOT NULL,
    thumbnail_url text NOT NULL,
    hidden boolean NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX videos_published_idx ON videos (published_at DESC) WHERE NOT hidden;

ALTER TABLE accounts ADD COLUMN movies_seen_at timestamptz;

GRANT SELECT ON video_channels, videos TO ratings_read, ratings_backup;
GRANT SELECT ON videos TO ratings_player;
GRANT SELECT, INSERT, UPDATE, DELETE ON video_channels, videos TO ratings_admin;

-- +goose Down
ALTER TABLE accounts DROP COLUMN movies_seen_at;
DROP TABLE videos;
DROP TABLE video_channels;
