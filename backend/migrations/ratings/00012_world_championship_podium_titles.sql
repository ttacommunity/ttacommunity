-- Admin can set the World Championship's silver and bronze titles (WCS, WCB), as the engine gives them.
-- +goose Up
ALTER TABLE player_title_overrides DROP CONSTRAINT player_title_overrides_title_check;
ALTER TABLE player_title_overrides ADD CONSTRAINT player_title_overrides_title_check
    CHECK (title = ANY (ARRAY['SGM', 'WCS', 'WCB', 'GM', 'M', 'P', 'G', 'S', 'B', 'W', '']));

-- +goose Down
-- Fails while a WCS or WCB title is set, rather than dropping it: clear those in admin first.
ALTER TABLE player_title_overrides DROP CONSTRAINT player_title_overrides_title_check;
ALTER TABLE player_title_overrides ADD CONSTRAINT player_title_overrides_title_check
    CHECK (title = ANY (ARRAY['SGM', 'GM', 'M', 'P', 'G', 'S', 'B', 'W', '']));
