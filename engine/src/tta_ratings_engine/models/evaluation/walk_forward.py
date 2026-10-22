"""Walk-forward (leak-free out-of-sample t+1) calibration and predictive benchmark engine."""

import math
from collections import defaultdict
from collections.abc import Callable
from typing import Any

import numpy as np
import psycopg
from psycopg.types.json import Jsonb

from tta_ratings_engine.data.db import load_pairs, require_running
from tta_ratings_engine.models.glicko2.calculator import (
    CALIBRATED_SIGMA,
    CALIBRATED_TAU,
    GOLDEN_MP_WEIGHTS,
    INACTIVITY_DRIFT_PER_MONTH,
    RESET_ALPHA_SOFTER,
    RESET_LAMBDA_SOFTER,
    PlayerThresholds,
    compute_decoupled_scaling,
    get_tier_mean,
)
from tta_ratings_engine.models.glicko2.engine import GLICKO2_SCALE as G2_SCALE
from tta_ratings_engine.models.glicko2.engine import Rating, update_rating

BIN_LABELS = [
    "50-55%",
    "55-60%",
    "60-65%",
    "65-70%",
    "70-75%",
    "75-80%",
    "80-85%",
    "85-90%",
    "90-95%",
    "95-100%",
]
IDEAL_POINTS = [0.525, 0.575, 0.625, 0.675, 0.725, 0.775, 0.825, 0.875, 0.925, 0.975]
BIN_EDGES = np.linspace(0.5, 1.0, 11)
NUM_BINS = len(BIN_LABELS)

# The models a run evaluates: the ones run.py computes.
ALL_MODELS = [
    "glicko2_daneo",
    "glicko2_daneo_softer",
    "glicko2_std",
]


def is_retro(m: str) -> bool:
    """Returns True if the model utilizes Option A 15-game emergent prior calibration."""
    return "retro" in m or "daneo" in m


def get_fam(m: str) -> str:
    """Extracts the underlying engine family."""
    if "daneo" in m:
        return "daneo"
    if "mp" in m:
        return "mp"
    return "std"


def compute_metrics_from_preds(
    preds: list[float], actuals: list[float]
) -> dict[str, Any]:
    """Computes Brier score, ECE, log loss, accuracy, and bin data from probability predictions."""
    if not preds:
        return {
            "matches": 0,
            "brier": 0.25,
            "ece": 0.0,
            "ece_pct": 0.0,
            "log_loss": 0.6931,
            "accuracy": 0.5,
            "bin_data": {
                "labels": BIN_LABELS,
                "ideal": IDEAL_POINTS,
                "actuals": IDEAL_POINTS,
                "preds": IDEAL_POINTS,
                "counts": [0] * NUM_BINS,
            },
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
    ll = float(
        -np.mean(y_arr * np.log(p_clipped) + (1.0 - y_arr) * np.log(1.0 - p_clipped))
    )

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
        "matches": n,
        "brier": round(brier, 4),
        "ece": round(ece, 4),
        "ece_pct": round(ece * 100.0, 2),
        "log_loss": round(ll, 4),
        "accuracy": round(accuracy * 100.0, 2),
        "bin_data": {
            "labels": BIN_LABELS,
            "ideal": IDEAL_POINTS,
            "actuals": bin_actuals,
            "preds": bin_preds,
            "counts": bin_counts,
        },
    }


def run_walk_forward_evaluation(
    player_count: int = 0,
    start_eval_month: str = "2022-01",
    verbose: bool = True,
    models_filter: list[str] | None = None,
    *,
    conn: psycopg.Connection,
    run_id: int,
    on_month: Callable[[str], None] | None = None,
) -> dict[str, Any]:
    """Executes a monthly expanding-window walk-forward evaluation across the run's models.

    Each month is predicted from the games before it alone, retro priors and
    thresholds included. Reads the run's pairs and replaces the format's
    results in the run, in one committed transaction. Calls on_month with each
    evaluated 'YYYY-MM' when it is done. Raises RunNotRunning before any work
    unless the run is running.
    """
    with conn.transaction():
        require_running(conn, run_id)
        fmt_label = f"{player_count}p" if player_count > 0 else "All Formats"
        if verbose:
            print("\n=======================================================")
            print(
                f"Starting Walk-Forward Calibration: {fmt_label} (Start: {start_eval_month})"
            )
            print("=======================================================")

        # Load chronological matches
        rows = load_pairs(conn, run_id, player_count)

        if verbose:
            print(f"Loaded {len(rows)} eligible pairwise matches.")

        # Group matches by calendar month YYYY-MM
        month_matches = defaultdict(list)
        for r in rows:
            m = r["date"][:7]
            month_matches[m].append(r)

        sorted_months = sorted(month_matches.keys())
        eval_months = [m for m in sorted_months if m >= start_eval_month]
        burn_in_months = [m for m in sorted_months if m < start_eval_month]

        if verbose:
            print(
                f"Burn-in period: {len(burn_in_months)} months ({burn_in_months[0]} to {burn_in_months[-1]})"
            )
            print(
                f"Evaluation period: {len(eval_months)} months ({eval_months[0]} to {eval_months[-1]})"
            )

        if models_filter:
            models = [m for m in ALL_MODELS if m in models_filter]
        else:
            models = ALL_MODELS

        g2_models = list(models)

        # Every month is predicted from the months before it alone: the
        # thresholds, the retro families' emergent priors (pass 1) and the
        # models' ratings (pass 2) all advance one month at a time, over
        # the games seen so far.
        player_thresholds = PlayerThresholds(standard_threshold=15, inactivity_days=365)

        families_retro = {get_fam(m) for m in g2_models if is_retro(m)}
        calibrated_priors: dict[str, dict[str, Rating]] = {
            fam: {} for fam in families_retro
        }
        ratings_p1 = {
            fam: defaultdict(lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA))
            for fam in families_retro
        }
        games_p1 = {fam: defaultdict(set) for fam in families_retro}

        g2_ratings: dict[str, dict[str, Rating]] = {}
        for m in g2_models:
            if is_retro(m):
                g2_ratings[m] = {}
            else:
                g2_ratings[m] = defaultdict(
                    lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA)
                )
        games_played_p2 = {m: defaultdict(set) for m in g2_models}

        def weight_of(fam, r, p_cnt):
            if fam == "daneo":
                return GOLDEN_MP_WEIGHTS.get(p_cnt, 1.0)
            if fam == "mp":
                return r["weight"]
            return 1.0

        def run_up(fam, r_cur, items):
            enc3 = [(it[0], it[1], it[2]) for it in items]
            return update_rating(
                r_cur, enc3, tau=CALIBRATED_TAU, weighted=fam in ("daneo", "mp")
            )

        def drift_inactive(r_map, active):
            for p in list(r_map.keys()):
                if p not in active and r_map[p].rd < 350.0:
                    r_cur = r_map[p]
                    phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2) ** 0.5
                    r_map[p] = Rating(
                        r_cur.rating, min(350.0, phi_new * G2_SCALE), r_cur.sigma
                    )

        def encounters_of(m_rows, fam, r_map, retro):
            enc = defaultdict(list)
            for r in m_rows:
                pa, pb, out = r["player_a"], r["player_b"], r["outcome_a"]
                mid = r["match_id"] if "match_id" in r else (r["date"], pa, pb)
                p_cnt = r.get("player_count", player_count)
                ra = r_map.get(pa, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if retro else r_map[pa]  # fmt: skip
                rb = r_map.get(pb, Rating(1500.0, 350.0, CALIBRATED_SIGMA)) if retro else r_map[pb]  # fmt: skip
                w = weight_of(fam, r, p_cnt)
                enc[pa].append((rb, out, w, p_cnt, mid))
                enc[pb].append((ra, 1.0 - out, w, p_cnt, mid))
            return enc

        def count_games(gp, p_name, enc_list):
            """Adds the player's new games to gp; returns (games before, games after, the new ids)."""
            prev_g = len(gp[p_name])
            distinct_new = []
            for it in enc_list:
                if it[4] not in gp[p_name]:
                    gp[p_name].add(it[4])
                    distinct_new.append(it[4])
            return prev_g, len(gp[p_name]), distinct_new

        def discover_priors(m_rows):
            """Pass 1 for one month: a retro family's prior is a player's rating at their threshold game."""
            for fam in families_retro:
                r_map, gp, priors = (
                    ratings_p1[fam],
                    games_p1[fam],
                    calibrated_priors[fam],
                )
                enc = encounters_of(m_rows, fam, r_map, False)
                for p_name, enc_list in enc.items():
                    prev_g, tot_g, distinct_new = count_games(gp, p_name, enc_list)
                    p_thresh = player_thresholds.get(p_name, 15)
                    if prev_g < p_thresh <= tot_g and p_name not in priors:
                        mids_up = set(distinct_new[: (p_thresh - prev_g)])
                        enc_up = [it for it in enc_list if it[4] in mids_up]
                        enc_after = [it for it in enc_list if it[4] not in mids_up]
                        r_at_k = run_up(fam, r_map[p_name], enc_up)
                        priors[p_name] = Rating(r_at_k.rating, r_at_k.rd, r_at_k.sigma)
                        r_map[p_name] = (
                            run_up(fam, r_at_k, enc_after) if enc_after else r_at_k
                        )
                    else:
                        r_map[p_name] = run_up(fam, r_map[p_name], enc_list)
                drift_inactive(r_map, set(enc.keys()))

        def update_month(m_rows):
            """Pass 2 for one month: each model's ratings after the month's games.

            A retro model rates a player as the standard model does until the
            family's prior for them is found, which stands in for the games up
            to the threshold from then on.
            """
            for m_key in g2_models:
                r_map, fam, retro = g2_ratings[m_key], get_fam(m_key), is_retro(m_key)
                gp = games_played_p2[m_key]
                enc = encounters_of(m_rows, fam, r_map, retro)
                for p_name, enc_list in enc.items():
                    if not retro:
                        r_map[p_name] = run_up(fam, r_map[p_name], enc_list)
                        continue
                    prev_g, tot_g, distinct_new = count_games(gp, p_name, enc_list)
                    cur_r = r_map.get(p_name, Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                    p_thresh = player_thresholds.get(p_name, 15)
                    prior = calibrated_priors[fam].get(p_name)
                    if prior is not None and prev_g < p_thresh <= tot_g:
                        mids_up = set(distinct_new[: (p_thresh - prev_g)])
                        enc_after = [it for it in enc_list if it[4] not in mids_up]
                        cur_r = Rating(prior.rating, prior.rd, prior.sigma)
                        r_map[p_name] = run_up(fam, cur_r, enc_after) if enc_after else cur_r  # fmt: skip
                    else:
                        r_map[p_name] = run_up(fam, cur_r, enc_list)
                drift_inactive(r_map, set(enc.keys()))

        def advance_month(m):
            """Takes in month m's games: thresholds, then pass 1, then pass 2."""
            m_rows = month_matches[m]
            player_thresholds.add(m_rows)
            discover_priors(m_rows)
            update_month(m_rows)

        def apply_annual_resets(prev_y: str, cur_y: str):
            if prev_y is not None and cur_y != prev_y:
                for m_key in g2_models:
                    if "_softer" in m_key:
                        r_map = g2_ratings[m_key]
                        act_players = [
                            r
                            for p, r in r_map.items()
                            if (
                                len(games_played_p2[m_key][p])
                                >= player_thresholds.get(p, 15)
                                if is_retro(m_key)
                                else True
                            )
                            and r.rd < 350.0
                        ]
                        if act_players:
                            pop_mu = sum(r.rating for r in act_players) / len(
                                act_players
                            )
                            pop_sigma = max(
                                50.0,
                                (
                                    sum((r.rating - pop_mu) ** 2 for r in act_players)
                                    / len(act_players)
                                )
                                ** 0.5,
                            )
                            pop_rd_mean = sum(r.rd for r in act_players) / len(
                                act_players
                            )
                            pop_rd_min = max(20.0, min(r.rd for r in act_players))
                        else:
                            pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = (
                                1500.0,
                                175.0,
                                75.0,
                                30.0,
                            )

                        for p_name in list(r_map.keys()):
                            is_elig = (
                                (
                                    len(games_played_p2[m_key][p_name])
                                    >= player_thresholds.get(p_name, 15)
                                )
                                if is_retro(m_key)
                                else True
                            )
                            if is_elig and r_map[p_name].rd < 350.0:
                                tm = get_tier_mean(
                                    r_map[p_name].rating,
                                    mean_rating=pop_mu,
                                    sigma=pop_sigma,
                                )
                                eff_alpha, eff_lambda = compute_decoupled_scaling(
                                    r_map[p_name].rating,
                                    r_map[p_name].rd,
                                    mean_rating=pop_mu,
                                    sigma_pop=pop_sigma,
                                    rd_mean=pop_rd_mean,
                                    rd_min=pop_rd_min,
                                    base_alpha=RESET_ALPHA_SOFTER,
                                    base_lambda=RESET_LAMBDA_SOFTER,
                                )
                                new_rd = min(350.0, r_map[p_name].rd * eff_alpha)
                                new_r = (1.0 - eff_lambda) * r_map[
                                    p_name
                                ].rating + eff_lambda * tm
                                r_map[p_name] = Rating(
                                    new_r, new_rd, r_map[p_name].sigma
                                )

        # Execute Burn-in Phase
        if verbose:
            print("Processing burn-in phase...")

        prev_year = None
        for m in burn_in_months:
            cur_year = m[:4]
            apply_annual_resets(prev_year, cur_year)
            prev_year = cur_year
            advance_month(m)

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
                pa, pb, out = r["player_a"], r["player_b"], r["outcome_a"]

                # Glicko-2 models predictions
                for m_key in g2_models:
                    r_map = g2_ratings[m_key]
                    ra = (
                        r_map.get(pa, Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                        if is_retro(m_key)
                        else r_map[pa]
                    )
                    rb = (
                        r_map.get(pb, Rating(1500.0, 350.0, CALIBRATED_SIGMA))
                        if is_retro(m_key)
                        else r_map[pb]
                    )

                    c = math.sqrt(ra.phi**2 + rb.phi**2)
                    g_rd = 1.0 / math.sqrt(1.0 + 3.0 * (c**2) / (math.pi**2))
                    p_val = 1.0 / (1.0 + math.exp(-g_rd * (ra.mu - rb.mu)))

                    if p_val >= 0.5:
                        month_preds[m_key].append(p_val)
                        month_actuals[m_key].append(out)
                    else:
                        month_preds[m_key].append(1.0 - p_val)
                        month_actuals[m_key].append(1.0 - out)

            # Record monthly metrics
            for m in models:
                res_m = compute_metrics_from_preds(month_preds[m], month_actuals[m])
                monthly_results[tm][m] = res_m
                overall_preds[m].extend(month_preds[m])
                overall_actuals[m].extend(month_actuals[m])

            if verbose and (idx % 6 == 0 or idx == len(eval_months) - 1):
                stat_parts = []
                for sample_m in ALL_MODELS:
                    if sample_m in monthly_results[tm]:
                        stat_parts.append(
                            f"{sample_m}: {monthly_results[tm][sample_m]['brier']:.4f}"
                        )
                print(
                    f"[{idx + 1}/{len(eval_months)}] Month {tm} ({len(m_matches):5d} games) | Brier -> "
                    + ", ".join(stat_parts)
                )

            # (B) Incremental Online Update for month tm matches
            advance_month(tm)

            if on_month:
                on_month(tm)

        # 3. Overall Aggregate Metrics
        aggregated_results = {}
        for m in models:
            aggregated_results[m] = compute_metrics_from_preds(
                overall_preds[m], overall_actuals[m]
            )

        if verbose:
            print(f"\n--- Walk-Forward Summary ({fmt_label}) ---")
            for m in models:
                agg = aggregated_results[m]
                print(
                    f"Model: {m:28s} | Out-of-Sample Brier: {agg['brier']:.4f} | ECE: {agg['ece_pct']:.2f}% | LogLoss: {agg['log_loss']:.4f} | Acc: {agg['accuracy']:.1f}%"
                )

        # 4. Save to Database
        save_models = [
            m for m in models if (models_filter is None or m in models_filter)
        ]
        # Clear previous entries for this player_count format
        if models_filter:
            conn.execute(
                "DELETE FROM walk_forward_calibration WHERE run_id = %s AND player_count = %s AND model_type = ANY(%s)",
                (run_id, player_count, list(models_filter)),
            )
        else:
            conn.execute(
                "DELETE FROM walk_forward_calibration WHERE run_id = %s AND player_count = %s",
                (run_id, player_count),
            )

        # Aggregated summary records, then individual monthly records
        records = [(m, "AGGREGATED", aggregated_results[m]) for m in save_models]
        records += [
            (m, tm, m_data[m])
            for tm, m_data in monthly_results.items()
            for m in save_models
        ]
        with conn.cursor() as cur:
            cur.executemany(
                """
                INSERT INTO walk_forward_calibration
                (run_id, model_type, player_count, period_month, matches_evaluated, brier_score, ece, log_loss, accuracy, bin_data)
                VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)
            """,
                [
                    (
                        run_id,
                        m,
                        player_count,
                        tm,
                        item["matches"],
                        item["brier"],
                        item["ece"],
                        item["log_loss"],
                        item["accuracy"],
                        Jsonb(item["bin_data"]),
                    )
                    for m, tm, item in records
                ],
            )

    if verbose:
        print(f"Saved walk-forward results to database for format {fmt_label}.")

    return {
        "player_count": player_count,
        "eval_months": eval_months,
        "aggregated": aggregated_results,
        "monthly": monthly_results,
    }


def run_walk_forward_all(
    start_eval_month: str = "2022-01",
    verbose: bool = True,
    models_filter: list[str] | None = None,
    formats: list[int] | None = None,
    *,
    conn: psycopg.Connection,
    run_id: int,
) -> dict[int, Any]:
    """Runs walk-forward evaluation across formats, committing each format's results into the run."""
    all_res = {}
    if formats is None:
        formats = [0, 2, 3, 4]
    for fmt in formats:
        all_res[fmt] = run_walk_forward_evaluation(
            player_count=fmt,
            start_eval_month=start_eval_month,
            verbose=verbose,
            models_filter=models_filter,
            conn=conn,
            run_id=run_id,
        )
    return all_res
