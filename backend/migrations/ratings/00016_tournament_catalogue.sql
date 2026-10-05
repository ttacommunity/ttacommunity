-- The tournament catalogue: each tournament as the site names it, its
-- seasons, and each season's stages, which carry CGE's ids. Every match
-- points at its stage, with its group and game number.

-- +goose Up
CREATE TABLE tournaments (
    tournament_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9_]*$'),
    name text NOT NULL CHECK (name <> '')
);

CREATE TABLE tournament_seasons (
    season_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    tournament_id bigint NOT NULL REFERENCES tournaments,
    label text NOT NULL CHECK (label <> ''),
    UNIQUE (tournament_id, label)
);

-- A stage labelled '' is its season's only stage, or its main one.
CREATE TABLE tournament_stages (
    stage_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    season_id bigint NOT NULL REFERENCES tournament_seasons,
    label text NOT NULL,
    cge_tournament integer CHECK (cge_tournament > 0),
    cge_season integer CHECK (cge_season > 0),
    CHECK ((cge_tournament IS NULL) = (cge_season IS NULL)),
    UNIQUE (season_id, label),
    UNIQUE (cge_tournament, cge_season)
);

ALTER TABLE matches
    ADD COLUMN stage_id bigint REFERENCES tournament_stages,
    ADD COLUMN group_label text NOT NULL DEFAULT '',
    ADD COLUMN game_number integer CHECK (game_number > 0);

CREATE INDEX matches_stage ON matches (stage_id);

-- +goose Down
DROP INDEX matches_stage;
ALTER TABLE matches
    DROP COLUMN game_number,
    DROP COLUMN group_label,
    DROP COLUMN stage_id;
DROP TABLE tournament_stages;
DROP TABLE tournament_seasons;
DROP TABLE tournaments;
