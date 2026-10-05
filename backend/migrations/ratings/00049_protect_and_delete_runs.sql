-- Admin protects runs to keep and deletes the rest. A protected run cannot be
-- deleted; a run marked deleting is being removed in the background (its
-- results go with it: every run table cascades). The oldest run, the
-- baseline, starts protected. The admin login still may not delete from
-- rating_runs: only through delete_run, which deletes nothing but a run marked
-- for deletion that is neither protected nor current.

-- +goose Up
ALTER TABLE rating_runs
    ADD COLUMN protected boolean NOT NULL DEFAULT false,
    ADD COLUMN deleting boolean NOT NULL DEFAULT false;

UPDATE rating_runs SET protected = true WHERE run_id = (SELECT min(run_id) FROM rating_runs);

GRANT UPDATE (protected, deleting) ON rating_runs TO ratings_admin;

-- +goose StatementBegin
CREATE FUNCTION delete_run(id bigint) RETURNS void
LANGUAGE plpgsql SECURITY DEFINER SET search_path = public AS $$
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

REVOKE ALL ON FUNCTION delete_run(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION delete_run(bigint) TO ratings_admin;

-- +goose Down
DROP FUNCTION delete_run(bigint);
REVOKE UPDATE (protected, deleting) ON rating_runs FROM ratings_admin;
ALTER TABLE rating_runs DROP COLUMN deleting, DROP COLUMN protected;
