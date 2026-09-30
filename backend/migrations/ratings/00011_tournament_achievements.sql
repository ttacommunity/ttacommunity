-- A player's tournament achievements. Two recorded facts the load job writes
-- (seasons' finish dates, and career honours from series the matches cannot
-- rank), and what the engine derives for each run: each player's medal
-- summary and their records, as the achievements page lists them.

-- +goose Up
-- The day a season of a series officially finished.
CREATE TABLE tournament_finish_dates (
    series text NOT NULL,
    season integer NOT NULL CHECK (season > 0),
    finish_date date NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (series, season)
);

-- A player's career medals in a series from its organisers' Hall of Fame,
-- and its points as the organisers wrote them.
CREATE TABLE tournament_career_honours (
    series text NOT NULL,
    player_name text NOT NULL,
    gold integer NOT NULL CHECK (gold >= 0),
    silver integer NOT NULL CHECK (silver >= 0),
    bronze integer NOT NULL CHECK (bronze >= 0),
    points text,
    source text NOT NULL,
    PRIMARY KEY (series, player_name)
);

-- Each player's titles and medals across the four major circuits, and the
-- accolades their page features, in order.
CREATE TABLE tournament_achievements (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    world_titles integer NOT NULL,
    international_titles integer NOT NULL,
    intermezzo_titles integer NOT NULL,
    royal_league_titles integer NOT NULL,
    gold integer NOT NULL,
    silver integer NOT NULL,
    bronze integer NOT NULL,
    accolades text[] NOT NULL,
    PRIMARY KEY (run_id, player_name)
);

-- Each player's tournament records as the page shows them: one row per
-- season, campaign or career total, numbered in the order they were made.
-- finish_date is text: a career total's is a label, not a date.
CREATE TABLE tournament_records (
    run_id bigint NOT NULL REFERENCES rating_runs ON DELETE CASCADE,
    player_name text NOT NULL,
    position integer NOT NULL CHECK (position > 0),
    tournament text NOT NULL,
    season text NOT NULL,
    division text NOT NULL,
    placement text NOT NULL,
    points text NOT NULL,
    medal text CHECK (medal IN ('gold', 'silver', 'bronze')),
    details text NOT NULL,
    finish_date text NOT NULL,
    career boolean NOT NULL,
    PRIMARY KEY (run_id, player_name, position)
);

-- +goose Down
DROP TABLE tournament_records;
DROP TABLE tournament_achievements;
DROP TABLE tournament_career_honours;
DROP TABLE tournament_finish_dates;
