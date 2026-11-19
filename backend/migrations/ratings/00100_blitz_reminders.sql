-- Blitz deadline reminders: a site event an endpoint can take, posted that many hours before a round's deadline
-- (blitz_reminder_hours). Each reminder is queued for one endpoint, and kept per round and endpoint so it is queued
-- once.
-- +goose Up
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10',
        'blitz_round', 'blitz_finished', 'blitz_reminder']);
ALTER TABLE notification_endpoints ADD COLUMN blitz_reminder_hours integer NOT NULL DEFAULT 24
    CHECK (blitz_reminder_hours BETWEEN 1 AND 168);

CREATE TABLE blitz_reminders (
    round_id bigint NOT NULL REFERENCES blitz_rounds ON DELETE CASCADE,
    endpoint_id bigint NOT NULL REFERENCES notification_endpoints ON DELETE CASCADE,
    queued_at timestamptz NOT NULL,
    PRIMARY KEY (round_id, endpoint_id)
);
REVOKE SELECT ON blitz_reminders FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_reminders TO ratings_admin;
GRANT SELECT ON blitz_reminders TO ratings_backup;

ALTER TABLE notification_outbox
    ADD COLUMN endpoint_id bigint REFERENCES notification_endpoints ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder')),
    DROP CONSTRAINT notification_outbox_blitz_check,
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind IN ('blitz_round', 'blitz_reminder')) = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind = 'blitz_finished') = (edition_id IS NOT NULL)
        AND (kind = 'blitz_reminder') = (endpoint_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind = 'blitz_reminder';
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_blitz_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN endpoint_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished')),
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind = 'blitz_round') = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind = 'blitz_finished') = (edition_id IS NOT NULL));
DROP TABLE blitz_reminders;
ALTER TABLE notification_endpoints DROP COLUMN blitz_reminder_hours;
UPDATE notification_endpoints SET events = array_remove(events, 'blitz_reminder');
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10',
        'blitz_round', 'blitz_finished']);
