-- A seat holds its player's id only: a game shows each player's current name, and the names they went by are in
-- player_names. A blitz game seats each side's member's player, so an entrant who renames keeps their player; a
-- member with no player is found or added by name, as before.
-- +goose Up
DROP VIEW game_lines;
DROP INDEX seats_player_games;
ALTER TABLE seats DROP COLUMN player_name;
CREATE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores,
       s.game_points, g.finished_at, s.player_ids
FROM games g
LEFT JOIN groups gr USING (group_id)
LEFT JOIN stages st USING (stage_id)
LEFT JOIN tournaments t USING (tournament_id)
LEFT JOIN stage_settings ss ON ss.stage_id = gr.stage_id
CROSS JOIN LATERAL (
    SELECT count(*)::smallint AS player_count,
           array_agg(name ORDER BY coalesce(starting_position, seat - 1), seat) AS players,
           array_agg(culture ORDER BY coalesce(starting_position, seat - 1), seat) AS cultures,
           array_agg(place ORDER BY coalesce(starting_position, seat - 1), seat) AS places,
           array_agg(outcome ORDER BY coalesce(starting_position, seat - 1), seat) AS outcomes,
           array_agg(coalesce(recorded_score, CASE WHEN outcome = 'finished' THEN culture ELSE -place END)
                     ORDER BY coalesce(starting_position, seat - 1), seat) AS scores,
           array_agg(CASE WHEN tb IS NULL THEN NULL
                          WHEN outcome = 'timed_out' THEN coalesce(ss.timeout_penalty, 0)
                          ELSE (SELECT avg(coalesce(tb[k], 0)) FROM generate_series(place, place + ties - 1) AS k)
                     END ORDER BY coalesce(starting_position, seat - 1), seat) AS game_points,
           array_agg(player_id ORDER BY coalesce(starting_position, seat - 1), seat) AS player_ids
    FROM (SELECT x.*, p.name, count(*) OVER (PARTITION BY x.place) AS ties,
                 coalesce(nullif(CASE count(*) OVER () WHEN 2 THEN ss.points_2p WHEN 3 THEN ss.points_3p WHEN 4 THEN ss.points_4p END, '{}'),
                          (CASE WHEN cardinality(ss.points_2p) > 0 THEN ss.points_2p
                                WHEN cardinality(ss.points_3p) > 0 THEN ss.points_3p
                                WHEN cardinality(ss.points_4p) > 0 THEN ss.points_4p END)[1:count(*) OVER ()]) AS tb
          FROM seats x JOIN players p USING (player_id) WHERE x.game_id = g.game_id) seat_rows) s;
GRANT SELECT ON game_lines TO ratings_read, ratings_admin;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION write_game(tournament bigint, stage text, name text, finished timestamptz, code text, source text,
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
BEGIN
    IF (SELECT array_agg((x->>'position')::integer ORDER BY (x->>'position')::integer) FROM jsonb_array_elements(seats) x)
       IS DISTINCT FROM (SELECT array_agg(i ORDER BY i) FROM generate_series(0, jsonb_array_length(seats) - 1) i) THEN
        RAISE EXCEPTION 'a game''s turn order is its seats'' positions 0 to %, each once', jsonb_array_length(seats) - 1;
    END IF;
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
        player := (s->>'player')::bigint;
        IF player IS NULL THEN
            INSERT INTO players (name) SELECT s->>'name' WHERE NOT EXISTS (SELECT FROM players p WHERE lower(p.name) = lower(s->>'name'));
            SELECT p.player_id INTO player FROM players p WHERE lower(p.name) = lower(s->>'name') ORDER BY p.player_id LIMIT 1;
        END IF;
        INSERT INTO seats (game_id, seat, player_id, cge_user, outcome, culture, place, starting_position)
        VALUES (written, n, player, (s->>'cge_user')::integer, s->>'outcome', (s->>'culture')::double precision,
                (s->>'place')::integer, (s->>'position')::smallint)
        ON CONFLICT (game_id, seat) DO UPDATE SET player_id = excluded.player_id,
            cge_user = excluded.cge_user,
            outcome = excluded.outcome, culture = excluded.culture, place = excluded.place,
            starting_position = excluded.starting_position;
        INSERT INTO group_members (group_id, player_id) VALUES (grp, player) ON CONFLICT DO NOTHING;
    END LOOP;
END
$$;
-- +goose StatementEnd

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


-- +goose Down
DROP VIEW game_lines;
ALTER TABLE seats ADD COLUMN player_name text;
UPDATE seats s SET player_name = p.name FROM players p WHERE p.player_id = s.player_id;
ALTER TABLE seats ALTER COLUMN player_name SET NOT NULL;
CREATE INDEX seats_player_games ON seats (player_name) INCLUDE (game_id, starting_position, seat, place, culture);

CREATE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores,
       s.game_points, g.finished_at, s.player_ids
FROM games g
LEFT JOIN groups gr USING (group_id)
LEFT JOIN stages st USING (stage_id)
LEFT JOIN tournaments t USING (tournament_id)
LEFT JOIN stage_settings ss ON ss.stage_id = gr.stage_id
CROSS JOIN LATERAL (
    SELECT count(*)::smallint AS player_count,
           array_agg(player_name ORDER BY coalesce(starting_position, seat - 1), seat) AS players,
           array_agg(culture ORDER BY coalesce(starting_position, seat - 1), seat) AS cultures,
           array_agg(place ORDER BY coalesce(starting_position, seat - 1), seat) AS places,
           array_agg(outcome ORDER BY coalesce(starting_position, seat - 1), seat) AS outcomes,
           array_agg(coalesce(recorded_score, CASE WHEN outcome = 'finished' THEN culture ELSE -place END)
                     ORDER BY coalesce(starting_position, seat - 1), seat) AS scores,
           array_agg(CASE WHEN tb IS NULL THEN NULL
                          WHEN outcome = 'timed_out' THEN coalesce(ss.timeout_penalty, 0)
                          ELSE (SELECT avg(coalesce(tb[k], 0)) FROM generate_series(place, place + ties - 1) AS k)
                     END ORDER BY coalesce(starting_position, seat - 1), seat) AS game_points,
           array_agg(player_id ORDER BY coalesce(starting_position, seat - 1), seat) AS player_ids
    FROM (SELECT x.*, count(*) OVER (PARTITION BY x.place) AS ties,
                 coalesce(nullif(CASE count(*) OVER () WHEN 2 THEN ss.points_2p WHEN 3 THEN ss.points_3p WHEN 4 THEN ss.points_4p END, '{}'),
                          (CASE WHEN cardinality(ss.points_2p) > 0 THEN ss.points_2p
                                WHEN cardinality(ss.points_3p) > 0 THEN ss.points_3p
                                WHEN cardinality(ss.points_4p) > 0 THEN ss.points_4p END)[1:count(*) OVER ()]) AS tb
          FROM seats x WHERE x.game_id = g.game_id) seat_rows) s;
GRANT SELECT ON game_lines TO ratings_read, ratings_admin;

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION write_game(tournament bigint, stage text, name text, finished timestamptz, code text, source text,
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
    IF (SELECT array_agg((x->>'position')::integer ORDER BY (x->>'position')::integer) FROM jsonb_array_elements(seats) x)
       IS DISTINCT FROM (SELECT array_agg(i ORDER BY i) FROM generate_series(0, jsonb_array_length(seats) - 1) i) THEN
        RAISE EXCEPTION 'a game''s turn order is its seats'' positions 0 to %, each once', jsonb_array_length(seats) - 1;
    END IF;
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
        INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, place, starting_position)
        VALUES (written, n, player, spelled, (s->>'cge_user')::integer, s->>'outcome', (s->>'culture')::double precision,
                (s->>'place')::integer, (s->>'position')::smallint)
        ON CONFLICT (game_id, seat) DO UPDATE SET player_id = excluded.player_id, player_name = excluded.player_name,
            cge_user = excluded.cge_user,
            outcome = excluded.outcome, culture = excluded.culture, place = excluded.place,
            starting_position = excluded.starting_position;
        INSERT INTO group_members (group_id, player_id) VALUES (grp, player) ON CONFLICT DO NOTHING;
    END LOOP;
END
$$;
-- +goose StatementEnd

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
           ma.cge_user AS cge_a, mb.cge_user AS cge_b INTO m
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
        jsonb_build_array(sides->0 || jsonb_build_object('name', m.name_a, 'cge_user', m.cge_a),
                          sides->1 || jsonb_build_object('name', m.name_b, 'cge_user', m.cge_b)), m.game_id);
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
