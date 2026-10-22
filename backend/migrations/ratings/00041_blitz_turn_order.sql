-- A blitz game keeps its turn order: write_game stores each seat's position (0 for the player who went first) and
-- refuses a game whose positions are not 0 to N-1, each once, so a player's report cannot skip it.
-- +goose Up
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
        INSERT INTO seats (game_id, seat, player_id, player_name, outcome, culture, place, starting_position)
        VALUES (written, n, player, spelled, s->>'outcome', (s->>'culture')::double precision, (s->>'place')::integer,
                (s->>'position')::smallint)
        ON CONFLICT (game_id, seat) DO UPDATE SET player_id = excluded.player_id, player_name = excluded.player_name,
            outcome = excluded.outcome, culture = excluded.culture, place = excluded.place,
            starting_position = excluded.starting_position;
        INSERT INTO group_members (group_id, player_id) VALUES (grp, player) ON CONFLICT DO NOTHING;
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose Down
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
