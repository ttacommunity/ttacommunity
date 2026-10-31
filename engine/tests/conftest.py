"""Test databases: one migrated template per session, and a fresh copy per test."""

import os
import secrets
from collections import deque
from concurrent.futures import ThreadPoolExecutor
from datetime import date
from pathlib import Path

import psycopg
import pytest
from psycopg import conninfo, sql

from tta_ratings_engine.data.db import connect

MIGRATIONS = Path(__file__).resolve().parents[2] / "backend" / "migrations" / "ratings"


def up_sql(path):
    """Returns the goose Up section of a migration file."""
    text = path.read_text(encoding="utf-8")
    return text.split("-- +goose Up", 1)[1].split("-- +goose Down", 1)[0]


def server_url():
    url = os.environ.get("TEST_DATABASE_URL")
    if not url:
        pytest.fail("TEST_DATABASE_URL is not set: the engine tests need a database")
    return url


def url_for(name):
    return conninfo.make_conninfo(server_url(), dbname=name)


def create_database(name, template=None):
    with psycopg.connect(server_url(), autocommit=True) as admin:
        stmt = sql.SQL("CREATE DATABASE {}").format(sql.Identifier(name))
        if template:
            stmt += sql.SQL(" TEMPLATE {}").format(sql.Identifier(template))
        admin.execute(stmt)


def drop_database(name):
    with psycopg.connect(server_url(), autocommit=True) as admin:
        admin.execute(
            sql.SQL("DROP DATABASE {} WITH (FORCE)").format(sql.Identifier(name))
        )


def skip_flushes():
    """Turns off the server's flushes to disk, which make each test's CREATE and DROP DATABASE slow.

    Only for a server thrown away after the run, as CI's is: a crash can corrupt it
    (https://www.postgresql.org/docs/current/non-durability.html).
    """
    with psycopg.connect(server_url(), autocommit=True) as admin:
        admin.execute("ALTER SYSTEM SET fsync = off")
        admin.execute("ALTER SYSTEM SET full_page_writes = off")
        admin.execute("SELECT pg_reload_conf()")


@pytest.fixture(scope="session")
def template_db():
    if os.environ.get("TEST_DATABASE_THROWAWAY") == "1":
        skip_flushes()
    name = "engine_template_" + secrets.token_hex(6)
    create_database(name)
    try:
        files = sorted(MIGRATIONS.glob("*.sql"))
        assert files, f"no migrations in {MIGRATIONS}"
        with psycopg.connect(url_for(name)) as c:
            for f in files:
                c.execute(up_sql(f))
            # The series the engine derives, with the settings the load job gives them when first seen.
            c.execute(
                "INSERT INTO series (slug, name, top_divisions, results_rules, podium_source) VALUES "
                "('international', 'International', '{Grandmaster,Diamond}', 'international', 'top_divisions'), "
                "('intermezzo', 'Intermezzo', '{Grandmaster,Diamond}', 'intermezzo', 'top_divisions'), "
                "('royal_league', 'Royal League', '{Emperor}', 'royal_league', 'top_divisions'), "
                "('worlds', 'World Championship', '{Final}', 'worlds', 'recorded')"
            )
            c.execute(
                "UPDATE series SET divisions = '{Grandmaster,Diamond,Master,Platinum,Gold,Silver,Bronze,Wood}', "
                "division_tiers = '{1,1,2,3,4,5,6,7}', super_gm = true, record_min_games = 6 "
                "WHERE slug IN ('international', 'intermezzo')"
            )
            c.execute(
                "UPDATE series SET divisions = '{Emperor,King,Prince,Duke,Marquess,Count,Viscount,Baron,Knight}', "
                "division_tiers = '{1,2,3,4,5,6,7,7,7}', super_gm = true, record_min_games = 5 "
                "WHERE slug = 'royal_league'"
            )
            c.execute(
                "INSERT INTO settings (series_id, points_2p, points_3p, points_4p) "
                "SELECT series_id, p2, p3, p4 FROM series JOIN (VALUES "
                "('international', NULL, '{5,2,0}'::float8[], '{6,3,1,0}'::float8[]), "
                "('intermezzo', NULL, '{5,2,0}', NULL), "
                "('royal_league', '{2,0}'::float8[], NULL, NULL), "
                "('worlds', '{4,0}', '{5,2,0}', '{6,3,1,0}')) AS v(slug, p2, p3, p4) USING (slug)"
            )
        yield name
    finally:
        drop_database(name)


class Copies:
    """Fresh copies of the template, made two ahead and dropped after use by two background threads.

    Tests run one at a time, so the copies a test needs are made while the tests before it run.
    """

    def __init__(self, template):
        self.template = template
        self.worker = ThreadPoolExecutor(max_workers=2)
        self.drops = []
        self.ready = deque(self.worker.submit(self.copy) for _ in range(2))

    def copy(self):
        name = "engine_test_" + secrets.token_hex(6)
        create_database(name, template=self.template)
        return name

    def take(self):
        """Returns a fresh copy's name, and starts making the next."""
        name = self.ready.popleft().result()
        self.ready.append(self.worker.submit(self.copy))
        return name

    def drop_later(self, name):
        self.drops.append(self.worker.submit(drop_database, name))

    def close(self):
        """Drops the spare copy and waits for every drop, raising the first that failed."""
        for spare in self.ready:
            self.drop_later(spare.result())
        self.worker.shutdown(wait=True)
        for done in self.drops:
            done.result()


@pytest.fixture(scope="session")
def copies(template_db):
    c = Copies(template_db)
    yield c
    c.close()


@pytest.fixture
def db_url(copies):
    name = copies.take()
    yield url_for(name)
    copies.drop_later(name)


@pytest.fixture
def conn(db_url):
    c = connect(db_url)
    yield c
    c.close()


def group_in(conn, slug, group="", stage=""):
    """Returns group of stage of test series slug's tournament 1, creating them; "rated" is in the community ranking."""
    se = conn.execute(
        "INSERT INTO series (slug, name) VALUES (%s, %s) ON CONFLICT (slug) DO UPDATE SET slug = excluded.slug "
        "RETURNING series_id",
        (slug, slug),
    ).fetchone()["series_id"]
    if slug == "rated":
        conn.execute(
            "INSERT INTO ranking_series (ranking_id, series_id) VALUES (community_ranking(), %s) "
            "ON CONFLICT DO NOTHING",
            (se,),
        )
    t = conn.execute(
        "INSERT INTO tournaments (series_id, label) VALUES (%s, '1') "
        "ON CONFLICT (series_id, label) DO UPDATE SET label = excluded.label RETURNING tournament_id",
        (se,),
    ).fetchone()["tournament_id"]
    st = conn.execute(
        "INSERT INTO stages (tournament_id, label) VALUES (%s, %s) "
        "ON CONFLICT (tournament_id, label) DO UPDATE SET label = excluded.label RETURNING stage_id",
        (t, stage),
    ).fetchone()["stage_id"]
    return conn.execute(
        "INSERT INTO groups (stage_id, name) VALUES (%s, %s) "
        "ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number, cge_group) DO UPDATE SET name = excluded.name "
        "RETURNING group_id",
        (st, group),
    ).fetchone()["group_id"]


@pytest.fixture
def add_match(conn):
    """Returns a function that loads one match, as the load job would, placed
    in test tournament `placed` ("rated" by default; None for no tournament)."""
    batch = conn.execute(
        "INSERT INTO import_batches (source) VALUES ('load') RETURNING batch_id"
    ).fetchone()["batch_id"]
    line = iter(range(2, 1_000_000))

    def add(
        *players,
        day=date(2024, 1, 1),
        finished_at=None,
        tournament="T",
        eligible=True,
        placed="rated",
        state="played",
        group="",
        stage="",
    ):
        match_id = conn.execute(
            "INSERT INTO games (batch_id, source_file, source_line, name, date, finished_at, glicko_eligible, group_id, state) "
            "VALUES (%s, 'test.csv', %s, %s, %s, %s, %s, %s, %s) RETURNING game_id",
            (
                batch,
                next(line),
                tournament,
                day,
                finished_at,
                eligible,
                group_in(conn, placed, group, stage) if placed else None,
                state,
            ),
        ).fetchone()["game_id"]
        for name, _ in players:
            conn.execute(
                "INSERT INTO players (name) VALUES (%s) ON CONFLICT DO NOTHING", (name,)
            )
        # Outcomes and places as the load job reads a file's scores: -1 and -5 timed out, other negatives resigned.
        conn.execute(
            "WITH s AS (SELECT s.*, CASE WHEN s.score >= 0 THEN 'finished' WHEN s.score IN (-1, -5) THEN 'timed_out' "
            "ELSE 'resigned' END AS outcome FROM unnest(%s::text[], %s::float8[]) WITH ORDINALITY AS s(name, score, seat)) "
            "INSERT INTO seats (game_id, seat, player_id, culture, recorded_score, outcome, place) "
            "SELECT %s, s.seat, p.player_id, greatest(s.score, 0), s.score, s.outcome, "
            "CASE WHEN s.outcome = 'timed_out' THEN 1 + count(*) FILTER (WHERE s.outcome <> 'timed_out') OVER () "
            "ELSE rank() OVER (ORDER BY CASE s.outcome WHEN 'finished' THEN 0 WHEN 'resigned' THEN 1 ELSE 2 END, s.score DESC) END "
            "FROM s JOIN players p USING (name)",
            ([n for n, _ in players], [sc for _, sc in players], match_id),
        )
        conn.commit()
        return match_id

    return add


@pytest.fixture
def name_of(conn):
    """Returns a function that gives a player's name by their id."""
    return lambda player_id: conn.execute(
        "SELECT name FROM players WHERE player_id = %s", (player_id,)
    ).fetchone()["name"]


@pytest.fixture
def run_id(conn):
    """A running rating run to write results into."""
    rid = conn.execute(
        "INSERT INTO rating_runs (engine_version, input_fingerprint) "
        "VALUES ('test', 'test') RETURNING run_id"
    ).fetchone()["run_id"]
    conn.commit()
    return rid


@pytest.fixture
def place(conn):
    """Returns a function that records official placements for a season's division."""

    def add(series, season, division, *players, points=None):
        for i, p in enumerate(players, start=1):
            conn.execute(
                "INSERT INTO players (name) VALUES (%s) ON CONFLICT DO NOTHING", (p,)
            )
            conn.execute(
                "INSERT INTO tournament_official_placements (series, season, division, player_id, place, points, "
                "reason, source) SELECT %s, %s, %s, player_id, %s, %s, 'test', 'sheet' FROM players WHERE name = %s",
                (series, season, division, i, points[i - 1] if points else None, p),
            )
        conn.commit()

    return add


@pytest.fixture
def move_last_game(conn):
    """Returns a function that moves the newest game into a group of series slug's stage (that stage of that
    tournament label, where named, else its only one), creating the group, and optionally sets CGE's finish date."""

    def move(slug, group="", label=None, stage=None, cge_finished=None):
        conn.execute(
            "WITH st AS (SELECT x.stage_id FROM stages x JOIN tournaments s USING (tournament_id) "
            "JOIN series t USING (series_id) WHERE t.slug = %(slug)s AND (%(label)s::text IS NULL OR s.label = %(label)s) "
            "AND (%(stage)s::text IS NULL OR x.label = %(stage)s)), "
            "gr AS (INSERT INTO groups (stage_id, name) SELECT stage_id, %(group)s FROM st "
            "ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number, cge_group) DO UPDATE SET name = excluded.name "
            "RETURNING group_id) "
            "UPDATE games SET group_id = (SELECT group_id FROM gr), cge_finished = coalesce(%(finished)s, cge_finished) "
            "WHERE game_id = (SELECT max(game_id) FROM games)",
            {
                "slug": slug,
                "group": group,
                "label": label,
                "stage": stage,
                "finished": cge_finished,
            },
        )

    return move
