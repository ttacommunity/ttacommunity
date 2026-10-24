-- Each run's seat statistics for every player, by format and seat, over the games the run rated: counted once as
-- the run is made instead of on every player page, which read each game's seats to tell a win from a shared first
-- place. A player's rows are written together. Runs already held are counted here.
-- +goose Up
CREATE TABLE player_seat_stats (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    player_count smallint NOT NULL,
    seat smallint NOT NULL,
    games integer NOT NULL,
    wins integer NOT NULL,
    draws integer NOT NULL,
    avg_culture numeric NOT NULL,
    p1 integer NOT NULL,
    p2 integer NOT NULL,
    p3 integer NOT NULL,
    p4 integer NOT NULL,
    PRIMARY KEY (run_id, player_name, player_count, seat)
) PARTITION BY LIST (run_id);
GRANT SELECT ON player_seat_stats TO ratings_read, ratings_admin;

-- +goose StatementBegin
DO $$
DECLARE
    r bigint;
BEGIN
    FOR r IN SELECT run_id FROM rating_runs LOOP
        EXECUTE format('CREATE TABLE %I PARTITION OF player_seat_stats FOR VALUES IN (%s)', 'player_seat_stats_run' || r, r);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- A seat is its starting position where the game has one; a first place is a win alone, a draw when shared.
-- +goose StatementBegin
CREATE FUNCTION derive_player_seat_stats(id bigint) RETURNS void LANGUAGE sql AS $$
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
REVOKE ALL ON FUNCTION derive_player_seat_stats(bigint) FROM PUBLIC;

-- +goose StatementBegin
DO $$ BEGIN PERFORM derive_player_seat_stats(run_id) FROM rating_runs; END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION derive_player_seat_stats(bigint);
DROP TABLE player_seat_stats;
