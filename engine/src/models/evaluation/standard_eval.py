"""Standard (in-sample match-time) calibration and predictive benchmark engine.

Evaluates match-by-match predicted win probability versus actual game outcomes
for all 9 rating engine variants (Glicko-2 Std, Glicko-2 MP, WHR across Continuous,
Soft Reset, and Hard Reset modes) across formats (All, 2P, 3P, 4P).
"""
import argparse
from collections import defaultdict
from datetime import datetime
import json
import math
from pathlib import Path
from typing import Dict, List, Tuple, Optional, Any
import numpy as np

from src.data.db import get_connection
from src.models.whr.engine import WHREngine, GLICKO2_SCALE, DEFAULT_RATING
from src.models.glicko2.engine import Rating, update_rating, GLICKO2_SCALE as G2_SCALE
from src.models.glicko2.calculator import (
    CALIBRATED_TAU, CALIBRATED_SIGMA, INACTIVITY_DRIFT_PER_MONTH, get_tier_mean,
    compute_decoupled_scaling, compute_player_thresholds,
    RESET_ALPHA_SOFT, RESET_LAMBDA_SOFT,
    RESET_ALPHA_SOFTER, RESET_LAMBDA_SOFTER,
    RESET_ALPHA_HARD, RESET_LAMBDA_HARD,
    GOLDEN_MP_WEIGHTS
)
from src.models.glicko2.adaptive_t import (
    calibrated_expectation, update_rating_adaptive,
    ADAPTIVE_T_PARAMS
)
from src.models.evaluation.walk_forward import compute_metrics_from_preds, BIN_LABELS, IDEAL_POINTS

CACHE_PATH = Path(__file__).resolve().parents[3] / "data" / "cache" / "calibration_cache.json"


def evaluate_standard_glicko2(
    weighted: bool = False,
    reset_mode: str = 'continuous',
    player_count: int = 0,
    engine: Optional[str] = None,
    retro_calibrated: bool = False,
    calibration_threshold: int = 15,
    db_path: Optional[str] = None
) -> Dict[str, Any]:
    """Runs sequential step-by-step match-time prediction evaluation for Glicko-2."""
    base_model = engine if engine is not None else ('glicko2_mp' if weighted else 'glicko2_std')
    if base_model == 'glicko2_daneo':
        retro_calibrated = True

    conn = get_connection(db_path)
    try:
        if player_count and player_count > 0:
            rows = conn.execute("""
                SELECT match_id, date, player_a, player_b, outcome_a, weight, player_count
                FROM pairwise_matches
                WHERE player_count = ? AND glicko_eligible = 1
                ORDER BY date, pairwise_id
            """, (player_count,)).fetchall()
        else:
            rows = conn.execute("""
                SELECT match_id, date, player_a, player_b, outcome_a, weight, player_count
                FROM pairwise_matches
                WHERE glicko_eligible = 1
                ORDER BY date, pairwise_id
            """).fetchall()

        periods = defaultdict(list)
        for r in rows:
            m_key = r['date'][:7]
            periods[m_key].append(r)

        sorted_months = sorted(periods.keys())

        # -----------------------------------------------------------------
        # PASS 1 (If retro_calibrated): Discover emergent priors at threshold
        # -----------------------------------------------------------------
        calibrated_priors: Dict[str, Rating] = {}
        player_thresholds: Dict[str, int] = {}
        if retro_calibrated:
            player_thresholds = compute_player_thresholds(rows, standard_threshold=calibration_threshold)
            ratings_p1: Dict[str, Rating] = defaultdict(lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA))
            games_played_p1 = defaultdict(set)
            prev_year_p1 = None

            for m_key in sorted_months:
                cur_year = m_key[:4]
                if prev_year_p1 is not None and cur_year != prev_year_p1 and reset_mode != 'continuous':
                    act_p1 = [r for r in ratings_p1.values() if r.rd < 350.0]
                    if act_p1:
                        pop_mu = sum(r.rating for r in act_p1) / len(act_p1)
                        pop_sigma = max(50.0, (sum((r.rating - pop_mu)**2 for r in act_p1) / len(act_p1))**0.5)
                        pop_rd_mean = sum(r.rd for r in act_p1) / len(act_p1)
                        pop_rd_min = max(20.0, min(r.rd for r in act_p1))
                    else:
                        pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = 1500.0, 175.0, 75.0, 30.0

                    base_a = RESET_ALPHA_SOFTER if reset_mode == 'softer' else (RESET_ALPHA_SOFT if reset_mode == 'soft' else RESET_ALPHA_HARD)
                    base_l = RESET_LAMBDA_SOFTER if reset_mode == 'softer' else (RESET_LAMBDA_SOFT if reset_mode == 'soft' else RESET_LAMBDA_HARD)

                    for p_name, r_cur in list(ratings_p1.items()):
                        if r_cur.rd < 350.0:
                            tm = get_tier_mean(r_cur.rating, mean_rating=pop_mu, sigma=pop_sigma)
                            eff_alpha, eff_lambda = compute_decoupled_scaling(
                                r_cur.rating, r_cur.rd,
                                mean_rating=pop_mu, sigma_pop=pop_sigma,
                                rd_mean=pop_rd_mean, rd_min=pop_rd_min,
                                base_alpha=base_a, base_lambda=base_l
                            )
                            new_rd = min(350.0, r_cur.rd * eff_alpha)
                            new_r = (1.0 - eff_lambda) * r_cur.rating + eff_lambda * tm
                            ratings_p1[p_name] = Rating(new_r, new_rd, r_cur.sigma)
                prev_year_p1 = cur_year

                p1_matches = periods[m_key]
                p1_encounters = defaultdict(list)
                for m in p1_matches:
                    pa, pb = m['player_a'], m['player_b']
                    mid = m['match_id'] if 'match_id' in m.keys() else (m['date'], pa, pb)
                    out_a = m['outcome_a']
                    m_pcount = m['player_count'] if 'player_count' in m.keys() else player_count

                    if base_model == 'glicko2_daneo':
                        w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0)
                    elif base_model == 'glicko2_mp' or weighted:
                        w = m['weight']
                    else:
                        w = 1.0

                    p1_encounters[pa].append((ratings_p1[pb], out_a, w, m_pcount, mid))
                    p1_encounters[pb].append((ratings_p1[pa], 1.0 - out_a, w, m_pcount, mid))

                p1_active = set(p1_encounters.keys())
                for p_name, enc_list in p1_encounters.items():
                    prev_games = len(games_played_p1[p_name])
                    distinct_new = []
                    for it in enc_list:
                        mid = it[4]
                        if mid not in games_played_p1[p_name]:
                            games_played_p1[p_name].add(mid)
                            distinct_new.append(mid)
                    total_games = len(games_played_p1[p_name])
                    p_thresh = player_thresholds.get(p_name, calibration_threshold)

                    def run_update_p1(r_cur, items):
                        if base_model == 'glicko2_adapt':
                            enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                            return update_rating_adaptive(r_cur, enc4, tau=CALIBRATED_TAU)
                        else:
                            enc3 = [(it[0], it[1], it[2]) for it in items]
                            return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=(base_model in ('glicko2_mp', 'glicko2_daneo') or weighted))

                    if prev_games < p_thresh and total_games >= p_thresh and p_name not in calibrated_priors:
                        needed = p_thresh - prev_games
                        mids_up_to_thresh = set(distinct_new[:needed])
                        enc_up_to_thresh = [it for it in enc_list if it[4] in mids_up_to_thresh]
                        enc_after_thresh = [it for it in enc_list if it[4] not in mids_up_to_thresh]

                        r_at_thresh = run_update_p1(ratings_p1[p_name], enc_up_to_thresh)
                        calibrated_priors[p_name] = Rating(r_at_thresh.rating, r_at_thresh.rd, r_at_thresh.sigma)
                        if enc_after_thresh:
                            ratings_p1[p_name] = run_update_p1(r_at_thresh, enc_after_thresh)
                        else:
                            ratings_p1[p_name] = r_at_thresh
                    else:
                        ratings_p1[p_name] = run_update_p1(ratings_p1[p_name], enc_list)

                for p_name in list(ratings_p1.keys()):
                    p_thresh = player_thresholds.get(p_name, calibration_threshold)
                    if p_name not in p1_active and ratings_p1[p_name].rd < 350.0 and len(games_played_p1[p_name]) >= p_thresh:
                        r_cur = ratings_p1[p_name]
                        phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                        new_rd = min(350.0, phi_new * G2_SCALE)
                        ratings_p1[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        # -----------------------------------------------------------------
        # PASS 2 (or Standard Pass): Chronological Evaluation with Option A* Freezing
        # -----------------------------------------------------------------
        ratings: Dict[str, Rating] = {}
        if retro_calibrated:
            for p_name, p_r in calibrated_priors.items():
                ratings[p_name] = Rating(p_r.rating, p_r.rd, p_r.sigma)

        def get_current_rating(p: str) -> Rating:
            if p not in ratings:
                ratings[p] = Rating(1500.0, 350.0, CALIBRATED_SIGMA)
            return ratings[p]

        games_played_p2 = defaultdict(set)
        preds = []
        actuals = []
        prev_year = None

        for m_key in sorted_months:
            cur_year = m_key[:4]

            # Seasonal reset transition
            if prev_year is not None and cur_year != prev_year and reset_mode != 'continuous':
                act_p2 = [r for p, r in ratings.items() if (len(games_played_p2[p]) >= player_thresholds.get(p, calibration_threshold) if retro_calibrated else True) and r.rd < 350.0]
                if act_p2:
                    pop_mu = sum(r.rating for r in act_p2) / len(act_p2)
                    pop_sigma = max(50.0, (sum((r.rating - pop_mu)**2 for r in act_p2) / len(act_p2))**0.5)
                    pop_rd_mean = sum(r.rd for r in act_p2) / len(act_p2)
                    pop_rd_min = max(20.0, min(r.rd for r in act_p2))
                else:
                    pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = 1500.0, 175.0, 75.0, 30.0

                base_a = RESET_ALPHA_SOFTER if reset_mode == 'softer' else (RESET_ALPHA_SOFT if reset_mode == 'soft' else RESET_ALPHA_HARD)
                base_l = RESET_LAMBDA_SOFTER if reset_mode == 'softer' else (RESET_LAMBDA_SOFT if reset_mode == 'soft' else RESET_LAMBDA_HARD)

                for p_name in list(ratings.keys()):
                    is_eligible = (len(games_played_p2[p_name]) >= player_thresholds.get(p_name, calibration_threshold)) if retro_calibrated else True
                    if is_eligible and ratings[p_name].rd < 350.0:
                        tm = get_tier_mean(ratings[p_name].rating, mean_rating=pop_mu, sigma=pop_sigma)
                        eff_alpha, eff_lambda = compute_decoupled_scaling(
                            ratings[p_name].rating, ratings[p_name].rd,
                            mean_rating=pop_mu, sigma_pop=pop_sigma,
                            rd_mean=pop_rd_mean, rd_min=pop_rd_min,
                            base_alpha=base_a, base_lambda=base_l
                        )
                        new_rd = min(350.0, ratings[p_name].rd * eff_alpha)
                        new_r = (1.0 - eff_lambda) * ratings[p_name].rating + eff_lambda * tm
                        ratings[p_name] = Rating(new_r, new_rd, ratings[p_name].sigma)

            prev_year = cur_year
            month_matches = periods[m_key]

            # First: predict each match in the month using pre-match state
            for m in month_matches:
                pa, pb = m['player_a'], m['player_b']
                out_a = m['outcome_a']
                ra = get_current_rating(pa)
                rb = get_current_rating(pb)
                m_pcount = m['player_count'] if 'player_count' in m.keys() else player_count

                if base_model == 'glicko2_adapt':
                    exp_prob = calibrated_expectation(ra.mu, rb.mu, rb.phi, player_count=m_pcount, phi_a=ra.phi)
                else:
                    c = math.sqrt(ra.phi ** 2 + rb.phi ** 2)
                    g_rd = 1.0 / math.sqrt(1.0 + 3.0 * (c ** 2) / (math.pi ** 2))
                    exp_prob = 1.0 / (1.0 + math.exp(-g_rd * (ra.mu - rb.mu)))

                if exp_prob >= 0.5:
                    preds.append(exp_prob)
                    actuals.append(out_a)
                else:
                    preds.append(1.0 - exp_prob)
                    actuals.append(1.0 - out_a)

            # Second: update ratings for this monthly batch
            player_encounters = defaultdict(list)
            for m in month_matches:
                pa, pb = m['player_a'], m['player_b']
                mid = m['match_id'] if 'match_id' in m.keys() else (m['date'], pa, pb)
                out_a = m['outcome_a']
                m_pcount = m['player_count'] if 'player_count' in m.keys() else player_count

                if base_model == 'glicko2_daneo':
                    w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0)
                elif base_model == 'glicko2_mp' or weighted:
                    w = m['weight']
                else:
                    w = 1.0

                ra = get_current_rating(pa)
                rb = get_current_rating(pb)

                player_encounters[pa].append((rb, out_a, w, m_pcount, mid))
                player_encounters[pb].append((ra, 1.0 - out_a, w, m_pcount, mid))

            active_players = set(player_encounters.keys())
            for p_name, enc_list in player_encounters.items():
                def run_update_p2(r_cur, items):
                    if base_model == 'glicko2_adapt':
                        enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                        return update_rating_adaptive(r_cur, enc4, tau=CALIBRATED_TAU)
                    else:
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=(base_model in ('glicko2_mp', 'glicko2_daneo') or weighted))

                if retro_calibrated:
                    prev_games = len(games_played_p2[p_name])
                    distinct_new = []
                    for it in enc_list:
                        mid = it[4]
                        if mid not in games_played_p2[p_name]:
                            games_played_p2[p_name].add(mid)
                            distinct_new.append(mid)
                    total_games = len(games_played_p2[p_name])
                    p_thresh = player_thresholds.get(p_name, calibration_threshold)

                    if p_name not in calibrated_priors:
                        ratings[p_name] = run_update_p2(get_current_rating(p_name), enc_list)
                    elif total_games <= p_thresh:
                        # Option A: Frozen during initial calibration window (1..p_thresh games)
                        pass
                    elif prev_games < p_thresh and total_games > p_thresh:
                        # Surgical Split in transition month: matches up to threshold are frozen; only matches after threshold update
                        needed = p_thresh - prev_games
                        mids_up_to_thresh = set(distinct_new[:needed])
                        enc_after_thresh = [it for it in enc_list if it[4] not in mids_up_to_thresh]
                        if enc_after_thresh:
                            ratings[p_name] = run_update_p2(get_current_rating(p_name), enc_after_thresh)
                    else:
                        ratings[p_name] = run_update_p2(get_current_rating(p_name), enc_list)
                else:
                    ratings[p_name] = run_update_p2(get_current_rating(p_name), enc_list)

            # Apply mild inactivity drift to already-active players who missed this month
            for p_name in list(ratings.keys()):
                p_thresh = player_thresholds.get(p_name, calibration_threshold)
                is_active_or_uncalibrated = (p_name in active_players) or (retro_calibrated and len(games_played_p2[p_name]) < p_thresh)
                if not is_active_or_uncalibrated and ratings[p_name].rd < 350.0:
                    r_cur = ratings[p_name]
                    phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                    new_rd = min(350.0, phi_new * G2_SCALE)
                    ratings[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        return compute_metrics_from_preds(preds, actuals)
    finally:
        conn.close()


def evaluate_standard_whr(
    reset_mode: str = 'continuous',
    player_count: int = 0,
    db_path: Optional[str] = None
) -> Dict[str, Any]:
    """Runs retrospective match-time prediction evaluation for WHR."""
    conn = get_connection(db_path)
    try:
        if player_count and player_count > 0:
            rows = conn.execute("""
                SELECT date, player_a, player_b, outcome_a, weight
                FROM pairwise_matches
                WHERE player_count = ? AND glicko_eligible = 1
                ORDER BY date, pairwise_id
            """, (player_count,)).fetchall()
        else:
            rows = conn.execute("""
                SELECT date, player_a, player_b, outcome_a, weight
                FROM pairwise_matches
                WHERE glicko_eligible = 1
                ORDER BY date, pairwise_id
            """).fetchall()

        whr = WHREngine()
        for r in rows:
            whr.add_game(r['date'], r['player_a'], r['player_b'], r['outcome_a'], r['weight'])

        whr.fit(max_iter=6, tol=1e-3, verbose=False)

        preds = []
        actuals = []

        phi = 1.61803398875
        lambda_soft = (1.0 - 1.0 / phi) * (1.0 / phi)
        lambda_hard = 1.0 / phi

        for r in rows:
            pa, pb = r['player_a'], r['player_b']
            d_str = r['date'][:10]
            out_a = r['outcome_a']

            res_a = whr.get_rating_on_date(pa, d_str)
            res_b = whr.get_rating_on_date(pb, d_str)

            ra = res_a[0] if res_a else DEFAULT_RATING
            rb = res_b[0] if res_b else DEFAULT_RATING

            if reset_mode == 'softer':
                ra = (1.0 - RESET_LAMBDA_SOFTER) * ra + RESET_LAMBDA_SOFTER * get_tier_mean(ra)
                rb = (1.0 - RESET_LAMBDA_SOFTER) * rb + RESET_LAMBDA_SOFTER * get_tier_mean(rb)
            elif reset_mode == 'soft':
                ra = (1.0 - RESET_LAMBDA_SOFT) * ra + RESET_LAMBDA_SOFT * get_tier_mean(ra)
                rb = (1.0 - RESET_LAMBDA_SOFT) * rb + RESET_LAMBDA_SOFT * get_tier_mean(rb)
            elif reset_mode == 'amplified':
                ra = (1.0 - RESET_LAMBDA_HARD) * ra + RESET_LAMBDA_HARD * get_tier_mean(ra)
                rb = (1.0 - RESET_LAMBDA_HARD) * rb + RESET_LAMBDA_HARD * get_tier_mean(rb)

            mu_a = (ra - DEFAULT_RATING) / GLICKO2_SCALE
            mu_b = (rb - DEFAULT_RATING) / GLICKO2_SCALE

            exp_prob = 1.0 / (1.0 + math.exp(-(mu_a - mu_b)))

            if exp_prob >= 0.5:
                preds.append(exp_prob)
                actuals.append(out_a)
            else:
                preds.append(1.0 - exp_prob)
                actuals.append(1.0 - out_a)

        return compute_metrics_from_preds(preds, actuals)
    finally:
        conn.close()


def run_standard_calibration_all(
    db_path: Optional[str] = None,
    verbose: bool = True,
    model_filter: Optional[List[str]] = None,
    formats: Optional[List[int]] = None
) -> Dict[str, Any]:
    """Computes and saves standard calibration metrics for all model configurations across formats."""
    conn = get_connection(db_path)
    all_results = {}
    if CACHE_PATH.exists():
        try:
            all_results = json.loads(CACHE_PATH.read_text(encoding="utf-8"))
            # convert string keys back to int for format dict
            all_results = {int(k): v for k, v in all_results.items()}
        except Exception:
            all_results = {}

    if formats is None:
        formats = [0, 2, 3, 4]

    model_configs = [
        ('glicko2_daneo', None, 'continuous', 'glicko2_daneo', True),
        ('glicko2_daneo_softer', None, 'softer', 'glicko2_daneo', True),
        ('glicko2_std', False, 'continuous', 'glicko2_std', False),
        ('glicko2_std_retro', False, 'continuous', 'glicko2_std', True),
        ('glicko2_std_softer', False, 'softer', 'glicko2_std', False),
        ('glicko2_std_softer_retro', False, 'softer', 'glicko2_std', True),
        ('glicko2_mp', True, 'continuous', 'glicko2_mp', False),
        ('glicko2_mp_retro', True, 'continuous', 'glicko2_mp', True),
        ('glicko2_mp_softer', True, 'softer', 'glicko2_mp', False),
        ('glicko2_mp_softer_retro', True, 'softer', 'glicko2_mp', True),
        ('glicko2_adapt', None, 'continuous', 'glicko2_adapt', False),
        ('glicko2_adapt_retro', None, 'continuous', 'glicko2_adapt', True),
        ('glicko2_adapt_softer', None, 'softer', 'glicko2_adapt', False),
        ('glicko2_adapt_softer_retro', None, 'softer', 'glicko2_adapt', True),
        ('whr', None, 'continuous', 'whr', False),
        ('whr_softer', None, 'softer', 'whr', False),
    ]

    try:
        with conn:
            for fmt in formats:
                if fmt not in all_results:
                    all_results[fmt] = {}
                fmt_label = f"{fmt}P" if fmt > 0 else "All"
                if verbose:
                    print(f"\n--- Standard Calibration: Format {fmt_label} ---")

                for m_type, is_mp, reset_m, eng_base, is_retro in model_configs:
                    if model_filter and m_type not in model_filter:
                        continue
                    if verbose:
                        print(f"  Evaluating {m_type} ({fmt_label})...")

                    if eng_base == 'whr':
                        res = evaluate_standard_whr(reset_mode=reset_m, player_count=fmt, db_path=db_path)
                    elif eng_base == 'glicko2_daneo':
                        res = evaluate_standard_glicko2(engine='glicko2_daneo', reset_mode=reset_m, player_count=fmt, retro_calibrated=True, db_path=db_path)
                    elif eng_base == 'glicko2_adapt':
                        res = evaluate_standard_glicko2(engine='glicko2_adapt', reset_mode=reset_m, player_count=fmt, retro_calibrated=is_retro, db_path=db_path)
                    else:
                        res = evaluate_standard_glicko2(weighted=is_mp, reset_mode=reset_m, player_count=fmt, retro_calibrated=is_retro, db_path=db_path)

                    all_results[fmt][m_type] = res

                    conn.execute("""
                        INSERT INTO standard_calibration (
                            model_type, player_count, period_month, matches_evaluated,
                            brier_score, ece, log_loss, accuracy, bin_data_json
                        ) VALUES (?, ?, 'AGGREGATED', ?, ?, ?, ?, ?, ?)
                        ON CONFLICT(model_type, player_count, period_month) DO UPDATE SET
                            matches_evaluated = excluded.matches_evaluated,
                            brier_score = excluded.brier_score,
                            ece = excluded.ece,
                            log_loss = excluded.log_loss,
                            accuracy = excluded.accuracy,
                            bin_data_json = excluded.bin_data_json
                    """, (
                        m_type, fmt, res['matches'], res['brier'], res['ece'],
                        res['log_loss'], res['accuracy'], json.dumps(res['bin_data'])
                    ))

                    if verbose:
                        print(f"    -> Brier: {res['brier']:.4f}, ECE: {res['ece_pct']:.2f}%, Accuracy: {res['accuracy']:.1f}%")

        # Save to disk cache
        CACHE_PATH.parent.mkdir(parents=True, exist_ok=True)
        CACHE_PATH.write_text(json.dumps(all_results, indent=2), encoding="utf-8")
        if verbose:
            print(f"Saved standard calibration cache to {CACHE_PATH}")

        return all_results
    finally:
        conn.close()


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Run standard in-sample calibration benchmark.")
    parser.add_argument("--format", type=int, default=0, help="Player count format (0=All, 2, 3, 4)")
    args = parser.parse_args()

    run_standard_calibration_all()
