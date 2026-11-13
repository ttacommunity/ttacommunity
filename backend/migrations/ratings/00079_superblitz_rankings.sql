-- SuperBlitz and Pulse rankings and their series mapping.
-- +goose Up
INSERT INTO rankings (slug, name, description) VALUES
    ('superblitz', 'SuperBlitz', 'Scheduled SuperBlitz tournaments (US Open and future blitz events)'),
    ('pulse', 'TTA Pulse', 'Real-time blitz matchmaking queue'),
    ('community_superblitz', 'Community + SuperBlitz', 'Classical community tournaments combined with scheduled SuperBlitz tournaments');

INSERT INTO ranking_series (ranking_id, series_id)
SELECT r.ranking_id, s.series_id
FROM rankings r, series s
WHERE r.slug = 'superblitz' AND s.slug = 'us_open';

INSERT INTO ranking_series (ranking_id, series_id)
SELECT cs.ranking_id, s.series_id
FROM rankings cs
CROSS JOIN (
    SELECT series_id FROM ranking_series WHERE ranking_id = (SELECT ranking_id FROM rankings WHERE slug = 'community')
    UNION
    SELECT series_id FROM series WHERE slug = 'us_open'
) s
WHERE cs.slug = 'community_superblitz';

-- +goose Down
DELETE FROM ranking_series WHERE ranking_id IN (
    SELECT ranking_id FROM rankings WHERE slug IN ('superblitz', 'pulse', 'community_superblitz')
);
DELETE FROM rankings WHERE slug IN ('superblitz', 'pulse', 'community_superblitz');
