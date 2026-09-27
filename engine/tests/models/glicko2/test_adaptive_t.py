import pytest
from tta_ratings_engine.models.glicko2.engine import Rating, update_rating, DEFAULT_TAU, apply_inactivity, g, E

def test_adaptive_temperature():
    from tta_ratings_engine.models.glicko2.adaptive_t import get_adaptive_temperature, calibrated_expectation, update_rating_adaptive
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
