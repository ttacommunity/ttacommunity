-- A banned member signs in and sees their own ban: the player login may read a member's ban reason.
-- +goose Up
GRANT SELECT (ban_reason) ON members TO ratings_player;

-- +goose Down
REVOKE SELECT (ban_reason) ON members FROM ratings_player;
