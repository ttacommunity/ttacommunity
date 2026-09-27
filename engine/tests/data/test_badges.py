import pytest
from tta_ratings_engine.data.badges import classify_division_tier, derive_tournament_badges

def test_badge_classification():
    """Tests tournament division to badge tier classification."""
    # Royal League
    assert classify_division_tier("RL_s08 - Emperor") == 1  # GM
    assert classify_division_tier("RL_s08 - King") == 2     # M
    assert classify_division_tier("RL_s08 - Prince A") == 3 # Platinum
    assert classify_division_tier("RL_s08 - Duke B") == 4   # Gold
    assert classify_division_tier("RL_s08 - Marquess C") == 5 # Silver
    assert classify_division_tier("RL_s08 - Count D") == 6  # Bronze
    assert classify_division_tier("RL_s08 - Viscount") == 7 # Wood

    # International Championship
    assert classify_division_tier("International S33 - Diamond") == 1  # GM
    assert classify_division_tier("International S33 - Platinum") == 2 # M
    assert classify_division_tier("International S33 - Gold") == 3     # Platinum
    assert classify_division_tier("International S33 - Silver") == 4   # Gold
    assert classify_division_tier("International S33 - Bronze") == 5   # Silver

    # Intermezzo
    assert classify_division_tier("Intermezzo S30 - Master 1") == 1    # GM
    assert classify_division_tier("Intermezzo S30 - Division 2") == 2
