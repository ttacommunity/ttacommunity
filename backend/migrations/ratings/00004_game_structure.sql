-- Games as series → tournament → stage → group → game → seat, with players' CGE accounts and settings that a
-- stage inherits from its tournament and series. The flat games (player1…score4) become one seat per player.
-- +goose Up
ALTER TABLE tournaments RENAME TO series;
ALTER TABLE series RENAME COLUMN tournament_id TO series_id;
ALTER TABLE series RENAME COLUMN season_name TO tournament_noun;
ALTER TABLE series RENAME COLUMN stage_name TO stage_noun;
ALTER TABLE ranking_tournaments RENAME TO ranking_series;
ALTER TABLE ranking_series RENAME COLUMN tournament_id TO series_id;

ALTER TABLE tournament_seasons RENAME TO tournaments;
ALTER TABLE tournaments RENAME COLUMN tournament_id TO series_id;
ALTER TABLE tournaments RENAME COLUMN season_id TO tournament_id;

ALTER TABLE tournament_stages RENAME TO stages;
ALTER TABLE stages RENAME COLUMN season_id TO tournament_id;
ALTER TABLE stages RENAME COLUMN cge_season TO cge_stage_number;

-- A group is keyed by its CGE stage too: one label can be reused by several CGE stages in one of ours.
CREATE TABLE groups (
    group_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    stage_id bigint NOT NULL REFERENCES stages,
    name text NOT NULL DEFAULT '',
    cge_tournament integer CHECK (cge_tournament > 0),
    cge_stage_number integer CHECK (cge_stage_number >= 0),
    cge_group integer UNIQUE CHECK (cge_group > 0),
    CHECK ((cge_tournament IS NULL) = (cge_stage_number IS NULL)),
    UNIQUE NULLS NOT DISTINCT (stage_id, name, cge_tournament, cge_stage_number)
);
CREATE INDEX groups_stage ON groups (stage_id);
-- A game without CGE ids joins its label's CGE stage when the label has exactly one.
CREATE TEMPORARY TABLE game_groups ON COMMIT DROP AS
SELECT m.match_id, m.stage_id, m.group_label,
       coalesce(m.cge_tournament, l.cge_tournament) AS cge_tournament, coalesce(m.cge_season, l.cge_season) AS cge_season
FROM matches m
LEFT JOIN (SELECT stage_id, group_label, min(cge_tournament) AS cge_tournament, min(cge_season) AS cge_season
           FROM matches WHERE cge_tournament IS NOT NULL GROUP BY 1, 2
           HAVING count(DISTINCT (cge_tournament, cge_season)) = 1) l USING (stage_id, group_label);
INSERT INTO groups (stage_id, name, cge_tournament, cge_stage_number)
SELECT DISTINCT stage_id, group_label, cge_tournament, cge_season FROM game_groups;

ALTER TABLE matches RENAME TO games;
ALTER TABLE games RENAME COLUMN match_id TO game_id;
ALTER TABLE games RENAME COLUMN tournament TO name;
ALTER TABLE games RENAME COLUMN replay_code TO share_code;
ALTER TABLE games
    ADD COLUMN group_id bigint REFERENCES groups, -- null until the game is placed
    ADD COLUMN cge_session integer UNIQUE CHECK (cge_session > 0),
    ADD COLUMN started_at timestamptz,
    ADD COLUMN finished_at timestamptz,
    ADD COLUMN speed text,
    ADD COLUMN timer text,
    ADD COLUMN rules text,
    ADD COLUMN card_set text,
    ADD COLUMN cge_players_in_game smallint CHECK (cge_players_in_game > 0),
    ADD CONSTRAINT games_share_code_key UNIQUE (share_code),
    DROP CONSTRAINT matches_state_check,
    -- running and prepared: CGE's games still being played or not yet started, kept, never rated.
    ADD CONSTRAINT games_state_check CHECK (state IN ('played', 'never_played', 'unconfirmed', 'running', 'prepared'));
UPDATE games g SET group_id = gr.group_id
FROM game_groups k JOIN groups gr ON gr.stage_id = k.stage_id AND gr.name = k.group_label
  AND gr.cge_tournament IS NOT DISTINCT FROM k.cge_tournament AND gr.cge_stage_number IS NOT DISTINCT FROM k.cge_season
WHERE k.match_id = g.game_id;
CREATE INDEX games_group ON games (group_id);

-- An account belongs to the player of its latest game; a second account on one player is an alias, linked with
-- its reason (who linked it is in admin_audit, which no export holds).
CREATE TABLE player_accounts (
    cge_user integer PRIMARY KEY CHECK (cge_user > 0),
    player_id bigint NOT NULL REFERENCES players,
    name text NOT NULL,
    banned boolean NOT NULL DEFAULT false,
    deleted boolean NOT NULL DEFAULT false,
    linked_reason text NOT NULL DEFAULT '',
    linked_on date
);
CREATE INDEX player_accounts_player ON player_accounts (player_id);
INSERT INTO player_accounts (cge_user, player_id, name, banned, deleted)
SELECT a.cge_user, p.player_id, a.cge_name, a.banned, a.cge_name ~ '^deleted_user_[0-9]+$'
FROM (SELECT DISTINCT ON (s.cge_user) s.cge_user, s.player_name, s.cge_name,
             bool_or(s.banned) OVER (PARTITION BY s.cge_user) AS banned
      FROM match_seats s JOIN games g ON g.game_id = s.match_id
      ORDER BY s.cge_user, g.date DESC, g.game_id DESC) a
JOIN players p ON p.name = a.player_name;

-- Culture is never negative; the file's score stays as recorded_score, and place keeps today's order.
CREATE TABLE seats (
    game_id bigint NOT NULL REFERENCES games ON DELETE CASCADE,
    seat smallint NOT NULL CHECK (seat BETWEEN 1 AND 4),
    player_id bigint NOT NULL REFERENCES players,
    player_name text NOT NULL,
    cge_user integer REFERENCES player_accounts,
    outcome text CHECK (outcome IN ('finished', 'resigned', 'timed_out')),
    culture double precision NOT NULL CHECK (culture >= 0),
    recorded_score double precision,
    place smallint NOT NULL CHECK (place BETWEEN 1 AND 4),
    starting_position smallint CHECK (starting_position BETWEEN 0 AND 3),
    cge_score double precision,
    cge_standing smallint CHECK (cge_standing > 0),
    PRIMARY KEY (game_id, seat),
    CHECK (outcome IS NULL OR outcome = 'finished' OR culture = 0)
);
CREATE INDEX seats_player_name ON seats (player_name);
CREATE INDEX seats_player ON seats (player_id);
CREATE INDEX seats_account ON seats (cge_user);
INSERT INTO seats (game_id, seat, player_id, player_name, cge_user, outcome, culture, recorded_score, place)
SELECT g.game_id, x.seat, p.player_id, x.name, s.cge_user, s.outcome,
       CASE WHEN s.outcome IN ('resigned', 'timed_out') THEN 0 ELSE greatest(x.score, 0) END,
       x.score,
       rank() OVER (PARTITION BY g.game_id ORDER BY x.score DESC)
FROM games g
CROSS JOIN LATERAL (VALUES (1, g.player1, g.score1), (2, g.player2, g.score2),
                           (3, g.player3, g.score3), (4, g.player4, g.score4)) x (seat, name, score)
JOIN players p ON p.name = x.name
LEFT JOIN match_seats s ON s.match_id = g.game_id AND s.seat = x.seat
WHERE x.name IS NOT NULL;

CREATE TABLE group_members (
    group_id bigint NOT NULL REFERENCES groups ON DELETE CASCADE,
    player_id bigint NOT NULL REFERENCES players,
    PRIMARY KEY (group_id, player_id)
);
INSERT INTO group_members (group_id, player_id)
SELECT DISTINCT g.group_id, s.player_id FROM seats s JOIN games g USING (game_id);

-- Each setting null = inherited (stage ← tournament ← series); an empty array or text = none.
CREATE TABLE settings (
    settings_id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    series_id bigint UNIQUE REFERENCES series ON DELETE CASCADE,
    tournament_id bigint UNIQUE REFERENCES tournaments ON DELETE CASCADE,
    stage_id bigint UNIQUE REFERENCES stages ON DELETE CASCADE,
    timeout_penalty double precision CHECK (timeout_penalty <= 0),
    points_2p double precision[] CHECK (cardinality(points_2p) IN (0, 2) AND 0 <= ALL (points_2p)),
    points_3p double precision[] CHECK (cardinality(points_3p) IN (0, 3) AND 0 <= ALL (points_3p)),
    points_4p double precision[] CHECK (cardinality(points_4p) IN (0, 4) AND 0 <= ALL (points_4p)),
    tiebreak_2p text,
    tiebreak_3p_plus text,
    ignore_ai_players boolean,
    each_plays integer CHECK (each_plays > 0),
    speed text,
    timer text,
    rules text,
    card_set text,
    source text NOT NULL DEFAULT '',
    CHECK (num_nonnulls(series_id, tournament_id, stage_id) = 1)
);
INSERT INTO settings (series_id, points_2p, points_3p, points_4p, source)
SELECT series_id, nullif(points_2p, '{}'), nullif(points_3p, '{}'), nullif(points_4p, '{}'), 'admin'
FROM series WHERE cardinality(points_2p) + cardinality(points_3p) + cardinality(points_4p) > 0;
INSERT INTO settings (tournament_id, timeout_penalty, tiebreak_2p, tiebreak_3p_plus, source)
SELECT t.tournament_id, r.timeout_penalty,
       nullif(split_part(r.tiebreaks, ' / ', 1), ''), nullif(split_part(r.tiebreaks, ' / ', 2), ''), r.source
FROM tournament_season_rules r
JOIN series se ON se.slug = r.series
JOIN tournaments t ON t.series_id = se.series_id AND t.label = r.season::text;
-- +goose StatementBegin
DO $$
BEGIN
    IF (SELECT count(*) FROM tournament_season_rules) <> (SELECT count(*) FROM settings WHERE tournament_id IS NOT NULL) THEN
        RAISE EXCEPTION 'a season rule names a tournament that does not exist';
    END IF;
END $$;
-- +goose StatementEnd

ALTER TABLE cge_tournaments DROP CONSTRAINT cge_tournaments_check;
ALTER TABLE cge_tournaments RENAME COLUMN tournament_id TO series_id;
ALTER TABLE cge_tournaments
    ADD COLUMN tournament_id bigint REFERENCES tournaments ON DELETE SET NULL,
    ADD COLUMN organiser_user integer REFERENCES player_accounts;
-- A catalogued season with no games yet (one still to start) becomes its tournament.
INSERT INTO tournaments (series_id, label)
SELECT DISTINCT c.series_id, c.season FROM cge_tournaments c
WHERE c.season <> '' AND NOT EXISTS (SELECT FROM tournaments t WHERE t.series_id = c.series_id AND t.label = c.season);
UPDATE cge_tournaments c SET tournament_id = t.tournament_id
FROM tournaments t WHERE t.series_id = c.series_id AND t.label = c.season AND c.season <> '';
-- +goose StatementBegin
DO $$
BEGIN
    IF EXISTS (SELECT FROM cge_tournaments WHERE season <> '' AND tournament_id IS NULL) THEN
        RAISE EXCEPTION 'a CGE tournament names a season that does not exist';
    END IF;
END $$;
-- +goose StatementEnd
ALTER TABLE cge_tournaments DROP COLUMN season, ADD CHECK (tournament_id IS NULL OR series_id IS NOT NULL);

CREATE TABLE cge_tournament_moderators (
    cge_tournament integer NOT NULL REFERENCES cge_tournaments ON DELETE CASCADE,
    cge_user integer NOT NULL REFERENCES player_accounts,
    PRIMARY KEY (cge_tournament, cge_user)
);

DROP TABLE match_seats;
DROP TABLE tournament_season_rules;
ALTER TABLE games
    DROP COLUMN player_count, DROP COLUMN player1, DROP COLUMN score1, DROP COLUMN player2, DROP COLUMN score2,
    DROP COLUMN player3, DROP COLUMN score3, DROP COLUMN player4, DROP COLUMN score4,
    DROP COLUMN stage_id, DROP COLUMN group_label, DROP COLUMN cge_tournament, DROP COLUMN cge_season;
ALTER TABLE series DROP COLUMN points_2p, DROP COLUMN points_3p, DROP COLUMN points_4p;

-- One row per game with its place in the catalogue and its seats in seat order: what most readers list.
-- Seats are gathered per game, so a filter on games still uses the indexes. scores are the source's own (a CGE
-- game has none: culture, or -place unfinished, which keeps the order), what the engine reads today.
CREATE VIEW game_lines AS
SELECT g.game_id, g.name, g.date, g.state, g.share_code, g.glicko_eligible, g.game_number, g.cge_finished,
       g.batch_id, g.group_id, gr.name AS group_name, gr.stage_id, st.tournament_id, t.series_id,
       gr.cge_tournament, gr.cge_stage_number, s.player_count, s.players, s.cultures, s.places, s.outcomes, s.scores
FROM games g
LEFT JOIN groups gr USING (group_id)
LEFT JOIN stages st USING (stage_id)
LEFT JOIN tournaments t USING (tournament_id)
CROSS JOIN LATERAL (
    SELECT count(*)::smallint AS player_count,
           array_agg(player_name ORDER BY seat) AS players, array_agg(culture ORDER BY seat) AS cultures,
           array_agg(place ORDER BY seat) AS places, array_agg(outcome ORDER BY seat) AS outcomes,
           array_agg(coalesce(recorded_score, CASE WHEN outcome = 'finished' THEN culture ELSE -place END) ORDER BY seat) AS scores
    FROM seats WHERE seats.game_id = g.game_id) s;

GRANT SELECT ON groups, group_members, seats, player_accounts, settings, cge_tournament_moderators, game_lines TO ratings_read;
GRANT SELECT ON groups, group_members, seats, player_accounts, settings, cge_tournament_moderators, game_lines TO ratings_admin;
GRANT INSERT, UPDATE, DELETE ON settings TO ratings_admin;

-- +goose Down
DELETE FROM games WHERE state IN ('running', 'prepared');
DROP VIEW game_lines;
REVOKE INSERT, UPDATE, DELETE ON settings FROM ratings_admin;

ALTER TABLE series
    ADD COLUMN points_2p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_2p) IN (0, 2) AND 0 <= ALL (points_2p)),
    ADD COLUMN points_3p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_3p) IN (0, 3) AND 0 <= ALL (points_3p)),
    ADD COLUMN points_4p double precision[] NOT NULL DEFAULT '{}' CHECK (cardinality(points_4p) IN (0, 4) AND 0 <= ALL (points_4p));
UPDATE series se SET points_2p = coalesce(s.points_2p, '{}'), points_3p = coalesce(s.points_3p, '{}'),
                     points_4p = coalesce(s.points_4p, '{}')
FROM settings s WHERE s.series_id = se.series_id;
GRANT UPDATE (points_2p, points_3p, points_4p) ON series TO ratings_admin;

CREATE TABLE tournament_season_rules (
    series text NOT NULL,
    season integer NOT NULL CHECK (season > 0),
    timeout_penalty double precision NOT NULL CHECK (timeout_penalty <= 0),
    scoring_3p text DEFAULT '' NOT NULL,
    scoring_4p text DEFAULT '' NOT NULL,
    tiebreaks text DEFAULT '' NOT NULL,
    source text NOT NULL,
    PRIMARY KEY (series, season)
);
INSERT INTO tournament_season_rules (series, season, timeout_penalty, scoring_3p, scoring_4p, tiebreaks, source)
SELECT se.slug, t.label::integer, s.timeout_penalty,
       array_to_string(se.points_3p, '-'), array_to_string(se.points_4p, '-'),
       CASE WHEN s.tiebreak_2p IS NULL THEN '' ELSE s.tiebreak_2p || ' / ' || coalesce(s.tiebreak_3p_plus, '') END,
       s.source
FROM settings s JOIN tournaments t ON t.tournament_id = s.tournament_id JOIN series se ON se.series_id = t.series_id
WHERE s.timeout_penalty IS NOT NULL AND t.label ~ '^[0-9]+$';
GRANT SELECT ON tournament_season_rules TO ratings_read, ratings_admin;
GRANT INSERT, UPDATE (timeout_penalty, source) ON tournament_season_rules TO ratings_admin;

ALTER TABLE games
    ADD COLUMN player_count smallint, ADD COLUMN player1 text, ADD COLUMN score1 double precision,
    ADD COLUMN player2 text, ADD COLUMN score2 double precision, ADD COLUMN player3 text, ADD COLUMN score3 double precision,
    ADD COLUMN player4 text, ADD COLUMN score4 double precision, ADD COLUMN stage_id bigint REFERENCES stages,
    ADD COLUMN group_label text NOT NULL DEFAULT '', ADD COLUMN cge_tournament integer CHECK (cge_tournament > 0),
    ADD COLUMN cge_season integer CHECK (cge_season >= 0);
UPDATE games g SET
    player_count = x.n, player1 = x.p[1], score1 = x.s[1], player2 = x.p[2], score2 = x.s[2],
    player3 = x.p[3], score3 = x.s[3], player4 = x.p[4], score4 = x.s[4]
FROM (SELECT game_id, count(*) AS n, array_agg(player_name ORDER BY seat) AS p,
             array_agg(coalesce(recorded_score, culture) ORDER BY seat) AS s
      FROM seats GROUP BY game_id) x
WHERE x.game_id = g.game_id;
UPDATE games g SET stage_id = gr.stage_id, group_label = gr.name, cge_tournament = gr.cge_tournament,
                   cge_season = gr.cge_stage_number
FROM groups gr WHERE gr.group_id = g.group_id;
ALTER TABLE games
    ALTER COLUMN player_count SET NOT NULL, ALTER COLUMN player1 SET NOT NULL, ALTER COLUMN score1 SET NOT NULL,
    ALTER COLUMN player2 SET NOT NULL, ALTER COLUMN score2 SET NOT NULL,
    ADD CONSTRAINT matches_player_count_check CHECK (player_count BETWEEN 2 AND 4);
CREATE INDEX matches_player1 ON games (player1, date);
CREATE INDEX matches_player2 ON games (player2, date);
CREATE INDEX matches_player3 ON games (player3, date);
CREATE INDEX matches_player4 ON games (player4, date);
CREATE INDEX matches_stage ON games (stage_id);

CREATE TABLE match_seats (
    match_id bigint NOT NULL REFERENCES games ON DELETE CASCADE,
    seat smallint NOT NULL CHECK (seat BETWEEN 1 AND 4),
    player_name text NOT NULL,
    outcome text NOT NULL CHECK (outcome IN ('finished', 'resigned', 'timed_out')),
    cge_user integer CHECK (cge_user > 0),
    banned boolean NOT NULL DEFAULT false,
    cge_name text,
    PRIMARY KEY (match_id, seat)
);
INSERT INTO match_seats (match_id, seat, player_name, outcome, cge_user, banned, cge_name)
SELECT s.game_id, s.seat, s.player_name, s.outcome, s.cge_user, a.banned, a.name
FROM seats s JOIN player_accounts a USING (cge_user) WHERE s.outcome IS NOT NULL;
GRANT SELECT ON match_seats TO ratings_read, ratings_admin;

ALTER TABLE cge_tournaments ADD COLUMN season text NOT NULL DEFAULT '';
UPDATE cge_tournaments c SET season = t.label FROM tournaments t WHERE t.tournament_id = c.tournament_id;
ALTER TABLE cge_tournaments DROP CONSTRAINT cge_tournaments_check;
ALTER TABLE cge_tournaments DROP COLUMN tournament_id, DROP COLUMN organiser_user;
ALTER TABLE cge_tournaments RENAME COLUMN series_id TO tournament_id;
ALTER TABLE cge_tournaments ADD CONSTRAINT cge_tournaments_check CHECK (tournament_id IS NOT NULL OR season = '');

DROP TABLE cge_tournament_moderators, settings, group_members, seats, player_accounts;
ALTER TABLE games DROP COLUMN group_id, DROP COLUMN cge_session, DROP COLUMN started_at, DROP COLUMN finished_at,
    DROP COLUMN speed, DROP COLUMN timer, DROP COLUMN rules, DROP COLUMN card_set, DROP COLUMN cge_players_in_game,
    DROP CONSTRAINT games_share_code_key;
DROP TABLE groups;
ALTER TABLE games DROP CONSTRAINT games_state_check,
    ADD CONSTRAINT matches_state_check CHECK (state IN ('played', 'never_played', 'unconfirmed'));
ALTER TABLE games RENAME COLUMN share_code TO replay_code;
ALTER TABLE games RENAME COLUMN name TO tournament;
ALTER TABLE games RENAME COLUMN game_id TO match_id;
ALTER TABLE games RENAME TO matches;

ALTER TABLE stages RENAME COLUMN cge_stage_number TO cge_season;
ALTER TABLE stages RENAME COLUMN tournament_id TO season_id;
ALTER TABLE stages RENAME TO tournament_stages;

ALTER TABLE tournaments RENAME COLUMN tournament_id TO season_id;
ALTER TABLE tournaments RENAME COLUMN series_id TO tournament_id;
ALTER TABLE tournaments RENAME TO tournament_seasons;

ALTER TABLE ranking_series RENAME COLUMN series_id TO tournament_id;
ALTER TABLE ranking_series RENAME TO ranking_tournaments;
ALTER TABLE series RENAME COLUMN stage_noun TO stage_name;
ALTER TABLE series RENAME COLUMN tournament_noun TO season_name;
ALTER TABLE series RENAME COLUMN series_id TO tournament_id;
ALTER TABLE series RENAME TO tournaments;
