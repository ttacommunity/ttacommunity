-- Games changed in admin: a game added to a group, a game removed, or a game replaced by a new one. A removed or
-- replaced game is kept, in its own state, and never rated; each change keeps the game's earlier state to undo it.
-- +goose Up
ALTER TABLE games DROP CONSTRAINT games_state_check,
    ADD CONSTRAINT games_state_check
        CHECK (state IN ('played', 'never_played', 'unconfirmed', 'running', 'prepared', 'replaced', 'removed'));

-- A game number belongs to one game of a group still in play; a removed or replaced game's can be used again.
CREATE UNIQUE INDEX games_group_number ON games (group_id, game_number) WHERE state NOT IN ('replaced', 'removed');

-- Who made a change is in admin_audit, which no export holds.
CREATE TABLE game_changes (
    change_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    action text NOT NULL CHECK (action IN ('add', 'remove', 'replace')),
    game_id bigint UNIQUE REFERENCES games,
    added_game_id bigint UNIQUE REFERENCES games,
    earlier_state text CHECK (earlier_state NOT IN ('replaced', 'removed')),
    reason text NOT NULL CHECK (btrim(reason) <> '' AND char_length(reason) <= 1000),
    at timestamptz NOT NULL DEFAULT now(),
    CHECK ((game_id IS NULL) = (action = 'add')),
    CHECK ((added_game_id IS NULL) = (action = 'remove')),
    CHECK ((earlier_state IS NULL) = (game_id IS NULL))
);
GRANT SELECT, INSERT ON game_changes TO ratings_admin;
GRANT SELECT ON game_changes TO ratings_backup;

-- +goose StatementBegin
-- undo_game_change undoes a change: the game it added is deleted with its rows in every run, and the game it removed
-- or replaced gets back its earlier state. A game added and since changed again is refused (that change first).
CREATE FUNCTION undo_game_change(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    c game_changes;
    batch bigint;
    grp bigint;
BEGIN
    DELETE FROM game_changes WHERE change_id = id RETURNING * INTO c;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'no game change %', id USING ERRCODE = 'no_data_found';
    END IF;
    IF c.added_game_id IS NOT NULL THEN
        SELECT g.batch_id, g.group_id INTO batch, grp FROM games g WHERE g.game_id = c.added_game_id;
        DELETE FROM tournament_games WHERE match_id = c.added_game_id;
        DELETE FROM pairwise_matches WHERE match_id = c.added_game_id;
        DELETE FROM games WHERE game_id = c.added_game_id;
        DELETE FROM import_batches b WHERE b.batch_id = batch
          AND NOT EXISTS (SELECT 1 FROM games x WHERE x.batch_id = batch) AND NOT EXISTS (SELECT 1 FROM refused_rows r WHERE r.batch_id = batch);
        DELETE FROM group_members gm WHERE gm.group_id = grp
          AND NOT EXISTS (SELECT 1 FROM games x JOIN seats s USING (game_id) WHERE x.group_id = grp AND s.player_id = gm.player_id);
    END IF;
    IF c.game_id IS NOT NULL THEN
        UPDATE games SET state = c.earlier_state WHERE game_id = c.game_id;
    END IF;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION undo_game_change(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION undo_game_change(bigint) TO ratings_admin;

-- +goose Down
-- Refused while a game is removed or replaced: undo those changes in admin first. A game added in admin stays.
DROP FUNCTION undo_game_change(bigint);
DROP TABLE game_changes;
DROP INDEX games_group_number;
ALTER TABLE games DROP CONSTRAINT games_state_check,
    ADD CONSTRAINT games_state_check CHECK (state IN ('played', 'never_played', 'unconfirmed', 'running', 'prepared'));
