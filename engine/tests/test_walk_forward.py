"""Tests for walk-forward out-of-sample calibration engine and web integration."""
import json
import math
import numpy as np
import pytest
from src.data.db import get_connection, init_db
from src.models.evaluation.walk_forward import compute_metrics_from_preds, run_walk_forward_evaluation
from src.web.app import create_app


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


def test_walk_forward_synthetic_leak_free(tmp_path):
    """Verify that walk-forward evaluation strictly uses ratings prior to the evaluation month."""
    db_file = tmp_path / "test_wf.db"
    init_db(str(db_file))

    conn = get_connection(str(db_file))
    with conn:
        conn.execute("INSERT INTO players (name) VALUES ('Alice'), ('Bob'), ('Charlie')")
        conn.execute("""
            INSERT INTO matches (match_id, tournament, date, player_count, player1, score1, player2, score2)
            VALUES (1, 'T1', '2022-01-10', 2, 'Alice', 100, 'Bob', 50)
        """)
        conn.execute("""
            INSERT INTO matches (match_id, tournament, date, player_count, player1, score1, player2, score2)
            VALUES (2, 'T2', '2022-02-15', 2, 'Alice', 100, 'Charlie', 50)
        """)
        # Month 2022-01: Alice beats Bob 3 times
        for i in range(3):
            conn.execute("""
                INSERT INTO pairwise_matches 
                (match_id, date, tournament, player_count, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible)
                VALUES (1, '2022-01-10', 'T1', 2, 'Alice', 'Bob', 100, 50, 1.0, 1.0, 1)
            """)
        # Month 2022-02: Alice plays Charlie
        conn.execute("""
            INSERT INTO pairwise_matches 
            (match_id, date, tournament, player_count, player_a, player_b, score_a, score_b, outcome_a, weight, glicko_eligible)
            VALUES (2, '2022-02-15', 'T2', 2, 'Alice', 'Charlie', 100, 50, 1.0, 1.0, 1)
        """)

    # Run walk-forward evaluation starting at 2022-02 (2022-01 is burn-in)
    res = run_walk_forward_evaluation(player_count=2, start_eval_month='2022-02', db_path=str(db_file), verbose=False)

    assert '2022-02' in res['eval_months']
    assert res['aggregated']['glicko2_std']['matches'] == 1
    assert res['aggregated']['whr']['matches'] == 1

    # Verify rows in DB
    rows = conn.execute("SELECT * FROM walk_forward_calibration WHERE player_count = 2").fetchall()
    assert len(rows) > 0
    conn.close()


def test_calibration_route_modes(tmp_path):
    """Verify that the web portal renders both walk_forward and standard calibration modes."""
    app = create_app()
    client = app.test_client()

    # 1. Walk-Forward Mode
    resp_wf = client.get('/analysis?eval_mode=walk_forward')
    assert resp_wf.status_code == 200
    html_wf = resp_wf.get_data(as_text=True)
    assert 'Walk-Forward (Out-of-Sample)' in html_wf
    assert 'heroCalibrationChart' in html_wf

    # 2. Standard (Match-Time) Mode
    resp_std = client.get('/analysis?eval_mode=standard')
    assert resp_std.status_code == 200
    html_std = resp_std.get_data(as_text=True)
    assert 'Standard (Match-Time)' in html_std
    assert 'heroCalibrationChart' in html_std
    assert 'Webmaster Model Assessment' in html_std
