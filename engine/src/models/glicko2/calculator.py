"""Glicko-2 calendar monthly batch calculator calibrated to TTA tournament cadence."""
from collections import defaultdict
from datetime import datetime
import math
import sqlite3
from typing import Dict, List, Tuple, Optional, Any

from src.data.db import get_connection
from src.models.glicko2.engine import Rating, update_rating, GLICKO2_SCALE

# Calibrated constants matching official TTA online implementation
CALIBRATED_TAU = 0.3
CALIBRATED_SIGMA = 0.03
INACTIVITY_DRIFT_PER_MONTH = 0.01  # gentle variance growth per missed month (~1.7 RD)

PHI = 1.61803398875

# DANeo Golden Ratio Multiplayer Information Decomposition Weights:
# W(N) = phi^(N-2) effective 1v1 equivalents
# w_N = phi^(N-2) / (N-1) pairwise weight factor
# Invariant transitive redundancy: Delta(3) = Delta(4) = phi^-2 ≈ 0.381966
GOLDEN_MP_WEIGHTS: Dict[int, float] = {
    2: 1.0,
    3: PHI / 2.0,           # phi / 2 ≈ 0.80901699
    4: (PHI ** 2) / 3.0,     # phi^2 / 3 ≈ 0.87267799
}

# Season reset parameters
# 1. Soft Reset (Canonical Golden Ratio powers)
RESET_ALPHA_SOFT = PHI  # phi ≈ 1.61803398875
RESET_LAMBDA_SOFT = (1.0 - 1.0 / PHI) * (1.0 / PHI)  # phi^-3 ≈ 0.23606798

# 2. Softer Reset (Geometric/arithmetic midpoint calibration requested by user)
RESET_ALPHA_SOFTER = (PHI + math.sqrt(PHI)) / 2.0  # (phi + sqrt(phi)) / 2 ≈ 1.4450268
RESET_LAMBDA_SOFTER = ((1.0 - 1.0 / PHI) * (1.0 / PHI)) / 2.0  # (phi^-3) / 2 ≈ 0.11803399

# 3. Hard / Amplified Reset (kept strictly for illustrative reasons)
RESET_ALPHA_HARD = PHI ** 2  # phi^2 ≈ 2.61803398875
RESET_LAMBDA_HARD = 1.0 / PHI  # phi^-1 ≈ 0.61803398875


# 16 Granular Competitive Tier Thresholds (Dynamic phi-Quantile Slicing)
# Symmetrically partitioned using powers of the Golden Ratio phi = (1 + sqrt(5)) / 2:
# z in {0, ±phi^-2, ±phi^-1, ±1.0, ±phi, ±2.0, ±phi^2, ±3.0}
# Above mean (>= pop_mu): deflates toward lower bound of tier N-1
# Below mean (< pop_mu): regresses upwards toward upper bound of tier N+1 / mean
PHI_TIER_SPECS = [
    ('Wood 1', -float('inf'), -3.0),
    ('Wood 2', -3.0, -(PHI**2)),            # [-3.0, -2.618)
    ('Bronze 1', -(PHI**2), -2.0),          # [-2.618, -2.0)
    ('Bronze 2', -2.0, -PHI),              # [-2.0, -1.618)
    ('Silver 1', -PHI, -1.0),              # [-1.618, -1.0)
    ('Silver 2', -1.0, -(1.0 / PHI)),      # [-1.0, -0.618)
    ('Gold 1', -(1.0 / PHI), -(1.0 / (PHI**2))),  # [-0.618, -0.382)
    ('Gold 2', -(1.0 / (PHI**2)), 0.0),    # [-0.382, 0.0)
    ('Platinum 1', 0.0, 1.0 / (PHI**2)),   # [0.0, +0.382)
    ('Platinum 2', 1.0 / (PHI**2), 1.0 / PHI),  # [+0.382, +0.618)
    ('Master 1', 1.0 / PHI, 1.0),          # [+0.618, +1.0)
    ('Master 2', 1.0, PHI),                # [+1.0, +1.618)
    ('GM 1', PHI, 2.0),                    # [+1.618, +2.0)
    ('GM 2', 2.0, PHI**2),                 # [+2.0, +2.618)
    ('SuperGM 1', PHI**2, 3.0),            # [+2.618, +3.0)
    ('SuperGM 2', 3.0, float('inf')),      # [≥ +3.0)
]

def get_tier_mean(rating: float, mean_rating: float = 1500.0, sigma: float = 175.0) -> float:
    """Calculates the target tier anchor for seasonal resets across dynamic phi-quantile tiers.
    
    - Players above the mean (> mean_rating) deflate toward the lower bound of Tier N-1.
    - Players at or below the mean (<= mean_rating) are not regressed upward (anchor is their own rating),
      as the reset is strictly deflation-only to prevent phantom rating drift for inactive accounts.
    """
    z = (rating - mean_rating) / max(1.0, sigma)
    if z <= 0.0:
        return rating

    for i in range(8, 16):
        name, z_low, z_high = PHI_TIER_SPECS[i]
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
    base_lambda: float = RESET_LAMBDA_SOFTER
) -> Tuple[float, float]:
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
        tau_mu = 1.0 - math.exp(-(z_mu ** 2) / (2.0 * PHI))
        phi_inv2 = 1.0 / (PHI ** 2)  # ≈ 0.381966
        eff_lambda = base_lambda * (phi_inv2 + (1.0 - phi_inv2) * tau_mu)

    return eff_alpha, eff_lambda


def compute_gaussian_phi_scaling(rating: float, mean_rating: float = 1500.0, sigma_pop: float = 175.0) -> Tuple[float, float]:
    """Legacy helper maintained for compatibility."""
    z = (rating - mean_rating) / sigma_pop
    tail_factor = 1.0 - math.exp(-(z ** 2) / (2.0 * PHI))
    alpha_mult = 1.0 + (PHI - 1.0) * tail_factor * 0.5
    lambda_mult = 1.0 + (1.0 / PHI) * tail_factor * 0.5
    return alpha_mult, lambda_mult


from src.models.glicko2.adaptive_t import update_rating_adaptive, ADAPTIVE_T_PARAMS


def compute_player_thresholds(
    rows: List[Any],
    standard_threshold: int = 15,
    inactivity_days: int = 365
) -> Dict[str, int]:
    """Computes per-player calibration thresholds.
    
    1. Standard threshold: 15 games.
    2. Second criterion: If a player has < 15 games across their history and has been inactive
       for > 1 year (> 365 days) after their last game, their calibration threshold is set
       to their max game count k.
    """
    p_total_games = defaultdict(set)
    p_last_date = {}
    max_date_str = '1900-01-01'

    for r in rows:
        d_str = r['date'][:10]
        if d_str > max_date_str:
            max_date_str = d_str
        mid = r['match_id'] if 'match_id' in r.keys() else (r['date'], r['player_a'], r['player_b'])
        pa, pb = r['player_a'], r['player_b']
        p_total_games[pa].add(mid)
        p_total_games[pb].add(mid)
        p_last_date[pa] = max(p_last_date.get(pa, d_str), d_str)
        p_last_date[pb] = max(p_last_date.get(pb, d_str), d_str)

    max_dt = datetime.strptime(max_date_str, '%Y-%m-%d').date() if max_date_str != '1900-01-01' else datetime.now().date()
    player_thresh = {}

    for p_name, g_set in p_total_games.items():
        cnt = len(g_set)
        if cnt >= standard_threshold:
            player_thresh[p_name] = standard_threshold
        else:
            p_dt = datetime.strptime(p_last_date[p_name], '%Y-%m-%d').date()
            if (max_dt - p_dt).days > inactivity_days:
                player_thresh[p_name] = max(1, cnt)
            else:
                player_thresh[p_name] = standard_threshold

    return player_thresh


def compute_glicko2_ratings(
    weighted: bool = False,
    player_count: int = 0,
    tau: float = CALIBRATED_TAU,
    reset_mode: str = 'continuous',
    engine: Optional[str] = None,
    retro_calibrated: bool = False,
    calibration_threshold: int = 15,
    db_path: Optional[str] = None
) -> Dict[str, Rating]:
    """Runs chronological monthly Glicko-2 updates for matches.
    
    player_count: 0 for all matches, or 2, 3, 4 for specific format.
    engine: 'glicko2_std', 'glicko2_mp', 'glicko2_adapt', or 'glicko2_daneo'.
    If engine is None: uses 'glicko2_mp' if weighted else 'glicko2_std'.
    reset_mode: 'continuous' (default career), 'softer' (gentle reset: RD x1.45, 11.8% tier reversion),
                'soft' (golden ratio phi RD inflation + 23.6% tier mean reversion),
                or 'amplified' (phi^2 RD inflation + 61.8% tier mean reversion, kept for illustrative reasons).
    retro_calibrated: If True, executes Two-Pass Retrospective Prior Calibration (Option A* Surgical Split).
                      Pass 1 extracts emergent ratings at the 15-game threshold.
                      Pass 2 recalculates history with calibrated priors, freezing updates during games 1..15.
    calibration_threshold: Number of ranked games to reach full calibration (default 15).
    """
    if engine is not None:
        base_model = engine
    else:
        base_model = 'glicko2_mp' if weighted else 'glicko2_std'

    if base_model == 'glicko2_daneo':
        retro_calibrated = True  # DANeo Gold Standard is inherently retro-calibrated
        if reset_mode != 'continuous':
            model_type = f"glicko2_daneo_{reset_mode}"
        else:
            model_type = "glicko2_daneo"
    elif reset_mode != 'continuous':
        model_type = f"{base_model}_{reset_mode}_retro" if retro_calibrated else f"{base_model}_{reset_mode}"
    else:
        model_type = f"{base_model}_retro" if retro_calibrated else base_model
        
    conn = get_connection(db_path)

    try:
        if player_count and player_count > 0:
            rows = conn.execute("""
                SELECT match_id, date, tournament, player_a, player_b, outcome_a, weight, player_count
                FROM pairwise_matches
                WHERE player_count = ? AND glicko_eligible = 1
                ORDER BY date, pairwise_id
            """, (player_count,)).fetchall()
        else:
            rows = conn.execute("""
                SELECT match_id, date, tournament, player_a, player_b, outcome_a, weight, player_count
                FROM pairwise_matches
                WHERE glicko_eligible = 1
                ORDER BY date, pairwise_id
            """).fetchall()

        # Group matches by calendar month (YYYY-MM)
        periods = defaultdict(list)
        period_dates = {}
        for r in rows:
            m_key = r['date'][:7]
            periods[m_key].append(r)
            period_dates[m_key] = r['date'][:10]

        sorted_months = sorted(periods.keys())

        # -----------------------------------------------------------------
        # PASS 1 (If retro_calibrated): Discovery of emergent ratings at threshold
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
                            tier_mean = get_tier_mean(r_cur.rating, mean_rating=pop_mu, sigma=pop_sigma)
                            eff_alpha, eff_lambda = compute_decoupled_scaling(
                                r_cur.rating, r_cur.rd,
                                mean_rating=pop_mu, sigma_pop=pop_sigma,
                                rd_mean=pop_rd_mean, rd_min=pop_rd_min,
                                base_alpha=base_a, base_lambda=base_l
                            )
                            new_rd = min(350.0, r_cur.rd * eff_alpha)
                            new_r = (1.0 - eff_lambda) * r_cur.rating + eff_lambda * tier_mean
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
                            return update_rating_adaptive(r_cur, enc4, tau=tau, params_dict=ADAPTIVE_T_PARAMS)
                        else:
                            enc3 = [(it[0], it[1], it[2]) for it in items]
                            return update_rating(r_cur, enc3, tau=tau, weighted=(base_model in ('glicko2_mp', 'glicko2_daneo') or weighted))

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
                        new_rd = min(350.0, phi_new * GLICKO2_SCALE)
                        ratings_p1[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        # -----------------------------------------------------------------
        # PASS 2 (or Standard Pass): Chronological calculation with Option A*
        # -----------------------------------------------------------------
        ratings: Dict[str, Rating] = {}
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
                    is_eligible_for_reset = (len(games_played_p2[p_name]) >= player_thresholds.get(p_name, calibration_threshold)) if retro_calibrated else True
                    if is_eligible_for_reset and ratings[p_name].rd < 350.0:
                        tier_mean = get_tier_mean(ratings[p_name].rating, mean_rating=pop_mu, sigma=pop_sigma)
                        eff_alpha, eff_lambda = compute_decoupled_scaling(
                            ratings[p_name].rating, ratings[p_name].rd,
                            mean_rating=pop_mu, sigma_pop=pop_sigma,
                            rd_mean=pop_rd_mean, rd_min=pop_rd_min,
                            base_alpha=base_a, base_lambda=base_l
                        )
                        new_rd = min(350.0, ratings[p_name].rd * eff_alpha)
                        new_r = (1.0 - eff_lambda) * ratings[p_name].rating + eff_lambda * tier_mean
                        ratings[p_name] = Rating(new_r, new_rd, ratings[p_name].sigma)

            prev_year = cur_year
            period_matches = periods[m_key]
            period_date_str = period_dates[m_key]

            # Collect matches per player in this month
            player_encounters = defaultdict(list)
            for m in period_matches:
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

            # Update active players
            for p_name, enc_list in player_encounters.items():
                def run_update_p2(r_cur, items):
                    if base_model == 'glicko2_adapt':
                        enc4 = [(it[0], it[1], it[2], it[3]) for it in items]
                        return update_rating_adaptive(r_cur, enc4, tau=tau, params_dict=ADAPTIVE_T_PARAMS)
                    else:
                        enc3 = [(it[0], it[1], it[2]) for it in items]
                        return update_rating(r_cur, enc3, tau=tau, weighted=(base_model in ('glicko2_mp', 'glicko2_daneo') or weighted))

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
                        # Option A: Frozen during initial calibration window (1..p_thresh games) to prevent double valuation
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

                curr = get_current_rating(p_name)
                history_records.append((
                    model_type, player_count, p_name, period_date_str,
                    round(curr.rating, 2), round(curr.rd, 2), round(curr.conservative_rating, 2)
                ))

            # Apply mild inactivity drift to already-active players who missed this month
            for p_name in list(ratings.keys()):
                p_thresh = player_thresholds.get(p_name, calibration_threshold)
                is_active_or_uncalibrated = (p_name in active_players) or (retro_calibrated and len(games_played_p2[p_name]) < p_thresh)
                if not is_active_or_uncalibrated and ratings[p_name].rd < 350.0:
                    r_cur = ratings[p_name]
                    phi_new = (r_cur.phi**2 + INACTIVITY_DRIFT_PER_MONTH**2)**0.5
                    new_rd = min(350.0, phi_new * GLICKO2_SCALE)
                    ratings[p_name] = Rating(r_cur.rating, new_rd, r_cur.sigma)

        # Aggregate player statistics for this format
        if player_count and player_count > 0:
            stats_rows = conn.execute("""
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
                INNER JOIN pairwise_matches pm ON (p.name = pm.player_a OR p.name = pm.player_b) AND pm.player_count = ?
                GROUP BY p.name
            """, (player_count,)).fetchall()
        else:
            stats_rows = conn.execute("""
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
                LEFT JOIN pairwise_matches pm ON p.name = pm.player_a OR p.name = pm.player_b
                GROUP BY p.name
            """).fetchall()
        player_stats = {r['name']: r for r in stats_rows}

        # Rank players by conservative rating C = rating - 3*rd
        player_rankings = []
        for p_name, r_obj in ratings.items():
            st = player_stats.get(p_name)
            opps = st['opps_count'] if st else 0
            if player_count and player_count > 0 and opps == 0:
                continue
            w_cnt = st['wins'] if st else 0
            l_cnt = st['losses'] if st else 0
            d_cnt = st['draws'] if st else 0
            wr = round((w_cnt + 0.5 * d_cnt) / max(1, opps) * 100.0, 2)
            lp = st['last_played'] if st else None

            player_rankings.append({
                'name': p_name,
                'rating': round(r_obj.rating, 2),
                'rd': round(r_obj.rd, 2),
                'sigma': round(r_obj.sigma, 4),
                'c_rating': round(r_obj.conservative_rating, 2),
                'opps': opps,
                'wins': w_cnt,
                'losses': l_cnt,
                'draws': d_cnt,
                'win_rate': wr,
                'last_played': lp
            })

        player_rankings.sort(key=lambda x: x['c_rating'], reverse=True)

        rating_rows = []
        for rank, p_data in enumerate(player_rankings, 1):
            rating_rows.append((
                model_type, player_count, p_data['name'], p_data['rating'], p_data['rd'],
                p_data['sigma'], p_data['c_rating'], rank, 0,
                p_data['opps'], p_data['opps'], p_data['wins'], p_data['losses'],
                p_data['draws'], p_data['win_rate'], p_data['last_played']
            ))

        with conn:
            conn.execute('DELETE FROM player_ratings WHERE model_type = ? AND player_count = ?;', (model_type, player_count))
            conn.execute('DELETE FROM rating_history WHERE model_type = ? AND player_count = ?;', (model_type, player_count))

            conn.executemany("""
                INSERT INTO player_ratings (
                    model_type, player_count, player_name, rating, rd, sigma, c_rating, rank, rank_delta,
                    games_played, opponents_count, wins, losses, draws, win_rate, last_played
                ) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
            """, rating_rows)

            conn.executemany("""
                INSERT INTO rating_history (
                    model_type, player_count, player_name, period_date, rating, rd, c_rating
                ) VALUES (?, ?, ?, ?, ?, ?, ?)
            """, history_records)

        fmt_label = f"{player_count}p" if player_count > 0 else "All"
        print(f"Successfully saved {len(rating_rows)} ratings and {len(history_records)} history points for {model_type} ({fmt_label}).")
        return ratings

    finally:
        conn.close()
