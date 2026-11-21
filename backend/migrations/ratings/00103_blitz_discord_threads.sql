-- Blitz games' private Discord threads, made by the bot: a second kind of endpoint, a bot channel (a channel id, no
-- webhook), taking blitz_threads and, in the threads of games unplayed, blitz_reminder; a notification per game of a published round with a deadline, per game whose
-- deadline moved, and per round published again with threads its games no longer have; and each thread made, kept per
-- edition, round, pair of players and endpoint with its message and when it was reminded and closed, so none is made
-- or reminded twice.
-- +goose Up
ALTER TABLE notification_endpoints
    ADD COLUMN kind text NOT NULL DEFAULT 'webhook' CHECK (kind IN ('webhook', 'bot_channel')),
    ADD COLUMN channel_id text NOT NULL DEFAULT '',
    DROP CONSTRAINT notification_endpoints_url_check,
    ADD CONSTRAINT notification_endpoints_url_check CHECK (CASE kind
        WHEN 'webhook' THEN url ~ '^https://(discord|discordapp)\.com/api/webhooks/[0-9]{17,20}/[A-Za-z0-9_-]{1,100}$' AND channel_id = ''
        ELSE url = '' AND channel_id ~ '^[0-9]{17,20}$' END),
    DROP CONSTRAINT notification_endpoints_events_check,
    ADD CONSTRAINT notification_endpoints_events_check CHECK (CASE kind
        WHEN 'webhook' THEN events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run',
            'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups']
        ELSE events <@ ARRAY['blitz_threads', 'blitz_reminder'] END);

CREATE TABLE blitz_discord_threads (
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    number integer NOT NULL,
    entry_low bigint NOT NULL REFERENCES blitz_entries ON DELETE CASCADE,
    entry_high bigint NOT NULL REFERENCES blitz_entries ON DELETE CASCADE,
    endpoint_id bigint NOT NULL REFERENCES notification_endpoints ON DELETE CASCADE,
    thread_id text NOT NULL CHECK (thread_id ~ '^[0-9]{17,20}$'),
    made_at timestamptz NOT NULL,
    posted_at timestamptz,
    message_id text NOT NULL DEFAULT '' CHECK (message_id = '' OR message_id ~ '^[0-9]{17,20}$'),
    reminded_at timestamptz,
    closed_at timestamptz,
    PRIMARY KEY (tournament_id, number, entry_low, entry_high, endpoint_id),
    CHECK (entry_low < entry_high)
);
REVOKE SELECT ON blitz_discord_threads FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_discord_threads TO ratings_admin;
GRANT SELECT ON blitz_discord_threads TO ratings_backup;

ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups', 'blitz_thread',
        'blitz_deadline', 'blitz_replaced')),
    DROP CONSTRAINT notification_outbox_check,
    ADD CONSTRAINT notification_outbox_check CHECK ((kind = 'sign_up') = (account_id IS NOT NULL)
        AND (kind = 'link_request') = (request_id IS NOT NULL)
        AND (kind IN ('blitz_report', 'blitz_flag', 'blitz_thread', 'blitz_deadline')) = (match_id IS NOT NULL)
        AND (kind IN ('feed_failed', 'feed_recovered')) = (feed_id IS NOT NULL) AND (kind = 'rss_item') = (rss_item_id IS NOT NULL)),
    DROP CONSTRAINT notification_outbox_blitz_check,
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind IN ('blitz_round', 'blitz_reminder', 'blitz_replaced')) = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind IN ('blitz_finished', 'blitz_sign_ups')) = (edition_id IS NOT NULL)
        AND (kind = 'blitz_reminder') = (endpoint_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind IN ('blitz_thread', 'blitz_deadline', 'blitz_replaced');
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_blitz_check,
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind IN ('blitz_round', 'blitz_reminder')) = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind IN ('blitz_finished', 'blitz_sign_ups')) = (edition_id IS NOT NULL)
        AND (kind = 'blitz_reminder') = (endpoint_id IS NOT NULL)),
    DROP CONSTRAINT notification_outbox_check,
    ADD CONSTRAINT notification_outbox_check CHECK ((kind = 'sign_up') = (account_id IS NOT NULL)
        AND (kind = 'link_request') = (request_id IS NOT NULL) AND (kind IN ('blitz_report', 'blitz_flag')) = (match_id IS NOT NULL)
        AND (kind IN ('feed_failed', 'feed_recovered')) = (feed_id IS NOT NULL) AND (kind = 'rss_item') = (rss_item_id IS NOT NULL)),
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups'));
DROP TABLE blitz_discord_threads;
DELETE FROM notification_endpoints WHERE kind = 'bot_channel';
ALTER TABLE notification_endpoints
    DROP CONSTRAINT notification_endpoints_events_check,
    ADD CONSTRAINT notification_endpoints_events_check CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failing', 'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups']),
    DROP CONSTRAINT notification_endpoints_url_check,
    ADD CONSTRAINT notification_endpoints_url_check
        CHECK (url ~ '^https://(discord|discordapp)\.com/api/webhooks/[0-9]{17,20}/[A-Za-z0-9_-]{1,100}$'),
    DROP COLUMN channel_id,
    DROP COLUMN kind;
