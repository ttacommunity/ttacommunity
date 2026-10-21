-- A linked member's empty Discord name and email come from their account's logins: at a link, when a way in is
-- added, and now for the members linked already. A name or email the member has is kept.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION fill_member_from_logins(account bigint) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    UPDATE members m
    SET discord_name = CASE WHEN btrim(m.discord_name) = '' THEN coalesce(d.username, '') ELSE m.discord_name END,
        email = CASE WHEN btrim(m.email) = '' THEN coalesce(e.subject, '') ELSE m.email END
    FROM accounts a
    LEFT JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
    LEFT JOIN logins e ON e.account_id = a.account_id AND e.provider = 'email'
    WHERE a.account_id = account AND m.member_id = a.member_id
      AND ((btrim(m.discord_name) = '' AND d.username IS NOT NULL) OR (btrim(m.email) = '' AND e.subject IS NOT NULL));
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION fill_member_from_logins(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION fill_member_from_logins(bigint) TO ratings_player, ratings_admin;

SELECT fill_member_from_logins(account_id) FROM accounts WHERE member_id IS NOT NULL;

-- +goose Down
-- The names and emails filled in stay on the members: an admin could have set the same.
DROP FUNCTION fill_member_from_logins(bigint);
