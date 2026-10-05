-- Admins manage the admin list from the admin page; the API allows it to the admin role only.

-- +goose Up
GRANT INSERT, UPDATE, DELETE ON admins TO ratings_admin;

-- +goose Down
REVOKE INSERT, UPDATE, DELETE ON admins FROM ratings_admin;
