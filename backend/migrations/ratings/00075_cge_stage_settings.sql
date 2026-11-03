-- CGE's "Each player plays: Nx" is a count per player count: each_plays becomes each_plays_2p, _3p and _4p (0 =
-- none), and a stage's least winner score (in %) for the winnerScore tiebreakers is kept. Both start empty, for
-- CGE's extract to give on its next load; game_lines is unchanged.
-- +goose Up
ALTER TABLE settings ADD COLUMN each_plays_2p integer CHECK (each_plays_2p >= 0),
    ADD COLUMN each_plays_3p integer CHECK (each_plays_3p >= 0),
    ADD COLUMN each_plays_4p integer CHECK (each_plays_4p >= 0),
    ADD COLUMN winner_score_minimum double precision CHECK (winner_score_minimum BETWEEN 0 AND 100);
DROP VIEW game_lines;
DROP VIEW stage_settings;
ALTER TABLE settings DROP COLUMN each_plays;
CREATE VIEW stage_settings AS
SELECT st.stage_id,
       coalesce(ss.timeout_penalty, ts.timeout_penalty, se.timeout_penalty) AS timeout_penalty,
       coalesce(ss.points_2p, ts.points_2p, se.points_2p) AS points_2p,
       coalesce(ss.points_3p, ts.points_3p, se.points_3p) AS points_3p,
       coalesce(ss.points_4p, ts.points_4p, se.points_4p) AS points_4p,
       coalesce(ss.tiebreak_2p, ts.tiebreak_2p, se.tiebreak_2p) AS tiebreak_2p,
       coalesce(ss.tiebreak_3p_plus, ts.tiebreak_3p_plus, se.tiebreak_3p_plus) AS tiebreak_3p_plus,
       coalesce(ss.ignore_ai_players, ts.ignore_ai_players, se.ignore_ai_players) AS ignore_ai_players,
       coalesce(ss.each_plays_2p, ts.each_plays_2p, se.each_plays_2p) AS each_plays_2p,
       coalesce(ss.each_plays_3p, ts.each_plays_3p, se.each_plays_3p) AS each_plays_3p,
       coalesce(ss.each_plays_4p, ts.each_plays_4p, se.each_plays_4p) AS each_plays_4p,
       coalesce(ss.winner_score_minimum, ts.winner_score_minimum, se.winner_score_minimum) AS winner_score_minimum
FROM stages st
JOIN tournaments t USING (tournament_id)
LEFT JOIN settings ss ON ss.stage_id = st.stage_id
LEFT JOIN settings ts ON ts.tournament_id = t.tournament_id
LEFT JOIN settings se ON se.series_id = t.series_id;
GRANT SELECT ON stage_settings TO ratings_read, ratings_admin;
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

-- +goose Down
ALTER TABLE settings ADD COLUMN each_plays integer CHECK (each_plays > 0);
-- The one count every player count with games shares, else none.
UPDATE settings SET each_plays = (SELECT CASE WHEN count(DISTINCT n) = 1 THEN min(n) END
                                  FROM unnest(ARRAY[each_plays_2p, each_plays_3p, each_plays_4p]) AS n WHERE n > 0);
DROP VIEW game_lines;
DROP VIEW stage_settings;
ALTER TABLE settings DROP COLUMN each_plays_2p, DROP COLUMN each_plays_3p, DROP COLUMN each_plays_4p,
    DROP COLUMN winner_score_minimum;
CREATE VIEW stage_settings AS
SELECT st.stage_id,
       coalesce(ss.timeout_penalty, ts.timeout_penalty, se.timeout_penalty) AS timeout_penalty,
       coalesce(ss.points_2p, ts.points_2p, se.points_2p) AS points_2p,
       coalesce(ss.points_3p, ts.points_3p, se.points_3p) AS points_3p,
       coalesce(ss.points_4p, ts.points_4p, se.points_4p) AS points_4p,
       coalesce(ss.tiebreak_2p, ts.tiebreak_2p, se.tiebreak_2p) AS tiebreak_2p,
       coalesce(ss.tiebreak_3p_plus, ts.tiebreak_3p_plus, se.tiebreak_3p_plus) AS tiebreak_3p_plus,
       coalesce(ss.ignore_ai_players, ts.ignore_ai_players, se.ignore_ai_players) AS ignore_ai_players,
       coalesce(ss.each_plays, ts.each_plays, se.each_plays) AS each_plays
FROM stages st
JOIN tournaments t USING (tournament_id)
LEFT JOIN settings ss ON ss.stage_id = st.stage_id
LEFT JOIN settings ts ON ts.tournament_id = t.tournament_id
LEFT JOIN settings se ON se.series_id = t.series_id;
GRANT SELECT ON stage_settings TO ratings_read, ratings_admin;
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
