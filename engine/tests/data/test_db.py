import psycopg
import pytest

from tta_ratings_engine.data.db import connect


def test_refuses_to_connect_without_a_database_url(monkeypatch):
    monkeypatch.delenv("RATINGS_DATABASE_URL", raising=False)

    with pytest.raises(RuntimeError, match="RATINGS_DATABASE_URL"):
        connect()


def test_connects_to_the_url_in_the_environment(monkeypatch, db_url):
    monkeypatch.setenv("RATINGS_DATABASE_URL", db_url)

    with connect() as c:
        assert c.execute("SELECT current_database() AS db").fetchone()["db"] in db_url


def test_an_unreachable_database_is_an_error_not_a_default(monkeypatch):
    monkeypatch.setenv(
        "RATINGS_DATABASE_URL", "postgres://nobody@127.0.0.1:1/none?connect_timeout=2"
    )

    with pytest.raises(psycopg.OperationalError):
        connect()


def test_rows_come_back_by_column_name(conn):
    assert conn.execute("SELECT 1 AS one").fetchone() == {"one": 1}


def test_the_test_database_has_the_migrated_schema(conn):
    tables = {
        r["table_name"]
        for r in conn.execute(
            "SELECT table_name FROM information_schema.tables WHERE table_schema = 'public'"
        )
    }

    assert {"matches", "players", "rating_runs", "pairwise_matches"} <= tables
