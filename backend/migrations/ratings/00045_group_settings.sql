-- Settle group and tournament settings: podium source, results rules flexibility, and admin permissions to manage them and move tournaments.
-- +goose Up
ALTER TABLE series
    ADD COLUMN podium_source text CHECK (podium_source IS NULL OR podium_source IN ('top_divisions', 'final_stage', 'recorded', 'none'));

UPDATE series SET podium_source = 'top_divisions' WHERE kind = 'league' OR results_rules IN ('international', 'intermezzo', 'royal_league');
UPDATE series SET podium_source = 'recorded' WHERE results_rules = 'worlds';
UPDATE series SET podium_source = 'final_stage' WHERE kind = 'cup' AND podium_source IS NULL;
UPDATE series SET podium_source = 'none' WHERE kind = 'ladder' AND podium_source IS NULL;

ALTER TABLE series DROP CONSTRAINT series_results_rules_key;

GRANT UPDATE (community_scoring, community_code, results_rules, podium_source) ON series TO ratings_admin;
GRANT UPDATE (series_id, label) ON tournaments TO ratings_admin;
GRANT UPDATE (series_id, tournament_id) ON cge_tournaments TO ratings_admin;

-- +goose Down
REVOKE UPDATE (series_id, tournament_id) ON cge_tournaments FROM ratings_admin;
REVOKE UPDATE (series_id, label) ON tournaments FROM ratings_admin;
REVOKE UPDATE (community_scoring, community_code, results_rules, podium_source) ON series FROM ratings_admin;
ALTER TABLE series ADD CONSTRAINT series_results_rules_key UNIQUE (results_rules);
ALTER TABLE series DROP COLUMN podium_source;
