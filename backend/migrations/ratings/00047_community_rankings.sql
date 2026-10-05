-- Each community leaderboard edition a run ranks: the events each player
-- scored in it (computed, or the organisers' sheet for an event the engine
-- does not compute yet), whether each is among their best eight, and the
-- ranking. An edition is a calendar year ("2025") or the four quarters to one
-- ("rolling 2026 Q2").

-- +goose Up
CREATE TABLE community_edition_points (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    edition text NOT NULL,
    player_name text NOT NULL,
    event text NOT NULL,
    points double precision NOT NULL,
    source text NOT NULL CHECK (source IN ('computed', 'sheet')),
    counted boolean NOT NULL,
    PRIMARY KEY (run_id, edition, player_name, event)
);

CREATE TABLE community_rankings (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    edition text NOT NULL,
    rank integer NOT NULL CHECK (rank > 0),
    player_name text NOT NULL,
    points double precision NOT NULL,
    events integer NOT NULL,
    PRIMARY KEY (run_id, edition, player_name)
);

GRANT SELECT ON community_edition_points, community_rankings TO ratings_admin;

-- +goose Down
DROP TABLE community_rankings;
DROP TABLE community_edition_points;
