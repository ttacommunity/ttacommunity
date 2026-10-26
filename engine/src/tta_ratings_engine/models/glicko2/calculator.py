"""Glicko-2 calendar monthly batch calculator calibrated to TTA tournament cadence."""

import math
from collections import defaultdict
from datetime import UTC, date, datetime
from typing import Any

import psycopg

from tta_ratings_engine.data.db import load_pairs, save_model_results
from tta_ratings_engine.models.glicko2.engine import (
    GLICKO2_SCALE,
    Rating,
    update_rating,
)

# Calibrated constants matching official TTA online implementation
CALIBRATED_TAU = 0.3
CALIBRATED_SIGMA = 0.03
INACTIVITY_DRIFT_PER_MONTH = 0.01  # gentle variance growth per missed month (~1.7 RD)

PHI = 1.61803398875

# DANeo Golden Ratio Multiplayer Information Decomposition Weights:
# W(N) = phi^(N-2) effective 1v1 equivalents
# w_N = phi^(N-2) / (N-1) pairwise weight factor
# Invariant transitive redundancy: Delta(3) = Delta(4) = phi^-2 ≈ 0.381966
GOLDEN_MP_WEIGHTS: dict[int, float] = {
    2: 1.0,
    3: PHI / 2.0,  # phi / 2 ≈ 0.80901699
    4: (PHI**2) / 3.0,  # phi^2 / 3 ≈ 0.87267799
}

# Season reset parameters (Softer Reset: geometric/arithmetic midpoint calibration)
RESET_ALPHA_SOFTER = (PHI + math.sqrt(PHI)) / 2.0  # (phi + sqrt(phi)) / 2 ≈ 1.4450268
RESET_LAMBDA_SOFTER = (
    (1.0 - 1.0 / PHI) * (1.0 / PHI)
) / 2.0  # (phi^-3) / 2 ≈ 0.11803399

CALIBRATION_THRESHOLD = 15  # ranked games to full calibration

ENGINES = ("glicko2_daneo", "glicko2_std")
RESET_MODES = ("continuous", "softer")


def model_name(engine: str, reset_mode: str) -> str:
    """Returns the model_type a run stores for engine with reset_mode.

    Raises ValueError for an engine or reset mode not in ENGINES or RESET_MODES.
    """
    if engine not in ENGINES:
        raise ValueError(f"unknown engine: {engine!r}")
    if reset_mode not in RESET_MODES:
        raise ValueError(f"unknown reset mode: {reset_mode!r}")
    return engine if reset_mode == "continuous" else f"{engine}_{reset_mode}"


# The models a run computes, by name, as (engine, reset mode): Glicko-D, continuous and with a
# season reset, which the site shows, and standard Glicko-2 as the benchmark (the country stats read it).
MODELS: dict[str, tuple[str, str]] = {
    model_name(e, r): (e, r)
    for e, r in [
        ("glicko2_daneo", "continuous"),
        ("glicko2_daneo", "softer"),
        ("glicko2_std", "continuous"),
    ]
}


# 16 Granular Competitive Tier Thresholds (Dynamic phi-Quantile Slicing)
# Symmetrically partitioned using powers of the Golden Ratio phi = (1 + sqrt(5)) / 2:
# z in {0, ±phi^-2, ±phi^-1, ±1.0, ±phi, ±2.0, ±phi^2, ±3.0}
# Above mean (>= pop_mu): deflates toward lower bound of tier N-1
# Below mean (< pop_mu): regresses upwards toward upper bound of tier N+1 / mean
PHI_TIER_SPECS = [
    ("Wood 1", -float("inf"), -3.0),
    ("Wood 2", -3.0, -(PHI**2)),  # [-3.0, -2.618)
    ("Bronze 1", -(PHI**2), -2.0),  # [-2.618, -2.0)
    ("Bronze 2", -2.0, -PHI),  # [-2.0, -1.618)
    ("Silver 1", -PHI, -1.0),  # [-1.618, -1.0)
    ("Silver 2", -1.0, -(1.0 / PHI)),  # [-1.0, -0.618)
    ("Gold 1", -(1.0 / PHI), -(1.0 / (PHI**2))),  # [-0.618, -0.382)
    ("Gold 2", -(1.0 / (PHI**2)), 0.0),  # [-0.382, 0.0)
    ("Platinum 1", 0.0, 1.0 / (PHI**2)),  # [0.0, +0.382)
    ("Platinum 2", 1.0 / (PHI**2), 1.0 / PHI),  # [+0.382, +0.618)
    ("Master 1", 1.0 / PHI, 1.0),  # [+0.618, +1.0)
    ("Master 2", 1.0, PHI),  # [+1.0, +1.618)
    ("GM 1", PHI, 2.0),  # [+1.618, +2.0)
    ("GM 2", 2.0, PHI**2),  # [+2.0, +2.618)
    ("SuperGM 1", PHI**2, 3.0),  # [+2.618, +3.0)
    ("SuperGM 2", 3.0, float("inf")),  # [≥ +3.0)
]


def get_tier_mean(
    rating: float, mean_rating: float = 1500.0, sigma: float = 175.0
) -> float:
    """Calculates the target tier anchor for seasonal resets across dynamic phi-quantile tiers.

    - Players above the mean (> mean_rating) deflate toward the lower bound of Tier N-1.
    - Players at or below the mean (<= mean_rating) are not regressed upward (anchor is their own rating),
      as the reset is strictly deflation-only to prevent phantom rating drift for inactive accounts.
    """
    z = (rating - mean_rating) / max(1.0, sigma)
    if z <= 0.0:
        return rating

    for i in range(8, 16):
        _name, z_low, z_high = PHI_TIER_SPECS[i]
        if (z >= z_low and z < z_high) or (i == 15 and z >= z_low):
            if i == 8:  # Platinum 1 -> deflates toward mean
                return mean_rating
            prev_z_low = PHI_TIER_SPECS[i - 1][1]
            return mean_rating + prev_z_low * sigma
    return mean_rating


def compute_decoupled_scaling(
    rating: float,
    rd: float,
    mean_rating: float = 1500.0,
    sigma_pop: float = 175.0,
    rd_mean: float = 75.0,
    rd_min: float = 30.0,
    base_alpha: float = RESET_ALPHA_SOFTER,
    base_lambda: float = RESET_LAMBDA_SOFTER,
) -> tuple[float, float]:
    """Decoupled parameter scaling for seasonal reset:

    1. Uncertainty inflation alpha(RD):
       - If player has near-average or high RD (>= rd_mean), uncertainty is already present -> minimal inflation (alpha ~ 1.0).
       - If player has exceptionally low RD (<= rd_min), certainty is rock-solid -> receives full base_alpha factor for seasonal mobility.

    2. Rating regression lambda(mu) [Deflation-Only / Asymmetric]:
       - Below-average players (rating <= mean_rating) receive ZERO upward regression (eff_lambda = 0.0).
         This prevents inactive accounts from accumulating unearned "free rating" and preserves matchmaking integrity.
       - Above-average players (rating > mean_rating) receive smooth regression toward their tier anchor:
         Median players receive damped regression (~phi^-2 * base_lambda), while extreme tails receive full base_lambda.
    """
    # 1. RD-based alpha scaling
    if rd >= rd_mean:
        tau_rd = 0.0
    elif rd <= rd_min:
        tau_rd = 1.0
    else:
        tau_rd = (rd_mean - rd) / (rd_mean - rd_min)
    eff_alpha = 1.0 + (base_alpha - 1.0) * tau_rd

    # 2. Rating-based lambda scaling (Deflation-Only)
    if rating <= mean_rating:
        eff_lambda = 0.0
    else:
        z_mu = (rating - mean_rating) / max(1.0, sigma_pop)
        tau_mu = 1.0 - math.exp(-(z_mu**2) / (2.0 * PHI))
        phi_inv2 = 1.0 / (PHI**2)  # ≈ 0.381966
        eff_lambda = base_lambda * (phi_inv2 + (1.0 - phi_inv2) * tau_mu)

    return eff_alpha, eff_lambda


class PlayerThresholds:
    """Per-player calibration thresholds over the games added so far.

    1. Standard threshold: 15 games.
    2. Second criterion: If a player has < 15 games across their history and has been inactive
       for > 1 year (> 365 days) after their last game, their calibration threshold is set
       to their max game count k.
    """

    def __init__(self, standard_threshold: int = 15, inactivity_days: int = 365):
        self.standard_threshold = standard_threshold
        self.inactivity_days = inactivity_days
        self.games: dict[str, set[Any]] = defaultdict(set)
        self.last_date: dict[str, str] = {}
        self.max_date_str = "1900-01-01"

    def add(self, rows: list[Any]) -> None:
        """Counts rows' games, in any order, as played."""
        for r in rows:
            d_str = r["date"][:10]
            self.max_date_str = max(self.max_date_str, d_str)
            mid = (
                r["match_id"]
                if "match_id" in r
                else (r["date"], r["player_a"], r["player_b"])
            )
            for p in (r["player_a"], r["player_b"]):
                self.games[p].add(mid)
                self.last_date[p] = max(self.last_date.get(p, d_str), d_str)

    def get(self, p_name: str, default: int) -> int:
        """Returns the player's threshold as of the last game added, or default for a player never seen."""
        if p_name not in self.games:
            return default
        cnt = len(self.games[p_name])
        if cnt >= self.standard_threshold:
            return self.standard_threshold
        max_dt = (
            date.fromisoformat(self.max_date_str)
            if self.max_date_str != "1900-01-01"
            else datetime.now(UTC).date()
        )
        p_dt = date.fromisoformat(self.last_date[p_name])
        if (max_dt - p_dt).days > self.inactivity_days:
            return max(1, cnt)
        return self.standard_threshold


def compute_player_thresholds(
    rows: list[Any], standard_threshold: int = 15, inactivity_days: int = 365
) -> dict[str, int]:
    """Computes every player's calibration threshold (PlayerThresholds) over rows."""
    thresholds = PlayerThresholds(standard_threshold, inactivity_days)
    thresholds.add(rows)
    return {p: thresholds.get(p, standard_threshold) for p in thresholds.games}


def compute_glicko2_ratings(
    engine: str = "glicko2_std",
    reset_mode: str = "continuous",
    player_count: int = 0,
    *,
    conn: psycopg.Connection,
    run_id: int,
) -> dict[str, Rating]:
    """Runs chronological monthly Glicko-2 updates for matches.

    Reads the run's pairs and writes the model's ratings and history into the
    run, replacing any it had, in one committed transaction.

    engine: 'glicko2_daneo' (Golden Ratio multiplayer weights, retro-calibrated) or 'glicko2_std'.
    reset_mode: 'continuous' (career) or 'softer' (season reset: RD x1.45, 11.8% tier reversion).
    player_count: 0 for all matches, or 2, 3, 4 for specific format.
    Retro calibration is the Two-Pass Retrospective Prior Calibration (Option A* Surgical Split):
    pass 1 extracts emergent ratings at the 15-game threshold, pass 2 recalculates history with
    calibrated priors, freezing updates during games 1..15.
    Raises ValueError for an unknown engine or reset mode.
    """
    model_type = model_name(engine, reset_mode)
    # DANeo Gold Standard is inherently retro-calibrated and multiplayer-weighted
    retro_calibrated = weighted = engine == "glicko2_daneo"

    with conn.transaction():
        rows = load_pairs(conn, run_id, player_count)

        # Group matches by calendar month (YYYY-MM)
        periods = defaultdict(list)
        period_dates = {}
        for r in rows:
            m_key = r["date"][:7]
            periods[m_key].append(r)
            period_dates[m_key] = r["date"][:10]

        sorted_months = sorted(periods.keys())

        # -----------------------------------------------------------------
        # PASS 1 (If retro_calibrated): Iterative Discovery of emergent priors
        # -----------------------------------------------------------------
        calibrated_priors: dict[str, Rating] = {}
        player_thresholds: dict[str, int] = {}
        if retro_calibrated:
            player_thresholds = compute_player_thresholds(
                rows, standard_threshold=CALIBRATION_THRESHOLD
            )

            def _run_pass1_iteration(
                current_priors: dict[str, Rating] | None, alpha: float = 0.5
            ) -> dict[str, Rating]:
                # ratings_discovery: tracking each player's own discovery path from 1500
                ratings_p1_discovery: dict[str, Rating] = defaultdict(
                    lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA)
                )
                # ratings_p1_world: ratings presented to opponents
                ratings_p1_world: dict[str, Rating] = defaultdict(
                    lambda: Rating(1500.0, 350.0, CALIBRATED_SIGMA)
                )
                if current_priors:
                    for p_name, pr in current_priors.items():
                        ratings_p1_world[p_name] = Rating(pr.rating, pr.rd, pr.sigma)

                games_played_p1 = defaultdict(set)
                prev_year_p1 = None
                new_discovered: dict[str, Rating] = {}

                for m_key in sorted_months:
                    cur_year = m_key[:4]
                    if (
                        prev_year_p1 is not None
                        and cur_year != prev_year_p1
                        and reset_mode != "continuous"
                    ):
                        act_p1 = [r for r in ratings_p1_world.values() if r.rd < 350.0]
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

                        for p_name, r_cur in list(ratings_p1_world.items()):
                            if r_cur.rd < 350.0:
                                tier_mean = get_tier_mean(
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
                                new_r = (
                                    1.0 - eff_lambda
                                ) * r_cur.rating + eff_lambda * tier_mean
                                ratings_p1_world[p_name] = Rating(
                                    new_r, new_rd, r_cur.sigma
                                )
                    prev_year_p1 = cur_year

                    p1_matches = periods[m_key]
                    p1_encounters = defaultdict(list)
                    for m in p1_matches:
                        pa, pb = m["player_a"], m["player_b"]
                        mid = m["match_id"] if "match_id" in m else (m["date"], pa, pb)
                        out_a = m["outcome_a"]
                        m_pcount = m.get("player_count", player_count)

                        w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0) if weighted else 1.0

                        # Opponents are evaluated via ratings_p1_world!
                        p1_encounters[pa].append(
                            (ratings_p1_world[pb], out_a, w, m_pcount, mid)
                        )
                        p1_encounters[pb].append(
                            (ratings_p1_world[pa], 1.0 - out_a, w, m_pcount, mid)
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

                        # 1. Update ratings_p1_discovery for player's personal discovery path
                        if (
                            prev_games < p_thresh
                            and total_games >= p_thresh
                            and p_name not in new_discovered
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
                                ratings_p1_discovery[p_name], enc_up_to_thresh
                            )
                            if current_priors and p_name in current_priors:
                                # Damped relaxation update for fixed-point stability
                                d_r = (1.0 - alpha) * current_priors[
                                    p_name
                                ].rating + alpha * r_at_thresh.rating
                                d_rd = (1.0 - alpha) * current_priors[
                                    p_name
                                ].rd + alpha * r_at_thresh.rd
                                new_discovered[p_name] = Rating(
                                    d_r, d_rd, r_at_thresh.sigma
                                )
                            else:
                                new_discovered[p_name] = Rating(
                                    r_at_thresh.rating,
                                    r_at_thresh.rd,
                                    r_at_thresh.sigma,
                                )

                            if enc_after_thresh:
                                ratings_p1_discovery[p_name] = run_update_p1(
                                    r_at_thresh, enc_after_thresh
                                )
                            else:
                                ratings_p1_discovery[p_name] = r_at_thresh
                        elif prev_games >= p_thresh:
                            ratings_p1_discovery[p_name] = run_update_p1(
                                ratings_p1_discovery[p_name], enc_list
                            )
                        else:
                            # Still provisional (total_games < p_thresh)
                            ratings_p1_discovery[p_name] = run_update_p1(
                                ratings_p1_discovery[p_name], enc_list
                            )

                        # 2. Update ratings_p1_world (Option A* Surgical Split)
                        if current_priors and p_name in current_priors:
                            if total_games <= p_thresh:
                                pass  # Frozen during provisional window
                            elif prev_games < p_thresh and total_games > p_thresh:
                                needed = p_thresh - prev_games
                                mids_up = set(distinct_new[:needed])
                                enc_after = [
                                    it for it in enc_list if it[4] not in mids_up
                                ]
                                if enc_after:
                                    ratings_p1_world[p_name] = run_update_p1(
                                        ratings_p1_world[p_name], enc_after
                                    )
                            else:
                                ratings_p1_world[p_name] = run_update_p1(
                                    ratings_p1_world[p_name], enc_list
                                )
                        else:
                            ratings_p1_world[p_name] = run_update_p1(
                                ratings_p1_world[p_name], enc_list
                            )

                    for p_name in list(ratings_p1_world.keys()):
                        p_thresh = player_thresholds.get(p_name, CALIBRATION_THRESHOLD)
                        is_frozen = (
                            current_priors
                            and p_name in current_priors
                            and len(games_played_p1[p_name]) < p_thresh
                        )
                        if (
                            not is_frozen
                            and p_name not in p1_active
                            and ratings_p1_world[p_name].rd < 350.0
                            and len(games_played_p1[p_name]) >= p_thresh
                        ):
                            r_cur = ratings_p1_world[p_name]
                            phi_new = (
                                r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2
                            ) ** 0.5
                            new_rd = min(350.0, phi_new * GLICKO2_SCALE)
                            ratings_p1_world[p_name] = Rating(
                                r_cur.rating, new_rd, r_cur.sigma
                            )

                return new_discovered

            # Initial Pass (k = 0): naive initial discovery
            calibrated_priors = _run_pass1_iteration(None, alpha=1.0)

            # Iterative refinement passes (k = 1..max_iterations-1)
            max_prior_iterations = 5
            tolerance = 0.5
            relaxation = 0.5

            if calibrated_priors and max_prior_iterations > 1:
                for _ in range(1, max_prior_iterations):
                    new_priors = _run_pass1_iteration(
                        calibrated_priors, alpha=relaxation
                    )
                    max_d = 0.0
                    for p_name, p_r in new_priors.items():
                        if p_name in calibrated_priors:
                            d = abs(p_r.rating - calibrated_priors[p_name].rating)
                            max_d = max(max_d, d)
                    calibrated_priors = new_priors
                    if max_d < tolerance:
                        break

        # -----------------------------------------------------------------
        # PASS 2 (or Standard Pass): Chronological calculation with Option A*
        # -----------------------------------------------------------------
        ratings: dict[str, Rating] = {}
        if retro_calibrated:
            for p_name, p_r in calibrated_priors.items():
                ratings[p_name] = Rating(p_r.rating, p_r.rd, p_r.sigma)

        def get_current_rating(p: str) -> Rating:
            if p not in ratings:
                ratings[p] = Rating(1500.0, 350.0, CALIBRATED_SIGMA)
            return ratings[p]

        history_records = []
        games_played_p2 = defaultdict(set)
        prev_year = None

        for m_key in sorted_months:
            cur_year = m_key[:4]

            # Apply season reset when transitioning across calendar year boundaries
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
                    is_eligible_for_reset = (
                        (
                            len(games_played_p2[p_name])
                            >= player_thresholds.get(p_name, CALIBRATION_THRESHOLD)
                        )
                        if retro_calibrated
                        else True
                    )
                    if is_eligible_for_reset and ratings[p_name].rd < 350.0:
                        tier_mean = get_tier_mean(
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
                        ].rating + eff_lambda * tier_mean
                        ratings[p_name] = Rating(new_r, new_rd, ratings[p_name].sigma)

            prev_year = cur_year
            period_matches = periods[m_key]
            period_date_str = period_dates[m_key]

            # Collect matches per player in this month
            player_encounters = defaultdict(list)
            for m in period_matches:
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

            # Update active players
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
                        # Option A: Frozen during initial calibration window (1..p_thresh games) to prevent double valuation
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

                curr = get_current_rating(p_name)
                history_records.append(
                    (
                        model_type,
                        player_count,
                        p_name,
                        period_date_str,
                        round(curr.rating, 2),
                        round(curr.rd, 2),
                        round(curr.conservative_rating, 2),
                    )
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
                    new_rd = min(350.0, phi_new * GLICKO2_SCALE)
                    ratings[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        # Aggregate player statistics for this format
        if player_count and player_count > 0:
            stats_rows = conn.execute(
                """
                SELECT 
                    p.name,
                    COUNT(pm.pairwise_id) as opps_count,
                    SUM(CASE WHEN pm.player_a = p.name AND pm.outcome_a = 1.0 THEN 1
                             WHEN pm.player_b = p.name AND pm.outcome_a = 0.0 THEN 1 ELSE 0 END) as wins,
                    SUM(CASE WHEN pm.player_a = p.name AND pm.outcome_a = 0.0 THEN 1
                             WHEN pm.player_b = p.name AND pm.outcome_a = 1.0 THEN 1 ELSE 0 END) as losses,
                    SUM(CASE WHEN pm.outcome_a = 0.5 THEN 1 ELSE 0 END) as draws,
                    MAX(pm.date) as last_played
                FROM players p
                INNER JOIN pairwise_matches pm ON (p.name = pm.player_a OR p.name = pm.player_b) AND pm.player_count = %s AND pm.run_id = %s
                GROUP BY p.name
            """,
                (player_count, run_id),
            ).fetchall()
        else:
            stats_rows = conn.execute(
                """
                SELECT 
                    p.name,
                    COUNT(pm.pairwise_id) as opps_count,
                    SUM(CASE WHEN pm.player_a = p.name AND pm.outcome_a = 1.0 THEN 1
                             WHEN pm.player_b = p.name AND pm.outcome_a = 0.0 THEN 1 ELSE 0 END) as wins,
                    SUM(CASE WHEN pm.player_a = p.name AND pm.outcome_a = 0.0 THEN 1
                             WHEN pm.player_b = p.name AND pm.outcome_a = 1.0 THEN 1 ELSE 0 END) as losses,
                    SUM(CASE WHEN pm.outcome_a = 0.5 THEN 1 ELSE 0 END) as draws,
                    MAX(pm.date) as last_played
                FROM players p
                LEFT JOIN pairwise_matches pm ON (p.name = pm.player_a OR p.name = pm.player_b) AND pm.run_id = %s
                GROUP BY p.name
            """,
                (run_id,),
            ).fetchall()
        player_stats = {r["name"]: r for r in stats_rows}

        # Rank players by conservative rating C = rating - 3*rd
        player_rankings = []
        for p_name, r_obj in ratings.items():
            st = player_stats.get(p_name)
            opps = st["opps_count"] if st else 0
            if player_count and player_count > 0 and opps == 0:
                continue
            w_cnt = st["wins"] if st else 0
            l_cnt = st["losses"] if st else 0
            d_cnt = st["draws"] if st else 0
            wr = round((w_cnt + 0.5 * d_cnt) / max(1, opps) * 100.0, 2)
            lp = st["last_played"] if st else None

            player_rankings.append(
                {
                    "name": p_name,
                    "rating": round(r_obj.rating, 2),
                    "rd": round(r_obj.rd, 2),
                    "sigma": round(r_obj.sigma, 4),
                    "c_rating": round(r_obj.conservative_rating, 2),
                    "opps": opps,
                    "wins": w_cnt,
                    "losses": l_cnt,
                    "draws": d_cnt,
                    "win_rate": wr,
                    "last_played": lp,
                }
            )

        player_rankings.sort(key=lambda x: x["c_rating"], reverse=True)

        rating_rows = []
        for rank, p_data in enumerate(player_rankings, 1):
            rating_rows.append(
                (
                    model_type,
                    player_count,
                    p_data["name"],
                    p_data["rating"],
                    p_data["rd"],
                    p_data["sigma"],
                    p_data["c_rating"],
                    rank,
                    0,
                    p_data["opps"],
                    p_data["opps"],
                    p_data["wins"],
                    p_data["losses"],
                    p_data["draws"],
                    p_data["win_rate"],
                    p_data["last_played"],
                )
            )

        save_model_results(
            conn, run_id, model_type, player_count, rating_rows, history_records
        )

        fmt_label = f"{player_count}p" if player_count > 0 else "All"
        print(
            f"Successfully saved {len(rating_rows)} ratings and {len(history_records)} history points for {model_type} ({fmt_label})."
        )
        return ratings
