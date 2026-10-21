-- Signing in with Microsoft: a login by the account's id for this app, with the email Microsoft vouches for, if any.
-- That email fills a linked member's empty email as an email login's address does, and goes when the account is
-- deleted; it never links an account.
-- +goose Up
ALTER TABLE logins
    DROP CONSTRAINT logins_provider_check,
    ADD CONSTRAINT logins_provider_check CHECK (provider IN ('discord', 'email', 'microsoft')),
    ADD COLUMN email text NOT NULL DEFAULT '' CONSTRAINT logins_contact_email_check CHECK (email = ''
        OR (provider = 'microsoft' AND email = lower(email) AND char_length(email) <= 254 AND email ~ '^[^@\s]+@[^@\s]+$'));

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fill_member_from_logins(account bigint) RETURNS void
    LANGUAGE sql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
    UPDATE members m
    SET discord_name = CASE WHEN btrim(m.discord_name) = '' THEN coalesce(d.username, '') ELSE m.discord_name END,
        email = CASE WHEN btrim(m.email) = '' THEN coalesce(e.subject, nullif(ms.email, ''), '') ELSE m.email END
    FROM accounts a
    LEFT JOIN logins d ON d.account_id = a.account_id AND d.provider = 'discord'
    LEFT JOIN logins e ON e.account_id = a.account_id AND e.provider = 'email'
    LEFT JOIN logins ms ON ms.account_id = a.account_id AND ms.provider = 'microsoft'
    WHERE a.account_id = account AND m.member_id = a.member_id
      AND ((btrim(m.discord_name) = '' AND d.username IS NOT NULL)
        OR (btrim(m.email) = '' AND (e.subject IS NOT NULL OR ms.email <> '')));
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_own_account(session bytea, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
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
        email = CASE WHEN lower(btrim(email)) IN (SELECT CASE provider WHEN 'email' THEN subject ELSE email END FROM logins
                                                  WHERE account_id = account AND provider IN ('email', 'microsoft'))
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
-- Microsoft sign-ins go, and the accounts left with no way in. Emails they filled stay on the members.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_own_account(session bytea, at timestamptz) RETURNS boolean
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION fill_member_from_logins(account bigint) RETURNS void
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

DELETE FROM logins WHERE provider = 'microsoft';
DELETE FROM accounts a WHERE NOT EXISTS (SELECT FROM logins WHERE account_id = a.account_id);
ALTER TABLE logins
    DROP COLUMN email,
    DROP CONSTRAINT logins_provider_check,
    ADD CONSTRAINT logins_provider_check CHECK (provider IN ('discord', 'email'));
