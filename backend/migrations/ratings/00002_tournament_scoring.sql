-- Each tournament's points per place, by player count, and each season's time-out penalty, set in admin;
-- the engine scores standings with them.
-- +goose Up
ALTER TABLE tournaments
    ADD COLUMN points_2p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_2p) IN (0, 2) AND 0 <= ALL (points_2p)),
    ADD COLUMN points_3p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_3p) IN (0, 3) AND 0 <= ALL (points_3p)),
    ADD COLUMN points_4p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_4p) IN (0, 4) AND 0 <= ALL (points_4p));

GRANT UPDATE (points_2p, points_3p, points_4p) ON tournaments TO ratings_admin;
GRANT INSERT, UPDATE (timeout_penalty, source) ON tournament_season_rules TO ratings_admin;

UPDATE tournaments SET points_3p = '{5,2,0}', points_4p = '{6,3,1,0}' WHERE slug = 'international';
UPDATE tournaments SET points_3p = '{5,2,0}' WHERE slug = 'intermezzo';
UPDATE tournaments SET points_2p = '{2,0}' WHERE slug = 'royal_league';
UPDATE tournaments SET points_2p = '{4,0}', points_3p = '{5,2,0}', points_4p = '{6,3,1,0}' WHERE slug = 'worlds';

-- +goose Down
REVOKE INSERT, UPDATE (timeout_penalty, source) ON tournament_season_rules FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN points_2p, DROP COLUMN points_3p, DROP COLUMN points_4p;
