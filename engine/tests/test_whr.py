"""Tests for WHR solver and engine convergence."""
import math
import numpy as np
import pytest
from src.models.whr.solver import solve_tridiagonal
from src.models.whr.engine import WHREngine

def test_thomas_solver_identity():
    # 3x3 diagonal system
    diag = np.array([2.0, 2.0, 2.0])
    off = np.array([-1.0, -1.0])
    rhs = np.array([1.0, 0.0, 1.0])

    # A = [[2, -1, 0], [-1, 2, -1], [0, -1, 2]]
    # A * [1, 1, 1] = [1, 0, 1] -> solution should be [1, 1, 1]
    sol = solve_tridiagonal(diag, off, rhs)
    np.testing.assert_allclose(sol, [1.0, 1.0, 1.0], atol=1e-5)

def test_whr_two_player_synthetic():
    whr = WHREngine(w2_per_day=0.01)
    
    # Day 1: Player A beats Player B
    whr.add_game('2023-01-01', 'Alice', 'Bob', 1.0)
    # Day 2: Player A beats Player B again
    whr.add_game('2023-01-02', 'Alice', 'Bob', 1.0)
    # Day 3: Player A beats Player B again
    whr.add_game('2023-01-03', 'Alice', 'Bob', 1.0)

    whr.fit(max_iter=10, tol=1e-3)

    alice_r, alice_rd = whr.get_current_rating('Alice')
    bob_r, bob_rd = whr.get_current_rating('Bob')

    # Alice won 3 games in a row -> Alice rating must be > 1500, Bob < 1500
    assert alice_r > 1500.0
    assert bob_r < 1500.0
    assert alice_r > bob_r


def test_golden_retrospective_decay_properties():
    from src.models.whr.engine import PHI, GOLDEN_RATIO_INV, GOLDEN_LAMBDA
    import math

    # Inverted golden ratio should equal 1/phi = phi - 1 ≈ 0.6180339887
    assert math.isclose(GOLDEN_RATIO_INV, 1.0 / PHI, rel_tol=1e-9)
    assert math.isclose(GOLDEN_RATIO_INV, PHI - 1.0, rel_tol=1e-9)

    # 1 year retention: phi^(-2) = 1 - 1/phi ≈ 0.3819660113 (retains 1/phi ≈ 0.6180339887)
    leakage_1yr = math.pow(PHI, -2.0)
    retention_1yr = 1.0 - leakage_1yr
    assert math.isclose(retention_1yr, GOLDEN_RATIO_INV, rel_tol=1e-7)

    # Lambda decay rate per day (3-year half-life)
    expected_lambda = math.log(PHI) / (3.0 * 365.25)
    assert math.isclose(GOLDEN_LAMBDA, expected_lambda, rel_tol=1e-9)


def test_whr_forward_inactivity_projection():
    whr = WHREngine(w2_per_day=0.005)
    whr.add_game('2023-01-01', 'Alice', 'Bob', 1.0)
    whr.fit(max_iter=5)

    # Rating on match day (unprojected)
    r_peak, rd_peak = whr.get_current_rating('Alice')

    # Rating with target_day = last_d (should be identical to peak)
    r_same, rd_same = whr.get_current_rating('Alice', target_day=0)
    assert math.isclose(r_peak, r_same, rel_tol=1e-5)
    assert math.isclose(rd_peak, rd_same, rel_tol=1e-5)

    # Rating projected 365 days forward into the future
    r_fwd, rd_fwd = whr.get_current_rating('Alice', target_day=365)
    assert math.isclose(r_peak, r_fwd, rel_tol=1e-5)  # Rating mean stays identical
    assert rd_fwd > rd_peak  # Uncertainty expands forward in time!
    assert rd_fwd <= 350.0

    # Rating projected 10,000 days forward into the future (must cap at starting RD = 350.0)
    r_huge, rd_huge = whr.get_current_rating('Alice', target_day=10000)
    assert rd_huge == 350.0
