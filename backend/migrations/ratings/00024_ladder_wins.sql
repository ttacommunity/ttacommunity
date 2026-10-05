-- Each player's wins in a ladder's top tier, as the ladder's organisers count
-- them (a tie can give half a win): a recorded fact the load job writes.

-- +goose Up
CREATE TABLE tournament_ladder_wins (
    ladder text NOT NULL,
    player_name text NOT NULL,
    wins numeric(5, 1) NOT NULL CHECK (wins > 0),
    source text NOT NULL,
    PRIMARY KEY (ladder, player_name)
);

-- +goose Down
DROP TABLE tournament_ladder_wins;
