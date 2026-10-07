-- What each tournament calls its seasons and stages, set in admin ('' for the defaults, Season and Stage).
-- Slow Burn's editions are its seasons and its stages are what its rules call seasons, so it takes those words.
-- +goose Up
ALTER TABLE tournaments
    ADD COLUMN season_name text NOT NULL DEFAULT '' CHECK (char_length(season_name) <= 32),
    ADD COLUMN stage_name text NOT NULL DEFAULT '' CHECK (char_length(stage_name) <= 32);

GRANT UPDATE (season_name, stage_name) ON tournaments TO ratings_admin;

UPDATE tournaments SET season_name = 'Edition', stage_name = 'Season' WHERE slug = 'slow_burn';

UPDATE tournament_season_notes n SET stage = 'Season ' || substring(n.stage FROM '^Stage ([0-9]+)$')
WHERE n.tournament = 'slow_burn' AND n.stage ~ '^Stage [0-9]+$';

UPDATE tournament_stages st SET label = 'Season ' || substring(st.label FROM '^Stage ([0-9]+)$'),
                                original_name = 'Season ' || substring(st.label FROM '^Stage ([0-9]+)$')
FROM tournament_seasons se JOIN tournaments t USING (tournament_id)
WHERE st.season_id = se.season_id AND t.slug = 'slow_burn' AND st.label ~ '^Stage [0-9]+$';

-- +goose Down
UPDATE tournament_stages st SET label = 'Stage ' || substring(st.label FROM '^Season ([0-9]+)$'),
                                original_name = 'Stage ' || substring(st.label FROM '^Season ([0-9]+)$')
FROM tournament_seasons se JOIN tournaments t USING (tournament_id)
WHERE st.season_id = se.season_id AND t.slug = 'slow_burn' AND st.label ~ '^Season [0-9]+$';

UPDATE tournament_season_notes n SET stage = 'Stage ' || substring(n.stage FROM '^Season ([0-9]+)$')
WHERE n.tournament = 'slow_burn' AND n.stage ~ '^Season [0-9]+$';

REVOKE UPDATE (season_name, stage_name) ON tournaments FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN season_name, DROP COLUMN stage_name;
