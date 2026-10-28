-- A linked member's Discord name is their Discord sign-in's, whenever they have one: a name an organiser typed is a
-- pre-fill. Email is still filled only when empty. A change to the name, and a way in added or removed, is audited by ids.
-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fill_member_from_logins(account bigint) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    INSERT INTO admin_audit (discord_id, action, target, detail)
    SELECT '', 'member.discord', m.member_id::text, jsonb_build_object('account', a.account_id)
    FROM accounts a JOIN members m ON m.member_id = a.member_id
    JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
    WHERE a.account_id = account AND m.discord_name <> d.username;

    UPDATE members m
    SET discord_name = coalesce(d.username, m.discord_name),
        email = CASE WHEN btrim(m.email) = '' THEN coalesce(e.subject, nullif(ms.email, ''), nullif(g.email, ''), '') ELSE m.email END
    FROM accounts a
    LEFT JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
    LEFT JOIN logins e ON e.account_id = a.account_id AND e.provider = 'email'
    LEFT JOIN logins ms ON ms.account_id = a.account_id AND ms.provider = 'microsoft'
    LEFT JOIN logins g ON g.account_id = a.account_id AND g.provider = 'google'
    WHERE a.account_id = account AND m.member_id = a.member_id
      AND (m.discord_name <> d.username
        OR (btrim(m.email) = '' AND (e.subject IS NOT NULL OR ms.email <> '' OR g.email <> '')));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION audit_own_login_added(session bytea, kind text, at timestamptz) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    SELECT at, '', 'login.add', l.account_id::text, jsonb_build_object('provider', kind)
    FROM player_sessions s JOIN logins l USING (login_id)
    WHERE s.token_hash = session AND s.expires_at > at
      AND EXISTS (SELECT FROM logins k WHERE k.account_id = l.account_id AND k.provider = kind);
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION audit_own_login_added(bytea, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION audit_own_login_added(bytea, text, timestamptz) TO ratings_player;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION remove_own_login(session bytea, kind text, at timestamptz) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    current bigint;
    account bigint;
    target bigint;
BEGIN
    SELECT s.login_id, l.account_id INTO current, account
    FROM player_sessions s JOIN logins l USING (login_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        RETURN 'signed-out';
    END IF;
    PERFORM FROM accounts WHERE account_id = account FOR UPDATE;
    SELECT login_id INTO target FROM logins WHERE account_id = account AND provider = kind;
    IF NOT FOUND THEN
        RETURN 'none';
    END IF;
    IF NOT EXISTS (SELECT FROM logins WHERE account_id = account AND login_id <> target) THEN
        RETURN 'last';
    END IF;
    UPDATE player_sessions SET login_id = (SELECT min(login_id) FROM logins WHERE account_id = account AND login_id <> target)
    WHERE token_hash = session AND login_id = target;
    DELETE FROM logins WHERE login_id = target;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'login.remove', account::text, jsonb_build_object('provider', kind));
    RETURN 'removed';
END
$$;
-- +goose StatementEnd

-- +goose Down
-- The audit rows written stay.
DROP FUNCTION audit_own_login_added(bytea, text, timestamptz);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION remove_own_login(session bytea, kind text, at timestamptz) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    current bigint;
    account bigint;
    target bigint;
BEGIN
    SELECT s.login_id, l.account_id INTO current, account
    FROM player_sessions s JOIN logins l USING (login_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        RETURN 'signed-out';
    END IF;
    PERFORM FROM accounts WHERE account_id = account FOR UPDATE;
    SELECT login_id INTO target FROM logins WHERE account_id = account AND provider = kind;
    IF NOT FOUND THEN
        RETURN 'none';
    END IF;
    IF NOT EXISTS (SELECT FROM logins WHERE account_id = account AND login_id <> target) THEN
        RETURN 'last';
    END IF;
    UPDATE player_sessions SET login_id = (SELECT min(login_id) FROM logins WHERE account_id = account AND login_id <> target)
    WHERE token_hash = session AND login_id = target;
    DELETE FROM logins WHERE login_id = target;
    RETURN 'removed';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fill_member_from_logins(account bigint) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    UPDATE members m
    SET discord_name = CASE WHEN btrim(m.discord_name) = '' THEN coalesce(d.username, '') ELSE m.discord_name END,
        email = CASE WHEN btrim(m.email) = '' THEN coalesce(e.subject, nullif(ms.email, ''), nullif(g.email, ''), '') ELSE m.email END
    FROM accounts a
    LEFT JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
    LEFT JOIN logins e ON e.account_id = a.account_id AND e.provider = 'email'
    LEFT JOIN logins ms ON ms.account_id = a.account_id AND ms.provider = 'microsoft'
    LEFT JOIN logins g ON g.account_id = a.account_id AND g.provider = 'google'
    WHERE a.account_id = account AND m.member_id = a.member_id
      AND ((btrim(m.discord_name) = '' AND d.username IS NOT NULL)
        OR (btrim(m.email) = '' AND (e.subject IS NOT NULL OR ms.email <> '' OR g.email <> '')));
$$;
-- +goose StatementEnd
