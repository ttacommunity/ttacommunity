-- Nostradamus: a prediction game over a tournament, in rounds of questions that participants answer, putting tokens
-- on some. A participant keeps the name it answered under, linked to the member of that TTA name where one exists.
-- Scoring lives here: nostradamus_points scores an answer, nostradamus_round_scores a participant's round.
-- +goose Up
CREATE TABLE nostradamus_editions (
    edition_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z0-9-]{1,40}$'),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    tournament_id bigint NOT NULL REFERENCES tournaments ON DELETE CASCADE
);

CREATE TABLE nostradamus_rounds (
    round_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    edition_id bigint NOT NULL REFERENCES nostradamus_editions ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    label text NOT NULL CHECK (label ~ '^[A-Za-z0-9 ]{1,20}$'),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    tokens integer NOT NULL CHECK (tokens BETWEEN 0 AND 10),
    token_points integer NOT NULL CHECK (token_points BETWEEN 0 AND 100),
    closes_at timestamptz NOT NULL,
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

CREATE TABLE nostradamus_questions (
    question_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    round_id bigint NOT NULL REFERENCES nostradamus_rounds ON DELETE CASCADE,
    position integer NOT NULL CHECK (position >= 0),
    label text NOT NULL CHECK (label <> '' AND char_length(label) <= 60),
    prompt text NOT NULL DEFAULT '' CHECK (char_length(prompt) <= 500),
    kind text NOT NULL CHECK (kind IN ('pick', 'set', 'number')),
    points integer NOT NULL CHECK (points BETWEEN 0 AND 100),
    near_points integer NOT NULL DEFAULT 0 CHECK (near_points BETWEEN 0 AND points AND (kind = 'number' OR near_points = 0)),
    -- A pick's outcome names everyone who tied.
    outcome text[] CHECK (outcome IS NULL OR nostradamus_valid(CASE kind WHEN 'pick' THEN 'set' ELSE kind END, outcome)),
    UNIQUE (round_id, position)
);
CREATE UNIQUE INDEX nostradamus_questions_label ON nostradamus_questions (round_id, lower(label));

CREATE TABLE nostradamus_participants (
    participant_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    edition_id bigint NOT NULL REFERENCES nostradamus_editions ON DELETE CASCADE,
    name text NOT NULL CHECK (btrim(name) = name AND name <> '' AND char_length(name) <= 100),
    member_id bigint REFERENCES members ON DELETE SET NULL,
    bot boolean NOT NULL DEFAULT false
);
CREATE UNIQUE INDEX nostradamus_participants_name ON nostradamus_participants (edition_id, lower(name));
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
-- nostradamus_check_answer refuses an answer not of its question's kind, from another edition's participant, or
-- whose token takes its participant past the round's tokens.
CREATE FUNCTION nostradamus_check_answer() RETURNS trigger
    LANGUAGE plpgsql
    AS $$
DECLARE
    q record;
BEGIN
    SELECT n.kind, n.round_id, r.edition_id, r.tokens INTO q
    FROM nostradamus_questions n JOIN nostradamus_rounds r USING (round_id)
    WHERE n.question_id = NEW.question_id;
    IF NOT nostradamus_valid(q.kind, NEW.answer) THEN
        RAISE EXCEPTION 'not a % answer', q.kind USING ERRCODE = 'check_violation';
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

-- Each participant's points in each round with a known outcome. A participant who did not answer the round scores
-- the lowest of the bots that did, or nothing without one.
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
       coalesce(s.points, b.points, 0) AS points
FROM nostradamus_rounds r
JOIN nostradamus_participants p USING (edition_id)
LEFT JOIN answered s ON s.round_id = r.round_id AND s.participant_id = p.participant_id
LEFT JOIN lowest_bot b ON b.round_id = r.round_id
WHERE EXISTS (SELECT FROM nostradamus_questions n WHERE n.round_id = r.round_id AND n.outcome IS NOT NULL);

GRANT SELECT ON nostradamus_editions, nostradamus_rounds, nostradamus_questions, nostradamus_participants,
    nostradamus_answers, nostradamus_round_scores TO ratings_read, ratings_admin, ratings_backup;
GRANT UPDATE (outcome) ON nostradamus_questions TO ratings_admin;
GRANT UPDATE (member_id) ON nostradamus_participants TO ratings_admin;

-- +goose Down
DROP VIEW nostradamus_round_scores;
DROP TABLE nostradamus_answers;
DROP TABLE nostradamus_participants;
DROP TABLE nostradamus_questions;
DROP TABLE nostradamus_rounds;
DROP TABLE nostradamus_editions;
DROP FUNCTION nostradamus_points(text, integer, integer, text[], text[], boolean, integer);
DROP FUNCTION nostradamus_check_answer();
DROP FUNCTION nostradamus_valid(text, text[]);
