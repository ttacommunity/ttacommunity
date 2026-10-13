-- Redefine game_lines to aggregate seats in turn order: by starting_position when known, else seat.
-- +goose Up
CREATE OR REPLACE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores,
       s.game_points
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

-- +goose Down
CREATE OR REPLACE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores,
       s.game_points
FROM games g
LEFT JOIN groups gr USING (group_id)
LEFT JOIN stages st USING (stage_id)
LEFT JOIN tournaments t USING (tournament_id)
LEFT JOIN stage_settings ss ON ss.stage_id = gr.stage_id
CROSS JOIN LATERAL (
    SELECT count(*)::smallint AS player_count,
           array_agg(player_name ORDER BY seat) AS players, array_agg(culture ORDER BY seat) AS cultures,
           array_agg(place ORDER BY seat) AS places, array_agg(outcome ORDER BY seat) AS outcomes,
           array_agg(coalesce(recorded_score, CASE WHEN outcome = 'finished' THEN culture ELSE -place END) ORDER BY seat) AS scores,
           array_agg(CASE WHEN tb IS NULL THEN NULL
                          WHEN outcome = 'timed_out' THEN coalesce(ss.timeout_penalty, 0)
                          ELSE (SELECT avg(coalesce(tb[k], 0)) FROM generate_series(place, place + ties - 1) AS k)
                     END ORDER BY seat) AS game_points
    FROM (SELECT x.*, count(*) OVER (PARTITION BY x.place) AS ties,
                 coalesce(nullif(CASE count(*) OVER () WHEN 2 THEN ss.points_2p WHEN 3 THEN ss.points_3p WHEN 4 THEN ss.points_4p END, '{}'),
                          (CASE WHEN cardinality(ss.points_2p) > 0 THEN ss.points_2p
                                WHEN cardinality(ss.points_3p) > 0 THEN ss.points_3p
                                WHEN cardinality(ss.points_4p) > 0 THEN ss.points_4p END)[1:count(*) OVER ()]) AS tb
          FROM seats x WHERE x.game_id = g.game_id) seat_rows) s;

GRANT SELECT ON game_lines TO ratings_read, ratings_admin;
