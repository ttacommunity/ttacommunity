-- A note shown beside a season, such as what a special season was: a
-- recorded fact the load job writes (later, admin edits it).

-- +goose Up
CREATE TABLE tournament_season_notes (
    tournament text NOT NULL,
    season text NOT NULL CHECK (season <> ''),
    note text NOT NULL CHECK (note <> ''),
    source text NOT NULL,
    PRIMARY KEY (tournament, season)
);

-- +goose Down
DROP TABLE tournament_season_notes;
