"""The series-page figures that are not league standings: each series' game and
edition counts, and the World Championship, Grand Slams and Ladders boards.

Ported from upstream's hall_of_fame.py (derive_series_live_metrics,
derive_world_championship_trophyboard, derive_grand_slams_trophyboard,
derive_ladders_trophyboard), reading matches and recorded placements.
"""

import re
from collections import defaultdict

from tta_ratings_engine.data.badges import HISTORICAL_WC_INFO

PLACES = ("gold", "silver", "bronze")
LABELS = ("Champion", "Runner-Up", "3rd Place")


def _like(name: str, pattern: str) -> bool:
    """Reports whether name matches an SQL LIKE pattern, ignoring case, as SQLite does."""
    regex = "".join(
        ".*" if c == "%" else "." if c == "_" else re.escape(c) for c in pattern
    )
    return re.fullmatch(regex, name, re.IGNORECASE | re.DOTALL) is not None


def _any(name: str, *patterns: str) -> bool:
    return any(_like(name, p) for p in patterns)


def _numbers(names, pattern):
    """Returns the first number pattern finds in each name, ignoring case."""
    return [
        int(m.group(1)) for n in names if (m := re.search(pattern, n, re.IGNORECASE))
    ]


def series_metrics(rows) -> dict[str, tuple[int, int]]:
    """Returns each series' (games, editions) as upstream's series pages count them.

    Games are matches whose name fits the series' patterns. Editions are the
    latest season number for leagues, the distinct "Worlds <n>" names for the
    World Championship, the distinct names before "Stage" for the cups, and a
    fixed 16 for the ladders.
    """
    names = [r["tournament"] for r in rows]
    series = {
        "international": lambda n: _like(n, "International%"),
        "intermezzo": lambda n: _like(n, "Intermezzo%"),
        "royal_league": lambda n: _any(n, "RL_%", "%Royal League%"),
        "worlds": lambda n: _any(n, "%World%", "%Worlds%"),
        "grand_slams": lambda n: _any(
            n,
            "%Survivor%",
            "%Slam%",
            "%Eiffel%",
            "%Wimbledon%",
            "%Slow Burn%",
            "Australian Open%",
            "French Open%",
        ),
        "ladders": lambda n: _any(n, "%Mercurial%", "%Sodium%", "%TCL%", "ML_%"),
    }
    got = {}
    for s, fits in series.items():
        mine = [n for n in names if fits(n)]
        distinct = set(mine)
        if s in ("international", "intermezzo"):
            editions = max(_numbers(distinct, r"S(\d+)"), default=0)
        elif s == "royal_league":
            editions = max(_numbers(distinct, r"s0?(\d+)"), default=0)
        elif s == "worlds":
            found = {
                m.group(0)
                for n in distinct
                if (
                    m := re.search(r"Worlds?\s*(\d+)", n, re.IGNORECASE)
                    or re.search(r"202\d", n)
                )
            }
            editions = len(found)
        elif s == "grand_slams":
            editions = len({n.split("Stage")[0].strip() for n in distinct})
        else:
            editions = 16
        got[s] = (len(mine), editions)
    return got


def _board_row(player, medals, **extra):
    return {"player_name": player, **medals, **extra}


def worlds_board(placements) -> list[dict]:
    """Returns the World Championship board from its recorded placements.

    Most golds, silvers and bronzes first, the latest champion first among
    equals, as upstream lists it. A World Champion's title is WC, any other
    medallist's GM.
    """
    medals = defaultdict(lambda: {"gold": 0, "silver": 0, "bronze": 0, "latest": 0})
    for p in placements:
        if p["place"] <= 3:
            m = medals[p["player_name"]]
            m[PLACES[p["place"] - 1]] += 1
            m["latest"] = max(m["latest"], p["season"])
    order = sorted(
        medals.items(),
        key=lambda kv: (-kv[1]["gold"], -kv[1]["silver"], -kv[1]["bronze"], -kv[1]["latest"], kv[0]),
    )  # fmt: skip
    return [
        _board_row(
            p,
            {k: m[k] for k in PLACES},
            title="WC" if p in HISTORICAL_WC_INFO else "GM",
            points=100.0 * m["gold"] + 50.0 * m["silver"] + 25.0 * m["bronze"],
        )
        for p, m in order
    ]


def _players(r):
    """Returns a game's players and scores, highest first, a missing score as 0."""
    seats = [(r[f"player{i}"], r[f"score{i}"]) for i in range(1, 5)][
        : r["player_count"]
    ]
    return sorted(
        ((p, float(s) if s is not None else 0.0) for p, s in seats if p),
        key=lambda ps: -ps[1],
    )


def grand_slams_board(rows) -> list[dict]:
    """Returns upstream's Grand Slams & Cups board, cup by cup as upstream derives each.

    rows are matches in match_id order. Each player's details are their first
    two honours.
    """
    trophies = {}

    def award(player, place, honour):
        t = trophies.setdefault(
            player, {"gold": 0, "silver": 0, "bronze": 0, "cups": []}
        )
        t[PLACES[place]] += 1
        t["cups"].append(honour)

    def matching(pattern):
        return [r for r in rows if _like(r["tournament"], pattern)]

    finals = matching("%Survivors Cup%Final%")
    if finals:
        latest = max(finals, key=lambda r: r["date"])
        for place, (p, _) in enumerate(_players(latest)[:3]):
            award(p, place, f"2026 Survivors Cup {LABELS[place]}")

    wins, scores = defaultdict(int), {}
    for r in matching("%Slow Burn%Final%"):
        players = _players(r)
        wins[players[0][0]] += 1
        for p, s in players:
            scores[p] = scores.get(p, 0.0) + s
    for place, p in enumerate(
        sorted(scores, key=lambda p: (wins[p], scores[p]), reverse=True)[:3]
    ):
        award(p, place, f"Slow Burn S15 {LABELS[place]}")

    wins = {}
    for r in matching("Wimbledon 2023 stage 7%"):
        (a, sa), (b, sb) = (
            (r["player1"], r["score1"] or 0),
            (r["player2"], r["score2"] or 0),
        )
        if sa != sb:
            winner = a if sa > sb else b
            wins[winner] = wins.get(winner, 0) + 1
    for place, (p, _) in enumerate(
        sorted(wins.items(), key=lambda kv: kv[1], reverse=True)[:2]
    ):
        award(p, place, f"2023 Wimbledon TTA {LABELS[place]}")

    wins, points = {}, defaultdict(float)
    for r in matching("Wimbledon 2025 Stage 10%"):
        (a, sa), (b, sb) = (
            (r["player1"], r["score1"] or 0),
            (r["player2"], r["score2"] or 0),
        )
        points[a] += sa
        points[b] += sb
        if sa != sb:
            winner = a if sa > sb else b
            wins[winner] = wins.get(winner, 0) + 1
    for place, p in enumerate(
        sorted(wins, key=lambda p: (wins[p], points[p]), reverse=True)[:3]
    ):
        award(p, place, f"2025 Wimbledon TTA {LABELS[place]}")

    top = {}
    for r in matching("%Eiffel Tower%The Top%"):
        seats = [
            (r[f"player{i}"], r[f"score{i}"]) for i in range(1, r["player_count"] + 1)
        ]
        players = sorted(
            ((p, float(s)) for p, s in seats if p and s is not None),
            key=lambda ps: -ps[1],
        )
        if players:
            top[players[0][0]] = top.get(players[0][0], 0) + 1
    for p, n in sorted(top.items(), key=lambda kv: kv[1], reverse=True)[:3]:
        award(p, 0, f"Eiffel Tower Pinnacle Champion ({n} Top Victories)")

    board = [
        _board_row(p, {k: t[k] for k in PLACES}, details=", ".join(t["cups"][:2]))
        for p, t in trophies.items()
    ]
    return sorted(board, key=lambda b: (b["gold"], b["silver"], b["bronze"], b["gold"] + b["silver"] + b["bronze"]), reverse=True)  # fmt: skip


def ladders_board(rows) -> list[dict]:
    """Returns upstream's ladders board: the 15 players with most ladder game wins.

    Mercurial, Sodium and Transcontinental games only; players of 30 games
    or more; most wins first, then win rate. rows are in match_id order.
    """
    stats = {}
    for r in rows:
        if not _any(r["tournament"], "%Mercurial%", "%Sodium%", "%TCL%"):
            continue
        players = _players(r)
        if not players:
            continue
        for p, _ in players:
            stats.setdefault(p, {"games": 0, "wins": 0})["games"] += 1
        stats[players[0][0]]["wins"] += 1
    board = [
        _board_row(p, {}, games=s["games"], wins=s["wins"],
                   win_rate=round(100.0 * s["wins"] / s["games"], 1))
        for p, s in stats.items()
        if s["games"] >= 30
    ]  # fmt: skip
    return sorted(board, key=lambda b: (b["wins"], b["win_rate"]), reverse=True)[:15]


# The circuits whose titles make the Hall of Fame's all-time table, in the
# order upstream reads their boards.
MAJOR_CIRCUITS = {
    "worlds": "world_titles",
    "international": "international_titles",
    "intermezzo": "intermezzo_titles",
    "royal_league": "royal_league_titles",
}


def legends(boards: dict[str, list[dict]], shown: int = 25) -> list[dict]:
    """Returns upstream's all-time major circuits table from the four circuits' boards.

    Each player's titles are their golds in each circuit; medals are summed
    across them. A player needs a title or two medals. Ordered by titles,
    World titles, golds, silvers, bronzes and medals, ties first seen first.
    """
    got = {}
    for series, titles in MAJOR_CIRCUITS.items():
        for b in boards.get(series, []):
            row = got.setdefault(
                b["player_name"],
                {"player_name": b["player_name"], **dict.fromkeys(MAJOR_CIRCUITS.values(), 0),
                 "gold": 0, "silver": 0, "bronze": 0},
            )  # fmt: skip
            row[titles] += b["gold"]
            for m in PLACES:
                row[m] += b[m]

    def titles(r):
        return sum(r[t] for t in MAJOR_CIRCUITS.values())

    def medals(r):
        return r["gold"] + r["silver"] + r["bronze"]

    kept = [r for r in got.values() if titles(r) > 0 or medals(r) >= 2]
    kept.sort(
        key=lambda r: (titles(r), r["world_titles"], r["gold"], r["silver"], r["bronze"], medals(r)),
        reverse=True,
    )  # fmt: skip
    return kept[:shown]
