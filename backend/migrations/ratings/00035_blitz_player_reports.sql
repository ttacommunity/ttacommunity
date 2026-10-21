-- Players report their own blitz games: each match keeps who reported its game and where the report stands
-- (reported, confirmed, flagged with a reason, or resolved by an organiser). write_game writes any tournament's game
-- and is granted to nobody; report_blitz_game and answer_blitz_report (confirm or flag) check a player's session and
-- blitz's rules, and are the player login's only way to write a game.
-- +goose Up
ALTER TABLE import_batches DROP CONSTRAINT import_batches_source_check;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_source_check CHECK (source IN ('load', 'admin', 'player'));
ALTER TABLE blitz_matches
    ADD COLUMN reported_by bigint REFERENCES blitz_entries,
    ADD COLUMN report text CHECK (report IN ('reported', 'confirmed', 'flagged', 'resolved')),
    ADD COLUMN flag_reason text NOT NULL DEFAULT '' CHECK (char_length(flag_reason) <= 500),
    ADD CONSTRAINT blitz_matches_reported_check CHECK ((report IS NULL) = (reported_by IS NULL) AND (report IS NULL OR game_id IS NOT NULL)),
    ADD CONSTRAINT blitz_matches_reporter_check CHECK (reported_by IN (entry_a, entry_b)),
    -- A flag's reason stays once an organiser resolves it.
    ADD CONSTRAINT blitz_matches_flag_check CHECK (CASE report WHEN 'flagged' THEN flag_reason <> '' WHEN 'resolved' THEN true ELSE flag_reason = '' END);

-- +goose StatementBegin
CREATE FUNCTION write_game(tournament bigint, stage text, name text, finished timestamptz, code text, source text,
                           source_file text, seats jsonb, game bigint, OUT written bigint, OUT taken_by bigint)
    LANGUAGE plpgsql
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    stg bigint;
    grp bigint;
    batch bigint;
    s jsonb;
    n integer := 0;
    player bigint;
    spelled text;
BEGIN
    SELECT g.game_id INTO taken_by FROM games g WHERE g.share_code = nullif(code, '') AND g.game_id IS DISTINCT FROM game;
    IF FOUND THEN
        RETURN;
    END IF;
    INSERT INTO stages (tournament_id, label) VALUES (tournament, stage) ON CONFLICT (tournament_id, label) DO NOTHING;
    SELECT st.stage_id INTO stg FROM stages st WHERE st.tournament_id = tournament AND st.label = stage;
    INSERT INTO groups (stage_id, name) VALUES (stg, '') ON CONFLICT DO NOTHING;
    SELECT g.group_id INTO grp FROM groups g WHERE g.stage_id = stg AND g.name = '' AND g.cge_tournament IS NULL;
    written := game;
    IF written IS NULL THEN
        INSERT INTO import_batches (source) VALUES (source) RETURNING batch_id INTO batch;
        INSERT INTO games (batch_id, source_file, source_line, name, date, finished_at, share_code, state, group_id)
        VALUES (batch, source_file, 0, name, (finished AT TIME ZONE 'UTC')::date, finished, nullif(code, ''), 'played', grp)
        RETURNING games.game_id INTO written;
    ELSE
        UPDATE games g SET share_code = nullif(code, ''), date = (finished AT TIME ZONE 'UTC')::date, finished_at = finished,
            name = write_game.name, group_id = grp
        WHERE g.game_id = written;
    END IF;
    FOR s IN SELECT * FROM jsonb_array_elements(seats) LOOP
        n := n + 1;
        INSERT INTO players (name) SELECT s->>'name' WHERE NOT EXISTS (SELECT FROM players p WHERE lower(p.name) = lower(s->>'name'));
        SELECT p.player_id, p.name INTO player, spelled FROM players p WHERE lower(p.name) = lower(s->>'name') ORDER BY p.player_id LIMIT 1;
        INSERT INTO seats (game_id, seat, player_id, player_name, outcome, culture, place)
        VALUES (written, n, player, spelled, s->>'outcome', (s->>'culture')::double precision, (s->>'place')::integer)
        ON CONFLICT (game_id, seat) DO UPDATE SET player_id = excluded.player_id, player_name = excluded.player_name,
            outcome = excluded.outcome, culture = excluded.culture, place = excluded.place;
        INSERT INTO group_members (group_id, player_id) VALUES (grp, player) ON CONFLICT DO NOTHING;
    END LOOP;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION write_game(bigint, text, text, timestamptz, text, text, text, jsonb, bigint) FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION blitz_report_target(session bytea, at timestamptz, slug text, edition text, id bigint,
                                    OUT status text, OUT account bigint, OUT entry bigint)
    LANGUAGE plpgsql
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    member bigint;
    banned boolean;
    ed bigint;
    m record;
BEGIN
    SELECT a.account_id, a.member_id INTO account, member
    FROM player_sessions s JOIN logins l USING (login_id) JOIN accounts a USING (account_id)
    WHERE s.token_hash = session AND s.expires_at > at;
    IF NOT FOUND THEN
        status := 'signed-out';
        RETURN;
    END IF;
    IF member IS NULL THEN
        status := 'not-linked';
        RETURN;
    END IF;
    SELECT mm.banned INTO banned FROM members mm WHERE mm.member_id = member;
    IF banned THEN
        status := 'banned';
        RETURN;
    END IF;
    SELECT t.tournament_id INTO ed
    FROM tournaments t JOIN series s USING (series_id) JOIN blitz_editions e ON e.tournament_id = t.tournament_id
    WHERE s.slug = blitz_report_target.slug AND t.label = edition AND s.blitz AND s.visible;
    IF NOT FOUND THEN
        status := 'no-edition';
        RETURN;
    END IF;
    SELECT bm.*, r.number, r.published_at, xa.member_id AS member_a, xb.member_id AS member_b INTO m
    FROM blitz_matches bm JOIN blitz_rounds r USING (round_id)
    JOIN blitz_entries xa ON xa.entry_id = bm.entry_a LEFT JOIN blitz_entries xb ON xb.entry_id = bm.entry_b
    WHERE bm.match_id = id AND r.tournament_id = ed
    FOR UPDATE OF bm;
    IF NOT FOUND OR m.published_at IS NULL THEN
        status := 'no-match';
        RETURN;
    END IF;
    IF member = m.member_a THEN
        entry := m.entry_a;
    ELSIF member = m.member_b THEN
        entry := m.entry_b;
    ELSE
        status := 'not-yours';
        RETURN;
    END IF;
    IF m.entry_b IS NULL THEN
        status := 'bye';
        RETURN;
    END IF;
    IF m.number <> (SELECT max(r.number) FROM blitz_rounds r WHERE r.tournament_id = ed) THEN
        status := 'closed';
        RETURN;
    END IF;
    status := 'ok';
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION blitz_report_target(bytea, timestamptz, text, text, bigint) FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION report_blitz_game(session bytea, at timestamptz, slug text, edition text, id bigint, code text,
                                  finished timestamptz, result text, sides jsonb) RETURNS text
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t record;
    m record;
    w record;
BEGIN
    SELECT * INTO t FROM blitz_report_target(session, at, slug, edition, id);
    IF t.status <> 'ok' THEN
        RETURN t.status;
    END IF;
    SELECT bm.*, r.kind, r.number, r.tournament_id, s.name AS series, ma.tta_name AS name_a, mb.tta_name AS name_b INTO m
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
        jsonb_build_array(sides->0 || jsonb_build_object('name', m.name_a), sides->1 || jsonb_build_object('name', m.name_b)), m.game_id);
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
REVOKE ALL ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb) TO ratings_player;

-- +goose StatementBegin
CREATE FUNCTION answer_blitz_report(session bytea, at timestamptz, slug text, edition text, id bigint, answer text,
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
REVOKE ALL ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text) TO ratings_player;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_blitz_game(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
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

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_blitz_game(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
DECLARE
    batch bigint;
    grp bigint;
    stage bigint;
BEGIN
    SELECT g.batch_id, g.group_id, gr.stage_id INTO batch, grp, stage
    FROM games g JOIN import_batches b ON b.batch_id = g.batch_id JOIN groups gr ON gr.group_id = g.group_id
    JOIN stages st ON st.stage_id = gr.stage_id JOIN blitz_editions e ON e.tournament_id = st.tournament_id
    WHERE g.game_id = id AND b.source = 'admin' AND st.label ~ '^Round [0-9]+$'
      AND NOT EXISTS (SELECT 1 FROM blitz_matches m WHERE m.game_id = g.game_id)
    FOR UPDATE OF g;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'game % is not a blitz game admin recorded, or a match still holds it', id;
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
DROP FUNCTION answer_blitz_report(bytea, timestamptz, text, text, bigint, text, text);
DROP FUNCTION report_blitz_game(bytea, timestamptz, text, text, bigint, text, timestamptz, text, jsonb);
DROP FUNCTION blitz_report_target(bytea, timestamptz, text, text, bigint);
DROP FUNCTION write_game(bigint, text, text, timestamptz, text, text, text, jsonb, bigint);
ALTER TABLE blitz_matches DROP COLUMN reported_by, DROP COLUMN report, DROP COLUMN flag_reason;
-- A game a player reported stays, as admin's.
UPDATE import_batches SET source = 'admin' WHERE source = 'player';
ALTER TABLE import_batches DROP CONSTRAINT import_batches_source_check;
ALTER TABLE import_batches ADD CONSTRAINT import_batches_source_check CHECK (source IN ('load', 'admin'));
