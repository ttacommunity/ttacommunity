"""Adaptive Temperature-Calibrated Glicko-2 Engine (Glicko-2 Adaptive-T).

Conceptualized and derived to eliminate upper-tail overconfidence and resolve
format-specific variance dispersion (2P vs 3P vs 4P) via an empirical temperature
scaling function T(P, N).
"""
import math
from typing import Dict, List, Tuple, Optional
from src.models.glicko2.engine import (
    Rating, GLICKO2_SCALE, DEFAULT_RATING, DEFAULT_RD, DEFAULT_SIGMA, MAX_RD, g, E, apply_inactivity
)

# Calibrated format-specific parameters (alpha_N, gamma_N)
# Derived via convex optimization to minimize ECE and derivative slope error on empirical match data:
ADAPTIVE_T_PARAMS: Dict[int, Tuple[float, float]] = {
    0: (0.12, 0.90),  # All formats aggregate (fallback / unified)
    2: (0.45, 0.80),  # 2-Player (higher temperature due to bilateral zero-sum variance)
    3: (0.18, 0.85),  # 3-Player (low temperature due to multilateral self-balancing equilibrium)
    4: (0.29, 0.75),  # 4-Player (simplex-constrained tournament dynamics)
}

# GlickoD Parameters:
# Empirical findings proved that 15-game emergent prior calibration (RPC) + bilateral composite variance
# eliminates tail overconfidence naturally without temperature scaling distortion (T = 1.0 everywhere):
DANEO_ADAPTIVE_T_PARAMS: Dict[int, Tuple[float, float]] = {
    0: (0.00, 0.90),  # All formats aggregate (T = 1.0)
    2: (0.00, 0.70),  # 2-Player (T = 1.0)
    3: (0.00, 0.70),  # 3-Player (T = 1.0)
    4: (0.00, 0.90),  # 4-Player (T = 1.0)
}


def get_adaptive_temperature(
    p_base: float,
    player_count: int = 0,
    params_dict: Optional[Dict[int, Tuple[float, float]]] = None
) -> float:
    """Calculates the adaptive temperature T(P, N) >= 1.0.
    
    p_base: Base logistic win probability for the favorite in [0.5, 1.0].
    player_count: Game format (2, 3, 4, or 0 for all).
    params_dict: Optional parameter mapping override (defaults to ADAPTIVE_T_PARAMS).
    """
    lookup = params_dict if params_dict is not None else ADAPTIVE_T_PARAMS
    alpha, gamma = lookup.get(player_count, lookup.get(0, (0.0, 1.0)))
    if alpha <= 0.0:
        return 1.0
    p_clamped = max(0.5, min(1.0, float(p_base)))
    dev = 2.0 * (p_clamped - 0.5)  # in [0.0, 1.0]
    return 1.0 + alpha * (dev ** gamma)


def calibrated_expectation(
    mu_a: float,
    mu_b: float,
    phi_b: float,
    player_count: int = 0,
    params_dict: Optional[Dict[int, Tuple[float, float]]] = None,
    phi_a: Optional[float] = None
) -> float:
    """Calculates calibrated match expectation E*(mu_a, mu_b, phi_b, N, phi_a).
    
    Guarantees exact unbiased 50/50 symmetry at mu_a == mu_b, uses bilateral composite
    uncertainty sqrt(phi_a^2 + phi_b^2) when phi_a is provided, and smoothly softens
    overconfidence in high-probability tails according to player count N.
    """
    if phi_a is not None:
        c = math.sqrt(phi_a ** 2 + phi_b ** 2)
        g_val = 1.0 / math.sqrt(1.0 + 3.0 * (c ** 2) / (math.pi ** 2))
    else:
        g_val = g(phi_b)
    x = g_val * (mu_a - mu_b)
    e_base = 1.0 / (1.0 + math.exp(-x))
    
    # Favorite probability
    p_fav = e_base if e_base >= 0.5 else (1.0 - e_base)
    temp = get_adaptive_temperature(p_fav, player_count=player_count, params_dict=params_dict)
    
    # Calibrated expectation
    return 1.0 / (1.0 + math.exp(-x / temp))


def update_rating_adaptive(
    player: Rating,
    matches: List[Tuple[Rating, float, float, int]],  # (opponent, outcome s_j, weight w_j, player_count N)
    tau: float = 0.3,
    params_dict: Optional[Dict[int, Tuple[float, float]]] = None
) -> Rating:
    """Updates a player's Glicko-2 rating using the Adaptive-T formulation."""
    if not matches:
        return apply_inactivity(player, periods=1)

    mu = player.mu
    phi = player.phi
    sigma = player.sigma

    v_inv = 0.0
    delta_sum = 0.0

    for opp, outcome, weight, p_count in matches:
        w = weight
        opp_mu = opp.mu
        opp_phi = opp.phi
        g_val = g(opp_phi)
        e_cal = calibrated_expectation(mu, opp_mu, opp_phi, player_count=p_count, params_dict=params_dict)

        # Variance contribution and discrepancy
        v_inv += w * (g_val * g_val) * e_cal * (1.0 - e_cal)
        delta_sum += w * g_val * (outcome - e_cal)

    if v_inv <= 0.0:
        return apply_inactivity(player, periods=1)

    v = 1.0 / v_inv
    delta = v * delta_sum

    # Illinois bracket optimization for sigma'
    a = math.log(sigma * sigma)

    def f(x: float) -> float:
        ex = math.exp(x)
        term1 = ex * (delta * delta - phi * phi - v - ex) / (2.0 * (phi * phi + v + ex) ** 2)
        term2 = (x - a) / (tau * tau)
        return term1 - term2

    A = a
    if delta * delta > phi * phi + v:
        B = math.log(delta * delta - phi * phi - v)
    else:
        k = 1
        while f(a - k * tau) < 0:
            k += 1
        B = a - k * tau

    fA = f(A)
    fB = f(B)

    iterations = 0
    while abs(B - A) > 1e-6 and iterations < 100:
        C = A + (A - B) * fA / (fB - fA)
        fC = f(C)
        if fC * fB < 0:
            A = B
            fA = fB
        else:
            fA = fA / 2.0
        B = C
        fB = fC
        iterations += 1

    new_sigma = math.exp(A / 2.0)
    phi_star = math.sqrt(phi * phi + new_sigma * new_sigma)
    new_phi = 1.0 / math.sqrt(1.0 / (phi_star * phi_star) + 1.0 / v)
    new_mu = mu + (new_phi * new_phi) * delta_sum

    new_rating = new_mu * GLICKO2_SCALE + DEFAULT_RATING
    new_rd = min(new_phi * GLICKO2_SCALE, MAX_RD)

    return Rating(new_rating, new_rd, new_sigma)
