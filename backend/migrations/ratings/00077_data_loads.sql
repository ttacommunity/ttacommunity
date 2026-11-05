-- The loads of games that changed what is rated, each dated by its last finished game. The leaderboard's
-- "last update" deltas compare with the board made from the load before the latest. The load job records them.
-- +goose Up
CREATE TABLE data_loads (
    batch_id bigint PRIMARY KEY REFERENCES import_batches,
    last_game date NOT NULL
);
GRANT SELECT ON data_loads TO ratings_read, ratings_admin, ratings_backup;

-- +goose Down
DROP TABLE data_loads;
