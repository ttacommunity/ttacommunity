-- A run's results and the recorded facts about players hold the player's id, not their name, so a rename changes
-- nothing they say. The games give the engine each seat's player id; CGE's standings find their player through the
-- CGE account. A name that is no player's stops the migration.
-- +goose Up
-- A new database's seeded stories name players it does not hold yet; a restore loads the stories with their players.
DELETE FROM tournament_stories WHERE NOT EXISTS (SELECT 1 FROM players);

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY[
        'community_edition_points', 'community_event_points', 'community_rankings', 'player_peak_titles',
        'player_ratings', 'player_seat_stats', 'player_titles', 'player_year_records', 'player_yearly_titles',
        'rating_history', 'tournament_achievements', 'tournament_boards', 'tournament_legends', 'tournament_records',
        'tournament_season_records', 'tournament_standings', 'community_scores', 'community_standings',
        'player_badges', 'player_title_overrides', 'tournament_career_honours', 'tournament_cup_podiums',
        'tournament_group_divisions', 'tournament_ladder_wins', 'tournament_official_placements',
        'tournament_podium_overrides', 'tournament_stories']
    LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN player_id bigint', t);
        EXECUTE format('UPDATE %I x SET player_id = p.player_id FROM players p WHERE p.name = x.player_name', t);
        EXECUTE format('ALTER TABLE %I ALTER COLUMN player_id SET NOT NULL', t);
        EXECUTE format('ALTER TABLE %I DROP COLUMN player_name', t);
    END LOOP;
    FOREACH t IN ARRAY ARRAY['player_a', 'player_b'] LOOP
        EXECUTE format('ALTER TABLE pairwise_matches ADD COLUMN %I bigint', t || '_id');
        EXECUTE format('UPDATE pairwise_matches x SET %I = p.player_id FROM players p WHERE p.name = x.%I', t || '_id', t);
        EXECUTE format('ALTER TABLE pairwise_matches DROP COLUMN %I', t);
        EXECUTE format('ALTER TABLE pairwise_matches RENAME COLUMN %I TO %I', t || '_id', t);
        EXECUTE format('ALTER TABLE pairwise_matches ALTER COLUMN %I SET NOT NULL', t);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE community_edition_points ADD CONSTRAINT community_edition_points_pkey PRIMARY KEY (run_id, edition, player_id, event);
ALTER TABLE community_event_points ADD CONSTRAINT community_event_points_pkey PRIMARY KEY (run_id, tournament, season, player_id);
ALTER TABLE community_rankings ADD CONSTRAINT community_rankings_pkey PRIMARY KEY (run_id, edition, player_id);
ALTER TABLE player_peak_titles ADD CONSTRAINT player_peak_titles_pkey PRIMARY KEY (run_id, player_id);
ALTER TABLE player_ratings ADD CONSTRAINT player_ratings_pkey PRIMARY KEY (run_id, model_type, player_count, player_id);
ALTER TABLE player_seat_stats ADD CONSTRAINT player_seat_stats_pkey PRIMARY KEY (run_id, player_id, player_count, seat);
ALTER TABLE player_titles ADD CONSTRAINT player_titles_pkey PRIMARY KEY (run_id, player_id);
ALTER TABLE player_year_records ADD CONSTRAINT player_year_records_pkey PRIMARY KEY (run_id, player_count, year, player_id);
ALTER TABLE player_yearly_titles ADD CONSTRAINT player_yearly_titles_pkey PRIMARY KEY (run_id, player_id, year);
ALTER TABLE rating_history ADD CONSTRAINT rating_history_pkey PRIMARY KEY (run_id, model_type, player_count, player_id, period_date);
CREATE INDEX rating_history_player ON rating_history (run_id, player_id, player_count, period_date)
    INCLUDE (model_type, rating, rd, c_rating);
ALTER TABLE tournament_achievements ADD CONSTRAINT tournament_achievements_pkey PRIMARY KEY (run_id, player_id);
ALTER TABLE tournament_boards ADD CONSTRAINT tournament_boards_pkey PRIMARY KEY (run_id, series, player_id);
ALTER TABLE tournament_records ADD CONSTRAINT tournament_records_pkey PRIMARY KEY (run_id, player_id, "position");
ALTER TABLE tournament_season_records ADD CONSTRAINT tournament_season_records_pkey PRIMARY KEY (run_id, series, season, player_id);
ALTER TABLE tournament_standings ADD CONSTRAINT tournament_standings_pkey PRIMARY KEY (run_id, series, season, group_label, player_id);
CREATE INDEX pairwise_matches_player_a_games ON pairwise_matches (run_id, player_a)
    INCLUDE (match_id, player_count, date, outcome_a, player_b);
CREATE INDEX pairwise_matches_player_b_games ON pairwise_matches (run_id, player_b)
    INCLUDE (match_id, player_count, date, outcome_a, player_a);

ALTER TABLE community_scores ADD CONSTRAINT community_scores_pkey PRIMARY KEY (edition, player_id, event),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE community_standings ADD CONSTRAINT community_standings_pkey PRIMARY KEY (edition, player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE player_badges ADD CONSTRAINT player_badges_pkey PRIMARY KEY (player_id, badge_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE player_title_overrides ADD CONSTRAINT player_title_overrides_pkey PRIMARY KEY (player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_career_honours ADD CONSTRAINT tournament_career_honours_pkey PRIMARY KEY (series, player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_group_divisions ADD CONSTRAINT tournament_group_divisions_pkey PRIMARY KEY (series, season, player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_ladder_wins ADD CONSTRAINT tournament_ladder_wins_pkey PRIMARY KEY (ladder, player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_official_placements
    ADD CONSTRAINT tournament_official_placements_pkey PRIMARY KEY (series, season, division, player_id),
    ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_cup_podiums ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_podium_overrides ADD FOREIGN KEY (player_id) REFERENCES players (player_id);
ALTER TABLE tournament_stories ADD FOREIGN KEY (player_id) REFERENCES players (player_id);

ALTER TABLE cge_standings DROP COLUMN player_name, ADD FOREIGN KEY (cge_user) REFERENCES player_accounts (cge_user);
CREATE INDEX seats_player_id_games ON seats (player_id) INCLUDE (game_id, starting_position, seat, place, culture);

CREATE OR REPLACE VIEW game_lines AS
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

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_catalogue_spans(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO catalogue_spans (run_id, series_id, tournament_id, stage_id, games, players, first_game, last_game)
    SELECT id, s.series_id, s.tournament_id, st.stage_id,
           count(DISTINCT g.game_id), count(DISTINCT p.player_id), min(g.date), max(g.date)
    FROM games g
    JOIN seats p USING (game_id)
    JOIN groups gr USING (group_id)
    JOIN stages st USING (stage_id)
    JOIN tournaments s USING (tournament_id)
    WHERE g.state = 'played'
    GROUP BY GROUPING SETS ((s.series_id), (s.series_id, s.tournament_id), (s.series_id, s.tournament_id, st.stage_id))
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_player_seat_stats(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO player_seat_stats (run_id, player_id, player_count, seat, games, wins, draws, avg_culture, p1, p2, p3, p4)
    SELECT id, s.player_id, r.player_count, coalesce(s.starting_position + 1, s.seat), count(*),
           count(*) FILTER (WHERE s.place = 1 AND f.firsts = 1), count(*) FILTER (WHERE s.place = 1 AND f.firsts > 1),
           round(avg(s.culture)::numeric, 1),
           count(*) FILTER (WHERE s.place = 1), count(*) FILTER (WHERE s.place = 2),
           count(*) FILTER (WHERE s.place = 3), count(*) FILTER (WHERE s.place = 4)
    FROM (SELECT match_id, player_a AS player_id, player_count FROM pairwise_matches WHERE run_id = id
          UNION
          SELECT match_id, player_b, player_count FROM pairwise_matches WHERE run_id = id) r
    JOIN seats s ON s.game_id = r.match_id AND s.player_id = r.player_id
    JOIN (SELECT game_id, count(*) FILTER (WHERE place = 1) AS firsts FROM seats GROUP BY game_id) f
      ON f.game_id = s.game_id
    GROUP BY s.player_id, r.player_count, coalesce(s.starting_position + 1, s.seat)
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_player_year_records(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO player_year_records (run_id, player_count, year, player_id, opponents, wins, losses, draws, last_played)
    SELECT id, player_count, extract(year FROM date)::integer, player, count(*),
           count(*) FILTER (WHERE outcome = 1), count(*) FILTER (WHERE outcome = 0),
           count(*) FILTER (WHERE outcome = 0.5), max(date)
    FROM (SELECT player_count, player_a AS player, outcome_a AS outcome, date FROM pairwise_matches WHERE run_id = id
          UNION ALL
          SELECT player_count, player_b, 1 - outcome_a, date FROM pairwise_matches WHERE run_id = id) mine
    GROUP BY player_count, extract(year FROM date), player
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_seat_tier_stats(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO seat_tier_stats (run_id, tier, player_count, seat, place, seats, outright_wins, draws, win_shares)
    WITH games_in_run AS (
        SELECT DISTINCT ON (match_id) match_id AS game_id, date, player_count
        FROM pairwise_matches WHERE run_id = id
    ),
    seat_rows AS (
        SELECT g.player_count, coalesce(s.starting_position + 1, s.seat) AS seat, s.place,
            avg(coalesce(rh.rating, 1500.0)) OVER game AS avg_rating,
            count(*) FILTER (WHERE s.place = 1) OVER game AS winners
        FROM games_in_run g
        JOIN seats s ON s.game_id = g.game_id
        LEFT JOIN LATERAL (
            SELECT rating FROM rating_history h
            WHERE h.run_id = id AND h.model_type = 'glicko2_daneo' AND h.player_count = 0
              AND h.player_id = s.player_id AND h.period_date <= g.date
            ORDER BY h.period_date DESC LIMIT 1
        ) rh ON true
        WINDOW game AS (PARTITION BY s.game_id)
    )
    SELECT id,
        CASE
            WHEN avg_rating >= 1750 THEN 'grandmaster'
            WHEN avg_rating >= 1625 THEN 'master'
            WHEN avg_rating >= 1500 THEN 'advanced'
            WHEN avg_rating >= 1375 THEN 'intermediate'
            ELSE 'beginner'
        END,
        player_count, seat, place, count(*),
        count(*) FILTER (WHERE place = 1 AND winners = 1),
        count(*) FILTER (WHERE place = 1 AND winners > 1),
        coalesce(sum(CASE WHEN place = 1 THEN 1.0::float8 / winners ELSE 0 END), 0)::float8
    FROM seat_rows
    GROUP BY 2, 3, 4, 5
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prewarm_player_pages(id bigint) RETURNS bigint
    LANGUAGE plpgsql
    SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    rel regclass;
    blocks bigint := 0;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_prewarm') THEN
        RETURN 0;
    END IF;
    FOR rel IN
        SELECT c.oid::regclass FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        LEFT JOIN pg_index x ON x.indexrelid = c.oid
        JOIN pg_class t ON t.oid = coalesce(x.indrelid, c.oid)
        WHERE i.inhparent = ANY (ARRAY[
                'pairwise_matches_player_a_games', 'pairwise_matches_player_b_games', 'rating_history_player',
                'tournament_records', 'tournament_records_pkey', 'player_ratings', 'player_ratings_pkey',
                'player_seat_stats', 'player_seat_stats_pkey', 'tournament_achievements', 'player_titles',
                'player_titles_pkey', 'player_peak_titles', 'player_peak_titles_pkey', 'player_yearly_titles',
                'player_yearly_titles_pkey']::regclass[])
          AND pg_get_expr(t.relpartbound, t.oid) = format('FOR VALUES IN (%L)', id)
        UNION ALL
        SELECT unnest(ARRAY[
            'rating_runs', 'players', 'players_lower_name', 'players_name_key', 'player_accounts',
            'player_accounts_player', 'members', 'members_player_id_key', 'matches_pkey', 'seats_pkey', 'groups',
            'groups_pkey', 'stages', 'tournament_stages_pkey', 'tournaments', 'tournament_seasons_pkey', 'settings',
            'settings_stage_id_key', 'settings_tournament_id_key', 'settings_series_id_key']::regclass[])
    LOOP
        blocks := blocks + pg_prewarm(rel);
    END LOOP;
    RETURN blocks;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION prewarm_player_pages(id bigint) RETURNS bigint
    LANGUAGE plpgsql
    SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    rel regclass;
    blocks bigint := 0;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_prewarm') THEN
        RETURN 0;
    END IF;
    FOR rel IN
        SELECT c.oid::regclass FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        LEFT JOIN pg_index x ON x.indexrelid = c.oid
        JOIN pg_class t ON t.oid = coalesce(x.indrelid, c.oid)
        WHERE i.inhparent = ANY (ARRAY[
                'pairwise_matches_player_a_games', 'pairwise_matches_player_b_games', 'rating_history_player',
                'tournament_records', 'tournament_records_player_lower', 'player_ratings', 'player_ratings_pkey',
                'player_seat_stats', 'player_seat_stats_pkey', 'tournament_achievements', 'player_titles',
                'player_titles_pkey', 'player_peak_titles', 'player_peak_titles_pkey', 'player_yearly_titles',
                'player_yearly_titles_pkey']::regclass[])
          AND pg_get_expr(t.relpartbound, t.oid) = format('FOR VALUES IN (%L)', id)
        UNION ALL
        SELECT unnest(ARRAY[
            'rating_runs', 'players', 'players_lower_name', 'players_name_key', 'player_accounts',
            'player_accounts_player', 'members', 'members_player_id_key', 'matches_pkey', 'seats_pkey', 'groups',
            'groups_pkey', 'stages', 'tournament_stages_pkey', 'tournaments', 'tournament_seasons_pkey', 'settings',
            'settings_stage_id_key', 'settings_tournament_id_key', 'settings_series_id_key']::regclass[])
    LOOP
        blocks := blocks + pg_prewarm(rel);
    END LOOP;
    RETURN blocks;
END
$$;
-- +goose StatementEnd

DROP VIEW game_lines;
CREATE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores,
       s.game_points, g.finished_at
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
                     END ORDER BY coalesce(starting_position, seat - 1), seat) AS game_points
    FROM (SELECT x.*, count(*) OVER (PARTITION BY x.place) AS ties,
                 coalesce(nullif(CASE count(*) OVER () WHEN 2 THEN ss.points_2p WHEN 3 THEN ss.points_3p WHEN 4 THEN ss.points_4p END, '{}'),
                          (CASE WHEN cardinality(ss.points_2p) > 0 THEN ss.points_2p
                                WHEN cardinality(ss.points_3p) > 0 THEN ss.points_3p
                                WHEN cardinality(ss.points_4p) > 0 THEN ss.points_4p END)[1:count(*) OVER ()]) AS tb
          FROM seats x WHERE x.game_id = g.game_id) seat_rows) s;
GRANT SELECT ON game_lines TO ratings_read, ratings_admin;

DROP INDEX seats_player_id_games;
ALTER TABLE cge_standings DROP CONSTRAINT cge_standings_cge_user_fkey, ADD COLUMN player_name text;
UPDATE cge_standings c SET player_name = a.name FROM player_accounts a WHERE a.cge_user = c.cge_user;
ALTER TABLE cge_standings ALTER COLUMN player_name SET NOT NULL;

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOREACH t IN ARRAY ARRAY['player_a', 'player_b'] LOOP
        EXECUTE format('ALTER TABLE pairwise_matches ADD COLUMN %I text', t || '_name');
        EXECUTE format('UPDATE pairwise_matches x SET %I = p.name FROM players p WHERE p.player_id = x.%I', t || '_name', t);
        EXECUTE format('ALTER TABLE pairwise_matches DROP COLUMN %I', t);
        EXECUTE format('ALTER TABLE pairwise_matches RENAME COLUMN %I TO %I', t || '_name', t);
        EXECUTE format('ALTER TABLE pairwise_matches ALTER COLUMN %I SET NOT NULL', t);
    END LOOP;
    FOREACH t IN ARRAY ARRAY[
        'community_edition_points', 'community_event_points', 'community_rankings', 'player_peak_titles',
        'player_ratings', 'player_seat_stats', 'player_titles', 'player_year_records', 'player_yearly_titles',
        'rating_history', 'tournament_achievements', 'tournament_boards', 'tournament_legends', 'tournament_records',
        'tournament_season_records', 'tournament_standings', 'community_scores', 'community_standings',
        'player_badges', 'player_title_overrides', 'tournament_career_honours', 'tournament_cup_podiums',
        'tournament_group_divisions', 'tournament_ladder_wins', 'tournament_official_placements',
        'tournament_podium_overrides', 'tournament_stories']
    LOOP
        EXECUTE format('ALTER TABLE %I ADD COLUMN player_name text', t);
        EXECUTE format('UPDATE %I x SET player_name = p.name FROM players p WHERE p.player_id = x.player_id', t);
        EXECUTE format('ALTER TABLE %I ALTER COLUMN player_name SET NOT NULL', t);
        EXECUTE format('ALTER TABLE %I DROP COLUMN player_id', t);
    END LOOP;
END $$;
-- +goose StatementEnd

ALTER TABLE tournament_stories ADD CONSTRAINT tournament_stories_player_name_check CHECK (player_name <> '');
CREATE INDEX pairwise_matches_player_a_games ON pairwise_matches (run_id, player_a)
    INCLUDE (match_id, player_count, date, outcome_a, player_b);
CREATE INDEX pairwise_matches_player_b_games ON pairwise_matches (run_id, player_b)
    INCLUDE (match_id, player_count, date, outcome_a, player_a);
ALTER TABLE community_edition_points ADD CONSTRAINT community_edition_points_pkey PRIMARY KEY (run_id, edition, player_name, event);
ALTER TABLE community_event_points ADD CONSTRAINT community_event_points_pkey PRIMARY KEY (run_id, tournament, season, player_name);
ALTER TABLE community_rankings ADD CONSTRAINT community_rankings_pkey PRIMARY KEY (run_id, edition, player_name);
ALTER TABLE player_peak_titles ADD CONSTRAINT player_peak_titles_pkey PRIMARY KEY (run_id, player_name);
ALTER TABLE player_ratings ADD CONSTRAINT player_ratings_pkey PRIMARY KEY (run_id, model_type, player_count, player_name);
ALTER TABLE player_seat_stats ADD CONSTRAINT player_seat_stats_pkey PRIMARY KEY (run_id, player_name, player_count, seat);
ALTER TABLE player_titles ADD CONSTRAINT player_titles_pkey PRIMARY KEY (run_id, player_name);
ALTER TABLE player_year_records ADD CONSTRAINT player_year_records_pkey PRIMARY KEY (run_id, player_count, year, player_name);
ALTER TABLE player_yearly_titles ADD CONSTRAINT player_yearly_titles_pkey PRIMARY KEY (run_id, player_name, year);
ALTER TABLE rating_history ADD CONSTRAINT rating_history_pkey PRIMARY KEY (run_id, model_type, player_count, player_name, period_date);
CREATE INDEX rating_history_player ON rating_history (run_id, player_name, player_count, period_date)
    INCLUDE (model_type, rating, rd, c_rating);
ALTER TABLE tournament_achievements ADD CONSTRAINT tournament_achievements_pkey PRIMARY KEY (run_id, player_name);
ALTER TABLE tournament_boards ADD CONSTRAINT tournament_boards_pkey PRIMARY KEY (run_id, series, player_name);
ALTER TABLE tournament_records ADD CONSTRAINT tournament_records_pkey PRIMARY KEY (run_id, player_name, "position");
CREATE INDEX tournament_records_player_lower ON tournament_records (run_id, lower(player_name));
ALTER TABLE tournament_season_records ADD CONSTRAINT tournament_season_records_pkey PRIMARY KEY (run_id, series, season, player_name);
ALTER TABLE tournament_standings ADD CONSTRAINT tournament_standings_pkey PRIMARY KEY (run_id, series, season, group_label, player_name);
ALTER TABLE community_scores ADD CONSTRAINT community_scores_pkey PRIMARY KEY (edition, player_name, event);
ALTER TABLE community_standings ADD CONSTRAINT community_standings_pkey PRIMARY KEY (edition, player_name);
ALTER TABLE player_badges ADD CONSTRAINT player_badges_pkey PRIMARY KEY (player_name, badge_id);
ALTER TABLE player_title_overrides ADD CONSTRAINT player_title_overrides_pkey PRIMARY KEY (player_name);
ALTER TABLE tournament_career_honours ADD CONSTRAINT tournament_career_honours_pkey PRIMARY KEY (series, player_name);
ALTER TABLE tournament_group_divisions ADD CONSTRAINT tournament_group_divisions_pkey PRIMARY KEY (series, season, player_name);
ALTER TABLE tournament_ladder_wins ADD CONSTRAINT tournament_ladder_wins_pkey PRIMARY KEY (ladder, player_name);
ALTER TABLE tournament_official_placements
    ADD CONSTRAINT tournament_official_placements_pkey PRIMARY KEY (series, season, division, player_name);

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_seat_tier_stats(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO seat_tier_stats (run_id, tier, player_count, seat, place, seats, outright_wins, draws, win_shares)
    WITH games_in_run AS (
        SELECT DISTINCT ON (match_id) match_id AS game_id, date, player_count
        FROM pairwise_matches WHERE run_id = id
    ),
    seat_rows AS (
        SELECT g.player_count, coalesce(s.starting_position + 1, s.seat) AS seat, s.place,
            avg(coalesce(rh.rating, 1500.0)) OVER game AS avg_rating,
            count(*) FILTER (WHERE s.place = 1) OVER game AS winners
        FROM games_in_run g
        JOIN seats s ON s.game_id = g.game_id
        LEFT JOIN LATERAL (
            SELECT rating FROM rating_history h
            WHERE h.run_id = id AND h.model_type = 'glicko2_daneo' AND h.player_count = 0
              AND h.player_name = s.player_name AND h.period_date <= g.date
            ORDER BY h.period_date DESC LIMIT 1
        ) rh ON true
        WINDOW game AS (PARTITION BY s.game_id)
    )
    SELECT id,
        CASE
            WHEN avg_rating >= 1750 THEN 'grandmaster'
            WHEN avg_rating >= 1625 THEN 'master'
            WHEN avg_rating >= 1500 THEN 'advanced'
            WHEN avg_rating >= 1375 THEN 'intermediate'
            ELSE 'beginner'
        END,
        player_count, seat, place, count(*),
        count(*) FILTER (WHERE place = 1 AND winners = 1),
        count(*) FILTER (WHERE place = 1 AND winners > 1),
        coalesce(sum(CASE WHEN place = 1 THEN 1.0::float8 / winners ELSE 0 END), 0)::float8
    FROM seat_rows
    GROUP BY 2, 3, 4, 5
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_player_year_records(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO player_year_records (run_id, player_count, year, player_name, opponents, wins, losses, draws, last_played)
    SELECT id, player_count, extract(year FROM date)::integer, name, count(*),
           count(*) FILTER (WHERE outcome = 1), count(*) FILTER (WHERE outcome = 0),
           count(*) FILTER (WHERE outcome = 0.5), max(date)
    FROM (SELECT player_count, player_a AS name, outcome_a AS outcome, date FROM pairwise_matches WHERE run_id = id
          UNION ALL
          SELECT player_count, player_b, 1 - outcome_a, date FROM pairwise_matches WHERE run_id = id) mine
    GROUP BY player_count, extract(year FROM date), name
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_player_seat_stats(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO player_seat_stats (run_id, player_name, player_count, seat, games, wins, draws, avg_culture, p1, p2, p3, p4)
    SELECT id, s.player_name, r.player_count, coalesce(s.starting_position + 1, s.seat), count(*),
           count(*) FILTER (WHERE s.place = 1 AND f.firsts = 1), count(*) FILTER (WHERE s.place = 1 AND f.firsts > 1),
           round(avg(s.culture)::numeric, 1),
           count(*) FILTER (WHERE s.place = 1), count(*) FILTER (WHERE s.place = 2),
           count(*) FILTER (WHERE s.place = 3), count(*) FILTER (WHERE s.place = 4)
    FROM (SELECT match_id, player_a AS player_name, player_count FROM pairwise_matches WHERE run_id = id
          UNION
          SELECT match_id, player_b, player_count FROM pairwise_matches WHERE run_id = id) r
    JOIN seats s ON s.game_id = r.match_id AND s.player_name = r.player_name
    JOIN (SELECT game_id, count(*) FILTER (WHERE place = 1) AS firsts FROM seats GROUP BY game_id) f
      ON f.game_id = s.game_id
    GROUP BY s.player_name, r.player_count, coalesce(s.starting_position + 1, s.seat)
    ORDER BY s.player_name
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION derive_catalogue_spans(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO catalogue_spans (run_id, series_id, tournament_id, stage_id, games, players, first_game, last_game)
    SELECT id, s.series_id, s.tournament_id, st.stage_id,
           count(DISTINCT g.game_id), count(DISTINCT p.player_name), min(g.date), max(g.date)
    FROM games g
    JOIN seats p USING (game_id)
    JOIN groups gr USING (group_id)
    JOIN stages st USING (stage_id)
    JOIN tournaments s USING (tournament_id)
    WHERE g.state = 'played'
    GROUP BY GROUPING SETS ((s.series_id), (s.series_id, s.tournament_id), (s.series_id, s.tournament_id, st.stage_id))
$$;
-- +goose StatementEnd
