-- The quarter the community leaderboard's organisers count each tournament
-- season in: a recorded fact the load job writes. A season counts in every
-- edition whose quarters hold its quarter.

-- +goose Up
CREATE TABLE community_quarters (
    tournament text NOT NULL,
    season text NOT NULL,
    year integer NOT NULL CHECK (year BETWEEN 2000 AND 2999),
    quarter integer NOT NULL CHECK (quarter BETWEEN 1 AND 4),
    source text NOT NULL,
    PRIMARY KEY (tournament, season)
);

GRANT SELECT ON community_quarters TO ratings_admin;

-- +goose Down
DROP TABLE community_quarters;
