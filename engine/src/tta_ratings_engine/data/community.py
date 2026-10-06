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

import psycopg

from tta_ratings_engine.data.db import require_running

# How each tournament's places are scored, by its slug.
LEAGUES = {"international", "intermezzo"}
# Cups are one CGE tournament a season; the others hold every season in one.
CUPS = {"worlds", "australian_open", "wimbledon"}
KINDS = {
    "international": "league",
    "intermezzo": "league",
    "worlds": "worlds",
    "australian_open": "rounds",
    "wimbledon": "knockout",
    "eiffel_tower": "floors",
}
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


# The organisers' event codes, as their sheet heads its columns.
CODES = {
    "worlds": "wrld",
    "australian_open": "aus",
    "wimbledon": "wmbl",
    "eiffel_tower": "eif",
}
SEASON_CODES = {
    "international": "ic_{}",
    "intermezzo": "im_{}",
    "royal_league": "rl_{:02d}",
}
# The organisers' sheet editions, as ours: its "2026" is the rolling year to 2026 Q2, Race 26 is 2026.
SHEET_EDITIONS = {
    "2024": "2024",
    "2025": "2025",
    "2026": "rolling 2026 Q2",
    "race26": "2026",
}


def event_code(tournament, season):
    """Returns the organisers' code for a season: ic_30, rl_05, or the tournament's own (wrld, aus …)."""
    if tournament in SEASON_CODES:
        return SEASON_CODES[tournament].format(int(season))
    return CODES[tournament]


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


LADDER_CODES = {"mercurial_ladder": "ml", "sodium_ladder": "sl"}


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
    """Returns {player: points} for one ladder round: games are (group, [(player, score)]).

    Each place earns its share of the rung's value, rounded; tied players average their places' unrounded shares.
    """
    rung = rungs([g for g, _ in games])
    shares = rule["shares"]
    got = {}
    for group, seats in games:
        value = rung_value(rung[group], rule)
        for name, score in seats:
            better = sum(1 for _, o in seats if o > score)
            tied = sum(1 for _, o in seats if o == score)
            share = (
                sum(
                    shares[p] if p < len(shares) else 0
                    for p in range(better, better + tied)
                )
                / tied
            )
            got[name] = got.get(name, 0) + round(value * share)
    return got


def _scored(kind, lines, table):
    """Returns {player: points} for one season's lines (stage, group, place, user, name), by its kind."""
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


def derive_community(*, conn: psycopg.Connection, run_id: int) -> None:
    """Replaces the run's community event points, for every season of a tournament in the leaderboard.

    In one committed transaction. Raises RunNotRunning unless the run is running.
    """
    with conn.transaction():
        require_running(conn, run_id)
        points = defaultdict(dict)
        for r in conn.execute(
            "SELECT tournament, from_edition, group_name, place, points FROM community_points"
        ):
            points[(r["tournament"], r["from_edition"])][
                (r["group_name"], r["place"])
            ] = r["points"]
        names = {
            r["cge_user"]: r["name"]
            for r in conn.execute(
                "SELECT cge_user, mode() WITHIN GROUP (ORDER BY player_name) AS name FROM match_seats "
                "WHERE cge_user IS NOT NULL GROUP BY cge_user"
            )
        }
        seasons = conn.execute(
            """SELECT t.slug, s.label, s.season_id, q.year,
                      (SELECT extract(year FROM max(m.date))::int FROM matches m JOIN tournament_stages x USING (stage_id)
                       WHERE x.season_id = s.season_id) AS finished
               FROM tournament_seasons s JOIN tournaments t USING (tournament_id)
               LEFT JOIN community_quarters q ON q.tournament = t.slug AND q.season = s.label
               WHERE t.in_community_leaderboard AND t.slug = ANY(%s)""",
            (list(KINDS),),
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
                         SELECT 1 FROM tournament_stages x WHERE x.season_id = %s
                         AND x.cge_tournament = c.cge_tournament AND (%s OR x.cge_season = c.cge_season))""",
                    (s["season_id"], s["slug"] in CUPS),
                )
            ]  # fmt: skip
            if not lines and s["slug"] in LEAGUES and s["label"].isdigit():
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
            table = table_for(points, s["slug"], edition)
            for name, pts in _scored(KINDS[s["slug"]], lines, table).items():
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
    quarters = {
        (r["tournament"], r["season"]): (r["year"], r["quarter"])
        for r in conn.execute(
            "SELECT tournament, season, year, quarter FROM community_quarters"
        )
    }
    finished = {
        (r["slug"], r["label"]): r["last"]
        for r in conn.execute(
            "SELECT t.slug, s.label, max(m.date) AS last FROM matches m JOIN tournament_stages x USING (stage_id) "
            "JOIN tournament_seasons s USING (season_id) JOIN tournaments t USING (tournament_id) GROUP BY 1, 2"
        )
    }
    latest = conn.execute(
        "SELECT max(date) AS d FROM matches WHERE state = 'played'"
    ).fetchone()["d"]
    events = defaultdict(dict)  # (year, quarter) -> {(player, code): points}
    for _, slug, season, name, pts in rows:
        q = quarters.get((slug, season))
        if q is None and finished.get((slug, season)):
            d = finished[(slug, season)]
            q = (d.year, (d.month - 1) // 3 + 1)
        if q is not None:
            events[q][(name, event_code(slug, season))] = pts
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
            ladders, edition, in_it, points
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
            """SELECT s.label, coalesce(m.cge_finished, m.date) AS date, m.group_label, m.player1, m.score1, m.player2, m.score2,
                      m.player3, m.score3, m.player4, m.score4
               FROM matches m JOIN tournament_stages x USING (stage_id) JOIN tournament_seasons s USING (season_id)
               JOIN tournaments t USING (tournament_id)
               WHERE t.slug = %s AND t.in_community_leaderboard AND m.state = 'played'""",
            (slug,),
        ):
            seats = [
                (r[f"player{i}"], r[f"score{i}"])
                for i in range(1, 5)
                if r[f"player{i}"]
            ]
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


def _ladder_points(ladders, edition, quarters, points):
    """Returns {(player, code): points} for each ladder an edition can score.

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
            got[(name, LADDER_CODES[slug])] = table.get(("leaderboard", rank), 0)
    return got
