-- The Grand Slams and Competitive Ladders pages as groups whose card admin writes: the ladders get a group of their
-- own, the Grand Slams group takes every cup its page counts, and each tournament gets a format in brief, which its
-- group's page lists as its format and regulations. All start with the text the pages showed.
-- +goose Up
ALTER TABLE series ADD COLUMN format_summary text NOT NULL DEFAULT '' CHECK (char_length(format_summary) <= 300);
GRANT UPDATE (format_summary) ON series TO ratings_admin;

INSERT INTO series (slug, name, original_name, kind, is_group)
SELECT 'ladders', 'Competitive Ladders (Mercurial, Sodium & Transcontinental)',
       'Competitive Ladders (Mercurial, Sodium & Transcontinental)', 'group', true
WHERE EXISTS (SELECT 1 FROM series WHERE slug IN ('mercurial_ladder', 'sodium_ladder', 'transcontinental_ladder'));
UPDATE series SET parent_id = (SELECT series_id FROM series WHERE slug = 'ladders')
WHERE slug IN ('mercurial_ladder', 'sodium_ladder', 'transcontinental_ladder') AND parent_id IS NULL;

UPDATE series SET parent_id = (SELECT series_id FROM series WHERE slug = 'grand_slams' AND is_group)
WHERE slug IN ('survivors_cup', 'slow_burn', 'eiffel_tower') AND parent_id IS NULL
  AND EXISTS (SELECT 1 FROM series WHERE slug = 'grand_slams' AND is_group);
UPDATE series SET name = 'Grand Slams & Championship Cups',
    original_name = CASE original_name WHEN 'Grand Slams' THEN 'Grand Slams & Championship Cups' ELSE original_name END
WHERE slug = 'grand_slams' AND name = 'Grand Slams';

UPDATE series s SET format_summary = f.text
FROM (VALUES
    ('survivors_cup', '11 progressive stages of 3-player survival tables with cuts to Upper/Lower semifinals and Grand Finals.'),
    ('wimbledon', 'Direct 2-player knockout duel rounds through to Quarterfinals, Semifinals, and Finals.'),
    ('eiffel_tower', 'Multi-level qualification climbing up the tower to ''The Top'' elite division.'),
    ('slow_burn', 'Extended multi-month endurance rounds culminating in a 3-game Finals series.'),
    ('mercurial_ladder', 'Rolling periodic element tiers (Hydrogen, Helium ... Neon, Sodium) with performance rung promotions.'),
    ('sodium_ladder', 'Continuous tiered metal ladders across ascending rungs.'),
    ('transcontinental_ladder', '100-stage continuous challenge ladder.')
) AS f (slug, text)
WHERE s.slug = f.slug;

INSERT INTO tournament_cards (tournament, icon, tagline, summary, subtitle, cadence) VALUES
    ('grand_slams', '🎾', 'Major Cups & Slams', 'Prestigious open championships and multi-stage endurance cups: Survivors Cup (3P survival elimination), Wimbledon TTA (1v1 knockout duel bracket), French Open (3P/4P clay season major), Eiffel Tower Tournament (floor climbing stages to The Top), and Slow Burn.', '4-Player Major Open Format', 'Annual / Seasonal Majors'),
    ('ladders', '🪜', 'Tiered Ladders', 'Perpetual rung-based ladders where competitors battle in 3-player and 2-player encounters to ascend through dynamic tier rankings.', '2 & 3-Player Continuous Ladder Format', 'Continuous / Quarterly')
ON CONFLICT (tournament) DO NOTHING;

-- +goose Down
DELETE FROM tournament_stories WHERE tournament IN ('grand_slams', 'ladders');
DELETE FROM tournament_cards WHERE tournament IN ('grand_slams', 'ladders');
UPDATE series SET name = 'Grand Slams',
    original_name = CASE original_name WHEN 'Grand Slams & Championship Cups' THEN 'Grand Slams' ELSE original_name END
WHERE slug = 'grand_slams' AND name = 'Grand Slams & Championship Cups';
UPDATE series SET parent_id = NULL
WHERE slug IN ('survivors_cup', 'slow_burn', 'eiffel_tower')
  AND parent_id = (SELECT series_id FROM series WHERE slug = 'grand_slams');
UPDATE series SET parent_id = NULL WHERE parent_id = (SELECT series_id FROM series WHERE slug = 'ladders');
DELETE FROM series WHERE slug = 'ladders';
REVOKE UPDATE (format_summary) ON series FROM ratings_admin;
ALTER TABLE series DROP COLUMN format_summary;
