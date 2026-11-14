-- Mark US Open seasons rated so the SuperBlitz and Community+SuperBlitz pipelines include them
-- (US Open is not in the community ranking, so Standard ratings still exclude it).
-- +goose Up
UPDATE tournaments SET rated = true
WHERE series_id = (SELECT series_id FROM series WHERE slug = 'us_open');

-- +goose Down
UPDATE tournaments SET rated = false
WHERE series_id = (SELECT series_id FROM series WHERE slug = 'us_open');
