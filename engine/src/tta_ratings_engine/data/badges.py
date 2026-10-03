"""Tournament Badge Derivation Engine for Through the Ages.

Implements the official badge award rules:
- Recency Rule: Badges are displayed only for players active within the last 1 year (12 months) in tournament play.
- Highest league in Intermezzo (3P), International Championship (4P), Royal League (2P), or World Championship -> GM (Grandmaster)
- 2nd highest league -> M (Master)
- 3rd highest league -> P (Platinum)
- 4th highest league -> G (Gold)
- 5th highest league -> S (Silver)
- 6th highest league -> B (Bronze)
- 7th highest league -> W (Wood)
- A win in the top division of the International, Intermezzo or Royal League
  (a first place or gold in the run's tournament records) -> SGM (Super GM)
- The Reigning World Champion retains the exclusive 'WC' title.
- Hover Reason: Shows the latest tournament division and season (e.g. 'Played in International Championship Grandmaster Season 6').
"""

import re
from datetime import date
from typing import Any

import psycopg
from dateutil.relativedelta import relativedelta

from tta_ratings_engine.data.db import require_running

# Tournament division tier rankings (1 = Highest / GM, 2 = M, 3 = P, 4 = G, 5 = S, 6 = B, 7 = W)
TIER_TITLE_MAP = {1: "GM", 2: "M", 3: "P", 4: "G", 5: "S", 6: "B", 7: "W"}

TIER_NAMES = {
    1: "Grandmaster",
    2: "Master",
    3: "Platinum",
    4: "Gold",
    5: "Silver",
    6: "Bronze",
    7: "Wood",
}

TITLE_ORDER = {
    "WC": 0,
    "SGM": 1,
    "GM": 2,
    "M": 3,
    "P": 4,
    "G": 5,
    "S": 6,
    "B": 7,
    "W": 8,
}

# The leagues and divisions whose win makes a Super GM, as the records name them
SGM_TOURNAMENTS = (
    "International Championship",
    "Intermezzo Championship",
    "Royal League",
)
SGM_DIVISIONS = ("Grandmaster", "Diamond", "Emperor")


def world_champions(conn) -> list[tuple[str, int]]:
    """Returns each World Championship season's winner, as recorded, oldest first: (player, season)."""
    rows = conn.execute(
        "SELECT player_name, season FROM tournament_official_placements "
        "WHERE series = 'worlds' AND place = 1 ORDER BY season, player_name"
    ).fetchall()
    return [(r["player_name"], r["season"]) for r in rows]


def classify_division_tier(tournament_name: str) -> int | None:
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
    if "rl_" in t_lower or "royal league" in t_lower:
        if "emperor" in t_lower:
            return 1
        elif "king" in t_lower:
            return 2
        elif "prince" in t_lower:
            return 3
        elif "duke" in t_lower:
            return 4
        elif "marquess" in t_lower:
            return 5
        elif "viscount" in t_lower or "baron" in t_lower or "knight" in t_lower:
            return 7
        elif "count" in t_lower:
            return 6

        m_div = re.search(r"div(?:ision)?\s*(\d+)", t_lower)
        if m_div:
            d_num = int(m_div.group(1))
            return min(7, max(1, d_num))
        return None

    # 2. International Championship (4P)
    # Tier 1: Grandmaster (GM) / Diamond / Tier 1
    # Tier 2: Master (M) / Master 1 / Master 2 / Div 2 / Division 2 / Tier 2
    # Tier 3: Platinum (P) / Div 3 / Division 3 / Tier 3
    # Tier 4: Gold (G) / Div 4 / Division 4 / Tier 4
    # Tier 5: Silver (S) / Div 5 / Division 5 / Tier 5
    # Tier 6: Bronze (B) / Div 6 / Division 6 / Tier 6
    # Tier 7: Wood (W) / Div 7 / Division 7 / Tier 7
    if "international" in t_lower:
        return _championship_tier(t_lower)

    # 3. Intermezzo Championship (3P), with the same divisions
    if "intermezzo" in t_lower:
        return _championship_tier(t_lower)

    return None


def _championship_tier(t_lower: str) -> int | None:
    """Returns the tier of an International or Intermezzo division, or None."""
    if "grandmaster" in t_lower or "diamond" in t_lower or "tier 1" in t_lower:
        return 1
    elif (
        "master" in t_lower
        or "tier 2" in t_lower
        or "div 2" in t_lower
        or "division 2" in t_lower
    ):
        return 2
    elif (
        "platinum" in t_lower
        or "tier 3" in t_lower
        or "div 3" in t_lower
        or "division 3" in t_lower
    ):
        return 3
    elif (
        "gold" in t_lower
        or "tier 4" in t_lower
        or "div 4" in t_lower
        or "division 4" in t_lower
    ):
        return 4
    elif (
        "silver" in t_lower
        or "tier 5" in t_lower
        or "div 5" in t_lower
        or "division 5" in t_lower
    ):
        return 5
    elif (
        "bronze" in t_lower
        or "tier 6" in t_lower
        or "div 6" in t_lower
        or "division 6" in t_lower
    ):
        return 6
    elif (
        "wood" in t_lower
        or "tier 7" in t_lower
        or "div 7" in t_lower
        or "division 7" in t_lower
    ):
        return 7
    return None


def parse_badge_reason(tournament_name: str, tier: int) -> str:
    """Parses tournament string into a clean, human-readable reason message."""
    t = tournament_name.strip()
    t_lower = t.lower()

    # Extract season number
    season_match = re.search(r"(?:s|season|edition)\s*0*(\d+)", t_lower)
    season_num = season_match.group(1) if season_match else None

    tier_name = TIER_NAMES.get(tier, "Wood")

    if "rl_" in t_lower or "royal league" in t_lower:
        if "baron" in t_lower:
            rl_title = "Baron"
        elif "knight" in t_lower:
            rl_title = "Knight"
        elif "viscount" in t_lower:
            rl_title = "Viscount"
        else:
            rl_title = {
                1: "Emperor",
                2: "King",
                3: "Prince",
                4: "Duke",
                5: "Marquess",
                6: "Count",
            }.get(tier, "Viscount")
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in Royal League {rl_title} {s_part}".strip()

    elif "international" in t_lower:
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in International Championship {tier_name} {s_part}".strip()

    elif "intermezzo" in t_lower:
        s_part = f"Season {season_num}" if season_num else ""
        return f"Played in Intermezzo Championship {tier_name} {s_part}".strip()

    return f"Played in {t}"


def derive_tournament_badges(
    *, conn: psycopg.Connection, run_id: int
) -> dict[str, Any]:
    """Derives tournament badges for players active within the last year, and returns them.

    Replaces the run's player_titles (active titles), player_peak_titles
    (career peaks, inactive players too) and player_yearly_titles (best title
    each year) with those of known players, in one committed transaction. Raises RunNotRunning unless the run is running.
    """
    with conn.transaction():
        require_running(conn, run_id)
        # Determine 1-year (12-month) recency cutoff based on latest match in DB
        max_row = conn.execute(
            "SELECT to_char(MAX(date), 'YYYY-MM-DD') AS max_date FROM matches WHERE state = 'played'"
        ).fetchone()
        if not max_row or not max_row["max_date"]:
            return {}
        max_date = date.fromisoformat(max_row["max_date"])
        cutoff_date = (max_date - relativedelta(years=1)).strftime("%Y-%m-%d")

        # All tournament matches: active badges (last year) and career peaks (all-time)
        rows = conn.execute(
            """
            SELECT tournament, to_char(date, 'YYYY-MM-DD') AS date, player1, player2, player3, player4
            FROM matches
            WHERE tournament IS NOT NULL AND state = 'played'
            ORDER BY matches.date DESC
        """
        ).fetchall()

        player_best = {}
        player_career_best = {}
        player_tier_counts = {}
        player_year_best = {}

        for r in rows:
            tourney = r["tournament"]
            tier = classify_division_tier(tourney)
            if tier is None:
                continue

            d_str = r["date"]
            yr = int(d_str[:4])

            participants = [r["player1"], r["player2"]]
            if r["player3"]:
                participants.append(r["player3"])
            if r["player4"]:
                participants.append(r["player4"])

            for p in participants:
                if not p:
                    continue

                if tier < player_year_best.get((p, yr), 8):
                    player_year_best[(p, yr)] = tier

                if d_str >= cutoff_date:
                    if (
                        p not in player_best
                        or tier < player_best[p]["tier"]
                        or (
                            tier == player_best[p]["tier"]
                            and d_str > player_best[p]["date"]
                        )
                    ):
                        player_best[p] = {
                            "tier": tier,
                            "tournament": tourney,
                            "date": d_str,
                            "reason": parse_badge_reason(tourney, tier),
                        }

                    if tier == 1:
                        player_tier_counts[p] = player_tier_counts.get(p, 0) + 1

                if (
                    p not in player_career_best
                    or tier < player_career_best[p]["tier"]
                    or (
                        tier == player_career_best[p]["tier"]
                        and d_str > player_career_best[p]["date"]
                    )
                ):
                    player_career_best[p] = {"tier": tier, "year": yr, "date": d_str}

        # World Champions hold the exclusive tier 0 ('WC') career peak, at their latest title
        champions = world_champions(conn)
        for p, yr in champions:
            player_career_best[p] = {"tier": 0, "year": yr, "date": f"{yr}-12-15"}

        records = conn.execute(
            """
            SELECT player_name, tournament, season, division, placement, medal, finish_date
            FROM tournament_records WHERE run_id = %s AND NOT career ORDER BY position
        """,
            (run_id,),
        ).fetchall()
        active_sgm, career_sgm = _super_gm(records, cutoff_date)

        derived_data = {}
        for p, info in player_best.items():
            if p in active_sgm:
                title, reason = "SGM", active_sgm[p]["reason"]
            else:
                title, reason = TIER_TITLE_MAP.get(info["tier"], "W"), info["reason"]
            derived_data[p] = {
                "title": title,
                "title_count": player_tier_counts.get(
                    p, 1 if title in ("GM", "SGM") else 0
                ),
                "badge_reason": reason,
            }

        # The latest season's World Champion holds the exclusive active WC badge
        if champions:
            latest = max(yr for _, yr in champions)
            for p, yr in champions:
                if yr == latest:
                    derived_data[p] = {
                        "title": "WC",
                        "title_count": 1,
                        "badge_reason": f"Reigning World Champion (World Championship {yr})",
                    }

        peaks = {}
        for p, c_info in player_career_best.items():
            if c_info["tier"] == 0:
                peaks[p] = ("WC", c_info["year"])
            elif p in career_sgm:
                peaks[p] = ("SGM", career_sgm[p]["year"] or c_info["year"])
            else:
                peaks[p] = (TIER_TITLE_MAP.get(c_info["tier"], "W"), c_info["year"])
        # A premier win with no titled game at all still makes a Super GM peak
        for p, s_info in career_sgm.items():
            peaks.setdefault(p, ("SGM", s_info["year"]))

        # Each year's title: the best division played, then the records, then the career peak
        yearly = {k: TIER_TITLE_MAP.get(t, "W") for k, t in player_year_best.items()}
        for key, code in _record_years(records).items():
            if code == "SGM" or key not in yearly:
                yearly[key] = code
        for p, (peak, yr) in peaks.items():
            best = yearly.get((p, yr))
            if yr is not None and (
                best is None or TITLE_ORDER[peak] < TITLE_ORDER[best]
            ):
                yearly[(p, yr)] = peak

        # Replace the run's titles and peaks; unknown names are skipped, as upstream's UPDATE did
        conn.execute("DELETE FROM player_titles WHERE run_id = %s", (run_id,))
        conn.execute("DELETE FROM player_peak_titles WHERE run_id = %s", (run_id,))
        conn.execute("DELETE FROM player_yearly_titles WHERE run_id = %s", (run_id,))
        with conn.cursor() as cur:
            cur.executemany(
                """
                INSERT INTO player_titles (run_id, player_name, title, title_count, badge_reason)
                SELECT %s, name, %s, %s, %s FROM players WHERE name = %s
            """,
                [
                    (
                        run_id,
                        data["title"],
                        data["title_count"],
                        data["badge_reason"],
                        p,
                    )
                    for p, data in derived_data.items()
                ],
            )
            cur.executemany(
                """
                INSERT INTO player_peak_titles (run_id, player_name, peak_title, peak_year)
                SELECT %s, name, %s, %s FROM players WHERE name = %s
            """,
                [(run_id, peak, yr, p) for p, (peak, yr) in peaks.items()],
            )
            cur.executemany(
                """
                INSERT INTO player_yearly_titles (run_id, player_name, year, title)
                SELECT %s, name, %s, %s FROM players WHERE name = %s
            """,
                [(run_id, yr, code, p) for (p, yr), code in yearly.items()],
            )
            # A World Champion holds WC in their title year, matched ignoring case as upstream's page does
            cur.executemany(
                """
                INSERT INTO player_yearly_titles (run_id, player_name, year, title)
                SELECT %s, name, %s, 'WC' FROM players WHERE lower(name) = lower(%s)
                ON CONFLICT (run_id, player_name, year) DO UPDATE SET title = 'WC'
            """,
                [(run_id, yr, p) for p, yr in champions],
            )

    return derived_data


def _super_gm(records, cutoff_date):
    """Returns the Super GM wins in the records: each player's latest within the
    active window, and their latest ever, with its year and badge reason.
    """
    active, career = {}, {}
    for r in records:
        won = r["placement"].startswith("1 /") or r["medal"] == "gold"
        if not (
            won
            and r["tournament"] in SGM_TOURNAMENTS
            and r["division"] in SGM_DIVISIONS
        ):
            continue
        p, finish = r["player_name"], r["finish_date"]
        year = int(finish[:4]) if finish[:4].isdigit() else None
        reason = f"Champion of {r['tournament']} {r['division']} ({r['season']})"
        if finish >= cutoff_date and (p not in active or finish > active[p]["date"]):
            active[p] = {"date": finish, "reason": reason}
        if p not in career or finish > career[p]["date"]:
            career[p] = {"date": finish, "year": year, "reason": reason}
    return active, career


def _record_years(records):
    """Returns the titles the records give each (player, year), as upstream's
    player page reads them: SGM for a top-division win, else the division's
    tier for a year no match titled (the caller keeps a match's title).
    """
    got = {}
    for r in records:
        finish = r["finish_date"]
        if not finish[:4].isdigit():
            continue
        key = (r["player_name"], int(finish[:4]))
        won = (
            r["placement"].startswith("1 /")
            or r["placement"] == "1"
            or r["medal"] == "gold"
        )
        if (
            r["tournament"] in SGM_TOURNAMENTS
            and r["division"] in SGM_DIVISIONS
            and won
        ):
            got[key] = "SGM"
        elif key not in got:
            tier = classify_division_tier(f"{r['tournament']} {r['division']}")
            if tier is not None:
                got[key] = TIER_TITLE_MAP.get(tier, "W")
    return got
