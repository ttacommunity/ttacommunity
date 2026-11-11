-- Nostradamus: a prediction game over a tournament. Organisers draft a round of questions, publish it with a deadline,
-- and close it once its outcomes are known; accepted participants answer before the deadline, putting tokens on some.
-- Scoring lives here (nostradamus_points, nostradamus_round_scores); players write only through the functions below.
-- +goose Up
CREATE TABLE nostradamus_editions (
    edition_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{1,40}$'),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    tournament_id bigint NOT NULL REFERENCES tournaments ON DELETE CASCADE
);

-- A round is a draft until published_at, open for answers until closes_at, and scored on the site from closed_at.
CREATE TABLE nostradamus_rounds (
    round_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    edition_id bigint NOT NULL REFERENCES nostradamus_editions ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    label text NOT NULL CHECK (label ~ '^[A-Za-z0-9 ]{1,20}$'),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    tokens integer NOT NULL CHECK (tokens BETWEEN 0 AND 10),
    token_points integer NOT NULL CHECK (token_points BETWEEN 0 AND 100),
    closes_at timestamptz,
    deadline_zone text CHECK (deadline_zone ~ '^[A-Za-z0-9_+/ -]{1,64}$'),
    published_at timestamptz,
    closed_at timestamptz,
    CHECK (published_at IS NULL OR (closes_at IS NOT NULL AND deadline_zone IS NOT NULL)),
    CHECK (closed_at IS NULL OR published_at IS NOT NULL),
    UNIQUE (edition_id, position),
    UNIQUE (edition_id, label)
);

-- +goose StatementBegin
-- nostradamus_valid reports whether values are an answer or outcome of this kind: one name or option, a set of
-- different names, or one whole number.
CREATE FUNCTION nostradamus_valid(kind text, vals text[]) RETURNS boolean
    LANGUAGE sql IMMUTABLE STRICT
    AS $$
    SELECT cardinality(vals) BETWEEN 1 AND 20
        AND NOT EXISTS (SELECT FROM unnest(vals) v WHERE btrim(v) = '' OR char_length(v) > 100)
        AND CASE kind
            WHEN 'pick' THEN cardinality(vals) = 1
            WHEN 'set' THEN (SELECT count(DISTINCT lower(v)) FROM unnest(vals) v) = cardinality(vals)
            WHEN 'number' THEN cardinality(vals) = 1 AND vals[1] ~ '^-?[0-9]{1,6}$'
            ELSE false
        END
$$;
-- +goose StatementEnd

-- A set takes picks names; asks_last marks a question about who finishes last, which a bot answers from the bottom.
CREATE TABLE nostradamus_questions (
    question_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    round_id bigint NOT NULL REFERENCES nostradamus_rounds ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    label text NOT NULL CHECK (label <> '' AND char_length(label) <= 60),
    prompt text NOT NULL DEFAULT '' CHECK (char_length(prompt) <= 500),
    kind text NOT NULL CHECK (kind IN ('pick', 'set', 'number')),
    points integer NOT NULL CHECK (points BETWEEN 0 AND 100),
    near_points integer NOT NULL DEFAULT 0 CHECK (near_points BETWEEN 0 AND points AND (kind = 'number' OR near_points = 0)),
    picks integer NOT NULL DEFAULT 1 CHECK (picks BETWEEN 1 AND 20 AND (kind = 'set' OR picks = 1)),
    asks_last boolean NOT NULL DEFAULT false CHECK (NOT asks_last OR kind <> 'number'),
    -- A pick's outcome names everyone who tied.
    outcome text[] CHECK (outcome IS NULL OR nostradamus_valid(CASE kind WHEN 'pick' THEN 'set' ELSE kind END, outcome)),
    UNIQUE (round_id, position)
);
CREATE UNIQUE INDEX nostradamus_questions_label ON nostradamus_questions (round_id, lower(label));

-- A question's choices, when it has any: an answer must be among them. Seed and leaderboard rank feed the bots.
CREATE TABLE nostradamus_choices (
    question_id bigint NOT NULL REFERENCES nostradamus_questions ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    label text NOT NULL CHECK (btrim(label) = label AND label <> '' AND char_length(label) <= 100),
    seed integer CHECK (seed > 0),
    leaderboard_rank integer CHECK (leaderboard_rank > 0),
    PRIMARY KEY (question_id, position)
);
CREATE UNIQUE INDEX nostradamus_choices_label ON nostradamus_choices (question_id, lower(label));

-- A bot answers by its rule: the best seed, the best Glicko, or the best leaderboard rank among a question's choices.
CREATE TABLE nostradamus_participants (
    participant_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    edition_id bigint NOT NULL REFERENCES nostradamus_editions ON DELETE CASCADE,
    name text NOT NULL CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 100),
    member_id bigint REFERENCES members ON DELETE SET NULL,
    bot boolean NOT NULL DEFAULT false,
    bot_rule text CHECK (bot_rule IS NULL OR (bot_rule IN ('seed', 'glicko', 'leaderboard') AND bot)),
    status text NOT NULL DEFAULT 'accepted' CHECK (status IN ('requested', 'accepted', 'refused')),
    requested_at timestamptz
);
CREATE UNIQUE INDEX nostradamus_participants_name ON nostradamus_participants (edition_id, lower(name));
CREATE UNIQUE INDEX nostradamus_participants_one_member ON nostradamus_participants (edition_id, member_id);
CREATE INDEX nostradamus_participants_member ON nostradamus_participants (member_id);

CREATE TABLE nostradamus_answers (
    question_id bigint NOT NULL REFERENCES nostradamus_questions ON DELETE CASCADE,
    participant_id bigint NOT NULL REFERENCES nostradamus_participants ON DELETE CASCADE,
    answer text[] NOT NULL,
    token boolean NOT NULL DEFAULT false,
    PRIMARY KEY (question_id, participant_id)
);
CREATE INDEX nostradamus_answers_participant ON nostradamus_answers (participant_id);

-- +goose StatementBegin
-- nostradamus_check_answer refuses an answer not of its question's kind, of the wrong size, not among its choices, in
-- a closed round, from another edition's participant, or whose token takes its participant past the round's tokens.
CREATE FUNCTION nostradamus_check_answer() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    q record;
BEGIN
    SELECT n.kind, n.picks, n.round_id, r.edition_id, r.tokens, r.closed_at INTO q
    FROM nostradamus_questions n JOIN nostradamus_rounds r USING (round_id)
    WHERE n.question_id = NEW.question_id;
    IF NOT nostradamus_valid(q.kind, NEW.answer) OR (q.kind = 'set' AND cardinality(NEW.answer) <> q.picks) THEN
        RAISE EXCEPTION 'not a % answer of % names', q.kind, q.picks USING ERRCODE = 'check_violation';
    END IF;
    IF EXISTS (SELECT FROM nostradamus_choices c WHERE c.question_id = NEW.question_id)
       AND EXISTS (SELECT FROM unnest(NEW.answer) v WHERE NOT EXISTS (
           SELECT FROM nostradamus_choices c WHERE c.question_id = NEW.question_id AND lower(c.label) = lower(v))) THEN
        RAISE EXCEPTION 'an answer not among the choices' USING ERRCODE = 'check_violation';
    END IF;
    IF q.closed_at IS NOT NULL THEN
        RAISE EXCEPTION 'a closed round' USING ERRCODE = 'check_violation';
    END IF;
    IF NOT EXISTS (SELECT FROM nostradamus_participants p WHERE p.participant_id = NEW.participant_id AND p.edition_id = q.edition_id) THEN
        RAISE EXCEPTION 'a participant of another edition' USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.token AND (SELECT count(*) FROM nostradamus_answers a JOIN nostradamus_questions n USING (question_id)
                      WHERE n.round_id = q.round_id AND a.participant_id = NEW.participant_id AND a.token
                        AND a.question_id <> NEW.question_id) >= q.tokens THEN
        RAISE EXCEPTION 'more than % tokens in a round', q.tokens USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END
$$;
-- +goose StatementEnd
CREATE TRIGGER nostradamus_check_answer BEFORE INSERT OR UPDATE ON nostradamus_answers
    FOR EACH ROW EXECUTE FUNCTION nostradamus_check_answer();

-- +goose StatementBegin
-- nostradamus_points scores an answer against its question's outcome, null while the outcome is unknown. A pick is
-- right when it is any of the outcome's names, a set when it holds exactly the outcome's names, a number when it is
-- the outcome; a number one off scores the near points. A token wins or loses token_points on whether it is right.
CREATE FUNCTION nostradamus_points(kind text, points integer, near_points integer, outcome text[], answer text[],
                                   token boolean, token_points integer) RETURNS integer
    LANGUAGE plpgsql IMMUTABLE
    AS $$
DECLARE
    right_answer boolean := false;
    near boolean := false;
    v text;
BEGIN
    IF outcome IS NULL THEN
        RETURN NULL;
    END IF;
    IF kind = 'pick' THEN
        FOREACH v IN ARRAY outcome LOOP
            right_answer := right_answer OR lower(v) = lower(answer[1]);
        END LOOP;
    ELSIF kind = 'set' THEN
        right_answer := cardinality(answer) = cardinality(outcome);
        FOREACH v IN ARRAY answer LOOP
            right_answer := right_answer AND lower(v) = ANY (SELECT lower(o) FROM unnest(outcome) o);
        END LOOP;
    ELSE
        right_answer := answer[1]::integer = outcome[1]::integer;
        near := abs(answer[1]::integer - outcome[1]::integer) = 1;
    END IF;
    RETURN CASE WHEN right_answer THEN points WHEN near THEN near_points ELSE 0 END
         + CASE WHEN NOT token THEN 0 WHEN right_answer THEN token_points ELSE -token_points END;
END
$$;
-- +goose StatementEnd

-- Each accepted participant's points in each round with a known outcome; closed says the site shows it. A participant
-- who did not answer the round scores the lowest of the bots that did, or nothing without one.
CREATE VIEW nostradamus_round_scores AS
WITH answered AS (
    SELECT n.round_id, a.participant_id,
           coalesce(sum(nostradamus_points(n.kind, n.points, n.near_points, n.outcome, a.answer, a.token, r.token_points)), 0)::integer AS points
    FROM nostradamus_answers a
    JOIN nostradamus_questions n USING (question_id)
    JOIN nostradamus_rounds r USING (round_id)
    GROUP BY n.round_id, a.participant_id
), lowest_bot AS (
    SELECT s.round_id, min(s.points) AS points
    FROM answered s JOIN nostradamus_participants p USING (participant_id)
    WHERE p.bot
    GROUP BY s.round_id
)
SELECT r.edition_id, r.round_id, p.participant_id, s.participant_id IS NULL AS missed,
       coalesce(s.points, b.points, 0) AS points, r.closed_at IS NOT NULL AS closed
FROM nostradamus_rounds r
JOIN nostradamus_participants p ON p.edition_id = r.edition_id AND p.status = 'accepted'
LEFT JOIN answered s ON s.round_id = r.round_id AND s.participant_id = p.participant_id
LEFT JOIN lowest_bot b ON b.round_id = r.round_id
WHERE EXISTS (SELECT FROM nostradamus_questions n WHERE n.round_id = r.round_id AND n.outcome IS NOT NULL);

-- How many answers named each value, for a closed round's poll; it names no one.
CREATE VIEW nostradamus_answer_counts AS
SELECT a.question_id, min(v) AS value, count(*)::integer AS answers
FROM nostradamus_answers a, unnest(a.answer) v
GROUP BY a.question_id, lower(v);

-- +goose StatementBegin
-- nostradamus_member_of resolves a player's session to its member: null with a refusal of 'signed-out', 'not-linked'
-- or 'banned'.
CREATE FUNCTION nostradamus_member_of(session bytea, at timestamptz, OUT member bigint, OUT refusal text)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
BEGIN
    SELECT a.member_id INTO member
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        refusal := 'signed-out';
    ELSIF member IS NULL THEN
        refusal := 'not-linked';
    ELSIF (SELECT m.banned FROM members m WHERE m.member_id = member) THEN
        member := NULL;
        refusal := 'banned';
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- request_nostradamus_entry asks for the session's member to play the edition. It returns the refusal of
-- nostradamus_member_of, 'no-edition', or the member's status: 'requested' when newly asked. A participant imported
-- under the member's TTA name, not linked yet, is linked to it and keeps its status.
CREATE FUNCTION request_nostradamus_entry(session bytea, at timestamptz, edition text) RETURNS text
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
-- submit_nostradamus_answers replaces the session's member's answers in a round with answers, a JSON list of
-- {question, answer, token}, all or none. It returns the refusal of nostradamus_member_of, 'not-open' for a round
-- unpublished or closed, 'not-entered' unless the member is accepted in the round's edition, 'past-deadline', 'bad'
-- for an answer the rules refuse, or 'saved:<n>'.
CREATE FUNCTION submit_nostradamus_answers(session bytea, at timestamptz, round bigint, answers jsonb) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    r record;
    participant bigint;
    saved integer;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN who.refusal;
    END IF;
    SELECT x.edition_id, x.published_at, x.closed_at, x.closes_at INTO r FROM nostradamus_rounds x WHERE x.round_id = round;
    IF NOT FOUND OR r.published_at IS NULL OR r.closed_at IS NOT NULL THEN
        RETURN 'not-open';
    END IF;
    SELECT p.participant_id INTO participant FROM nostradamus_participants p
    WHERE p.edition_id = r.edition_id AND p.member_id = who.member AND p.status = 'accepted';
    IF NOT FOUND THEN
        RETURN 'not-entered';
    END IF;
    IF at >= r.closes_at THEN
        RETURN 'past-deadline';
    END IF;
    IF jsonb_typeof(answers) <> 'array' OR jsonb_array_length(answers) > 200
       OR EXISTS (SELECT FROM jsonb_array_elements(answers) e WHERE jsonb_typeof(e) <> 'object' OR jsonb_typeof(e->'answer') <> 'array')
       OR EXISTS (SELECT FROM jsonb_to_recordset(answers) AS a(question bigint)
                  WHERE NOT EXISTS (SELECT FROM nostradamus_questions n WHERE n.question_id = a.question AND n.round_id = round)) THEN
        RETURN 'bad';
    END IF;
    DELETE FROM nostradamus_answers a USING nostradamus_questions n
    WHERE a.question_id = n.question_id AND n.round_id = round AND a.participant_id = participant;
    INSERT INTO nostradamus_answers (question_id, participant_id, answer, token)
    SELECT a.question, participant, ARRAY(SELECT btrim(x) FROM jsonb_array_elements_text(a.answer) x), coalesce(a.token, false)
    FROM jsonb_to_recordset(answers) AS a(question bigint, answer jsonb, token boolean);
    GET DIAGNOSTICS saved = ROW_COUNT;
    RETURN 'saved:' || saved;
EXCEPTION WHEN check_violation OR unique_violation OR not_null_violation OR invalid_text_representation THEN
    RETURN 'bad';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
-- my_nostradamus returns the session's member's status in the edition ('none' when they have not asked) and their
-- answers there: a first row with the status alone, then one per answer; or one row with the refusal of
-- nostradamus_member_of as the status.
CREATE FUNCTION my_nostradamus(session bytea, at timestamptz, edition text)
    RETURNS TABLE (status text, question_id bigint, answer text[], token boolean)
    LANGUAGE plpgsql STABLE SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    who record;
    p record;
BEGIN
    who := nostradamus_member_of(session, at);
    IF who.refusal IS NOT NULL THEN
        RETURN QUERY SELECT who.refusal, NULL::bigint, NULL::text[], NULL::boolean;
        RETURN;
    END IF;
    SELECT x.participant_id, x.status INTO p FROM nostradamus_participants x JOIN nostradamus_editions e USING (edition_id)
    WHERE e.slug = edition AND x.member_id = who.member;
    IF NOT FOUND THEN
        RETURN QUERY SELECT 'none'::text, NULL::bigint, NULL::text[], NULL::boolean;
        RETURN;
    END IF;
    RETURN QUERY SELECT p.status::text, NULL::bigint, NULL::text[], NULL::boolean
        UNION ALL
        SELECT p.status::text, a.question_id, a.answer, a.token FROM nostradamus_answers a WHERE a.participant_id = p.participant_id;
END
$$;
-- +goose StatementEnd

REVOKE ALL ON FUNCTION nostradamus_member_of(bytea, timestamptz) FROM PUBLIC;
REVOKE ALL ON FUNCTION request_nostradamus_entry(bytea, timestamptz, text) FROM PUBLIC;
REVOKE ALL ON FUNCTION submit_nostradamus_answers(bytea, timestamptz, bigint, jsonb) FROM PUBLIC;
REVOKE ALL ON FUNCTION my_nostradamus(bytea, timestamptz, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION request_nostradamus_entry(bytea, timestamptz, text), submit_nostradamus_answers(bytea, timestamptz, bigint, jsonb),
    my_nostradamus(bytea, timestamptz, text) TO ratings_player;

GRANT SELECT ON nostradamus_editions, nostradamus_rounds, nostradamus_questions, nostradamus_choices, nostradamus_participants,
    nostradamus_answers, nostradamus_round_scores, nostradamus_answer_counts TO ratings_admin, ratings_backup;
-- The site's role never reads an answer: a player sees only their own, through my_nostradamus.
GRANT SELECT ON nostradamus_editions, nostradamus_rounds, nostradamus_questions, nostradamus_choices, nostradamus_participants,
    nostradamus_round_scores, nostradamus_answer_counts TO ratings_read;
REVOKE SELECT ON nostradamus_answers FROM ratings_read;
GRANT INSERT, UPDATE ON nostradamus_editions TO ratings_admin;
GRANT INSERT, UPDATE, DELETE ON nostradamus_rounds, nostradamus_questions, nostradamus_choices, nostradamus_participants,
    nostradamus_answers TO ratings_admin;

-- +goose Down
DROP FUNCTION my_nostradamus(bytea, timestamptz, text);
DROP FUNCTION submit_nostradamus_answers(bytea, timestamptz, bigint, jsonb);
DROP FUNCTION request_nostradamus_entry(bytea, timestamptz, text);
DROP FUNCTION nostradamus_member_of(bytea, timestamptz);
DROP VIEW nostradamus_answer_counts;
DROP VIEW nostradamus_round_scores;
DROP TABLE nostradamus_answers;
DROP TABLE nostradamus_participants;
DROP TABLE nostradamus_choices;
DROP TABLE nostradamus_questions;
DROP TABLE nostradamus_rounds;
DROP TABLE nostradamus_editions;
DROP FUNCTION nostradamus_points(text, integer, integer, text[], text[], boolean, integer);
DROP FUNCTION nostradamus_check_answer();
DROP FUNCTION nostradamus_valid(text, text[]);
