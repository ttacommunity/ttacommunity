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
    text = path.read_text()
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


def stage_in(conn, slug):
    """Returns the stage of test tournament slug, creating it; "rated" is in the community ranking."""
    t = conn.execute(
        "INSERT INTO tournaments (slug, name) VALUES (%s, %s) ON CONFLICT (slug) DO UPDATE SET slug = excluded.slug "
        "RETURNING tournament_id",
        (slug, slug),
    ).fetchone()["tournament_id"]
    if slug == "rated":
        conn.execute(
            "INSERT INTO ranking_tournaments (ranking_id, tournament_id) VALUES (community_ranking(), %s) "
            "ON CONFLICT DO NOTHING",
            (t,),
        )
    s = conn.execute(
        "INSERT INTO tournament_seasons (tournament_id, label) VALUES (%s, '1') "
        "ON CONFLICT (tournament_id, label) DO UPDATE SET label = excluded.label RETURNING season_id",
        (t,),
    ).fetchone()["season_id"]
    return conn.execute(
        "INSERT INTO tournament_stages (season_id, label) VALUES (%s, '') "
        "ON CONFLICT (season_id, label) DO UPDATE SET label = excluded.label RETURNING stage_id",
        (s,),
    ).fetchone()["stage_id"]


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
    ):
        cols = {
            "batch_id": batch,
            "source_file": "test.csv",
            "source_line": next(line),
            "tournament": tournament,
            "date": day,
            "player_count": len(players),
            "glicko_eligible": eligible,
            "stage_id": stage_in(conn, placed) if placed else None,
            "state": state,
        }
        for i, (name, score) in enumerate(players, start=1):
            cols[f"player{i}"] = name
            cols[f"score{i}"] = score
            conn.execute(
                "INSERT INTO players (name) VALUES (%s) ON CONFLICT DO NOTHING",
                (name,),
            )
        stmt = sql.SQL(
            "INSERT INTO matches ({}) VALUES ({}) RETURNING match_id"
        ).format(
            sql.SQL(", ").join(map(sql.Identifier, cols)),
            sql.SQL(", ").join(sql.Placeholder() * len(cols)),
        )
        match_id = conn.execute(stmt, list(cols.values())).fetchone()["match_id"]
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
