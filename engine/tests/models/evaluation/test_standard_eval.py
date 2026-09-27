from datetime import date
from pathlib import Path

import pytest

import tta_ratings_engine
from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.evaluation.standard_eval import (
    evaluate_standard_glicko2,
    evaluate_standard_whr,
    run_standard_calibration_all,
)

ENGINE_DIR = Path(tta_ratings_engine.__file__).resolve().parents[2]


def seed(add_match):
    for i, (a, b) in enumerate(
        [("Alice", "Bob"), ("Bob", "Carol"), ("Alice", "Carol")], start=1
    ):
        add_match((a, 100), (b, 50), day=date(2024, i, 1))


def stored(conn, run_id):
    return conn.execute(
        "SELECT model_type, player_count, period_month, matches_evaluated, bin_data "
        "FROM standard_calibration WHERE run_id = %s ORDER BY model_type",
        (run_id,),
    ).fetchall()


def new_run(conn):
    rid = conn.execute(
        "INSERT INTO rating_runs (engine_version, input_fingerprint) "
        "VALUES ('test', 'test') RETURNING run_id"
    ).fetchone()["run_id"]
    conn.commit()
    return rid


def test_calibration_is_stored_in_the_run_and_nowhere_else(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    before = set(ENGINE_DIR.rglob("*.json"))

    run_standard_calibration_all(
        verbose=False,
        model_filter=["glicko2_std", "whr"],
        formats=[2],
        conn=conn,
        run_id=run_id,
    )

    rows = stored(conn, run_id)
    assert [
        (r["model_type"], r["player_count"], r["period_month"], r["matches_evaluated"])
        for r in rows
    ] == [("glicko2_std", 2, "AGGREGATED", 3), ("whr", 2, "AGGREGATED", 3)]
    assert len(rows[0]["bin_data"]["labels"]) == 10
    assert set(ENGINE_DIR.rglob("*.json")) == before
    assert not (ENGINE_DIR / "data").exists()


def test_calibrating_again_replaces_the_runs_rows(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    args = {"verbose": False, "model_filter": ["glicko2_std"], "formats": [2]}

    run_standard_calibration_all(**args, conn=conn, run_id=run_id)
    run_standard_calibration_all(**args, conn=conn, run_id=run_id)

    assert len(stored(conn, run_id)) == 1


def test_evaluation_reads_only_the_runs_pairs(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    add_match(("Dave", 100), ("Erin", 50), day=date(2024, 4, 1))
    later = new_run(conn)
    expand_pairwise(conn, later)

    g = evaluate_standard_glicko2(player_count=2, conn=conn, run_id=run_id)
    w = evaluate_standard_whr(player_count=2, conn=conn, run_id=run_id)

    assert (g["matches"], w["matches"]) == (3, 3)


def test_a_finished_run_is_never_written_to(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    conn.execute(
        "UPDATE rating_runs SET status = 'failed' WHERE run_id = %s", (run_id,)
    )
    conn.commit()

    with pytest.raises(RunNotRunning):
        run_standard_calibration_all(
            verbose=False,
            model_filter=["glicko2_std"],
            formats=[2],
            conn=conn,
            run_id=run_id,
        )

    assert stored(conn, run_id) == []
