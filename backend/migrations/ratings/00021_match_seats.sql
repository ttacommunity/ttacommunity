-- What CGE's pages say about a game already loaded: each seat's CGE user and
-- how it ended, and the game's CGE tournament, stage and finish date. The
-- load job attaches them; scores and dates the ratings use are not changed.

-- +goose Up
CREATE TABLE match_seats (
    match_id bigint NOT NULL REFERENCES matches ON DELETE CASCADE,
    seat smallint NOT NULL CHECK (seat BETWEEN 1 AND 4),
    player_name text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('finished', 'resigned', 'timed_out')),
    cge_user integer CHECK (cge_user > 0),
    banned boolean NOT NULL DEFAULT false,
    PRIMARY KEY (match_id, seat)
);

ALTER TABLE matches
    ADD COLUMN cge_tournament integer CHECK (cge_tournament > 0),
    ADD COLUMN cge_season integer CHECK (cge_season >= 0),
    ADD COLUMN cge_finished date;

-- +goose Down
ALTER TABLE matches
    DROP COLUMN cge_finished,
    DROP COLUMN cge_season,
    DROP COLUMN cge_tournament;
DROP TABLE match_seats;
