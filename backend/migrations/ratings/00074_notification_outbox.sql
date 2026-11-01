-- Notifications for admins and organisers. The endpoints admin posts them to: a Discord webhook (the only format so
-- far), the events it receives and text put before each message. The outbox: a player's request to be linked, or their
-- blitz report or flag, queued in the same transaction as what it tells of. The api posts each to every endpoint
-- taking its event, then deletes it; repeats sent together are posted once, written from the data as it is then.
-- +goose Up
CREATE TABLE notification_endpoints (
    endpoint_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    url text NOT NULL CHECK (url ~ '^https://(discord|discordapp)\.com/api/webhooks/[0-9]{17,20}/[A-Za-z0-9_-]{1,100}$'),
    format text NOT NULL DEFAULT 'discord' CHECK (format IN ('discord')),
    events text[] NOT NULL CHECK (events <@ ARRAY['link_request', 'blitz_report', 'blitz_flag']),
    prefix text NOT NULL DEFAULT '' CHECK (char_length(prefix) <= 20),
    created_at timestamptz NOT NULL
);
REVOKE SELECT ON notification_endpoints FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON notification_endpoints TO ratings_admin;
GRANT SELECT ON notification_endpoints TO ratings_backup;

CREATE TABLE notification_outbox (
    notification_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    kind text NOT NULL CHECK (kind IN ('link_request', 'blitz_report', 'blitz_flag')),
    request_id bigint REFERENCES link_requests ON DELETE CASCADE,
    match_id bigint REFERENCES blitz_matches ON DELETE CASCADE,
    created_at timestamptz NOT NULL,
    next_at timestamptz NOT NULL,
    attempts integer NOT NULL DEFAULT 0 CHECK (attempts >= 0),
    -- The endpoints it reached on an earlier try, so a retry posts only to the rest.
    sent_to bigint[] NOT NULL DEFAULT '{}',
    CHECK ((kind = 'link_request') = (request_id IS NOT NULL) AND (kind <> 'link_request') = (match_id IS NOT NULL))
);
CREATE INDEX notification_outbox_due ON notification_outbox (next_at);
REVOKE SELECT ON notification_outbox FROM ratings_read;
GRANT INSERT ON notification_outbox TO ratings_player;
GRANT SELECT, UPDATE, DELETE ON notification_outbox TO ratings_admin;

-- +goose Down
DROP TABLE notification_outbox;
DROP TABLE notification_endpoints;
