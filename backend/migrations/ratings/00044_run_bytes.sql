-- The bytes of the rows a run wrote to its result tables, recorded once as the run ends, so admin shows a run's
-- size as it was, unmoved by later deletes or vacuums. Runs already held are measured here.
-- +goose Up
ALTER TABLE rating_runs ADD COLUMN result_bytes bigint NOT NULL DEFAULT 0;

-- +goose StatementBegin
DO $$
DECLARE t text;
BEGIN
    FOR t IN SELECT conrelid::regclass::text FROM pg_constraint
             WHERE confrelid = 'rating_runs'::regclass AND contype = 'f' AND confdeltype = 'c' LOOP
        EXECUTE format('UPDATE rating_runs r SET result_bytes = r.result_bytes + c.b
                        FROM (SELECT run_id, sum(pg_column_size(t.*)) AS b FROM %s t GROUP BY run_id) c WHERE c.run_id = r.run_id', t);
    END LOOP;
END $$;
-- +goose StatementEnd

-- +goose Down
ALTER TABLE rating_runs DROP COLUMN result_bytes;
