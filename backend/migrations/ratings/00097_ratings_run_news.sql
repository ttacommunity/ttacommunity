-- Ratings run news: two site events an endpoint can take, a community run published for the first time and the
-- changes in its top 10. Each notification names the run and the run current before it, which the post compares; a
-- run deleted before its news is sent leaves it unsent.
-- +goose Up
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10']);

ALTER TABLE notification_outbox
    ADD COLUMN published_run_id bigint REFERENCES rating_runs ON DELETE SET NULL,
    ADD COLUMN before_run_id bigint REFERENCES rating_runs ON DELETE SET NULL,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10')),
    ADD CONSTRAINT notification_outbox_run_check CHECK (kind IN ('ratings_run', 'ratings_top10')
        OR (published_run_id IS NULL AND before_run_id IS NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind IN ('ratings_run', 'ratings_top10');
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_run_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN before_run_id,
    DROP COLUMN published_run_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended'));
UPDATE notification_endpoints SET events = array_remove(array_remove(events, 'ratings_run'), 'ratings_top10');
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing']);
