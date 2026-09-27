-- The engine's input: every match loaded, every row refused and why, and
-- the players named in the matches.

-- +goose Up
CREATE TABLE import_batches (
    batch_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    source text NOT NULL CHECK (source IN ('load', 'admin')),
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE matches (
    match_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id bigint NOT NULL REFERENCES import_batches,
    source_file text NOT NULL,
    source_line integer NOT NULL,
    tournament text NOT NULL,
    date date NOT NULL,
    player_count smallint NOT NULL CHECK (player_count BETWEEN 2 AND 4),
    player1 text NOT NULL,
    score1 double precision NOT NULL,
    player2 text NOT NULL,
    score2 double precision NOT NULL,
    player3 text,
    score3 double precision,
    player4 text,
    score4 double precision,
    replay_code text,
    glicko_eligible boolean NOT NULL DEFAULT true,
    CONSTRAINT matches_one_copy_per_game UNIQUE NULLS NOT DISTINCT (
        tournament, date, player1, score1, player2, score2, player3, score3, player4, score4
    )
);

CREATE INDEX matches_date ON matches (date);
CREATE INDEX matches_player1 ON matches (player1, date);
CREATE INDEX matches_player2 ON matches (player2, date);
CREATE INDEX matches_player3 ON matches (player3, date);
CREATE INDEX matches_player4 ON matches (player4, date);

CREATE TABLE refused_rows (
    refused_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    batch_id bigint NOT NULL REFERENCES import_batches,
    source_file text NOT NULL,
    source_line integer NOT NULL,
    reason text NOT NULL,
    fields text[] NOT NULL
);

CREATE INDEX refused_rows_reason ON refused_rows (reason);

-- Names are matched exactly: players who differ only by case are different players.
CREATE TABLE players (
    player_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name text NOT NULL UNIQUE
);

-- +goose Down
DROP TABLE players;
DROP TABLE refused_rows;
DROP TABLE matches;
DROP TABLE import_batches;
