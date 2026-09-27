from pathlib import Path

import tta_ratings_engine
from tta_ratings_engine.data.db import get_connection, init_db
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.evaluation.standard_eval import (
    run_standard_calibration_all,
)

ENGINE_DIR = Path(tta_ratings_engine.__file__).resolve().parents[2]


def seed(db):
    init_db(db)
    conn = get_connection(db)
    with conn:
        for i, (a, b) in enumerate(
            [("Alice", "Bob"), ("Bob", "Carol"), ("Alice", "Carol")], start=1
        ):
            conn.execute(
                "INSERT INTO matches (match_id, tournament, date, player_count, player1, score1, player2, score2) "
                "VALUES (?, 'T', ?, 2, ?, 100, ?, 50)",
                (i, f"2024-0{i}-01", a, b),
            )
            conn.execute("INSERT OR IGNORE INTO players (name) VALUES (?), (?)", (a, b))
    expand_pairwise(conn)
    conn.close()


def test_calibration_is_stored_in_the_database_and_nowhere_else(tmp_path):
    db = str(tmp_path / "engine.db")
    seed(db)
    before = set(ENGINE_DIR.rglob("*.json"))

    run_standard_calibration_all(
        db_path=db, verbose=False, model_filter=["glicko2_std"], formats=[2]
    )

    conn = get_connection(db)
    stored = conn.execute(
        "SELECT model_type, player_count, matches_evaluated FROM standard_calibration"
    ).fetchall()
    conn.close()
    assert [tuple(r) for r in stored] == [("glicko2_std", 2, 3)]
    assert set(ENGINE_DIR.rglob("*.json")) == before
    assert not (ENGINE_DIR / "data").exists()
