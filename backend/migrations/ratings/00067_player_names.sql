-- Every name a player has gone by, deleted and banned accounts' too: the first and last day a game shows it, and
-- where we saw it. A name can belong to several players at different times. Filled here from the names on the
-- seats; the names other files gave (the first match file, CGE's extract, organisers' sheets) come with the data.
-- +goose Up
CREATE TABLE player_names (
    player_id bigint NOT NULL REFERENCES players,
    name text NOT NULL CHECK (name <> ''),
    first_seen date NOT NULL,
    last_seen date NOT NULL CHECK (last_seen >= first_seen),
    source text NOT NULL CHECK (source IN ('games', 'match_file', 'cge_extract', 'sheet')),
    PRIMARY KEY (player_id, name)
);
CREATE INDEX player_names_lower ON player_names (lower(name));
GRANT SELECT ON player_names TO ratings_read, ratings_admin, ratings_backup;

INSERT INTO player_names (player_id, name, first_seen, last_seen, source)
SELECT s.player_id, s.player_name, min(g.date), max(g.date), 'games'
FROM seats s JOIN games g USING (game_id)
GROUP BY s.player_id, s.player_name;

-- +goose Down
DROP TABLE player_names;
