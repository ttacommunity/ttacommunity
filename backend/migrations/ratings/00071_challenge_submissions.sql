-- Players submit their own challenge entries: a signed-in player whose account is linked to a member, not banned,
-- through submit_challenge_entries, the player login's only way to write them. An entry made here is source 'site',
-- so re-importing the old site's entries never replaces it. Each submission writes an audit row.
-- +goose Up
ALTER TABLE challenge_entries DROP CONSTRAINT challenge_entries_source_check;
ALTER TABLE challenge_entries ADD CONSTRAINT challenge_entries_source_check CHECK (source IN ('sheet', 'web', 'site'));

-- +goose StatementBegin
CREATE FUNCTION submit_challenge_entries(session bytea, at timestamptz, entries jsonb) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    account bigint;
    member bigint;
    name text;
    banned boolean;
    saved integer;
BEGIN
    SELECT a.account_id, a.member_id INTO account, member
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        RETURN 'signed-out';
    END IF;
    IF member IS NULL THEN
        RETURN 'not-linked';
    END IF;
    SELECT m.tta_name, m.banned INTO name, banned FROM members m WHERE m.member_id = member;
    IF banned THEN
        RETURN 'banned';
    END IF;
    IF jsonb_typeof(entries) <> 'array' OR jsonb_array_length(entries) NOT BETWEEN 1 AND 120
       OR EXISTS (SELECT FROM jsonb_to_recordset(entries) AS e(challenge text)
                  WHERE NOT EXISTS (SELECT FROM challenges c WHERE c.challenge_id = e.challenge))
       OR (SELECT count(DISTINCT e.challenge) <> count(*) FROM jsonb_to_recordset(entries) AS e(challenge text))
       OR EXISTS (SELECT FROM jsonb_to_recordset(entries) AS e(proof_url text)
                  WHERE e.proof_url !~ '^https://discord\.com/channels/358698681672925195/\d+/\d+$') THEN
        RETURN 'bad';
    END IF;
    INSERT INTO challenge_entries (challenge_id, member_id, name, account_id, domination, proof_url, created_at, source)
    SELECT e.item->>'challenge', member, name, account, (e.item->>'domination')::integer, e.item->>'proof_url', at, 'site'
    FROM jsonb_array_elements(entries) WITH ORDINALITY AS e(item, n)
    ORDER BY e.n;
    GET DIAGNOSTICS saved = ROW_COUNT;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'challenge.entries.submit', member::text, jsonb_build_object('account', account, 'entries', entries));
    RETURN 'saved:' || saved;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION submit_challenge_entries(bytea, timestamptz, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION submit_challenge_entries(bytea, timestamptz, jsonb) TO ratings_player;

-- +goose Down
-- Fails while entries made on this site are kept, rather than delete them.
DROP FUNCTION submit_challenge_entries(bytea, timestamptz, jsonb);
ALTER TABLE challenge_entries DROP CONSTRAINT challenge_entries_source_check;
ALTER TABLE challenge_entries ADD CONSTRAINT challenge_entries_source_check CHECK (source IN ('sheet', 'web'));
