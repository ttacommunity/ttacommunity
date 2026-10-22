-- What we keep about a player, shown to them: own_member gives the player login the whole member its session is
-- linked to, and no other.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION own_member(session bytea, at timestamptz)
    RETURNS TABLE (tta_name text, cge_user integer, player_id bigint, discord_name text, email text, country_code text,
                   zone text, banned boolean, ban_reason text, banned_at timestamptz, created_at timestamptz)
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT m.tta_name, m.cge_user, m.player_id, m.discord_name, m.email, m.country_code, m.zone, m.banned, m.ban_reason,
           m.banned_at, m.created_at
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    JOIN members m ON m.member_id = a.member_id
    WHERE s.token_hash = session AND s.expires_at > at
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION own_member(bytea, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION own_member(bytea, timestamptz) TO ratings_player;

-- +goose Down
DROP FUNCTION own_member(bytea, timestamptz);
