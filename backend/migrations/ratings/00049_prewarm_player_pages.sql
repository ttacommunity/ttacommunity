-- prewarm_player_pages(run) loads what the player pages read for a run into shared buffers: the run's partitions
-- of the tables and indexes they read, and the small input tables beside them. Called when a run is published, so
-- the first visitors to the new run's player pages do not wait on the disk. Returns the blocks loaded: 0 where the
-- pg_prewarm extension is missing, which only a superuser can create.
-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    IF (SELECT rolsuper FROM pg_roles WHERE rolname = current_user) THEN
        CREATE EXTENSION IF NOT EXISTS pg_prewarm;
    END IF;
END
$$;
-- +goose StatementEnd

-- +goose StatementBegin
CREATE FUNCTION prewarm_player_pages(id bigint) RETURNS bigint
    LANGUAGE plpgsql SECURITY DEFINER
    SET search_path TO public, pg_temp
    AS $$
DECLARE
    rel regclass;
    blocks bigint := 0;
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_extension WHERE extname = 'pg_prewarm') THEN
        RETURN 0;
    END IF;
    FOR rel IN
        SELECT c.oid::regclass FROM pg_inherits i
        JOIN pg_class c ON c.oid = i.inhrelid
        LEFT JOIN pg_index x ON x.indexrelid = c.oid
        JOIN pg_class t ON t.oid = coalesce(x.indrelid, c.oid)
        WHERE i.inhparent = ANY (ARRAY[
                'pairwise_matches_player_a_games', 'pairwise_matches_player_b_games', 'rating_history_player',
                'tournament_records', 'tournament_records_player_lower', 'player_ratings', 'player_ratings_pkey',
                'player_seat_stats', 'player_seat_stats_pkey', 'tournament_achievements', 'player_titles',
                'player_titles_pkey', 'player_peak_titles', 'player_peak_titles_pkey', 'player_yearly_titles',
                'player_yearly_titles_pkey']::regclass[])
          AND pg_get_expr(t.relpartbound, t.oid) = format('FOR VALUES IN (%L)', id)
        UNION ALL
        SELECT unnest(ARRAY[
            'rating_runs', 'players', 'players_lower_name', 'players_name_key', 'player_accounts',
            'player_accounts_player', 'members', 'members_player_id_key', 'matches_pkey', 'seats_pkey', 'groups',
            'groups_pkey', 'stages', 'tournament_stages_pkey', 'tournaments', 'tournament_seasons_pkey', 'settings',
            'settings_stage_id_key', 'settings_tournament_id_key', 'settings_series_id_key']::regclass[])
    LOOP
        blocks := blocks + pg_prewarm(rel);
    END LOOP;
    RETURN blocks;
END
$$;
-- +goose StatementEnd
REVOKE ALL ON FUNCTION prewarm_player_pages(bigint) FROM PUBLIC;
GRANT EXECUTE ON FUNCTION prewarm_player_pages(bigint) TO ratings_admin;

-- +goose Down
DROP FUNCTION prewarm_player_pages(bigint);
-- +goose StatementBegin
DO $$
BEGIN
    IF (SELECT rolsuper FROM pg_roles WHERE rolname = current_user) THEN
        DROP EXTENSION IF EXISTS pg_prewarm;
    END IF;
END
$$;
-- +goose StatementEnd
