-- Whether a season is run on this site, CGE only advertising it: its CGE tournament gives its page, never its
-- stages or games. Set in admin.
-- +goose Up
ALTER TABLE tournaments ADD COLUMN run_here boolean NOT NULL DEFAULT false;
GRANT UPDATE (run_here) ON tournaments TO ratings_admin;

-- +goose Down
REVOKE UPDATE (run_here) ON tournaments FROM ratings_admin;
ALTER TABLE tournaments DROP COLUMN run_here;
