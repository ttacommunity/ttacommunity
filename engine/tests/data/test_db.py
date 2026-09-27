import pytest

from tta_ratings_engine.data.db import get_connection, init_db

ENGINE_TABLES = {
    "players",
    "matches",
    "pairwise_matches",
    "player_ratings",
    "rating_history",
    "walk_forward_calibration",
    "standard_calibration",
}


def tables(conn):
    rows = conn.execute(
        "SELECT name FROM sqlite_master WHERE type = 'table'"
    ).fetchall()
    return {r["name"] for r in rows} - {"sqlite_sequence"}


def columns(conn, table):
    return {r["name"] for r in conn.execute(f"PRAGMA table_info({table})")}


def test_refuses_to_connect_without_a_database_path(monkeypatch):
    monkeypatch.delenv("ENGINE_DB_PATH", raising=False)

    with pytest.raises(RuntimeError, match="ENGINE_DB_PATH"):
        get_connection()


def test_connects_to_the_path_in_the_environment(monkeypatch, tmp_path):
    db = tmp_path / "from-env.db"
    monkeypatch.setenv("ENGINE_DB_PATH", str(db))

    get_connection().close()
    assert db.exists()


def test_creates_only_the_engine_tables(tmp_path):
    db = str(tmp_path / "engine.db")
    init_db(db)
    conn = get_connection(db)

    assert tables(conn) == ENGINE_TABLES
    assert {"glicko_eligible", "replay_code"} <= columns(conn, "matches")
    assert "glicko_eligible" in columns(conn, "pairwise_matches")
    assert "badge_reason" in columns(conn, "players")
    conn.close()


def test_creating_twice_keeps_existing_rows(tmp_path):
    db = str(tmp_path / "engine.db")
    init_db(db)
    conn = get_connection(db)
    conn.execute("INSERT INTO players (name) VALUES ('Alice')")
    conn.commit()

    init_db(db)
    assert conn.execute("SELECT COUNT(*) FROM players").fetchone()[0] == 1
    conn.close()
