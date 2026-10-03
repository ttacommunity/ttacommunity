-- A run's latest progress line, which the engine writes as it goes, so admin can
-- show a run in progress (its own table: a step's lock on the run's row must not
-- hold it up); and admin publishes a run by making it current.

-- +goose Up
CREATE TABLE run_progress (
    run_id     bigint PRIMARY KEY REFERENCES rating_runs ON DELETE CASCADE,
    line       text NOT NULL,
    updated_at timestamptz NOT NULL DEFAULT now()
);
GRANT SELECT ON run_progress TO ratings_admin;
GRANT INSERT, UPDATE ON current_run TO ratings_admin;
GRANT UPDATE (published_at) ON rating_runs TO ratings_admin;

-- +goose Down
REVOKE UPDATE (published_at) ON rating_runs FROM ratings_admin;
REVOKE INSERT, UPDATE ON current_run FROM ratings_admin;
REVOKE SELECT ON run_progress FROM ratings_admin;
DROP TABLE run_progress;
