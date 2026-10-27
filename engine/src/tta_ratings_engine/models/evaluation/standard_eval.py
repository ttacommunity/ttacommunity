"""Standard (in-sample match-time) calibration and predictive benchmark engine.

Evaluates match-by-match predicted win probability versus actual game outcomes
for the models a run computes (Glicko-D continuous and with a season reset, and
standard Glicko-2) across formats (All, 2P, 3P, 4P).
"""

from collections import defaultdict
from typing import Any

import psycopg
from psycopg.types.json import Jsonb

from tta_ratings_engine.data.db import load_pairs, require_running
from tta_ratings_engine.models.evaluation.walk_forward import (
    compute_metrics_from_preds,
    favourite,
)
from tta_ratings_engine.models.glicko2.calculator import (
    CALIBRATION_THRESHOLD,
    MODELS,
    SecondPass,
    by_month,
    compute_player_thresholds,
    count_new_games,
    drifted,
    model_name,
    month_encounters,
    new_rating,
    new_year,
    rate,
    season_reset,
    split_at_threshold,
)
from tta_ratings_engine.models.glicko2.engine import Rating


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
    softer = reset_mode != "continuous"

    with conn.transaction():
        rows = load_pairs(conn, run_id, player_count)
        months = by_month(rows)

        thresholds: dict[str, int] = {}
        priors: dict[str, Rating] = {}
        if retro_calibrated:
            thresholds = compute_player_thresholds(
                rows, standard_threshold=CALIBRATION_THRESHOLD
            )
            priors = _one_pass_priors(months, thresholds, weighted, softer, player_count)  # fmt: skip

        model = SecondPass(priors, thresholds, weighted, retro_calibrated, player_count)  # fmt: skip
        preds = []
        actuals = []
        previous = None
        for month, matches in months:
            if softer and new_year(previous, month):
                model.season_reset()
            previous = month

            # First: predict each match in the month using pre-match state
            for m in matches:
                ra = model.rating(m["player_a"])
                rb = model.rating(m["player_b"])
                p, actual = favourite(ra, rb, m["outcome_a"])
                preds.append(p)
                actuals.append(actual)

            # Second: update ratings for this monthly batch
            model.rate_month(matches)

        return compute_metrics_from_preds(preds, actuals)


def _one_pass_priors(
    months: list[tuple[str, list[Any]]],
    thresholds: dict[str, int],
    weighted: bool,
    softer: bool,
    player_count: int,
) -> dict[str, Rating]:
    """Pass 1 in a single pass: each player's prior is their rating at their threshold game."""
    ratings_p1: dict[str, Rating] = defaultdict(new_rating)
    games: dict[str, set[Any]] = defaultdict(set)
    priors: dict[str, Rating] = {}
    previous = None
    for month, matches in months:
        if softer and new_year(previous, month):
            season_reset(ratings_p1, lambda p: True)
        previous = month

        encounters = month_encounters(
            matches, ratings_p1.__getitem__, weighted, player_count, b_first=True
        )
        for p_name, enc_list in encounters.items():
            prev_games, total_games, distinct_new = count_new_games(
                games[p_name], enc_list
            )
            p_thresh = thresholds.get(p_name, CALIBRATION_THRESHOLD)
            if prev_games < p_thresh <= total_games and p_name not in priors:
                enc_up, enc_after = split_at_threshold(
                    enc_list, distinct_new, p_thresh - prev_games
                )
                r_at = rate(ratings_p1[p_name], enc_up, weighted)
                priors[p_name] = Rating(r_at.rating, r_at.rd, r_at.sigma)
                ratings_p1[p_name] = rate(r_at, enc_after, weighted) if enc_after else r_at  # fmt: skip
            else:
                ratings_p1[p_name] = rate(ratings_p1[p_name], enc_list, weighted)

        for p_name in list(ratings_p1.keys()):
            p_thresh = thresholds.get(p_name, CALIBRATION_THRESHOLD)
            if (
                p_name not in encounters
                and ratings_p1[p_name].rd < 350.0
                and len(games[p_name]) >= p_thresh
            ):
                ratings_p1[p_name] = drifted(ratings_p1[p_name])
    return priors


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
