-- A member's zone may be Unknown (paired as UTC) or a region (US East, Europe Central) as well as an IANA zone.
-- Unknown becomes the default, and the members given UTC by the old default are Unknown: nothing told them apart.
-- +goose Up
ALTER TABLE members ALTER COLUMN zone SET DEFAULT 'Unknown';
UPDATE members SET zone = 'Unknown' WHERE zone = 'UTC';

-- +goose Down
-- The old code knows only IANA zones: Unknown and the regions go back to UTC.
UPDATE members SET zone = 'UTC' WHERE zone NOT LIKE '%/%';
ALTER TABLE members ALTER COLUMN zone SET DEFAULT 'UTC';
