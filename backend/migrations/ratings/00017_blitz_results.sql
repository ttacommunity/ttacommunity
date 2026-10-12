-- Blitz results: a match's result (a win, a draw or a double loss) from a played game or the organisers' ruling,
-- each edition's tiebreak seed, and the admin login's right to write the games, seats, players and stages a
-- result makes. The login still cannot delete games, players or runs: the api checks that at start.
-- +goose Up
ALTER TABLE blitz_editions ADD COLUMN tiebreak_seed bigint NOT NULL DEFAULT floor(random() * 2147483647)::bigint;
ALTER TABLE blitz_matches
    ADD COLUMN result text CHECK (result IN ('a', 'b', 'draw', 'double_loss')),
    ADD COLUMN game_id bigint UNIQUE REFERENCES games,
    ADD COLUMN ruling text NOT NULL DEFAULT '' CHECK (char_length(ruling) <= 1000),
    ADD COLUMN decided_at timestamptz,
    -- A result comes from a game or a ruling (a ruling may overrule a played game); a bye has none.
    ADD CONSTRAINT blitz_matches_result_source_check CHECK ((result IS NULL) = (game_id IS NULL AND ruling = '')),
    ADD CONSTRAINT blitz_matches_decided_check CHECK ((result IS NULL) = (decided_at IS NULL)),
    ADD CONSTRAINT blitz_matches_bye_check CHECK (entry_b IS NOT NULL OR result IS NULL);
GRANT INSERT, UPDATE ON games, seats, players, stages, groups, group_members, import_batches TO ratings_admin;

-- +goose Down
REVOKE INSERT, UPDATE ON games, seats, players, stages, groups, group_members, import_batches FROM ratings_admin;
ALTER TABLE blitz_matches DROP COLUMN result, DROP COLUMN game_id, DROP COLUMN ruling, DROP COLUMN decided_at;
ALTER TABLE blitz_editions DROP COLUMN tiebreak_seed;
