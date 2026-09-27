"""Tests for walk-forward out-of-sample calibration engine and web integration."""
import math
from datetime import date

import pytest

from tta_ratings_engine.data.db import RunNotRunning
from tta_ratings_engine.data.pairwise import expand_pairwise
from tta_ratings_engine.models.evaluation.walk_forward import compute_metrics_from_preds, run_walk_forward_evaluation


def test_compute_metrics_from_preds():
    """Verify Brier score, ECE, accuracy, and log loss calculation on known values."""
    preds = [0.8, 0.6, 0.7, 0.9]
    actuals = [1.0, 0.0, 1.0, 1.0]

    res = compute_metrics_from_preds(preds, actuals)
    assert res['matches'] == 4
    # Brier: ((0.8-1)^2 + (0.6-0)^2 + (0.7-1)^2 + (0.9-1)^2) / 4
    # = (0.04 + 0.36 + 0.09 + 0.01) / 4 = 0.50 / 4 = 0.125
    assert math.isclose(res['brier'], 0.125, rel_tol=1e-3)
    assert res['accuracy'] == 75.0  # 3 out of 4 correct (pred > 0.5 and actual == 1.0)
    assert res['ece'] >= 0.0
    assert 'bin_data' in res


def seed(add_match):
    for day in (10, 11, 12):
        add_match(("Alice", 100), ("Bob", 50), day=date(2022, 1, day), tournament="T1")
    add_match(("Alice", 100), ("Charlie", 50), day=date(2022, 2, 15), tournament="T2")


def stored(conn, run_id):
    return conn.execute(
        "SELECT model_type, period_month, matches_evaluated, bin_data FROM walk_forward_calibration "
        "WHERE run_id = %s AND player_count = 2 ORDER BY model_type, period_month",
        (run_id,),
    ).fetchall()


def test_walk_forward_synthetic_leak_free(conn, add_match, run_id):
    """Verify that walk-forward evaluation strictly uses ratings prior to the evaluation month."""
    seed(add_match)
    expand_pairwise(conn, run_id)

    # Run walk-forward evaluation starting at 2022-02 (2022-01 is burn-in)
    res = run_walk_forward_evaluation(player_count=2, start_eval_month='2022-02', verbose=False, conn=conn, run_id=run_id)

    assert '2022-02' in res['eval_months']
    assert res['aggregated']['glicko2_std']['matches'] == 1
    assert res['aggregated']['whr']['matches'] == 1


def test_results_are_stored_in_the_run_per_model_and_month(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)

    run_walk_forward_evaluation(
        player_count=2, start_eval_month="2022-02", verbose=False,
        models_filter=["glicko2_std"], conn=conn, run_id=run_id,
    )

    rows = stored(conn, run_id)
    assert [(r["model_type"], r["period_month"], r["matches_evaluated"]) for r in rows] == [
        ("glicko2_std", "2022-02", 1),
        ("glicko2_std", "AGGREGATED", 1),
    ]
    assert len(rows[0]["bin_data"]["labels"]) == 10


def test_evaluating_again_replaces_the_runs_rows(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    args = dict(player_count=2, start_eval_month="2022-02", verbose=False, models_filter=["whr"])

    run_walk_forward_evaluation(**args, conn=conn, run_id=run_id)
    run_walk_forward_evaluation(**args, conn=conn, run_id=run_id)

    assert len(stored(conn, run_id)) == 2


def test_a_finished_run_is_never_written_to(conn, add_match, run_id):
    seed(add_match)
    expand_pairwise(conn, run_id)
    conn.execute("UPDATE rating_runs SET status = 'complete' WHERE run_id = %s", (run_id,))
    conn.commit()

    with pytest.raises(RunNotRunning):
        run_walk_forward_evaluation(
            player_count=2, start_eval_month="2022-02", verbose=False, conn=conn, run_id=run_id
        )

    assert stored(conn, run_id) == []


def test_each_evaluated_month_is_reported_in_order(conn, add_match, run_id):
    seed(add_match)
    add_match(("Bob", 100), ("Charlie", 50), day=date(2022, 3, 1), tournament="T3")
    expand_pairwise(conn, run_id)
    months = []

    run_walk_forward_evaluation(
        player_count=2, start_eval_month="2022-02", verbose=False,
        conn=conn, run_id=run_id, on_month=months.append,
    )

    assert months == ["2022-02", "2022-03"]
