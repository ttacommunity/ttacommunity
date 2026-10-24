-- Every table of a run's results is partitioned by run, one partition per run (<table>_run<id>): a run's rows are
-- written, vacuumed and read on their own, and deleting a run drops its partitions instead of deleting row by row.
-- A trigger adds a new run's partitions; delete_run drops them. Existing rows are copied across, so this takes a
-- while on a large database and holds the run tables locked until it ends.
-- +goose Up
-- +goose StatementBegin
DO $$
DECLARE
    t text;
    r bigint;
    col text;
    ddl text;
    restore text[];
BEGIN
    FOR t IN SELECT conrelid::regclass::text FROM pg_constraint
             WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' ORDER BY 1 LOOP
        -- What goes back on the partitioned table: constraints, other indexes, then grants.
        restore := ARRAY(
            SELECT format('ALTER TABLE %I ADD CONSTRAINT %I %s', t, conname, pg_get_constraintdef(oid))
            FROM pg_constraint WHERE conrelid = t::regclass AND contype IN ('p', 'u', 'c', 'f', 'x')
            UNION ALL
            SELECT pg_get_indexdef(indexrelid) FROM pg_index i
            WHERE indrelid = t::regclass AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid)
            UNION ALL
            SELECT format('GRANT %s ON %I TO %s', a.privilege_type, t,
                          CASE a.grantee WHEN 0 THEN 'PUBLIC' ELSE quote_ident(a.grantee::regrole::text) END)
            FROM pg_class c, aclexplode(c.relacl) a WHERE c.oid = t::regclass AND a.grantee <> c.relowner);
        EXECUTE format('ALTER TABLE %I RENAME TO %I', t, t || '_plain');
        EXECUTE format('CREATE TABLE %I (LIKE %I INCLUDING DEFAULTS INCLUDING IDENTITY INCLUDING GENERATED '
                       'INCLUDING STORAGE INCLUDING COMPRESSION INCLUDING COMMENTS) PARTITION BY LIST (run_id)',
                       t, t || '_plain');
        FOR r IN SELECT run_id FROM rating_runs ORDER BY 1 LOOP
            EXECUTE format('CREATE TABLE %I PARTITION OF %I FOR VALUES IN (%s)', t || '_run' || r, t, r);
        END LOOP;
        EXECUTE format('INSERT INTO %I OVERRIDING SYSTEM VALUE SELECT * FROM %I', t, t || '_plain');
        EXECUTE format('DROP TABLE %I', t || '_plain');
        FOREACH ddl IN ARRAY restore LOOP
            EXECUTE ddl;
        END LOOP;
        FOR col IN SELECT attname FROM pg_attribute WHERE attrelid = t::regclass AND attidentity <> '' LOOP
            EXECUTE format('ALTER SEQUENCE %s RENAME TO %I', pg_get_serial_sequence(t, col), t || '_' || col || '_seq');
            EXECUTE format('SELECT setval(%L, coalesce(max(%I), 0) + 1, false) FROM %I',
                           t || '_' || col || '_seq', col, t);
        END LOOP;
        EXECUTE format('ANALYZE %I', t);
    END LOOP;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION add_run_partitions() RETURNS trigger
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    t text;
BEGIN
    -- Made apart, then attached: attaching leaves the table readable, creating it as a partition would not.
    FOR t IN SELECT c.relname FROM pg_partitioned_table p JOIN pg_class c ON c.oid = p.partrelid
             JOIN pg_attribute a ON a.attrelid = p.partrelid AND a.attnum = p.partattrs[0]
             WHERE c.relnamespace = 'public'::regnamespace AND a.attname = 'run_id' LOOP
        EXECUTE format('CREATE TABLE %I (LIKE %I INCLUDING DEFAULTS INCLUDING CONSTRAINTS INCLUDING GENERATED)',
                       t || '_run' || NEW.run_id, t);
        EXECUTE format('ALTER TABLE %I ATTACH PARTITION %I FOR VALUES IN (%s)', t, t || '_run' || NEW.run_id, NEW.run_id);
    END LOOP;
    RETURN NULL;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION add_run_partitions() FROM PUBLIC;
CREATE TRIGGER rating_runs_partitions AFTER INSERT ON rating_runs
    FOR EACH ROW EXECUTE FUNCTION add_run_partitions();

-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_run(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    p regclass;
BEGIN
    PERFORM 1 FROM rating_runs r
    WHERE r.run_id = id AND r.deleting AND NOT r.protected
      AND NOT EXISTS (SELECT 1 FROM current_run c WHERE c.run_id = r.run_id)
    FOR UPDATE;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'run % is not marked for deletion, or is protected or current', id;
    END IF;
    FOR p IN SELECT c.oid::regclass FROM pg_class c
             WHERE c.relispartition AND pg_get_expr(c.relpartbound, c.oid) = format('FOR VALUES IN (%L)', id) LOOP
        EXECUTE format('DROP TABLE %s', p);
    END LOOP;
    DELETE FROM rating_runs WHERE run_id = id;
END
$$;
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION delete_run(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
BEGIN
    DELETE FROM rating_runs r
    WHERE r.run_id = id AND r.deleting AND NOT r.protected
      AND NOT EXISTS (SELECT 1 FROM current_run c WHERE c.run_id = r.run_id);
    IF NOT FOUND THEN
        RAISE EXCEPTION 'run % is not marked for deletion, or is protected or current', id;
    END IF;
END
$$;
-- +goose StatementEnd
DROP TRIGGER rating_runs_partitions ON rating_runs;
DROP FUNCTION add_run_partitions();

-- +goose StatementBegin
DO $$
DECLARE
    t text;
    col text;
    ddl text;
    restore text[];
BEGIN
    FOR t IN SELECT conrelid::regclass::text FROM pg_constraint
             WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' AND conparentid = 0
             ORDER BY 1 LOOP
        restore := ARRAY(
            SELECT format('ALTER TABLE %I ADD CONSTRAINT %I %s', t, conname, pg_get_constraintdef(oid))
            FROM pg_constraint WHERE conrelid = t::regclass AND contype IN ('p', 'u', 'c', 'f', 'x')
            UNION ALL
            SELECT replace(pg_get_indexdef(indexrelid), ' ON ONLY ', ' ON ') FROM pg_index i
            WHERE indrelid = t::regclass AND NOT EXISTS (SELECT 1 FROM pg_constraint c WHERE c.conindid = i.indexrelid)
            UNION ALL
            SELECT format('GRANT %s ON %I TO %s', a.privilege_type, t,
                          CASE a.grantee WHEN 0 THEN 'PUBLIC' ELSE quote_ident(a.grantee::regrole::text) END)
            FROM pg_class c, aclexplode(c.relacl) a WHERE c.oid = t::regclass AND a.grantee <> c.relowner);
        EXECUTE format('ALTER TABLE %I RENAME TO %I', t, t || '_parted');
        EXECUTE format('CREATE TABLE %I (LIKE %I INCLUDING DEFAULTS INCLUDING IDENTITY INCLUDING GENERATED '
                       'INCLUDING STORAGE INCLUDING COMPRESSION INCLUDING COMMENTS)', t, t || '_parted');
        EXECUTE format('INSERT INTO %I OVERRIDING SYSTEM VALUE SELECT * FROM %I', t, t || '_parted');
        EXECUTE format('DROP TABLE %I', t || '_parted');
        FOREACH ddl IN ARRAY restore LOOP
            EXECUTE ddl;
        END LOOP;
        FOR col IN SELECT attname FROM pg_attribute WHERE attrelid = t::regclass AND attidentity <> '' LOOP
            EXECUTE format('ALTER SEQUENCE %s RENAME TO %I', pg_get_serial_sequence(t, col), t || '_' || col || '_seq');
            EXECUTE format('SELECT setval(%L, coalesce(max(%I), 0) + 1, false) FROM %I',
                           t || '_' || col || '_seq', col, t);
        END LOOP;
        EXECUTE format('ANALYZE %I', t);
    END LOOP;
END
$$;
-- +goose StatementEnd
