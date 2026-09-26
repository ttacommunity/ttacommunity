"""Unit and integration tests for reset tiers, tournament badges, delta configuration, and analysis routes."""
import pytest
import sqlite3
from pathlib import Path

from src.models.glicko2.calculator import get_tier_mean
from src.data.badges import classify_division_tier, derive_tournament_badges
from src.data.merger import set_active_delta_snapshot, get_active_delta_config, list_update_snapshots
from src.web.app import create_app


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


def test_delta_config(tmp_path):
    """Tests setting and retrieving active delta snapshot config."""
    db_file = tmp_path / "test.db"
    from src.data.db import init_db, get_connection
    init_db(str(db_file))
    conn = get_connection(str(db_file))

    try:
        res = set_active_delta_snapshot("2026-09-15_all_games.csv", conn=conn)
        assert res["status"] == "success"
        assert res["active_snapshot"] == "2026-09-15_all_games.csv"
        assert res["cutoff_date"] == "2026-09-15"

        cfg = get_active_delta_config(conn=conn)
        assert cfg is not None
        assert cfg["active_snapshot_file"] == "2026-09-15_all_games.csv"
        assert cfg["cutoff_date"] == "2026-09-15"
    finally:
        conn.close()


def test_analysis_and_movers_routes():
    """Tests web routes for analysis and multi-engine player comparison."""
    app = create_app()
    client = app.test_client()

    # 1. Overview analysis page with standard mode
    resp = client.get('/analysis?eval_mode=standard&format=0&reset_mode=continuous')
    assert resp.status_code == 200
    assert b"Model Calibration" in resp.data or b"Reliability Calibration" in resp.data
    assert b"Overall Predictive Model Rankings" in resp.data or b"Predictive Model Rankings" in resp.data

    # 2. Overview analysis page with walk-forward mode
    resp_wf = client.get('/analysis?eval_mode=walk_forward&format=0')
    assert resp_wf.status_code == 200
    assert b"Walk-Forward" in resp_wf.data

    # 3. Multi-Engine Player Comparison page
    resp_movers = client.get('/analysis/movers?search=Weidenbaum')
    assert resp_movers.status_code == 200
    assert b"Multi-Engine Player Comparison" in resp_movers.data
    assert b"Weidenbaum" in resp_movers.data

    # 4. FAQ page with dynamic mean
    resp_faq = client.get('/faq')
    assert resp_faq.status_code == 200
    assert b"SuperGM" in resp_faq.data
    assert b"Grandmaster" in resp_faq.data


def test_analysis_softer_reset_filter_and_10_bins():
    """Ensure softer reset mode filter, softer models in table, and 10 probability bins are present."""
    app = create_app()
    client = app.test_client()

    resp = client.get('/analysis?eval_mode=standard&format=0')
    assert resp.status_code == 200
    html = resp.get_data(as_text=True)

    # 1. Softer Reset filter option
    assert 'btn_reset_softer' in html
    assert 'Season Reset' in html or 'Softer Reset' in html

    # 2. Values in table
    assert any(label in html for label in ['G2 Adapt (Softer)', 'Glicko-2 Adaptive-T (Softer Reset)', 'G2 Adapt (Season)', 'Glicko-2 Adaptive-T (Season Reset)'])

    # 3. 10 probability bins on the x-axis
    assert '50-55%' in html
    assert '95-100%' in html
    assert '10 Probability Bins' in html


def test_faq_gold_standard_and_grozz_badge():
    """Verify FAQ Gold Standard section rendering, tab title without crown emoji, and Grozz's GM badge."""
    app = create_app()
    client = app.test_client()

    resp_faq = client.get('/faq')
    assert resp_faq.status_code == 200
    faq_html = resp_faq.get_data(as_text=True)

    # 1. GlickoD tab name has NO crown emoji in button
    assert 'GlickoD</button>' in faq_html
    assert '👑 GlickoD</button>' not in faq_html

    # 2. faq-gold-standard section exists as its own top-level faq-section
    assert '<div id="faq-gold-standard" class="faq-section">' in faq_html
    assert 'GlickoD Engine' in faq_html

    # 3. Analysis page includes glicko2_daneo
    resp_analysis = client.get('/analysis')
    assert resp_analysis.status_code == 200
    analysis_html = resp_analysis.get_data(as_text=True)
    assert 'glicko2_daneo' in analysis_html
    assert 'GlickoD' in analysis_html

    # 4. Check Grozz's GM badge in database
    from src.data.db import get_connection
    conn = get_connection()
    try:
        row = conn.execute("SELECT title, title_count, badge_reason FROM players WHERE name = 'Grozz'").fetchone()
        assert row is not None
        assert row['title'] == 'GM'
        assert row['title_count'] >= 1
        assert 'Royal League Emperor' in (row['badge_reason'] or '')
    finally:
        conn.close()


