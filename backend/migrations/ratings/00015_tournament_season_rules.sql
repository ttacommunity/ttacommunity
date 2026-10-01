-- Each season's rules as its tournament page states them: the points a
-- timed-out game costs, the scoring and the tiebreakers. A recorded fact the
-- load job writes; the engine applies the time-out penalty to standings.

-- +goose Up
CREATE TABLE tournament_season_rules (
    series text NOT NULL,
    season integer NOT NULL CHECK (season > 0),
    timeout_penalty double precision NOT NULL CHECK (timeout_penalty <= 0),
    scoring_3p text NOT NULL DEFAULT '',
    scoring_4p text NOT NULL DEFAULT '',
    tiebreaks text NOT NULL DEFAULT '',
    source text NOT NULL,
    PRIMARY KEY (series, season)
);

-- +goose Down
DROP TABLE tournament_season_rules;
