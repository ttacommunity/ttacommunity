"""Tournament results for a run: each game's division, group standings, the
top division's trophyboard and its seasonal records.

Ported from upstream's hall_of_fame.py and parse_tournaments.py, reading the
database instead of spreadsheets: a division the match name does not give
comes from tournament_group_divisions, a recorded fact.
"""

import re
from collections import defaultdict
from collections.abc import Callable
from dataclasses import dataclass

import psycopg

from tta_ratings_engine.data.badges import HISTORICAL_WC_INFO
from tta_ratings_engine.data.db import require_running

# A share of the winner's score counts at least this much (the rulebooks'
# relative performance score).
PERFORMANCE_FLOOR = 0.5


def _strip_game(name: str) -> str:
    """Returns a match name without its trailing game number, as upstream's normalize_campaign."""
    s = re.sub(r"[\s\-_]+game\s*\d+.*$", "", name.strip(), flags=re.IGNORECASE)
    s = re.sub(r"[\s\-_]+g\d+.*$", "", s, flags=re.IGNORECASE)
    return s.strip(" -_")


def parse_international(name: str) -> tuple[int, str | None, str | None] | None:
    """Returns (season, division, group) for an International game, or None for another series.

    Division and group are None when the name does not give them
    ("International S20"). The group is the division with its number
    ("Silver 5"); the division drops the number.
    """
    m = re.match(
        r"International\s+S(\d+)(?:\s*-\s*(.*))?", _strip_game(name), re.IGNORECASE
    )
    if not m:
        return None
    group = (m.group(2) or "").strip() or None
    division = re.sub(r"\s+\d+$", "", group) if group else None
    return int(m.group(1)), division, group


@dataclass(frozen=True)
class Series:
    """How one series is named, scored and ranked."""

    parse: Callable[[str], tuple[int, str | None, str | None] | None]
    points: dict[int, list[float]]
    top_divisions: frozenset[str]
    record_min_games: int


SERIES = {
    "international": Series(
        parse=parse_international,
        points={4: [6, 3, 1, 0], 3: [5, 2, 0]},
        top_divisions=frozenset({"Grandmaster", "Diamond"}),
        record_min_games=6,
    ),
}


def game_points(
    scores: list[tuple[str, float]], table: list[float]
) -> dict[str, float]:
    """Returns each player's points for one game; tied players share their places' points."""
    order = sorted(scores, key=lambda ps: -ps[1])
    got = {}
    i = 0
    while i < len(order):
        j = i
        while j + 1 < len(order) and order[j + 1][1] == order[i][1]:
            j += 1
        share = sum(table[i : j + 1]) / (j - i + 1)
        for k in range(i, j + 1):
            got[order[k][0]] = share
        i = j + 1
    return got


def _places(scores):
    """Returns each player's place in one game, tied players sharing the better one."""
    return {p: 1 + sum(1 for _, s in scores if s > score) for p, score in scores}


def rank_group(games: list[list[tuple[str, float]]], table: dict[int, list[float]]):
    """Returns a group's standings as dicts in place order.

    Ranked by points, then for a two-way tie by head-to-head (who finished
    above the other more often), then by relative performance score, then by
    name. The rulebooks' extra games and previous-season rule are not applied.
    """
    points = defaultdict(float)
    games_played = defaultdict(int)
    placings = defaultdict(lambda: [0, 0, 0, 0])
    above = defaultdict(lambda: defaultdict(int))
    performance = defaultdict(float)
    for scores in games:
        for p, pts in game_points(scores, table[len(scores)]).items():
            points[p] += pts
        best = max(s for _, s in scores)
        for p, place in _places(scores).items():
            games_played[p] += 1
            placings[p][place - 1] += 1
        for p, s in scores:
            share = s / best if best > 0 else 0.0
            performance[p] += 1.0 if s == best else max(PERFORMANCE_FLOOR, share)
            for q, t in scores:
                if s > t:
                    above[p][q] += 1

    order = []
    for pts in sorted(set(points.values()), reverse=True):
        tied = sorted(
            (p for p in points if points[p] == pts), key=lambda p: (-performance[p], p)
        )
        if len(tied) == 2:
            a, b = tied
            if above[b][a] > above[a][b]:
                tied = [b, a]
            elif above[a][b] == above[b][a]:
                tied = sorted(tied, key=lambda p: (-performance[p], p))
        order.extend(tied)
    return [
        {
            "player_name": p,
            "place": i,
            "points": points[p],
            "games": games_played[p],
            "placings": placings[p],
        }
        for i, p in enumerate(order, start=1)
    ]


def _components(games):
    """Returns the groups of players who played each other, each as a set, oldest game first."""
    parent = {}

    def find(p):
        while parent.setdefault(p, p) != p:
            parent[p] = parent[parent[p]]
            p = parent[p]
        return p

    for players in games:
        for p in players[1:]:
            parent[find(p)] = find(players[0])
        find(players[0])
    groups = {}
    for players in games:
        groups.setdefault(find(players[0]), set()).update(players)
    return list(groups.values())


def classify(rows, recorded):
    """Returns (match_id, series, season, division, group, source) for every tournament game.

    rows are matches in match_id order; recorded maps (series, season,
    player) to a recorded division. A game without a named division takes a
    recorded one only when every player in it has the same one; recorded
    games of one division that never meet form separate numbered groups.
    """
    out = []
    pending = defaultdict(list)
    for r in rows:
        for series, rules in SERIES.items():
            parsed = rules.parse(r["tournament"])
            if parsed is None:
                continue
            season, division, group = parsed
            if division:
                out.append([r["match_id"], series, season, division, group, "name"])
                break
            players = [r[f"player{i}"] for i in range(1, r["player_count"] + 1)]
            divs = {recorded.get((series, season, p)) for p in players}
            row = [r["match_id"], series, season, None, None, None]
            if len(divs) == 1 and None not in divs:
                pending[(series, season, divs.pop())].append((row, players))
            out.append(row)
            break
    for (_series, _season, division), games in pending.items():
        groups = _components([players for _, players in games])
        for row, players in games:
            k = next(i for i, g in enumerate(groups) if players[0] in g)
            label = division if len(groups) == 1 else f"{division} {k + 1}"
            row[3:] = [division, label, "recorded"]
    return [tuple(row) for row in out]


def _scores(r):
    return [(r[f"player{i}"], r[f"score{i}"]) for i in range(1, r["player_count"] + 1)]


def season_records(rows, rules: Series):
    """Returns upstream's premier seasonal records: one per player and season.

    As upstream's get_premier_seasonal_records: named top-division games
    only, a tied place scored by seat order, seasons of at least
    record_min_games games. rows are in match_id order.
    """
    got = {}
    for r in rows:
        order = sorted(_scores(r), key=lambda ps: -ps[1])
        table = rules.points[len(order)]
        for place, (p, _) in enumerate(order, start=1):
            rec = got.setdefault(
                (r["season"], p),
                {"division": r["division"], "points": 0.0, "games": 0,
                 "placings": [0, 0, 0, 0], "first_match_id": r["match_id"]},
            )  # fmt: skip
            rec["points"] += table[place - 1]
            rec["games"] += 1
            rec["placings"][place - 1] += 1
            rec["last_date"] = r["date"]
    return {k: v for k, v in got.items() if v["games"] >= rules.record_min_games}


def derive_tournaments(*, conn: psycopg.Connection, run_id: int) -> None:
    """Replaces the run's tournament games, standings, boards and seasonal records.

    In one committed transaction. Raises RunNotRunning unless the run is running.
    """
    with conn.transaction():
        require_running(conn, run_id)
        rows = conn.execute(
            "SELECT match_id, tournament, date, player_count, player1, score1, player2, score2, "
            "player3, score3, player4, score4 FROM matches ORDER BY match_id"
        ).fetchall()
        recorded = {
            (r["series"], r["season"], r["player_name"]): r["division"]
            for r in conn.execute(
                "SELECT series, season, division, player_name FROM tournament_group_divisions"
            ).fetchall()
        }
        classified = classify(rows, recorded)
        by_id = {r["match_id"]: r for r in rows}

        groups = defaultdict(list)
        named_top = defaultdict(list)
        for match_id, series, season, division, group, source in classified:
            if division is None:
                continue
            groups[(series, season, division, group)].append(_scores(by_id[match_id]))
            if source == "name" and division in SERIES[series].top_divisions:
                named_top[series].append(
                    {**by_id[match_id], "season": season, "division": division}
                )

        standing_rows = []
        medals = defaultdict(
            lambda: {"gold": 0, "silver": 0, "bronze": 0, "points": 0.0}
        )
        for (series, season, division, group), games in groups.items():
            ranked = rank_group(games, SERIES[series].points)
            for s in ranked:
                standing_rows.append(
                    (run_id, series, season, division, group, s["player_name"],
                     s["place"], s["points"], s["games"], s["placings"])
                )  # fmt: skip
                if division in SERIES[series].top_divisions:
                    m = medals[(series, s["player_name"])]
                    m["points"] += s["points"]
                    if s["place"] <= 3:
                        m[("gold", "silver", "bronze")[s["place"] - 1]] += 1

        board_rows = [
            (run_id, series, p, m["gold"], m["silver"], m["bronze"], m["points"],
             "WC" if p in HISTORICAL_WC_INFO else ("GM" if m["gold"] else "M"))
            for (series, p), m in medals.items()
            if m["gold"] + m["silver"] + m["bronze"] > 0
        ]  # fmt: skip

        record_rows = [
            (run_id, series, season, rec["division"], p, rec["points"], rec["games"],
             rec["placings"], rec["last_date"], rec["first_match_id"])
            for series, top_rows in named_top.items()
            for (season, p), rec in season_records(top_rows, SERIES[series]).items()
        ]  # fmt: skip

        for table in (
            "tournament_season_records",
            "tournament_boards",
            "tournament_standings",
            "tournament_games",
        ):
            conn.execute(f"DELETE FROM {table} WHERE run_id = %s", (run_id,))
        with conn.cursor().copy(
            "COPY tournament_games (run_id, match_id, series, season, division, "
            "group_label, division_source) FROM STDIN"
        ) as copy:
            for row in classified:
                copy.write_row((run_id, *row))
        with conn.cursor() as cur:
            cur.executemany(
                "INSERT INTO tournament_standings VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                standing_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_boards VALUES (%s, %s, %s, %s, %s, %s, %s, %s)",
                board_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_season_records VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                record_rows,
            )
