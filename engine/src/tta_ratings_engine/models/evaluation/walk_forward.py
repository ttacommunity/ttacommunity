"""Walk-forward (leak-free out-of-sample t+1) calibration and predictive benchmark engine."""

import functools
import math
from collections import defaultdict
from collections.abc import Callable
from typing import Any

import numpy as np
import psycopg
from psycopg.types.json import Jsonb

from tta_ratings_engine.data.db import load_pairs, require_running
from tta_ratings_engine.models.glicko2.calculator import (
    MODELS,
    PlayerThresholds,
    by_month,
    count_new_games,
    drifted,
    month_encounters,
    new_rating,
    new_year,
    rate,
    season_reset,
    split_at_threshold,
)
from tta_ratings_engine.models.glicko2.engine import Rating

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


def is_retro(m: str) -> bool:
    """Returns True if model m (a MODELS name) uses Option A 15-game emergent prior calibration."""
    return get_fam(m) == "glicko2_daneo"


def get_fam(m: str) -> str:
    """Returns the engine of model m (a MODELS name)."""
    return MODELS[m][0]


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


def favourite(ra: Rating, rb: Rating, score_a: float) -> tuple[float, float]:
    """Returns the favourite's expected score in a game of ra against rb (at least 0.5), and the favourite's score."""
    c = math.sqrt(ra.phi**2 + rb.phi**2)
    g_rd = 1.0 / math.sqrt(1.0 + 3.0 * (c**2) / (math.pi**2))
    exp_prob = 1.0 / (1.0 + math.exp(-g_rd * (ra.mu - rb.mu)))
    if exp_prob >= 0.5:
        return exp_prob, score_a
    return 1.0 - exp_prob, 1.0 - score_a


class _Models:
    """The models as a walk forward advances: thresholds, the retro families' priors and each model's ratings.

    Each advances one month at a time, over the games seen so far.
    """

    def __init__(self, models: list[str], player_count: int):
        self.models = models
        self.player_count = player_count
        self.thresholds = PlayerThresholds(standard_threshold=15, inactivity_days=365)
        families_retro = {get_fam(m) for m in models if is_retro(m)}
        self.priors: dict[str, dict[str, Rating]] = {f: {} for f in families_retro}
        self.ratings_p1 = {f: defaultdict(new_rating) for f in families_retro}
        self.games_p1: dict[str, dict[str, set[Any]]] = {
            f: defaultdict(set) for f in families_retro
        }
        self.ratings: dict[str, dict[str, Rating]] = {
            m: {} if is_retro(m) else defaultdict(new_rating) for m in models
        }
        self.games: dict[str, dict[str, set[Any]]] = {
            m: defaultdict(set) for m in models
        }

    def rating(self, m: str, p_name: str) -> Rating:
        """Returns model m's rating of a player; a standard model adds a new player, a retro one only reads."""
        r_map = self.ratings[m]
        return r_map.get(p_name, new_rating()) if is_retro(m) else r_map[p_name]

    def predict(self, rows: list[Any]) -> dict[str, tuple[list[float], list[float]]]:
        """Returns each model's predictions of rows and the favourites' scores, from the ratings so far."""
        out: dict[str, tuple[list[float], list[float]]] = {
            m: ([], []) for m in self.models
        }
        for r in rows:
            for m in self.models:
                ra = self.rating(m, r["player_a"])
                rb = self.rating(m, r["player_b"])
                p, actual = favourite(ra, rb, r["outcome_a"])
                out[m][0].append(p)
                out[m][1].append(actual)
        return out

    def season_reset(self) -> None:
        """Resets the season-reset models; a retro model resets only calibrated players."""
        for m in self.models:
            if MODELS[m][1] == "softer":
                season_reset(self.ratings[m], functools.partial(self._resets, m))

    def _resets(self, m: str, p_name: str) -> bool:
        if not is_retro(m):
            return True
        return len(self.games[m][p_name]) >= self.thresholds.get(p_name, 15)

    def advance(self, rows: list[Any]) -> None:
        """Takes in a month's games: thresholds, then pass 1, then pass 2."""
        self.thresholds.add(rows)
        self._discover_priors(rows)
        self._update(rows)

    def _discover_priors(self, rows: list[Any]) -> None:
        """Pass 1 for one month: a retro family's prior is a player's rating at their threshold game."""
        for fam in self.priors:
            r_map, gp, priors = (
                self.ratings_p1[fam],
                self.games_p1[fam],
                self.priors[fam],
            )
            weighted = fam == "glicko2_daneo"
            enc = month_encounters(rows, r_map.__getitem__, weighted, self.player_count)
            for p_name, enc_list in enc.items():
                prev_g, tot_g, distinct_new = count_new_games(gp[p_name], enc_list)
                p_thresh = self.thresholds.get(p_name, 15)
                if prev_g < p_thresh <= tot_g and p_name not in priors:
                    enc_up, enc_after = split_at_threshold(
                        enc_list, distinct_new, p_thresh - prev_g
                    )
                    r_at_k = rate(r_map[p_name], enc_up, weighted)
                    priors[p_name] = Rating(r_at_k.rating, r_at_k.rd, r_at_k.sigma)
                    r_map[p_name] = rate(r_at_k, enc_after, weighted) if enc_after else r_at_k  # fmt: skip
                else:
                    r_map[p_name] = rate(r_map[p_name], enc_list, weighted)
            _drift_inactive(r_map, enc)

    def _update(self, rows: list[Any]) -> None:
        """Pass 2 for one month: each model's ratings after the month's games.

        A retro model rates a player as the standard model does until the
        family's prior for them is found, which stands in for the games up
        to the threshold from then on.
        """
        for m in self.models:
            r_map, fam, retro = self.ratings[m], get_fam(m), is_retro(m)
            weighted = fam == "glicko2_daneo"
            rating_of = functools.partial(self.rating, m)
            enc = month_encounters(rows, rating_of, weighted, self.player_count)
            for p_name, enc_list in enc.items():
                if not retro:
                    r_map[p_name] = rate(r_map[p_name], enc_list, weighted)
                    continue
                prev_g, tot_g, distinct_new = count_new_games(
                    self.games[m][p_name], enc_list
                )
                cur_r = r_map.get(p_name, new_rating())
                p_thresh = self.thresholds.get(p_name, 15)
                prior = self.priors[fam].get(p_name)
                if prior is not None and prev_g < p_thresh <= tot_g:
                    _, enc_after = split_at_threshold(
                        enc_list, distinct_new, p_thresh - prev_g
                    )
                    cur_r = Rating(prior.rating, prior.rd, prior.sigma)
                    r_map[p_name] = rate(cur_r, enc_after, weighted) if enc_after else cur_r  # fmt: skip
                else:
                    r_map[p_name] = rate(cur_r, enc_list, weighted)
            _drift_inactive(r_map, enc)


def _drift_inactive(r_map: dict[str, Rating], active: dict[str, Any]) -> None:
    """Drifts, in place, every player in r_map with an RD under 350 who is not in active."""
    for p in list(r_map.keys()):
        if p not in active and r_map[p].rd < 350.0:
            r_map[p] = drifted(r_map[p])


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

        rows = load_pairs(conn, run_id, player_count)
        if verbose:
            print(f"Loaded {len(rows)} eligible pairwise matches.")

        month_matches = dict(by_month(rows))
        eval_months = [m for m in month_matches if m >= start_eval_month]
        burn_in_months = [m for m in month_matches if m < start_eval_month]
        if verbose:
            print(
                f"Burn-in period: {len(burn_in_months)} months ({burn_in_months[0]} to {burn_in_months[-1]})"
            )
            print(
                f"Evaluation period: {len(eval_months)} months ({eval_months[0]} to {eval_months[-1]})"
            )

        if models_filter:
            models = [m for m in MODELS if m in models_filter]
        else:
            models = list(MODELS)
        state = _Models(models, player_count)

        if verbose:
            print("Processing burn-in phase...")
        previous = None
        for m in burn_in_months:
            if new_year(previous, m):
                state.season_reset()
            previous = m
            state.advance(month_matches[m])

        monthly_results: dict[str, dict[str, Any]] = defaultdict(dict)
        overall_preds: dict[str, list[float]] = {m: [] for m in models}
        overall_actuals: dict[str, list[float]] = {m: [] for m in models}
        for idx, tm in enumerate(eval_months):
            if new_year(previous, tm):
                state.season_reset()
            previous = tm
            m_matches = month_matches[tm]
            if not m_matches:
                continue

            # (A) Pure Out-of-Sample Predictions strictly BEFORE observing matches in month tm
            predictions = state.predict(m_matches)
            for m in models:
                preds, actuals = predictions[m]
                monthly_results[tm][m] = compute_metrics_from_preds(preds, actuals)
                overall_preds[m].extend(preds)
                overall_actuals[m].extend(actuals)

            if verbose and (idx % 6 == 0 or idx == len(eval_months) - 1):
                stat_parts = [
                    f"{m}: {monthly_results[tm][m]['brier']:.4f}"
                    for m in models
                    if m in monthly_results[tm]
                ]
                print(
                    f"[{idx + 1}/{len(eval_months)}] Month {tm} ({len(m_matches):5d} games) | Brier -> "
                    + ", ".join(stat_parts)
                )

            # (B) Incremental Online Update for month tm matches
            state.advance(m_matches)
            if on_month:
                on_month(tm)

        aggregated_results = {
            m: compute_metrics_from_preds(overall_preds[m], overall_actuals[m])
            for m in models
        }
        if verbose:
            print(f"\n--- Walk-Forward Summary ({fmt_label}) ---")
            for m in models:
                agg = aggregated_results[m]
                print(
                    f"Model: {m:28s} | Out-of-Sample Brier: {agg['brier']:.4f} | ECE: {agg['ece_pct']:.2f}% | LogLoss: {agg['log_loss']:.4f} | Acc: {agg['accuracy']:.1f}%"
                )

        save_models = [
            m for m in models if (models_filter is None or m in models_filter)
        ]
        _save(conn, run_id, player_count, models_filter, save_models, aggregated_results, monthly_results)  # fmt: skip

    if verbose:
        print(f"Saved walk-forward results to database for format {fmt_label}.")

    return {
        "player_count": player_count,
        "eval_months": eval_months,
        "aggregated": aggregated_results,
        "monthly": monthly_results,
    }


def _save(
    conn: psycopg.Connection,
    run_id: int,
    player_count: int,
    models_filter: list[str] | None,
    save_models: list[str],
    aggregated_results: dict[str, Any],
    monthly_results: dict[str, dict[str, Any]],
) -> None:
    """Replaces the format's walk-forward rows in the run: aggregated, then month by month."""
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
