-- Which member a TTA username names, in one place: the member of that username in any case, or the one with the
-- ratings player or CGE account of that name. Admin's approval uses it; a player asking to be linked learns only
-- whether that member is linked to another account already.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION named_member(name text) RETURNS bigint
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
    SELECT m.member_id FROM members m
    WHERE lower(m.tta_name) = lower(name)
       OR m.player_id = (SELECT player_id FROM players WHERE lower(players.name) = lower(named_member.name) ORDER BY player_id LIMIT 1)
       OR m.cge_user = (SELECT cge_user FROM player_accounts a WHERE lower(a.name) = lower(named_member.name) AND NOT a.deleted
                        ORDER BY cge_user LIMIT 1)
    ORDER BY m.member_id LIMIT 1
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION name_linked_elsewhere(name text, account bigint) RETURNS boolean
    LANGUAGE sql STABLE SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
    SELECT EXISTS (SELECT FROM accounts a WHERE a.member_id = named_member(name) AND a.account_id <> account)
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION named_member(text) FROM PUBLIC;
REVOKE ALL ON FUNCTION name_linked_elsewhere(text, bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION named_member(text) TO ratings_admin;
GRANT EXECUTE ON FUNCTION name_linked_elsewhere(text, bigint) TO ratings_player;

-- +goose Down
DROP FUNCTION name_linked_elsewhere(text, bigint);
DROP FUNCTION named_member(text);
