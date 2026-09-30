-- The community leaderboard, recorded as its maintainer publishes it: each
-- edition's standings, and each player's points in each event of it. Names
-- are kept as written, whether or not they are players here. sheet_row is
-- the row's place in the maintainer's sheet, which orders equal ranks.

-- +goose Up
CREATE TABLE community_standings (
    edition text NOT NULL,
    sheet_row integer NOT NULL CHECK (sheet_row > 0),
    rank integer NOT NULL CHECK (rank > 0),
    player_name text NOT NULL,
    points double precision NOT NULL,
    events integer NOT NULL CHECK (events >= 0),
    source text NOT NULL,
    PRIMARY KEY (edition, player_name)
);

CREATE TABLE community_scores (
    edition text NOT NULL,
    player_name text NOT NULL,
    event text NOT NULL,
    score double precision NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (edition, player_name, event)
);

-- +goose Down
DROP TABLE community_scores;
DROP TABLE community_standings;
