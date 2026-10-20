-- A player's own country and time zone, given when they sign in before an admin or organiser links them to a
-- member: shown to whoever answers their request, then moved to the member.
-- +goose Up
ALTER TABLE logins
    ADD COLUMN country_code text CHECK (country_code ~ '^[A-Z]{2}$'),
    ADD COLUMN zone text CHECK (zone <> '' AND char_length(zone) <= 64);

-- +goose Down
ALTER TABLE logins DROP COLUMN country_code, DROP COLUMN zone;
