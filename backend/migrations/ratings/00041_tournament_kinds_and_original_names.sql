-- What used to be lists in the code, as data admin can set: a tournament's
-- kind (league, cup or ladder) and the divisions whose standings give a
-- league's podium; and the card's subtitle and cadence. Every tournament,
-- season and stage also keeps the name it had when it came in, so one we
-- rename can still be matched to its source.

-- +goose Up
ALTER TABLE tournaments ADD COLUMN kind text NOT NULL DEFAULT 'cup' CHECK (kind IN ('league', 'cup', 'ladder'));
ALTER TABLE tournaments ADD COLUMN top_divisions text[] NOT NULL DEFAULT '{}';
UPDATE tournaments SET kind = 'league' WHERE slug IN
    ('international', 'intermezzo', 'royal_league', 'nations_league', 'premier_league', 'duel_curve', 'eiffel_tower');
UPDATE tournaments SET kind = 'ladder' WHERE slug IN ('mercurial_ladder', 'sodium_ladder', 'transcontinental_ladder');
UPDATE tournaments SET top_divisions = '{Grandmaster,Diamond}' WHERE slug IN ('international', 'intermezzo');
UPDATE tournaments SET top_divisions = '{Emperor}' WHERE slug = 'royal_league';
UPDATE tournaments SET top_divisions = '{Final}' WHERE slug = 'worlds';
UPDATE tournaments SET top_divisions = '{Apple}' WHERE slug = 'nations_league';
GRANT UPDATE (kind, top_divisions) ON tournaments TO ratings_admin;

ALTER TABLE tournament_cards ADD COLUMN subtitle text NOT NULL DEFAULT '' CHECK (length(subtitle) <= 60);
ALTER TABLE tournament_cards ADD COLUMN cadence text NOT NULL DEFAULT '' CHECK (length(cadence) <= 60);
UPDATE tournament_cards SET subtitle = '3/4-Player Competitive Format', cadence = 'Quarterly (4 seasons / year)' WHERE tournament = 'international';
UPDATE tournament_cards SET subtitle = '3-Player Competitive Format', cadence = 'Quarterly (4 seasons / year)' WHERE tournament = 'intermezzo';
UPDATE tournament_cards SET subtitle = '2-Player Competitive Format', cadence = 'Quarterly (4 seasons / year)' WHERE tournament = 'royal_league';
UPDATE tournament_cards SET subtitle = '4-Player Competitive Format', cadence = 'Annual (Every Year)' WHERE tournament = 'worlds';

ALTER TABLE tournaments ADD COLUMN original_name text NOT NULL DEFAULT '';
ALTER TABLE tournament_seasons ADD COLUMN original_name text NOT NULL DEFAULT '';
ALTER TABLE tournament_stages ADD COLUMN original_name text NOT NULL DEFAULT '';
UPDATE tournaments SET original_name = name;
UPDATE tournament_seasons SET original_name = label;
UPDATE tournament_stages SET original_name = label;

-- +goose Down
ALTER TABLE tournament_stages DROP COLUMN original_name;
ALTER TABLE tournament_seasons DROP COLUMN original_name;
ALTER TABLE tournaments DROP COLUMN original_name;
ALTER TABLE tournament_cards DROP COLUMN cadence;
ALTER TABLE tournament_cards DROP COLUMN subtitle;
REVOKE UPDATE (kind, top_divisions) ON tournaments FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN top_divisions;
ALTER TABLE tournaments DROP COLUMN kind;
