-- A player deletes their own account: delete_own_login deletes the login holding a live session, with its sessions
-- and requests, clears its member's Discord name, country and zone, and audits it by ids alone. The player login
-- still cannot delete logins itself, and can delete only the one whose session it holds.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION delete_own_login(session bytea, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
DECLARE
    login bigint;
    member bigint;
BEGIN
    SELECT l.login_id, l.member_id INTO login, member
    FROM player_sessions s JOIN logins l USING (login_id)
    WHERE s.token_hash = session AND s.expires_at > at
    FOR UPDATE OF l;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    UPDATE members SET discord_name = '', country_code = NULL, zone = 'Unknown' WHERE member_id = member;
    DELETE FROM logins WHERE login_id = login;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'login.delete-own', login::text, CASE WHEN member IS NULL THEN '{}' ELSE jsonb_build_object('member', member) END);
    RETURN true;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION delete_own_login(bytea, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION delete_own_login(bytea, timestamptz) TO ratings_player;

-- +goose Down
DROP FUNCTION delete_own_login(bytea, timestamptz);
