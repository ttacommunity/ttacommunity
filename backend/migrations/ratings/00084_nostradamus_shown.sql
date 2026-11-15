-- A player can ask to join, or read their place in, only a game the site shows: one with a round published.
-- request_nostradamus_entry and my_nostradamus answer 'no-edition' for any other, as for an unknown game.

-- +goose Up
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION request_nostradamus_entry(session bytea, at timestamptz, edition text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    ed bigint;
    found_status text;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN who.refusal;
    END IF;
    SELECT e.edition_id INTO ed FROM nostradamus_editions e
    WHERE e.slug = edition AND EXISTS (SELECT FROM nostradamus_rounds r WHERE r.edition_id = e.edition_id AND r.published_at IS NOT NULL);
    IF NOT FOUND THEN
        RETURN 'no-edition';
    END IF;
    SELECT p.status INTO found_status FROM nostradamus_participants p WHERE p.edition_id = ed AND p.member_id = who.member;
    IF FOUND THEN
        RETURN found_status;
    END IF;
    UPDATE nostradamus_participants p SET member_id = who.member
    FROM members m
    WHERE m.member_id = who.member AND p.edition_id = ed AND p.member_id IS NULL AND NOT p.bot AND lower(p.name) = lower(m.tta_name)
    RETURNING p.status INTO found_status;
    IF FOUND THEN
        RETURN found_status;
    END IF;
    INSERT INTO nostradamus_participants (edition_id, name, member_id, status, requested_at)
    SELECT ed, m.tta_name, m.member_id, 'requested', at FROM members m WHERE m.member_id = who.member;
    RETURN 'requested';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION my_nostradamus(session bytea, at timestamptz, edition text)
    RETURNS TABLE (status text, question_id bigint, answer text[], tokens integer)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    p record;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN QUERY SELECT who.refusal, NULL::bigint, NULL::text[], NULL::integer;
        RETURN;
    END IF;
    IF NOT EXISTS (SELECT FROM nostradamus_editions e
                   WHERE e.slug = edition AND EXISTS (SELECT FROM nostradamus_rounds r WHERE r.edition_id = e.edition_id AND r.published_at IS NOT NULL)) THEN
        RETURN QUERY SELECT 'no-edition'::text, NULL::bigint, NULL::text[], NULL::integer;
        RETURN;
    END IF;
    SELECT x.participant_id, x.status INTO p FROM nostradamus_participants x JOIN nostradamus_editions e USING (edition_id)
    WHERE e.slug = edition AND x.member_id = who.member;
    IF NOT FOUND THEN
        RETURN QUERY SELECT 'none'::text, NULL::bigint, NULL::text[], NULL::integer;
        RETURN;
    END IF;
    RETURN QUERY SELECT p.status::text, NULL::bigint, NULL::text[], NULL::integer
        UNION ALL
        SELECT p.status::text, a.question_id, a.answer, a.tokens FROM nostradamus_answers a WHERE a.participant_id = p.participant_id;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION request_nostradamus_entry(session bytea, at timestamptz, edition text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    ed bigint;
    found_status text;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN who.refusal;
    END IF;
    SELECT e.edition_id INTO ed FROM nostradamus_editions e WHERE e.slug = edition;
    IF NOT FOUND THEN
        RETURN 'no-edition';
    END IF;
    SELECT p.status INTO found_status FROM nostradamus_participants p WHERE p.edition_id = ed AND p.member_id = who.member;
    IF FOUND THEN
        RETURN found_status;
    END IF;
    UPDATE nostradamus_participants p SET member_id = who.member
    FROM members m
    WHERE m.member_id = who.member AND p.edition_id = ed AND p.member_id IS NULL AND NOT p.bot AND lower(p.name) = lower(m.tta_name)
    RETURNING p.status INTO found_status;
    IF FOUND THEN
        RETURN found_status;
    END IF;
    INSERT INTO nostradamus_participants (edition_id, name, member_id, status, requested_at)
    SELECT ed, m.tta_name, m.member_id, 'requested', at FROM members m WHERE m.member_id = who.member;
    RETURN 'requested';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION my_nostradamus(session bytea, at timestamptz, edition text)
    RETURNS TABLE (status text, question_id bigint, answer text[], tokens integer)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    p record;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN QUERY SELECT who.refusal, NULL::bigint, NULL::text[], NULL::integer;
        RETURN;
    END IF;
    SELECT x.participant_id, x.status INTO p FROM nostradamus_participants x JOIN nostradamus_editions e USING (edition_id)
    WHERE e.slug = edition AND x.member_id = who.member;
    IF NOT FOUND THEN
        RETURN QUERY SELECT 'none'::text, NULL::bigint, NULL::text[], NULL::integer;
        RETURN;
    END IF;
    RETURN QUERY SELECT p.status::text, NULL::bigint, NULL::text[], NULL::integer
        UNION ALL
        SELECT p.status::text, a.question_id, a.answer, a.tokens FROM nostradamus_answers a WHERE a.participant_id = p.participant_id;
END
$$;
-- +goose StatementEnd
