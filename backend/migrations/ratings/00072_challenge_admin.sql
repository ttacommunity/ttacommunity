-- Admins look after challenge entries: they correct an entry's domination or proof, mark it checked or bugged, hide
-- it with a reason or show it again, and merge a name into another or into the member it belongs to.
-- +goose Up
GRANT UPDATE (domination, proof_url, checked, bugged, hidden, hidden_reason, name, member_id) ON challenge_entries TO ratings_admin;
GRANT UPDATE (name, member_id) ON challenge_week_places TO ratings_admin;

-- +goose Down
REVOKE UPDATE ON challenge_week_places FROM ratings_admin;
REVOKE UPDATE ON challenge_entries FROM ratings_admin;
