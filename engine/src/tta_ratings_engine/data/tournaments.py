"""Tournament results for a run: each game's series, season and division, group
standings, each series' top-division trophyboard and its seasonal records.

Ported from upstream's hall_of_fame.py and parse_tournaments.py, reading the
database instead of spreadsheets and code: what a match name does not say
comes from recorded facts (divisions, official placements, season corrections).
"""

import functools
import re
from collections import defaultdict
from collections.abc import Callable
from dataclasses import dataclass
from typing import NamedTuple

import psycopg

from tta_ratings_engine.data.achievements import player_achievements, player_records
from tta_ratings_engine.data.badges import HISTORICAL_WC_INFO
from tta_ratings_engine.data.db import require_running
from tta_ratings_engine.data.match_order import assembled_order
from tta_ratings_engine.data.series_pages import (
    grand_slams_board,
    ladders_board,
    legends,
    series_metrics,
    worlds_board,
)

# A share of the winner's score counts at least this much (the rulebooks'
# relative performance score).
PERFORMANCE_FLOOR = 0.5

# Upstream's Royal League trophyboard points for a gold, silver and bronze.
MEDAL_POINTS = (10.0, 5.0, 2.0)

RL_DIVISIONS = (
    "Emperor", "King", "Prince", "Duke", "Marquess", "Count", "Viscount", "Baron", "Knight",
)  # fmt: skip


class Parsed(NamedTuple):
    """What a match name says: its season, division and group, and if it is a tiebreak game."""

    season: int
    division: str | None
    group: str | None
    tiebreak: bool = False


def _strip_game(name: str) -> str:
    """Returns a match name without its trailing game number, as upstream's normalize_campaign."""
    s = re.sub(r"[\s\-_]+game\s*\d+.*$", "", name.strip(), flags=re.IGNORECASE)
    s = re.sub(r"[\s\-_]+g\d+.*$", "", s, flags=re.IGNORECASE)
    return s.strip(" -_")


def _league(prefix: str) -> Callable[[str], Parsed | None]:
    """Returns a parser for "<prefix> S<season> - <Division> <n>" names."""

    def parse(name: str) -> Parsed | None:
        m = re.match(
            prefix + r"\s+S(\d+)(?:\s*-\s*(.*))?", _strip_game(name), re.IGNORECASE
        )
        if not m:
            return None
        group = (m.group(2) or "").strip() or None
        division = re.sub(r"\s+\d+$", "", group) if group else None
        return Parsed(int(m.group(1)), division, group)

    return parse


parse_international = _league("International")
parse_international.__doc__ = (
    "Returns what an International game's name says, or None for another series."
)
parse_intermezzo = _league("Intermezzo")
parse_intermezzo.__doc__ = (
    "Returns what an Intermezzo game's name says, or None for another series."
)


def parse_royal_league(name: str) -> Parsed | None:
    """Returns what a Royal League game's name says, or None for another series.

    Regular games are "RL_s09 - Marquess 3"; tiebreak games are "RL_tb_03",
    "RL_tb_s06", "RL_2w_tb_s02" or "RL_3w_tb_s02", their division the first
    Royal League division their name mentions.
    """
    s = _strip_game(name)
    m = re.match(r"RL_(?:\dw_)?tb_s?0*(\d+)(?:\s*-?\s*(.*))?$", s)
    if m:
        rest = (m.group(2) or "").strip() or None
        division = next((d for d in RL_DIVISIONS if rest and d in rest.split()), None)
        return Parsed(int(m.group(1)), division, rest if division else None, True)
    m = re.match(r"RL_s0*(\d+)(?:\s*-\s*(.*))?$", s)
    if not m:
        return None
    group = (m.group(2) or "").strip() or None
    division = re.sub(r"\s+\d+$", "", group) if group else None
    return Parsed(int(m.group(1)), division, group)


def _like(name: str, *words: str) -> bool:
    """Reports whether name contains any of words, ignoring case, as SQLite's LIKE."""
    return any(w.lower() in name.lower() for w in words)


def _master_division(name: str) -> bool:
    """Reports whether name is a game of the Master 1 or 2 division; "Grandmaster 1" is not."""
    return re.search(r"\bMaster [12]\b", name, re.IGNORECASE) is not None


def parse_worlds(name: str) -> Parsed | None:
    """Returns a World Championship game's edition number ("Worlds 2025", "Worlds 2"), or None.

    Its stages are not divisions, so division and group are None.
    """
    if not _like(name, "World"):
        return None
    m = re.search(r"Worlds?\s*(\d+)", name, re.IGNORECASE)
    return Parsed(int(m.group(1)) if m else 0, None, None)


@dataclass(frozen=True)
class Series:
    """How one series is named, scored, ranked and shown, as upstream does it."""

    parse: Callable[[str], Parsed | None]
    points: dict[int, list[float]]
    top_divisions: frozenset[str]
    duel: bool
    in_records: Callable[[str], bool]
    record_min_games: int
    record_division: str
    season_records: bool = True


SERIES = {
    "international": Series(
        parse=parse_international,
        points={4: [6, 3, 1, 0], 3: [5, 2, 0]},
        top_divisions=frozenset({"Grandmaster", "Diamond"}),
        duel=False,
        in_records=lambda n: (
            n.lower().startswith("international")
            and _like(n, "Grandmaster", "Diamond")
            and not _master_division(n)
        ),
        record_min_games=6,
        record_division="Grandmaster",
    ),
    "intermezzo": Series(
        parse=parse_intermezzo,
        points={3: [5, 2, 0]},
        top_divisions=frozenset({"Grandmaster", "Diamond"}),
        duel=False,
        in_records=lambda n: (
            n.lower().startswith("intermezzo")
            and _like(n, "Grandmaster", "GM", "Diamond", "Premier")
            and not _master_division(n)
        ),
        record_min_games=6,
        record_division="Grandmaster",
    ),
    "royal_league": Series(
        parse=parse_royal_league,
        points={2: [2, 0]},
        top_divisions=frozenset({"Emperor"}),
        duel=True,
        in_records=lambda n: (
            (re.match(r"RL.", n, re.IGNORECASE) is not None or _like(n, "Royal League"))
            and _like(n, "Emperor")
        ),
        record_min_games=5,
        record_division="Emperor",
    ),
    # Its board comes from recorded placements (series_pages.worlds_board);
    # only its best games are listed.
    "worlds": Series(
        parse=parse_worlds,
        points={},
        top_divisions=frozenset(),
        duel=False,
        in_records=lambda n: _like(n, "Stage 4", "Final", "Upper", "Lower"),
        record_min_games=0,
        record_division="",
        season_records=False,
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


def points_table(points: dict[int, list[float]], players: int) -> list[float]:
    """Returns the points for each place in a game of that many players.

    A game with fewer players than any table (one left a league game) takes
    the top places of the smallest, as upstream's records score it.
    """
    return points.get(players) or min(points.values(), key=len)[:players]


def _places(scores):
    """Returns each player's place in one game, tied players sharing the better one."""
    return {p: 1 + sum(1 for _, s in scores if s > score) for p, score in scores}


def _tally(games, table):
    """Returns each player's points, games, placings, head-to-head, performance, wins and score."""
    t = {
        "points": defaultdict(float),
        "games": defaultdict(int),
        "placings": defaultdict(lambda: [0, 0, 0, 0]),
        "above": defaultdict(lambda: defaultdict(int)),
        "h2h": defaultdict(lambda: defaultdict(float)),
        "performance": defaultdict(float),
        "wins": defaultdict(int),
        "score": defaultdict(float),
    }
    for scores in games:
        got = game_points(scores, points_table(table, len(scores)))
        best = max(s for _, s in scores)
        for p, place in _places(scores).items():
            t["points"][p] += got[p]
            t["games"][p] += 1
            t["placings"][p][place - 1] += 1
        for p, s in scores:
            share = s / best if best > 0 else 0.0
            t["performance"][p] += 1.0 if s == best else max(PERFORMANCE_FLOOR, share)
            t["score"][p] += s
            t["wins"][p] += (
                1 if s == best and sum(x == best for _, x in scores) == 1 else 0
            )
            for q, u in scores:
                if s > u:
                    t["above"][p][q] += 1
                if q != p:
                    t["h2h"][p][q] += got[p]
    return t


def _league_order(t):
    """Orders a league group: points, then a two-way tie by head-to-head, then performance, then name.

    The rulebooks' extra games and previous-season rule are not applied.
    """
    points, above, performance = t["points"], t["above"], t["performance"]
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
    return order


def _duel_order(t):
    """Orders a Royal League group as upstream: points, head-to-head points, wins, total score."""
    points, h2h, wins, score = t["points"], t["h2h"], t["wins"], t["score"]

    def compare(a, b):
        for x, y in (
            (points[a], points[b]),
            (h2h[a][b], h2h[b][a]),
            (wins[a], wins[b]),
            (score[a], score[b]),
        ):
            if x != y:
                return -1 if x > y else 1
        return 0

    return sorted(sorted(points), key=functools.cmp_to_key(compare))


def rank_group(games: list[list[tuple[str, float]]], rules: Series):
    """Returns a group's standings as dicts in place order, by the series' rules."""
    t = _tally(games, rules.points)
    order = _duel_order(t) if rules.duel else _league_order(t)
    return [
        {
            "player_name": p,
            "place": i,
            "points": t["points"][p],
            "games": t["games"][p],
            "placings": t["placings"][p],
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


def classify(rows, recorded, corrections):
    """Returns a dict per tournament game: match_id, series, season, name_season, division,
    group, source, tiebreak and in_records.

    rows are matches in match_id order; recorded maps (series, season,
    player) to a recorded division; corrections map (series, name season) to
    the real season, or None to leave the games out. A game without a named
    division takes a recorded one only when every player in it has the same
    one; recorded games of one division that never meet form numbered groups.
    """
    out = []
    pending = defaultdict(list)
    for r in rows:
        for series, rules in SERIES.items():
            parsed = rules.parse(r["tournament"])
            if parsed is None:
                continue
            season = corrections.get((series, parsed.season), parsed.season)
            g = {
                "match_id": r["match_id"], "series": series, "season": season,
                "name_season": parsed.season, "division": None, "group": None,
                "source": None, "tiebreak": parsed.tiebreak,
                "in_records": rules.in_records(r["tournament"]),
            }  # fmt: skip
            out.append(g)
            if season is None:
                break
            if parsed.division:
                g.update(division=parsed.division, group=parsed.group, source="name")
                break
            players = [r[f"player{i}"] for i in range(1, r["player_count"] + 1)]
            divs = {recorded.get((series, season, p)) for p in players}
            if len(divs) == 1 and None not in divs and not parsed.tiebreak:
                pending[(series, season, divs.pop())].append((g, players))
            break
    for (_series, _season, division), games in pending.items():
        groups = _components([players for _, players in games])
        for g, players in games:
            k = next(i for i, grp in enumerate(groups) if players[0] in grp)
            label = division if len(groups) == 1 else f"{division} {k + 1}"
            g.update(division=division, group=label, source="recorded")
    return out


def _scores(r):
    return [(r[f"player{i}"], r[f"score{i}"]) for i in range(1, r["player_count"] + 1)]


def season_records(rows, rules: Series):
    """Returns the premier seasonal records: one per player and season.

    From upstream's get_premier_seasonal_records: its records games only, and
    seasons of at least record_min_games games; but by the corrected season
    (a season left out has none), and tied players share their places' points
    and the better place, as the series' rules say. placings are the 1st, 2nd, 3rd and 4th places; in
    Royal League, wins, draws and losses. rows are in match_id order.
    """
    got = {}
    for r in rows:
        order = sorted(_scores(r), key=lambda ps: -ps[1])
        shared = game_points(order, points_table(rules.points, len(order)))
        places = _places(order)
        for place, (p, score) in enumerate(order, start=1):
            rec = got.setdefault(
                (r["season"], p),
                {"division": rules.record_division, "points": 0.0, "games": 0,
                 "placings": [0, 0, 0] if len(rules.points) == 1 else [0, 0, 0, 0],
                 "first_match_id": r["match_id"]},
            )  # fmt: skip
            rec["games"] += 1
            rec["last_date"] = r["date"]
            if rules.duel:
                other = order[1 if place == 1 else 0][1]
                result = 0 if score > other else (1 if score == other else 2)
                rec["points"] += (2.0, 1.0, 0.0)[result]
                rec["placings"][result] += 1
            else:
                rec["points"] += shared[p]
                rec["placings"][places[p] - 1] += 1
    return {k: v for k, v in got.items() if v["games"] >= rules.record_min_games}


def _league_boards(standings, official):
    """Returns each league's trophyboard in upstream's order: medals in the top
    division, official placements over derived ones.

    standings and official map (series, season, division) to rows of
    player_name, place and points. Ordered by golds, silvers, bronzes, career
    points (league points, or in Royal League upstream's medal points), then
    the season of the first medal and name.
    """
    medals = defaultdict(
        lambda: {"gold": 0, "silver": 0, "bronze": 0, "points": 0.0, "first": None}
    )
    seasons = {**standings, **official}
    for (series, season, division), rows in sorted(seasons.items()):
        rules = SERIES[series]
        if division not in rules.top_divisions:
            continue
        for s in rows:
            m = medals[(series, s["player_name"])]
            if not rules.duel:
                m["points"] += s["points"] or 0.0
            if s["place"] <= 3:
                m[("gold", "silver", "bronze")[s["place"] - 1]] += 1
                m["first"] = season if m["first"] is None else m["first"]
                if rules.duel:
                    m["points"] += MEDAL_POINTS[s["place"] - 1]
    boards = defaultdict(list)
    for (series, p), m in medals.items():
        if m["first"] is not None:
            title = "WC" if p in HISTORICAL_WC_INFO else ("GM" if m["gold"] else "M")
            boards[series].append(
                {"player_name": p, **m, "first_season": m["first"], "title": title}
            )
    for board in boards.values():
        board.sort(
            key=lambda b: (-b["gold"], -b["silver"], -b["bronze"], -b["points"], b["first_season"], b["player_name"])
        )  # fmt: skip
    return boards


def _board_rows(run_id, boards):
    """Returns board rows to insert, each board numbered in its given order."""
    return [
        (run_id, series, b["player_name"], b.get("gold"), b.get("silver"), b.get("bronze"),
         b.get("points", 0.0), b.get("title"), b.get("first_season"), position,
         b.get("details"), b.get("games"), b.get("wins"), b.get("win_rate"))
        for series, board in boards.items()
        for position, b in enumerate(board, start=1)
    ]  # fmt: skip


def derive_tournaments(*, conn: psycopg.Connection, run_id: int) -> None:
    """Replaces the run's tournament games, standings, boards, seasonal records
    and each player's achievements and records.

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
        corrections = {
            (r["series"], r["name_season"]): r["season"]
            for r in conn.execute(
                "SELECT series, name_season, season FROM tournament_season_corrections"
            ).fetchall()
        }
        official = defaultdict(list)
        for r in conn.execute(
            "SELECT series, season, division, player_name, place, points "
            "FROM tournament_official_placements ORDER BY place, player_name"
        ).fetchall():
            official[(r["series"], r["season"], r["division"])].append(r)
        dates = {
            (r["series"], r["season"]): r["finish_date"].isoformat()
            for r in conn.execute(
                "SELECT series, season, finish_date FROM tournament_finish_dates"
            ).fetchall()
        }
        honours = conn.execute(
            "SELECT series, player_name, gold, silver, bronze, points "
            "FROM tournament_career_honours ORDER BY series, player_name"
        ).fetchall()
        classified = classify(rows, recorded, corrections)
        by_id = {r["match_id"]: r for r in rows}

        groups = defaultdict(list)
        records = defaultdict(list)
        for g in classified:
            r = by_id[g["match_id"]]
            # A season without division names has its top division recorded from the organisers' sheet.
            if (
                g["source"] == "recorded"
                and g["division"] in SERIES[g["series"]].top_divisions
            ):
                g["in_records"] = True
            if (
                g["in_records"]
                and g["season"] is not None
                and not g["tiebreak"]
                and SERIES[g["series"]].season_records
            ):
                records[g["series"]].append({**r, **g})
            if g["division"] is not None and not g["tiebreak"]:
                key = (g["series"], g["season"], g["division"], g["group"])
                groups[key].append(_scores(r))

        standing_rows = []
        derived = defaultdict(list)
        for (series, season, division, group), games in groups.items():
            for s in rank_group(games, SERIES[series]):
                standing_rows.append(
                    (run_id, series, season, division, group, s["player_name"],
                     s["place"], s["points"], s["games"], s["placings"])
                )  # fmt: skip
                derived[(series, season, division)].append(s)
        boards = _league_boards(derived, official)
        boards["worlds"] = worlds_board(
            [{**p, "season": season} for (series, season, _), placed in official.items()
             if series == "worlds" for p in placed]
        )  # fmt: skip
        boards["grand_slams"] = grand_slams_board(rows)
        boards["ladders"] = ladders_board(rows)
        legend_rows = [
            (run_id, position, r["player_name"], r["world_titles"], r["international_titles"],
             r["intermezzo_titles"], r["royal_league_titles"], r["gold"], r["silver"], r["bronze"])
            for position, r in enumerate(legends(boards), start=1)
        ]  # fmt: skip
        board_rows = _board_rows(run_id, boards)
        series_rows = [
            (run_id, series, games, editions)
            for series, (games, editions) in series_metrics(rows).items()
        ]

        worlds = {
            (season, p["player_name"]): p["place"]
            for (series, season, _), placed in official.items()
            if series == "worlds"
            for p in placed
        }
        achievement_rows = [
            (run_id, a["player_name"], a["world_titles"], a["international_titles"],
             a["intermezzo_titles"], a["royal_league_titles"], a["gold"], a["silver"],
             a["bronze"], a["accolades"])
            for a in player_achievements(boards, worlds)
        ]  # fmt: skip
        player_record_rows = player_records(
            rows=assembled_order(rows),
            top_divisions={
                s: SERIES[s].top_divisions for s in ("international", "intermezzo")
            },
            standings=derived,
            official=official,
            boards=boards,
            honours=honours,
            dates=dates,
        )

        record_rows = [
            (run_id, series, season, rec["division"], p, rec["points"], rec["games"],
             rec["placings"], rec["last_date"], rec["first_match_id"])
            for series, top_rows in records.items()
            for (season, p), rec in season_records(top_rows, SERIES[series]).items()
        ]  # fmt: skip

        for table in (
            "tournament_records",
            "tournament_achievements",
            "tournament_series",
            "tournament_legends",
            "tournament_season_records",
            "tournament_boards",
            "tournament_standings",
            "tournament_games",
        ):
            conn.execute(f"DELETE FROM {table} WHERE run_id = %s", (run_id,))
        with conn.cursor().copy(
            "COPY tournament_games (run_id, match_id, series, season, name_season, division, "
            "group_label, division_source, tiebreak, in_records) FROM STDIN"
        ) as copy:
            for g in classified:
                copy.write_row(
                    (run_id, g["match_id"], g["series"], g["season"], g["name_season"],
                     g["division"], g["group"], g["source"], g["tiebreak"], g["in_records"])
                )  # fmt: skip
        with conn.cursor().copy(
            "COPY tournament_records (run_id, player_name, position, tournament, season, "
            "division, placement, points, medal, details, finish_date, career) FROM STDIN"
        ) as copy:
            for r in player_record_rows:
                copy.write_row(
                    (run_id, r["player_name"], r["position"], r["tournament"], r["season"],
                     r["division"], r["placement"], r["points"], r["medal"], r["details"],
                     r["finish_date"], r["career"])
                )  # fmt: skip
        with conn.cursor() as cur:
            cur.executemany(
                "INSERT INTO tournament_standings VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                standing_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_boards (run_id, series, player_name, gold, silver, "
                "bronze, points, title, first_season, position, details, games, wins, "
                "win_rate) VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                board_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_series VALUES (%s, %s, %s, %s)", series_rows
            )
            cur.executemany(
                "INSERT INTO tournament_legends VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                legend_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_achievements VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                achievement_rows,
            )
            cur.executemany(
                "INSERT INTO tournament_season_records VALUES (%s, %s, %s, %s, %s, %s, %s, %s, %s, %s)",
                record_rows,
            )
