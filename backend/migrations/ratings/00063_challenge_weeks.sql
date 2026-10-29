-- The weekly challenge, a finished event (September 2024 to July 2025): each week's challenge and the names that won
-- or came second, as the old site kept them. Loaded whole by the load job; nothing else writes it.
-- +goose Up
CREATE TABLE challenge_weeks (
    week integer PRIMARY KEY CHECK (week > 0),
    week_of date NOT NULL,
    challenge_id text NOT NULL REFERENCES challenges
);

CREATE TABLE challenge_week_places (
    week integer NOT NULL REFERENCES challenge_weeks ON DELETE CASCADE,
    place text NOT NULL CHECK (place IN ('winner', 'runner_up')),
    position integer NOT NULL CHECK (position >= 0),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    domination integer NOT NULL CHECK (domination BETWEEN -9999 AND 9999),
    PRIMARY KEY (week, place, position)
);

GRANT SELECT ON challenge_weeks, challenge_week_places TO ratings_read, ratings_admin, ratings_backup;

-- +goose Down
DROP TABLE challenge_week_places;
DROP TABLE challenge_weeks;
