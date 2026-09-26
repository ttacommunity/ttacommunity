import pytest


def test_daneo_golden_ratio_weights_and_invariant():
    """Tests the Golden Ratio multiplayer information weights and dimensional invariant."""
    from src.models.glicko2.calculator import GOLDEN_MP_WEIGHTS, PHI
    # Check weights
    assert abs(GOLDEN_MP_WEIGHTS[2] - 1.0) < 1e-6
    assert abs(GOLDEN_MP_WEIGHTS[3] - PHI / 2.0) < 1e-6
    assert abs(GOLDEN_MP_WEIGHTS[4] - (PHI ** 2) / 3.0) < 1e-6

    # Check redundancy invariant Delta(N) == phi^-2
    phi_inv_sq = 1.0 / (PHI ** 2)
    delta_3 = (3 - 1) - (GOLDEN_MP_WEIGHTS[3] * 2)  # 2 - phi
    delta_4 = (4 - 1) - (GOLDEN_MP_WEIGHTS[4] * 3)  # 3 - phi^2
    assert abs(delta_3 - phi_inv_sq) < 1e-6
    assert abs(delta_4 - phi_inv_sq) < 1e-6
