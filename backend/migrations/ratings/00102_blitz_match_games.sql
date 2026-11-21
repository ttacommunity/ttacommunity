-- Blitz match games: a match is played as games, each kept here with its number, its game, the game's own result
-- and where a player's report of it stands. A knockout round may offer best of 3 (best_of): a match is one game
-- until its players agree to three (games), one proposing (proposed_by) and the other accepting before game 1. The
-- match keeps its result: blitz_match_result works it out from the games, unless an organiser's ruling decides it.
-- +goose Up
CREATE TABLE blitz_match_games (
    match_id bigint NOT NULL REFERENCES blitz_matches ON DELETE CASCADE,
    number integer NOT NULL CHECK (number BETWEEN 1 AND 3),
    game_id bigint NOT NULL UNIQUE REFERENCES games,
    result text NOT NULL CHECK (result IN ('a', 'b', 'draw')),
    reported_by bigint REFERENCES blitz_entries,
    report text CHECK (report IN ('reported', 'confirmed', 'flagged', 'resolved')),
    flag_reason text NOT NULL DEFAULT '' CHECK (char_length(flag_reason) <= 500),
    PRIMARY KEY (match_id, number),
    CONSTRAINT blitz_match_games_reported_check CHECK ((report IS NULL) = (reported_by IS NULL)),
    -- A flag's reason stays once an organiser resolves it.
    CONSTRAINT blitz_match_games_flag_check CHECK (CASE report WHEN 'flagged' THEN flag_reason <> '' WHEN 'resolved' THEN true ELSE flag_reason = '' END)
);
GRANT SELECT, INSERT, UPDATE, DELETE ON blitz_match_games TO ratings_admin;
GRANT SELECT ON blitz_match_games TO ratings_read, ratings_backup;

-- Seat 1 is the match's first player, so the game's result is whichever seat placed better.
INSERT INTO blitz_match_games (match_id, number, game_id, result, reported_by, report, flag_reason)
SELECT m.match_id, 1, m.game_id, CASE WHEN sa.place < sb.place THEN 'a' WHEN sa.place > sb.place THEN 'b' ELSE 'draw' END,
       m.reported_by, m.report, m.flag_reason
FROM blitz_matches m JOIN seats sa ON sa.game_id = m.game_id AND sa.seat = 1 JOIN seats sb ON sb.game_id = m.game_id AND sb.seat = 2;
ALTER TABLE blitz_matches
    DROP CONSTRAINT blitz_matches_result_source_check,
    DROP CONSTRAINT blitz_matches_reported_check,
    DROP CONSTRAINT blitz_matches_reporter_check,
    DROP CONSTRAINT blitz_matches_flag_check,
    DROP COLUMN game_id,
    DROP COLUMN reported_by,
    DROP COLUMN report,
    DROP COLUMN flag_reason,
    ADD COLUMN games integer NOT NULL DEFAULT 1 CHECK (games IN (1, 3)),
    ADD COLUMN proposed_by bigint REFERENCES blitz_entries,
    ADD CONSTRAINT blitz_matches_proposal_check CHECK (proposed_by IS NULL OR (games = 1 AND proposed_by IN (entry_a, entry_b)));
ALTER TABLE blitz_rounds
    ADD COLUMN best_of integer NOT NULL DEFAULT 1 CHECK (best_of IN (1, 3)),
    ADD CONSTRAINT blitz_rounds_best_of_knockout_check CHECK (best_of = 1 OR kind = 'knockout');
ALTER TABLE notification_outbox ADD COLUMN game_number integer CHECK (game_number BETWEEN 1 AND 3);

-- +goose StatementBegin
CREATE FUNCTION blitz_games_result(id bigint) RETURNS text
    LANGUAGE sql STABLE
    SET search_path TO public, pg_temp
    AS $$
    -- One game decides a one-game match, a drawn knockout game sending the higher seed through. Best of 3: a win is
    -- 1 and a draw ½, the first to 2 wins, and the higher seed takes a match level after three.
    SELECT CASE
        WHEN x.games = 1 AND s.first = 'draw' AND r.kind = 'knockout' THEN CASE WHEN x.seed_a < x.seed_b THEN 'a' ELSE 'b' END
        WHEN x.games = 1 THEN s.first
        WHEN s.a >= 2 THEN 'a'
        WHEN s.b >= 2 THEN 'b'
        WHEN s.played = 3 THEN CASE WHEN x.seed_a < x.seed_b THEN 'a' ELSE 'b' END
    END
    FROM blitz_matches x JOIN blitz_rounds r USING (round_id)
    CROSS JOIN LATERAL (
        SELECT count(*) AS played, max(g.result) FILTER (WHERE g.number = 1) AS first,
               coalesce(sum(CASE g.result WHEN 'a' THEN 1 WHEN 'draw' THEN 0.5 ELSE 0 END), 0) AS a,
               coalesce(sum(CASE g.result WHEN 'b' THEN 1 WHEN 'draw' THEN 0.5 ELSE 0 END), 0) AS b
        FROM blitz_match_games g WHERE g.match_id = x.match_id) s
    WHERE x.match_id = id;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION blitz_games_result(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION blitz_games_result(bigint) TO ratings_admin;

-- +goose StatementBegin
CREATE FUNCTION blitz_game_open(id bigint, game_number integer) RETURNS text
    LANGUAGE sql STABLE
    SET search_path TO public, pg_temp
    AS $$
    -- Whether a match takes game game_number: one of its agreed games, after the one before it, and while the games
    -- so far have not decided it (a game it has can always be corrected).
    SELECT CASE
        WHEN game_number < 1 OR game_number > x.games THEN 'no-game'
        WHEN game_number > 1 AND NOT EXISTS (SELECT 1 FROM blitz_match_games g WHERE g.match_id = id AND g.number = game_number - 1) THEN 'no-game'
        WHEN NOT EXISTS (SELECT 1 FROM blitz_match_games g WHERE g.match_id = id AND g.number = game_number)
             AND blitz_games_result(id) IS NOT NULL THEN 'not-needed'
        ELSE 'ok'
    END
    FROM blitz_matches x WHERE x.match_id = id;
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION blitz_game_open(bigint, integer) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION blitz_game_open(bigint, integer) TO ratings_admin;

-- +goose StatementBegin
CREATE FUNCTION blitz_match_result(id bigint, at timestamptz) RETURNS void
    LANGUAGE sql
    SET search_path TO public, pg_temp
    AS $$
    UPDATE blitz_matches m SET result = d.result, decided_at = CASE WHEN d.result IS NULL THEN NULL ELSE at END
    FROM (SELECT blitz_games_result(id) AS result) d
    WHERE m.match_id = id AND m.ruling = '';
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION blitz_match_result(bigint, timestamptz) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION blitz_match_result(bigint, timestamptz) TO ratings_admin;

DROP FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb);
-- +goose StatementBegin
CREATE FUNCTION report_blitz_game(session bytea, at timestamptz, slug text, edition text, id bigint, game_number integer,
                                  code text, finished timestamptz, result text, sides jsonb) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    m record;
    g record;
    w record;
    open text;
BEGIN
    IF finished > date_trunc('minute', at) THEN
        RETURN 'ahead';
    END IF;
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    open := blitz_game_open(id, game_number);
    IF open <> 'ok' THEN
        RETURN open;
    END IF;
    SELECT bm.*, r.number AS round, r.tournament_id, s.name AS series, ma.tta_name AS name_a, mb.tta_name AS name_b,
           ma.player_id AS player_a, mb.player_id AS player_b, ma.cge_user AS cge_a, mb.cge_user AS cge_b INTO m
    FROM blitz_matches bm JOIN blitz_rounds r USING (round_id) JOIN tournaments tt ON tt.tournament_id = r.tournament_id
    JOIN series s USING (series_id)
    JOIN blitz_entries xa ON xa.entry_id = bm.entry_a JOIN members ma ON ma.member_id = xa.member_id
    JOIN blitz_entries xb ON xb.entry_id = bm.entry_b JOIN members mb ON mb.member_id = xb.member_id
    WHERE bm.match_id = id;
    SELECT * INTO g FROM blitz_match_games bg WHERE bg.match_id = id AND bg.number = game_number;
    IF m.ruling <> '' OR (g.game_id IS NOT NULL AND (g.report IS DISTINCT FROM 'reported' OR g.reported_by <> t.entry)) THEN
        RETURN 'locked';
    END IF;
    SELECT * INTO w FROM write_game(m.tournament_id, 'Round ' || m.round,
        format('%s %s Round %s', m.series, edition, m.round) || CASE WHEN m.games > 1 THEN ' Game ' || game_number ELSE '' END,
        finished, code, 'player', 'blitz match ' || id || CASE WHEN game_number > 1 THEN ' game ' || game_number ELSE '' END,
        jsonb_build_array(sides->0 || jsonb_build_object('player', m.player_a, 'name', m.name_a, 'cge_user', m.cge_a),
                          sides->1 || jsonb_build_object('player', m.player_b, 'name', m.name_b, 'cge_user', m.cge_b)), g.game_id);
    IF w.taken_by IS NOT NULL THEN
        RETURN 'taken:' || w.taken_by;
    END IF;
    INSERT INTO blitz_match_games (match_id, number, game_id, result, reported_by, report)
    VALUES (id, game_number, w.written, report_blitz_game.result, t.entry, 'reported')
    ON CONFLICT (match_id, number) DO UPDATE SET result = excluded.result, reported_by = excluded.reported_by, report = 'reported', flag_reason = '';
    -- A proposal not accepted before game 1 lapses.
    UPDATE blitz_matches SET proposed_by = NULL WHERE match_id = id;
    PERFORM blitz_match_result(id, at);
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'blitz.match.report', id::text,
            jsonb_build_object('account', t.account, 'game', game_number, 'share_code', code, 'finished', finished, 'sides', sides));
    RETURN 'reported';
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, integer, text, timestamptz, text, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, integer, text, timestamptz, text, jsonb) TO ratings_player;

DROP FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text);
-- +goose StatementBegin
CREATE FUNCTION answer_blitz_report(session bytea, at timestamptz, slug text, edition text, id bigint, game_number integer,
                                    answer text, reason text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    g record;
BEGIN
    IF answer NOT IN ('confirmed', 'flagged') OR (answer = 'flagged') = (coalesce(btrim(reason), '') = '')
       OR char_length(btrim(reason)) > 500 THEN
        RETURN 'bad';
    END IF;
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    SELECT bg.report, bg.reported_by INTO g FROM blitz_match_games bg WHERE bg.match_id = id AND bg.number = game_number;
    IF g.report IS DISTINCT FROM 'reported' OR g.reported_by = t.entry THEN
        RETURN 'locked';
    END IF;
    UPDATE blitz_match_games SET report = answer, flag_reason = coalesce(btrim(reason), '') WHERE match_id = id AND number = game_number;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', CASE answer WHEN 'confirmed' THEN 'blitz.match.confirm' ELSE 'blitz.match.flag' END, id::text,
            jsonb_strip_nulls(jsonb_build_object('account', t.account, 'game', game_number, 'reason', nullif(btrim(reason), ''))));
    RETURN answer;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, integer, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, integer, text, text) TO ratings_player;

-- +goose StatementBegin
CREATE FUNCTION answer_blitz_best_of(session bytea, at timestamptz, slug text, edition text, id bigint, answer text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    m record;
BEGIN
    IF answer NOT IN ('propose', 'accept', 'decline', 'withdraw') THEN
        RETURN 'bad-best-of';
    END IF;
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    SELECT bm.games, bm.proposed_by, bm.ruling, r.best_of INTO m FROM blitz_matches bm JOIN blitz_rounds r USING (round_id) WHERE bm.match_id = id;
    IF m.best_of <> 3 THEN
        RETURN 'not-offered';
    END IF;
    IF m.games = 3 OR m.ruling <> '' OR EXISTS (SELECT 1 FROM blitz_match_games g WHERE g.match_id = id) THEN
        RETURN 'best-of-closed';
    END IF;
    IF (answer IN ('accept', 'decline') AND (m.proposed_by IS NULL OR m.proposed_by = t.entry))
       OR (answer = 'withdraw' AND m.proposed_by IS DISTINCT FROM t.entry) THEN
        RETURN 'bad-best-of';
    END IF;
    -- A proposal answered by one of the opponent's is agreed.
    UPDATE blitz_matches
    SET games = CASE WHEN answer = 'accept' OR (answer = 'propose' AND proposed_by <> t.entry) THEN 3 ELSE 1 END,
        proposed_by = CASE WHEN answer = 'propose' AND proposed_by IS NULL THEN t.entry
                           WHEN answer = 'propose' AND proposed_by = t.entry THEN proposed_by END
    WHERE match_id = id;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'blitz.match.best_of', id::text, jsonb_build_object('account', t.account, 'answer', answer));
    RETURN 'answered';
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION answer_blitz_best_of(bytea, timestamptz, text, text, bigint, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION answer_blitz_best_of(bytea, timestamptz, text, text, bigint, text) TO ratings_player;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_blitz_game(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    batch bigint;
    grp bigint;
    stage bigint;
BEGIN
    SELECT g.batch_id, g.group_id, gr.stage_id INTO batch, grp, stage
    FROM games g JOIN import_batches b ON b.batch_id = g.batch_id JOIN groups gr ON gr.group_id = g.group_id
    JOIN stages st ON st.stage_id = gr.stage_id JOIN blitz_editions e ON e.tournament_id = st.tournament_id
    WHERE g.game_id = id AND b.source IN ('admin', 'player') AND st.label ~ '^Round [0-9]+$'
      AND NOT EXISTS (SELECT 1 FROM blitz_match_games m WHERE m.game_id = g.game_id)
    FOR UPDATE OF g;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'game % is not a blitz game admin or a player recorded, or a match still holds it', id;
    END IF;
    DELETE FROM tournament_games WHERE match_id = id;
    DELETE FROM pairwise_matches WHERE match_id = id;
    DELETE FROM games WHERE game_id = id;
    DELETE FROM import_batches b WHERE b.batch_id = batch
      AND NOT EXISTS (SELECT 1 FROM games x WHERE x.batch_id = batch) AND NOT EXISTS (SELECT 1 FROM refused_rows r WHERE r.batch_id = batch);
    DELETE FROM group_members gm WHERE gm.group_id = grp
      AND NOT EXISTS (SELECT 1 FROM games x JOIN seats s USING (game_id) WHERE x.group_id = grp AND s.player_id = gm.player_id);
    DELETE FROM groups gr WHERE gr.group_id = grp AND NOT EXISTS (SELECT 1 FROM games x WHERE x.group_id = grp);
    DELETE FROM stages st WHERE st.stage_id = stage AND NOT EXISTS (SELECT 1 FROM groups x WHERE x.stage_id = stage);
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DO $$
DECLARE
    several text;
BEGIN
    SELECT string_agg(DISTINCT match_id::text, ', ') INTO several FROM blitz_match_games WHERE number > 1;
    IF several IS NOT NULL THEN
        RAISE EXCEPTION 'blitz matches % have more than one game, which the old schema cannot hold: reset them first', several;
    END IF;
END $$;
-- +goose StatementEnd
DROP FUNCTION answer_blitz_best_of(bytea, timestamptz, text, text, bigint, text);
DROP FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, integer, text, timestamptz, text, jsonb);
DROP FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, integer, text, text);
ALTER TABLE notification_outbox DROP COLUMN game_number;
ALTER TABLE blitz_rounds DROP CONSTRAINT blitz_rounds_best_of_knockout_check, DROP COLUMN best_of;
ALTER TABLE blitz_matches
    DROP CONSTRAINT blitz_matches_proposal_check,
    DROP COLUMN games,
    DROP COLUMN proposed_by,
    ADD COLUMN game_id bigint UNIQUE REFERENCES games,
    ADD COLUMN reported_by bigint REFERENCES blitz_entries,
    ADD COLUMN report text CHECK (report IN ('reported', 'confirmed', 'flagged', 'resolved')),
    ADD COLUMN flag_reason text NOT NULL DEFAULT '' CHECK (char_length(flag_reason) <= 500);
UPDATE blitz_matches m SET game_id = g.game_id, reported_by = g.reported_by, report = g.report, flag_reason = g.flag_reason
FROM blitz_match_games g WHERE g.match_id = m.match_id AND g.number = 1;
ALTER TABLE blitz_matches
    ADD CONSTRAINT blitz_matches_result_source_check CHECK ((result IS NULL) = (game_id IS NULL AND ruling = '')),
    ADD CONSTRAINT blitz_matches_reported_check CHECK ((report IS NULL) = (reported_by IS NULL) AND (report IS NULL OR game_id IS NOT NULL)),
    ADD CONSTRAINT blitz_matches_reporter_check CHECK (reported_by IN (entry_a, entry_b)),
    ADD CONSTRAINT blitz_matches_flag_check CHECK (CASE report WHEN 'flagged' THEN flag_reason <> '' WHEN 'resolved' THEN true ELSE flag_reason = '' END);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION report_blitz_game(session bytea, at timestamptz, slug text, edition text, id bigint, code text,
                                  finished timestamptz, result text, sides jsonb) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    m record;
    w record;
BEGIN
    IF finished > date_trunc('minute', at) THEN
        RETURN 'ahead';
    END IF;
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    SELECT bm.*, r.kind, r.number, r.tournament_id, s.name AS series, ma.tta_name AS name_a, mb.tta_name AS name_b,
           ma.player_id AS player_a, mb.player_id AS player_b, ma.cge_user AS cge_a, mb.cge_user AS cge_b INTO m
    FROM blitz_matches bm JOIN blitz_rounds r USING (round_id) JOIN tournaments tt ON tt.tournament_id = r.tournament_id
    JOIN series s USING (series_id)
    JOIN blitz_entries xa ON xa.entry_id = bm.entry_a JOIN members ma ON ma.member_id = xa.member_id
    JOIN blitz_entries xb ON xb.entry_id = bm.entry_b JOIN members mb ON mb.member_id = xb.member_id
    WHERE bm.match_id = id;
    IF m.result IS NOT NULL AND (m.report IS DISTINCT FROM 'reported' OR m.reported_by <> t.entry) THEN
        RETURN 'locked';
    END IF;
    -- A drawn knockout game sends the higher seed through.
    IF m.kind = 'knockout' AND result = 'draw' THEN
        result := CASE WHEN m.seed_a < m.seed_b THEN 'a' ELSE 'b' END;
    END IF;
    SELECT * INTO w FROM write_game(m.tournament_id, 'Round ' || m.number, format('%s %s Round %s', m.series, edition, m.number), finished,
        code, 'player', 'blitz match ' || id,
        jsonb_build_array(sides->0 || jsonb_build_object('player', m.player_a, 'name', m.name_a, 'cge_user', m.cge_a),
                          sides->1 || jsonb_build_object('player', m.player_b, 'name', m.name_b, 'cge_user', m.cge_b)), m.game_id);
    IF w.taken_by IS NOT NULL THEN
        RETURN 'taken:' || w.taken_by;
    END IF;
    UPDATE blitz_matches SET result = report_blitz_game.result, game_id = w.written, ruling = '', decided_at = at,
        reported_by = t.entry, report = 'reported'
    WHERE match_id = id;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', 'blitz.match.report', id::text,
            jsonb_build_object('account', t.account, 'share_code', code, 'finished', finished, 'sides', sides));
    RETURN 'reported';
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION answer_blitz_report(session bytea, at timestamptz, slug text, edition text, id bigint, answer text,
                                    reason text) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    m record;
BEGIN
    IF answer NOT IN ('confirmed', 'flagged') OR (answer = 'flagged') = (coalesce(btrim(reason), '') = '')
       OR char_length(btrim(reason)) > 500 THEN
        RETURN 'bad';
    END IF;
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    SELECT bm.report, bm.reported_by INTO m FROM blitz_matches bm WHERE bm.match_id = id;
    IF m.report IS DISTINCT FROM 'reported' OR m.reported_by = t.entry THEN
        RETURN 'locked';
    END IF;
    UPDATE blitz_matches SET report = answer, flag_reason = coalesce(btrim(reason), '') WHERE match_id = id;
    INSERT INTO admin_audit (at, discord_id, action, target, detail)
    VALUES (at, '', CASE answer WHEN 'confirmed' THEN 'blitz.match.confirm' ELSE 'blitz.match.flag' END, id::text,
            jsonb_strip_nulls(jsonb_build_object('account', t.account, 'reason', nullif(btrim(reason), ''))));
    RETURN answer;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_blitz_game(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    batch bigint;
    grp bigint;
    stage bigint;
BEGIN
    SELECT g.batch_id, g.group_id, gr.stage_id INTO batch, grp, stage
    FROM games g JOIN import_batches b ON b.batch_id = g.batch_id JOIN groups gr ON gr.group_id = g.group_id
    JOIN stages st ON st.stage_id = gr.stage_id JOIN blitz_editions e ON e.tournament_id = st.tournament_id
    WHERE g.game_id = id AND b.source IN ('admin', 'player') AND st.label ~ '^Round [0-9]+$'
      AND NOT EXISTS (SELECT 1 FROM blitz_matches m WHERE m.game_id = g.game_id)
    FOR UPDATE OF g;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'game % is not a blitz game admin or a player recorded, or a match still holds it', id;
    END IF;
    DELETE FROM tournament_games WHERE match_id = id;
    DELETE FROM pairwise_matches WHERE match_id = id;
    DELETE FROM games WHERE game_id = id;
    DELETE FROM import_batches b WHERE b.batch_id = batch
      AND NOT EXISTS (SELECT 1 FROM games x WHERE x.batch_id = batch) AND NOT EXISTS (SELECT 1 FROM refused_rows r WHERE r.batch_id = batch);
    DELETE FROM group_members gm WHERE gm.group_id = grp
      AND NOT EXISTS (SELECT 1 FROM games x JOIN seats s USING (game_id) WHERE x.group_id = grp AND s.player_id = gm.player_id);
    DELETE FROM groups gr WHERE gr.group_id = grp AND NOT EXISTS (SELECT 1 FROM games x WHERE x.group_id = grp);
    DELETE FROM stages st WHERE st.stage_id = stage AND NOT EXISTS (SELECT 1 FROM groups x WHERE x.stage_id = stage);
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb) TO ratings_player;
REVOKE ALL ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text) TO ratings_player;
DROP FUNCTION blitz_match_result(bigint, timestamptz);
DROP FUNCTION blitz_game_open(bigint, integer);
DROP FUNCTION blitz_games_result(bigint);
DROP TABLE blitz_match_games;
