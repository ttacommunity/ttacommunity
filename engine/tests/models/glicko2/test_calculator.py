import pytest
from src.models.glicko2.calculator import get_tier_mean

def test_tier_regression_rules():
    """Tests 16-tier dynamic phi-quantile hierarchy and deflation-only reset rules."""
    # 1. SuperGM 2 (z >= 3.0, >= 2025 with sigma=175) -> deflates toward SuperGM 1 lower bound (1500 + phi^2 * 175)
    mean_val = get_tier_mean(2050.0)
    assert abs(mean_val - (1500.0 + (1.61803398875**2) * 175.0)) < 1e-4

    # 2. SuperGM 1 (phi^2 <= z < 3.0) -> deflates toward GM 2 lower bound (1500 + 2.0 * 175 = 1850)
    assert abs(get_tier_mean(1980.0) - 1850.0) < 1e-4

    # 3. GM 2 (2.0 <= z < phi^2) -> deflates toward GM 1 lower bound (1500 + phi * 175 ≈ 1783.16)
    assert abs(get_tier_mean(1900.0) - (1500.0 + 1.61803398875 * 175.0)) < 1e-4

    # 4. Platinum 1 (0.0 <= z < phi^-2) -> deflates toward mean (1500.0)
    assert get_tier_mean(1520.0) == 1500.0

    # 5. Deflation-Only Invariant: Below or at mean (<= 1500), no upward regression
    assert get_tier_mean(1500.0) == 1500.0
    assert get_tier_mean(1400.0) == 1400.0
    assert get_tier_mean(1200.0) == 1200.0
    assert get_tier_mean(900.0) == 900.0


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
