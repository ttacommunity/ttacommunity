"""WHR rating calculator and persistence for TTA-Glicko2-WHR."""
from datetime import datetime
import sqlite3
from typing import Optional, Dict

from src.data.db import get_connection
from src.models.whr.engine import WHREngine, DEFAULT_W2_PER_DAY
from src.models.glicko2.calculator import (
    get_tier_mean,
    RESET_ALPHA_SOFT, RESET_LAMBDA_SOFT,
    RESET_ALPHA_SOFTER, RESET_LAMBDA_SOFTER,
    RESET_ALPHA_HARD, RESET_LAMBDA_HARD
)

def compute_whr_ratings(
    w2_per_day: float = DEFAULT_W2_PER_DAY,
    player_count: int = 0,
    max_iter: int = 7,
    tol: float = 1e-3,
    reset_mode: str = 'continuous',
    db_path: Optional[str] = None,
    verbose: bool = True
):
    model_type = f"whr_{reset_mode}" if reset_mode != 'continuous' else 'whr'
    conn = get_connection(db_path)

    try:
        fmt_label = f"{player_count}p" if player_count > 0 else "All"
        if verbose:
            print(f'Loading pairwise matches for WHR ({fmt_label})...')

        if player_count and player_count > 0:
            rows = conn.execute("""
                SELECT date, player_a, player_b, outcome_a, weight
                FROM pairwise_matches
                WHERE player_count = ? AND glicko_eligible = 1
                ORDER BY date, pairwise_id
            """, (player_count,)).fetchall()
        else:
            rows = conn.execute("""
                SELECT date, player_a, player_b, outcome_a, weight
                FROM pairwise_matches
                WHERE glicko_eligible = 1
                ORDER BY date, pairwise_id
            """).fetchall()

        whr = WHREngine(w2_per_day=w2_per_day)
        for r in rows:
            whr.add_game(r['date'], r['player_a'], r['player_b'], r['outcome_a'], r['weight'])

        if verbose:
            print(f'Fitting WHR across {len(whr.players)} players over {len(rows)} matches...')

        whr.fit(max_iter=max_iter, tol=tol, verbose=verbose)

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

        player_rankings = []
        history_records = []
        max_day = whr.get_max_day()

        for p_name in whr.players:
            cur = whr.get_current_rating(p_name, target_day=max_day)
            if not cur:
                continue
            r_val, rd_val = cur[0], cur[1]
            if reset_mode == 'softer':
                tier_mean = get_tier_mean(r_val)
                r_val = (1.0 - RESET_LAMBDA_SOFTER) * r_val + RESET_LAMBDA_SOFTER * tier_mean
                rd_val = min(350.0, rd_val * RESET_ALPHA_SOFTER)
            elif reset_mode == 'soft':
                tier_mean = get_tier_mean(r_val)
                r_val = (1.0 - RESET_LAMBDA_SOFT) * r_val + RESET_LAMBDA_SOFT * tier_mean
                rd_val = min(350.0, rd_val * RESET_ALPHA_SOFT)
            elif reset_mode == 'amplified':
                tier_mean = get_tier_mean(r_val)
                r_val = (1.0 - RESET_LAMBDA_HARD) * r_val + RESET_LAMBDA_HARD * tier_mean
                rd_val = min(350.0, rd_val * RESET_ALPHA_HARD)
            c_val = r_val - 3.0 * rd_val

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
                'rating': round(r_val, 2),
                'rd': round(rd_val, 2),
                'sigma': round(w2_per_day, 4),
                'c_rating': round(c_val, 2),
                'opps': opps,
                'wins': w_cnt,
                'losses': l_cnt,
                'draws': d_cnt,
                'win_rate': wr,
                'last_played': lp
            })

            # Sample rating history: save monthly or key points to keep size balanced
            p_history = whr.get_ratings(p_name)
            # Sample every 30 days or all if < 50
            if len(p_history) <= 50:
                sampled = p_history
            else:
                step = max(1, len(p_history) // 50)
                sampled = p_history[::step]
                if p_history[-1] not in sampled:
                    sampled.append(p_history[-1])

            for d_str, hist_r, hist_rd in sampled:
                if reset_mode == 'softer':
                    tm = get_tier_mean(hist_r)
                    hist_r = (1.0 - RESET_LAMBDA_SOFTER) * hist_r + RESET_LAMBDA_SOFTER * tm
                    hist_rd = min(350.0, hist_rd * RESET_ALPHA_SOFTER)
                elif reset_mode == 'soft':
                    tm = get_tier_mean(hist_r)
                    hist_r = (1.0 - RESET_LAMBDA_SOFT) * hist_r + RESET_LAMBDA_SOFT * tm
                    hist_rd = min(350.0, hist_rd * RESET_ALPHA_SOFT)
                elif reset_mode == 'amplified':
                    tm = get_tier_mean(hist_r)
                    hist_r = (1.0 - RESET_LAMBDA_HARD) * hist_r + RESET_LAMBDA_HARD * tm
                    hist_rd = min(350.0, hist_rd * RESET_ALPHA_HARD)
                history_records.append((
                    model_type, player_count, p_name, d_str,
                    round(hist_r, 2), round(hist_rd, 2), round(hist_r - 3.0 * hist_rd, 2)
                ))

        # Sort by conservative rating
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

        if verbose:
            fmt_label = f"{player_count}p" if player_count > 0 else "All"
            print(f"Successfully saved {len(rating_rows)} ratings and {len(history_records)} history points for WHR ({fmt_label}).")

        return whr
    finally:
        conn.close()
