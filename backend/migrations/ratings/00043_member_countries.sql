-- A player's country lives on their member alone, where a player sets it on the site: every player with a country
-- from our data gets a member holding it (a member who chose one keeps theirs), and players loses the column.
-- +goose Up
UPDATE members m SET player_id = p.player_id
FROM players p
WHERE m.player_id IS NULL AND p.country_code IS NOT NULL AND lower(p.name) = lower(m.tta_name)
  AND NOT EXISTS (SELECT 1 FROM members o WHERE o.player_id = p.player_id);
UPDATE members m SET country_code = p.country_code
FROM players p
WHERE p.player_id = m.player_id AND m.country_code IS NULL AND p.country_code IS NOT NULL;
INSERT INTO members (tta_name, player_id, country_code)
SELECT p.name, p.player_id, p.country_code
FROM players p
WHERE p.country_code IS NOT NULL
  AND NOT EXISTS (SELECT 1 FROM members m WHERE m.player_id = p.player_id OR lower(m.tta_name) = lower(p.name));
ALTER TABLE players DROP COLUMN country_code;

-- +goose Down
-- Each member's country is copied back to their player. The members added stay: a member with a country is a shape
-- this schema holds too.
ALTER TABLE players ADD COLUMN country_code text CONSTRAINT players_country_code_check CHECK (country_code ~ '^[A-Z]{2}$');
UPDATE players p SET country_code = m.country_code FROM members m WHERE m.player_id = p.player_id AND m.country_code IS NOT NULL;
