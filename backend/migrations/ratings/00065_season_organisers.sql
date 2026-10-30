-- Each season's organiser, as admin sets it: empty for none, when the site names its CGE tournament's organiser.
-- +goose Up
ALTER TABLE tournaments ADD COLUMN organiser text NOT NULL DEFAULT '' CHECK (length(organiser) <= 100);
GRANT UPDATE (organiser) ON tournaments TO ratings_admin;

-- +goose Down
REVOKE UPDATE (organiser) ON tournaments FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN organiser;
