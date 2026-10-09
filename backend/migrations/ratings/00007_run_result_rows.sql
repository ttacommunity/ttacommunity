-- Each run's rows in each of its result tables, recorded once as the run ends, so admin shows a run's size
-- without counting its rows. Runs already held are counted here.
-- +goose Up
ALTER TABLE rating_runs ADD COLUMN result_rows jsonb NOT NULL DEFAULT '{}';

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT conrelid::regclass::text FROM pg_constraint
             WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' LOOP
        EXECUTE format('UPDATE rating_runs r SET result_rows = r.result_rows || jsonb_build_object(%L, c.n)
                        FROM (SELECT run_id, count(*) AS n FROM %s GROUP BY run_id) c WHERE c.run_id = r.run_id', t, t);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE rating_runs DROP COLUMN result_rows;
