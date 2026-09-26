"""Walk-forward (leak-free out-of-sample t+1) calibration and predictive benchmark engine."""
import argparse
from collections import defaultdict
from datetime import datetime
import json
import math
from typing import Dict, List, Tuple, Optional, Any
import numpy as np

from src.data.db import get_connection
from src.models.whr.engine import WHREngine, GLICKO2_SCALE, DEFAULT_RATING
from src.models.glicko2.engine import Rating, update_rating, GLICKO2_SCALE as G2_SCALE, g
from src.models.glicko2.calculator import (
    CALIBRATED_TAU, CALIBRATED_SIGMA, INACTIVITY_DRIFT_PER_MONTH, get_tier_mean,
    RESET_ALPHA_SOFTER, RESET_LAMBDA_SOFTER,
    GOLDEN_MP_WEIGHTS, compute_decoupled_scaling, compute_player_thresholds
)
from src.models.glicko2.adaptive_t import (
    calibrated_expectation, update_rating_adaptive
)

BIN_LABELS = ['50-55%', '55-60%', '60-65%', '65-70%', '70-75%', '75-80%', '80-85%', '85-90%', '90-95%', '95-100%']
IDEAL_POINTS = [0.525, 0.575, 0.625, 0.675, 0.725, 0.775, 0.825, 0.875, 0.925, 0.975]
BIN_EDGES = np.linspace(0.5, 1.0, 11)
NUM_BINS = len(BIN_LABELS)

ALL_16_MODELS = [
    'glicko2_daneo', 'glicko2_daneo_softer',
    'glicko2_std', 'glicko2_std_retro', 'glicko2_std_softer', 'glicko2_std_softer_retro',
    'glicko2_mp', 'glicko2_mp_retro', 'glicko2_mp_softer', 'glicko2_mp_softer_retro',
    'glicko2_adapt', 'glicko2_adapt_retro', 'glicko2_adapt_softer', 'glicko2_adapt_softer_retro',
    'whr', 'whr_softer'
]
ALL_12_MODELS = ALL_16_MODELS  # Alias for backward compatibility


def is_retro(m: str) -> bool:
    """Returns True if the model utilizes Option A 15-game emergent prior calibration."""
    return 'retro' in m or 'daneo' in m


def get_fam(m: str) -> str:
    """Extracts the underlying engine family."""
    if 'daneo' in m: return 'daneo'
    if 'adapt' in m: return 'adapt'
    if 'mp' in m: return 'mp'
    if 'whr' in m: return 'whr'
    return 'std'


def compute_metrics_from_preds(preds: List[float], actuals: List[float]) -> Dict[str, Any]:
    """Computes Brier score, ECE, log loss, accuracy, and bin data from probability predictions."""
    if not preds:
        return {
            'matches': 0,
            'brier': 0.25,
            'ece': 0.0,
            'ece_pct': 0.0,
            'log_loss': 0.6931,
            'accuracy': 0.5,
            'bin_data': {
                'labels': BIN_LABELS,
                'ideal': IDEAL_POINTS,
                'actuals': IDEAL_POINTS,
                'preds': IDEAL_POINTS,
                'counts': [0] * NUM_BINS
            }
        }

    p_arr = np.array(preds, dtype=float)
    y_arr = np.array(actuals, dtype=float)
    n = len(p_arr)

    # 1. Brier Score (MSE)
    brier = float(np.mean((p_arr - y_arr) ** 2))

    # 2. Binary Accuracy (treating 0.5 draws as 0.5 point)
    correct = np.sum((p_arr > 0.5) & (y_arr > 0.5)) + 0.5 * np.sum(y_arr == 0.5)
    accuracy = float(correct / n)

    # 3. Log Loss (cross-entropy with epsilon clipping)
    eps = 1e-7
    p_clipped = np.clip(p_arr, eps, 1.0 - eps)
    ll = float(-np.mean(y_arr * np.log(p_clipped) + (1.0 - y_arr) * np.log(1.0 - p_clipped)))

    # 4. Bin Frequencies & ECE
    bin_actuals = []
    bin_preds = []
    bin_counts = []
    for i in range(NUM_BINS):
        low, high = BIN_EDGES[i], BIN_EDGES[i + 1]
        mask = (p_arr >= low) & (p_arr < high if i < NUM_BINS - 1 else p_arr <= high)
        cnt = int(np.sum(mask))
        bin_counts.append(cnt)
        if cnt > 0:
            bin_actuals.append(round(float(np.mean(y_arr[mask])), 4))
            bin_preds.append(round(float(np.mean(p_arr[mask])), 4))
        else:
            mid = round(float((low + high) / 2), 4)
            bin_actuals.append(mid)
            bin_preds.append(mid)

    ece = 0.0
    for i in range(NUM_BINS):
        cnt = bin_counts[i]
        if cnt > 0:
            ece += (cnt / n) * abs(bin_actuals[i] - bin_preds[i])

    return {
        'matches': n,
        'brier': round(brier, 4),
        'ece': round(ece, 4),
        'ece_pct': round(ece * 100.0, 2),
        'log_loss': round(ll, 4),
        'accuracy': round(accuracy * 100.0, 2),
        'bin_data': {
            'labels': BIN_LABELS,
            'ideal': IDEAL_POINTS,
            'actuals': bin_actuals,
            'preds': bin_preds,
            'counts': bin_counts
        }
    }


def run_walk_forward_evaluation(
    player_count: int = 0,
    start_eval_month: str = '2022-01',
    db_path: Optional[str] = None,
    verbose: bool = True,
    models_filter: Optional[List[str]] = None
) -> Dict[str, Any]:
    """Executes a monthly expanding-window walk-forward evaluation across all 16 models."""
    conn = get_connection(db_path)
    try:
        fmt_label = f"{player_count}p" if player_count > 0 else "All Formats"
        if verbose:
            print(f"\n=======================================================")
            print(f"Starting Walk-Forward Calibration: {fmt_label} (Start: {start_eval_month})")
            print(f"=======================================================")

        # Load chronological matches
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

        if verbose:
            print(f"Loaded {len(rows)} eligible pairwise matches.")

        # Compute dynamic player calibration thresholds (15 games, or max k if inactive > 1 year)
        player_thresholds = compute_player_thresholds(rows, standard_threshold=15, inactivity_days=365)

        # Group matches by calendar month YYYY-MM
        month_matches = defaultdict(list)
        for r in rows:
            m = r['date'][:7]
            month_matches[m].append(r)

        sorted_months = sorted(month_matches.keys())
        eval_months = [m for m in sorted_months if m >= start_eval_month]
        burn_in_months = [m for m in sorted_months if m < start_eval_month]

        if verbose:
            print(f"Burn-in period: {len(burn_in_months)} months ({burn_in_months[0]} to {burn_in_months[-1]})")
            print(f"Evaluation period: {len(eval_months)} months ({eval_months[0]} to {eval_months[-1]})")

        if models_filter:
            models = [m for m in ALL_16_MODELS if m in models_filter]
        else:
            models = ALL_16_MODELS

        g2_models = [m for m in models if 'whr' not in m]
        has_whr = any('whr' in m for m in models)

        # -------------------------------------------------------------
        # PASS 1: Emergent priors for retrospective calibration (Option A)
        # -------------------------------------------------------------
        families_retro = set(get_fam(m) for m in g2_models if is_retro(m))
        calibrated_priors = {fam: {} for fam in families_retro}

        if families_retro:
            if verbose:
                print(f"Executing Pass 1: Discovering 15-game emergent priors for {len(families_retro)} engine families...")
            for fam in families_retro:
                ratings_p1 = defaultdict(lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                games_p1 = defaultdict(set)
                for m in sorted_months:
                    p1_matches = month_matches[m]
                    p1_enc = defaultdict(list)
                    for r in p1_matches:
                        pa, pb, out = r['player_a'], r['player_b'], r['outcome_a']
                        mid = r['match_id'] if 'match_id' in r.keys() else (r['date'], pa, pb)
                        p_cnt = r['player_count'] if 'player_count' in r.keys() else player_count
                        if fam == 'daneo': w = GOLDEN_MP_WEIGHTS.get(p_cnt, 1.0)
                        elif fam == 'mp': w = r['weight']
                        else: w = 1.0
                        p1_enc[pa].append((ratings_p1[pb], out, w, p_cnt, mid))
                        p1_enc[pb].append((ratings_p1[pa], 1.0 - out, w, p_cnt, mid))

                    p1_act = set(p1_enc.keys())
                    for p_name, enc_list in p1_enc.items():
                        prev_g = len(games_p1[p_name])
                        distinct_new = []
                        for it in enc_list:
                            mid = it[4]
                            if mid not in games_p1[p_name]:
                                games_p1[p_name].add(mid)
                                distinct_new.append(mid)
                        tot_g = len(games_p1[p_name])

                        def run_up_p1(r_cur, items):
                            if fam == 'adapt':
                                enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                                return update_rating_adaptive(r_cur, enc4, tau=CALIBRATED_TAU)
                            elif fam in ('daneo', 'mp'):
                                enc3 = [(it[0], it[1], it[2]) for it in items]
                                return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=True)
                            else:
                                enc3 = [(it[0], it[1], it[2]) for it in items]
                                return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=False)

                        p_thresh = player_thresholds.get(p_name, 15)
                        if prev_g < p_thresh and tot_g >= p_thresh and p_name not in calibrated_priors[fam]:
                            needed = p_thresh - prev_g
                            mids_up = set(distinct_new[:needed])
                            enc_up = [it for it in enc_list if it[4] in mids_up]
                            enc_after = [it for it in enc_list if it[4] not in mids_up]
                            r_at_k = run_up_p1(ratings_p1[p_name], enc_up)
                            calibrated_priors[fam][p_name] = Rating(r_at_k.rating, r_at_k.rd, r_at_k.sigma)
                            ratings_p1[p_name] = run_up_p1(r_at_k, enc_after) if enc_after else r_at_k
                        else:
                            ratings_p1[p_name] = run_up_p1(ratings_p1[p_name], enc_list)

                    for p_name in list(ratings_p1.keys()):
                        if p_name not in p1_act and ratings_p1[p_name].rd < 350.0:
                            r_cur = ratings_p1[p_name]
                            phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                            ratings_p1[p_name] = Rating(r_cur.rating, min(350.0, phi_new * G2_SCALE), r_cur.sigma)

        # -------------------------------------------------------------
        # PASS 2: Walk-Forward Expanding Evaluation Loop
        # -------------------------------------------------------------
        g2_ratings = {}
        for m in g2_models:
            fam = get_fam(m)
            if is_retro(m):
                g2_ratings[m] = {p: Rating(r.rating, r.rd, r.sigma) for p, r in calibrated_priors[fam].items()}
            else:
                g2_ratings[m] = defaultdict(lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA))

        games_played_p2 = {m: defaultdict(set) for m in g2_models}
        whr = WHREngine() if has_whr else None

        def apply_annual_resets(prev_y: str, cur_y: str):
            if prev_y is not None and cur_y != prev_y:
                for m_key in g2_models:
                    if '_softer' in m_key:
                        r_map = g2_ratings[m_key]
                        act_players = [r for p, r in r_map.items() if (len(games_played_p2[m_key][p]) >= player_thresholds.get(p, 15) if is_retro(m_key) else True) and r.rd < 350.0]
                        if act_players:
                            pop_mu = sum(r.rating for r in act_players) / len(act_players)
                            pop_sigma = max(50.0, (sum((r.rating - pop_mu)**2 for r in act_players) / len(act_players))**0.5)
                            pop_rd_mean = sum(r.rd for r in act_players) / len(act_players)
                            pop_rd_min = max(20.0, min(r.rd for r in act_players))
                        else:
                            pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = 1500.0, 175.0, 75.0, 30.0

                        for p_name in list(r_map.keys()):
                            is_elig = (len(games_played_p2[m_key][p_name]) >= player_thresholds.get(p_name, 15)) if is_retro(m_key) else True
                            if is_elig and r_map[p_name].rd < 350.0:
                                tm = get_tier_mean(r_map[p_name].rating, mean_rating=pop_mu, sigma=pop_sigma)
                                eff_alpha, eff_lambda = compute_decoupled_scaling(
                                    r_map[p_name].rating, r_map[p_name].rd,
                                    mean_rating=pop_mu, sigma_pop=pop_sigma,
                                    rd_mean=pop_rd_mean, rd_min=pop_rd_min,
                                    base_alpha=RESET_ALPHA_SOFTER, base_lambda=RESET_LAMBDA_SOFTER
                                )
                                new_rd = min(350.0, r_map[p_name].rd * eff_alpha)
                                new_r = (1.0 - eff_lambda) * r_map[p_name].rating + eff_lambda * tm
                                r_map[p_name] = Rating(new_r, new_rd, r_map[p_name].sigma)

        # Execute Burn-in Phase
        if verbose:
            print("Processing burn-in phase...")

        prev_year = None
        for m in burn_in_months:
            cur_year = m[:4]
            apply_annual_resets(prev_year, cur_year)
            prev_year = cur_year

            m_rows = month_matches[m]
            if has_whr:
                for r in m_rows:
                    whr.add_game(r['date'], r['player_a'], r['player_b'], r['outcome_a'], r['weight'])

            encounters = {m_key: defaultdict(list) for m_key in g2_models}
            for r in m_rows:
                pa, pb, out = r['player_a'], r['player_b'], r['outcome_a']
                mid = r['match_id'] if 'match_id' in r.keys() else (r['date'], pa, pb)
                m_pcount = r['player_count'] if 'player_count' in r.keys() else player_count

                for m_key in g2_models:
                    r_map = g2_ratings[m_key]
                    fam = get_fam(m_key)
                    ra = r_map.get(pa, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pa]
                    rb = r_map.get(pb, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pb]
                    if fam == 'daneo': w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0)
                    elif fam == 'mp': w = r['weight']
                    else: w = 1.0
                    encounters[m_key][pa].append((rb, out, w, m_pcount, mid))
                    encounters[m_key][pb].append((ra, 1.0 - out, w, m_pcount, mid))

            for m_key in g2_models:
                r_map = g2_ratings[m_key]
                fam = get_fam(m_key)
                gp = games_played_p2[m_key]
                act = set(encounters[m_key].keys())

                def run_up_burn(r_cur, items):
                    if fam == 'adapt':
                        enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                        return update_rating_adaptive(r_cur, enc4, tau=CALIBRATED_TAU)
                    elif fam in ('daneo', 'mp'):
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=True)
                    else:
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=False)

                for p_name, enc_list in encounters[m_key].items():
                    if is_retro(m_key):
                        prev_g = len(gp[p_name])
                        distinct_new = []
                        for it in enc_list:
                            mid = it[4]
                            if mid not in gp[p_name]:
                                gp[p_name].add(mid)
                                distinct_new.append(mid)
                        tot_g = len(gp[p_name])
                        cur_r = r_map.get(p_name, Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                        p_thresh = player_thresholds.get(p_name, 15)
                        if p_name not in calibrated_priors[fam]:
                            r_map[p_name] = run_up_burn(cur_r, enc_list)
                        elif tot_g <= p_thresh:
                            pass
                        elif prev_g < p_thresh and tot_g > p_thresh:
                            mids_up = set(distinct_new[:(p_thresh - prev_g)])
                            enc_after = [it for it in enc_list if it[4] not in mids_up]
                            if enc_after:
                                r_map[p_name] = run_up_burn(cur_r, enc_after)
                        else:
                            r_map[p_name] = run_up_burn(cur_r, enc_list)
                    else:
                        r_map[p_name] = run_up_burn(r_map[p_name], enc_list)

                for p in list(r_map.keys()):
                    if p not in act and r_map[p].rd < 350.0:
                        r_cur = r_map[p]
                        phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                        r_map[p] = Rating(r_cur.rating, min(350.0, phi_new * G2_SCALE), r_cur.sigma)

        # Fit WHR burn-in
        if has_whr and burn_in_months:
            whr.fit(max_iter=6, tol=1e-3, verbose=False)
            if verbose:
                print("Burn-in complete.")

        # Data collection for evaluation
        monthly_results = defaultdict(dict)
        overall_preds = {m: [] for m in models}
        overall_actuals = {m: [] for m in models}

        # Walk-Forward Expanding Evaluation Loop
        for idx, tm in enumerate(eval_months):
            cur_year = tm[:4]
            apply_annual_resets(prev_year, cur_year)
            prev_year = cur_year

            m_matches = month_matches[tm]
            if not m_matches:
                continue

            # (A) Pure Out-of-Sample Predictions strictly BEFORE observing matches in month tm
            month_preds = {m: [] for m in models}
            month_actuals = {m: [] for m in models}

            for r in m_matches:
                pa, pb, out = r['player_a'], r['player_b'], r['outcome_a']
                m_pcount = r['player_count'] if 'player_count' in r.keys() else player_count

                # Glicko-2 models predictions
                for m_key in g2_models:
                    r_map = g2_ratings[m_key]
                    fam = get_fam(m_key)
                    ra = r_map.get(pa, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pa]
                    rb = r_map.get(pb, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pb]

                    if fam == 'adapt':
                        p_val = calibrated_expectation(ra.mu, rb.mu, rb.phi, player_count=m_pcount, phi_a=ra.phi)
                    else:
                        c = math.sqrt(ra.phi ** 2 + rb.phi ** 2)
                        g_rd = 1.0 / math.sqrt(1.0 + 3.0 * (c ** 2) / (math.pi ** 2))
                        p_val = 1.0 / (1.0 + math.exp(-g_rd * (ra.mu - rb.mu)))

                    if p_val >= 0.5:
                        month_preds[m_key].append(p_val)
                        month_actuals[m_key].append(out)
                    else:
                        month_preds[m_key].append(1.0 - p_val)
                        month_actuals[m_key].append(1.0 - out)

                # WHR predictions strictly prior to observing month tm
                if has_whr:
                    cur_a = whr.get_current_rating(pa)
                    cur_b = whr.get_current_rating(pb)
                    raw_a = cur_a[0] if cur_a else 1500.0
                    raw_b = cur_b[0] if cur_b else 1500.0

                    if 'whr' in models:
                        p_whr = 1.0 / (1.0 + 10.0 ** (-(raw_a - raw_b) / 400.0))
                        if p_whr >= 0.5:
                            month_preds['whr'].append(p_whr)
                            month_actuals['whr'].append(out)
                        else:
                            month_preds['whr'].append(1.0 - p_whr)
                            month_actuals['whr'].append(1.0 - out)

                    if 'whr_softer' in models:
                        r_softer_a = (1.0 - RESET_LAMBDA_SOFTER) * raw_a + RESET_LAMBDA_SOFTER * get_tier_mean(raw_a)
                        r_softer_b = (1.0 - RESET_LAMBDA_SOFTER) * raw_b + RESET_LAMBDA_SOFTER * get_tier_mean(raw_b)
                        p_softer = 1.0 / (1.0 + 10.0 ** (-(r_softer_a - r_softer_b) / 400.0))
                        if p_softer >= 0.5:
                            month_preds['whr_softer'].append(p_softer)
                            month_actuals['whr_softer'].append(out)
                        else:
                            month_preds['whr_softer'].append(1.0 - p_softer)
                            month_actuals['whr_softer'].append(1.0 - out)

            # Record monthly metrics
            for m in models:
                res_m = compute_metrics_from_preds(month_preds[m], month_actuals[m])
                monthly_results[tm][m] = res_m
                overall_preds[m].extend(month_preds[m])
                overall_actuals[m].extend(month_actuals[m])

            if verbose and (idx % 6 == 0 or idx == len(eval_months) - 1):
                stat_parts = []
                for sample_m in ['glicko2_daneo', 'glicko2_std_retro', 'glicko2_std', 'whr']:
                    if sample_m in monthly_results[tm]:
                        stat_parts.append(f"{sample_m}: {monthly_results[tm][sample_m]['brier']:.4f}")
                print(f"[{idx+1}/{len(eval_months)}] Month {tm} ({len(m_matches):5d} games) | Brier -> " + ", ".join(stat_parts))

            # (B) Incremental Online Update for month tm matches
            if has_whr:
                for r in m_matches:
                    whr.add_game(r['date'], r['player_a'], r['player_b'], r['outcome_a'], r['weight'])
                whr.fit(max_iter=3, tol=1e-3, verbose=False)

            encounters = {m_key: defaultdict(list) for m_key in g2_models}
            for r in m_matches:
                pa, pb, out = r['player_a'], r['player_b'], r['outcome_a']
                mid = r['match_id'] if 'match_id' in r.keys() else (r['date'], pa, pb)
                m_pcount = r['player_count'] if 'player_count' in r.keys() else player_count

                for m_key in g2_models:
                    r_map = g2_ratings[m_key]
                    fam = get_fam(m_key)
                    ra = r_map.get(pa, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pa]
                    rb = r_map.get(pb, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if is_retro(m_key) else r_map[pb]
                    if fam == 'daneo': w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0)
                    elif fam == 'mp': w = r['weight']
                    else: w = 1.0
                    encounters[m_key][pa].append((rb, out, w, m_pcount, mid))
                    encounters[m_key][pb].append((ra, 1.0 - out, w, m_pcount, mid))

            for m_key in g2_models:
                r_map = g2_ratings[m_key]
                fam = get_fam(m_key)
                gp = games_played_p2[m_key]
                act = set(encounters[m_key].keys())

                def run_up_eval(r_cur, items):
                    if fam == 'adapt':
                        enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                        return update_rating_adaptive(r_cur, enc4, tau=CALIBRATED_TAU)
                    elif fam in ('daneo', 'mp'):
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=True)
                    else:
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=CALIBRATED_TAU, weighted=False)

                for p_name, enc_list in encounters[m_key].items():
                    if is_retro(m_key):
                        prev_g = len(gp[p_name])
                        distinct_new = []
                        for it in enc_list:
                            mid = it[4]
                            if mid not in gp[p_name]:
                                gp[p_name].add(mid)
                                distinct_new.append(mid)
                        tot_g = len(gp[p_name])
                        cur_r = r_map.get(p_name, Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                        p_thresh = player_thresholds.get(p_name, 15)
                        if p_name not in calibrated_priors[fam]:
                            r_map[p_name] = run_up_eval(cur_r, enc_list)
                        elif tot_g <= p_thresh:
                            pass
                        elif prev_g < p_thresh and tot_g > p_thresh:
                            mids_up = set(distinct_new[:(p_thresh - prev_g)])
                            enc_after = [it for it in enc_list if it[4] not in mids_up]
                            if enc_after:
                                r_map[p_name] = run_up_eval(cur_r, enc_after)
                        else:
                            r_map[p_name] = run_up_eval(cur_r, enc_list)
                    else:
                        r_map[p_name] = run_up_eval(r_map[p_name], enc_list)

                for p in list(r_map.keys()):
                    if p not in act and r_map[p].rd < 350.0:
                        r_cur = r_map[p]
                        phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                        r_map[p] = Rating(r_cur.rating, min(350.0, phi_new * G2_SCALE), r_cur.sigma)

        # 3. Overall Aggregate Metrics
        aggregated_results = {}
        for m in models:
            aggregated_results[m] = compute_metrics_from_preds(overall_preds[m], overall_actuals[m])

        if verbose:
            print(f"\n--- Walk-Forward Summary ({fmt_label}) ---")
            for m in models:
                agg = aggregated_results[m]
                print(f"Model: {m:28s} | Out-of-Sample Brier: {agg['brier']:.4f} | ECE: {agg['ece_pct']:.2f}% | LogLoss: {agg['log_loss']:.4f} | Acc: {agg['accuracy']:.1f}%")

        # 4. Save to Database
        save_models = [m for m in models if (models_filter is None or m in models_filter)]
        with conn:
            # Clear previous entries for this player_count format
            if models_filter:
                placeholders = ','.join('?' for _ in models_filter)
                conn.execute(f"DELETE FROM walk_forward_calibration WHERE player_count = ? AND model_type IN ({placeholders})", (player_count, *models_filter))
            else:
                conn.execute("DELETE FROM walk_forward_calibration WHERE player_count = ?", (player_count,))

            # Insert aggregated summary records
            for m in save_models:
                agg = aggregated_results[m]
                conn.execute("""
                    INSERT INTO walk_forward_calibration 
                    (model_type, player_count, period_month, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data_json)
                    VALUES (?, ?, 'AGGREGATED', ?, ?, ?, ?, ?, ?)
                """, (
                    m, player_count, agg['matches'], agg['brier'], agg['ece'],
                    agg['log_loss'], agg['accuracy'], json.dumps(agg['bin_data'])
                ))

            # Insert individual monthly records
            for tm, m_data in monthly_results.items():
                for m in save_models:
                    item = m_data[m]
                    conn.execute("""
                        INSERT INTO walk_forward_calibration 
                        (model_type, player_count, period_month, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data_json)
                        VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
                    """, (
                        m, player_count, tm, item['matches'], item['brier'], item['ece'],
                        item['log_loss'], item['accuracy'], json.dumps(item['bin_data'])
                    ))

        if verbose:
            print(f"Saved walk-forward results to database for format {fmt_label}.")

        return {
            'player_count': player_count,
            'eval_months': eval_months,
            'aggregated': aggregated_results,
            'monthly': monthly_results
        }
    finally:
        conn.close()


def run_walk_forward_all(
    start_eval_month: str = '2022-01',
    db_path: Optional[str] = None,
    verbose: bool = True,
    models_filter: Optional[List[str]] = None,
    formats: Optional[List[int]] = None
) -> Dict[int, Any]:
    """Runs walk-forward evaluation across formats."""
    all_res = {}
    if formats is None:
        formats = [0, 2, 3, 4]
    for fmt in formats:
        all_res[fmt] = run_walk_forward_evaluation(
            player_count=fmt,
            start_eval_month=start_eval_month,
            db_path=db_path,
            verbose=verbose,
            models_filter=models_filter
        )
    return all_res


if __name__ == '__main__':
    parser = argparse.ArgumentParser(description="Run walk-forward out-of-sample calibration benchmark.")
    parser.add_argument('--format', type=int, default=0, help="Format player count (0=All, 2=2p, 3=3p, 4=4p)")
    parser.add_argument('--all-formats', action='store_true', help="Run across all formats (0, 2, 3, 4)")
    parser.add_argument('--start-month', type=str, default='2022-01', help="Starting month for out-of-sample evaluation (default: 2022-01)")
    args = parser.parse_args()

    if args.all_formats:
        run_walk_forward_all(start_eval_month=args.start_month)
    else:
        run_walk_forward_evaluation(player_count=args.format, start_eval_month=args.start_month)
