-- Player accounts: one for each player from their first sign-in, holding the ways they sign in (logins), the member
-- they are linked to and how, their requests to be linked, and the country and zone they gave before the link. Each
-- existing login becomes an account of its own, with the same id. A session stays with the login it signed in by.
-- +goose Up
CREATE TABLE accounts (
    account_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    member_id bigint UNIQUE REFERENCES members,
    linked_how text CHECK (linked_how IN ('match', 'approved')),
    linked_at timestamptz,
    -- Off once an admin unlinks it, so a wrong match by a recorded name is not made again at the next sign-in.
    auto_link boolean NOT NULL DEFAULT true,
    country_code text CHECK (country_code ~ '^[A-Z]{2}$'),
    zone text CHECK (zone <> '' AND char_length(zone) <= 64),
    created_at timestamptz NOT NULL,
    CHECK ((member_id IS NULL) = (linked_how IS NULL) AND (member_id IS NULL) = (linked_at IS NULL))
);

INSERT INTO accounts (account_id, member_id, linked_how, linked_at, auto_link, country_code, zone, created_at)
OVERRIDING SYSTEM VALUE
SELECT login_id, member_id, linked_how, linked_at, auto_link, country_code, zone, created_at FROM logins;
SELECT setval(pg_get_serial_sequence('accounts', 'account_id'), coalesce(max(account_id), 0) + 1, false) FROM accounts;

ALTER TABLE logins ADD COLUMN account_id bigint REFERENCES accounts ON DELETE CASCADE;
UPDATE logins SET account_id = login_id;
ALTER TABLE logins
    ALTER COLUMN account_id SET NOT NULL,
    DROP COLUMN member_id,
    DROP COLUMN linked_how,
    DROP COLUMN linked_at,
    DROP COLUMN auto_link,
    DROP COLUMN country_code,
    DROP COLUMN zone,
    ADD CONSTRAINT logins_one_of_each UNIQUE (account_id, provider);

ALTER TABLE link_requests ADD COLUMN account_id bigint REFERENCES accounts ON DELETE CASCADE;
UPDATE link_requests SET account_id = login_id;
ALTER TABLE link_requests ALTER COLUMN account_id SET NOT NULL, DROP COLUMN login_id;
CREATE UNIQUE INDEX link_requests_one_pending ON link_requests (account_id) WHERE status = 'pending';
CREATE INDEX link_requests_account ON link_requests (account_id, request_id);

DROP FUNCTION delete_own_login(bytea, timestamptz);
-- +goose StatementBegin
CREATE FUNCTION delete_own_account(session bytea, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
DECLARE
    account bigint;
    member bigint;
BEGIN
    SELECT a.account_id, a.member_id INTO account, member
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    WHERE s.token_hash = session AND s.expires_at > at
    FOR UPDATE OF a;
    IF NOT FOUND THEN
        RETURN false;
    END IF;
    UPDATE members SET discord_name = '', country_code = NULL, zone = 'Unknown' WHERE member_id = member;
    DELETE FROM accounts WHERE account_id = account;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'account.delete-own', account::text, CASE WHEN member IS NULL THEN '{}' ELSE jsonb_build_object('member', member) END);
    RETURN true;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION delete_own_account(bytea, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION delete_own_account(bytea, timestamptz) TO ratings_player;

REVOKE ALL ON accounts FROM ratings_read;
GRANT SELECT, INSERT, UPDATE ON accounts TO ratings_player;
GRANT SELECT, UPDATE, DELETE ON accounts TO ratings_admin;

-- +goose Down
-- Each login gets its account's member and details back; a request goes to the account's first login.
DROP FUNCTION delete_own_account(bytea, timestamptz);
ALTER TABLE logins
    DROP CONSTRAINT logins_one_of_each,
    ADD COLUMN member_id bigint REFERENCES members,
    ADD COLUMN linked_how text CHECK (linked_how IN ('match', 'approved')),
    ADD COLUMN linked_at timestamptz,
    ADD COLUMN auto_link boolean NOT NULL DEFAULT true,
    ADD COLUMN country_code text CHECK (country_code ~ '^[A-Z]{2}$'),
    ADD COLUMN zone text CHECK (zone <> '' AND char_length(zone) <= 64);
UPDATE logins l SET member_id = a.member_id, linked_how = a.linked_how, linked_at = a.linked_at, auto_link = a.auto_link,
    country_code = a.country_code, zone = a.zone
FROM accounts a WHERE a.account_id = l.account_id;
ALTER TABLE logins ADD CHECK ((member_id IS NULL) = (linked_how IS NULL) AND (member_id IS NULL) = (linked_at IS NULL));
CREATE UNIQUE INDEX logins_member_provider ON logins (member_id, provider) WHERE member_id IS NOT NULL;

ALTER TABLE link_requests ADD COLUMN login_id bigint REFERENCES logins ON DELETE CASCADE;
UPDATE link_requests r SET login_id = (SELECT min(login_id) FROM logins l WHERE l.account_id = r.account_id);
ALTER TABLE link_requests ALTER COLUMN login_id SET NOT NULL, DROP COLUMN account_id;
CREATE UNIQUE INDEX link_requests_one_pending ON link_requests (login_id) WHERE status = 'pending';
CREATE INDEX link_requests_login ON link_requests (login_id, request_id);

ALTER TABLE logins DROP COLUMN account_id;
DROP TABLE accounts;

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
