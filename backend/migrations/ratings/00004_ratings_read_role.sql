-- ratings_read: SELECT only, for every public route. It cannot log in; each
-- deployment creates a login role that is a member of it.

-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    CREATE ROLE ratings_read NOLOGIN;
EXCEPTION WHEN duplicate_object THEN
    NULL; -- roles are per server, and another database may have made it
END
$$;
-- +goose StatementEnd
GRANT USAGE ON SCHEMA public TO ratings_read;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO ratings_read;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO ratings_read;

-- +goose Down
-- The role stays: other databases on the server may use it.
ALTER DEFAULT PRIVILEGES IN SCHEMA public REVOKE SELECT ON TABLES FROM ratings_read;
REVOKE SELECT ON ALL TABLES IN SCHEMA public FROM ratings_read;
REVOKE USAGE ON SCHEMA public FROM ratings_read;
