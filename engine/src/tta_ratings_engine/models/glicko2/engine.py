"""Glicko-2 rating system engine supporting standard and MP-weighted modes."""
from dataclasses import dataclass
import math
from typing import List, Tuple, Optional

# Constants
DEFAULT_RATING = 1500.0
DEFAULT_RD = 350.0
DEFAULT_SIGMA = 0.06
DEFAULT_TAU = 0.5  # Standard Glicko-2 tau (0.3 - 1.2)
GLICKO2_SCALE = 173.7178
MAX_RD = 350.0
MIN_RD = 20.0

@dataclass
class Rating:
    rating: float = DEFAULT_RATING
    rd: float = DEFAULT_RD
    sigma: float = DEFAULT_SIGMA

    @property
    def mu(self) -> float:
        return (self.rating - DEFAULT_RATING) / GLICKO2_SCALE

    @property
    def phi(self) -> float:
        return self.rd / GLICKO2_SCALE

    @property
    def conservative_rating(self) -> float:
        return self.rating - 3.0 * self.rd


def g(phi: float) -> float:
    return 1.0 / math.sqrt(1.0 + (3.0 * phi * phi) / (math.pi * math.pi))


def E(mu: float, mu_j: float, phi_j: float) -> float:
    return 1.0 / (1.0 + math.exp(-g(phi_j) * (mu - mu_j)))


def apply_inactivity(player: Rating, periods: int = 1) -> Rating:
    if periods <= 0:
        return Rating(player.rating, player.rd, player.sigma)
    phi = player.phi
    sigma = player.sigma
    new_phi = math.sqrt(phi * phi + periods * (sigma * sigma))
    new_rd = min(new_phi * GLICKO2_SCALE, MAX_RD)
    return Rating(player.rating, new_rd, player.sigma)


def update_rating(
    player: Rating,
    matches: List[Tuple[Rating, float, float]],  # (opponent, outcome s_j, weight w_j)
    tau: float = DEFAULT_TAU,
    weighted: bool = False
) -> Rating:
    """Update a player's rating given a list of matches in a rating period.
    
    matches: list of (opponent_rating, outcome, weight)
      outcome: 1.0 for win, 0.5 for draw, 0.0 for loss
      weight: fractional match weight (e.g. 1.0 / (N - 1))
    weighted: if False, weight is ignored (treated as 1.0)
    """
    if not matches:
        return apply_inactivity(player, periods=1)

    mu = player.mu
    phi = player.phi
    sigma = player.sigma

    # Step 3: Compute variance v and score discrepancy delta
    v_inv = 0.0
    delta_sum = 0.0

    for opp, outcome, weight in matches:
        w = weight if weighted else 1.0
        opp_mu = opp.mu
        opp_phi = opp.phi
        g_val = g(opp_phi)
        e_val = E(mu, opp_mu, opp_phi)

        v_inv += w * (g_val * g_val) * e_val * (1.0 - e_val)
        delta_sum += w * g_val * (outcome - e_val)

    if v_inv <= 0.0:
        return apply_inactivity(player, periods=1)

    v = 1.0 / v_inv
    delta = v * delta_sum

    # Step 5: Determine new volatility sigma' using Illinois bracket algorithm
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

    # Bracket search (Illinois algorithm)
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

    new_sigma = math.exp(B / 2.0)

    # Step 6: Update rating deviation to new pre-rating period value
    phi_star = math.sqrt(phi * phi + new_sigma * new_sigma)

    # Step 7: Update rating and RD
    new_phi = 1.0 / math.sqrt((1.0 / (phi_star * phi_star)) + (1.0 / v))
    new_mu = mu + (new_phi * new_phi) * delta_sum

    new_rating = new_mu * GLICKO2_SCALE + DEFAULT_RATING
    new_rd = max(new_phi * GLICKO2_SCALE, MIN_RD)

    return Rating(new_rating, new_rd, new_sigma)
