-- Challenges: the game's single-player challenges, copied from the committed list by the load job, and the
-- dominations players enter for them. An entry belongs to a member, or keeps the name it was entered under until an
-- admin links it. Who submitted it, why it is hidden and whether its proof was checked are for admins only.
-- +goose Up
CREATE TABLE challenges (
    challenge_id text PRIMARY KEY CHECK (challenge_id ~ '^[a-z0-9_]{1,40}$'),
    position integer NOT NULL CHECK (position >= 0),
    challenge_set text NOT NULL CHECK (challenge_set IN ('original', 'expansion', 'mix')),
    challenge_group text NOT NULL CHECK (challenge_group ~ '^[a-z_]{1,40}$'),
    slot integer NOT NULL CHECK (slot >= 0),
    difficulty integer NOT NULL CHECK (difficulty BETWEEN 1 AND 10),
    players integer NOT NULL CHECK (players BETWEEN 2 AND 4),
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    description text[] NOT NULL DEFAULT '{}',
    rules text[] NOT NULL DEFAULT '{}',
    opponents jsonb NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(opponents) = 'array')
);

CREATE TABLE challenge_entries (
    entry_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    challenge_id text NOT NULL REFERENCES challenges,
    member_id bigint REFERENCES members ON DELETE SET NULL,
    name text NOT NULL CHECK (name <> '' AND char_length(name) <= 100),
    account_id bigint REFERENCES accounts ON DELETE SET NULL,
    domination integer NOT NULL CHECK (domination BETWEEN -9999 AND 9999),
    proof_url text CHECK (proof_url ~ '^https://' AND char_length(proof_url) <= 500),
    created_at timestamptz NOT NULL,
    source text NOT NULL CHECK (source IN ('sheet', 'web')),
    hidden boolean NOT NULL DEFAULT false,
    hidden_reason text NOT NULL DEFAULT '' CHECK (char_length(hidden_reason) <= 500),
    bugged boolean NOT NULL DEFAULT false,
    checked boolean NOT NULL DEFAULT false
);
CREATE INDEX challenge_entries_challenge ON challenge_entries (challenge_id, domination DESC);
CREATE INDEX challenge_entries_member ON challenge_entries (member_id);

GRANT SELECT ON challenges, challenge_entries TO ratings_admin;
-- The site's role never reads who submitted an entry, why it is hidden or whether it was checked.
REVOKE SELECT ON challenge_entries FROM ratings_read;
GRANT SELECT (entry_id, challenge_id, member_id, name, domination, proof_url, created_at, hidden, bugged)
    ON challenge_entries TO ratings_read;
GRANT SELECT ON challenges, challenge_entries TO ratings_backup;

-- +goose Down
DROP TABLE challenge_entries;
DROP TABLE challenges;
