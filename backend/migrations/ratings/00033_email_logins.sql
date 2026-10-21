-- Signing in with an emailed link: a login by email address, the links sent (kept two days, for the sending
-- limits), an account linked to the one member whose recorded email is its address, and a player removing one of
-- their ways in. The player login still cannot read members' emails or delete a login: functions do both.
-- +goose Up
ALTER TABLE logins
    DROP CONSTRAINT logins_provider_check,
    ADD CONSTRAINT logins_provider_check CHECK (provider IN ('discord', 'email')),
    DROP CONSTRAINT logins_username_check,
    ADD CONSTRAINT logins_username_check CHECK (username <> '' AND char_length(username) <= 254),
    ADD CONSTRAINT logins_email_check CHECK (provider <> 'email'
        OR (subject = lower(subject) AND username = subject AND char_length(subject) <= 254 AND subject ~ '^[^@\s]+@[^@\s]+$'));

CREATE TABLE email_links (
    token_hash bytea PRIMARY KEY CHECK (octet_length(token_hash) = 32),
    address text NOT NULL CHECK (address = lower(address) AND char_length(address) <= 254),
    -- Set when the link adds a way in to this account, rather than signing in.
    account_id bigint REFERENCES accounts ON DELETE CASCADE,
    return_path text NOT NULL CHECK (char_length(return_path) <= 300),
    created_at timestamptz NOT NULL,
    expires_at timestamptz NOT NULL,
    used_at timestamptz
);
CREATE INDEX email_links_address ON email_links (address, created_at);
CREATE INDEX email_links_created ON email_links (created_at);
REVOKE ALL ON email_links FROM ratings_read;
GRANT SELECT, INSERT, UPDATE, DELETE ON email_links TO ratings_player;

-- +goose StatementBegin
CREATE FUNCTION link_by_email(account bigint, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
BEGIN
    UPDATE accounts a SET member_id = m.member_id, linked_how = 'match', linked_at = at
    FROM logins l, members m
    WHERE a.account_id = account AND a.member_id IS NULL AND a.auto_link
      AND l.account_id = a.account_id AND l.provider = 'email' AND lower(btrim(m.email)) = l.subject
      AND (SELECT count(*) FROM members o WHERE lower(btrim(o.email)) = l.subject) = 1
      AND NOT EXISTS (SELECT FROM accounts o WHERE o.member_id = m.member_id);
    RETURN FOUND;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION link_by_email(bigint, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION link_by_email(bigint, timestamptz) TO ratings_player;

-- +goose StatementBegin
CREATE FUNCTION remove_own_login(session bytea, kind text, at timestamptz) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
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
REVOKE ALL ON FUNCTION remove_own_login(bytea, text, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION remove_own_login(bytea, text, timestamptz) TO ratings_player;

-- Deleting an account also clears the member's email when the account signed in with it, so a new sign-in is not
-- linked by it again.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_own_account(session bytea, at timestamptz) RETURNS boolean
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
    UPDATE members SET discord_name = '', country_code = NULL, zone = 'Unknown',
        email = CASE WHEN lower(btrim(email)) IN (SELECT subject FROM logins WHERE account_id = account AND provider = 'email')
                     THEN '' ELSE email END
    WHERE member_id = member;
    DELETE FROM email_links WHERE account_id = account
       OR address IN (SELECT subject FROM logins WHERE account_id = account AND provider = 'email');
    DELETE FROM accounts WHERE account_id = account;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'account.delete-own', account::text, CASE WHEN member IS NULL THEN '{}' ELSE jsonb_build_object('member', member) END);
    RETURN true;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- Email sign-ins go, and the accounts left with no way in.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_own_account(session bytea, at timestamptz) RETURNS boolean
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
DROP FUNCTION remove_own_login(bytea, text, timestamptz);
DROP FUNCTION link_by_email(bigint, timestamptz);
DROP TABLE email_links;
DELETE FROM logins WHERE provider = 'email';
DELETE FROM accounts a WHERE NOT EXISTS (SELECT FROM logins WHERE account_id = a.account_id);
ALTER TABLE logins
    DROP CONSTRAINT logins_email_check,
    DROP CONSTRAINT logins_username_check,
    ADD CONSTRAINT logins_username_check CHECK (username <> '' AND char_length(username) <= 100),
    DROP CONSTRAINT logins_provider_check,
    ADD CONSTRAINT logins_provider_check CHECK (provider = 'discord');
