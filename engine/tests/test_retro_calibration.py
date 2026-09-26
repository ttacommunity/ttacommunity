import pytest
from src.web.app import create_app
from src.web.routes.analysis import MODEL_METADATA, get_ranked_models, compute_calibration
from src.data.db import get_connection

def test_whr_retro_immunity():
    """Tests that WHR is immune to retro calibration and resolves to base WHR."""
    app = create_app()
    app.config['TESTING'] = True
    with app.test_client() as client:
        res = client.get('/?model=whr&retro=1')
        assert res.status_code == 200
        assert b'Whole-History Rating' in res.data

def test_leaderboard_retro_resolution():
    """Tests that leaderboard properly routes to _retro model for Glicko models."""
    app = create_app()
    app.config['TESTING'] = True
    with app.test_client() as client:
        res = client.get('/?model=glicko2_adapt&reset=softer&retro=1')
        assert res.status_code == 200
        assert b'15-Game Retro Prior' in res.data or b'Retro' in res.data

def test_analysis_retro_registration():
    """Tests that analysis metadata registers all retro models with retro=True."""
    assert 'glicko2_adapt_softer_retro' in MODEL_METADATA
    assert MODEL_METADATA['glicko2_adapt_softer_retro']['retro'] is True
    assert MODEL_METADATA['glicko2_adapt_softer']['retro'] is False

    conn = get_connection()
    try:
        calib_data = compute_calibration(conn, player_count=0, eval_mode='standard')
        ranked = get_ranked_models(calib_data)
        retro_keys = [rm['key'] for rm in ranked if rm.get('retro')]
        assert 'glicko2_adapt_softer_retro' in retro_keys
    finally:
        conn.close()

def test_retro_models_populated_in_db():
    """Tests that retro models are populated across all 4 formats in player_ratings and standard_calibration."""
    conn = get_connection()
    try:
        # Check ratings count
        rating_counts = conn.execute(
            'SELECT COUNT(DISTINCT player_count) FROM player_ratings WHERE model_type = ?',
            ('glicko2_adapt_softer_retro',)
        ).fetchone()[0]
        assert rating_counts == 4

        # Check standard calibration count
        calib_counts = conn.execute(
            'SELECT COUNT(DISTINCT player_count) FROM standard_calibration WHERE model_type = ?',
            ('glicko2_adapt_softer_retro',)
        ).fetchone()[0]
        assert calib_counts == 4
    finally:
        conn.close()

def test_analysis_view_renders_retro_lever():
    """Tests that /analysis renders the Prior Calibration lever."""
    app = create_app()
    app.config['TESTING'] = True
    with app.test_client() as client:
        res = client.get('/analysis')
        assert res.status_code == 200
        assert b'Prior Calibration Lever' in res.data
        assert b'15G Retro' in res.data


def test_daneo_gold_standard_populated_in_db():
    """Tests that glicko2_daneo models are populated in player_ratings and standard_calibration."""
    conn = get_connection()
    try:
        for model_key in ['glicko2_daneo', 'glicko2_daneo_softer', 'glicko2_daneo_soft']:
            rc = conn.execute(
                'SELECT COUNT(DISTINCT player_count) FROM player_ratings WHERE model_type = ?',
                (model_key,)
            ).fetchone()[0]
            assert rc == 4, f"Expected 4 formats for {model_key} in player_ratings, got {rc}"

            cc = conn.execute(
                'SELECT COUNT(DISTINCT player_count) FROM standard_calibration WHERE model_type = ?',
                (model_key,)
            ).fetchone()[0]
            assert cc == 4, f"Expected 4 formats for {model_key} in standard_calibration, got {cc}"
    finally:
        conn.close()


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


def test_leaderboard_and_faq_daneo_routes():
    """Tests that leaderboard and FAQ routes expose Gold Standard DANeo."""
    app = create_app()
    app.config['TESTING'] = True
    with app.test_client() as client:
        # Leaderboard with glicko2_daneo
        res_lb = client.get('/?model=glicko2_daneo')
        assert res_lb.status_code == 200
        assert b'GlickoD' in res_lb.data

        # FAQ with gold-standard
        res_faq = client.get('/faq')
        assert res_faq.status_code == 200
        assert b'faq-gold-standard' in res_faq.data
        assert b'Full-Circle Journey' in res_faq.data
