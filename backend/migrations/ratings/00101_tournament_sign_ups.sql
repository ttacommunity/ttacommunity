-- Tournament sign-ups: admin opens a tournament's sign-ups, with a closing time or none, and a player with an account
-- signs up while they take entries. A linked player is entered at once; an unlinked one's sign-up waits, with the TTA
-- username they gave, until their account is linked while sign-ups still take entries. Only blitz editions offer them
-- so far, and stop taking entries once round 1 is paired. Opening them is a site event, posted once per tournament.
-- +goose Up
ALTER TABLE tournaments
    ADD COLUMN sign_ups_open boolean NOT NULL DEFAULT false,
    ADD COLUMN sign_ups_close_at timestamptz,
    ADD COLUMN sign_ups_posted boolean NOT NULL DEFAULT false;
GRANT UPDATE (sign_ups_open, sign_ups_close_at, sign_ups_posted) ON tournaments TO ratings_admin;

CREATE TABLE tournament_sign_ups (
    sign_up_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tournament_id bigint NOT NULL REFERENCES tournaments ON DELETE CASCADE,
    account_id bigint NOT NULL REFERENCES accounts ON DELETE CASCADE,
    tta_name text NOT NULL CHECK (btrim(tta_name) <> '' AND char_length(tta_name) <= 100),
    signed_up_at timestamptz NOT NULL,
    UNIQUE (tournament_id, account_id)
);
REVOKE SELECT ON tournament_sign_ups FROM ratings_read;
GRANT SELECT, DELETE ON tournament_sign_ups TO ratings_admin;
GRANT SELECT ON tournament_sign_ups TO ratings_backup;

-- sign_ups_taking says whether tournament t takes entries at at: its sign-ups open, before their closing time, a
-- blitz edition with no round paired yet.
-- +goose StatementBegin
CREATE FUNCTION sign_ups_taking(t bigint, at timestamptz) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT tt.sign_ups_open AND at < coalesce(tt.sign_ups_close_at, 'infinity')
           AND s.blitz AND NOT EXISTS (SELECT 1 FROM blitz_rounds r WHERE r.tournament_id = tt.tournament_id)
    FROM tournaments tt JOIN series s USING (series_id)
    WHERE tt.tournament_id = t
$$;
-- +goose StatementEnd

-- sign_ups_waiting counts tournament t's sign-ups waiting for an account to be linked; the site shows the count, never
-- who they are.
-- +goose StatementBegin
CREATE FUNCTION sign_ups_waiting(t bigint) RETURNS integer
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT count(*)::integer FROM tournament_sign_ups u WHERE u.tournament_id = t
$$;
-- +goose StatementEnd

-- enter_pending_sign_ups enters account's member in each tournament it signed up for, unlinked, that takes entries at
-- at, and returns how many. Nothing for an unlinked account or a banned member; the other sign-ups stay pending.
-- +goose StatementBegin
CREATE FUNCTION enter_pending_sign_ups(account bigint, at timestamptz) RETURNS integer
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    member bigint;
    n integer;
BEGIN
    SELECT a.member_id INTO member FROM accounts a JOIN members m USING (member_id) WHERE a.account_id = account AND NOT m.banned;
    IF member IS NULL THEN
        RETURN 0;
    END IF;
    WITH due AS (
        DELETE FROM tournament_sign_ups u
        WHERE u.account_id = account AND sign_ups_taking(u.tournament_id, at)
        RETURNING u.tournament_id
    ), entered AS (
        INSERT INTO blitz_entries (tournament_id, member_id, joined_at)
        SELECT d.tournament_id, member, at FROM due d
        ON CONFLICT DO NOTHING
        RETURNING 1
    )
    SELECT count(*) INTO n FROM entered;
    RETURN n;
END
$$;
-- +goose StatementEnd

-- enter_own_pending_sign_ups is enter_pending_sign_ups for the account whose session hashes to session at at.
-- +goose StatementBegin
CREATE FUNCTION enter_own_pending_sign_ups(session bytea, at timestamptz) RETURNS integer
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT coalesce((SELECT enter_pending_sign_ups(l.account_id, at)
                     FROM player_sessions s JOIN logins l USING (login_id)
                     WHERE s.token_hash = session AND s.expires_at > at), 0)
$$;
-- +goose StatementEnd

-- sign_up_target finds the player whose session hashes to session at at, and the shown blitz edition, locked, or
-- why not: signed-out, banned, no-edition or closed (not taking entries).
-- +goose StatementBegin
CREATE FUNCTION sign_up_target(session bytea, at timestamptz, slug text, edition text,
                               OUT status text, OUT account bigint, OUT member bigint, OUT tournament bigint)
    LANGUAGE plpgsql
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    banned boolean;
BEGIN
    SELECT a.account_id, a.member_id INTO account, member
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        status := 'signed-out';
        RETURN;
    END IF;
    SELECT mm.banned INTO banned FROM members mm WHERE mm.member_id = member;
    IF banned THEN
        status := 'banned';
        RETURN;
    END IF;
    SELECT t.tournament_id INTO tournament
    FROM tournaments t JOIN series s USING (series_id)
    WHERE s.slug = sign_up_target.slug AND t.label = edition AND s.blitz AND s.visible
    FOR UPDATE OF t;
    IF NOT FOUND THEN
        status := 'no-edition';
        RETURN;
    END IF;
    status := CASE WHEN sign_ups_taking(tournament, at) THEN 'ok' ELSE 'closed' END;
END
$$;
-- +goose StatementEnd

-- sign_up signs the session's player up for the edition at at: a linked player is entered ('entered'); an unlinked one
-- gives name, kept as their pending sign-up ('pending'), or 'name-needed' without one. Signing up again is the same.
-- It returns sign_up_target's refusals, and the account.
-- +goose StatementBegin
CREATE FUNCTION sign_up(session bytea, at timestamptz, slug text, edition text, name text, OUT status text, OUT account bigint)
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
BEGIN
    SELECT * INTO t FROM sign_up_target(session, at, slug, edition);
    account := t.account;
    IF t.status <> 'ok' THEN
        status := t.status;
        RETURN;
    END IF;
    IF t.member IS NOT NULL THEN
        INSERT INTO blitz_entries (tournament_id, member_id, joined_at) VALUES (t.tournament, t.member, at) ON CONFLICT DO NOTHING;
        DELETE FROM tournament_sign_ups WHERE tournament_id = t.tournament AND account_id = t.account;
        status := 'entered';
        RETURN;
    END IF;
    IF coalesce(btrim(name), '') = '' OR char_length(btrim(name)) > 100 THEN
        status := 'name-needed';
        RETURN;
    END IF;
    INSERT INTO tournament_sign_ups (tournament_id, account_id, tta_name, signed_up_at) VALUES (t.tournament, t.account, btrim(name), at)
    ON CONFLICT (tournament_id, account_id) DO UPDATE SET tta_name = excluded.tta_name;
    status := 'pending';
END
$$;
-- +goose StatementEnd

-- withdraw_sign_up takes the session's player out of the edition at at: their entry, while they have played nothing
-- (no round is paired while it takes entries), and their pending sign-up. It returns 'withdrawn' or sign_up_target's
-- refusals.
-- +goose StatementBegin
CREATE FUNCTION withdraw_sign_up(session bytea, at timestamptz, slug text, edition text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
BEGIN
    SELECT * INTO t FROM sign_up_target(session, at, slug, edition);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    DELETE FROM blitz_entries WHERE tournament_id = t.tournament AND member_id = t.member;
    DELETE FROM tournament_sign_ups WHERE tournament_id = t.tournament AND account_id = t.account;
    RETURN 'withdrawn';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- my_sign_ups is every pending sign-up of the account whose session hashes to session at at, with its tournament:
-- none for a session that has ended.
CREATE FUNCTION my_sign_ups(session bytea, at timestamptz)
    RETURNS TABLE (slug text, series text, label text, sign_up_id bigint, account_id bigint, tta_name text, signed_up_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT s.slug, s.name, t.label, u.sign_up_id, u.account_id, u.tta_name, u.signed_up_at
    FROM player_sessions ps JOIN logins l USING (login_id)
    JOIN tournament_sign_ups u ON u.account_id = l.account_id
    JOIN tournaments t ON t.tournament_id = u.tournament_id JOIN series s ON s.series_id = t.series_id
    WHERE ps.token_hash = session AND ps.expires_at > at
    ORDER BY u.signed_up_at, u.sign_up_id
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION sign_ups_taking(bigint, timestamptz), sign_ups_waiting(bigint), enter_pending_sign_ups(bigint, timestamptz),
    enter_own_pending_sign_ups(bytea, timestamptz),
    sign_up_target(bytea, timestamptz, text, text), sign_up(bytea, timestamptz, text, text, text),
    withdraw_sign_up(bytea, timestamptz, text, text), my_sign_ups(bytea, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION sign_ups_taking(bigint, timestamptz), sign_ups_waiting(bigint) TO ratings_read, ratings_admin, ratings_player;
GRANT EXECUTE ON FUNCTION enter_pending_sign_ups(bigint, timestamptz) TO ratings_admin;
GRANT EXECUTE ON FUNCTION enter_own_pending_sign_ups(bytea, timestamptz), sign_up(bytea, timestamptz, text, text, text),
    withdraw_sign_up(bytea, timestamptz, text, text), my_sign_ups(bytea, timestamptz) TO ratings_player;

ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10',
        'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups']);
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder', 'blitz_sign_ups')),
    DROP CONSTRAINT notification_outbox_blitz_check,
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind IN ('blitz_round', 'blitz_reminder')) = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind IN ('blitz_finished', 'blitz_sign_ups')) = (edition_id IS NOT NULL)
        AND (kind = 'blitz_reminder') = (endpoint_id IS NOT NULL));

-- +goose Down
DELETE FROM notification_outbox WHERE kind = 'blitz_sign_ups';
ALTER TABLE notification_outbox
    DROP CONSTRAINT notification_outbox_blitz_check,
    DROP CONSTRAINT notification_outbox_kind_check,
    ADD CONSTRAINT notification_outbox_kind_check CHECK (kind IN ('sign_up', 'link_request', 'blitz_report', 'blitz_flag',
        'feed_failed', 'feed_recovered', 'twitch_live', 'twitch_ended', 'rss_item', 'youtube_live', 'youtube_ended',
        'ratings_run', 'ratings_top10', 'blitz_round', 'blitz_finished', 'blitz_reminder')),
    ADD CONSTRAINT notification_outbox_blitz_check CHECK ((kind IN ('blitz_round', 'blitz_reminder')) = (round_id IS NOT NULL)
        AND (kind = 'blitz_round') = (round_pairs IS NOT NULL) AND (kind = 'blitz_finished') = (edition_id IS NOT NULL)
        AND (kind = 'blitz_reminder') = (endpoint_id IS NOT NULL));
UPDATE notification_endpoints SET events = array_remove(events, 'blitz_sign_ups');
ALTER TABLE notification_endpoints DROP CONSTRAINT notification_endpoints_events_check;
ALTER TABLE notification_endpoints ADD CONSTRAINT notification_endpoints_events_check
    CHECK (events <@ ARRAY['sign_up', 'link_request', 'blitz_report', 'blitz_flag', 'feed_failing', 'ratings_run', 'ratings_top10',
        'blitz_round', 'blitz_finished', 'blitz_reminder']);
DROP FUNCTION my_sign_ups(bytea, timestamptz);
DROP FUNCTION withdraw_sign_up(bytea, timestamptz, text, text);
DROP FUNCTION sign_up(bytea, timestamptz, text, text, text);
DROP FUNCTION sign_up_target(bytea, timestamptz, text, text);
DROP FUNCTION enter_own_pending_sign_ups(bytea, timestamptz);
DROP FUNCTION enter_pending_sign_ups(bigint, timestamptz);
DROP FUNCTION sign_ups_waiting(bigint);
DROP FUNCTION sign_ups_taking(bigint, timestamptz);
DROP TABLE tournament_sign_ups;
ALTER TABLE tournaments DROP COLUMN sign_ups_posted, DROP COLUMN sign_ups_close_at, DROP COLUMN sign_ups_open;
