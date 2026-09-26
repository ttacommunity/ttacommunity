"""Unit and integration tests for reset tiers, tournament badges, delta configuration, and analysis routes."""
import pytest
import sqlite3
from pathlib import Path

from src.models.glicko2.calculator import get_tier_mean
from src.data.badges import classify_division_tier, derive_tournament_badges


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
    assert classify_division_tier("Intermezzo S30 - Division 2") == 2  # M
