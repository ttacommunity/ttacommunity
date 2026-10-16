-- A blitz entrant is in (active) or dropped: no longer pending. A pending entrant in no match is deleted (one who
-- does not confirm is removed before round 1), and one already paired is in; so this never fails.
-- +goose Up
DELETE FROM blitz_entries x
WHERE x.status = 'pending' AND NOT EXISTS (SELECT 1 FROM blitz_matches m WHERE x.entry_id IN (m.entry_a, m.entry_b));
UPDATE blitz_entries SET status = 'active' WHERE status = 'pending';
ALTER TABLE blitz_entries DROP CONSTRAINT blitz_entries_status_check;
ALTER TABLE blitz_entries ADD CONSTRAINT blitz_entries_status_check CHECK (status IN ('active', 'dropped'));

-- +goose Down
-- The pending entrants deleted on the way up are not restored.
ALTER TABLE blitz_entries DROP CONSTRAINT blitz_entries_status_check;
ALTER TABLE blitz_entries ADD CONSTRAINT blitz_entries_status_check CHECK (status IN ('pending', 'active', 'dropped'));
