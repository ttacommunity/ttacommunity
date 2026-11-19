-- Blitz news: two site events an endpoint can take, a round published and an edition finished. A round's
-- notification carries its games; once posted, they are kept per round number, so a round paired again is posted
-- only with other games. An edition is marked once its finish is queued; those finished before this are marked now.
-- +goose Up
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10',
        'blitz_round', 'blitz_finished']);

CREATE TABLE blitz_round_posts (
    tournament_id bigint NOT NULL REFERENCES blitz_editions ON DELETE CASCADE,
    number integer NOT NULL CHECK (number > 0),
    pairs text NOT NULL,
    PRIMARY KEY (tournament_id, number)
);
REVOKE SELECT ON blitz_round_posts FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_round_posts TO ratings_admin;
GRANT SELECT ON blitz_round_posts TO ratings_backup;

ALTER TABLE blitz_editions ADD COLUMN finish_posted boolean NOT NULL DEFAULT false;
UPDATE blitz_editions e SET finish_posted = true
WHERE EXISTS (
        SELECT 1 FROM blitz_rounds r JOIN blitz_matches m USING (round_id)
        WHERE r.tournament_id = e.tournament_id AND r.kind = 'knockout' AND r.published_at IS NOT NULL AND m.result IN ('a', 'b')
          AND NOT EXISTS (SELECT 1 FROM blitz_matches o WHERE o.round_id = r.round_id AND o.match_id <> m.match_id))
   OR (e.knockout_size = 0
       AND EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = e.tournament_id AND r.number = e.swiss_rounds AND r.published_at IS NOT NULL)
       AND NOT EXISTS (SELECT 1 FROM blitz_rounds r JOIN blitz_matches m USING (round_id)
                       WHERE r.tournament_id = e.tournament_id AND r.number = e.swiss_rounds AND m.entry_b IS NOT NULL AND m.result IS NULL));

ALTER TABLE notification_outbox
    ADD COLUMN round_id bigint REFERENCES blitz_rounds ON DELETE CASCADE,
    ADD COLUMN round_pairs text,
    ADD COLUMN edition_id bigint REFERENCES blitz_editions ON DELETE CASCADE,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished')),
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind = 'blitz_round') = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind = 'blitz_finished') = (edition_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind IN ('blitz_round', 'blitz_finished');
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_blitz_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    DROP COLUMN edition_id,
    DROP COLUMN round_pairs,
    DROP COLUMN round_id,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10'));
ALTER TABLE blitz_editions DROP COLUMN finish_posted;
DROP TABLE blitz_round_posts;
UPDATE notification_endpoints SET events = array_remove(array_remove(events, 'blitz_round'), 'blitz_finished');
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10']);
