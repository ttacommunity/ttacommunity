"""Each player's tournament achievements for a run: the medal summary from the
four major circuits' boards, and the records their achievements page lists.

Ported from upstream's hall_of_fame.py (sync_tournament_achievements) and
parse_tournaments.py (parse_all_tournaments), reading the run's standings and
the recorded facts instead of spreadsheets. The records keep upstream's rules.
"""

import re
from collections import defaultdict

from tta_ratings_engine.data.seats import by_place, seats_of

PLACES = ("gold", "silver", "bronze")
FALLBACK_DATE = "2026-01-01"
CAREER = "Career / All-Time"
# Each league's name, and its career division and season details, on the sheet rows.
SHEETS = {
    "international": ("International Championship", "Grandmaster / Hall of Fame", "International Championship Standing"),
    "intermezzo": ("Intermezzo Championship", "Hall of Fame", "Intermezzo Standing"),
}  # fmt: skip
RL_PLACES = (
    ("Gold Medal", "Royal League Emperor Division Winner"),
    ("Silver Medal", "Royal League Emperor Division Runner-up"),
    ("Bronze Medal", "Royal League Emperor Division 3rd Place"),
)
LADDER_WORDS = ("ladder", "tcl", "mercurial", "sodium")


def player_achievements(
    boards: dict[str, list[dict]], worlds: dict[tuple[int, str], int]
) -> list[dict]:
    """Returns each player's titles, medals and featured accolades.

    boards maps a series to its board rows (player_name, gold, silver,
    bronze); worlds maps (year, player) to a World Championship place. A
    player with no title and no medal is left out.
    """
    got = {}

    def row(p):
        return got.setdefault(p, {
            "player_name": p, "world_titles": 0, "international_titles": 0, "intermezzo_titles": 0,
            "royal_league_titles": 0, "gold": 0, "silver": 0, "bronze": 0, "accolades": [],
        })  # fmt: skip

    for b in boards.get("worlds", []):
        r = row(b["player_name"])
        years = {place: sorted((y for (y, p), pl in worlds.items() if p == b["player_name"] and pl == place),
                               reverse=True) for place in (1, 2)}  # fmt: skip
        r["world_titles"] += b["gold"]
        for m in PLACES:
            r[m] += b[m]
        r["accolades"] += [f"{y} World Champion" for y in years[1]] if b["gold"] else []
        r["accolades"] += (
            [f"{y} Vice-Champion" for y in years[2]] if b["silver"] else []
        )
    for series, titles, champion in (
        ("international", "international_titles", "International Champion"),
        ("intermezzo", "intermezzo_titles", "Intermezzo Champion"),
        ("royal_league", "royal_league_titles", "Royal League Emperor Champion"),
    ):
        for b in boards.get(series, []):
            r = row(b["player_name"])
            r[titles] += b["gold"]
            for m in PLACES:
                r[m] += b[m]
            if b["gold"]:
                r["accolades"].append(f"{b['gold']}x {champion}")
    kept = [r for r in got.values() if r["gold"] + r["silver"] + r["bronze"] > 0]
    for r in kept:
        r["accolades"] = r["accolades"][:6]
    return kept


# Tournaments recorded from the catalogue rather than from match names, as editions of stages.
CATALOGUE_RECORDS = frozenset({"slow_burn"})


def record_key(row) -> tuple[str, str, str]:
    """Returns a match's record tournament, season and division: from the catalogue for CATALOGUE_RECORDS
    ("Slow Burn 1 (2024)", its stage, its group), else read from its name as upstream does."""
    if row.get("catalogue_slug") in CATALOGUE_RECORDS:
        stage = row["catalogue_stage"] or row["catalogue_season"]
        return (
            f"{row['catalogue_name']} {row['catalogue_season']}",
            stage,
            row.get("group_label") or stage,
        )
    return campaign(row["tournament"])


def campaign(name: str) -> tuple[str, str, str]:
    """Returns the tournament, season and division upstream reads from a match name."""
    if not name:
        return ("Unknown Tournament", "Season", "Main")
    s = name.strip()
    s = re.sub(r"[\s\-_]+game\s*\d+.*$", "", s, flags=re.IGNORECASE)
    s = re.sub(r"[\s\-_]+g\d+.*$", "", s, flags=re.IGNORECASE)
    s = s.strip(" -_")
    for pattern, tournament, default in (
        (
            r"International\s+S(\d+)(?:\s*-\s*(.*))?",
            "International Championship",
            "Grandmaster",
        ),
        (
            r"Intermezzo\s+S(\d+)(?:\s*-\s*(.*))?",
            "Intermezzo Championship",
            "Grandmaster",
        ),
        (r"RL_s0?(\d+)(?:\s*-\s*(.*))?", "Royal League", "Division"),
    ):
        m = re.match(pattern, s, re.IGNORECASE)
        if m:
            return (tournament, f"Season {m.group(1)}", m.group(2) or default)
    m = re.match(
        r"Worlds\s+(\d+)(?:\s*[-–]\s*)?(stage\s*\d+)?(?:\s*[-–]\s*(.*))?",
        s,
        re.IGNORECASE,
    )
    if m:
        stage = m.group(2) or "Championship"
        return (f"World Championship {m.group(1)}", stage.title(), m.group(3) or stage)
    # Upstream files NL_s under Sodium Ladder and calls DC_s "Diamond Cup"; here they are their own tournaments.
    for pattern, tournament, default in (
        (
            r"(?:Mercurial\s+Season|ML_[sr])\s*0?(\d+)(?:\s*-\s*(.*))?",
            "Mercurial Ladder",
            "Ladder Division",
        ),
        (
            r"(?:Sodium\s+Season|Sodium\s+Ladder\s+Season)\s*0?(\d+)(?:\s*-\s*(.*))?",
            "Sodium Ladder",
            "Ladder Division",
        ),
        (r"NL_s0?(\d+)(?:\s*-\s*(.*))?", "Nations League", "Division"),
        (
            r"(?:PL_s|Premier\s+League\s+)0?(\d+)(?:\s*-\s*(.*))?",
            "Premier League",
            "Division",
        ),
        (r"DC_s0?(\d+)(?:\s*-\s*(.*))?", "Duel Curve", "Division"),
        (r"Slow\s+Burn\s*S?0?(\d+)(?:\s*-\s*(.*))?", "Slow Burn", "Division"),
        (r"Eiffel\s+Tower\s+S(\d+)(?:\s*-\s*(.*))?", "Eiffel Tower Cup", "Division"),
    ):
        m = re.match(pattern, s, re.IGNORECASE)
        if m:
            return (tournament, f"Season {int(m.group(1))}", m.group(2) or default)
    for pattern, tournament in (
        (
            r"Survivors\s+Cup\s+(\d+)(?:\s+Stage\s+(\d+))?(?:\s*-\s*(.*))?",
            "Survivors Cup",
        ),
        (
            r"Leaderboard\s+Trophy\s+(\d+)(?:\s+Stage\s+(\d+))?(?:\s*-\s*(.*))?",
            "Leaderboard Trophy",
        ),
    ):
        m = re.match(pattern, s, re.IGNORECASE)
        if m:
            year, stage, sub = m.groups()
            stage_part = f"Stage {stage}" if stage else None
            division = " - ".join(p for p in (stage_part, sub) if p) or "Championship"
            return (tournament, year, division)
    parts = [p.strip() for p in s.split(" - ") if p.strip()]
    if len(parts) >= 2:
        return (parts[0], parts[1], parts[2] if len(parts) > 2 else "Main")
    return (s, "Tournament", "Main")


def finish_date(
    tournament: str,
    season: str,
    fallback: str | None,
    dates: dict[tuple[str, int], str],
) -> str:
    """Returns a league season's recorded finish date, else fallback, else upstream's 2026-01-01."""
    for word, series in (("International", "international"), ("Intermezzo", "intermezzo"),
                         ("Royal League", "royal_league")):  # fmt: skip
        if word in tournament:
            m = re.search(r"Season\s+(\d+)", season)
            if m and (series, int(m.group(1))) in dates:
                return dates[(series, int(m.group(1)))]
            break
    return fallback or FALLBACK_DATE


def _medal(place: int) -> str | None:
    return PLACES[place - 1] if place <= 3 else None


def _pct(wins: int, games: int) -> float:
    return round(wins / max(1, games) * 100.0, 1)


def _worlds_rows(official, dates):
    champions = sorted((season, p["player_name"]) for (series, season, _), placed in official.items()
                       if series == "worlds" for p in placed if p["place"] == 1)  # fmt: skip
    reigning = max((s for s, _ in champions), default=None)
    rows, keys = [], {}
    for year, p in champions:
        title = "Reigning World Champion" if year == reigning else "World Champion"
        rec = {"player_name": p, "tournament": "World Championship", "season": str(year),
               "division": "Championship Final", "placement": "1 / 4", "points": "Gold Medal & World Title",
               "medal": "gold", "details": f"Through the Ages World Championship {year} Winner & {title}",
               "finish_date": dates.get(("worlds", year), FALLBACK_DATE), "career": False}  # fmt: skip
        rows.append(rec)
        for key in ((p, "World Championship", str(year)), (p, f"World Championship {year}", "Championship"),
                    (p, f"World Championship {year}", str(year))):  # fmt: skip
            keys[key] = rec
    return rows, keys


def _sheet_rows(series, top, standings, official, boards, dates):
    """Returns a league's rows as upstream reads them from the organisers' sheet:
    a career row and one row per top-division season for each player.

    A season ranks its players by points, a tie in the sheet's order (most
    golds, silvers, bronzes, then career points).
    """
    tournament, career_division, standing = SHEETS[series]
    seasons = defaultdict(dict)
    for (s, season, division), placed in {**standings, **official}.items():
        if s == series and division in top:
            for p in placed:
                seasons[season][p["player_name"]] = p["points"] or 0.0
    career = defaultdict(
        lambda: {"gold": 0, "silver": 0, "bronze": 0, "points": 0.0, "seasons": 0}
    )
    for season, scores in seasons.items():
        for p, points in scores.items():
            career[p]["points"] += points
            career[p]["seasons"] += 1
    for b in boards.get(series, []):
        career[b["player_name"]].update({m: b[m] for m in PLACES})
    order = sorted(career, key=lambda p: (-career[p]["gold"], -career[p]["silver"], -career[p]["bronze"],
                                          -career[p]["points"], p))  # fmt: skip
    ranks = {}
    for season, scores in seasons.items():
        ranked = sorted((p for p in order if p in scores), key=lambda p: -scores[p])
        for rank, p in enumerate(ranked, start=1):
            ranks[(p, season)] = (rank, len(ranked), scores[p])

    rows, keys = [], {}
    for p in order:
        c = career[p]
        first = next((m for m in PLACES if c[m] > 0), None)
        rows.append({"player_name": p, "tournament": tournament, "season": f"Career ({c['seasons']} seasons)",
                     "division": career_division,
                     "placement": f"{c['gold']}x 1st, {c['silver']}x 2nd, {c['bronze']}x 3rd",
                     "points": f"{c['points']:g} pts", "medal": first,
                     "details": f"Participated in {c['seasons']} seasons, total score: {c['points']:g} pts",
                     "finish_date": CAREER, "career": True})  # fmt: skip
        for season in sorted(s for s in seasons if p in seasons[s]):
            rank, total, points = ranks[(p, season)]
            rec = {"player_name": p, "tournament": tournament, "season": f"Season {season}",
                   "division": "Grandmaster", "placement": f"{rank} / {total}", "points": f"{points:g} pts",
                   "medal": _medal(rank), "details": f"Season {season} {standing} ({rank} of {total})",
                   "finish_date": dates.get((series, season), FALLBACK_DATE), "career": False}  # fmt: skip
            rows.append(rec)
            keys[(p, tournament, f"Season {season}")] = rec
    return rows, keys


def _royal_league_rows(official, dates):
    """Returns the Royal League podium rows, latest season first as the organiser's page lists them."""
    rows, keys = [], {}
    podiums = sorted(((season, placed) for (series, season, division), placed in official.items()
                      if series == "royal_league" and division == "Emperor"), reverse=True)  # fmt: skip
    for season, placed in podiums:
        for place in (1, 2, 3):
            points, details = RL_PLACES[place - 1]
            for p in (x["player_name"] for x in placed if x["place"] == place):
                rec = {"player_name": p, "tournament": "Royal League", "season": f"Season {season}",
                       "division": "Emperor", "placement": f"{place} / 8", "points": points,
                       "medal": PLACES[place - 1], "details": f"{details} (Season {season})",
                       "finish_date": dates.get(("royal_league", season), FALLBACK_DATE), "career": False}  # fmt: skip
                rows.append(rec)
                keys[(p, "Royal League", f"Season {season}")] = rec
    return rows, keys


def _honour_rows(honours):
    """Returns the organisers' career Hall of Fame rows: Sodium, then Mercurial, then Q&D."""
    by_series = defaultdict(list)
    for h in honours:
        by_series[h["series"]].append(h)
    rows = []

    def add(h, tournament, season, division, placement, points, medal, details):
        rows.append({"player_name": h["player_name"], "tournament": tournament, "season": season,
                     "division": division, "placement": placement, "points": points, "medal": medal,
                     "details": details, "finish_date": CAREER, "career": True})  # fmt: skip

    for h in by_series["sodium_ladder"]:
        g, s = h["gold"], h["silver"]
        add(h, "Sodium Ladder", "Career Hall of Fame", "Tier 1", f"{g} Titles, {s} Runner-ups",
            f"{h['points']} income tax pts" if h["points"] else "", "gold" if g else ("silver" if s else None),
            f"Sodium Ladder Career Standings: {g} T1 wins, {s} 2nd places")  # fmt: skip
    for h in by_series["mercurial_ladder"]:
        g = h["gold"]
        add(h, "Mercurial Ladder", "Career Hall of Fame", "Premium Tier 1", f"{g} Championships", f"{g} Titles",
            "gold" if g else None, f"Mercurial Ladder Career: {g} Premium Tier 1 wins")  # fmt: skip
    for h in by_series["q_and_d"]:
        g, s, b = h["gold"], h["silver"], h["bronze"]
        add(h, "Quick and Dirty (Q&D)", "Career Trophy Hall of Fame", "Championship",
            f"{g} Gold, {s} Silver, {b} Bronze", f"{g + s + b} Medals",
            "gold" if g else ("silver" if s else ("bronze" if b else None)),
            f"Q&D Hall of Fame Medalist: {g}x Gold, {s}x Silver, {b}x Bronze")  # fmt: skip
    return rows


def _campaign_rows(rows, keys, dates):
    """Returns upstream's campaign rows: every match-name group a player played
    in, and appends each official row's record to it instead where one exists.
    """
    groups = {}
    for r in rows:
        key = record_key(r)
        g = groups.setdefault(key, {"matches": [], "players": set()})
        players = [
            (s.player.strip(), s.culture, s.place) for s in by_place(seats_of(r))
        ]
        g["players"].update(p for p, _, _ in players)
        g["matches"].append(
            {"date": str(r["date"]), "count": r["player_count"], "players": players}
        )

    out = []
    for (tournament, season, division), g in groups.items():
        size = len(g["players"])
        ladder = any(
            w in tournament.lower() or w in division.lower() for w in LADDER_WORDS
        )
        played = {}
        for m in g["matches"]:
            for rank, (p, score, place) in enumerate(m["players"], start=1):
                pc = played.setdefault(
                    p, {"games": 0, "wins": 0, "total": 0, "finish": m["date"]}
                )
                pc["games"] += 1
                pc["wins"] += 1 if place == 1 else 0
                pc["total"] += score
                pc["last"], pc["rank"], pc["count"] = score, rank, m["count"]
                pc["finish"] = max(pc["finish"], m["date"])
        single = len(g["matches"]) == 1
        ranks = {}
        if not ladder and size > 1 and not single:
            ranked = sorted(
                played,
                key=lambda p: (played[p]["wins"], played[p]["total"]),
                reverse=True,
            )
            ranks = {p: i for i, p in enumerate(ranked, start=1)}
        for p, pc in played.items():
            n, w = pc["games"], pc["wins"]
            pct = _pct(w, n)
            finish = finish_date(tournament, season, pc["finish"], dates)
            if ladder or n == 1 or single:
                placement = f"{pc['rank']} / {pc['count']}"
                points = f"{pc['last']:g} pts" if pc["last"] > 0 else f"{w}W - {n - w}L"
            else:
                placement = f"{ranks.get(p, 1)} / {size}"
                points = f"{w}W - {n - w}L ({pct}%)"
            medal = (
                "gold"
                if placement.startswith("1 /")
                and (n >= 5 or size >= 4)
                and is_top_division(tournament, division)
                else None
            )
            official = keys.get((p, tournament, season))
            if official is not None:
                if division and division != "Main":
                    official["division"] = division
                official["details"] += f" • Record: {w}W - {n - w}L ({pct}% win rate)"
                if official["finish_date"] in (FALLBACK_DATE, ""):
                    official["finish_date"] = finish
                continue
            out.append({"player_name": p, "tournament": tournament, "season": season, "division": division,
                        "placement": placement, "points": points, "medal": medal,
                        "details": f"Competed in {division}: {n} matches played, {w} victories ({pct}% win rate)",
                        "finish_date": finish, "career": False})  # fmt: skip
    return out


def player_records(
    *, rows, top_divisions, standings, official, boards, honours, dates
) -> list[dict]:
    """Returns every player's records in the order upstream makes them, each numbered by position.

    rows are the matches in the order upstream's table holds them. standings and official map (series,
    season, division) to rows of player_name, place and points; boards map a
    series to its board rows; top_divisions map a league to its top
    divisions' names; honours are the recorded career honours; dates
    map (series, season) to a finish date.
    """
    worlds, keys = _worlds_rows(official, dates)
    made = list(worlds)
    for series in SHEETS:
        sheet, sheet_keys = _sheet_rows(
            series, top_divisions[series], standings, official, boards, dates
        )
        made += sheet
        keys |= sheet_keys
    rl, rl_keys = _royal_league_rows(official, dates)
    keys |= rl_keys
    made += rl + _honour_rows(honours)
    made += _campaign_rows(rows, keys, dates)
    made = _top_tier_medals(_one_row_per_cup(made))
    return [{**r, "position": i} for i, r in enumerate(made, start=1)]


def is_top_division(tournament: str, division: str) -> bool:
    """Reports whether a group is its tournament's top division, as upstream decides which wins earn gold."""
    if not division:
        return False
    d, t = division.strip().lower(), tournament.strip().lower()
    if "royal league" in t:
        return d == "emperor"
    if "international" in t or "intermezzo" in t:
        return d in ("grandmaster", "championship", "premier")
    if "world" in t:
        return d in ("championship", "final", "finals", "grand final", "main")
    if "ladder" in t:
        return d in (
            "tier 1",
            "tier 01",
            "hydrogen",
            "1-hydrogen",
            "ladder division 1",
            "top tier",
        )
    if "slam" in t or "open" in t:
        return d in ("championship", "final", "finals", "main")
    if "cup" in t or "trophy" in t:
        return d in ("championship", "final", "finals", "stage 11", "stage 3")
    return d in ("championship", "grandmaster", "emperor", "tier 1")


# Upstream's lower-tier medal clearing: (tournaments, divisions that keep a medal, or None for none).
_TOP_TIER = (
    (("Royal League",), ("emperor",)),
    (
        ("International Championship", "Intermezzo Championship"),
        ("grandmaster", "championship", "premier"),
    ),
    (
        ("Mercurial Ladder",),
        ("tier 1", "tier 01", "hydrogen", "mercurial tier", "premium tier 1"),
    ),
    (("Sodium Ladder",), ("tier 1", "tier 01", "01 tier", "1 tier", "sodium tier")),
    (
        ("Premier League", "Duel Curve", "Slow Burn"),
        ("tier 1", "division 1", "championship", "premier", "main"),
    ),
)


def _top_tier_medals(records):
    """Returns the records with medals kept only for each tournament's top tier, as upstream clears them.

    Career rows keep theirs: upstream's clearing also strips "Grandmaster / Hall of Fame", which looks unintended.
    """
    out = []
    for r in records:
        d = (r["division"] or "").lower()
        keep = True
        if r["tournament"].startswith("TCL Round"):
            keep = False
        # A catalogue edition ("Slow Burn 1 (2024)") follows its tournament's rule.
        base = re.sub(r" \d+ \(\d{4}\)$", "", r["tournament"])
        for tournaments, divisions in _TOP_TIER:
            if base in tournaments:
                keep = d in divisions or (
                    r["tournament"] == "Mercurial Ladder" and d.startswith("1-hydrogen")
                )
        out.append(r if keep or r["career"] or not r["medal"] else {**r, "medal": None})
    return out


_CUPS = ("Survivors Cup", "Leaderboard Trophy")


def _stage(division: str) -> int:
    """Returns the stage a cup division names ("Stage 4 - group 2" is 4), or 1."""
    m = re.search(r"Stage\s*(\d+)", division, re.IGNORECASE)
    return int(m.group(1)) if m else 1


def _place(placement: str | None) -> int:
    """Returns the place a placement starts with ("2 / 4" is 2), or upstream's 3."""
    m = re.match(r"(\d+)\s*/", placement or "")
    return int(m.group(1)) if m else 3


def _one_row_per_cup(records):
    """Returns the records with each Survivors Cup and Leaderboard Trophy year as one row per player, last.

    As upstream: ranked by the furthest stage, then the place there, then (Survivors Cup) total points;
    the top three take the medals, out of everyone who played.
    """
    cups, out = {}, []
    for r in records:
        if (
            r["tournament"] in _CUPS
            and not r["career"]
            and r["division"] != "Championship"
        ):
            cups.setdefault((r["tournament"], r["season"]), {}).setdefault(
                r["player_name"], []
            ).append(r)
        else:
            out.append(r)
    for (tournament, year), players in cups.items():
        ranks = []
        for p, rows in players.items():
            furthest = max(_stage(r["division"]) for r in rows)
            last = [r for r in rows if _stage(r["division"]) == furthest]
            place = min(_place(r["placement"]) for r in last)
            games = wins = points = 0
            for r in rows:
                m = re.search(
                    r"(\d+)\s+matches played,\s+(\d+)\s+victories", r["details"] or ""
                )
                games, wins = (
                    games + (int(m.group(1)) if m else 1),
                    wins + (int(m.group(2)) if m else 0),
                )
                if m := re.search(r"(\d+)\s*pts", r["points"] or ""):
                    points += int(m.group(1))
            finish = max(r["finish_date"] for r in last)
            ranks.append((p, furthest, place, points, games, wins, finish))
        survivors = tournament == "Survivors Cup"
        ranks.sort(key=lambda x: (-x[1], x[2], -x[3] if survivors else 0))
        for i, (p, furthest, _, points, games, wins, finish) in enumerate(
            ranks, start=1
        ):
            details = f"Advanced to Stage {furthest} • {games} matches played, {wins} victories ({_pct(wins, games)}% win rate)"
            if survivors:
                details += f" • Cumulative Score: {points:,} pts"
            out.append({"player_name": p, "tournament": tournament, "season": year, "division": "Championship",
                        "placement": f"{i} / {len(ranks)}", "points": f"{points} pts" if survivors else "",
                        "medal": _medal(i), "details": details, "finish_date": finish, "career": False})  # fmt: skip
    return out
