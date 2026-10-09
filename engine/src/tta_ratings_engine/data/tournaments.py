"""Tournament results for a run: each game's series, season and division, group
standings, each series' top-division trophyboard and its seasonal records.

Ported from upstream's hall_of_fame.py and parse_tournaments.py, reading the
database instead of spreadsheets and code: what a match name does not say
comes from recorded facts (divisions, official placements, season corrections).
"""

import functools
import itertools
import re
from collections import defaultdict
from collections.abc import Callable
from dataclasses import dataclass, field, replace
from typing import NamedTuple

import psycopg

from tta_ratings_engine.data.achievements import player_achievements, player_records
from tta_ratings_engine.data.db import require_running
from tta_ratings_engine.data.match_order import assembled_order
from tta_ratings_engine.data.seats import SEAT_COLUMNS, Seat, by_place, seats_of
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


class SeriesRulesMissing(Exception):
    """Raised when a series the engine derives has no tournament settings, scoring or top divisions."""


@dataclass(frozen=True)
class Series:
    """How one series is named, scored, ranked and shown, as upstream does it.

    points (by player count) and top_divisions come from the tournament settings, through series_rules.
    """

    parse: Callable[[str], Parsed | None]
    duel: bool
    in_records: Callable[[str], bool]
    record_min_games: int
    record_division: str
    season_records: bool = True
    points: dict[int, list[float]] = field(default_factory=dict)
    top_divisions: frozenset[str] = frozenset()


SERIES = {
    "international": Series(
        parse=parse_international,
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
        duel=False,
        in_records=lambda n: _like(n, "Stage 4", "Final", "Upper", "Lower"),
        record_min_games=0,
        record_division="",
        season_records=False,
    ),
}


def series_rules(conn) -> tuple[dict[str, Series], Series]:
    """Returns each series holding one of SERIES' rule sets (its results_rules), by slug, with its points and top
    divisions from its tournament settings, and the World Championship final's rules.

    The World Championship's points score only its final: its other stages have no standings. Raises
    SeriesRulesMissing when a rule set has no series, a league no points or top divisions, or the final no points.
    """
    rows = {
        r["results_rules"]: r
        for r in conn.execute(
            "SELECT se.slug, se.results_rules, se.top_divisions, "
            "coalesce(s.points_2p, '{}') AS points_2p, coalesce(s.points_3p, '{}') AS points_3p, "
            "coalesce(s.points_4p, '{}') AS points_4p "
            "FROM series se LEFT JOIN settings s ON s.series_id = se.series_id WHERE se.results_rules IS NOT NULL"
        ).fetchall()
    }
    rules, final = {}, None
    for name, base in SERIES.items():
        r = rows.get(name)
        if r is None:
            raise SeriesRulesMissing(f"no series holds the {name} rules")
        slug = r["slug"]
        points = {n: list(r[f"points_{n}p"]) for n in (2, 3, 4) if r[f"points_{n}p"]}
        top = frozenset(r["top_divisions"])
        if not points:
            raise SeriesRulesMissing(f"{slug} has no points per place")
        if name == "worlds":
            rules[slug], final = (
                replace(base, top_divisions=top),
                replace(base, points=points),
            )
            continue
        if not top:
            raise SeriesRulesMissing(f"{slug} has no top divisions")
        rules[slug] = replace(base, points=points, top_divisions=top)
    return rules, final


def holder(conn, name) -> str:
    """Returns the slug of the series holding the rule set name."""
    return conn.execute(
        "SELECT slug FROM series WHERE results_rules = %s", (name,)
    ).fetchone()["slug"]


def worlds_finals(conn) -> dict[int, list[list[Seat]]]:
    """Returns each finished World Championship's final, as its played games' seats, by season.

    The final is the last stage of a season whose CGE tournament is finished, when its games are one group.
    """
    rows = conn.execute(
        f"""
        SELECT se.label::int AS season, m.group_name AS group_label, {SEAT_COLUMNS}
        FROM series t
        JOIN tournaments se ON se.series_id = t.series_id
        JOIN stages st ON st.tournament_id = se.tournament_id
        JOIN cge_tournaments c ON c.cge_tournament = st.cge_tournament
        JOIN game_lines m ON m.stage_id = st.stage_id
        WHERE t.results_rules = 'worlds' AND c.status = 'finished' AND se.label ~ '^[0-9]+$'
          AND coalesce(m.state, 'played') = 'played'
          AND st.cge_stage_number = (SELECT max(s2.cge_stage_number) FROM stages s2 WHERE s2.tournament_id = se.tournament_id)
        ORDER BY m.date, m.game_id
        """
    ).fetchall()
    games, groups = defaultdict(list), defaultdict(set)
    for r in rows:
        games[r["season"]].append(seats_of(r))
        groups[r["season"]].add(r["group_label"] or "")
    return {season: g for season, g in games.items() if len(groups[season]) == 1}


def final_podium(games, final: Series):
    """Returns a final's standings under final's rules, or None when they cannot split a tie for a podium place."""
    t = _tally(games)
    order = _league_order(t)
    points, above, performance = t["points"], t["above"], t["performance"]
    for a, b in itertools.pairwise(order[:4]):
        tied = [p for p in order if points[p] == points[a]]
        level = points[a] == points[b] and performance[a] == performance[b]
        if level and (len(tied) != 2 or above[a][b] == above[b][a]):
            return None
    return rank_group(games, final)


def _tally(games):
    """Returns each player's points, games, placings, head-to-head, performance, wins and culture.

    games are each game's seats; points are the seats' game points (a timed-out seat's are its penalty).
    """
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
    for seats in games:
        best = max(s.culture for s in seats)
        winners = sum(1 for s in seats if s.place == 1)
        for s in seats:
            p, got = s.player, s.points or 0.0
            t["points"][p] += got
            t["games"][p] += 1
            t["placings"][p][s.place - 1] += 1
            share = s.culture / best if best > 0 else 0.0
            t["performance"][p] += (
                1.0 if s.place == 1 else max(PERFORMANCE_FLOOR, share)
            )
            t["score"][p] += s.culture
            t["wins"][p] += 1 if s.place == 1 and winners == 1 else 0
            for o in seats:
                if s.place < o.place:
                    t["above"][p][o.player] += 1
                if o.player != p:
                    t["h2h"][p][o.player] += got
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


def rank_group(games: list[list[Seat]], rules: Series):
    """Returns a group's standings as dicts in place order, by its seats' game points and the series' tiebreaks."""
    t = _tally(games)
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


def classify(rows, recorded, corrections, series_rules):
    """Returns a dict per tournament game: match_id, series, season, name_season, division,
    group, source, tiebreak and in_records.

    rows are matches in match_id order; series_rules map each series to its Series; recorded maps (series, season,
    player) to a recorded division; corrections map (series, name season) to
    the real season, or None to leave the games out. A game without a named
    division takes the group CGE recorded for it, else a recorded one only when
    every player in it has the same one; recorded games of one division that
    never meet form numbered groups.
    """
    out = []
    pending = defaultdict(list)
    for r in rows:
        for series, rules in series_rules.items():
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
            # A league game named without its division: the group CGE recorded for it, when it has one.
            cge_group = (r.get("group_label") or "").strip()
            if cge_group and not parsed.tiebreak and rules.points:
                division = re.sub(r"\s+\d+$", "", cge_group)
                g.update(division=division, group=cge_group, source="cge")
                break
            players = list(r["players"])
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


def season_records(rows, rules: Series):
    """Returns the premier seasonal records: one per player and season.

    From upstream's get_premier_seasonal_records: its records games only, and
    seasons of at least record_min_games games; but by the corrected season
    (a season left out has none); points are the seats' game points, tied players sharing their places'.
    placings are the 1st, 2nd, 3rd and
    4th places; in Royal League, wins, draws and losses. rows are in match_id order.
    """
    got = {}
    for r in rows:
        order = by_place(seats_of(r))
        for i, seat in enumerate(order):
            p = seat.player
            rec = got.setdefault(
                (r["season"], p),
                {"division": rules.record_division, "points": 0.0, "games": 0,
                 "placings": [0, 0, 0] if len(rules.points) == 1 else [0, 0, 0, 0],
                 "first_match_id": r["match_id"]},
            )  # fmt: skip
            rec["games"] += 1
            rec["last_date"] = r["date"]
            if rules.duel:
                other = order[1 if i == 0 else 0].place
                result = 0 if seat.place < other else (1 if seat.place == other else 2)
                rec["points"] += (2.0, 1.0, 0.0)[result]
                rec["placings"][result] += 1
            else:
                rec["points"] += seat.points or 0.0
                rec["placings"][seat.place - 1] += 1
    return {k: v for k, v in got.items() if v["games"] >= rules.record_min_games}


def _league_boards(standings, official, series_rules):
    """Returns each league's trophyboard in upstream's order: medals in the top
    division, official placements over derived ones.

    series_rules map each series to its Series; a series without points has no standings, so no board here.
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
        rules = series_rules[series]
        if not rules.points or division not in rules.top_divisions:
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
            title = "GM" if m["gold"] else "M"
            boards[series].append(
                {"player_name": p, **m, "first_season": m["first"], "title": title}
            )
    for board in boards.values():
        board.sort(
            key=lambda b: (-b["gold"], -b["silver"], -b["bronze"], -b["points"], b["first_season"], b["player_name"])
        )  # fmt: skip
    return boards


def _worlds_podiums(conn, run_id, official, standing_rows, final, worlds):
    """Puts each World Championship season's podium into official, by precedence: admin's, recorded, derived.

    worlds is the slug of the series holding the World Championship rules. A derived final, scored by final's
    rules, adds its standings to standing_rows.
    """
    recorded = {season for series, season, _ in official if series == worlds}
    for season, games in sorted(worlds_finals(conn).items()):
        if season in recorded:
            continue
        standings = final_podium(games, final)
        if standings is None:
            continue
        for s in standings:
            standing_rows.append(
                (run_id, worlds, season, "Final", "Final", s["player_name"],
                 s["place"], s["points"], s["games"], s["placings"])
            )  # fmt: skip
        official[(worlds, season, "Final")] = [
            {"series": worlds, "season": season, "division": "Final", **s}
            for s in standings[:3]
        ]
    overrides = defaultdict(list)
    for r in conn.execute(
        "SELECT season::int AS season, place, player_name FROM tournament_podium_overrides "
        "WHERE tournament = %s AND season ~ '^[0-9]+$' ORDER BY place",
        (worlds,),
    ).fetchall():
        overrides[r["season"]].append(
            {"series": worlds, "division": "Final", "points": None, **r}
        )
    for season, placed in overrides.items():
        for key in [k for k in official if k[0] == worlds and k[1] == season]:
            del official[key]
        official[(worlds, season, "Final")] = placed


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

    In one committed transaction. Raises RunNotRunning unless the run is running, and SeriesRulesMissing
    when a series has no settings to score it with.
    """
    with conn.transaction():
        require_running(conn, run_id)
        rules, final = series_rules(conn)
        rows = conn.execute(
            f"SELECT m.game_id AS match_id, m.name AS tournament, m.date, m.player_count, {SEAT_COLUMNS}, "
            "m.group_name AS group_label, t.slug AS catalogue_slug, t.name AS catalogue_name, "
            "se.label AS catalogue_season, st.label AS catalogue_stage "
            "FROM game_lines m LEFT JOIN stages st ON st.stage_id = m.stage_id "
            "LEFT JOIN tournaments se ON se.tournament_id = m.tournament_id "
            "LEFT JOIN series t ON t.series_id = m.series_id WHERE m.state = 'played' ORDER BY m.game_id"
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
            "FROM tournament_official_placements WHERE series = ANY(%s) "
            "ORDER BY place, player_name",
            (list(SERIES),),
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
        classified = classify(rows, recorded, corrections, rules)
        by_id = {r["match_id"]: r for r in rows}

        groups = defaultdict(list)
        records = defaultdict(list)
        for g in classified:
            r = by_id[g["match_id"]]
            # A season without division names has its top division recorded from the organisers' sheet.
            if (
                g["source"] == "recorded"
                and g["division"] in rules[g["series"]].top_divisions
            ):
                g["in_records"] = True
            if (
                g["in_records"]
                and g["season"] is not None
                and not g["tiebreak"]
                and rules[g["series"]].season_records
            ):
                records[g["series"]].append({**r, **g})
            if g["division"] is not None and not g["tiebreak"]:
                key = (g["series"], g["season"], g["division"], g["group"])
                groups[key].append(seats_of(r))

        standing_rows = []
        derived = defaultdict(list)
        for (series, season, division, group), games in groups.items():
            for s in rank_group(games, rules[series]):
                standing_rows.append(
                    (run_id, series, season, division, group, s["player_name"],
                     s["place"], s["points"], s["games"], s["placings"])
                )  # fmt: skip
                derived[(series, season, division)].append(s)
        worlds = holder(conn, "worlds")
        _worlds_podiums(conn, run_id, official, standing_rows, final, worlds)
        boards = _league_boards(derived, official, rules)
        boards["worlds"] = worlds_board(
            [{**p, "season": season} for (series, season, _), placed in official.items()
             if series == worlds for p in placed]
        )  # fmt: skip
        boards["grand_slams"] = grand_slams_board(
            conn.execute(
                "SELECT cup, season, final, finished, place, player_name FROM tournament_cup_podiums"
            ).fetchall(),
            {
                r["slug"]: r["name"]
                for r in conn.execute("SELECT slug, name FROM series").fetchall()
            },
        )
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

        world_places = {
            (season, p["player_name"]): p["place"]
            for (series, season, _), placed in official.items()
            if series == worlds
            for p in placed
        }
        achievement_rows = [
            (run_id, a["player_name"], a["world_titles"], a["international_titles"],
             a["intermezzo_titles"], a["royal_league_titles"], a["gold"], a["silver"],
             a["bronze"], a["accolades"])
            for a in player_achievements(boards, world_places)
        ]  # fmt: skip
        player_record_rows = player_records(
            rows=assembled_order(rows),
            top_divisions={
                s: rules[s].top_divisions
                for s in (holder(conn, "international"), holder(conn, "intermezzo"))
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
            for (season, p), rec in season_records(top_rows, rules[series]).items()
        ]  # fmt: skip

        written = (
            "tournament_records",
            "tournament_achievements",
            "tournament_series",
            "tournament_legends",
            "tournament_season_records",
            "tournament_boards",
            "tournament_standings",
            "tournament_games",
        )
        for table in written:
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
        # Without fresh statistics, the titles' query on tournament_games gets a plan for the run's rows missing.
        for table in written:
            conn.execute(f"ANALYZE {table}")
