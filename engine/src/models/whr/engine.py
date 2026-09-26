"""Whole-History Rating (WHR) engine based on R?mi Coulom (2008), highly optimized."""
import bisect
from dataclasses import dataclass, field
from datetime import datetime
import math
from typing import Dict, List, Tuple, Optional
import numpy as np

from src.models.whr.solver import solve_tridiagonal

GLICKO2_SCALE = 173.7178
DEFAULT_RATING = 1500.0
DEFAULT_RD = 350.0
DEFAULT_W2_PER_DAY = 0.002
DEFAULT_PRIOR_VAR = 2.0  # prior variance on initial skill (~245 RD)

# Golden Retrospective Decay Constants (3-Year Informational Half-Life)
# Golden ratio phi = (1 + sqrt(5)) / 2 ≈ 1.6180339887
# Continuous decay rate: lambda = ln(phi) / (3.0 * 365.25) ≈ 0.00043917 day^-1
PHI = (1.0 + math.sqrt(5.0)) / 2.0
GOLDEN_RATIO_INV = 1.0 / PHI
GOLDEN_LAMBDA = math.log(PHI) / (3.0 * 365.25)

@dataclass
class PlayerDay:
    day: int
    date_str: str
    r: float = 0.0
    var: float = 1.0
    matches: List[Tuple[str, float, float]] = field(default_factory=list)  # (opp_name, outcome, weight)


class WHRPlayer:
    def __init__(self, name: str):
        self.name = name
        self.days: Dict[int, PlayerDay] = {}
        self.ordered_days: List[int] = []

    def add_match(self, day: int, date_str: str, opp_name: str, outcome: float, weight: float = 1.0):
        if day not in self.days:
            self.days[day] = PlayerDay(day=day, date_str=date_str)
            self.ordered_days = sorted(self.days.keys())
        self.days[day].matches.append((opp_name, outcome, weight))


class WHREngine:
    def __init__(self, w2_per_day: float = DEFAULT_W2_PER_DAY):
        self.w2 = w2_per_day
        self.players: Dict[str, WHRPlayer] = {}
        self.min_date: Optional[datetime] = None
        self.max_day: int = 0

    def _date_to_day(self, date_str: str) -> int:
        dt = datetime.strptime(date_str[:10], "%Y-%m-%d")
        if self.min_date is None:
            self.min_date = dt
        d = (dt - self.min_date).days
        if d > self.max_day:
            self.max_day = d
        return d

    def add_game(self, date_str: str, p1: str, p2: str, outcome_p1: float, weight: float = 1.0):
        d = self._date_to_day(date_str)
        if p1 not in self.players:
            self.players[p1] = WHRPlayer(p1)
        if p2 not in self.players:
            self.players[p2] = WHRPlayer(p2)

        self.players[p1].add_match(d, date_str, p2, outcome_p1, weight)
        self.players[p2].add_match(d, date_str, p1, 1.0 - outcome_p1, weight)

    def iterate(self) -> float:
        """Perform one full Newton-Raphson pass across all players. Returns max delta."""
        max_delta = 0.0

        for player_name, player in self.players.items():
            days = player.ordered_days
            n = len(days)
            if n == 0:
                continue

            r_vec = np.array([player.days[d].r for d in days], dtype=float)

            # 1. Likelihood gradient and negative Hessian
            grad_lik = np.zeros(n, dtype=float)
            hess_lik = np.zeros(n, dtype=float)

            for i, d in enumerate(days):
                pday = player.days[d]
                r_i = pday.r
                for opp_name, outcome, weight in pday.matches:
                    opp = self.players[opp_name]
                    opp_days = opp.ordered_days
                    # Binary search using bisect_right
                    pos = bisect.bisect_right(opp_days, d) - 1
                    pos = max(0, min(pos, len(opp_days) - 1))
                    opp_r = opp.days[opp_days[pos]].r

                    # Bradley-Terry: P(win) = 1 / (1 + exp(-(r_i - opp_r)))
                    diff = r_i - opp_r
                    if diff > 35.0:
                        p_win = 1.0
                    elif diff < -35.0:
                        p_win = 0.0
                    else:
                        gamma = math.exp(diff)
                        p_win = gamma / (1.0 + gamma)

                    grad_lik[i] += weight * (outcome - p_win)
                    hess_lik[i] += weight * (p_win * (1.0 - p_win))

            # 2. Prior precision matrix (tridiagonal) with Canonical Coulom Brownian Precision
            diag_prior = np.zeros(n, dtype=float)
            off_prior = np.zeros(n - 1, dtype=float)

            if n == 1:
                diag_prior[0] = 1.0 / DEFAULT_PRIOR_VAR
            else:
                for k in range(n - 1):
                    dt = max(1, days[k + 1] - days[k])
                    # Canonical Coulom Brownian motion prior precision: inv_sigma2 = 1.0 / (w2 * dt)
                    inv_sigma2 = 1.0 / (self.w2 * dt)

                    diag_prior[k] += inv_sigma2
                    diag_prior[k + 1] += inv_sigma2
                    off_prior[k] = -inv_sigma2

                # Add prior variance to root
                diag_prior[0] += 1.0 / DEFAULT_PRIOR_VAR

            # Compute prior * r
            prior_r = np.zeros(n, dtype=float)
            prior_r[0] = diag_prior[0] * r_vec[0] + (off_prior[0] * r_vec[1] if n > 1 else 0.0)
            for k in range(1, n - 1):
                prior_r[k] = off_prior[k - 1] * r_vec[k - 1] + diag_prior[k] * r_vec[k] + off_prior[k] * r_vec[k + 1]
            if n > 1:
                prior_r[n - 1] = off_prior[n - 2] * r_vec[n - 2] + diag_prior[n - 1] * r_vec[n - 1]

            diag_total = hess_lik + diag_prior
            rhs_total = grad_lik - prior_r

            delta_r = solve_tridiagonal(diag_total, off_prior, rhs_total)

            # Update r and approximate variance
            for i, d in enumerate(days):
                player.days[d].r += delta_r[i]
                player.days[d].var = 1.0 / max(diag_total[i], 1e-4)

            max_delta = max(max_delta, float(np.max(np.abs(delta_r))))

        return max_delta

    def fit(self, max_iter: int = 10, tol: float = 1e-3, verbose: bool = False):
        for it in range(max_iter):
            max_delta = self.iterate()
            if verbose:
                print(f'WHR Iteration {it + 1}: max_delta = {max_delta:.5f}')
            if max_delta < tol:
                break

    def get_max_day(self) -> int:
        return self.max_day

    def get_ratings(self, player_name: str) -> List[Tuple[str, float, float]]:
        if player_name not in self.players:
            return []
        p = self.players[player_name]
        res = []
        for d in p.ordered_days:
            pday = p.days[d]
            r_scale = pday.r * GLICKO2_SCALE + DEFAULT_RATING
            rd_scale = math.sqrt(pday.var) * GLICKO2_SCALE
            res.append((pday.date_str, r_scale, rd_scale))
        return res

    def get_current_rating(self, player_name: str, target_day: Optional[int] = None) -> Optional[Tuple[float, float]]:
        if player_name not in self.players or not self.players[player_name].ordered_days:
            return None
        p = self.players[player_name]
        last_d = p.ordered_days[-1]
        pday = p.days[last_d]
        r_scale = pday.r * GLICKO2_SCALE + DEFAULT_RATING
        var = pday.var

        # Forward Brownian projection into target_day (t_now) for active leaderboard
        if target_day is not None and target_day > last_d:
            dt = target_day - last_d
            var += self.w2 * dt

        rd_scale = min(DEFAULT_RD, math.sqrt(var) * GLICKO2_SCALE)
        return (r_scale, rd_scale)

    def get_rating_on_date(self, player_name: str, date_str: str) -> Optional[Tuple[float, float]]:
        """Returns (rating, rd) on Glicko scale for player_name on a given match date."""
        if player_name not in self.players or not self.players[player_name].ordered_days:
            return None
        p = self.players[player_name]
        d = self._date_to_day(date_str)
        if d in p.days:
            pday = p.days[d]
            return (pday.r * GLICKO2_SCALE + DEFAULT_RATING, math.sqrt(pday.var) * GLICKO2_SCALE)

        # Find closest prior day
        prior_days = [day for day in p.ordered_days if day <= d]
        if prior_days:
            target_d = prior_days[-1]
            pday = p.days[target_d]
            dt = d - target_d
            var = pday.var + self.w2 * dt
            return (pday.r * GLICKO2_SCALE + DEFAULT_RATING, min(DEFAULT_RD, math.sqrt(var) * GLICKO2_SCALE))
        else:
            first_d = p.ordered_days[0]
            pday = p.days[first_d]
            return (pday.r * GLICKO2_SCALE + DEFAULT_RATING, math.sqrt(pday.var) * GLICKO2_SCALE)

