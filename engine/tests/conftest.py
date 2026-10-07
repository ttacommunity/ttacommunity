"""Test databases: one migrated template per session, and a fresh copy per test."""

import os
import secrets
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


@pytest.fixture(scope="session")
def template_db():
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
                "INSERT INTO series (slug, name, top_divisions) VALUES "
                "('international', 'International', '{Grandmaster,Diamond}'), "
                "('intermezzo', 'Intermezzo', '{Grandmaster,Diamond}'), "
                "('royal_league', 'Royal League', '{Emperor}'), "
                "('worlds', 'World Championship', '{Final}')"
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


@pytest.fixture
def db_url(template_db):
    name = "engine_test_" + secrets.token_hex(6)
    create_database(name, template=template_db)
    yield url_for(name)
    drop_database(name)


@pytest.fixture
def conn(db_url):
    c = connect(db_url)
    yield c
    c.close()


def group_in(conn, slug, group=""):
    """Returns group of test series slug's tournament 1, creating them; "rated" is in the community ranking."""
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
        "INSERT INTO stages (tournament_id, label) VALUES (%s, '') "
        "ON CONFLICT (tournament_id, label) DO UPDATE SET label = excluded.label RETURNING stage_id",
        (t,),
    ).fetchone()["stage_id"]
    return conn.execute(
        "INSERT INTO groups (stage_id, name) VALUES (%s, %s) "
        "ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number) DO UPDATE SET name = excluded.name "
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
        tournament="T",
        eligible=True,
        placed="rated",
        state="played",
        group="",
    ):
        match_id = conn.execute(
            "INSERT INTO games (batch_id, source_file, source_line, name, date, glicko_eligible, group_id, state) "
            "VALUES (%s, 'test.csv', %s, %s, %s, %s, %s, %s) RETURNING game_id",
            (
                batch,
                next(line),
                tournament,
                day,
                eligible,
                group_in(conn, placed, group) if placed else None,
                state,
            ),
        ).fetchone()["game_id"]
        for name, _ in players:
            conn.execute(
                "INSERT INTO players (name) VALUES (%s) ON CONFLICT DO NOTHING", (name,)
            )
        conn.execute(
            "INSERT INTO seats (game_id, seat, player_id, player_name, culture, recorded_score, place) "
            "SELECT %s, s.seat, p.player_id, s.name, greatest(s.score, 0), s.score, "
            "rank() OVER (ORDER BY s.score DESC) "
            "FROM unnest(%s::text[], %s::float8[]) WITH ORDINALITY AS s(name, score, seat) JOIN players p USING (name)",
            (match_id, [n for n, _ in players], [sc for _, sc in players]),
        )
        conn.commit()
        return match_id

    return add


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
                "INSERT INTO tournament_official_placements "
                "VALUES (%s, %s, %s, %s, %s, %s, 'test', 'sheet')",
                (series, season, division, p, i, points[i - 1] if points else None),
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
            "ON CONFLICT (stage_id, name, cge_tournament, cge_stage_number) DO UPDATE SET name = excluded.name "
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
