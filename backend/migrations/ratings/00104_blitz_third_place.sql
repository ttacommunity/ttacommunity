-- Blitz third-place matches: an edition may have its semi-finals' losers play for third, in a match of the final round
-- marked as the third-place match.
-- +goose Up
ALTER TABLE blitz_editions ADD COLUMN third_place boolean NOT NULL DEFAULT false;
ALTER TABLE blitz_matches
    ADD COLUMN third_place boolean NOT NULL DEFAULT false,
    ADD CONSTRAINT blitz_matches_third_place_check CHECK (NOT third_place OR slot IS NOT NULL);

-- +goose Down
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM blitz_matches WHERE third_place) THEN
        RAISE EXCEPTION 'a blitz round has a third-place match, which the old schema would take for a second final: delete that round first';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE blitz_matches DROP COLUMN third_place;
ALTER TABLE blitz_editions DROP COLUMN third_place;
