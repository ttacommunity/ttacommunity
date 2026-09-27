import psycopg
import pytest

from tta_ratings_engine.data.db import RunNotRunning, connect, require_running


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


@pytest.mark.parametrize("status", ["complete", "failed"])
def test_a_run_that_is_not_running_is_refused(conn, run_id, status):
    conn.execute("UPDATE rating_runs SET status = %s WHERE run_id = %s", (status, run_id))

    with pytest.raises(RunNotRunning, match=f"run {run_id} is {status}"):
        require_running(conn, run_id)


def test_a_missing_run_is_refused(conn, run_id):
    with pytest.raises(RunNotRunning, match="does not exist"):
        require_running(conn, run_id + 1)


def test_a_running_run_is_accepted(conn, run_id):
    require_running(conn, run_id)
