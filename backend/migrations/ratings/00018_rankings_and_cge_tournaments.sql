-- Rankings: a set of tournaments rated with the same models. The community
-- ranking takes every tournament with games, as the original site did.
-- cge_tournaments is CGE's catalogue as its pages show it, each row linked to
-- one of our tournaments (and a season) when it is one of ours.

-- +goose Up
CREATE TABLE rankings (
    ranking_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    slug text NOT NULL UNIQUE CHECK (slug ~ '^[a-z][a-z0-9_]*$'),
    name text NOT NULL CHECK (name <> ''),
    description text NOT NULL DEFAULT ''
);

CREATE TABLE ranking_tournaments (
    ranking_id bigint NOT NULL REFERENCES rankings ON DELETE CASCADE,
    tournament_id bigint NOT NULL REFERENCES tournaments ON DELETE CASCADE,
    PRIMARY KEY (ranking_id, tournament_id)
);

INSERT INTO rankings (slug, name, description)
VALUES ('community', 'Community ratings', 'Every tournament the original community ratings site rated');
INSERT INTO ranking_tournaments (ranking_id, tournament_id)
SELECT r.ranking_id, t.tournament_id FROM rankings r CROSS JOIN tournaments t WHERE r.slug = 'community';

CREATE TABLE cge_tournaments (
    cge_tournament integer PRIMARY KEY CHECK (cge_tournament > 0),
    name text NOT NULL CHECK (name <> ''),
    status text NOT NULL,
    organiser text NOT NULL DEFAULT '',
    created date,
    started date,
    finished date,
    description text NOT NULL DEFAULT '',
    mentions_glicko text NOT NULL DEFAULT '',
    links text NOT NULL DEFAULT '',
    tournament_id bigint REFERENCES tournaments,
    season text NOT NULL DEFAULT '',
    CHECK (tournament_id IS NOT NULL OR season = '')
);

-- +goose Down
DROP TABLE cge_tournaments;
DROP TABLE ranking_tournaments;
DROP TABLE rankings;
