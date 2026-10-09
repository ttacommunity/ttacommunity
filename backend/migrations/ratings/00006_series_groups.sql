-- Groups of related tournaments, made in admin: a group holds tournaments and groups, has no games of its own,
-- and has a type of its own, "series" (editions of one event) or "group". Only a group holds members, and
-- nothing is in itself, directly or through its members. Admin renames a group and writes its description.
-- +goose Up
ALTER TABLE series
    ADD COLUMN parent_id bigint REFERENCES series,
    ADD COLUMN description text NOT NULL DEFAULT '' CHECK (char_length(description) <= 2000),
    ADD COLUMN is_group boolean NOT NULL DEFAULT false;
CREATE INDEX series_parent ON series (parent_id);

ALTER TABLE series DROP CONSTRAINT tournaments_kind_check,
    ADD CONSTRAINT series_kind_check CHECK (kind IN ('league', 'cup', 'ladder', 'group', 'series'));

-- +goose StatementBegin
CREATE FUNCTION series_no_cycle() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    IF NEW.parent_id IS NOT NULL AND NOT (SELECT is_group FROM series WHERE series_id = NEW.parent_id) THEN
        RAISE EXCEPTION 'series % can only be in a group', NEW.slug
            USING ERRCODE = 'check_violation', CONSTRAINT = 'series_parent_group';
    END IF;
    IF NEW.parent_id IS NOT NULL AND EXISTS (
        WITH RECURSIVE up (id) AS (
            SELECT NEW.parent_id
            UNION SELECT s.parent_id FROM series s JOIN up ON s.series_id = up.id WHERE s.parent_id IS NOT NULL)
        SELECT 1 FROM up WHERE id = NEW.series_id) THEN
        RAISE EXCEPTION 'series % would be in itself', NEW.slug
            USING ERRCODE = 'check_violation', CONSTRAINT = 'series_no_cycle';
    END IF;
    RETURN NEW;
END $$;
-- +goose StatementEnd
CREATE TRIGGER series_no_cycle BEFORE INSERT OR UPDATE OF parent_id ON series
    FOR EACH ROW EXECUTE FUNCTION series_no_cycle();

GRANT INSERT (slug, name, original_name, kind, description, is_group) ON series TO ratings_admin;
GRANT UPDATE (parent_id, description, name) ON series TO ratings_admin;

-- +goose Down
REVOKE UPDATE (parent_id, description, name) ON series FROM ratings_admin;
REVOKE INSERT (slug, name, original_name, kind, description, is_group) ON series FROM ratings_admin;
DROP TRIGGER series_no_cycle ON series;
DROP FUNCTION series_no_cycle();
UPDATE series SET kind = 'cup' WHERE kind IN ('group', 'series');
ALTER TABLE series DROP CONSTRAINT series_kind_check,
    ADD CONSTRAINT tournaments_kind_check CHECK (kind IN ('league', 'cup', 'ladder'));
ALTER TABLE series DROP COLUMN parent_id, DROP COLUMN description, DROP COLUMN is_group;
