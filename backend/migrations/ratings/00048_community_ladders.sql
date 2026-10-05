-- How the community leaderboard scores a ladder's rounds, from an edition on:
-- a rung's winner earns 1000 × ratio^steps (a step a rung, or half a step a
-- rung after half_steps_from), each place its share; rolling editions sum the
-- latest `rounds`. A recorded fact the load job writes.

-- +goose Up
CREATE TABLE community_ladders (
    tournament text NOT NULL,
    from_edition integer NOT NULL CHECK (from_edition BETWEEN 2000 AND 2999),
    ratio double precision NOT NULL CHECK (ratio > 0 AND ratio < 1),
    half_steps_from integer CHECK (half_steps_from > 1),
    shares double precision[] NOT NULL CHECK (cardinality(shares) > 0),
    rounds integer NOT NULL CHECK (rounds > 0),
    source text NOT NULL,
    PRIMARY KEY (tournament, from_edition)
);

GRANT SELECT ON community_ladders TO ratings_admin;

-- +goose Down
DROP TABLE community_ladders;
