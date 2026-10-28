-- A league's rules as settings: its divisions in order with each one's title tier (1 the highest, 7 the lowest),
-- whether a top-division win makes a Super GM, and the fewest games for a season record. Each season record says
-- which series and division its games were in, so titles read no names.
-- +goose Up
ALTER TABLE series
    ADD COLUMN divisions text[] NOT NULL DEFAULT '{}',
    ADD COLUMN division_tiers smallint[] NOT NULL DEFAULT '{}',
    ADD COLUMN super_gm boolean NOT NULL DEFAULT false,
    ADD COLUMN record_min_games smallint NOT NULL DEFAULT 0 CHECK (record_min_games >= 0),
    ADD CONSTRAINT series_division_tiers_check CHECK (
        cardinality(division_tiers) = cardinality(divisions) AND 1 <= ALL (division_tiers) AND 7 >= ALL (division_tiers));

UPDATE series SET divisions = '{Grandmaster,Diamond,Master,Platinum,Gold,Silver,Bronze,Wood}',
                  division_tiers = '{1,1,2,3,4,5,6,7}', super_gm = true, record_min_games = 6
    WHERE results_rules IN ('international', 'intermezzo');
UPDATE series SET divisions = '{Emperor,King,Prince,Duke,Marquess,Count,Viscount,Baron,Knight}',
                  division_tiers = '{1,2,3,4,5,6,7,7,7}', super_gm = true, record_min_games = 5
    WHERE results_rules = 'royal_league';

GRANT UPDATE (divisions, division_tiers, super_gm, record_min_games) ON series TO ratings_admin;

ALTER TABLE tournament_records ADD COLUMN series text, ADD COLUMN series_division text;

-- +goose Down
ALTER TABLE tournament_records DROP COLUMN series_division, DROP COLUMN series;
REVOKE UPDATE (divisions, division_tiers, super_gm, record_min_games) ON series FROM ratings_admin;
ALTER TABLE series DROP CONSTRAINT series_division_tiers_check, DROP COLUMN record_min_games, DROP COLUMN super_gm,
    DROP COLUMN division_tiers, DROP COLUMN divisions;
