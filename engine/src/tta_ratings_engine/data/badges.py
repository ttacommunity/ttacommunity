"""Tournament Badge Derivation Engine for Through the Ages.

Implements the official badge award rules:
- Recency Rule: Badges are displayed only for players active within the last 1 year (12 months) in tournament play.
- Highest league in Intermezzo (3P), International Championship (4P) or Royal League (2P) -> GM (Grandmaster)
- 2nd highest league -> M (Master)
- 3rd highest league -> P (Platinum)
- 4th highest league -> G (Gold)
- 5th highest league -> S (Silver)
- 6th highest league -> B (Bronze)
- 7th highest league -> W (Wood)
- A win in the top division of the International, Intermezzo or Royal League
  (a gold medal in the run's tournament records) -> SGM (Super GM)
- The Reigning World Champion retains the exclusive 'WC' title; a World Championship podium gives WC, WCS or
  WCB in its year and as a peak. Playing in the World Championship gives no division title.
- A title set in admin replaces the derived one (an active player's), or takes it away.
- Hover Reason: Shows the latest tournament division and season (e.g. 'Played in International Championship Grandmaster Season 6').
"""

from datetime import UTC, date, datetime
from typing import Any, NamedTuple

from dateutil.relativedelta import relativedelta

from tta_ratings_engine.data.db import DictConnection, require_running

_EPOCH = datetime.min.replace(tzinfo=UTC)


def _finish_key(dt: Any) -> datetime:
    """Returns dt as a UTC datetime, or the epoch when missing."""
    if dt is None:
        return _EPOCH
    if isinstance(dt, datetime):
        if dt.tzinfo is None:
            return dt.replace(tzinfo=UTC)
        return dt.astimezone(UTC)
    return _EPOCH


# Tournament division tier rankings (1 = Highest / GM, 2 = M, 3 = P, 4 = G, 5 = S, 6 = B, 7 = W)
TIER_TITLE_MAP = {1: "GM", 2: "M", 3: "P", 4: "G", 5: "S", 6: "B", 7: "W"}

TITLE_ORDER = {
    "WC": 0,
    "SGM": 1,
    "WCS": 2,
    "WCB": 3,
    "GM": 4,
    "M": 5,
    "P": 6,
    "G": 7,
    "S": 8,
    "B": 9,
    "W": 10,
}

# The leagues' names in a badge's hover text
SERIES_NAMES = {
    "international": "International Championship",
    "intermezzo": "Intermezzo Championship",
    "royal_league": "Royal League",
}


def world_podiums(conn, run_id) -> list[tuple[str, int, int]]:
    """Returns each World Championship season's podium finishes (place 1, 2, 3), oldest first: (player, season, place).

    They are the medals of the run's World Championship records, whose podium tournaments.worlds_podium chose.
    """
    rows = conn.execute(
        """
        SELECT player_name, season::int AS season,
               CASE medal WHEN 'gold' THEN 1 WHEN 'silver' THEN 2 ELSE 3 END AS place
        FROM tournament_records
        WHERE run_id = %s AND NOT career AND tournament = 'World Championship' AND medal IS NOT NULL
        ORDER BY season, place, player_name
        """,
        (run_id,),
    ).fetchall()
    return [(r["player_name"], r["season"], r["place"]) for r in rows]


def world_champions(conn, run_id) -> list[tuple[str, int]]:
    """Returns each World Championship season's winner, oldest first: (player, season)."""
    return [(p, yr) for p, yr, pl in world_podiums(conn, run_id) if pl == 1]


class Leagues(NamedTuple):
    """The league settings titles read: each (series, division)'s tier, 1 the highest, and the top divisions whose
    win makes a Super GM, by series."""

    tiers: dict[tuple[str, str], int]
    super_gm: dict[str, frozenset[str]]


def league_settings(conn) -> Leagues:
    """Returns the divisions' tiers and the Super GM top divisions of every series that has division tiers."""
    rows = conn.execute(
        "SELECT slug, divisions, division_tiers, super_gm, top_divisions FROM series WHERE cardinality(divisions) > 0"
    ).fetchall()
    return Leagues(
        {
            (r["slug"], d): t
            for r in rows
            for d, t in zip(r["divisions"], r["division_tiers"], strict=True)
        },
        {r["slug"]: frozenset(r["top_divisions"]) for r in rows if r["super_gm"]},
    )


def _badge_reason(r) -> str:
    """Returns a game's hover text: its league, division and season, as the run recorded them."""
    season = f" Season {r['season']}" if r.get("season") is not None else ""
    return f"Played in {SERIES_NAMES.get(r['series'], r['series'])} {r['division']}{season}"


class Titles(NamedTuple):
    """A run's titles: active (each recently active player's title, count and reason), peaks (each player's
    career peak and its year) and yearly (each (player, year)'s best title)."""

    active: dict[str, dict[str, Any]]
    peaks: dict[str, tuple[str, int | None]]
    yearly: dict[tuple[str, int], str]


def derive_tournament_badges(*, conn: DictConnection, run_id: int) -> dict[str, Any]:
    """Derives tournament badges for players active within the last year, and returns them.

    Replaces the run's player_titles (active titles), player_peak_titles
    (career peaks, inactive players too) and player_yearly_titles (best title
    each year) with those of known players, in one committed transaction. Raises RunNotRunning unless the run is running.
    """
    with conn.transaction():
        require_running(conn, run_id)
        # Determine 1-year (12-month) recency cutoff based on latest match in DB
        max_row = conn.execute(
            "SELECT to_char(MAX(date), 'YYYY-MM-DD') AS max_date FROM games WHERE state = 'played'"
        ).fetchone()
        if not max_row or not max_row["max_date"]:
            return {}
        max_date = date.fromisoformat(max_row["max_date"])
        cutoff_date = (max_date - relativedelta(years=1)).strftime("%Y-%m-%d")

        # All played games, with the series, division and season the run recorded
        rows = conn.execute(
            """
            SELECT m.game_id, m.name AS tournament, to_char(m.date, 'YYYY-MM-DD') AS date,
                   m.finished_at, m.players, tg.series, tg.division, tg.season
            FROM game_lines m
            LEFT JOIN tournament_games tg ON tg.match_id = m.game_id AND tg.run_id = %s
            WHERE m.state = 'played'
            ORDER BY m.date DESC, m.finished_at DESC NULLS LAST, m.game_id DESC
        """,
            (run_id,),
        ).fetchall()
        podiums = world_podiums(conn, run_id)
        records = conn.execute(
            """
            SELECT player_name, tournament, season, division, medal, finish_date, series, series_division
            FROM tournament_records WHERE run_id = %s AND NOT career ORDER BY position
        """,
            (run_id,),
        ).fetchall()
        overrides = conn.execute(
            "SELECT player_name, title, reason FROM player_title_overrides"
        ).fetchall()
        titles = badge_titles(
            rows, cutoff_date, podiums, records, overrides, league_settings(conn)
        )
        _write_titles(conn, run_id, titles, podiums)

    return titles.active


def _write_titles(conn: DictConnection, run_id: int, titles: Titles, podiums) -> None:
    """Replaces the run's active, peak and yearly titles; World Championship podiums hold their title in its year."""
    derived_data, peaks, yearly = titles
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
        # World Championship podium finishers hold their title (WC, WCS, WCB) in their title year
        cur.executemany(
            """
            INSERT INTO player_yearly_titles (run_id, player_name, year, title)
            SELECT %s, name, %s, %s FROM players WHERE name = %s
            ON CONFLICT (run_id, player_name, year) DO UPDATE SET title = EXCLUDED.title
        """,
            [
                (run_id, yr, "WC" if pl == 1 else "WCS" if pl == 2 else "WCB", p)
                for p, yr, pl in podiums
            ],
        )


def badge_titles(
    rows, cutoff_date, champions, records, overrides, leagues: Leagues
) -> Titles:
    """Returns the titles the games, World Championship podiums, records and admin's overrides give.

    rows are the played games newest first (date as YYYY-MM-DD, players, and the series, division and season the
    run recorded); cutoff_date (YYYY-MM-DD) starts the active window; champions are the World Championship's
    podiums as world_podiums gives them, (player, season, place); records are the run's season records, each with
    the series and division its games were in; overrides are admin's titles ('' takes one away); leagues are the
    divisions' tiers and the Super GM top divisions.
    """
    best = _best_tiers(rows, cutoff_date, leagues.tiers)
    active_sgm, career_sgm = _super_gm(records, cutoff_date, leagues.super_gm)
    derived_data = _active_titles(best, champions, active_sgm, overrides)
    peaks = _peak_titles(best.career, champions, career_sgm)
    return Titles(
        derived_data, peaks, _yearly_titles(best.yearly, records, peaks, leagues)
    )


class _BestTiers(NamedTuple):
    """Each player's best division tier: in the active window (with its game), ever, and each year."""

    active: dict[str, dict[str, Any]]
    career: dict[str, dict[str, Any]]
    tier_counts: dict[str, int]
    yearly: dict[tuple[str, int], int]


def _best_tiers(rows, cutoff_date, tiers) -> _BestTiers:
    """Returns each player's best tiers over rows, the latest game winning a tie; tier_counts counts
    their tier-1 games since cutoff_date. tiers map a (series, division) to its tier."""
    player_best: dict[str, dict[str, Any]] = {}
    player_career_best: dict[str, dict[str, Any]] = {}
    player_tier_counts: dict[str, int] = {}
    player_year_best: dict[tuple[str, int], int] = {}

    for r in rows:
        tier = tiers.get((r["series"], r["division"]))
        if tier is None:
            continue

        d_str = r["date"]
        yr = int(d_str[:4])

        participants = list(r["players"])

        game_key = (
            d_str,
            _finish_key(r.get("finished_at")),
            r.get("game_id") or r.get("match_id") or 0,
        )

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
                        and game_key > player_best[p]["key"]
                    )
                ):
                    player_best[p] = {
                        "tier": tier,
                        "date": d_str,
                        "key": game_key,
                        "reason": _badge_reason(r),
                    }

                if tier == 1:
                    player_tier_counts[p] = player_tier_counts.get(p, 0) + 1

            if (
                p not in player_career_best
                or tier < player_career_best[p]["tier"]
                or (
                    tier == player_career_best[p]["tier"]
                    and game_key > player_career_best[p]["key"]
                )
            ):
                player_career_best[p] = {
                    "tier": tier,
                    "year": yr,
                    "date": d_str,
                    "key": game_key,
                }
    return _BestTiers(
        player_best, player_career_best, player_tier_counts, player_year_best
    )


def _active_titles(
    best: _BestTiers, champions, active_sgm, overrides
) -> dict[str, dict[str, Any]]:
    """Returns each recently active player's title, count and reason: SGM, the latest World Champion's WC,
    else their best tier's, with admin's overrides on top."""
    player_best, player_tier_counts = best.active, best.tier_counts
    world_champ_list = [(p, yr) for p, yr, place in champions if place == 1]
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
    if world_champ_list:
        latest = max(yr for _, yr in world_champ_list)
        for p, yr in world_champ_list:
            if yr == latest:
                derived_data[p] = {
                    "title": "WC",
                    "title_count": 1,
                    "badge_reason": f"Reigning World Champion (World Championship {yr})",
                }

    # A title set in admin replaces the derived one for a recently active player; '' takes it away
    for r in overrides:
        p = r["player_name"]
        if p not in derived_data:
            continue
        if r["title"] == "":
            del derived_data[p]
        else:
            derived_data[p] = {
                "title": r["title"],
                "title_count": 1,
                "badge_reason": f"Set in admin: {r['reason']}",
            }
    return derived_data


def _peak_titles(
    player_career_best, champions, career_sgm
) -> dict[str, tuple[str, int | None]]:
    """Returns each player's career peak title and its year: their best tier, outranked by a Worlds podium or SGM."""
    peaks = {}
    for p, c_info in player_career_best.items():
        peaks[p] = (TIER_TITLE_MAP.get(c_info["tier"], "W"), c_info["year"])

    # A player on several podiums keeps the best: WC over WCS over WCB
    career_worlds = {}
    for title, wanted in (("WCB", 3), ("WCS", 2), ("WC", 1)):
        for p, yr, place in champions:
            if place == wanted:
                career_worlds[p] = (title, yr)

    # Major titles & Worlds podiums outrank division tiers
    for p, (w_title, yr) in career_worlds.items():
        if p not in peaks or TITLE_ORDER[w_title] < TITLE_ORDER.get(peaks[p][0], 99):
            peaks[p] = (w_title, yr)

    for p, s_info in career_sgm.items():
        if p not in peaks or TITLE_ORDER["SGM"] < TITLE_ORDER.get(peaks[p][0], 99):
            peaks[p] = ("SGM", s_info["year"])
    return peaks


def _yearly_titles(
    player_year_best, records, peaks, leagues
) -> dict[tuple[str, int], str]:
    """Returns each (player, year)'s title: the best division played, then the records, then the career peak."""
    yearly = {k: TIER_TITLE_MAP.get(t, "W") for k, t in player_year_best.items()}
    for key, code in _record_years(records, leagues).items():
        if code == "SGM" or key not in yearly:
            yearly[key] = code
    for p, (peak, yr) in peaks.items():
        best = yearly.get((p, yr))
        if yr is not None and (best is None or TITLE_ORDER[peak] < TITLE_ORDER[best]):
            yearly[(p, yr)] = peak

    return yearly


def _is_super_gm_win(r, super_gm) -> bool:
    """Reports whether a season record is a top-division win in a series whose win makes a Super GM (super_gm maps
    those series to their top divisions): a gold medal there, which only a podium has."""
    return r["medal"] == "gold" and r["series_division"] in super_gm.get(
        r["series"], ()
    )


def _super_gm(records, cutoff_date, super_gm):
    """Returns the Super GM wins in the records: each player's latest within the
    active window, and their latest ever, with its year and badge reason.
    """
    active, career = {}, {}
    for r in records:
        if not _is_super_gm_win(r, super_gm):
            continue
        p, finish = r["player_name"], r["finish_date"]
        year = int(finish[:4]) if finish[:4].isdigit() else None
        reason = f"Champion of {r['tournament']} {r['division']} ({r['season']})"
        if finish >= cutoff_date and (p not in active or finish > active[p]["date"]):
            active[p] = {"date": finish, "reason": reason}
        if p not in career or finish > career[p]["date"]:
            career[p] = {"date": finish, "year": year, "reason": reason}
    return active, career


def _record_years(records, leagues):
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
        if _is_super_gm_win(r, leagues.super_gm):
            got[key] = "SGM"
        elif key not in got:
            tier = leagues.tiers.get((r["series"], r["series_division"]))
            if tier is not None:
                got[key] = TIER_TITLE_MAP.get(tier, "W")
    return got
