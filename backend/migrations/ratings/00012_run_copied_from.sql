-- A run that recomputed only tournament results names the run whose ratings
-- and calibration it copied. Null for a full run.

-- +goose Up
ALTER TABLE rating_runs ADD COLUMN copied_from bigint REFERENCES rating_runs (run_id);

-- +goose Down
ALTER TABLE rating_runs DROP COLUMN copied_from;
