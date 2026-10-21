-- The backup job's role, ratings_backup: it reads every table a backup holds (store.BackupTables) and the schema
-- version, and writes nothing. A table added to the backup needs its grant here, in a new migration.
-- +goose Up
-- +goose StatementBegin
DO $$
BEGIN
    -- Roles are per server: another database may have made it, and an owner without CREATEROLE can migrate once
    -- it exists.
    IF NOT EXISTS (SELECT FROM pg_roles WHERE rolname = 'ratings_backup') THEN
        CREATE ROLE ratings_backup NOLOGIN;
    END IF;
END $$;
-- +goose StatementEnd

GRANT SELECT ON
    import_batches, players, player_accounts,
    series, tournaments, stages, groups, group_members, settings,
    games, seats,
    rankings, ranking_series, cge_tournaments, cge_tournament_moderators, cge_standings,
    community_editions, community_points, community_quarters, community_ladders, community_scores, community_standings,
    tournament_career_honours, tournament_cup_podiums, tournament_finish_dates,
    tournament_group_divisions, tournament_ladder_wins, tournament_official_placements,
    tournament_season_corrections, tournament_season_notes,
    tournament_podium_overrides, tournament_regulations, player_title_overrides,
    badges, player_badges, tournament_cards, tournament_stories,
    refused_rows, admins, admin_audit, members, blitz_editions, blitz_entries, blitz_organisers, blitz_thread_texts,
    blitz_rounds, blitz_matches, blitz_skips, accounts, logins, link_requests
TO ratings_backup;

-- +goose StatementBegin
DO $$
BEGIN
    IF to_regclass('public.goose_db_version') IS NOT NULL THEN
        GRANT SELECT ON public.goose_db_version TO ratings_backup;
    END IF;
END $$;
-- +goose StatementEnd

-- +goose Down
REVOKE ALL ON ALL TABLES IN SCHEMA public FROM ratings_backup;
