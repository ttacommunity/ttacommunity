-- Admin can add a season before it has games (to carry its note) and remove one that has none.

-- +goose Up
GRANT INSERT, DELETE ON tournament_seasons TO ratings_admin;

-- +goose Down
REVOKE INSERT, DELETE ON tournament_seasons FROM ratings_admin;
