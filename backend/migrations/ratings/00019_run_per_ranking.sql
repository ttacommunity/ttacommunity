-- Each run rates one ranking, and each ranking has its own current run. A run
-- that names no ranking is the community ranking's, as every run was before.

-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION community_ranking() RETURNS bigint LANGUAGE sql STABLE AS $$
    SELECT ranking_id FROM rankings WHERE slug = 'community'
$$;
-- +goose StatementEnd

ALTER TABLE rating_runs ADD COLUMN ranking_id bigint REFERENCES rankings DEFAULT community_ranking();
UPDATE rating_runs SET ranking_id = community_ranking();
ALTER TABLE rating_runs ALTER COLUMN ranking_id SET NOT NULL;
ALTER TABLE rating_runs ADD UNIQUE (run_id, ranking_id);

-- A ranking's current run must be one of its own.
ALTER TABLE current_run ADD COLUMN ranking_id bigint DEFAULT community_ranking();
UPDATE current_run SET ranking_id = community_ranking();
ALTER TABLE current_run
    ALTER COLUMN ranking_id SET NOT NULL,
    DROP CONSTRAINT current_run_pkey,
    DROP COLUMN only_row,
    ADD PRIMARY KEY (ranking_id),
    ADD FOREIGN KEY (run_id, ranking_id) REFERENCES rating_runs (run_id, ranking_id);

-- +goose Down
DELETE FROM current_run WHERE ranking_id <> community_ranking();
ALTER TABLE current_run
    DROP CONSTRAINT current_run_pkey,
    DROP CONSTRAINT current_run_run_id_ranking_id_fkey,
    DROP COLUMN ranking_id,
    ADD COLUMN only_row boolean PRIMARY KEY DEFAULT true CHECK (only_row);
ALTER TABLE rating_runs DROP COLUMN ranking_id;
DROP FUNCTION community_ranking();
