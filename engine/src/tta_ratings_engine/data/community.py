"""The community leaderboard's points for each player in each event, as the organisers score them.

A season's places come from CGE's standings tables (recorded facts), or, for
a league season CGE's tables lack, from the run's own standings. Each place is
worth what the organisers' points table for the season's edition gives
(recorded facts, versioned by edition). How a place maps to a table cell
depends on the tournament: a league's division, a World Championship's last
stage, a cup's round, the Eiffel Tower's floor.
"""

import re
from collections import Counter, defaultdict
from datetime import date
from typing import Any

from tta_ratings_engine.data.db import DictConnection, require_running


class CommunitySeasonUndated(Exception):
    """Raised when a season scores in the community leaderboard with neither a quarter nor a game to date it."""


class CommunityCodeMissing(Exception):
    """Raised when a series scores in the community leaderboard without its code in the organisers' sheet."""


ROUNDS = {256: "R256", 128: "R128", 64: "R64", 32: "R32", 16: "QF", 8: "SF", 4: "Final"}
FLOORS = {"The Top", "The Elevator", "2nd Floor", "The Stairs"}


def table_for(points, tournament, edition):
    """Returns a tournament's points table for an edition: the latest from or before it, else the earliest.

    points maps (tournament, from_edition) to {(group, place): points}.
    """
    editions = sorted(e for t, e in points if t == tournament)
    if not editions:
        return {}
    before = [e for e in editions if e <= edition]
    return points[(tournament, before[-1] if before else editions[0])]


def place_points(table, group, place, upper=False):
    """Returns what a place in a group earns.

    A player leaving from an Upper group takes the group's "0" row where it
    has one; a place the table does not list takes the next place it does.
    """
    listed = sorted(p for g, p in table if g == group)
    if upper and 0 in listed:
        return table[(group, 0)]
    nxt = next((p for p in listed if p >= place and p > 0), None)
    return 0 if nxt is None else table[(group, nxt)]


def division(group):
    """Returns a league group's division: its name without a group number ("Bronze 12" is Bronze)."""
    return re.sub(r"\s+\d+$", "", group)


def round_name(players):
    """Returns a cup round's name by how many players its stage holds, or None for an odd stage."""
    return ROUNDS.get(players)


def wimbledon_round(stage, stages):
    """Returns a Wimbledon stage's round: the last is the Final, the one before the SF, the rest R and their number."""
    last = sorted(stages)
    if stage == last[-1]:
        return "Final"
    if stage == last[-2]:
        return "SF"
    return f"R{stage}"


def eiffel_floor(group):
    """Returns an Eiffel Tower group's floor in the points table: its own name, or Other for the lower floors."""
    return group if group in FLOORS else "Other"


# The organisers' sheet editions, as ours: its "2026" is the rolling year to 2026 Q2, Race 26 is 2026.
SHEET_EDITIONS = {
    "2024": "2024",
    "2025": "2025",
    "2026": "rolling 2026 Q2",
    "race26": "2026",
}


def event_code(code, season):
    """Returns the organisers' code for a season from its series' code: ic_{} gives ic_30, wrld stays wrld."""
    return code.format(int(season)) if "{" in code else code


def code_of(codes, slug):
    """Returns slug's code from series_codes, or raises CommunityCodeMissing naming it."""
    if slug not in codes:
        raise CommunityCodeMissing(
            f"{slug} scores in the community leaderboard but has no community code"
        )
    return codes[slug]


def series_codes(conn):
    """Returns each series' code in the organisers' sheet, by slug, for those that have one."""
    return {
        r["slug"]: r["community_code"]
        for r in conn.execute(
            "SELECT slug, community_code FROM series WHERE community_code IS NOT NULL"
        )
    }


def editions(first, latest):
    """Returns {edition: set of (year, quarter)}: every calendar year from the first event's to the latest
    game's, and the four quarters to each quarter that is over, from the first event's quarter on."""
    got = {
        str(y): {(y, q) for q in (1, 2, 3, 4)} for y in range(first[0], latest.year + 1)
    }
    quarter = (first[0], first[1])
    current = (latest.year, (latest.month - 1) // 3 + 1)
    while quarter < current:
        y, q = quarter
        got[f"rolling {y} Q{q}"] = {
            (y - (q - i <= 0), (q - i - 1) % 4 + 1) for i in range(4)
        }
        quarter = (y + q // 4, q % 4 + 1)
    return got


def best_eight(events):
    """Returns a player's total, their eight best events' points, and how many events scored."""
    scores = sorted(events.values(), reverse=True)
    return sum(scores[:8]), sum(1 for x in scores if x > 0)


# The organisers' sheet columns, left to right: of equal points, the event
# further left counts (its highlight rule, RANK plus COUNTIF to the left).
COLUMNS = [
    "wrld",
    "ic",
    "im",
    "rl",
    "wmbl",
    "aus",
    "surv",
    "snail",
    "sl",
    "ml",
    "eif",
    "qd",
    "ldrb",
]


def sheet_column(code):
    """Returns where an event's column sits in the organisers' sheet: kind, then season."""
    kind, _, season = code.partition("_")
    place = COLUMNS.index(kind) if kind in COLUMNS else len(COLUMNS)
    return (place, int(season) if season.isdigit() else 0, code)


def counted(events):
    """Returns the codes of a player's eight best events, ties going to the sheet's earlier column."""
    return set(sorted(events, key=lambda c: (-events[c], sheet_column(c)))[:8])


def ranked(totals):
    """Returns [(rank, player)] by total, highest first; equal totals share a rank."""
    order = sorted(totals, key=lambda p: (-totals[p], p.lower()))
    got = []
    for i, p in enumerate(order, start=1):
        got.append((got[-1][0] if got and totals[p] == totals[got[-1][1]] else i, p))
    return got


def rungs(groups):
    """Returns {group: rung}: the top tier (a group without a number) first, then element numbers from the highest.

    Every group of one element shares its rung.
    """
    numbered = {
        g: int(re.match(r"-?\d+", g).group(0)) for g in groups if re.match(r"-?\d+", g)
    }
    top = [g for g in groups if g not in numbered]
    order = sorted(set(numbered.values()), reverse=True)
    got = {g: 1 for g in top}
    for g, n in numbered.items():
        got[g] = (1 if top else 0) + order.index(n) + 1
    return got


def rung_value(rung, rule):
    """Returns what a rung's winner earns before rounding: 1000 × ratio^steps, half a step a rung after half_steps_from."""
    half = rule["half_steps_from"]
    steps = rung - 1 if not half or rung <= half else half - 1 + (rung - half) / 2
    return 1000 * rule["ratio"] ** steps


def round_points(games, rule):
    """Returns {player: points} for one ladder round: games are (group, [(player, place)]).

    Each place earns its share of the rung's value, rounded; tied players average their places' unrounded shares.
    """
    rung = rungs([g for g, _ in games])
    shares = rule["shares"]
    got = {}
    for group, seats in games:
        value = rung_value(rung[group], rule)
        for name, place in seats:
            better = sum(1 for _, o in seats if o < place)
            tied = sum(1 for _, o in seats if o == place)
            share = (
                sum(
                    shares[p] if p < len(shares) else 0
                    for p in range(better, better + tied)
                )
                / tied
            )
            got[name] = got.get(name, 0) + round(value * share)
    return got


def scored(kind, lines, table):
    """Returns {player: points} for one season's lines (stage, group, place, name, and points for floors), by its
    kind, from table's points by (group, place)."""
    if kind == "league":
        return {
            ln["name"]: place_points(table, division(ln["group"]), ln["place"])
            for ln in lines
        }
    last = {}
    for ln in sorted(lines, key=lambda ln: ln["stage"]):
        last[ln["name"]] = ln
    if kind == "worlds":
        return {
            n: place_points(
                table, f"Stage {ln['stage']}", ln["place"], upper="Upper" in ln["group"]
            )
            for n, ln in last.items()
        }
    if kind == "rounds":
        players = Counter()
        for ln in lines:
            players[ln["stage"]] += 1
        return {
            n: place_points(table, round_name(players[ln["stage"]]), ln["place"])
            for n, ln in last.items()
        }
    if kind == "knockout":
        stages = {ln["stage"] for ln in lines}
        return {
            n: place_points(table, wimbledon_round(ln["stage"], stages), ln["place"])
            for n, ln in last.items()
        }
    if kind == "floors":
        # Timeouts cost Eiffel Tower points; a player not above 0 takes the table's "<0" row: nothing.
        return {
            n: place_points(table, eiffel_floor(ln["group"]), ln["place"])
            if ln["points"] > 0
            else 0
            for n, ln in last.items()
        }
    raise ValueError(f"unknown kind {kind!r}")


def derive_community(*, conn: DictConnection, run_id: int) -> None:
    """Replaces the run's community event points, for every season of a tournament in the leaderboard.

    In one committed transaction. Raises RunNotRunning unless the run is running, CommunitySeasonUndated for a
    season with nothing to date it by, and CommunityCodeMissing for a series without its sheet code.
    """
    with conn.transaction():
        require_running(conn, run_id)
        points: defaultdict[tuple[str, Any], dict[tuple[str, Any], Any]] = defaultdict(
            dict
        )
        for r in conn.execute(
            "SELECT tournament, from_edition, group_name, place, points FROM community_points"
        ):
            points[(r["tournament"], r["from_edition"])][
                (r["group_name"], r["place"])
            ] = r["points"]
        names = {
            r["cge_user"]: r["name"]
            for r in conn.execute(
                "SELECT cge_user, mode() WITHIN GROUP (ORDER BY player_name) AS name FROM seats "
                "WHERE cge_user IS NOT NULL GROUP BY cge_user"
            )
        }
        seasons = conn.execute(
            # A series' community_scoring says how its places score; a cup is one CGE tournament a season, the
            # others hold every season in one.
            """SELECT t.slug, t.community_scoring AS scoring, t.kind = 'cup' AS cup, s.label, s.tournament_id AS season_id, q.year,
                      (SELECT extract(year FROM max(m.date))::int FROM game_lines m
                       WHERE m.tournament_id = s.tournament_id) AS finished
               FROM tournaments s JOIN series t USING (series_id)
               LEFT JOIN community_quarters q ON q.tournament = t.slug AND q.season = s.label
               WHERE t.in_community_leaderboard AND s.in_community AND t.community_scoring IS NOT NULL"""
        ).fetchall()
        rows = []
        for s in seasons:
            lines = [
                {"stage": r["cge_season"], "group": r["group_name"], "place": r["place"] or r["row_number"],
                 "name": names.get(r["cge_user"], r["player_name"]), "points": r["points"]}
                for r in conn.execute(
                    # A cup's season is every stage of its CGE tournament; the others', their linked stages.
                    """SELECT c.cge_season, c.group_name, c.place, c.row_number, c.cge_user, c.player_name, c.points
                       FROM cge_standings c
                       WHERE c.games > 0 AND EXISTS (
                         SELECT 1 FROM stages x WHERE x.tournament_id = %s
                         AND x.cge_tournament = c.cge_tournament AND (%s OR x.cge_stage_number = c.cge_season))""",
                    (s["season_id"], s["cup"]),
                )
            ]  # fmt: skip
            if not lines and s["scoring"] == "league" and s["label"].isdigit():
                lines = [
                    {"stage": 0, "group": r["division"], "place": r["place"], "name": r["player_name"]}
                    for r in conn.execute(
                        "SELECT division, place, player_name FROM tournament_standings "
                        "WHERE run_id = %s AND series = %s AND season = %s",
                        (run_id, s["slug"], int(s["label"])),
                    )
                ]  # fmt: skip
            if not lines:
                continue
            edition = s["year"] or s["finished"]
            if edition is None:
                raise CommunitySeasonUndated(
                    f"{s['slug']} season {s['label']} has CGE standings but no quarter and no games"
                )
            table = table_for(points, s["slug"], edition)
            for name, pts in scored(s["scoring"], lines, table).items():
                rows.append((run_id, s["slug"], s["label"], name, pts))
        _rank_editions(conn, run_id, rows, points)
        conn.execute("DELETE FROM community_event_points WHERE run_id = %s", (run_id,))
        with conn.cursor() as cur:
            cur.executemany(
                "INSERT INTO community_event_points VALUES (%s, %s, %s, %s, %s)", rows
            )


def _rank_editions(conn, run_id, rows, points):
    """Replaces the run's editions: each player's events in each, their best eight, and the ranking.

    rows are the run's computed event points. An event the engine does not
    compute takes the organisers' sheet points, in the editions the sheet has.
    """
    codes = series_codes(conn)
    quarters = {
        (r["tournament"], r["season"]): (r["year"], r["quarter"])
        for r in conn.execute(
            "SELECT tournament, season, year, quarter FROM community_quarters"
        )
    }
    finished = {
        (r["slug"], r["label"]): r["last"]
        for r in conn.execute(
            "SELECT t.slug, s.label, max(m.date) AS last FROM game_lines m "
            "JOIN tournaments s ON s.tournament_id = m.tournament_id JOIN series t ON t.series_id = m.series_id GROUP BY 1, 2"
        )
    }
    latest = conn.execute(
        "SELECT max(date) AS d FROM games WHERE state = 'played'"
    ).fetchone()["d"]
    events = defaultdict(dict)  # (year, quarter) -> {(player, code): points}
    chosen = defaultdict(
        dict
    )  # (year, quarter) -> {(player, code): the season's finish and label}
    for _, slug, season, name, pts in rows:
        q = quarters.get((slug, season))
        if q is None and finished.get((slug, season)):
            d = finished[(slug, season)]
            q = (d.year, (d.month - 1) // 3 + 1)
        if q is None:
            continue
        # Two counting seasons giving one event in a quarter: the later finished, then the later label, counts;
        # never whichever row came last. A season that should not count is switched off in admin.
        key = (name, event_code(code_of(codes, slug), season))
        rank = (finished.get((slug, season)) or date.min, season)
        if key not in chosen[q] or rank > chosen[q][key]:
            chosen[q][key], events[q][key] = rank, pts
    ladders = _ladder_rounds(conn)
    firsts = list(events) + [
        (d.year, (d.month - 1) // 3 + 1)
        for _, _, rounds in ladders.values()
        for _, d, _, _ in rounds
    ]
    if not firsts or latest is None:
        conn.execute(
            "DELETE FROM community_edition_points WHERE run_id = %s", (run_id,)
        )
        conn.execute("DELETE FROM community_rankings WHERE run_id = %s", (run_id,))
        return
    spelling = {
        r["name"].lower(): r["name"] for r in conn.execute("SELECT name FROM players")
    }
    sheet = defaultdict(dict)
    for r in conn.execute(
        "SELECT edition, player_name, event, score FROM community_scores"
    ):
        if r["edition"] in SHEET_EDITIONS:
            name = spelling.get(r["player_name"].lower(), r["player_name"])
            sheet[SHEET_EDITIONS[r["edition"]]][(name, r["event"])] = r["score"]
    point_rows, rank_rows = [], []
    for edition, in_it in editions(min(firsts), latest).items():
        scored = defaultdict(dict)
        sources = {}
        for q in in_it:
            for (name, code), pts in events.get(q, {}).items():
                scored[name][code] = pts
                sources[(name, code)] = "computed"
        for (name, code), pts in _ladder_points(
            ladders, edition, in_it, points, codes
        ).items():
            scored[name][code] = pts
            sources[(name, code)] = "computed"
        computed = {code for _, code in sources}
        for (name, code), pts in sheet.get(edition, {}).items():
            if code not in computed:
                scored[name][code] = pts
                sources[(name, code)] = "sheet"
        totals, counts = {}, {}
        for name, evs in scored.items():
            totals[name], counts[name] = best_eight(evs)
            best = counted(evs)
            for code, pts in evs.items():
                point_rows.append(
                    (
                        run_id,
                        edition,
                        name,
                        code,
                        pts,
                        sources[(name, code)],
                        code in best,
                    )
                )
        rank_rows += [
            (run_id, edition, rank, p, totals[p], counts[p])
            for rank, p in ranked(totals)
        ]
    conn.execute("DELETE FROM community_edition_points WHERE run_id = %s", (run_id,))
    conn.execute("DELETE FROM community_rankings WHERE run_id = %s", (run_id,))
    with conn.cursor() as cur:
        cur.executemany(
            "INSERT INTO community_edition_points VALUES (%s, %s, %s, %s, %s, %s, %s)",
            point_rows,
        )
        cur.executemany(
            "INSERT INTO community_rankings VALUES (%s, %s, %s, %s, %s, %s)", rank_rows
        )


def _ladder_rounds(conn):
    """Returns each ladder's rule and rounds: {slug: (rules by from_edition, [(round, finished, games or None, year)])}.

    A round's year is the one the organisers recorded for it (community_quarters), else the year it finished.

    A round finished on its last game's CGE finish date, else its match file date.
    games are (group, [(player, score)]) of the round's played games, or None
    when any lacks a group name, so the round cannot be scored.
    """
    rules = defaultdict(dict)
    for r in conn.execute(
        "SELECT tournament, from_edition, ratio, half_steps_from, shares FROM community_ladders"
    ):
        rules[r["tournament"]][r["from_edition"]] = {
            "ratio": r["ratio"],
            "half_steps_from": r["half_steps_from"],
            "shares": r["shares"],
        }
    sizes = {
        r["tournament"]: r["rounds"]
        for r in conn.execute(
            "SELECT DISTINCT ON (tournament) tournament, rounds FROM community_ladders ORDER BY tournament, from_edition DESC"
        )
    }
    got = {}
    for slug in rules:
        years = {
            r["season"]: r["year"]
            for r in conn.execute(
                "SELECT season, year FROM community_quarters WHERE tournament = %s",
                (slug,),
            )
        }
        rounds = defaultdict(list)
        finished = {}
        for r in conn.execute(
            """SELECT s.label, coalesce(m.cge_finished, m.date) AS date, m.group_name AS group_label, m.players, m.places
               FROM game_lines m JOIN tournaments s ON s.tournament_id = m.tournament_id
               JOIN series t ON t.series_id = m.series_id
               WHERE t.slug = %s AND t.in_community_leaderboard AND s.in_community AND m.state = 'played'""",
            (slug,),
        ):
            seats = list(zip(r["players"], r["places"]))
            rounds[r["label"]].append((r["group_label"], seats))
            finished[r["label"]] = max(finished.get(r["label"], r["date"]), r["date"])
        got[slug] = (
            rules[slug],
            sizes[slug],
            [
                (
                    label,
                    finished[label],
                    None if any(g == "" for g, _ in games) else games,
                    years.get(label, finished[label].year),
                )
                for label, games in sorted(
                    rounds.items(), key=lambda kv: finished[kv[0]]
                )
            ],
        )
    return got


def _ladder_points(ladders, edition, quarters, points, codes):
    """Returns {(player, code): points} for each ladder an edition can score, code being its series' (codes).

    A calendar year sums its rounds (by their year); a rolling year the latest
    rounds finished by its end. The totals are ranked (equal totals share a
    rank) into the ladder's leaderboard table. A ladder with a round in the
    window that cannot be scored, or an edition before its first rule, is left
    out, for the sheet's points to stand.
    """
    last = max(quarters)
    end = date(
        last[0] + last[1] // 4, last[1] % 4 * 3 + 1, 1
    )  # the day after the edition
    got = {}
    for slug, (rules, size, rounds) in ladders.items():
        done = [r for r in rounds if r[1] < end]
        window = (
            [r for r in done if r[3] == last[0]] if edition.isdigit() else done[-size:]
        )
        if not window or any(games is None for _, _, games, _ in window):
            continue
        year = last[0]
        known = [e for e in rules if e <= year]
        if not known:
            continue  # scored by a rule not recorded: the sheet's points stand
        rule = rules[max(known)]
        totals = Counter()
        for _, _, games, _ in window:
            totals.update(round_points(games, rule))
        table = table_for(points, slug, year)
        for rank, name in ranked({n: t for n, t in totals.items() if t > 0}):
            got[(name, code_of(codes, slug))] = table.get(("leaderboard", rank), 0)
    return got
