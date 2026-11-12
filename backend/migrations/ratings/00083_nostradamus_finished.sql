-- A Nostradamus game finished by its organiser, so the site lists it among past games; a game also counts as
-- finished once every CGE tournament of its season has, which needs nothing stored.
-- +goose Up
ALTER TABLE nostradamus_editions ADD COLUMN finished_at timestamptz;
GRANT UPDATE (finished_at) ON nostradamus_editions TO ratings_admin;

-- +goose Down
REVOKE UPDATE (finished_at) ON nostradamus_editions FROM ratings_admin;
ALTER TABLE nostradamus_editions DROP COLUMN finished_at;
