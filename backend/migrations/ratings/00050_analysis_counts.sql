-- Each run's counts for the analysis pages, made once as the run is made instead of on a pod's first request: seat
-- results by skill tier (a game's tier is its players' mean Glicko-D rating when it was played) and games and pairs
-- per format. Runs already held are counted here.
-- +goose Up
CREATE TABLE seat_tier_stats (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    tier text NOT NULL CHECK (tier IN ('grandmaster', 'master', 'advanced', 'intermediate', 'beginner')),
    player_count smallint NOT NULL,
    seat smallint NOT NULL,
    place smallint NOT NULL,
    seats integer NOT NULL,
    outright_wins integer NOT NULL,
    draws integer NOT NULL,
    win_shares double precision NOT NULL,
    PRIMARY KEY (run_id, tier, player_count, seat, place)
) PARTITION BY LIST (run_id);
GRANT SELECT ON seat_tier_stats TO ratings_read, ratings_admin;

CREATE TABLE run_format_counts (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_count smallint NOT NULL,
    games integer NOT NULL,
    pairs integer NOT NULL,
    PRIMARY KEY (run_id, player_count)
) PARTITION BY LIST (run_id);
GRANT SELECT ON run_format_counts TO ratings_read, ratings_admin;

-- +goose StatementBegin
DO $$
DECLARE
    r bigint;
    t text;
BEGIN
    FOR r IN SELECT run_id FROM rating_runs LOOP
        FOREACH t IN ARRAY ARRAY['seat_tier_stats', 'run_format_counts'] LOOP
            EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES IN (%s)', t || '_run' || r, t, r);
        END LOOP;
    END LOOP;
END
$$;
-- +goose StatementEnd

-- A seat is its starting position where the game has one; a first place is a win alone, a draw when shared, and
-- each first place holds an equal share of the win. A player with no rating yet counts as 1500.
-- +goose StatementBegin
CREATE FUNCTION derive_seat_tier_stats(id bigint) RETURNS void LANGUAGE sql AS $$
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
REVOKE ALL ON FUNCTION derive_seat_tier_stats(bigint) FROM PUBLIC;

-- +goose StatementBegin
CREATE FUNCTION derive_run_format_counts(id bigint) RETURNS void LANGUAGE sql AS $$
    INSERT INTO run_format_counts (run_id, player_count, games, pairs)
    SELECT id, player_count, count(DISTINCT match_id), count(*) FROM pairwise_matches WHERE run_id = id
    GROUP BY player_count
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION derive_run_format_counts(bigint) FROM PUBLIC;

-- +goose StatementBegin
DO $$ BEGIN PERFORM derive_seat_tier_stats(run_id), derive_run_format_counts(run_id) FROM rating_runs; END $$;
-- +goose StatementEnd

-- +goose Down
DROP FUNCTION derive_run_format_counts(bigint);
DROP FUNCTION derive_seat_tier_stats(bigint);
DROP TABLE run_format_counts;
DROP TABLE seat_tier_stats;
