-- The community leaderboard points each player earned in each event, as a
-- run derives them from the places and the points tables.

-- +goose Up
CREATE TABLE community_event_points (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    tournament text NOT NULL,
    season text NOT NULL,
    player_name text NOT NULL,
    points integer NOT NULL CHECK (points >= 0),
    PRIMARY KEY (run_id, tournament, season, player_name)
);

GRANT SELECT ON community_event_points TO ratings_admin;

-- +goose Down
DROP TABLE community_event_points;
