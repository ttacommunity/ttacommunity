"""Standard (in-sample match-time) calibration and predictive benchmark engine.

Evaluates match-by-match predicted win probability versus actual game outcomes
for the models a run computes (Glicko-D continuous and with a season reset, and
standard Glicko-2) across formats (All, 2P, 3P, 4P).
"""

import math
from collections import defaultdict
from typing import Any

import psycopg
from psycopg.types.json import Jsonb

from tta_ratings_engine.data.db import load_pairs, require_running
from tta_ratings_engine.models.evaluation.walk_forward import (
    compute_metrics_from_preds,
)
from tta_ratings_engine.models.glicko2.calculator import (
    CALIBRATED_SIGMA,
    CALIBRATED_TAU,
    CALIBRATION_THRESHOLD,
    GOLDEN_MP_WEIGHTS,
    INACTIVITY_DRIFT_PER_MONTH,
    MODELS,
    RESET_ALPHA_SOFTER,
    RESET_LAMBDA_SOFTER,
    compute_decoupled_scaling,
    compute_player_thresholds,
    get_tier_mean,
    model_name,
)
from tta_ratings_engine.models.glicko2.engine import GLICKO2_SCALE as G2_SCALE
from tta_ratings_engine.models.glicko2.engine import Rating, update_rating


def evaluate_standard_glicko2(
    engine: str = "glicko2_std",
    reset_mode: str = "continuous",
    player_count: int = 0,
    *,
    conn: psycopg.Connection,
    run_id: int,
) -> dict[str, Any]:
    """Runs sequential step-by-step match-time prediction evaluation for Glicko-2 on the run's pairs.

    engine and reset_mode are as for compute_glicko2_ratings. Raises ValueError for an unknown either.
    """
    model_name(engine, reset_mode)
    retro_calibrated = weighted = engine == "glicko2_daneo"

    with conn.transaction():
        rows = load_pairs(conn, run_id, player_count)

        periods = defaultdict(list)
        for r in rows:
            m_key = r["date"][:7]
            periods[m_key].append(r)

        sorted_months = sorted(periods.keys())

        # -----------------------------------------------------------------
        # PASS 1 (If retro_calibrated): Discover emergent priors at threshold
        # -----------------------------------------------------------------
        calibrated_priors: dict[str, Rating] = {}
        player_thresholds: dict[str, int] = {}
        if retro_calibrated:
            player_thresholds = compute_player_thresholds(
                rows, standard_threshold=CALIBRATION_THRESHOLD
            )
            ratings_p1: dict[str, Rating] = defaultdict(
                lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA)
            )
            games_played_p1 = defaultdict(set)
            prev_year_p1 = None

            for m_key in sorted_months:
                cur_year = m_key[:4]
                if (
                    prev_year_p1 is not None
                    and cur_year != prev_year_p1
                    and reset_mode != "continuous"
                ):
                    act_p1 = [r for r in ratings_p1.values() if r.rd < 350.0]
                    if act_p1:
                        pop_mu = sum(r.rating for r in act_p1) / len(act_p1)
                        pop_sigma = max(
                            50.0,
                            (
                                sum((r.rating - pop_mu) ** 2 for r in act_p1)
                                / len(act_p1)
                            )
                            ** 0.5,
                        )
                        pop_rd_mean = sum(r.rd for r in act_p1) / len(act_p1)
                        pop_rd_min = max(20.0, min(r.rd for r in act_p1))
                    else:
                        pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = (
                            1500.0,
                            175.0,
                            75.0,
                            30.0,
                        )

                    for p_name, r_cur in list(ratings_p1.items()):
                        if r_cur.rd < 350.0:
                            tm = get_tier_mean(
                                r_cur.rating, mean_rating=pop_mu, sigma=pop_sigma
                            )
                            eff_alpha, eff_lambda = compute_decoupled_scaling(
                                r_cur.rating,
                                r_cur.rd,
                                mean_rating=pop_mu,
                                sigma_pop=pop_sigma,
                                rd_mean=pop_rd_mean,
                                rd_min=pop_rd_min,
                                base_alpha=RESET_ALPHA_SOFTER,
                                base_lambda=RESET_LAMBDA_SOFTER,
                            )
                            new_rd = min(350.0, r_cur.rd * eff_alpha)
                            new_r = (1.0 - eff_lambda) * r_cur.rating + eff_lambda * tm
                            ratings_p1[p_name] = Rating(new_r, new_rd, r_cur.sigma)
                prev_year_p1 = cur_year

                p1_matches = periods[m_key]
                p1_encounters = defaultdict(list)
                for m in p1_matches:
                    pa, pb = m["player_a"], m["player_b"]
                    mid = m["match_id"] if "match_id" in m else (m["date"], pa, pb)
                    out_a = m["outcome_a"]
                    m_pcount = m.get("player_count", player_count)

                    w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0) if weighted else 1.0

                    p1_encounters[pa].append((ratings_p1[pb], out_a, w, m_pcount, mid))
                    p1_encounters[pb].append(
                        (ratings_p1[pa], 1.0 - out_a, w, m_pcount, mid)
                    )

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
                    p_thresh = player_thresholds.get(p_name, CALIBRATION_THRESHOLD)

                    def run_update_p1(r_cur, items):
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(
                            r_cur,
                            enc3,
                            tau=CALIBRATED_TAU,
                            weighted=weighted,
                        )

                    if (
                        prev_games < p_thresh
                        and total_games >= p_thresh
                        and p_name not in calibrated_priors
                    ):
                        needed = p_thresh - prev_games
                        mids_up_to_thresh = set(distinct_new[:needed])
                        enc_up_to_thresh = [
                            it for it in enc_list if it[4] in mids_up_to_thresh
                        ]
                        enc_after_thresh = [
                            it for it in enc_list if it[4] not in mids_up_to_thresh
                        ]

                        r_at_thresh = run_update_p1(
                            ratings_p1[p_name], enc_up_to_thresh
                        )
                        calibrated_priors[p_name] = Rating(
                            r_at_thresh.rating, r_at_thresh.rd, r_at_thresh.sigma
                        )
                        if enc_after_thresh:
                            ratings_p1[p_name] = run_update_p1(
                                r_at_thresh, enc_after_thresh
                            )
                        else:
                            ratings_p1[p_name] = r_at_thresh
                    else:
                        ratings_p1[p_name] = run_update_p1(ratings_p1[p_name], enc_list)

                for p_name in list(ratings_p1.keys()):
                    p_thresh = player_thresholds.get(p_name, CALIBRATION_THRESHOLD)
                    if (
                        p_name not in p1_active
                        and ratings_p1[p_name].rd < 350.0
                        and len(games_played_p1[p_name]) >= p_thresh
                    ):
                        r_cur = ratings_p1[p_name]
                        phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2) ** 0.5
                        new_rd = min(350.0, phi_new * G2_SCALE)
                        ratings_p1[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        # -----------------------------------------------------------------
        # PASS 2 (or Standard Pass): Chronological Evaluation with Option A* Freezing
        # -----------------------------------------------------------------
        ratings: dict[str, Rating] = {}
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
            if (
                prev_year is not None
                and cur_year != prev_year
                and reset_mode != "continuous"
            ):
                act_p2 = [
                    r
                    for p, r in ratings.items()
                    if (
                        len(games_played_p2[p])
                        >= player_thresholds.get(p, CALIBRATION_THRESHOLD)
                        if retro_calibrated
                        else True
                    )
                    and r.rd < 350.0
                ]
                if act_p2:
                    pop_mu = sum(r.rating for r in act_p2) / len(act_p2)
                    pop_sigma = max(
                        50.0,
                        (sum((r.rating - pop_mu) ** 2 for r in act_p2) / len(act_p2))
                        ** 0.5,
                    )
                    pop_rd_mean = sum(r.rd for r in act_p2) / len(act_p2)
                    pop_rd_min = max(20.0, min(r.rd for r in act_p2))
                else:
                    pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = (
                        1500.0,
                        175.0,
                        75.0,
                        30.0,
                    )

                for p_name in list(ratings.keys()):
                    is_eligible = (
                        (
                            len(games_played_p2[p_name])
                            >= player_thresholds.get(p_name, CALIBRATION_THRESHOLD)
                        )
                        if retro_calibrated
                        else True
                    )
                    if is_eligible and ratings[p_name].rd < 350.0:
                        tm = get_tier_mean(
                            ratings[p_name].rating, mean_rating=pop_mu, sigma=pop_sigma
                        )
                        eff_alpha, eff_lambda = compute_decoupled_scaling(
                            ratings[p_name].rating,
                            ratings[p_name].rd,
                            mean_rating=pop_mu,
                            sigma_pop=pop_sigma,
                            rd_mean=pop_rd_mean,
                            rd_min=pop_rd_min,
                            base_alpha=RESET_ALPHA_SOFTER,
                            base_lambda=RESET_LAMBDA_SOFTER,
                        )
                        new_rd = min(350.0, ratings[p_name].rd * eff_alpha)
                        new_r = (1.0 - eff_lambda) * ratings[
                            p_name
                        ].rating + eff_lambda * tm
                        ratings[p_name] = Rating(new_r, new_rd, ratings[p_name].sigma)

            prev_year = cur_year
            month_matches = periods[m_key]

            # First: predict each match in the month using pre-match state
            for m in month_matches:
                pa, pb = m["player_a"], m["player_b"]
                out_a = m["outcome_a"]
                ra = get_current_rating(pa)
                rb = get_current_rating(pb)
                m_pcount = m.get("player_count", player_count)

                c = math.sqrt(ra.phi**2 + rb.phi**2)
                g_rd = 1.0 / math.sqrt(1.0 + 3.0 * (c**2) / (math.pi**2))
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
                pa, pb = m["player_a"], m["player_b"]
                mid = m["match_id"] if "match_id" in m else (m["date"], pa, pb)
                out_a = m["outcome_a"]
                m_pcount = m.get("player_count", player_count)

                w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0) if weighted else 1.0

                ra = get_current_rating(pa)
                rb = get_current_rating(pb)

                player_encounters[pa].append((rb, out_a, w, m_pcount, mid))
                player_encounters[pb].append((ra, 1.0 - out_a, w, m_pcount, mid))

            active_players = set(player_encounters.keys())
            for p_name, enc_list in player_encounters.items():

                def run_update_p2(r_cur, items):
                    enc3 = [(it[0], it[1], it[2]) for it in items]
                    return update_rating(
                        r_cur,
                        enc3,
                        tau=CALIBRATED_TAU,
                        weighted=weighted,
                    )

                if retro_calibrated:
                    prev_games = len(games_played_p2[p_name])
                    distinct_new = []
                    for it in enc_list:
                        mid = it[4]
                        if mid not in games_played_p2[p_name]:
                            games_played_p2[p_name].add(mid)
                            distinct_new.append(mid)
                    total_games = len(games_played_p2[p_name])
                    p_thresh = player_thresholds.get(p_name, CALIBRATION_THRESHOLD)

                    if p_name not in calibrated_priors:
                        ratings[p_name] = run_update_p2(
                            get_current_rating(p_name), enc_list
                        )
                    elif total_games <= p_thresh:
                        # Option A: Frozen during initial calibration window (1..p_thresh games)
                        pass
                    elif prev_games < p_thresh and total_games > p_thresh:
                        # Surgical Split in transition month: matches up to threshold are frozen; only matches after threshold update
                        needed = p_thresh - prev_games
                        mids_up_to_thresh = set(distinct_new[:needed])
                        enc_after_thresh = [
                            it for it in enc_list if it[4] not in mids_up_to_thresh
                        ]
                        if enc_after_thresh:
                            ratings[p_name] = run_update_p2(
                                get_current_rating(p_name), enc_after_thresh
                            )
                    else:
                        ratings[p_name] = run_update_p2(
                            get_current_rating(p_name), enc_list
                        )
                else:
                    ratings[p_name] = run_update_p2(
                        get_current_rating(p_name), enc_list
                    )

            # Apply mild inactivity drift to already-active players who missed this month
            for p_name in list(ratings.keys()):
                p_thresh = player_thresholds.get(p_name, CALIBRATION_THRESHOLD)
                is_active_or_uncalibrated = (p_name in active_players) or (
                    retro_calibrated and len(games_played_p2[p_name]) < p_thresh
                )
                if not is_active_or_uncalibrated and ratings[p_name].rd < 350.0:
                    r_cur = ratings[p_name]
                    phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2) ** 0.5
                    new_rd = min(350.0, phi_new * G2_SCALE)
                    ratings[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        return compute_metrics_from_preds(preds, actuals)


def run_standard_calibration_all(
    verbose: bool = True,
    model_filter: list[str] | None = None,
    formats: list[int] | None = None,
    *,
    conn: psycopg.Connection,
    run_id: int,
) -> dict[str, Any]:
    """Computes and saves standard calibration metrics for all model configurations across formats.

    Writes into the run in one committed transaction. Raises RunNotRunning
    before any work unless the run is running.
    """
    all_results = {}

    if formats is None:
        formats = [0, 2, 3, 4]

    with conn.transaction():
        require_running(conn, run_id)
        for fmt in formats:
            if fmt not in all_results:
                all_results[fmt] = {}
            fmt_label = f"{fmt}P" if fmt > 0 else "All"
            if verbose:
                print(f"\n--- Standard Calibration: Format {fmt_label} ---")

            for m_type, (engine, reset_mode) in MODELS.items():
                if model_filter and m_type not in model_filter:
                    continue
                if verbose:
                    print(f"  Evaluating {m_type} ({fmt_label})...")

                res = evaluate_standard_glicko2(
                    engine=engine,
                    reset_mode=reset_mode,
                    player_count=fmt,
                    conn=conn,
                    run_id=run_id,
                )

                all_results[fmt][m_type] = res

                conn.execute(
                    """
                    INSERT INTO standard_calibration (
                        run_id, model_type, player_count, period_month, matches_evaluated,
                        brier_score, ece, log_loss, accuracy, bin_data
                    ) VALUES (%s, %s, %s, 'AGGREGATED', %s, %s, %s, %s, %s, %s)
                    ON CONFLICT (run_id, model_type, player_count, period_month) DO UPDATE SET
                        matches_evaluated = excluded.matches_evaluated,
                        brier_score = excluded.brier_score,
                        ece = excluded.ece,
                        log_loss = excluded.log_loss,
                        accuracy = excluded.accuracy,
                        bin_data = excluded.bin_data
                """,
                    (
                        run_id,
                        m_type,
                        fmt,
                        res["matches"],
                        res["brier"],
                        res["ece"],
                        res["log_loss"],
                        res["accuracy"],
                        Jsonb(res["bin_data"]),
                    ),
                )

                if verbose:
                    print(
                        f"    -> Brier: {res['brier']:.4f}, ECE: {res['ece_pct']:.2f}%, Accuracy: {res['accuracy']:.1f}%"
                    )

    return all_results
