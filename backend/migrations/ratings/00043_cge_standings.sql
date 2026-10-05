-- CGE's own standings table for each group of each stage, as its pages show
-- it: a recorded fact the load job writes, for checking our games against.
-- Players are CGE users, whether or not they are players here.

-- +goose Up
CREATE TABLE cge_standings (
    cge_tournament integer NOT NULL CHECK (cge_tournament > 0),
    cge_season integer NOT NULL CHECK (cge_season > 0),
    group_name text NOT NULL,
    row_number integer NOT NULL CHECK (row_number > 0),
    place integer CHECK (place > 0),
    player_name text NOT NULL,
    cge_user integer NOT NULL CHECK (cge_user > 0),
    banned boolean NOT NULL,
    games integer NOT NULL CHECK (games >= 0),
    points double precision NOT NULL,
    places_2p integer[] CHECK (cardinality(places_2p) = 2),
    places_3p integer[] CHECK (cardinality(places_3p) = 3),
    places_4p integer[] CHECK (cardinality(places_4p) = 4),
    source text NOT NULL,
    PRIMARY KEY (cge_tournament, cge_season, group_name, row_number)
);

CREATE INDEX cge_standings_player ON cge_standings (cge_tournament, cge_season, cge_user);

GRANT SELECT ON cge_standings TO ratings_admin;

-- +goose Down
DROP TABLE cge_standings;
