-- The community leaderboard's points tables: what each place in a group (a
-- division, stage, round or rung) of a tournament earns, from an edition on.
-- A recorded fact the load job writes, from the organisers' sheet.

-- +goose Up
CREATE TABLE community_points (
    tournament text NOT NULL,
    from_edition integer NOT NULL CHECK (from_edition BETWEEN 2000 AND 2999),
    group_name text NOT NULL,
    place integer NOT NULL CHECK (place >= 0),
    points integer NOT NULL CHECK (points >= 0),
    source text NOT NULL,
    PRIMARY KEY (tournament, from_edition, group_name, place)
);

GRANT SELECT ON community_points TO ratings_admin;

-- +goose Down
DROP TABLE community_points;
