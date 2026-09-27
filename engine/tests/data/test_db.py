from datetime import date

import psycopg
import pytest

from tta_ratings_engine.data.db import (
    RunNotRunning,
    connect,
    load_pairs,
    require_running,
)
from tta_ratings_engine.data.pairwise import expand_pairwise


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
    conn.execute(
        "UPDATE rating_runs SET status = %s WHERE run_id = %s", (status, run_id)
    )

    with pytest.raises(RunNotRunning, match=f"run {run_id} is {status}"):
        require_running(conn, run_id)


def test_a_missing_run_is_refused(conn, run_id):
    with pytest.raises(RunNotRunning, match="does not exist"):
        require_running(conn, run_id + 1)


def test_a_running_run_is_accepted(conn, run_id):
    require_running(conn, run_id)


def test_pairs_are_loaded_in_date_then_match_order_with_text_dates(
    conn, add_match, run_id
):
    add_match(("A", 1), ("B", 2), day=date(2024, 2, 1))
    add_match(("C", 1), ("D", 2), day=date(2024, 1, 1))
    add_match(("E", 1), ("F", 2), ("G", 3), day=date(2024, 1, 1))
    expand_pairwise(conn, run_id)

    rows = load_pairs(conn, run_id, 0)

    assert [(r["date"], r["player_a"]) for r in rows] == [
        ("2024-01-01", "C"),
        ("2024-01-01", "E"),
        ("2024-01-01", "E"),
        ("2024-01-01", "F"),
        ("2024-02-01", "A"),
    ]
    assert set(rows[0]) == {
        "match_id",
        "date",
        "tournament",
        "player_a",
        "player_b",
        "outcome_a",
        "weight",
        "player_count",
    }


def test_loading_a_format_keeps_only_its_pairs(conn, add_match, run_id):
    add_match(("A", 1), ("B", 2))
    add_match(("E", 1), ("F", 2), ("G", 3))
    expand_pairwise(conn, run_id)

    assert {r["player_count"] for r in load_pairs(conn, run_id, 3)} == {3}
    assert len(load_pairs(conn, run_id, 2)) == 1


def test_loading_leaves_out_ineligible_pairs_and_other_runs(conn, add_match, run_id):
    add_match(("A", 1), ("B", 2), eligible=False)
    expand_pairwise(conn, run_id)
    add_match(("C", 1), ("D", 2))

    assert load_pairs(conn, run_id, 0) == []
