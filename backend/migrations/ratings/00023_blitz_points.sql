-- What each outcome of an edition's games is worth, 0 to 10 points in halves, FIDE's by default: a win 1, a draw a
-- half, a loss 0, a win or loss by forfeit as one played, the pairing's bye as a win, a skipped round 0.
-- +goose Up
ALTER TABLE blitz_editions
    ADD COLUMN points_win numeric NOT NULL DEFAULT 1
        CONSTRAINT blitz_editions_points_win_check CHECK (points_win BETWEEN 0 AND 10 AND points_win * 2 = trunc(points_win * 2)),
    ADD COLUMN points_draw numeric NOT NULL DEFAULT 0.5
        CONSTRAINT blitz_editions_points_draw_check CHECK (points_draw BETWEEN 0 AND 10 AND points_draw * 2 = trunc(points_draw * 2)),
    ADD COLUMN points_loss numeric NOT NULL DEFAULT 0
        CONSTRAINT blitz_editions_points_loss_check CHECK (points_loss BETWEEN 0 AND 10 AND points_loss * 2 = trunc(points_loss * 2)),
    ADD COLUMN points_forfeit_win numeric NOT NULL DEFAULT 1
        CONSTRAINT blitz_editions_points_forfeit_win_check CHECK (points_forfeit_win BETWEEN 0 AND 10 AND points_forfeit_win * 2 = trunc(points_forfeit_win * 2)),
    ADD COLUMN points_forfeit_loss numeric NOT NULL DEFAULT 0
        CONSTRAINT blitz_editions_points_forfeit_loss_check CHECK (points_forfeit_loss BETWEEN 0 AND 10 AND points_forfeit_loss * 2 = trunc(points_forfeit_loss * 2)),
    ADD COLUMN points_bye numeric NOT NULL DEFAULT 1
        CONSTRAINT blitz_editions_points_bye_check CHECK (points_bye BETWEEN 0 AND 10 AND points_bye * 2 = trunc(points_bye * 2)),
    ADD COLUMN points_skip numeric NOT NULL DEFAULT 0
        CONSTRAINT blitz_editions_points_skip_check CHECK (points_skip BETWEEN 0 AND 10 AND points_skip * 2 = trunc(points_skip * 2)),
    ADD CONSTRAINT blitz_editions_points_order_check CHECK (points_win >= points_draw AND points_draw >= points_loss);

-- +goose Down
ALTER TABLE blitz_editions
    DROP COLUMN points_win,
    DROP COLUMN points_draw,
    DROP COLUMN points_loss,
    DROP COLUMN points_forfeit_win,
    DROP COLUMN points_forfeit_loss,
    DROP COLUMN points_bye,
    DROP COLUMN points_skip;
