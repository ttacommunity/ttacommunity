-- Blitz resets: delete_blitz_game deletes one game admin recorded for a blitz edition once no match holds it, with
-- its seats, and the import batch, group members, group and "Round N" stage it leaves empty. The admin login still
-- cannot delete games itself.
-- +goose Up
-- +goose StatementBegin
CREATE FUNCTION delete_blitz_game(id bigint) RETURNS void
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO 'public'
    AS $$
DECLARE
    batch bigint;
    grp bigint;
    stage bigint;
BEGIN
    DELETE FROM games g
    USING import_batches b, groups gr, stages st, blitz_editions e
    WHERE g.game_id = id AND b.batch_id = g.batch_id AND b.source = 'admin'
      AND gr.group_id = g.group_id AND st.stage_id = gr.stage_id AND st.label ~ '^Round [0-9]+$' AND e.tournament_id = st.tournament_id
      AND NOT EXISTS (SELECT 1 FROM blitz_matches m WHERE m.game_id = g.game_id)
    RETURNING g.batch_id, g.group_id, gr.stage_id INTO batch, grp, stage;
    IF NOT FOUND THEN
        RAISE EXCEPTION 'game % is not a blitz game admin recorded, or a match still holds it', id;
    END IF;
    DELETE FROM import_batches b WHERE b.batch_id = batch
      AND NOT EXISTS (SELECT 1 FROM games x WHERE x.batch_id = batch) AND NOT EXISTS (SELECT 1 FROM refused_rows r WHERE r.batch_id = batch);
    DELETE FROM group_members gm WHERE gm.group_id = grp
      AND NOT EXISTS (SELECT 1 FROM games x JOIN seats s USING (game_id) WHERE x.group_id = grp AND s.player_id = gm.player_id);
    DELETE FROM groups gr WHERE gr.group_id = grp AND NOT EXISTS (SELECT 1 FROM games x WHERE x.group_id = grp);
    DELETE FROM stages st WHERE st.stage_id = stage AND NOT EXISTS (SELECT 1 FROM groups x WHERE x.stage_id = stage);
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION delete_blitz_game(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION delete_blitz_game(bigint) TO ratings_admin;

-- +goose Down
DROP FUNCTION delete_blitz_game(bigint);
