-- Tournament results. The first three tables are recorded facts the load job
-- writes; the others are what the engine derives for each run from the
-- matches and those facts.

-- +goose Up
-- The division a player played in one season, where match names do not say.
CREATE TABLE tournament_group_divisions (
    series text NOT NULL,
    season integer NOT NULL CHECK (season > 0),
    division text NOT NULL,
    player_name text NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (series, season, player_name)
);

-- A place the organisers recorded in a division's final standings, applied
-- over the derived one: a season with no games, or a ruling.
CREATE TABLE tournament_official_placements (
    series text NOT NULL,
    season integer NOT NULL CHECK (season > 0),
    division text NOT NULL,
    player_name text NOT NULL,
    place integer NOT NULL CHECK (place > 0),
    points double precision,
    reason text NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (series, season, division, player_name)
);

-- The real season of games whose name gives the wrong one. A null season
-- leaves those games out of tournament results (the ratings still count them).
CREATE TABLE tournament_season_corrections (
    series text NOT NULL,
    name_season integer NOT NULL CHECK (name_season > 0),
    season integer CHECK (season > 0),
    reason text NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (series, name_season)
);

-- Each tournament game's series, season (corrected), the season its name
-- gives, division and group. The division is null where neither the name nor
-- a recorded fact says; division_source says which gave it. A tiebreak game
-- decides a tie and is not part of its group's standings.
CREATE TABLE tournament_games (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    match_id bigint NOT NULL REFERENCES matches,
    series text NOT NULL,
    season integer,
    name_season integer NOT NULL,
    division text,
    group_label text,
    division_source text CHECK (division_source IN ('name', 'recorded')),
    tiebreak boolean NOT NULL DEFAULT false,
    PRIMARY KEY (run_id, match_id),
    CHECK ((division IS NULL) = (division_source IS NULL))
);

CREATE INDEX tournament_games_series ON tournament_games (run_id, series, season, division);

-- A group's final standings, by the series' points and tiebreak rules.
-- placings counts the player's 1st, 2nd, 3rd and 4th places in its games.
CREATE TABLE tournament_standings (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    series text NOT NULL,
    season integer NOT NULL,
    division text NOT NULL,
    group_label text NOT NULL,
    player_name text NOT NULL,
    place integer NOT NULL CHECK (place > 0),
    points double precision NOT NULL,
    games integer NOT NULL,
    placings integer[] NOT NULL,
    PRIMARY KEY (run_id, series, season, group_label, player_name)
);

-- Each medallist's podiums in a series' top division, and their career
-- points there, which order players with equal medals; then the season of
-- their first medal.
CREATE TABLE tournament_boards (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    series text NOT NULL,
    player_name text NOT NULL,
    gold integer NOT NULL,
    silver integer NOT NULL,
    bronze integer NOT NULL,
    points double precision NOT NULL,
    title text NOT NULL,
    first_season integer NOT NULL,
    PRIMARY KEY (run_id, series, player_name)
);

-- Top-division seasons scored as upstream's seasonal records page scores
-- them. first_match_id orders seasons with equal points and wins.
CREATE TABLE tournament_season_records (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    series text NOT NULL,
    season integer NOT NULL,
    division text NOT NULL,
    player_name text NOT NULL,
    points double precision NOT NULL,
    games integer NOT NULL,
    placings integer[] NOT NULL,
    last_date date NOT NULL,
    first_match_id bigint NOT NULL,
    PRIMARY KEY (run_id, series, season, player_name)
);

-- +goose Down
DROP TABLE tournament_season_records;
DROP TABLE tournament_boards;
DROP TABLE tournament_standings;
DROP TABLE tournament_games;
DROP TABLE tournament_season_corrections;
DROP TABLE tournament_official_placements;
DROP TABLE tournament_group_divisions;
