-- Every SECURITY DEFINER function searches the temporary schema last, as PostgreSQL's "Writing SECURITY DEFINER
-- Functions Safely" advises, so an object a session creates there cannot stand in for one the function uses.
-- +goose Up
ALTER FUNCTION delete_run(bigint) SET search_path TO public, pg_temp;
ALTER FUNCTION delete_blitz_game(bigint) SET search_path TO public, pg_temp;
ALTER FUNCTION delete_own_account(bytea, timestamptz) SET search_path TO public, pg_temp;
ALTER FUNCTION link_by_email(bigint, timestamptz) SET search_path TO public, pg_temp;
ALTER FUNCTION remove_own_login(bytea, text, timestamptz) SET search_path TO public, pg_temp;
ALTER FUNCTION named_member(text) SET search_path TO public, pg_temp;
ALTER FUNCTION name_linked_elsewhere(text, bigint) SET search_path TO public, pg_temp;

-- +goose Down
ALTER FUNCTION delete_run(bigint) SET search_path TO 'public';
ALTER FUNCTION delete_blitz_game(bigint) SET search_path TO 'public';
ALTER FUNCTION delete_own_account(bytea, timestamptz) SET search_path TO 'public';
ALTER FUNCTION link_by_email(bigint, timestamptz) SET search_path TO 'public';
ALTER FUNCTION remove_own_login(bytea, text, timestamptz) SET search_path TO 'public';
ALTER FUNCTION named_member(text) SET search_path TO 'public';
ALTER FUNCTION name_linked_elsewhere(text, bigint) SET search_path TO 'public';
