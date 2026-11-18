-- One table of feeds for every source a notification endpoint can take, each with its kind: Reddit's feeds move in
-- unchanged, as kind reddit, their subreddit as the source, the kinds of post they send as post_kinds.
-- +goose Up
ALTER TABLE reddit_feeds RENAME TO feeds;
ALTER TABLE feeds RENAME CONSTRAINT reddit_feeds_pkey TO feeds_pkey;
ALTER TABLE feeds RENAME COLUMN subreddit TO source;
ALTER TABLE feeds RENAME COLUMN kinds TO post_kinds;
ALTER TABLE feeds RENAME CONSTRAINT reddit_feeds_kinds_check TO feeds_post_kinds_check;
ALTER TABLE feeds RENAME CONSTRAINT reddit_feeds_last_error_check TO feeds_last_error_check;
ALTER TABLE feeds RENAME CONSTRAINT reddit_feeds_failures_check TO feeds_failures_check;
ALTER TABLE feeds ADD COLUMN kind text NOT NULL DEFAULT 'reddit' CONSTRAINT feeds_kind_check CHECK (kind IN ('reddit'));
ALTER TABLE feeds ALTER COLUMN kind DROP DEFAULT;
ALTER TABLE feeds DROP CONSTRAINT reddit_feeds_subreddit_check,
    ADD CONSTRAINT feeds_source_check CHECK (kind <> 'reddit' OR source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$');
DROP INDEX reddit_feeds_subreddit;
CREATE UNIQUE INDEX feeds_kind_source ON feeds (kind, lower(source));

-- +goose Down
DELETE FROM feeds WHERE kind <> 'reddit';
DROP INDEX feeds_kind_source;
ALTER TABLE feeds DROP CONSTRAINT feeds_source_check,
    ADD CONSTRAINT reddit_feeds_subreddit_check CHECK (source ~ '^[A-Za-z0-9][A-Za-z0-9_]{2,20}$');
ALTER TABLE feeds DROP COLUMN kind;
ALTER TABLE feeds RENAME CONSTRAINT feeds_failures_check TO reddit_feeds_failures_check;
ALTER TABLE feeds RENAME CONSTRAINT feeds_last_error_check TO reddit_feeds_last_error_check;
ALTER TABLE feeds RENAME CONSTRAINT feeds_post_kinds_check TO reddit_feeds_kinds_check;
ALTER TABLE feeds RENAME COLUMN post_kinds TO kinds;
ALTER TABLE feeds RENAME COLUMN source TO subreddit;
ALTER TABLE feeds RENAME CONSTRAINT feeds_pkey TO reddit_feeds_pkey;
ALTER TABLE feeds RENAME TO reddit_feeds;
CREATE UNIQUE INDEX reddit_feeds_subreddit ON reddit_feeds (lower(subreddit));
