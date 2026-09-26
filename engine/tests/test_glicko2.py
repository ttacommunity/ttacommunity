"""Tests for Glicko-2 engine mathematical correctness."""
import math
import pytest
from src.models.glicko2.engine import Rating, update_rating, DEFAULT_TAU, apply_inactivity, g, E

def test_glickman_example_paper():
    """Verify the official example from Mark Glickman's Glicko-2 paper."""
    # Player rating 1500, RD 200, vol 0.06
    player = Rating(rating=1500.0, rd=200.0, sigma=0.06)
    
    # 3 opponents from paper:
    # 1: rating 1400, RD 30, outcome 1.0 (win)
    # 2: rating 1550, RD 100, outcome 0.0 (loss)
    # 3: rating 1700, RD 300, outcome 0.0 (loss)
    opp1 = Rating(rating=1400.0, rd=30.0, sigma=0.06)
    opp2 = Rating(rating=1550.0, rd=100.0, sigma=0.06)
    opp3 = Rating(rating=1700.0, rd=300.0, sigma=0.06)

    matches = [
        (opp1, 1.0, 1.0),
        (opp2, 0.0, 1.0),
        (opp3, 0.0, 1.0),
    ]

    new_rating = update_rating(player, matches, tau=DEFAULT_TAU, weighted=False)

    # Glickman paper expected: rating ~ 1464.06, RD ~ 151.52, sigma ~ 0.05999
    assert abs(new_rating.rating - 1464.06) < 0.1
    assert abs(new_rating.rd - 151.52) < 0.1
    assert abs(new_rating.sigma - 0.05999) < 0.001

def test_mp_weighted_glicko2():
    """Verify that 3 matches with weight 1/3 retain higher RD than 3 unweighted matches."""
    player = Rating(rating=1500.0, rd=200.0, sigma=0.06)
    opp = Rating(rating=1500.0, rd=100.0, sigma=0.06)

    matches_unweighted = [
        (opp, 1.0, 1.0),
        (opp, 1.0, 1.0),
        (opp, 0.0, 1.0),
    ]

    matches_weighted = [
        (opp, 1.0, 1.0 / 3.0),
        (opp, 1.0, 1.0 / 3.0),
        (opp, 0.0, 1.0 / 3.0),
    ]

    res_unweighted = update_rating(player, matches_unweighted, tau=DEFAULT_TAU, weighted=False)
    res_weighted = update_rating(player, matches_weighted, tau=DEFAULT_TAU, weighted=True)

    # In weighted mode, evidence weight is 1 match total instead of 3 matches.
    # Therefore, RD in weighted mode should be strictly greater (less reduction in uncertainty) than unweighted!
    assert res_weighted.rd > res_unweighted.rd

def test_conservative_rating():
    player = Rating(rating=2000.0, rd=50.0, sigma=0.06)
    assert player.conservative_rating == 2000.0 - 3.0 * 50.0 == 1850.0

def test_adaptive_temperature():
    from src.models.glicko2.adaptive_t import get_adaptive_temperature, calibrated_expectation, update_rating_adaptive
    # Parity: P=0.5 -> T=1.0 exactly across all formats
    assert get_adaptive_temperature(0.50, 2) == 1.0
    assert get_adaptive_temperature(0.50, 3) == 1.0
    assert get_adaptive_temperature(0.50, 4) == 1.0
    assert get_adaptive_temperature(0.50, 0) == 1.0

    # High favorite: P=0.90 -> T > 1.0
    t_2p = get_adaptive_temperature(0.90, 2)
    t_3p = get_adaptive_temperature(0.90, 3)
    t_4p = get_adaptive_temperature(0.90, 4)
    assert t_2p > 1.0
    assert t_3p > 1.0
    assert t_4p > 1.0
    # Hierarchy: 2P duel has highest positive feedback thermal expansion
    assert t_2p > t_4p > t_3p

    # Calibrated expectation softens favorite overconfidence
    player = Rating(rating=1800.0, rd=50.0, sigma=0.06)
    opp = Rating(rating=1500.0, rd=50.0, sigma=0.06)
    
    # Standard expectation
    g_opp = g(opp.phi)
    e_std = E(player.mu, opp.mu, opp.phi)
    # Adaptive expectation softens tail
    e_calib = calibrated_expectation(player.mu, opp.mu, opp.phi, player_count=2)
    assert e_calib < e_std  # softer expectation for favorite in 2p

    # Test update_rating_adaptive runs and updates rating cleanly
    player_init = Rating(rating=1500.0, rd=200.0, sigma=0.06)
    matches = [(opp, 1.0, 0.5, 3)]  # (opp, outcome, weight, player_count)
    res = update_rating_adaptive(player_init, matches)
    assert res.rating > 1500.0
    assert res.rd < 200.0


