-- What a run rated: its games, players and one-to-one pairs, and the dates of
-- its first and last games. The engine writes them as a run completes; this
-- fills them for the runs completed before.

-- +goose Up
ALTER TABLE rating_runs
    ADD COLUMN games integer,
    ADD COLUMN players integer,
    ADD COLUMN pairs integer,
    ADD COLUMN first_game date,
    ADD COLUMN last_game date;

UPDATE rating_runs r SET
    games = s.games, players = s.players, pairs = s.pairs, first_game = s.first_game, last_game = s.last_game
FROM (
    SELECT p.run_id, count(DISTINCT p.match_id) AS games, count(*) AS pairs, min(p.date) AS first_game,
        max(p.date) AS last_game,
        (SELECT count(*) FROM (
            SELECT player_a FROM pairwise_matches WHERE run_id = p.run_id
            UNION SELECT player_b FROM pairwise_matches WHERE run_id = p.run_id) n) AS players
    FROM pairwise_matches p GROUP BY p.run_id
) s
WHERE s.run_id = r.run_id AND r.status = 'complete';

-- +goose Down
ALTER TABLE rating_runs
    DROP COLUMN games,
    DROP COLUMN players,
    DROP COLUMN pairs,
    DROP COLUMN first_game,
    DROP COLUMN last_game;
