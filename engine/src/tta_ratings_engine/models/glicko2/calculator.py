"""Glicko-2 calendar monthly batch calculator calibrated to TTA tournament cadence."""

import math
from collections import defaultdict
from collections.abc import Callable
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
            mid = match_id(r)
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


def new_rating() -> Rating:
    """Returns the rating a player starts from, before any game."""
    return Rating(1500.0, 350.0, CALIBRATED_SIGMA)


def match_id(m: Any) -> Any:
    """Returns pair m's game: its match_id, or (date, player_a, player_b) for a pair without one."""
    return m["match_id"] if "match_id" in m else (m["date"], m["player_a"], m["player_b"])  # fmt: skip


def by_month(rows: list[Any]) -> list[tuple[str, list[Any]]]:
    """Returns rows grouped by calendar month ('YYYY-MM'), as (month, rows), months in order."""
    periods: dict[str, list[Any]] = defaultdict(list)
    for r in rows:
        periods[r["date"][:7]].append(r)
    return [(m, periods[m]) for m in sorted(periods)]


def new_year(previous: str | None, month: str) -> bool:
    """Returns True if month ('YYYY-MM') starts a calendar year after previous (None before the first)."""
    return previous is not None and month[:4] != previous[:4]


# A player's encounter: (opponent's rating, score, weight, player count, game).
Encounter = tuple[Rating, float, float, int, Any]


def month_encounters(
    matches: list[Any],
    rating_of: Callable[[str], Rating],
    weighted: bool,
    player_count: int,
    b_first: bool = False,
) -> dict[str, list[Encounter]]:
    """Returns each player's encounters in matches, players in the order they first play.

    rating_of gives an opponent's rating; weighted weighs a pair by its format's
    GOLDEN_MP_WEIGHTS, else 1. b_first looks up player_b before player_a: where
    rating_of adds new players to a map, that sets their order in it.
    """
    encounters: dict[str, list[Encounter]] = defaultdict(list)
    for m in matches:
        pa, pb = m["player_a"], m["player_b"]
        out_a = m["outcome_a"]
        m_pcount = m.get("player_count", player_count)
        w = GOLDEN_MP_WEIGHTS.get(m_pcount, 1.0) if weighted else 1.0
        if b_first:
            rb = rating_of(pb)
            ra = rating_of(pa)
        else:
            ra = rating_of(pa)
            rb = rating_of(pb)
        mid = match_id(m)
        encounters[pa].append((rb, out_a, w, m_pcount, mid))
        encounters[pb].append((ra, 1.0 - out_a, w, m_pcount, mid))
    return encounters


def count_new_games(
    seen: set[Any], encounters: list[Encounter]
) -> tuple[int, int, list[Any]]:
    """Adds encounters' games to seen, a player's games so far.

    Returns the number of games before, the number after, and the new games in order.
    """
    before = len(seen)
    new = []
    for it in encounters:
        if it[4] not in seen:
            seen.add(it[4])
            new.append(it[4])
    return before, len(seen), new


def split_at_threshold(
    encounters: list[Encounter], new_games: list[Any], needed: int
) -> tuple[list[Encounter], list[Encounter]]:
    """Splits encounters into those of the first `needed` of new_games, and the rest."""
    up_to = set(new_games[:needed])
    return (
        [it for it in encounters if it[4] in up_to],
        [it for it in encounters if it[4] not in up_to],
    )


def rate(r: Rating, encounters: list[Encounter], weighted: bool) -> Rating:
    """Returns r after one Glicko-2 update over encounters, at the calibrated tau."""
    return update_rating(
        r,
        [(it[0], it[1], it[2]) for it in encounters],
        tau=CALIBRATED_TAU,
        weighted=weighted,
    )


def drifted(r: Rating) -> Rating:
    """Returns r after a month without games: its RD grows by the inactivity drift, to 350 at most."""
    phi_new = (r.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2) ** 0.5
    return Rating(r.rating, min(350.0, phi_new * GLICKO2_SCALE), r.sigma)


def season_reset(ratings: dict[str, Rating], eligible: Callable[[str], bool]) -> None:
    """Resets, in place, every eligible player whose RD is under 350, for a new season.

    Each is regressed toward their tier anchor and their RD widened
    (compute_decoupled_scaling), against those players' mean, spread and RD.
    """
    active = [r for p, r in ratings.items() if eligible(p) and r.rd < 350.0]
    if active:
        pop_mu = sum(r.rating for r in active) / len(active)
        pop_sigma = max(
            50.0, (sum((r.rating - pop_mu) ** 2 for r in active) / len(active)) ** 0.5
        )
        pop_rd_mean = sum(r.rd for r in active) / len(active)
        pop_rd_min = max(20.0, min(r.rd for r in active))
    else:
        pop_mu, pop_sigma, pop_rd_mean, pop_rd_min = 1500.0, 175.0, 75.0, 30.0

    for p_name in list(ratings.keys()):
        r_cur = ratings[p_name]
        if eligible(p_name) and r_cur.rd < 350.0:
            tier_mean = get_tier_mean(r_cur.rating, mean_rating=pop_mu, sigma=pop_sigma)
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
            new_r = (1.0 - eff_lambda) * r_cur.rating + eff_lambda * tier_mean
            ratings[p_name] = Rating(new_r, new_rd, r_cur.sigma)


def _prior_pass(
    months: list[tuple[str, list[Any]]],
    thresholds: dict[str, int],
    weighted: bool,
    softer: bool,
    player_count: int,
    current_priors: dict[str, Rating] | None,
    alpha: float,
) -> dict[str, Rating]:
    """One pass of the retro calibration's pass 1: each player's rating at their threshold game.

    Each player is followed on their own path from 1500 (discovery), against
    opponents as the world sees them. With current_priors, the world starts
    from them, holds a player at theirs until their threshold game, and a new
    prior moves alpha of the way from the current one.
    """
    discovery: dict[str, Rating] = defaultdict(new_rating)
    world: dict[str, Rating] = defaultdict(new_rating)
    if current_priors:
        for p_name, pr in current_priors.items():
            world[p_name] = Rating(pr.rating, pr.rd, pr.sigma)
    games: dict[str, set[Any]] = defaultdict(set)
    found: dict[str, Rating] = {}

    previous = None
    for month, matches in months:
        if softer and new_year(previous, month):
            season_reset(world, lambda p: True)
        previous = month

        encounters = month_encounters(
            matches, world.__getitem__, weighted, player_count, b_first=True
        )
        for p_name, enc_list in encounters.items():
            prev_games, total_games, distinct_new = count_new_games(
                games[p_name], enc_list
            )
            p_thresh = thresholds.get(p_name, CALIBRATION_THRESHOLD)
            has_prior = current_priors is not None and p_name in current_priors

            if prev_games < p_thresh <= total_games and p_name not in found:
                enc_up, enc_after = split_at_threshold(
                    enc_list, distinct_new, p_thresh - prev_games
                )
                r_at = rate(discovery[p_name], enc_up, weighted)
                if has_prior:
                    # Damped relaxation update for fixed-point stability
                    cur = current_priors[p_name]
                    d_r = (1.0 - alpha) * cur.rating + alpha * r_at.rating
                    d_rd = (1.0 - alpha) * cur.rd + alpha * r_at.rd
                    found[p_name] = Rating(d_r, d_rd, r_at.sigma)
                else:
                    found[p_name] = Rating(r_at.rating, r_at.rd, r_at.sigma)
                discovery[p_name] = (
                    rate(r_at, enc_after, weighted) if enc_after else r_at
                )
            else:
                discovery[p_name] = rate(discovery[p_name], enc_list, weighted)

            # Option A* Surgical Split: a prior stands in for the games up to the threshold
            if not has_prior:
                world[p_name] = rate(world[p_name], enc_list, weighted)
            elif total_games <= p_thresh:
                pass
            elif prev_games < p_thresh and total_games > p_thresh:
                _, enc_after = split_at_threshold(
                    enc_list, distinct_new, p_thresh - prev_games
                )
                if enc_after:
                    world[p_name] = rate(world[p_name], enc_after, weighted)
            else:
                world[p_name] = rate(world[p_name], enc_list, weighted)

        for p_name in list(world.keys()):
            p_thresh = thresholds.get(p_name, CALIBRATION_THRESHOLD)
            is_frozen = (
                current_priors
                and p_name in current_priors
                and len(games[p_name]) < p_thresh
            )
            if (
                not is_frozen
                and p_name not in encounters
                and world[p_name].rd < 350.0
                and len(games[p_name]) >= p_thresh
            ):
                world[p_name] = drifted(world[p_name])

    return found


def calibrated_priors(
    months: list[tuple[str, list[Any]]],
    thresholds: dict[str, int],
    weighted: bool,
    softer: bool,
    player_count: int,
) -> dict[str, Rating]:
    """Pass 1 of the retro calibration: each player's prior, refined over up to five passes.

    The first pass is naive; each after it starts from the last one's priors
    and moves each half way (relaxation 0.5), until none moves by 0.5 or more.
    """
    priors = _prior_pass(
        months, thresholds, weighted, softer, player_count, None, alpha=1.0
    )
    max_prior_iterations = 5
    tolerance = 0.5
    relaxation = 0.5
    if priors and max_prior_iterations > 1:
        for _ in range(1, max_prior_iterations):
            new_priors = _prior_pass(
                months, thresholds, weighted, softer, player_count, priors, relaxation
            )
            max_d = 0.0
            for p_name, p_r in new_priors.items():
                if p_name in priors:
                    max_d = max(max_d, abs(p_r.rating - priors[p_name].rating))
            priors = new_priors
            if max_d < tolerance:
                break
    return priors


class SecondPass:
    """Pass 2: a model's ratings, month by month.

    A retro-calibrated model starts a player with a prior from it, holds them
    there until their threshold game, then rates only the games after it.
    """

    def __init__(
        self,
        priors: dict[str, Rating],
        thresholds: dict[str, int] | PlayerThresholds,
        weighted: bool,
        retro: bool,
        player_count: int,
    ):
        self.ratings = {p: Rating(r.rating, r.rd, r.sigma) for p, r in priors.items()}
        self.priors = priors
        self.thresholds = thresholds
        self.weighted = weighted
        self.retro = retro
        self.player_count = player_count
        self.games: dict[str, set[Any]] = defaultdict(set)

    def rating(self, p_name: str) -> Rating:
        """Returns the player's rating, starting them from new_rating() when they have none."""
        if p_name not in self.ratings:
            self.ratings[p_name] = new_rating()
        return self.ratings[p_name]

    def calibrated(self, p_name: str) -> bool:
        """Returns True once the player has played their threshold's number of games."""
        return len(self.games[p_name]) >= self.thresholds.get(
            p_name, CALIBRATION_THRESHOLD
        )

    def season_reset(self) -> None:
        """Resets the rated players for a new season; a retro model resets only calibrated ones."""
        season_reset(self.ratings, lambda p: not self.retro or self.calibrated(p))

    def rate_month(self, matches: list[Any]) -> list[str]:
        """Rates a month's pairs, then drifts the calibrated players who missed it.

        Returns the players who played, in the order they first played.
        """
        encounters = month_encounters(
            matches, self.rating, self.weighted, self.player_count
        )
        for p_name, enc_list in encounters.items():
            self._rate(p_name, enc_list)
        for p_name in list(self.ratings.keys()):
            is_active_or_uncalibrated = (p_name in encounters) or (
                self.retro and not self.calibrated(p_name)
            )
            if not is_active_or_uncalibrated and self.ratings[p_name].rd < 350.0:
                self.ratings[p_name] = drifted(self.ratings[p_name])
        return list(encounters)

    def _rate(self, p_name: str, enc_list: list[Encounter]) -> None:
        if not self.retro:
            self.ratings[p_name] = rate(self.rating(p_name), enc_list, self.weighted)
            return
        prev_games, total_games, distinct_new = count_new_games(
            self.games[p_name], enc_list
        )
        p_thresh = self.thresholds.get(p_name, CALIBRATION_THRESHOLD)
        if p_name not in self.priors:
            self.ratings[p_name] = rate(self.rating(p_name), enc_list, self.weighted)
        elif total_games <= p_thresh:
            # Option A: Frozen during initial calibration window (1..p_thresh games) to prevent double valuation
            pass
        elif prev_games < p_thresh and total_games > p_thresh:
            # Surgical Split in transition month: matches up to threshold are frozen; only matches after threshold update
            _, enc_after = split_at_threshold(
                enc_list, distinct_new, p_thresh - prev_games
            )
            if enc_after:
                self.ratings[p_name] = rate(
                    self.rating(p_name), enc_after, self.weighted
                )
        else:
            self.ratings[p_name] = rate(self.rating(p_name), enc_list, self.weighted)


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
            priors = calibrated_priors(months, thresholds, weighted, softer, player_count)  # fmt: skip

        model = SecondPass(priors, thresholds, weighted, retro_calibrated, player_count)  # fmt: skip
        history_records = []
        previous = None
        for month, matches in months:
            if softer and new_year(previous, month):
                model.season_reset()
            previous = month
            period_date_str = matches[-1]["date"][:10]
            for p_name in model.rate_month(matches):
                curr = model.ratings[p_name]
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

        ratings = model.ratings
        rating_rows = _rating_rows(
            model_type, player_count, ratings, _player_stats(conn, run_id, player_count)
        )
        save_model_results(
            conn, run_id, model_type, player_count, rating_rows, history_records
        )

        fmt_label = f"{player_count}p" if player_count > 0 else "All"
        print(
            f"Successfully saved {len(rating_rows)} ratings and {len(history_records)} history points for {model_type} ({fmt_label})."
        )
        return ratings


def _player_stats(
    conn: psycopg.Connection, run_id: int, player_count: int
) -> dict[str, Any]:
    """Returns each player's pairs, wins, losses, draws and last game in the run's format, by player id."""
    if player_count and player_count > 0:
        stats_rows = conn.execute(
            """
            SELECT
                p.player_id,
                COUNT(pm.pairwise_id) as opps_count,
                SUM(CASE WHEN pm.player_a = p.player_id AND pm.outcome_a = 1.0 THEN 1
                         WHEN pm.player_b = p.player_id AND pm.outcome_a = 0.0 THEN 1 ELSE 0 END) as wins,
                SUM(CASE WHEN pm.player_a = p.player_id AND pm.outcome_a = 0.0 THEN 1
                         WHEN pm.player_b = p.player_id AND pm.outcome_a = 1.0 THEN 1 ELSE 0 END) as losses,
                SUM(CASE WHEN pm.outcome_a = 0.5 THEN 1 ELSE 0 END) as draws,
                MAX(pm.date) as last_played
            FROM players p
            INNER JOIN pairwise_matches pm ON (p.player_id = pm.player_a OR p.player_id = pm.player_b) AND pm.player_count = %s AND pm.run_id = %s
            GROUP BY p.player_id
        """,
            (player_count, run_id),
        ).fetchall()
    else:
        stats_rows = conn.execute(
            """
            SELECT
                p.player_id,
                COUNT(pm.pairwise_id) as opps_count,
                SUM(CASE WHEN pm.player_a = p.player_id AND pm.outcome_a = 1.0 THEN 1
                         WHEN pm.player_b = p.player_id AND pm.outcome_a = 0.0 THEN 1 ELSE 0 END) as wins,
                SUM(CASE WHEN pm.player_a = p.player_id AND pm.outcome_a = 0.0 THEN 1
                         WHEN pm.player_b = p.player_id AND pm.outcome_a = 1.0 THEN 1 ELSE 0 END) as losses,
                SUM(CASE WHEN pm.outcome_a = 0.5 THEN 1 ELSE 0 END) as draws,
                MAX(pm.date) as last_played
            FROM players p
            LEFT JOIN pairwise_matches pm ON (p.player_id = pm.player_a OR p.player_id = pm.player_b) AND pm.run_id = %s
            GROUP BY p.player_id
        """,
            (run_id,),
        ).fetchall()
    return {r["player_id"]: r for r in stats_rows}


def _rating_rows(
    model_type: str,
    player_count: int,
    ratings: dict[str, Rating],
    player_stats: dict[str, Any],
) -> list[tuple[Any, ...]]:
    """Returns the model's player_ratings rows, ranked by conservative rating C = rating - 3*rd.

    A format leaves out players with no pair in it.
    """
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
                "player": p_name,
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
                p_data["player"],
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
    return rating_rows
