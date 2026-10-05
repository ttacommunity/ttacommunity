-- Each finished cup year's podium, as the cup's final shows it: a recorded
-- fact the load job writes, which the Grand Slams board is built from.

-- +goose Up
CREATE TABLE tournament_cup_podiums (
    cup text NOT NULL,
    season text NOT NULL CHECK (season <> ''),
    final text NOT NULL DEFAULT '',
    place integer NOT NULL CHECK (place BETWEEN 1 AND 3),
    player_name text NOT NULL,
    source text NOT NULL,
    finished date NOT NULL,
    PRIMARY KEY (cup, season, final, place)
);

-- +goose Down
DROP TABLE tournament_cup_podiums;
