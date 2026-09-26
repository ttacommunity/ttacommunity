"""Tournament Badge Derivation Engine for Through the Ages.

Implements the official badge award rules:
- Recency Rule: Badges are displayed only for players active within the last 6 months in tournament play.
- Highest league in Intermezzo (3P), International Championship (4P), Royal League (2P), or World Championship -> GM (Grandmaster)
- 2nd highest league -> M (Master)
- 3rd highest league -> P (Platinum)
- 4th highest league -> G (Gold)
- 5th highest league -> S (Silver)
- 6th highest league -> B (Bronze)
- 7th highest league -> W (Wood)
- The Reigning World Champion retains the exclusive 'WC' title.
- Hover Reason: Shows the latest tournament division and season (e.g. 'Played in International Championship Grandmaster Season 6').
"""
import re
import sqlite3
from typing import Dict, List, Tuple, Optional, Any
from datetime import datetime
from dateutil.relativedelta import relativedelta
from pathlib import Path
from src.data.db import get_connection

# Tournament division tier rankings (1 = Highest / GM, 2 = M, 3 = P, 4 = G, 5 = S, 6 = B, 7 = W)
TIER_TITLE_MAP = {
    1: 'GM',
    2: 'M',
    3: 'P',
    4: 'G',
    5: 'S',
    6: 'B',
    7: 'W'
}

TIER_NAMES = {
    1: 'Grandmaster',
    2: 'Master',
    3: 'Platinum',
    4: 'Gold',
    5: 'Silver',
    6: 'Bronze',
    7: 'Wood'
}

TITLE_ORDER = {'WC': 0, 'GM': 1, 'M': 2, 'P': 3, 'G': 4, 'S': 5, 'B': 6, 'W': 7}

# Reigning World Champion holding active 'WC' title
REIGNING_WC_PLAYERS = {'a440'}


def classify_division_tier(tournament_name: str) -> Optional[int]:
    """Classifies a match's tournament/division string into a tier from 1 (highest) to 7 (wood).
    
    Strictly restricted to:
    - International Championship (4P)
    - Intermezzo Championship (3P)
    - Royal League (2P)
    
    Returns None if the tournament is not one of these three, or if no valid division tier is present.
    """
    t_lower = tournament_name.lower().strip()

    # 1. Royal League (2P)
    # Tier 1: Emperor
    # Tier 2: King
    # Tier 3: Prince
    # Tier 4: Duke
    # Tier 5: Marquess
    # Tier 6: Count
    # Tier 7: Viscount / Baron / Knight
    if 'rl_' in t_lower or 'royal league' in t_lower:
        if 'emperor' in t_lower:
            return 1
        elif 'king' in t_lower:
            return 2
        elif 'prince' in t_lower:
            return 3
        elif 'duke' in t_lower:
            return 4
        elif 'marquess' in t_lower:
            return 5
        elif 'viscount' in t_lower or 'baron' in t_lower or 'knight' in t_lower:
            return 7
        elif 'count' in t_lower:
            return 6
        
        m_div = re.search(r'div(?:ision)?\s*(\d+)', t_lower)
        if m_div:
            d_num = int(m_div.group(1))
            return min(7, max(1, d_num))
        return None

    # 2. International Championship (4P)
    # Tier 1: Diamond / Master 1 / Premier / Grandmaster
    # Tier 2: Platinum / Div 2 / Master 2
    # Tier 3: Gold / Div 3
    # Tier 4: Silver / Div 4
    # Tier 5: Bronze / Div 5
    # Tier 6: Div 6
    # Tier 7: Wood / Div 7
    if 'international' in t_lower:
        if 'diamond' in t_lower or 'grandmaster' in t_lower or 'master 1' in t_lower or 'tier 1' in t_lower:
            return 1
        elif 'platinum' in t_lower or 'master 2' in t_lower or 'div 2' in t_lower or 'division 2' in t_lower or 'tier 2' in t_lower:
            return 2
        elif 'gold' in t_lower or 'div 3' in t_lower or 'division 3' in t_lower or 'tier 3' in t_lower:
            return 3
        elif 'silver' in t_lower or 'div 4' in t_lower or 'division 4' in t_lower or 'tier 4' in t_lower:
            return 4
        elif 'bronze' in t_lower or 'div 5' in t_lower or 'division 5' in t_lower or 'tier 5' in t_lower:
            return 5
        elif 'div 6' in t_lower or 'division 6' in t_lower or 'tier 6' in t_lower:
            return 6
        elif 'wood' in t_lower or 'div 7' in t_lower or 'division 7' in t_lower or 'tier 7' in t_lower:
            return 7
        return None

    # 3. Intermezzo Championship (3P)
    # Tier 1: Diamond / Master 1 / Grandmaster / Master (when top tier)
    # Tier 2: Platinum / Master 2 / Div 2
    # Tier 3: Gold / Div 3
    # Tier 4: Silver / Div 4
    # Tier 5: Bronze / Div 5
    # Tier 6: Div 6
    # Tier 7: Wood / Div 7
    if 'intermezzo' in t_lower:
        if 'diamond' in t_lower or 'grandmaster' in t_lower or 'master 1' in t_lower or 'tier 1' in t_lower:
            return 1
        elif 'platinum' in t_lower or 'master 2' in t_lower or 'div 2' in t_lower or 'division 2' in t_lower or 'tier 2' in t_lower:
            return 2
        elif 'gold' in t_lower or 'div 3' in t_lower or 'division 3' in t_lower or 'tier 3' in t_lower:
            return 3
        elif 'silver' in t_lower or 'div 4' in t_lower or 'division 4' in t_lower or 'tier 4' in t_lower:
            return 4
        elif 'bronze' in t_lower or 'div 5' in t_lower or 'division 5' in t_lower or 'tier 5' in t_lower:
            return 5
        elif 'div 6' in t_lower or 'division 6' in t_lower or 'tier 6' in t_lower:
            return 6
        elif 'wood' in t_lower or 'div 7' in t_lower or 'division 7' in t_lower or 'tier 7' in t_lower:
            return 7
        return None

    return None


def parse_badge_reason(tournament_name: str, tier: int) -> str:
    """Parses tournament string into a clean, human-readable reason message."""
    t = tournament_name.strip()
    t_lower = t.lower()
    
    # Extract season number
    season_match = re.search(r'(?:s|season|edition)\s*0*(\d+)', t_lower)
    season_num = season_match.group(1) if season_match else None
    
    tier_name = TIER_NAMES.get(tier, 'Wood')
    
    if 'rl_' in t_lower or 'royal league' in t_lower:
        rl_title = "Emperor" if tier == 1 else ("King" if tier == 2 else ("Prince" if tier == 3 else ("Duke" if tier == 4 else ("Marquess" if tier == 5 else ("Count" if tier == 6 else "Viscount")))))
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in Royal League {rl_title} {s_part}".strip()
    
    elif 'international' in t_lower:
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in International Championship {tier_name} {s_part}".strip()
        
    elif 'intermezzo' in t_lower:
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in Intermezzo Championship {tier_name} {s_part}".strip()
        
    return f"Played in {t}"


def derive_tournament_badges(db_path: Optional[str] = None) -> Dict[str, Any]:
    """Derives and saves tournament badges for all active players within 6 months."""
    conn = get_connection(db_path)
    try:
        # Determine 6-month recency cutoff based on latest match in DB
        max_row = conn.execute("SELECT MAX(date) FROM matches").fetchone()
        if not max_row or not max_row[0]:
            return {}
        max_date = datetime.strptime(max_row[0], "%Y-%m-%d")
        cutoff_date = (max_date - relativedelta(months=6)).strftime("%Y-%m-%d")

        # Select matches played within last 6 months
        rows = conn.execute("""
            SELECT tournament, date, player1, player2, player3, player4
            FROM matches
            WHERE date >= ? AND tournament IS NOT NULL
            ORDER BY date DESC
        """, (cutoff_date,)).fetchall()

        player_best = {}
        player_tier_counts = {}

        for r in rows:
            tourney = r['tournament']
            tier = classify_division_tier(tourney)
            if tier is None:
                continue

            participants = [r['player1'], r['player2']]
            if r['player3']:
                participants.append(r['player3'])
            if r['player4']:
                participants.append(r['player4'])

            for p in participants:
                if not p:
                    continue
                if p not in player_best or tier < player_best[p]['tier'] or (tier == player_best[p]['tier'] and r['date'] > player_best[p]['date']):
                    player_best[p] = {
                        'tier': tier,
                        'tournament': tourney,
                        'date': r['date'],
                        'reason': parse_badge_reason(tourney, tier)
                    }

                if tier == 1:
                    player_tier_counts[p] = player_tier_counts.get(p, 0) + 1

        derived_data = {}
        for p, info in player_best.items():
            title = TIER_TITLE_MAP.get(info['tier'], 'W')
            reason = info['reason']
            derived_data[p] = {
                'title': title,
                'title_count': player_tier_counts.get(p, 1 if title == 'GM' else 0),
                'badge_reason': reason
            }

        # Reigning World Champion holds exclusive active WC badge
        for p in REIGNING_WC_PLAYERS:
            derived_data[p] = {
                'title': 'WC',
                'title_count': 1,
                'badge_reason': 'Reigning World Champion (World Championship 2025)'
            }

        # Clear badges for all players first (inactive players lose badge)
        conn.execute("UPDATE players SET title = NULL, title_count = 0, badge_reason = NULL")

        # Update active players
        for p, data in derived_data.items():
            conn.execute("""
                UPDATE players
                SET title = ?, title_count = ?, badge_reason = ?
                WHERE name = ?
            """, (data['title'], data['title_count'], data['badge_reason'], p))

        conn.commit()
        return derived_data
    finally:
        conn.close()


if __name__ == '__main__':
    res = derive_tournament_badges()
    print(f"Derived active tournament badges for {len(res)} players.")
