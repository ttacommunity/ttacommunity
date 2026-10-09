-- How the engine treats each series, held with it instead of in the engine by slug: how its places score in the
-- community leaderboard and its column code in the organisers' sheet ("ic_{}" adds the season), and which of the
-- engine's rule sets derives its standings, trophyboard and records (one series each).
-- +goose Up
ALTER TABLE series
    ADD COLUMN community_scoring text CHECK (community_scoring IN ('league', 'worlds', 'rounds', 'knockout', 'floors')),
    ADD COLUMN community_code text CHECK (community_code ~ '^[a-z]+(_\{(:02d)?\})?$'),
    ADD COLUMN results_rules text UNIQUE CHECK (results_rules IN ('international', 'intermezzo', 'royal_league', 'worlds'));

UPDATE series SET community_scoring = v.scoring FROM (VALUES
    ('international', 'league'), ('intermezzo', 'league'), ('worlds', 'worlds'),
    ('australian_open', 'rounds'), ('wimbledon', 'knockout'), ('eiffel_tower', 'floors')) AS v(slug, scoring)
WHERE series.slug = v.slug;
UPDATE series SET community_code = v.code FROM (VALUES
    ('international', 'ic_{}'), ('intermezzo', 'im_{}'), ('royal_league', 'rl_{:02d}'), ('worlds', 'wrld'),
    ('australian_open', 'aus'), ('wimbledon', 'wmbl'), ('eiffel_tower', 'eif'), ('mercurial_ladder', 'ml'),
    ('sodium_ladder', 'sl')) AS v(slug, code)
WHERE series.slug = v.slug;
UPDATE series SET results_rules = slug WHERE slug IN ('international', 'intermezzo', 'royal_league', 'worlds');

-- +goose Down
ALTER TABLE series DROP COLUMN community_scoring, DROP COLUMN community_code, DROP COLUMN results_rules;
