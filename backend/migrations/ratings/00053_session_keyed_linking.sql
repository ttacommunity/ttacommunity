-- The player login links its account by email and fills its member's details only through its own session:
-- link_own_by_email and fill_own_member. The account-keyed functions stay the owner's, and the admin login's.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION link_own_by_email(session bytea, at timestamptz) RETURNS boolean
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT coalesce((SELECT link_by_email(l.account_id, at)
                     FROM player_sessions s JOIN logins l USING (login_id)
                     WHERE s.token_hash = session AND s.expires_at > at), false);
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION fill_own_member(session bytea, at timestamptz) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    SELECT fill_member_from_logins(l.account_id)
    FROM player_sessions s JOIN logins l USING (login_id)
    WHERE s.token_hash = session AND s.expires_at > at;
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION link_own_by_email(bytea, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION fill_own_member(bytea, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION link_own_by_email(bytea, timestamptz), fill_own_member(bytea, timestamptz) TO ratings_player;
REVOKE EXECUTE ON FUNCTION link_by_email(bigint, timestamptz), fill_member_from_logins(bigint) FROM ratings_player;

-- +goose Down
GRANT EXECUTE ON FUNCTION link_by_email(bigint, timestamptz), fill_member_from_logins(bigint) TO ratings_player;
DROP FUNCTION fill_own_member(bytea, timestamptz);
DROP FUNCTION link_own_by_email(bytea, timestamptz);
